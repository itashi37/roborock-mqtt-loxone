package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mqtt-home/roborock-mqtt/config"
	"github.com/mqtt-home/roborock-mqtt/integration/autoupdate"
	bridgehealth "github.com/mqtt-home/roborock-mqtt/integration/health"
	"github.com/mqtt-home/roborock-mqtt/integration/localmqtt"
	"github.com/mqtt-home/roborock-mqtt/integration/updates"
	"github.com/mqtt-home/roborock-mqtt/integration/watchdog"
	loxonedirect "github.com/mqtt-home/roborock-mqtt/loxone/direct"
	"github.com/mqtt-home/roborock-mqtt/roborock"
	"github.com/mqtt-home/roborock-mqtt/supervisor"
	"github.com/mqtt-home/roborock-mqtt/updater"
	"github.com/mqtt-home/roborock-mqtt/version"
	"github.com/mqtt-home/roborock-mqtt/web"
	"github.com/philipparndt/go-logger"
)

var (
	deviceManager         *roborock.DeviceManager
	scheduleEngine        *roborock.ScheduleEngine
	notAtHomeStore        *roborock.NotAtHomeStore
	scheduleStore         *roborock.ScheduleStore
	maintenanceChecker    *roborock.MaintenanceChecker
	webServer             *web.WebServer
	stopPolling           chan struct{}
	stopSchedule          chan struct{}
	dataDir               string
	loxoneCoreStore       = roborock.NewLoxoneCoreStore()
	loxoneActivityTracker *roborock.LoxoneActivityTracker
	loxoneDiagnostics     = roborock.NewLoxoneDiagnosticStore(50)
	loxoneRoomOverrides   *roborock.LoxoneRoomOverrideStore
	loxoneProbe           = newMQTTLoopbackProbe()
	loxonePublishMu       sync.Mutex
	deviceStateStore      = roborock.NewDeviceStateStore()
	capabilityStore       = roborock.NewCapabilityStore()
	commandCoordinator    *roborock.CommandCoordinator
	directSynchronizer    *loxonedirect.Synchronizer
	localBroker           = localmqtt.New()
	scheduleSignals       *roborock.SignalListener
	integrationMu         sync.Mutex
	healthPublisher       *bridgehealth.Publisher
	watchdogMonitor       *watchdog.Monitor
	watchdogRecovery      atomic.Bool
	processStartedAt      = time.Now()
	updateChecker         = updates.NewChecker(version.Version, version.GitCommit)
	updaterClient         *updates.UpdaterClient
	autoUpdateScheduler   *autoupdate.Scheduler
	processSupervisor     *supervisor.Runtime
)

func automaticUpdateGuard() autoupdate.GuardState {
	guard := autoupdate.GuardState{}
	for _, state := range deviceStateStore.All() {
		if state.Status == nil {
			continue
		}
		if state.Status.InCleaning {
			guard.Cleaning, guard.RobotActive = true, true
		}
		switch strings.ToLower(state.Status.State) {
		case "cleaning", "spot_cleaning", "segment_cleaning", "zoned_cleaning", "paused", "returning", "returning_home", "going_to_target", "washing_mop", "going_to_wash_mop", "emptying_dustbin", "servicing_dock", "mapping", "manual":
			guard.RobotActive = true
		}
	}
	if commandCoordinator != nil {
		guard.CommandsInFlight = commandCoordinator.Diagnostics().InFlight
	}
	return guard
}

func startAutomaticUpdates() {
	if autoUpdateScheduler != nil {
		autoUpdateScheduler.Stop()
	}
	autoUpdateScheduler = autoupdate.NewScheduler(autoupdate.Dependencies{
		Settings: func() config.UpdateConfig { return config.Get().Updates },
		Check: func(ctx context.Context, channel string) (updates.Info, error) {
			return updateChecker.Check(ctx, channel)
		},
		Install: startManualUpdate,
		Operation: func(ctx context.Context) (updater.Operation, error) {
			if updaterClient == nil {
				return updater.Operation{}, fmt.Errorf("isolated updater is not configured")
			}
			return updaterClient.Status(ctx)
		},
		Guard: automaticUpdateGuard, DataDir: dataDir,
	})
	autoUpdateScheduler.Start()
}

func configureUpdaterClient() error {
	tokenPath := filepath.Join(dataDir, "updater-token")
	token, err := os.ReadFile(tokenPath)
	if os.IsNotExist(err) {
		buffer := make([]byte, 32)
		if _, err := rand.Read(buffer); err != nil {
			return fmt.Errorf("generate updater token: %w", err)
		}
		encoded := fmt.Sprintf("%x", buffer)
		if err := os.WriteFile(tokenPath, []byte(encoded+"\n"), 0600); err != nil {
			return fmt.Errorf("persist updater token: %w", err)
		}
		token = []byte(encoded)
	} else if err != nil {
		return fmt.Errorf("read updater token: %w", err)
	}
	if err := os.Chmod(tokenPath, 0600); err != nil {
		return fmt.Errorf("secure updater token: %w", err)
	}
	endpoint := strings.TrimSpace(os.Getenv("ROBOROCK_UPDATER_URL"))
	if endpoint == "" {
		endpoint = "http://updater:8090"
	}
	client, err := updates.NewUpdaterClient(endpoint, string(token))
	if err != nil {
		return err
	}
	updaterClient = client
	return nil
}

func startManualUpdate(ctx context.Context, channel string) (updater.Operation, error) {
	if updaterClient == nil {
		return updater.Operation{}, fmt.Errorf("isolated updater is not configured")
	}
	info, err := updateChecker.Check(ctx, channel)
	if err != nil {
		return updater.Operation{}, err
	}
	if !info.Available || info.LatestVersion == "" {
		return updater.Operation{}, fmt.Errorf("no update is available on the %s channel", channel)
	}
	if !info.ArtifactReady {
		return updater.Operation{}, fmt.Errorf("the %s update image is not published yet (status: %s)", channel, info.ArtifactStatus)
	}
	tag, expected := "edge", "edge"
	expectedCommit := info.LatestCommit
	if channel != "edge" {
		tag, expected = "v"+strings.TrimPrefix(info.LatestVersion, "v"), strings.TrimPrefix(info.LatestVersion, "v")
		expectedCommit = ""
	}
	requestIDBytes := make([]byte, 16)
	if _, err := rand.Read(requestIDBytes); err != nil {
		return updater.Operation{}, fmt.Errorf("generate update request ID: %w", err)
	}
	return updaterClient.Start(ctx, updater.Request{RequestID: fmt.Sprintf("web-%x", requestIDBytes), Tag: tag, ExpectedVersion: expected, ExpectedCommit: expectedCommit})
}

func installedChannel() string {
	value := strings.ToLower(strings.TrimSpace(version.Version))
	if value == "dev" || value == "edge" || strings.Contains(value, "edge") || strings.Contains(value, "dirty") {
		return "edge"
	}
	return "stable"
}

