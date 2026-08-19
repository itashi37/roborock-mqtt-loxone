package roborock

import (
	"testing"
	"time"
)

func TestDeviceStateStorePublishesTransportIndependentSnapshots(t *testing.T) {
	store := NewDeviceStateStore()
	updates := make([]DeviceStateUpdate, 0)
	store.Subscribe(func(update DeviceStateUpdate) { updates = append(updates, update) })
	now := time.Unix(20, 0)
	device := &ManagedDevice{Info: DeviceInfo{DID: "did-a", Name: "Robot", Model: "qrevo"}, Slug: "robot"}
	capabilities := InitialDeviceCapabilities(now)
	store.Seed(device, capabilities, now)
	status := &PublishedStatus{State: "cleaning", Battery: 81}
	store.UpdateStatus("robot", status, capabilities, now.Add(time.Second))
	status.Battery = 1
	store.UpdateCurrentRoom("robot", &CurrentRoom{ID: 23, Name: "Cuisine"}, now.Add(2*time.Second))

	snapshot, ok := store.Get("robot")
	if !ok || snapshot.Status.Battery != 81 || snapshot.CurrentRoom.ID != 23 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if len(updates) != 3 || updates[1].Change != DeviceStateStatus || updates[2].Change != DeviceStateRoom {
		t.Fatalf("unexpected updates: %+v", updates)
	}
}

func TestDeviceStateStoreSeparatesCloudAvailabilityFromRobotResponse(t *testing.T) {
	store := NewDeviceStateStore()
	now := time.Unix(20, 0)
	device := &ManagedDevice{Info: DeviceInfo{DID: "did-a", Name: "Robot"}, Slug: "robot"}
	capabilities := InitialDeviceCapabilities(now)
	store.Seed(device, capabilities, now)
	store.UpdateAvailability("robot", true, now)
	store.UpdateHealth("robot", DeviceHealth{Online: true}, now)
	state, _ := store.Get("robot")
	if state.RobotOnline {
		t.Fatal("cloud connection without a robot status must not report robot_online")
	}
	store.UpdateStatus("robot", &PublishedStatus{State: "idle"}, capabilities, now)
	store.UpdateHealth("robot", DeviceHealth{Online: true}, now)
	state, _ = store.Get("robot")
	if !state.RobotOnline {
		t.Fatal("a responding robot should report robot_online")
	}
	store.UpdateHealth("robot", DeviceHealth{Online: true, ConsecutiveFailures: 3}, now)
	state, _ = store.Get("robot")
	if state.RobotOnline {
		t.Fatal("repeated polling failures must clear robot_online")
	}
}
