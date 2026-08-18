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