func dataVolumeStatus() web.DataVolumeStatus {
	status := web.DataVolumeStatus{Path: dataDir}
	temporary, err := os.CreateTemp(dataDir, ".health-*")
	if err != nil {
		status.Error = err.Error()
		return status
	}
	name := temporary.Name()
	if closeErr := temporary.Close(); closeErr != nil {
		status.Error = closeErr.Error()
		return status
	}
	if removeErr := os.Remove(name); removeErr != nil {
		status.Error = removeErr.Error()
		return status
	}
	status.Writable = true
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(dataDir, &filesystem); err != nil {
		status.Error = err.Error()
		return status
	}
	status.FreeBytes = uint64(filesystem.Bavail) * uint64(filesystem.Bsize)
	return status
}

func systemStatusSnapshot() web.SystemStatus {
	now := time.Now()
	health := watchdog.Assess(watchdog.Observation{ObservedAt: now, StartedAt: processStartedAt}, watchdogConfig(config.Get().Watchdog))
	if watchdogMonitor != nil {
		health = watchdogMonitor.Report()
	}
	mqttDiagnostics := localBroker.Diagnostics()
	directDiagnostics := directDiagnosticsSnapshot()
	failedDirectInputs := make([]web.SystemInputFailure, 0)
	for _, input := range directDiagnostics.Inputs {
		if input.LastError == "" {
			continue
		}
		failedDirectInputs = append(failedDirectInputs, web.SystemInputFailure{
			Name: input.Input, Field: input.Field, Kind: string(input.Kind), Error: input.LastError,
		})
	}
	status := web.SystemStatus{
		Product: "roborock-mqtt-loxone", Version: version.Version, GitCommit: version.GitCommit,
		BuildTime: version.BuildTime, GoVersion: version.GoVersion, Architecture: runtime.GOOS + "/" + runtime.GOARCH,
		Channel: installedChannel(), UptimeSeconds: int64(now.Sub(processStartedAt).Seconds()),
		StartedAt: processStartedAt, LastRestart: processStartedAt, LastWatchdogReason: health.LastWatchdogReason,
		Health: health, DataVolume: dataVolumeStatus(), Update: updateChecker.Last(),
		Transports: map[string]web.SystemTransportStatus{
			"mqtt":   {Enabled: localMQTTEnabled(), Connected: mqttDiagnostics.Connected, LastSuccess: mqttDiagnostics.ConnectedAt, LastError: mqttDiagnostics.LastError},
			"direct": {Enabled: config.Get().Loxone.Direct.Enabled, Connected: !directDiagnostics.LastTransmission.IsZero() && directDiagnostics.LastError == "", LastSuccess: directDiagnostics.LastTransmission, LastError: directDiagnostics.LastError, FailedInputs: failedDirectInputs},
		},
	}
	status.UpdateSettings = config.Get().Updates
	if autoUpdateScheduler != nil {
		status.AutoUpdate = autoUpdateScheduler.Diagnostics()
	}
	if processSupervisor != nil {
		status.Supervisor = processSupervisor.Status()
	}
	return status
}

func watchdogConfig(cfg config.WatchdogConfig) watchdog.Config {
	return watchdog.Config{
		Enabled: cfg.IsEnabled(), CheckInterval: time.Duration(cfg.CheckIntervalSeconds) * time.Second,
		StaleAfter:         time.Duration(cfg.StaleAfterSeconds) * time.Second,
		ReconnectAfter:     time.Duration(cfg.ReconnectAfterSeconds) * time.Second,
		RebuildAfter:       time.Duration(cfg.RebuildAfterSeconds) * time.Second,
		ResetAfter:         time.Duration(cfg.ResetAfterSeconds) * time.Second,
		RestartAfter:       time.Duration(cfg.RestartAfterSeconds) * time.Second,
		RecoveryHysteresis: cfg.RecoveryHysteresisChecks, MaxRestarts: cfg.MaxRestartsPerHour,
		RestartWindow: time.Hour, MaxQueueDepth: cfg.MaxQueueDepth,
	}
}

func watchdogObservation(restClient *roborock.Client, now time.Time) watchdog.Observation {
	observation := watchdog.Observation{ObservedAt: now, StartedAt: processStartedAt, Authenticated: restClient.IsAuthenticated()}
	if deviceManager != nil {
		observation.BridgeStarted = true
		observation.RoborockLoopLastActive = deviceManager.LoopLastActive()
		observation.CloudConnected = deviceManager.ConnectedCount() > 0
		observation.LastCloudMessage = deviceManager.LastCloudMessage()
		observation.LastRobotUpdate = deviceManager.LastRobotUpdate()
	}
	if commandCoordinator != nil {
		diagnostics := commandCoordinator.Diagnostics()
		observation.DispatcherInFlight = diagnostics.InFlight
		observation.DispatcherOldest = diagnostics.Oldest
		observation.DispatcherLastCompleted = diagnostics.LastCompleted
	}
	mqttDiagnostics := localBroker.Diagnostics()
	observation.LocalMQTTEnabled = localMQTTEnabled()
	observation.LocalMQTTConnected = mqttDiagnostics.Connected
	observation.DirectEnabled = config.Get().Loxone.Direct.Enabled
	if observation.DirectEnabled {
		diagnostics := directDiagnosticsSnapshot()
		observation.DirectPending = diagnostics.Pending
		observation.DirectLastSuccess = diagnostics.LastTransmission
		observation.DirectLastError = diagnostics.LastError
	}
	return observation
}

func runWatchdogRecovery(action, reason string, restClient *roborock.Client) {
	if !watchdogRecovery.CompareAndSwap(false, true) {
		logger.Warn("Watchdog recovery already running", "requested", action)
		return
	}
	go func() {
		defer watchdogRecovery.Store(false)
		logger.Warn("Watchdog recovery", "action", action, "reason", reason)
		if deviceManager != nil {
			deviceManager.ReconnectAll()
		}
		if action == "rebuild" || action == "reset" {
			integrationMu.Lock()
			configureDirectLoxone()
			if err := configureLocalMQTT(restClient); err != nil {
				logger.Warn("Watchdog integration rebuild failed", "error", err)
			}
			integrationMu.Unlock()
		}
	}()
}

func startWatchdog(restClient *roborock.Client) {
	if watchdogMonitor != nil {
		watchdogMonitor.Stop()
	}
	guard := watchdog.NewRestartGuard(dataDir)
	watchdogMonitor = watchdog.NewMonitor(
		watchdogConfig(config.Get().Watchdog),
		func(now time.Time) watchdog.Observation { return watchdogObservation(restClient, now) },
		watchdog.Actions{
			Reconnect: func(reason string) { runWatchdogRecovery("reconnect", reason, restClient) },
			Rebuild:   func(reason string) { runWatchdogRecovery("rebuild", reason, restClient) },
			Reset:     func(reason string) { runWatchdogRecovery("reset", reason, restClient) },
			Exit: func(reason string) {
				if processSupervisor != nil {
					processSupervisor.RequestRestart(reason)
				}
			},
		}, guard,
	)
	watchdogMonitor.Start()
}

func localMQTTEnabled() bool { return config.Get().MQTT.IsEnabled() }

