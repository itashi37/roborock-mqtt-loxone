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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mqtt-home/roborock-mqtt/config"
	"github.com/mqtt-home/roborock-mqtt/roborock"
	"github.com/mqtt-home/roborock-mqtt/version"
	"github.com/mqtt-home/roborock-mqtt/web"
	"github.com/philipparndt/go-logger"
	"github.com/philipparndt/mqtt-gateway/mqtt"
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
)

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
	mqtt.PublishAbsolute(loxoneTopic("_bridge", "diagnostic"), nonce, false)
	select {
	case <-channel:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("MQTT loopback timed out: %w", ctx.Err())
	}
}

func publishDeviceMap(slug string, pngData []byte) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/map"
	mqtt.PublishAbsolute(topic, pngData, cfg.MQTT.Retain)
	logger.Debug("Published map", "device", slug, "topic", topic, "size", len(pngData))
}

func publishDeviceCurrentRoom(slug string, room *roborock.CurrentRoom) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/current_room"

	data, err := json.Marshal(room)
	if err != nil {
		logger.Error("Failed to marshal current room", "error", err)
		return
	}

	mqtt.PublishAbsolute(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published current room", "device", slug, "topic", topic, "payload", string(data))
}

func loxoneTopic(slug, suffix string) string {
	base := strings.TrimSuffix(strings.TrimSpace(config.Get().Loxone.Topic), "/")
	return base + "/" + slug + "/" + suffix
}

