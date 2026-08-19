package watchdog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Config struct {
	Enabled            bool
	CheckInterval      time.Duration
	StaleAfter         time.Duration
	ReconnectAfter     time.Duration
	RebuildAfter       time.Duration
	ResetAfter         time.Duration
	RestartAfter       time.Duration
	RecoveryHysteresis int
	MaxRestarts        int
	RestartWindow      time.Duration
	MaxQueueDepth      int
}

func DefaultConfig() Config {
	return Config{
		Enabled: true, CheckInterval: 30 * time.Second, StaleAfter: 2 * time.Minute,
		ReconnectAfter: 2 * time.Minute, RebuildAfter: 5 * time.Minute,
		ResetAfter: 8 * time.Minute, RestartAfter: 15 * time.Minute,
		RecoveryHysteresis: 2, MaxRestarts: 3, RestartWindow: time.Hour, MaxQueueDepth: 256,
	}
}

type Observation struct {
	ObservedAt              time.Time `json:"observed_at"`
	StartedAt               time.Time `json:"started_at"`
	Authenticated           bool      `json:"authenticated"`
	BridgeStarted           bool      `json:"bridge_started"`
	RoborockLoopLastActive  time.Time `json:"roborock_loop_last_active,omitempty"`
	CloudConnected          bool      `json:"cloud_connected"`
	LastCloudMessage        time.Time `json:"last_cloud_message,omitempty"`
	LastRobotUpdate         time.Time `json:"last_robot_update,omitempty"`
	DispatcherInFlight      int       `json:"dispatcher_in_flight"`
	DispatcherOldest        time.Time `json:"dispatcher_oldest,omitempty"`
	DispatcherLastCompleted time.Time `json:"dispatcher_last_completed,omitempty"`
	LocalMQTTEnabled        bool      `json:"local_mqtt_enabled"`
	LocalMQTTConnected      bool      `json:"local_mqtt_connected"`
	DirectEnabled           bool      `json:"direct_enabled"`
	DirectPending           int       `json:"direct_pending"`
	DirectLastSuccess       time.Time `json:"direct_last_success,omitempty"`
	DirectLastError         string    `json:"direct_last_error,omitempty"`
}

type Component struct {
	Healthy      bool      `json:"healthy"`
	Required     bool      `json:"required"`
	Detail       string    `json:"detail,omitempty"`
	LastActivity time.Time `json:"last_activity,omitempty"`
}

type Report struct {
	Status              string               `json:"status"`
	Live                bool                 `json:"live"`
	Ready               bool                 `json:"ready"`
	ObservedAt          time.Time            `json:"observed_at"`
	UptimeSeconds       int64                `json:"uptime_seconds"`
	Reasons             []string             `json:"reasons"`
	Components          map[string]Component `json:"components"`
	LastAction          string               `json:"last_action,omitempty"`
	LastActionAt        time.Time            `json:"last_action_at,omitempty"`
	LastWatchdogReason  string               `json:"last_watchdog_reason,omitempty"`
	LastWatchdogRestart time.Time            `json:"last_watchdog_restart,omitempty"`
	RestartSuppressed   bool                 `json:"restart_suppressed"`
}