func deviceIntegrationModes(slug string) (bool, bool) {
	deviceID := ""
	if deviceManager != nil {
		if device := deviceManager.GetDevice(slug); device != nil {
			deviceID = device.Info.DID
		}
	}
	return config.Get().Loxone.DeviceModes(deviceID, slug)
}

func localMQTTEnabledFor(slug string) bool {
	mqttEnabled, _ := deviceIntegrationModes(slug)
	return localMQTTEnabled() && mqttEnabled
}

func directLoxoneEnabledFor(slug string) bool {
	_, directEnabled := deviceIntegrationModes(slug)
	return config.Get().Loxone.Direct.Enabled && directEnabled
}

func loxoneMQTTEnabledFor(slug string) bool {
	return localMQTTEnabledFor(slug) && config.Get().Loxone.Enabled
}

func publishLocalMQTT(topic string, payload any, retained bool) {
	if err := localBroker.Publish(topic, payload, retained); err != nil {
		logger.Warn("Local MQTT publish failed", "topic", topic, "error", err)
	}
}

type mqttLoopbackProbe struct {
	mu      sync.Mutex
	waiters map[string]chan struct{}
}

func newMQTTLoopbackProbe() *mqttLoopbackProbe {
	return &mqttLoopbackProbe{waiters: make(map[string]chan struct{})}
}

func (p *mqttLoopbackProbe) receive(payload []byte) {
	nonce := string(payload)
	p.mu.Lock()
	channel := p.waiters[nonce]
	if channel != nil {
		delete(p.waiters, nonce)
		close(channel)
	}
	p.mu.Unlock()
}

func (p *mqttLoopbackProbe) test(ctx context.Context) error {
	if !localMQTTEnabled() {
		return fmt.Errorf("local MQTT integration is disabled")
	}
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Errorf("generate MQTT test nonce: %w", err)
	}
	nonce := fmt.Sprintf("%x", buffer)
	channel := make(chan struct{})
	p.mu.Lock()
	p.waiters[nonce] = channel
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.waiters, nonce)
		p.mu.Unlock()
	}()
	if err := localBroker.Publish(loxoneTopic("_bridge", "diagnostic"), nonce, false); err != nil {
		return err
	}
	select {
	case <-channel:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("MQTT loopback timed out: %w", ctx.Err())
	}
}

func publishDeviceMap(slug string, pngData []byte) {
	if !localMQTTEnabledFor(slug) {
		return
	}
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/map"
	publishLocalMQTT(topic, pngData, cfg.MQTT.Retain)
	logger.Debug("Published map", "device", slug, "topic", topic, "size", len(pngData))
}