func publishLoxoneScalar(slug, suffix, payload string) {
	if !config.Get().Loxone.Enabled {
		return
	}
	topic := loxoneTopic(slug, suffix)
	mqtt.PublishAbsolute(topic, payload, true)
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
	if !config.Get().Loxone.Enabled {
		return
	}
	for _, activity := range activities {
		data, err := roborock.MarshalLoxoneActivity(activity)
		if err != nil {
			logger.Error("Failed to marshal Loxone activity", "device", slug, "error", err)
			continue
		}
		mqtt.PublishAbsolute(loxoneTopic(slug, "activity"), string(data), false)
		if activity.Type == "command" {
			mqtt.PublishAbsolute(loxoneTopic(slug, "last_command"), string(data), true)
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
	publishDeviceCurrentRoom(slug, room)
	publishLoxoneCurrentRoom(slug, room)
}

func wireLoxoneWeb(restClient *roborock.Client) {
	if webServer == nil {
		return
	}
	webServer.SetLoxoneIntegration(&web.LoxoneDependencies{
		Core:          loxoneCoreStore,
		Diagnostics:   loxoneDiagnostics,
		RoomOverrides: loxoneRoomOverrides,
		PublishCommand: func(slug, command string) error {
			if !config.Get().Loxone.Enabled {
				return fmt.Errorf("Loxone mode is disabled")
			}
			if deviceManager == nil || deviceManager.GetDevice(slug) == nil {
				return fmt.Errorf("unknown robot %q", slug)
			}
			mqtt.PublishAbsolute(loxoneTopic(slug, "command"), command, false)
			return nil
		},
		TestMQTT: loxoneProbe.test,
		RefreshRoom: func(slug string) {
			refreshLoxoneCurrentRoom(slug, restClient)
		},
	})
}

func publishLoxoneActivity(slug string, activity *roborock.LoxoneActivity) {
	if activity != nil {
		publishLoxoneActivities(slug, []roborock.LoxoneActivity{*activity})
	}
}

func publishLoxoneStatus(slug string, status *roborock.PublishedStatus) {
	if !config.Get().Loxone.Enabled {
		return
	}
	loxonePublishMu.Lock()
	defer loxonePublishMu.Unlock()

	observedAt := time.Now()
	for suffix, payload := range roborock.LoxoneStatusScalars(status, observedAt) {
		publishLoxoneScalar(slug, suffix, payload)
	}
	publishLoxoneCore(slug, loxoneCoreStore.UpdateStatus(slug, status, observedAt))
}

func publishLoxoneAvailability(slug string, online bool) {
	if !config.Get().Loxone.Enabled {
		return
	}
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
	if !config.Get().Loxone.Enabled {
		return
	}
	loxonePublishMu.Lock()
	defer loxonePublishMu.Unlock()

	for suffix, payload := range roborock.LoxoneCurrentRoomScalars(room) {
		publishLoxoneScalar(slug, suffix, payload)
	}
	publishLoxoneCore(slug, loxoneCoreStore.UpdateCurrentRoom(slug, room))
}

func publishDeviceScenes(slug string, scenes []roborock.Scene) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/scenes"

	data, err := json.Marshal(scenes)
	if err != nil {
		logger.Error("Failed to marshal scenes", "error", err)
		return
	}

	mqtt.PublishAbsolute(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published scenes", "device", slug, "topic", topic, "count", len(scenes))
}

func publishDeviceSchedule(slug string, state *roborock.ScheduleState) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/schedule"

	data, err := json.Marshal(state)
	if err != nil {
		logger.Error("Failed to marshal schedule state", "error", err)
		return
	}

	mqtt.PublishAbsolute(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published schedule state", "device", slug, "topic", topic, "dayType", state.ActiveDay)
}

// publishDeviceAvailability publishes a device's cloud-connection state to the
// local broker as a retained `<topic>/<slug>/availability` message. Consumers
// (e.g. the wall-display Wall API) use this to mark a device unavailable rather
// than trust a stale retained `<topic>/<slug>/status`.
func publishDeviceAvailability(slug string, online bool) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/availability"
	payload := "offline"
	if online {
		payload = "online"
	}
	// Availability is always retained so late subscribers see the current state.
	mqtt.PublishAbsolute(topic, payload, true)
	logger.Debug("Published availability", "device", slug, "topic", topic, "state", payload)
}

func publishDeviceStatus(slug string, status *roborock.PublishedStatus) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/status"

	data, err := json.Marshal(status)
	if err != nil {
		logger.Error("Failed to marshal status", "error", err)
		return
	}

	mqtt.PublishAbsolute(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published status", "device", slug, "topic", topic)
}

func subscribeToCommands() {
	cfg := config.Get()

	for _, md := range deviceManager.GetDevices() {
		dev := md // capture
		topic := cfg.MQTT.Topic + "/" + dev.Slug + "/set"

		logger.Info("Subscribing to MQTT commands", "device", dev.Slug, "topic", topic)

		mqtt.Subscribe(topic, func(topic string, payload []byte) {
			logger.Debug("Received MQTT command", "device", dev.Slug, "topic", topic, "payload", string(payload))

			if dev.CloudMQTT == nil {
				logger.Warn("Device not connected, ignoring command", "device", dev.Slug)
				return
			}

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

			go dispatchCommand(dev, cmd.Action, cmd.Segments, cmd.Speed, cmd.Mode, cmd.Level, cmd.SceneID)
		})
	}
}

func subscribeToLoxoneCommands(restClient *roborock.Client) {
	cfg := config.Get()
	if !cfg.Loxone.Enabled {
		return
	}

	for _, md := range deviceManager.GetDevices() {
		dev := md // capture
		topic := loxoneTopic(dev.Slug, "command")

		logger.Info("Subscribing to Loxone MQTT commands", "device", dev.Slug, "topic", topic)
		// Delete a retained command before subscribing. The MQTT gateway callback
		// does not expose the retained flag, so this prevents replay after restart.
		mqtt.PublishAbsolute(topic, "", true)
		mqtt.Subscribe(topic, func(topic string, payload []byte) {
			logger.Debug("Received Loxone MQTT command", "device", dev.Slug, "topic", topic, "payload", string(payload))

			roomNames := loxoneRoomNames(dev, restClient)
			cmd, parseErr := roborock.ParseLoxoneCommand(string(payload), roomNames, dev.Scenes)
			online := dev.CloudMQTT != nil && dev.CloudMQTT.IsConnected()
			decision := loxoneActivityTracker.BeginCommand(dev.Slug, string(payload), cmd, parseErr, online, time.Now())
			publishLoxoneActivities(dev.Slug, decision.Activities)
			if !decision.Dispatch {
				return
			}

			time.AfterFunc(time.Duration(cfg.Loxone.CommandTimeoutSeconds)*time.Second, func() {
				publishLoxoneActivity(dev.Slug, loxoneActivityTracker.ExpireCommand(dev.Slug, decision.ID, time.Now()))
			})
			go func() {
				if err := executeCommand(dev, cmd.Action, cmd.Segments, cmd.Speed, cmd.Mode, "", cmd.SceneID); err != nil {
					logger.Error("Loxone command failed", "device", dev.Slug, "action", cmd.Action, "error", err)
					publishLoxoneActivity(dev.Slug, loxoneActivityTracker.MarkFailed(dev.Slug, decision.ID, err.Error(), time.Now()))
					return
				}
				publishLoxoneActivity(dev.Slug, loxoneActivityTracker.MarkRunning(dev.Slug, decision.ID, time.Now()))
			}()
		})
	}
}

func dispatchCommand(dev *roborock.ManagedDevice, action string, segments []int, speed, mode, level string, sceneID int) {
	if err := executeCommand(dev, action, segments, speed, mode, level, sceneID); err != nil {
		logger.Error("Command failed", "device", dev.Slug, "action", action, "error", err)
	}
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
	loxoneActivityTracker = roborock.NewLoxoneActivityTracker(time.Duration(cfg.Loxone.CommandDebounceMS) * time.Millisecond)
	if store, err := roborock.NewLoxoneRoomOverrideStore(dataDir); err != nil {
		logger.Error("Failed to load Loxone room overrides", "error", err)
	} else {
		loxoneRoomOverrides = store
	}

	// Start local MQTT
	mqtt.Start(cfg.MQTT, "roborock_mqtt")
	mqtt.Subscribe(loxoneTopic("_bridge", "diagnostic"), func(_ string, payload []byte) {
		loxoneProbe.receive(payload)
	})

	// Initialize maintenance checker
	maintenanceChecker = roborock.NewMaintenanceChecker(dataDir)

	// Create device manager for all devices
	deviceManager = roborock.NewDeviceManager(restClient.GetLoginData(), restClient.GetDevices(), restClient, dataDir)
	deviceManager.SetStatusCallback(func(slug string, status *roborock.PublishedStatus) {
		publishDeviceStatus(slug, status)
		publishLoxoneStatus(slug, status)
		if cfg.Loxone.Enabled {
			publishLoxoneActivities(slug, loxoneActivityTracker.UpdateStatus(slug, status, time.Now()))
		}
		// Push live status (including the cleaning ETA) to connected web clients.
		if webServer != nil {
			webServer.BroadcastDeviceStatus(slug, status)
		}
		// Check maintenance thresholds — only when consumable data was actually fetched
		// (if ALL values are zero, the poll likely failed)
		c := status.Consumables
		if c.MainBrushWorkTime > 0 || c.SideBrushWorkTime > 0 || c.FilterWorkTime > 0 || c.SensorDirtyTime > 0 || c.DustCollectionWorkTimes > 0 {
			if dev := deviceManager.GetDevice(slug); dev != nil {
				maintenanceChecker.Check(dev.Info.Name, &status.ConsumablePercents, &status.Consumables)
			}
		}
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
		publishDeviceCurrentRoom(slug, room)
		publishLoxoneCurrentRoom(slug, room)
		if cfg.Loxone.Enabled {
			publishLoxoneActivities(slug, loxoneActivityTracker.UpdateRoom(slug, room, time.Now()))
		}
	})
	deviceManager.SetAvailabilityCallback(func(slug string, online bool) {
		publishDeviceAvailability(slug, online)
		publishLoxoneAvailability(slug, online)
		if cfg.Loxone.Enabled {
			publishLoxoneActivities(slug, loxoneActivityTracker.UpdateAvailability(slug, online, time.Now()))
		}
		if webServer != nil {
			webServer.BroadcastDeviceAvailability(slug, online)
		}
	})

	// Seed a retained `offline` for every device before connecting, so the
	// availability topic is never absent and consumers start from a safe default.
	for _, md := range deviceManager.GetDevices() {
		publishDeviceAvailability(md.Slug, false)
		publishLoxoneAvailability(md.Slug, false)
		publishLoxoneCurrentRoom(md.Slug, nil)
	}

	// Load cached maps from disk (available before first poll)
	deviceManager.LoadMapCaches()
	for _, md := range deviceManager.GetDevices() {
		if png := md.GetMapPNG(); png != nil {
			publishDeviceMap(md.Slug, png)
		}
	}

	// Connect all devices to Roborock cloud MQTT
	deviceManager.ConnectAll()

	// Publish scenes per device
	for _, md := range deviceManager.GetDevices() {
		if len(md.Scenes) > 0 {
			publishDeviceScenes(md.Slug, md.Scenes)
		}
	}

	// Initial poll
	deviceManager.PollAll()

	// Subscribe to local MQTT commands per device
	subscribeToCommands()
	subscribeToLoxoneCommands(restClient)
	if cfg.Loxone.Enabled {
		for _, device := range deviceManager.GetDevices() {
			dev := device
			mqtt.Subscribe(loxoneTopic(dev.Slug, "last_command"), func(_ string, payload []byte) {
				var activity roborock.LoxoneActivity
				if err := json.Unmarshal(payload, &activity); err == nil {
					loxoneDiagnostics.RestoreLastCommand(dev.Slug, activity)
				}
			})
		}
	}
	wireLoxoneWeb(restClient)

	// Start polling
	stopPolling = make(chan struct{})
	go deviceManager.StartPolling(time.Duration(cfg.Roborock.PollingInterval)*time.Second, stopPolling)

	// Initialize schedule engine (provisioned from config + user from data dir)
	notAtHomeStore = roborock.NewNotAtHomeStore(dataDir)
	scheduleStore = roborock.NewScheduleStore(dataDir)

	signals := roborock.NewSignalListener(
		cfg.Roborock.ScheduleSignals.PublicHoliday,
		cfg.Roborock.ScheduleSignals.Vacation,
	)
	signals.Subscribe()

	scheduleEngine = roborock.NewScheduleEngine(cfg.Roborock.Schedules, scheduleStore, deviceManager, signals, notAtHomeStore)

	scheduleCallback := func(slug string, state *roborock.ScheduleState) {
		publishDeviceSchedule(slug, state)
	}
	scheduleEngine.SetStateChangeCallback(scheduleCallback)
	scheduleEngine.SetActionCallback(scheduleCallback)

	signals.SetOnChange(func() {
		scheduleEngine.CheckDayTypeChanges()
	})

	stopSchedule = make(chan struct{})
	go scheduleEngine.StartTicker(stopSchedule)

	logger.Info("Bridge started", "devices", len(restClient.GetDevices()))
}

func main() {
	logger.Init("info", logger.Logger())
	logger.Info("roborock-mqtt-loxone", "version", version.Info())
	initPprof()

	if len(os.Args) < 2 {
		logger.Error("No configuration file specified")
		os.Exit(1)
	}

	configFile := os.Args[1]
	logger.Info("Configuration file", "path", configFile)

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		return
	}

	logger.SetLevel(cfg.LogLevel)

	// Data and session directories next to the config file
	dataDir = filepath.Dir(configFile)
	sessionDir := filepath.Join(dataDir, ".session")

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
	<-quitChannel

	if stopSchedule != nil {
		close(stopSchedule)
	}
	if stopPolling != nil {
		close(stopPolling)
	}
	if deviceManager != nil {
		deviceManager.DisconnectAll()
	}
	logger.Info("Shutdown complete")
}

func initPprof() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
}
