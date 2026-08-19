package roborock

import (
	"sync"
	"testing"
	"time"
)

func TestCommandCoordinatorSharesValidationTrackerAndDispatch(t *testing.T) {
	var mu sync.Mutex
	var dispatched []LoxoneCommand
	var activities []LoxoneActivity
	context := CommandContext{
		Slug: "robot", Online: true,
		RoomNames:    map[string]string{"23": "Cuisine"},
		Scenes:       []Scene{{ID: 7, Name: "Repas"}},
		Capabilities: InitialDeviceCapabilities(time.Now()),
	}
	coordinator := NewCommandCoordinator(time.Minute, time.Second,
		func(slug string) (CommandContext, bool) { return context, slug == "robot" },
		func(_ CommandContext, command LoxoneCommand) error {
			mu.Lock()
			dispatched = append(dispatched, command)
			mu.Unlock()
			return nil
		},
		func(_ string, next []LoxoneActivity) {
			mu.Lock()
			activities = append(activities, next...)
			mu.Unlock()
		},
	)
	coordinator.UpdateStatus("robot", &PublishedStatus{State: "idle"}, time.Now())

	result := coordinator.SubmitText("robot", "clean_room_id:23")
	if !result.Accepted || result.State != "accepted" || result.ID == "" {
		t.Fatalf("unexpected accepted result: %+v", result)
	}
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		count := len(dispatched)
		mu.Unlock()
		if count == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(dispatched) != 1 || dispatched[0].Segments[0] != 23 {
		t.Fatalf("unexpected dispatches: %+v", dispatched)
	}
	if len(activities) < 2 || activities[0].State != "accepted" || activities[1].State != "running" {
		t.Fatalf("unexpected activities: %+v", activities)
	}
}

func TestCommandCoordinatorRejectsUnknownInventoryAndOffline(t *testing.T) {
	context := CommandContext{Slug: "robot", Online: true, RoomNames: map[string]string{"23": "Cuisine"}, Capabilities: InitialDeviceCapabilities(time.Now())}
	coordinator := NewCommandCoordinator(time.Second, time.Second,
		func(string) (CommandContext, bool) { return context, true },
		func(CommandContext, LoxoneCommand) error { t.Fatal("rejected command was dispatched"); return nil }, nil)
	if result := coordinator.SubmitText("robot", "clean_room_id:99"); result.State != "failed" {
		t.Fatalf("inactive room was not rejected: %+v", result)
	}
	context.Online = false
	if result := coordinator.SubmitText("robot", "start"); result.State != "failed" || result.Error != "robot offline" {
		t.Fatalf("offline command was not rejected: %+v", result)
	}
}

func TestCommandCoordinatorExposesInFlightDiagnostics(t *testing.T) {
	release := make(chan struct{})
	coordinator := NewCommandCoordinator(time.Second, time.Second,
		func(string) (CommandContext, bool) { return CommandContext{Slug: "robot", Online: true}, true },
		func(CommandContext, LoxoneCommand) error { <-release; return nil }, nil)
	coordinator.UpdateStatus("robot", &PublishedStatus{State: "idle"}, time.Now())
	result := coordinator.SubmitText("robot", "start")
	if !result.Accepted {
		t.Fatalf("command was not accepted: %+v", result)
	}
	deadline := time.Now().Add(time.Second)
	for coordinator.Diagnostics().InFlight != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if diagnostics := coordinator.Diagnostics(); diagnostics.InFlight != 1 || diagnostics.Oldest.IsZero() {
		t.Fatalf("unexpected in-flight diagnostics: %+v", diagnostics)
	}
	close(release)
	for coordinator.Diagnostics().InFlight != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if diagnostics := coordinator.Diagnostics(); diagnostics.InFlight != 0 || diagnostics.LastCompleted.IsZero() {
		t.Fatalf("unexpected completion diagnostics: %+v", diagnostics)
	}
}
