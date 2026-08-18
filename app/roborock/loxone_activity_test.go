package roborock

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMarshalLoxoneActivityContract(t *testing.T) {
	errorText := ""
	command := LoxoneActivity{Type: "command", TS: 1700000000, ID: "cmd-1", Command: "dock", State: "accepted", Error: &errorText}
	data, err := MarshalLoxoneActivity(command)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `{"type":"command","ts":1700000000,"id":"cmd-1","command":"dock","state":"accepted","error":""}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}

	event := LoxoneActivity{Type: "event", TS: 1700000001, Event: "room_entered", RoomID: 23, RoomName: "Cuisine"}
	data, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `{"type":"event","ts":1700000001,"event":"room_entered","room_id":23,"room_name":"Cuisine"}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestLoxoneCommandLifecycleAndTimeout(t *testing.T) {
	tracker := NewLoxoneActivityTracker(2 * time.Second)
	now := time.Unix(1700000000, 0)
	tracker.UpdateAvailability("vac", true, now)
	tracker.UpdateStatus("vac", &PublishedStatus{State: "idle"}, now)

	decision := tracker.BeginCommand("vac", "start", LoxoneCommand{Action: "start"}, nil, true, now)
	if !decision.Dispatch || len(decision.Activities) != 1 || decision.Activities[0].State != "accepted" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if running := tracker.MarkRunning("vac", decision.ID, now.Add(time.Second)); running == nil || running.State != "running" {
		t.Fatalf("unexpected running activity: %+v", running)
	}
	activities := tracker.UpdateStatus("vac", &PublishedStatus{State: "cleaning", InCleaning: true}, now.Add(2*time.Second))
	assertActivity(t, activities, "event", "cleaning_started", "")
	assertActivity(t, activities, "command", "", "completed")
	if expired := tracker.ExpireCommand("vac", decision.ID, now.Add(90*time.Second)); expired != nil {
		t.Fatalf("completed command expired: %+v", expired)
	}

	tracker.UpdateStatus("vac", &PublishedStatus{State: "idle"}, now.Add(3*time.Second))
	second := tracker.BeginCommand("vac", "start", LoxoneCommand{Action: "start"}, nil, true, now.Add(4*time.Second))
	if expired := tracker.ExpireCommand("vac", second.ID, now.Add(94*time.Second)); expired == nil || expired.State != "failed" || stringValue(expired.Error) != "confirmation timeout" {
		t.Fatalf("unexpected timeout activity: %+v", expired)
	}
}

func TestLoxoneCommandRejectsOfflineDuplicateAndIncompatible(t *testing.T) {
	tracker := NewLoxoneActivityTracker(2 * time.Second)
	now := time.Unix(1700000000, 0)
	offline := tracker.BeginCommand("vac", "start", LoxoneCommand{Action: "start"}, nil, false, now)
	if offline.Dispatch || stringValue(offline.Activities[0].Error) != "robot offline" {
		t.Fatalf("unexpected offline decision: %+v", offline)
	}

	tracker.UpdateStatus("vac", &PublishedStatus{State: "idle"}, now)
	first := tracker.BeginCommand("vac", "fan:turbo", LoxoneCommand{Action: "set_fan_speed", Speed: "turbo"}, nil, true, now)
	if !first.Dispatch {
		t.Fatalf("first command rejected: %+v", first)
	}
	duplicate := tracker.BeginCommand("vac", " FAN : TURBO ", LoxoneCommand{Action: "set_fan_speed", Speed: "turbo"}, nil, true, now.Add(time.Second))
	if duplicate.Dispatch || stringValue(duplicate.Activities[0].Error) != "duplicate command" {
		t.Fatalf("unexpected duplicate decision: %+v", duplicate)
	}

	tracker.UpdateStatus("vac", &PublishedStatus{State: "returning_home"}, now.Add(3*time.Second))
	incompatible := tracker.BeginCommand("vac", "clean_room_id:23", LoxoneCommand{Action: "segment_clean", Segments: []int{23}}, nil, true, now.Add(3*time.Second))
	if incompatible.Dispatch || !strings.Contains(stringValue(incompatible.Activities[0].Error), "incompatible") {
		t.Fatalf("unexpected incompatible decision: %+v", incompatible)
	}
}

func TestDockConfirmationRequiresActualDockArrival(t *testing.T) {
	tracker := NewLoxoneActivityTracker(time.Second)
	now := time.Unix(1700000000, 0)
	tracker.UpdateStatus("vac", &PublishedStatus{State: "cleaning", InCleaning: true}, now)
	decision := tracker.BeginCommand("vac", "dock", LoxoneCommand{Action: "dock"}, nil, true, now)
	if !decision.Dispatch {
		t.Fatalf("dock rejected: %+v", decision)
	}

	for i, status := range []*PublishedStatus{
		{State: "returning_home"},
		{State: "washing_mop", InCleaning: true},
		{State: "returning_home"},
	} {
		activities := tracker.UpdateStatus("vac", status, now.Add(time.Duration(i+1)*time.Second))
		if hasCommandState(activities, "completed") {
			t.Fatalf("dock completed from service/intermediate state: %+v", activities)
		}
	}

	activities := tracker.UpdateStatus("vac", &PublishedStatus{State: "charging"}, now.Add(4*time.Second))
	assertActivity(t, activities, "command", "", "completed")
	assertActivity(t, activities, "event", "returned_to_dock", "")
}

func TestLoxoneRobotEventTransitions(t *testing.T) {
	tracker := NewLoxoneActivityTracker(time.Second)
	now := time.Unix(1700000000, 0)
	if activities := tracker.UpdateStatus("vac", &PublishedStatus{State: "idle"}, now); len(activities) != 0 {
		t.Fatalf("baseline emitted activities: %+v", activities)
	}

	activities := tracker.UpdateStatus("vac", &PublishedStatus{State: "cleaning", InCleaning: true}, now.Add(time.Second))
	assertActivity(t, activities, "event", "cleaning_started", "")
	activities = tracker.UpdateRoom("vac", &CurrentRoom{ID: 23, Name: "Cuisine"}, now.Add(2*time.Second))
	assertActivity(t, activities, "event", "room_entered", "")
	if activities[0].RoomID != 23 || activities[0].RoomName != "Cuisine" {
		t.Fatalf("wrong room event: %+v", activities[0])
	}
	if repeated := tracker.UpdateRoom("vac", &CurrentRoom{ID: 23, Name: "Cuisine"}, now.Add(3*time.Second)); len(repeated) != 0 {
		t.Fatalf("repeated room emitted event: %+v", repeated)
	}

	activities = tracker.UpdateStatus("vac", &PublishedStatus{State: "paused"}, now.Add(4*time.Second))
	assertActivity(t, activities, "event", "paused", "")
	activities = tracker.UpdateStatus("vac", &PublishedStatus{State: "cleaning", InCleaning: true}, now.Add(5*time.Second))
	assertActivity(t, activities, "event", "resumed", "")
	activities = tracker.UpdateStatus("vac", &PublishedStatus{State: "returning_home"}, now.Add(6*time.Second))
	assertActivity(t, activities, "event", "cleaning_completed", "")
	activities = tracker.UpdateStatus("vac", &PublishedStatus{State: "charging"}, now.Add(7*time.Second))
	assertActivity(t, activities, "event", "returned_to_dock", "")
}

func TestLoxoneMidCycleMopWashDoesNotRestartCleaning(t *testing.T) {
	tracker := NewLoxoneActivityTracker(time.Second)
	now := time.Unix(1700000000, 0)
	tracker.UpdateStatus("vac", &PublishedStatus{State: "cleaning", InCleaning: true}, now)
	tracker.UpdateStatus("vac", &PublishedStatus{State: "washing_mop", InCleaning: true}, now.Add(time.Second))
	activities := tracker.UpdateStatus("vac", &PublishedStatus{State: "cleaning", InCleaning: true}, now.Add(2*time.Second))
	if hasEvent(activities, "cleaning_started") {
		t.Fatalf("mop wash emitted a second cleaning_started: %+v", activities)
	}
}

func TestLoxoneErrorCancelsCleaningCycle(t *testing.T) {
	tracker := NewLoxoneActivityTracker(time.Second)
	now := time.Unix(1700000000, 0)
	tracker.UpdateStatus("vac", &PublishedStatus{State: "cleaning", InCleaning: true}, now)
	activities := tracker.UpdateStatus("vac", &PublishedStatus{State: "error", ErrorCode: 12}, now.Add(time.Second))
	assertActivity(t, activities, "event", "error", "")
	if activities[0].ErrorCode != 12 || activities[0].ErrorText != "error_12" {
		t.Fatalf("wrong error event: %+v", activities[0])
	}
	activities = tracker.UpdateStatus("vac", &PublishedStatus{State: "idle"}, now.Add(2*time.Second))
	if hasEvent(activities, "cleaning_completed") {
		t.Fatalf("errored cycle emitted cleaning_completed: %+v", activities)
	}
}

func TestLoxoneOfflineFailsPendingCommands(t *testing.T) {
	tracker := NewLoxoneActivityTracker(time.Second)
	now := time.Unix(1700000000, 0)
	tracker.UpdateStatus("vac", &PublishedStatus{State: "idle"}, now)
	decision := tracker.BeginCommand("vac", "start", LoxoneCommand{Action: "start"}, nil, true, now)
	activities := tracker.UpdateAvailability("vac", false, now.Add(time.Second))
	assertActivity(t, activities, "command", "", "failed")
	if stringValue(activities[0].Error) != "robot offline" || activities[0].ID != decision.ID {
		t.Fatalf("wrong offline failure: %+v", activities[0])
	}
}

func TestLoxoneActivityTrackerConcurrentUpdates(t *testing.T) {
	tracker := NewLoxoneActivityTracker(time.Millisecond)
	now := time.Unix(1700000000, 0)
	tracker.UpdateStatus("vac", &PublishedStatus{State: "idle"}, now)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			tracker.UpdateStatus("vac", &PublishedStatus{State: "idle", Battery: i}, now.Add(time.Duration(i)*time.Millisecond))
		}(i)
		go func(i int) {
			defer wg.Done()
			tracker.BeginCommand("vac", "fan:turbo", LoxoneCommand{Action: "set_fan_speed", Speed: "turbo"}, nil, true, now.Add(time.Duration(i)*time.Millisecond))
		}(i)
	}
	wg.Wait()
}

func assertActivity(t *testing.T, activities []LoxoneActivity, activityType, event, state string) {
	t.Helper()
	for _, activity := range activities {
		if activity.Type == activityType && (event == "" || activity.Event == event) && (state == "" || activity.State == state) {
			return
		}
	}
	t.Fatalf("activity type=%q event=%q state=%q not found in %+v", activityType, event, state, activities)
}

func hasCommandState(activities []LoxoneActivity, state string) bool {
	for _, activity := range activities {
		if activity.Type == "command" && activity.State == state {
			return true
		}
	}
	return false
}

func hasEvent(activities []LoxoneActivity, event string) bool {
	for _, activity := range activities {
		if activity.Type == "event" && activity.Event == event {
			return true
		}
	}
	return false
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
