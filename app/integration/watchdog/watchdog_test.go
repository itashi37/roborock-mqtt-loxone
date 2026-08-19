package watchdog

import (
	"testing"
	"time"
)

func TestAssessSeparatesLivenessReadinessAndExternalFailures(t *testing.T) {
	now := time.Unix(1700000000, 0)
	cfg := DefaultConfig()
	base := Observation{ObservedAt: now, StartedAt: now.Add(-time.Hour), Authenticated: true, BridgeStarted: true,
		RoborockLoopLastActive: now, CloudConnected: true, LastCloudMessage: now, LastRobotUpdate: now,
		LocalMQTTEnabled: true, LocalMQTTConnected: true}
	report := Assess(base, cfg)
	if report.Status != "healthy" || !report.Live || !report.Ready || report.UptimeSeconds != 3600 {
		t.Fatalf("unexpected healthy report: %+v", report)
	}
	base.CloudConnected = false
	report = Assess(base, cfg)
	if report.Status != "degraded" || !report.Live || report.Ready {
		t.Fatalf("cloud outage must degrade readiness without killing liveness: %+v", report)
	}
	base.CloudConnected = true
	base.RoborockLoopLastActive = now.Add(-10 * time.Minute)
	report = Assess(base, cfg)
	if report.Live || report.Status != "unhealthy" {
		t.Fatalf("stuck loop must fail liveness: %+v", report)
	}
}

func TestMonitorUsesProgressiveRecoveryAndOnlyRestartsFatalHealth(t *testing.T) {
	now := time.Unix(1700000000, 0)
	observation := Observation{StartedAt: now.Add(-time.Hour), Authenticated: true, BridgeStarted: true,
		RoborockLoopLastActive: now.Add(-time.Hour), CloudConnected: false}
	actions := []string{}
	monitor := NewMonitor(Config{Enabled: true, CheckInterval: time.Hour, StaleAfter: time.Second,
		ReconnectAfter: time.Second, RebuildAfter: 2 * time.Second, ResetAfter: 3 * time.Second,
		RestartAfter: 4 * time.Second, RecoveryHysteresis: 2, MaxQueueDepth: 2},
		func(at time.Time) Observation { observation.ObservedAt = at; return observation }, Actions{
			Reconnect: func(string) { actions = append(actions, "reconnect") },
			Rebuild:   func(string) { actions = append(actions, "rebuild") },
			Reset:     func(string) { actions = append(actions, "reset") },
			Exit:      func(string) { actions = append(actions, "restart") },
		}, nil)
	monitor.Step(now)
	monitor.Step(now.Add(time.Second))
	monitor.Step(now.Add(2 * time.Second))
	monitor.Step(now.Add(3 * time.Second))
	monitor.Step(now.Add(4 * time.Second))
	want := []string{"reconnect", "rebuild", "reset", "restart"}
	if len(actions) != len(want) {
		t.Fatalf("actions=%v want=%v", actions, want)
	}
	for index := range want {
		if actions[index] != want[index] {
			t.Fatalf("actions=%v want=%v", actions, want)
		}
	}

	// A cloud-only outage is recoverable but must never request process exit.
	actions = nil
	observation.RoborockLoopLastActive = now.Add(10 * time.Second)
	monitor = NewMonitor(Config{Enabled: true, StaleAfter: time.Hour, ReconnectAfter: time.Second,
		RebuildAfter: 2 * time.Second, ResetAfter: 3 * time.Second, RestartAfter: 4 * time.Second, MaxQueueDepth: 2},
		func(at time.Time) Observation { observation.ObservedAt = at; return observation }, Actions{Exit: func(string) { actions = append(actions, "restart") }}, nil)
	monitor.Step(now)
	for second := 1; second <= 6; second++ {
		monitor.Step(now.Add(time.Duration(second) * time.Second))
	}
	if len(actions) != 0 {
		t.Fatalf("cloud-only outage requested restart: %v", actions)
	}
}

func TestDirectLoxoneConfigurationErrorDoesNotReconnectRoborock(t *testing.T) {
	now := time.Unix(1700000000, 0)
	observation := Observation{
		StartedAt:              now.Add(-time.Hour),
		Authenticated:          true,
		BridgeStarted:          true,
		RoborockLoopLastActive: now,
		CloudConnected:         true,
		LastCloudMessage:       now,
		LastRobotUpdate:        now,
		DirectEnabled:          true,
		DirectLastSuccess:      now,
		DirectLastError:        "Loxone HTTP status 404",
	}
	actions := []string{}
	monitor := NewMonitor(Config{
		Enabled: true, StaleAfter: time.Hour, ReconnectAfter: time.Second,
		RebuildAfter: 2 * time.Second, ResetAfter: 3 * time.Second,
		RestartAfter: 4 * time.Second, MaxQueueDepth: 2,
	}, func(at time.Time) Observation {
		observation.ObservedAt = at
		return observation
	}, Actions{
		Reconnect: func(string) { actions = append(actions, "reconnect") },
		Rebuild:   func(string) { actions = append(actions, "rebuild") },
		Reset:     func(string) { actions = append(actions, "reset") },
		Exit:      func(string) { actions = append(actions, "restart") },
	}, nil)

	for second := 0; second <= 6; second++ {
		report := monitor.Step(now.Add(time.Duration(second) * time.Second))
		if report.Status != "degraded" || report.Ready {
			t.Fatalf("Direct error must remain visible as degraded readiness: %+v", report)
		}
	}
	if len(actions) != 0 {
		t.Fatalf("Direct configuration error triggered Roborock recovery: %v", actions)
	}
}

func TestRestartGuardPreventsRestartLoop(t *testing.T) {
	guard := NewRestartGuard(t.TempDir())
	now := time.Unix(1700000000, 0)
	for index := 0; index < 3; index++ {
		if !guard.Allow(now, 3, time.Hour) {
			t.Fatal("restart unexpectedly denied")
		}
		if err := guard.Record(now.Add(time.Duration(index)*time.Minute), "stuck loop"); err != nil {
			t.Fatal(err)
		}
	}
	if guard.Allow(now.Add(4*time.Minute), 3, time.Hour) {
		t.Fatal("restart loop was not suppressed")
	}
	reloaded := NewRestartGuard(filepathDir(guard.path))
	reason, _ := reloaded.Last()
	if reason != "stuck loop" {
		t.Fatalf("persisted reason=%q", reason)
	}
}

func TestRestartGuardPersistsRecoveryReasonWithoutCountingRestart(t *testing.T) {
	guard := NewRestartGuard(t.TempDir())
	now := time.Unix(1700000000, 0)
	if err := guard.RecordReason(now, "local_mqtt"); err != nil {
		t.Fatal(err)
	}
	if !guard.Allow(now, 1, time.Hour) {
		t.Fatal("a recovery reason was incorrectly counted as a restart")
	}
	reason, observedAt := guard.Last()
	if reason != "local_mqtt" || !observedAt.Equal(now) {
		t.Fatalf("last=(%q, %v), want persisted recovery reason", reason, observedAt)
	}
}

func filepathDir(path string) string {
	for index := len(path) - 1; index >= 0; index-- {
		if path[index] == '/' {
			return path[:index]
		}
	}
	return "."
}