func publishDeviceCurrentRoom(slug string, room *roborock.CurrentRoom) {
	if !localMQTTEnabledFor(slug) {
		return
	}
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/current_room"

	data, err := json.Marshal(room)
	if err != nil {
		logger.Error("Failed to marshal current room", "error", err)
		return
	}

	publishLocalMQTT(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published current room", "device", slug, "topic", topic, "payload", string(data))
}

func loxoneTopic(slug, suffix string) string {
	base := strings.TrimSuffix(strings.TrimSpace(config.Get().Loxone.Topic), "/")
	return base + "/" + slug + "/" + suffix
}

func bridgeHealthSnapshot(now time.Time) bridgehealth.Snapshot {
	snapshot := bridgehealth.Snapshot{BridgeAlive: true, RobotOnline: map[string]bool{}, Heartbeat: now.Unix()}
	if deviceManager == nil {
		return snapshot
	}
	snapshot.CloudConnected = deviceManager.ConnectedCount() > 0
	for _, state := range deviceStateStore.All() {
		snapshot.RobotOnline[state.Slug] = state.RobotOnline
	}
	return snapshot
}

func directInstallationHealthValues(snapshot bridgehealth.Snapshot) []loxonedirect.StateValue {
	mapping := loxonedirect.InputMapping{Prefix: config.Get().Loxone.Direct.InputPrefix, Overrides: config.Get().Loxone.Direct.Inputs}
	digital := func(value bool) string {
		if value {
			return "1"
		}
		return "0"
	}
	return []loxonedirect.StateValue{
		loxonedirect.InstallationValue("bridge_alive", loxonedirect.Digital, digital(snapshot.BridgeAlive), mapping),
		loxonedirect.InstallationValue("cloud_connected", loxonedirect.Digital, digital(snapshot.CloudConnected), mapping),
		loxonedirect.InstallationValue("bridge_heartbeat", loxonedirect.Analog, strconv.FormatInt(snapshot.Heartbeat, 10), mapping),
	}
}

func publishBridgeHealth(snapshot bridgehealth.Snapshot) {
	if localMQTTEnabled() && config.Get().Loxone.Enabled {
		alive := "0"
		if snapshot.BridgeAlive {
			alive = "1"
		}
		cloud := "0"
		if snapshot.CloudConnected {
			cloud = "1"
		}
		publishLocalMQTT(loxoneTopic("_bridge", "bridge_alive"), alive, true)
		publishLocalMQTT(loxoneTopic("_bridge", "cloud_connected"), cloud, true)
		publishLocalMQTT(loxoneTopic("_bridge", "bridge_heartbeat"), strconv.FormatInt(snapshot.Heartbeat, 10), false)
		for slug, online := range snapshot.RobotOnline {
			payload := "0"
			if online {
				payload = "1"
			}
			publishLoxoneScalar(slug, "robot_online", payload)
		}
	}
	integrationMu.Lock()
	defer integrationMu.Unlock()
	if directSynchronizer != nil {
		for _, value := range directInstallationHealthValues(snapshot) {
			directSynchronizer.UpdateInstallation(value, false)
		}
	}
}

func startHealthPublisher() {
	if healthPublisher != nil {
		healthPublisher.Stop()
	}
	healthPublisher = bridgehealth.NewPublisher(bridgehealth.DefaultHeartbeatInterval, bridgeHealthSnapshot, publishBridgeHealth)
	healthPublisher.Start()
}

func expectedRetainedTopics() []string {
	if deviceManager == nil || !localMQTTEnabled() {
		return nil
	}
	cfg := config.Get()
	topics := make([]string, 0)
	legacySuffixes := []string{"availability", "status", "map", "current_room", "scenes", "schedule"}
	loxoneSuffixes := []string{
		"online", "state", "battery", "current_room_id", "current_room_name", "clean_area", "clean_time_seconds",
		"error_code", "error_text", "last_seen", "maintenance/main_brush", "maintenance/side_brush",
		"maintenance/filter", "maintenance/sensor", "core", "last_command",
		"robot_online",
	}
	bridgeSuffixes := []string{"bridge_alive", "cloud_connected"}
	for _, device := range deviceManager.GetDevices() {
		if !localMQTTEnabledFor(device.Slug) {
			continue
		}
		base := strings.TrimSuffix(cfg.MQTT.Topic, "/") + "/" + device.Slug + "/"
		for _, suffix := range legacySuffixes {
			topics = append(topics, base+suffix)
		}
		if loxoneMQTTEnabledFor(device.Slug) {
			for _, suffix := range loxoneSuffixes {
				topics = append(topics, loxoneTopic(device.Slug, suffix))
			}
		}
	}
	if cfg.Loxone.Enabled {
		for _, suffix := range bridgeSuffixes {
			topics = append(topics, loxoneTopic("_bridge", suffix))
		}
	}
	return topics
}

func cleanupStaleRetainedTopics() {
	if !localMQTTEnabled() {
		return
	}
	ledger, err := roborock.NewRetainedTopicLedger(dataDir)
	if err != nil {
		logger.Warn("Failed to load retained topic ledger", "error", err)
		return
	}
	stale, err := ledger.Reconcile(expectedRetainedTopics())
	if err != nil {
		logger.Warn("Failed to update retained topic ledger", "error", err)
		return
	}
	for _, topic := range stale {
		publishLocalMQTT(topic, "", true)
		logger.Info("Cleared obsolete retained MQTT topic", "topic", topic)
	}
}

func publishLoxoneScalar(slug, suffix, payload string) {
	if !loxoneMQTTEnabledFor(slug) {
		return
	}
	topic := loxoneTopic(slug, suffix)
	publishLocalMQTT(topic, payload, true)
	logger.Debug("Published Loxone scalar", "device", slug, "topic", topic, "payload", payload)
}

func publishLoxoneCore(slug string, core roborock.LoxoneCore) {
	data, err := roborock.MarshalLoxoneCore(core)
	if err != nil {
		logger.Error("Failed to marshal Loxone core", "device", slug, "error", err)
		return
	}
	publishLoxoneScalar(slug, "core", string(data))
}

func publishLoxoneActivities(slug string, activities []roborock.LoxoneActivity) {
	for _, activity := range activities {
		data, err := roborock.MarshalLoxoneActivity(activity)
		if err != nil {
			logger.Error("Failed to marshal Loxone activity", "device", slug, "error", err)
			continue
		}
		if loxoneMQTTEnabledFor(slug) {
			publishLocalMQTT(loxoneTopic(slug, "activity"), string(data), false)
			if activity.Type == "command" {
				publishLocalMQTT(loxoneTopic(slug, "last_command"), string(data), true)
			}
		}
		loxoneDiagnostics.Record(slug, activity)
		if webServer != nil {
			webServer.BroadcastLoxoneActivity(slug, activity)
		}
		logger.Debug("Published Loxone activity", "device", slug, "payload", string(data))
	}
}

func loxoneRoomNames(dev *roborock.ManagedDevice, restClient *roborock.Client) map[string]string {
	cfg := config.Get()
	overrides := map[string]string{}
	if loxoneRoomOverrides != nil {
		overrides = loxoneRoomOverrides.ForDevice(dev.Slug)
	}
	return roborock.CommandableRoomNames(
		dev.GetRoomMappings(),
		restClient.GetRoomNameMap(),
		cfg.Roborock.RoomNames[dev.Info.Name],
		overrides,
	)
}

func refreshLoxoneCurrentRoom(slug string, restClient *roborock.Client) {
	if deviceManager == nil {
		return
	}
	dev := deviceManager.GetDevice(slug)
	if dev == nil {
		return
	}
	room, err := roborock.CurrentRoomFromVectorJSON(dev.GetVectorMapJSON(), loxoneRoomNames(dev, restClient))
	if err != nil {
		logger.Warn("Failed to refresh Loxone current room", "device", slug, "error", err)
		return
	}
	deviceStateStore.UpdateCurrentRoom(slug, room, time.Now())
}

func wireLoxoneWeb(restClient *roborock.Client) {
	if webServer == nil {
		return
	}
	webServer.SetLoxoneIntegration(&web.LoxoneDependencies{
		Core:          loxoneCoreStore,
		Diagnostics:   loxoneDiagnostics,
		RoomOverrides: loxoneRoomOverrides,
		Capabilities:  capabilityStore,
		PublishCommand: func(slug, command string) error {
			if !config.Get().Loxone.Enabled {
				return fmt.Errorf("Loxone mode is disabled")
			}
			if commandCoordinator == nil {
				return fmt.Errorf("command coordinator is not ready")
			}
			result := commandCoordinator.SubmitText(slug, command)
			if result.State == "failed" {
				return fmt.Errorf("%s", result.Error)
			}
			return nil
		},
		TestMQTT: loxoneProbe.test,
		RefreshRoom: func(slug string) {
			refreshLoxoneCurrentRoom(slug, restClient)
		},
		DirectDiagnostics: directDiagnosticsSnapshot,
		ResendDirect:      resendDirectSnapshots,
		SubmitCommand: func(slug, command string) roborock.CommandSubmission {
			if !directLoxoneEnabledFor(slug) {
				return roborock.CommandSubmission{Command: command, State: "failed", Error: "Direct Loxone is disabled for this robot", Failure: "not_found"}
			}
			if commandCoordinator == nil {
				return roborock.CommandSubmission{Command: command, State: "failed", Error: "command coordinator is not ready", Failure: "conflict"}
			}
			return commandCoordinator.SubmitText(slug, command)
		},
		FindCommand: loxoneDiagnostics.FindCommand,
	})
}

func directClientFromConfig(cfg config.DirectLoxoneConfig) (*loxonedirect.Client, error) {
	return loxonedirect.NewClient(loxonedirect.ClientConfig{
		Scheme: cfg.Scheme, Host: cfg.Host, Port: cfg.Port,
		Username: cfg.Username, Password: cfg.Password,
		Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
	})
}

func configureDirectLoxone() {
	if directSynchronizer != nil {
		directSynchronizer.Close()
		directSynchronizer = nil
	}
	cfg := config.Get()
	if !cfg.Loxone.Direct.Enabled {
		return
	}
	directClient, err := directClientFromConfig(cfg.Loxone.Direct)
	if err != nil {
		logger.Error("Direct Loxone configuration is invalid", "error", err)
		return
	}
	directSynchronizer = loxonedirect.NewSynchronizer(
		directClient,
		loxonedirect.InputMapping{Prefix: cfg.Loxone.Direct.InputPrefix, Overrides: cfg.Loxone.Direct.Inputs},
		cfg.Loxone.Direct.MaxRetries,
		time.Duration(cfg.Loxone.Direct.RetryDelayMS)*time.Millisecond,
		func() []roborock.InternalDeviceState {
			states := deviceStateStore.All()
			filtered := make([]roborock.InternalDeviceState, 0, len(states))
			for _, state := range states {
				if directLoxoneEnabledFor(state.Slug) {
					filtered = append(filtered, state)
				}
			}
			return filtered
		},
	)
	directSynchronizer.SetInstallationValues(func() []loxonedirect.StateValue {
		return directInstallationHealthValues(bridgeHealthSnapshot(time.Now()))
	})
	directSynchronizer.ResendAll()
}

func directDiagnosticsSnapshot() loxonedirect.SyncDiagnostics {
	integrationMu.Lock()
	defer integrationMu.Unlock()
	if directSynchronizer == nil {
		return loxonedirect.SyncDiagnostics{Inputs: []loxonedirect.InputDiagnostic{}}
	}
	return directSynchronizer.Diagnostics()
}

func resendDirectSnapshots() {
	integrationMu.Lock()
	defer integrationMu.Unlock()
	if directSynchronizer != nil {
		directSynchronizer.ResendAll()
	}
}

func updateDirectState(state roborock.InternalDeviceState) {
	integrationMu.Lock()
	defer integrationMu.Unlock()
	if directSynchronizer != nil && directLoxoneEnabledFor(state.Slug) {
		directSynchronizer.Update(state)
	}
}

func restoreLastCommandSubscriptions() {
	if deviceManager == nil || !config.Get().Loxone.Enabled {
		return
	}
	for _, device := range deviceManager.GetDevices() {
		dev := device
		if !loxoneMQTTEnabledFor(dev.Slug) {
			continue
		}
		_ = localBroker.Subscribe(loxoneTopic(dev.Slug, "last_command"), func(_ string, payload []byte) {
			var activity roborock.LoxoneActivity
			if err := json.Unmarshal(payload, &activity); err == nil {
				loxoneDiagnostics.RestoreLastCommand(dev.Slug, activity)
			}
		})
	}
}

func republishLocalSnapshots() {
	if deviceManager == nil || !localBroker.IsConnected() {
		return
	}
	for _, state := range deviceStateStore.All() {
		publishDeviceAvailability(state.Slug, state.Online)
		publishLoxoneAvailability(state.Slug, state.Online)
		if state.Status != nil {
			publishDeviceStatus(state.Slug, state.Status)
			publishLoxoneStatus(state.Slug, state.Status)
		}
		publishDeviceCurrentRoom(state.Slug, state.CurrentRoom)
		publishLoxoneCurrentRoom(state.Slug, state.CurrentRoom)
		if len(state.Scenes) > 0 {
			publishDeviceScenes(state.Slug, state.Scenes)
		}
		if device := deviceManager.GetDevice(state.Slug); device != nil && device.GetMapPNG() != nil {
			publishDeviceMap(state.Slug, device.GetMapPNG())
		}
	}
}

func configureLocalMQTT(restClient *roborock.Client) error {
	localBroker.Stop()
	localBroker.ClearSubscriptions()
	if !localMQTTEnabled() {
		return nil
	}
	cfg := config.Get()
	var lastWill *localmqtt.LastWill
	if cfg.Loxone.Enabled {
		lastWill = &localmqtt.LastWill{
			Topic: loxoneTopic("_bridge", "bridge_alive"), OfflinePayload: "0", OnlinePayload: "1", Retained: true,
		}
	}
	if err := localBroker.StartWithWill(cfg.MQTT, lastWill); err != nil {
		return err
	}
	_ = localBroker.Subscribe(loxoneTopic("_bridge", "diagnostic"), func(_ string, payload []byte) {
		loxoneProbe.receive(payload)
	})
	if deviceManager != nil {
		subscribeToCommands()
		subscribeToLoxoneCommands(restClient)
		restoreLastCommandSubscriptions()
		cleanupStaleRetainedTopics()
		republishLocalSnapshots()
	}
	if scheduleSignals != nil {
		scheduleSignals.Subscribe()
	}
	return nil
}

func applyRuntimeSettings(restClient *roborock.Client, settings config.RuntimeSettings) error {
	integrationMu.Lock()
	if err := config.SaveRuntimeSettings(settings); err != nil {
		integrationMu.Unlock()
		return err
	}
	restClient.SetUsername(settings.RoborockUsername)
	configureDirectLoxone()
	if err := configureLocalMQTT(restClient); err != nil {
		integrationMu.Unlock()
		logger.Warn("Local MQTT reconfiguration failed; Direct Loxone remains available", "error", err)
		return fmt.Errorf("save succeeded but local MQTT connection failed: %w", err)
	}
	integrationMu.Unlock()
	if healthPublisher != nil {
		healthPublisher.PublishNow()
	}
	return nil
}

func wireIntegrationSettings(restClient *roborock.Client) {
	if webServer == nil {
		return
	}
	webServer.SetIntegrationSettings(&web.IntegrationSettingsDependencies{
		Apply:      func(settings config.RuntimeSettings) error { return applyRuntimeSettings(restClient, settings) },
		MQTTStatus: localBroker.Diagnostics,
		TestMQTT: func(ctx context.Context, mqttConfig config.MQTTConfig) error {
			temporary := localmqtt.New()
			if err := temporary.Start(mqttConfig); err != nil {
				return err
			}
			defer temporary.Stop()
			return temporary.Test(ctx, strings.TrimSuffix(mqttConfig.Topic, "/")+"/_bridge/setup-test")
		},
		TestDirect: func(ctx context.Context, directConfig config.DirectLoxoneConfig) error {
			client, err := directClientFromConfig(directConfig)
			if err != nil {
				return err
			}
			return client.Test(ctx)
		},
	})
}

func publishLoxoneActivity(slug string, activity *roborock.LoxoneActivity) {
	if activity != nil {
		publishLoxoneActivities(slug, []roborock.LoxoneActivity{*activity})
	}
}

func publishLoxoneStatus(slug string, status *roborock.PublishedStatus) {
	loxonePublishMu.Lock()
	defer loxonePublishMu.Unlock()

	observedAt := time.Now()
	for suffix, payload := range roborock.LoxoneStatusScalars(status, observedAt) {
		publishLoxoneScalar(slug, suffix, payload)
	}
	publishLoxoneCore(slug, loxoneCoreStore.UpdateStatus(slug, status, observedAt))
}

func publishLoxoneAvailability(slug string, online bool) {
	loxonePublishMu.Lock()
	defer loxonePublishMu.Unlock()

	payload := "0"
	if online {
		payload = "1"
	}
	publishLoxoneScalar(slug, "online", payload)
	if !online {
		publishLoxoneScalar(slug, "state", "offline")
	}
	publishLoxoneCore(slug, loxoneCoreStore.UpdateAvailability(slug, online))
}

func publishLoxoneCurrentRoom(slug string, room *roborock.CurrentRoom) {
	loxonePublishMu.Lock()
	defer loxonePublishMu.Unlock()

	for suffix, payload := range roborock.LoxoneCurrentRoomScalars(room) {
		publishLoxoneScalar(slug, suffix, payload)
	}
	publishLoxoneCore(slug, loxoneCoreStore.UpdateCurrentRoom(slug, room))
}

func publishDeviceScenes(slug string, scenes []roborock.Scene) {
	if !localMQTTEnabledFor(slug) {
		return
	}
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/scenes"

	data, err := json.Marshal(scenes)
	if err != nil {
		logger.Error("Failed to marshal scenes", "error", err)
		return
	}

	publishLocalMQTT(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published scenes", "device", slug, "topic", topic, "count", len(scenes))
}

func publishDeviceSchedule(slug string, state *roborock.ScheduleState) {
	if !localMQTTEnabledFor(slug) {
		return
	}
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/schedule"

	data, err := json.Marshal(state)
	if err != nil {
		logger.Error("Failed to marshal schedule state", "error", err)
		return
	}

	publishLocalMQTT(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published schedule state", "device", slug, "topic", topic, "dayType", state.ActiveDay)
}

// publishDeviceAvailability publishes a device's cloud-connection state to the
// local broker as a retained `<topic>/<slug>/availability` message. Consumers
// (e.g. the wall-display Wall API) use this to mark a device unavailable rather
// than trust a stale retained `<topic>/<slug>/status`.
func publishDeviceAvailability(slug string, online bool) {
	if !localMQTTEnabledFor(slug) {
		return
	}
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/availability"
	payload := "offline"
	if online {
		payload = "online"
	}
	// Availability is always retained so late subscribers see the current state.
	publishLocalMQTT(topic, payload, true)
	logger.Debug("Published availability", "device", slug, "topic", topic, "state", payload)
}

func publishDeviceStatus(slug string, status *roborock.PublishedStatus) {
	if !localMQTTEnabledFor(slug) {
		return
	}
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/status"

	data, err := json.Marshal(status)
	if err != nil {
		logger.Error("Failed to marshal status", "error", err)
		return
	}

	publishLocalMQTT(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published status", "device", slug, "topic", topic)
}

func subscribeToCommands() {
	if !localMQTTEnabled() || commandCoordinator == nil {
		return
	}
	cfg := config.Get()

	for _, md := range deviceManager.GetDevices() {
		dev := md // capture
		if !localMQTTEnabledFor(dev.Slug) {
			continue
		}
		topic := cfg.MQTT.Topic + "/" + dev.Slug + "/set"

		logger.Info("Subscribing to MQTT commands", "device", dev.Slug, "topic", topic)

		_ = localBroker.Subscribe(topic, func(topic string, payload []byte) {
			logger.Debug("Received MQTT command", "device", dev.Slug, "topic", topic, "payload", string(payload))

			var cmd struct {
				Action   string `json:"action"`
				Segments []int  `json:"segments,omitempty"`
				Speed    string `json:"speed,omitempty"`
				Mode     string `json:"mode,omitempty"`
				Level    string `json:"level,omitempty"`
				SceneID  int    `json:"scene_id,omitempty"`
			}

			if err := json.Unmarshal(payload, &cmd); err != nil {
				logger.Error("Failed to parse command", "error", err)
				return
			}
			parsed := roborock.LoxoneCommand{Action: cmd.Action, Segments: cmd.Segments, Speed: cmd.Speed, Mode: cmd.Mode, Level: cmd.Level, SceneID: cmd.SceneID}
			context, ok := commandContextForSlug(dev.Slug, nil)
			if !ok {
				logger.Warn("Unknown device for MQTT command", "device", dev.Slug)
				return
			}
			commandCoordinator.SubmitParsed(context, string(payload), parsed, nil)
		})
	}
}

func subscribeToLoxoneCommands(restClient *roborock.Client) {
	if !localMQTTEnabled() || commandCoordinator == nil {
		return
	}

	for _, md := range deviceManager.GetDevices() {
		dev := md // capture
		if !loxoneMQTTEnabledFor(dev.Slug) {
			continue
		}
		topic := loxoneTopic(dev.Slug, "command")

		logger.Info("Subscribing to Loxone MQTT commands", "device", dev.Slug, "topic", topic)
		// Delete a retained command before subscribing. The MQTT gateway callback
		// does not expose the retained flag, so this prevents replay after restart.
		publishLocalMQTT(topic, "", true)
		_ = localBroker.Subscribe(topic, func(topic string, payload []byte) {
			logger.Debug("Received Loxone MQTT command", "device", dev.Slug, "topic", topic, "payload", string(payload))
			commandCoordinator.SubmitText(dev.Slug, string(payload))
		})
	}
}

func commandContextForSlug(slug string, restClient *roborock.Client) (roborock.CommandContext, bool) {
	if deviceManager == nil {
		return roborock.CommandContext{}, false
	}
	dev := deviceManager.GetDevice(slug)
	if dev == nil {
		return roborock.CommandContext{}, false
	}
	if restClient == nil {
		restClient = deviceManager.RESTClient()
	}
	roomNames := map[string]string{}
	if restClient != nil {
		roomNames = loxoneRoomNames(dev, restClient)
	}
	scenes, _ := dev.GetScenes()
	return roborock.CommandContext{
		Slug: slug, Online: dev.CloudMQTT != nil && dev.CloudMQTT.IsConnected(),
		RoomNames: roomNames, Scenes: scenes, Capabilities: capabilityStore.Get(slug),
	}, true
}

func executeCommand(dev *roborock.ManagedDevice, action string, segments []int, speed, mode, level string, sceneID int) (err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Panic in command processing", "panic", r)
			err = fmt.Errorf("panic in command processing: %v", r)
		}
	}()

	switch action {
	case "start":
		logger.Info("Starting vacuum", "device", dev.Slug)
		err = dev.CloudMQTT.Start()
	case "pause":
		logger.Info("Pausing vacuum", "device", dev.Slug)
		err = dev.CloudMQTT.Pause()
	case "dock":
		logger.Info("Sending vacuum to dock", "device", dev.Slug)
		err = dev.CloudMQTT.Dock()
	case "stop":
		logger.Info("Stopping vacuum", "device", dev.Slug)
		err = dev.CloudMQTT.Stop()
	case "locate":
		logger.Info("Locating vacuum", "device", dev.Slug)
		err = dev.CloudMQTT.Locate()
	case "empty_dustbin":
		logger.Info("Starting dock dust collection", "device", dev.Slug)
		err = dev.CloudMQTT.StartDustCollection()
	case "stop_emptying":
		logger.Info("Stopping dock dust collection", "device", dev.Slug)
		err = dev.CloudMQTT.StopDustCollection()
	case "wash_mop":
		logger.Info("Starting dock mop wash", "device", dev.Slug)
		err = dev.CloudMQTT.StartWash()
	case "stop_washing":
		logger.Info("Stopping dock mop wash", "device", dev.Slug)
		err = dev.CloudMQTT.StopWash()
	case "dry_mop":
		logger.Info("Starting dock mop drying", "device", dev.Slug)
		err = dev.CloudMQTT.SetDryerEnabled(true)
	case "stop_drying":
		logger.Info("Stopping dock mop drying", "device", dev.Slug)
		err = dev.CloudMQTT.SetDryerEnabled(false)
	case "segment_clean":
		logger.Info("Starting segment clean", "device", dev.Slug, "segments", segments)
		deviceManager.NoteSegmentClean(dev.Slug, segments)
		err = dev.CloudMQTT.SegmentClean(segments)
	case "set_fan_speed":
		logger.Info("Setting fan speed", "device", dev.Slug, "speed", speed)
		err = dev.CloudMQTT.SetFanSpeed(speed)
	case "set_mop_mode":
		logger.Info("Setting mop mode", "device", dev.Slug, "mode", mode)
		err = dev.CloudMQTT.SetMopMode(mode)
	case "set_water_box":
		logger.Info("Setting water box level", "device", dev.Slug, "level", level)
		err = dev.CloudMQTT.SetWaterBox(level)
	case "scene":
		logger.Info("Executing scene", "device", dev.Slug, "sceneID", sceneID)
		deviceManager.NoteSceneStarted(dev.Slug, sceneID)
		err = deviceManager.ExecuteScene(sceneID)
	default:
		return fmt.Errorf("unknown action %q", action)
	}
	return err
}

// startBridge initializes the MQTT bridge after successful authentication.
func startBridge(restClient *roborock.Client) {
	cfg := config.Get()
	if store, err := roborock.NewLoxoneRoomOverrideStore(dataDir); err != nil {
		logger.Error("Failed to load Loxone room overrides", "error", err)
	} else {
		loxoneRoomOverrides = store
	}

	// Initialize maintenance checker
	maintenanceChecker = roborock.NewMaintenanceChecker(dataDir)

	// Create device manager for all devices
	deviceManager = roborock.NewDeviceManager(restClient.GetLoginData(), restClient.GetDevices(), restClient, dataDir)
	cleanupStaleRetainedTopics()
	deviceStateStore = roborock.NewDeviceStateStore()
	capabilityStore = roborock.NewCapabilityStore(dataDir)
	commandCoordinator = roborock.NewCommandCoordinator(
		time.Duration(cfg.Loxone.CommandDebounceMS)*time.Millisecond,
		time.Duration(cfg.Loxone.CommandTimeoutSeconds)*time.Second,
		func(slug string) (roborock.CommandContext, bool) { return commandContextForSlug(slug, restClient) },
		func(context roborock.CommandContext, command roborock.LoxoneCommand) error {
			dev := deviceManager.GetDevice(context.Slug)
			if dev == nil {
				return fmt.Errorf("unknown robot %q", context.Slug)
			}
			return executeCommand(dev, command.Action, command.Segments, command.Speed, command.Mode, command.Level, command.SceneID)
		},
		publishLoxoneActivities,
	)
	loxoneActivityTracker = commandCoordinator.Tracker()

	deviceStateStore.Subscribe(func(update roborock.DeviceStateUpdate) {
		state := update.State
		updateDirectState(state)
		switch update.Change {
		case roborock.DeviceStateStatus:
			publishDeviceStatus(state.Slug, state.Status)
			publishLoxoneStatus(state.Slug, state.Status)
			commandCoordinator.UpdateStatus(state.Slug, state.Status, state.UpdatedAt)
			if webServer != nil {
				webServer.BroadcastDeviceStatus(state.Slug, state.Status)
			}
			if state.Status != nil {
				c := state.Status.Consumables
				if c.MainBrushWorkTime > 0 || c.SideBrushWorkTime > 0 || c.FilterWorkTime > 0 || c.SensorDirtyTime > 0 || c.DustCollectionWorkTimes > 0 {
					maintenanceChecker.Check(state.Name, &state.Status.ConsumablePercents, &state.Status.Consumables)
				}
			}
		case roborock.DeviceStateAvailability:
			publishDeviceAvailability(state.Slug, state.Online)
			publishLoxoneAvailability(state.Slug, state.Online)
			commandCoordinator.UpdateAvailability(state.Slug, state.Online, state.UpdatedAt)
			if webServer != nil {
				webServer.BroadcastDeviceAvailability(state.Slug, state.Online)
			}
		case roborock.DeviceStateRoom:
			publishDeviceCurrentRoom(state.Slug, state.CurrentRoom)
			publishLoxoneCurrentRoom(state.Slug, state.CurrentRoom)
			commandCoordinator.UpdateRoom(state.Slug, state.CurrentRoom, state.UpdatedAt)
		case roborock.DeviceStateInventory:
			if len(state.Scenes) > 0 {
				publishDeviceScenes(state.Slug, state.Scenes)
			}
		case roborock.DeviceStateHealth:
			if healthPublisher != nil {
				healthPublisher.PublishNow()
			}
		}
	})
	deviceManager.SetStatusCallback(func(slug string, status *roborock.PublishedStatus) {
		capabilities := capabilityStore.ObserveStatus(slug, status, time.Now())
		deviceStateStore.UpdateStatus(slug, status, capabilities, time.Now())
	})
	deviceManager.SetMapCallback(func(slug string, pngData []byte) {
		publishDeviceMap(slug, pngData)

		dev := deviceManager.GetDevice(slug)
		if dev == nil {
			return
		}
		roomNames := loxoneRoomNames(dev, restClient)
		room, err := roborock.CurrentRoomFromVectorJSON(dev.GetVectorMapJSON(), roomNames)
		if err != nil {
			logger.Warn("Failed to determine current room", "device", slug, "error", err)
			return
		}
		deviceStateStore.UpdateCurrentRoom(slug, room, time.Now())
	})
	deviceManager.SetAvailabilityCallback(func(slug string, online bool) {
		deviceStateStore.UpdateAvailability(slug, online, time.Now())
	})
	deviceManager.SetInventoryCallback(func(slug string, mappings []roborock.RoomMapping, roomsKnown bool, scenes []roborock.Scene, scenesKnown bool) {
		capabilities := capabilityStore.UpdateInventory(slug, mappings, roomsKnown, scenes, scenesKnown, time.Now())
		deviceStateStore.UpdateInventory(slug, mappings, scenes, capabilities, time.Now())
	})
	deviceManager.SetHealthCallback(func(slug string, health roborock.DeviceHealth) {
		deviceStateStore.UpdateHealth(slug, health, time.Now())
	})
	configureDirectLoxone()
	if err := configureLocalMQTT(restClient); err != nil {
		logger.Error("Failed to start local MQTT integration; continuing without it", "error", err)
	}

	// Seed a retained `offline` for every device before connecting, so the
	// availability topic is never absent and consumers start from a safe default.
	for _, md := range deviceManager.GetDevices() {
		capabilities := capabilityStore.Ensure(md.Slug, time.Now())
		deviceStateStore.Seed(md, capabilities, time.Now())
		deviceStateStore.UpdateAvailability(md.Slug, false, time.Now())
		deviceStateStore.UpdateCurrentRoom(md.Slug, nil, time.Now())
	}
	startHealthPublisher()

	// Load cached maps from disk (available before first poll)
	deviceManager.LoadMapCaches()
	for _, md := range deviceManager.GetDevices() {
		if png := md.GetMapPNG(); png != nil {
			publishDeviceMap(md.Slug, png)
		}
	}

	// Connect all devices to Roborock cloud MQTT
	deviceManager.ConnectAll()

	// Initial poll
	deviceManager.PollAll()

	wireLoxoneWeb(restClient)

	// Start polling
	stopPolling = make(chan struct{})
	go deviceManager.StartPolling(time.Duration(cfg.Roborock.PollingInterval)*time.Second, stopPolling)

	// Initialize schedule engine (provisioned from config + user from data dir)
	notAtHomeStore = roborock.NewNotAtHomeStore(dataDir)
	scheduleStore = roborock.NewScheduleStore(dataDir)

	scheduleSignals = roborock.NewSignalListener(
		cfg.Roborock.ScheduleSignals.PublicHoliday,
		cfg.Roborock.ScheduleSignals.Vacation,
		func(topic string, handler func(string, []byte)) error { return localBroker.Subscribe(topic, handler) },
	)
	if localMQTTEnabled() {
		scheduleSignals.Subscribe()
	}

	scheduleEngine = roborock.NewScheduleEngine(cfg.Roborock.Schedules, scheduleStore, deviceManager, scheduleSignals, notAtHomeStore)

	scheduleCallback := func(slug string, state *roborock.ScheduleState) {
		publishDeviceSchedule(slug, state)
	}
	scheduleEngine.SetStateChangeCallback(scheduleCallback)
	scheduleEngine.SetActionCallback(scheduleCallback)

	scheduleSignals.SetOnChange(func() {
		scheduleEngine.CheckDayTypeChanges()
	})

	stopSchedule = make(chan struct{})
	go scheduleEngine.StartTicker(stopSchedule)

	logger.Info("Bridge started", "devices", len(restClient.GetDevices()))
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--healthcheck" {
		client := http.Client{Timeout: 5 * time.Second}
		response, err := client.Get(os.Args[2])
		if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
			os.Exit(1)
		}
		_ = response.Body.Close()
		return
	}
	logger.Init("info", logger.Logger())
	logger.Info("roborock-mqtt-loxone", "version", version.Info())
	initPprof()

	if len(os.Args) < 2 {
		logger.Error("No configuration file specified")
		os.Exit(1)
	}

	configFile := os.Args[1]
	logger.Info("Configuration file", "path", configFile)
	if err := config.EnsureConfigFile(configFile); err != nil {
		logger.Error("Failed to initialize configuration", "error", err)
		return
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		return
	}

	logger.SetLevel(cfg.LogLevel)

	// Data and session directories next to the config file
	dataDir = filepath.Dir(configFile)
	sessionDir := filepath.Join(dataDir, ".session")
	processSupervisor = supervisor.NewRuntime(os.Getenv("ROBOROCK_SUPERVISOR"), dataDir)
	if err := configureUpdaterClient(); err != nil {
		logger.Warn("Manual updater client is unavailable", "error", err)
	}

	// Initialize Roborock REST client
	restClient := roborock.NewClient(cfg.Roborock.BaseURL, cfg.Roborock.Username, cfg.Roborock.Password, cfg.Roborock.ClientID)
	restClient.SetSessionDir(sessionDir)

	// Try to restore a saved session
	if restClient.LoadSession() {
		if !restClient.IsAuthenticated() {
			logger.Info("Saved session found but no devices, discovering...")
			if err := restClient.DiscoverDevice(); err != nil {
				logger.Warn("Device discovery failed, will retry via web UI", "error", err)
			} else {
				_ = restClient.SaveSession()
			}
		}
		if restClient.IsAuthenticated() {
			logger.Info("Using saved session, starting bridge...")
			startBridge(restClient)
		}
	} else {
		logger.Info("No saved session. Waiting for authentication via web UI...")
	}

	// Start web server (always, needed for login UI when not authenticated)
	webServer = web.NewWebServer(deviceManager, restClient, func() {
		startBridge(restClient)
		webServer.SetDeviceManager(deviceManager)
		wireLoxoneWeb(restClient)
		if scheduleEngine != nil {
			webServer.SetScheduleEngine(scheduleEngine)
			webServer.SetNotAtHomeStore(notAtHomeStore)
			webServer.SetScheduleStore(scheduleStore)
			scheduleEngine.SetStateChangeCallback(func(slug string, state *roborock.ScheduleState) {
				publishDeviceSchedule(slug, state)
				webServer.BroadcastScheduleState(slug, state)
			})
			scheduleEngine.SetActionCallback(func(slug string, state *roborock.ScheduleState) {
				publishDeviceSchedule(slug, state)
				webServer.BroadcastScheduleState(slug, state)
			})
		}
	})
	wireLoxoneWeb(restClient)
	wireIntegrationSettings(restClient)
	startWatchdog(restClient)
	webServer.SetHealthProvider(func() watchdog.Report {
		if watchdogMonitor == nil {
			return watchdog.Assess(watchdogObservation(restClient, time.Now()), watchdogConfig(config.Get().Watchdog))
		}
		return watchdogMonitor.Report()
	})
	webServer.SetSystemIntegration(&web.SystemDependencies{
		Status: systemStatusSnapshot,
		CheckUpdates: func(ctx context.Context, channel string) (updates.Info, error) {
			return updateChecker.Check(ctx, channel)
		},
		UpdaterStatus: func(ctx context.Context) (updater.Operation, error) {
			if updaterClient == nil {
				return updater.Operation{}, fmt.Errorf("isolated updater is not configured")
			}
			return updaterClient.Status(ctx)
		},
		InstallUpdate:      startManualUpdate,
		SaveUpdateSettings: config.SaveUpdateConfig,
	})
	startAutomaticUpdates()

	// Wire schedule engine to web server if bridge already started
	if scheduleEngine != nil {
		webServer.SetScheduleEngine(scheduleEngine)
		webServer.SetNotAtHomeStore(notAtHomeStore)
		webServer.SetScheduleStore(scheduleStore)
		scheduleEngine.SetStateChangeCallback(func(slug string, state *roborock.ScheduleState) {
			publishDeviceSchedule(slug, state)
			webServer.BroadcastScheduleState(slug, state)
		})
		scheduleEngine.SetActionCallback(func(slug string, state *roborock.ScheduleState) {
			publishDeviceSchedule(slug, state)
			webServer.BroadcastScheduleState(slug, state)
		})
	}

	go func() {
		port := cfg.Web.Port
		if port == 0 {
			port = 8080
		}
		logger.Info("Web interface available", "url", "http://localhost:"+strconv.Itoa(port))
		if err := webServer.Start(port); err != nil {
			logger.Error("Failed to start web server", "error", err)
		}
	}()

	logger.Info("Application ready")
	roborock.SendEmail("[Info] roborock-mqtt-loxone started", "The roborock-mqtt-loxone application has started and is ready.")

	quitChannel := make(chan os.Signal, 1)
	signal.Notify(quitChannel, syscall.SIGINT, syscall.SIGTERM)
	exitCode := 0
	select {
	case <-quitChannel:
	case reason := <-processSupervisor.RestartRequests():
		exitCode = 2
		logger.Error("Watchdog requested process restart", "reason", reason)
	}

	if watchdogMonitor != nil {
		watchdogMonitor.Stop()
	}
	if autoUpdateScheduler != nil {
		autoUpdateScheduler.Stop()
	}

	if stopSchedule != nil {
		close(stopSchedule)
	}
	if stopPolling != nil {
		close(stopPolling)
	}
	if deviceManager != nil {
		deviceManager.DisconnectAll()
	}
	if healthPublisher != nil {
		healthPublisher.Stop()
	}
	if directSynchronizer != nil {
		directSynchronizer.Close()
	}
	localBroker.Stop()
	logger.Info("Shutdown complete")
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func initPprof() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
}
