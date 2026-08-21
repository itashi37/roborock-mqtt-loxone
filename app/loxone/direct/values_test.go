package direct

import (
	"testing"
	"time"

	"github.com/mqtt-home/roborock-mqtt/roborock"
)

func TestValuesForStateUsesCorrectLoxoneTypesAndUnits(t *testing.T) {
	state := roborock.InternalDeviceState{
		Slug: "salon", Online: true, RobotOnline: true, UpdatedAt: time.Unix(1700000000, 0),
		CurrentRoom: &roborock.CurrentRoom{ID: 23, Name: "Cuisine"},
		Scenes:      []roborock.Scene{{ID: 42, Name: "Après les repas"}},
		Status: &roborock.PublishedStatus{
			State: "washing_mop", Battery: 82, ErrorCode: 7, Error: "Blocked",
			CleanArea: 12_500_000, CleanTime: 321,
			ConsumablePercents: roborock.ConsumablePercents{MainBrush: 90, SideBrush: 80, Filter: 70, Sensor: 60},
			Program:            func() *string { value := "scene:42"; return &value }(),
		},
	}
	values := ValuesForState(state, InputMapping{Prefix: "RR"})
	byField := make(map[string]StateValue)
	for _, value := range values {
		byField[value.Field] = value
	}
	checks := map[string]struct {
		value string
		kind  ValueKind
	}{
		"online": {"1", Digital}, "robot_online": {"1", Digital}, "running": {"1", Digital}, "battery": {"82", Analog},
		"state": {"14", Analog}, "state_text": {"washing_mop", Text},
		"current_room_name": {"Cuisine", Text}, "clean_area": {"12.50", Analog},
		"active_program": {"scene", Text}, "active_scene_id": {"42", Analog},
		"active_scene_name":  {"Après les repas", Text},
		"clean_time_seconds": {"321", Analog}, "last_seen": {"1700000000", Analog},
	}
	for field, check := range checks {
		if got := byField[field]; got.Value != check.value || got.Kind != check.kind {
			t.Fatalf("%s = %+v, want %q/%s", field, got, check.value, check.kind)
		}
	}
	if byField["battery"].Input != "RR_salon_battery" {
		t.Fatalf("unexpected default input: %s", byField["battery"].Input)
	}
}

func TestValuesForStateClearsActiveSceneAfterCleaning(t *testing.T) {
	values := ValuesForState(roborock.InternalDeviceState{Slug: "robot", UpdatedAt: time.Unix(1, 0), Status: &roborock.PublishedStatus{}}, InputMapping{})
	byField := make(map[string]StateValue)
	for _, value := range values {
		byField[value.Field] = value
	}
	if byField["active_program"].Value != "" || byField["active_scene_id"].Value != "0" || byField["active_scene_name"].Value != "" {
		t.Fatalf("active scene was not cleared: %+v", byField)
	}
	if byField["running"].Value != "0" || byField["running"].Kind != Digital {
		t.Fatalf("running was not cleared: %+v", byField["running"])
	}
}

func TestValuesForStateKeepsRunningDuringDockServiceInsideCleaningMission(t *testing.T) {
	program := "seg:7"
	values := ValuesForState(roborock.InternalDeviceState{
		Slug:      "robot",
		UpdatedAt: time.Unix(1, 0),
		Status:    &roborock.PublishedStatus{State: "washing_mop", Program: &program},
	}, InputMapping{})

	for _, value := range values {
		if value.Field == "running" {
			if value.Value != "1" || value.Kind != Digital {
				t.Fatalf("running during active mission = %+v, want 1/digital", value)
			}
			return
		}
	}
	t.Fatal("running Virtual Input is missing")
}

func TestInstallationValueUsesCompactBridgeInputNameAndOverride(t *testing.T) {
	value := InstallationValue("bridge_heartbeat", Analog, "1700000000", InputMapping{Prefix: "RR"})
	if value.Robot != "_bridge" || value.Input != "RR_bridge_heartbeat" || value.Value != "1700000000" {
		t.Fatalf("unexpected installation value: %+v", value)
	}
	overridden := InstallationValue("bridge_alive", Digital, "1", InputMapping{Overrides: map[string]map[string]string{"_bridge": {"bridge_alive": "Bridge OK"}}})
	if overridden.Input != "Bridge OK" {
		t.Fatalf("installation override not applied: %+v", overridden)
	}
}

func TestInputMappingOverride(t *testing.T) {
	values := ValuesForState(roborock.InternalDeviceState{Slug: "robot", UpdatedAt: time.Unix(1, 0)}, InputMapping{
		Prefix: "RR", Overrides: map[string]map[string]string{"robot": {"battery": "Custom Battery"}},
	})
	for _, value := range values {
		if value.Field == "battery" && value.Input != "Custom Battery" {
			t.Fatalf("override not applied: %+v", value)
		}
	}
}

func TestValuesForStateAddsOnlyReportedDockFields(t *testing.T) {
	dockType, wash := 3, 1
	values := ValuesForState(roborock.InternalDeviceState{Slug: "robot", UpdatedAt: time.Unix(1, 0), Status: &roborock.PublishedStatus{DockType: &dockType, WashStatus: &wash}}, InputMapping{})
	byField := make(map[string]StateValue)
	for _, value := range values {
		byField[value.Field] = value
	}
	if byField["dock_type"].Value != "3" || byField["wash_status"].Value != "1" {
		t.Fatalf("reported dock fields missing: %+v", byField)
	}
	if _, ok := byField["dry_status"]; ok {
		t.Fatal("unreported dry_status must not be pushed")
	}
}