func Assess(observation Observation, cfg Config) Report {
	now := observation.ObservedAt
	if now.IsZero() {
		now = time.Now()
	}
	staleAfter := cfg.StaleAfter
	if staleAfter <= 0 {
		staleAfter = DefaultConfig().StaleAfter
	}
	maxQueueDepth := cfg.MaxQueueDepth
	if maxQueueDepth <= 0 {
		maxQueueDepth = DefaultConfig().MaxQueueDepth
	}
	fresh := func(value time.Time) bool { return !value.IsZero() && now.Sub(value) <= staleAfter }
	loopHealthy := !observation.BridgeStarted || fresh(observation.RoborockLoopLastActive)
	cloudHealthy := !observation.BridgeStarted || (observation.CloudConnected && fresh(observation.LastCloudMessage))
	dispatcherHealthy := observation.DispatcherInFlight <= maxQueueDepth
	if dispatcherHealthy && observation.DispatcherInFlight > 0 {
		dispatcherHealthy = observation.DispatcherOldest.IsZero() || now.Sub(observation.DispatcherOldest) <= staleAfter
	}
	queueHealthy := observation.DirectPending <= maxQueueDepth
	components := map[string]Component{
		"process":        {Healthy: true, Required: true, Detail: "HTTP event loop responding", LastActivity: now},
		"roborock_loop":  {Healthy: loopHealthy, Required: observation.BridgeStarted, LastActivity: observation.RoborockLoopLastActive},
		"cloud":          {Healthy: cloudHealthy, Required: observation.BridgeStarted, LastActivity: observation.LastCloudMessage},
		"robot_updates":  {Healthy: fresh(observation.LastRobotUpdate), Required: observation.BridgeStarted, LastActivity: observation.LastRobotUpdate},
		"dispatcher":     {Healthy: dispatcherHealthy, Required: observation.BridgeStarted, LastActivity: observation.DispatcherLastCompleted},
		"direct_queue":   {Healthy: queueHealthy, Required: observation.DirectEnabled, Detail: fmt.Sprintf("%d pending", observation.DirectPending)},
		"local_mqtt":     {Healthy: !observation.LocalMQTTEnabled || observation.LocalMQTTConnected, Required: observation.LocalMQTTEnabled},
		"direct_loxone":  {Healthy: !observation.DirectEnabled || (observation.DirectLastError == "" && !observation.DirectLastSuccess.IsZero()), Required: observation.DirectEnabled, Detail: observation.DirectLastError, LastActivity: observation.DirectLastSuccess},
		"authentication": {Healthy: observation.Authenticated, Required: true},
	}
	reasons := make([]string, 0)
	for name, component := range components {
		if component.Required && !component.Healthy {
			reasons = append(reasons, name)
		}
	}
	sort.Strings(reasons)
	live := components["process"].Healthy && loopHealthy && dispatcherHealthy && queueHealthy
	ready := live && observation.Authenticated && observation.BridgeStarted && observation.CloudConnected && components["local_mqtt"].Healthy && components["direct_loxone"].Healthy
	status := "healthy"
	if !live {
		status = "unhealthy"
	} else if !ready || len(reasons) > 0 {
		status = "degraded"
	}
	uptime := int64(0)
	if !observation.StartedAt.IsZero() && now.After(observation.StartedAt) {
		uptime = int64(now.Sub(observation.StartedAt).Seconds())
	}
	return Report{Status: status, Live: live, Ready: ready, ObservedAt: now, UptimeSeconds: uptime, Reasons: reasons, Components: components}
}

type Actions struct {
	Reconnect func(string)
	Rebuild   func(string)
	Reset     func(string)
	Exit      func(string)
}

type Source func(time.Time) Observation

type Monitor struct {
	cfg     Config
	source  Source
	actions Actions
	guard   *RestartGuard

	mu            sync.RWMutex
	report        Report
	issueSince    time.Time
	stage         int
	healthyStreak int
	stop          chan struct{}
	done          chan struct{}
}

func NewMonitor(cfg Config, source Source, actions Actions, guard *RestartGuard) *Monitor {
	defaults := DefaultConfig()
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = defaults.CheckInterval
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = defaults.StaleAfter
	}
	if cfg.ReconnectAfter <= 0 {
		cfg.ReconnectAfter = defaults.ReconnectAfter
	}
	if cfg.RebuildAfter <= 0 {
		cfg.RebuildAfter = defaults.RebuildAfter
	}
	if cfg.ResetAfter <= 0 {
		cfg.ResetAfter = defaults.ResetAfter
	}
	if cfg.RestartAfter <= 0 {
		cfg.RestartAfter = defaults.RestartAfter
	}
	if cfg.RecoveryHysteresis <= 0 {
		cfg.RecoveryHysteresis = defaults.RecoveryHysteresis
	}
	if cfg.MaxRestarts <= 0 {
		cfg.MaxRestarts = defaults.MaxRestarts
	}
	if cfg.RestartWindow <= 0 {
		cfg.RestartWindow = defaults.RestartWindow
	}
	if cfg.MaxQueueDepth <= 0 {
		cfg.MaxQueueDepth = defaults.MaxQueueDepth
	}
	return &Monitor{cfg: cfg, source: source, actions: actions, guard: guard}
}

func (m *Monitor) Start() {
	if m.source == nil {
		return
	}
	m.Step(time.Now())
	if !m.cfg.Enabled {
		return
	}
	m.mu.Lock()
	if m.stop != nil {
		m.mu.Unlock()
		return
	}
	m.stop, m.done = make(chan struct{}), make(chan struct{})
	stop, done := m.stop, m.done
	m.mu.Unlock()
	go func() {
		defer close(done)
		ticker := time.NewTicker(m.cfg.CheckInterval)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				m.Step(now)
			case <-stop:
				return
			}
		}
	}()
}

func (m *Monitor) Stop() {
	m.mu.Lock()
	if m.stop == nil {
		m.mu.Unlock()
		return
	}
	stop, done := m.stop, m.done
	m.stop, m.done = nil, nil
	close(stop)
	m.mu.Unlock()
	<-done
}

