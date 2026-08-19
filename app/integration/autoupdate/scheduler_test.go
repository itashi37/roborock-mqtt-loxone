package autoupdate

import (
	"context"
	"testing"
	"time"

	"github.com/mqtt-home/roborock-mqtt/config"
	"github.com/mqtt-home/roborock-mqtt/integration/updates"
	"github.com/mqtt-home/roborock-mqtt/updater"
)

func boolean(value bool) *bool { return &value }

func baseSettings() config.UpdateConfig {
	return config.UpdateConfig{Mode: "automatic", Channel: "stable", WindowStart: "00:00", WindowEnd: "23:59", DelayHours: 24,
		AllowedDays: []int{0, 1, 2, 3, 4, 5, 6}, PreventRobotActive: boolean(true), PreventCleaning: boolean(true), PreventCommandInProgress: boolean(true)}
}

func newTestScheduler(t *testing.T, settings config.UpdateConfig, guard GuardState, installed *int) *Scheduler {
	t.Helper()
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.Local)
	return NewScheduler(Dependencies{
		Settings: func() config.UpdateConfig { return settings }, DataDir: t.TempDir(), CheckInterval: time.Hour,
		Check: func(context.Context, string) (updates.Info, error) {
			return updates.Info{Channel: settings.Channel, LatestVersion: "1.2.3", Available: true, PublishedAt: now.Add(-48 * time.Hour)}, nil
		},
		Install: func(context.Context, string) (updater.Operation, error) {
			(*installed)++
			return updater.Operation{ID: "operation", Stage: updater.StagePreparing}, nil
		},
		Operation: func(context.Context) (updater.Operation, error) {
			return updater.Operation{Stage: updater.StageIdle}, nil
		},
		Guard: func() GuardState { return guard },
	})
}

func TestNotifyOnlyNeverInstalls(t *testing.T) {
	settings := baseSettings()
	settings.Mode = "notify"
	installed := 0
	diagnostics := newTestScheduler(t, settings, GuardState{}, &installed).Step(context.Background(), time.Date(2026, 8, 19, 12, 0, 0, 0, time.Local))
	if installed != 0 || diagnostics.LastDecision != "update available; notification only" {
		t.Fatalf("installed=%d diagnostics=%+v", installed, diagnostics)
	}
}

func TestAutomaticStableUpdateStartsInsideWindow(t *testing.T) {
	installed := 0
	diagnostics := newTestScheduler(t, baseSettings(), GuardState{}, &installed).Step(context.Background(), time.Date(2026, 8, 19, 12, 0, 0, 0, time.Local))
	if installed != 1 || diagnostics.LastOperation != "operation" {
		t.Fatalf("installed=%d diagnostics=%+v", installed, diagnostics)
	}
}

func TestAutomaticEdgeRequiresExplicitAuthorization(t *testing.T) {
	settings := baseSettings()
	settings.Channel = "edge"
	installed := 0
	diagnostics := newTestScheduler(t, settings, GuardState{}, &installed).Step(context.Background(), time.Date(2026, 8, 19, 12, 0, 0, 0, time.Local))
	if installed != 0 || diagnostics.LastDecision != "edge automatic updates require explicit authorization" {
		t.Fatalf("installed=%d diagnostics=%+v", installed, diagnostics)
	}
}

func TestAutomaticUpdateDefersForRobotAndCommands(t *testing.T) {
	for name, guard := range map[string]GuardState{
		"cleaning": {Cleaning: true, RobotActive: true}, "active": {RobotActive: true}, "command": {CommandsInFlight: 1},
	} {
		t.Run(name, func(t *testing.T) {
			installed := 0
			diagnostics := newTestScheduler(t, baseSettings(), guard, &installed).Step(context.Background(), time.Date(2026, 8, 19, 12, 0, 0, 0, time.Local))
			if installed != 0 || diagnostics.LastDecision == "automatic update started" {
				t.Fatalf("installed=%d diagnostics=%+v", installed, diagnostics)
			}
		})
	}
}

func TestOvernightWindow(t *testing.T) {
	if !insideWindow(time.Date(2026, 8, 19, 23, 0, 0, 0, time.Local), "22:00", "03:00") || !insideWindow(time.Date(2026, 8, 19, 2, 0, 0, 0, time.Local), "22:00", "03:00") || insideWindow(time.Date(2026, 8, 19, 12, 0, 0, 0, time.Local), "22:00", "03:00") {
		t.Fatal("overnight update window was evaluated incorrectly")
	}
}
