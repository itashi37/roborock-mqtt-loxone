package roborock

import (
	"testing"
	"time"
)

func TestCapabilitiesRemainUnknownWithoutEvidence(t *testing.T) {
	capabilities := NewCapabilityStore().Ensure("robot", time.Unix(1, 0))
	if capabilities.Locate.Supported != nil || capabilities.DockEmpty.Supported != nil || capabilities.MopWash.Supported != nil {
		t.Fatal("unverified advanced capabilities must remain unknown")
	}
}

func TestCapabilitiesUseAuthoritativeInventoryAndObservedStatus(t *testing.T) {
	store := NewCapabilityStore()
	now := time.Unix(10, 0)
	capabilities := store.UpdateInventory("robot", []RoomMapping{{SegmentID: 23}}, true, []Scene{{ID: 7}}, true, now)
	if capabilities.Rooms.Supported == nil || !*capabilities.Rooms.Supported || capabilities.Rooms.Source != "get_room_mapping" {
		t.Fatalf("unexpected rooms capability: %+v", capabilities.Rooms)
	}
	capabilities = store.ObserveStatus("robot", &PublishedStatus{FanSpeed: "turbo", MopMode: "deep", WaterBox: "moderate", State: "charging"}, now)
	if capabilities.Fan.Supported == nil || !*capabilities.Fan.Supported || capabilities.Dock.Supported == nil || !*capabilities.Dock.Supported {
		t.Fatalf("observed capabilities not recorded: %+v", capabilities)
	}
	if capabilities.DockEmpty.Supported != nil {
		t.Fatal("charging does not prove dock empty support")
	}
}