func (m *Monitor) Step(now time.Time) Report {
	observation := m.source(now)
	observation.ObservedAt = now
	report := Assess(observation, m.cfg)
	needsRecovery := !report.Live || (observation.BridgeStarted && !observation.CloudConnected) ||
		(observation.LocalMQTTEnabled && !observation.LocalMQTTConnected) ||
		(observation.DirectEnabled && (observation.DirectPending > m.cfg.MaxQueueDepth || observation.DirectLastError != "")) ||
		contains(report.Reasons, "cloud") || contains(report.Reasons, "robot_updates")

	m.mu.Lock()
	defer m.mu.Unlock()
	if !needsRecovery {
		m.healthyStreak++
		if m.healthyStreak >= m.cfg.RecoveryHysteresis {
			m.issueSince = time.Time{}
			m.stage = 0
		}
	} else {
		m.healthyStreak = 0
		if m.issueSince.IsZero() {
			m.issueSince = now
		}
		elapsed := now.Sub(m.issueSince)
		reason := joinReasons(report.Reasons)
		switch {
		case m.stage < 1 && elapsed >= m.cfg.ReconnectAfter:
			m.stage = 1
			m.recordAction(&report, "reconnect", reason, now)
			if m.actions.Reconnect != nil {
				m.actions.Reconnect(reason)
			}
		case m.stage < 2 && elapsed >= m.cfg.RebuildAfter:
			m.stage = 2
			m.recordAction(&report, "rebuild", reason, now)
			if m.actions.Rebuild != nil {
				m.actions.Rebuild(reason)
			}
		case m.stage < 3 && elapsed >= m.cfg.ResetAfter:
			m.stage = 3
			m.recordAction(&report, "reset", reason, now)
			if m.actions.Reset != nil {
				m.actions.Reset(reason)
			}
		case m.stage < 4 && elapsed >= m.cfg.RestartAfter && !report.Live:
			m.stage = 4
			allowed := m.guard == nil || m.guard.Allow(now, m.cfg.MaxRestarts, m.cfg.RestartWindow)
			if allowed {
				m.recordAction(&report, "restart", reason, now)
				if m.guard != nil {
					_ = m.guard.Record(now, reason)
				}
				if m.actions.Exit != nil {
					m.actions.Exit(reason)
				}
			} else {
				report.RestartSuppressed = true
				m.recordAction(&report, "restart_suppressed", reason, now)
			}
		}
	}
	if m.guard != nil {
		report.LastWatchdogReason, report.LastWatchdogRestart = m.guard.Last()
	}
	if report.LastAction == "" {
		report.LastAction, report.LastActionAt = m.report.LastAction, m.report.LastActionAt
	}
	m.report = report
	return report
}

func (m *Monitor) recordAction(report *Report, action, reason string, now time.Time) {
	report.LastAction, report.LastActionAt = action, now
	report.LastWatchdogReason = reason
	if m.guard != nil {
		_ = m.guard.RecordReason(now, reason)
	}
}

func (m *Monitor) Report() Report {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneReport(m.report)
}

func cloneReport(report Report) Report {
	report.Reasons = append([]string(nil), report.Reasons...)
	components := make(map[string]Component, len(report.Components))
	for key, value := range report.Components {
		components[key] = value
	}
	report.Components = components
	return report
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func joinReasons(reasons []string) string {
	if len(reasons) == 0 {
		return "watchdog health failure"
	}
	result := reasons[0]
	for _, reason := range reasons[1:] {
		result += "," + reason
	}
	return result
}

type restartState struct {
	Restarts   []time.Time `json:"restarts"`
	LastReason string      `json:"last_reason,omitempty"`
	LastAt     time.Time   `json:"last_at,omitempty"`
}

type RestartGuard struct {
	mu    sync.Mutex
	path  string
	state restartState
}

func NewRestartGuard(dataDir string) *RestartGuard {
	guard := &RestartGuard{path: filepath.Join(dataDir, "watchdog-state.json")}
	data, err := os.ReadFile(guard.path)
	if err == nil {
		_ = json.Unmarshal(data, &guard.state)
	}
	return guard
}

func (g *RestartGuard) Allow(now time.Time, maximum int, window time.Duration) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prune(now, window)
	return len(g.state.Restarts) < maximum
}

func (g *RestartGuard) Record(now time.Time, reason string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.state.Restarts = append(g.state.Restarts, now)
	g.state.LastReason, g.state.LastAt = reason, now
	return g.saveLocked()
}

// RecordReason persists a recovery reason without counting it as a process
// restart. This keeps diagnostics useful after reconnect/rebuild actions.
func (g *RestartGuard) RecordReason(now time.Time, reason string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.state.LastReason, g.state.LastAt = reason, now
	return g.saveLocked()
}

func (g *RestartGuard) saveLocked() error {
	data, err := json.MarshalIndent(g.state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(g.path), 0700); err != nil {
		return err
	}
	temporary := g.path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(temporary, g.path)
}

func (g *RestartGuard) Last() (string, time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state.LastReason, g.state.LastAt
}

func (g *RestartGuard) prune(now time.Time, window time.Duration) {
	cutoff := now.Add(-window)
	kept := g.state.Restarts[:0]
	for _, value := range g.state.Restarts {
		if !value.Before(cutoff) {
			kept = append(kept, value)
		}
	}
	g.state.Restarts = kept
}
