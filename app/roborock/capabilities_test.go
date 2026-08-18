package roborock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCapabilitiesRemainUnknownWithoutEvidence(t *testing.T) {
	capabilities := NewCapabilityStore().Ensure("robot", time.Unix(1, 0))
	if capabilities.Locate.Supported != nil || capabilities.DockEmpty.Supported != nil || capabilities.MopWash.Supported != nil {
		t.Fatal("unverified advanced capabilities must remain unknown")
	}
}

func TestCapabilitiesPersistWithoutSecrets(t *testing.T) {
	dir := t.TempDir()
	store := NewCapabilityStore(dir)
	store.ObserveAdvancedDiagnostics("robot", AdvancedDiagnostics{Fields: map[string]any{"support_find_me": true}}, time.Unix(10, 0))
	reloaded := NewCapabilityStore(dir).Get("robot")
	if reloaded.Locate.Supported == nil || !*reloaded.Locate.Supported {
		t.Fatalf("capability was not restored: %+v", reloaded)
	}
	path := filepath.Join(dir, "device-capabilities.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions = %o", info.Mode().Perm())
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

func TestCapabilitiesUseDockStatusAndExplicitFeatureEvidence(t *testing.T) {
	store := NewCapabilityStore()
	now := time.Unix(20, 0)
	dockType, dust, wash, dry := 3, 0, 0, 1
	capabilities := store.ObserveStatus("robot", &PublishedStatus{
		DockType: &dockType, DustCollectionStatus: &dust, WashStatus: &wash, DryStatus: &dry,
	}, now)
	for name, capability := range map[string]Capability{
		"dock": capabilities.Dock, "dock_empty": capabilities.DockEmpty,
		"mop_wash": capabilities.MopWash, "mop_dry": capabilities.MopDry,
	} {
		if capability.Supported == nil || !*capability.Supported {
			t.Fatalf("%s was not detected: %+v", name, capability)
		}
	}
	capabilities = store.ObserveAdvancedDiagnostics("robot", AdvancedDiagnostics{Fields: map[string]any{
		"support_find_me": float64(1), "support_app_stop": true,
	}}, now.Add(time.Second))
	if capabilities.Locate.Supported == nil || !*capabilities.Locate.Supported || capabilities.Stop.Supported == nil || !*capabilities.Stop.Supported {
		t.Fatalf("explicit features not recorded: %+v", capabilities)
	}
	if capabilities.Locate.Confidence != CapabilityReported {
		t.Fatalf("confidence = %s", capabilities.Locate.Confidence)
	}
}

func TestNumericFeatureBitmaskDoesNotGuessCapabilities(t *testing.T) {
	store := NewCapabilityStore()
	capabilities := store.ObserveAdvancedDiagnostics("robot", AdvancedDiagnostics{Fields: map[string]any{"feature_flags": float64(918273)}}, time.Now())
	if capabilities.Locate.Supported != nil || capabilities.Stop.Supported != nil {
		t.Fatalf("bitmask must remain uninterpreted: %+v", capabilities)
	}
}
