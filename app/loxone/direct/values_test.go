package direct

import (
	"testing"
	"time"

	"github.com/mqtt-home/roborock-mqtt/roborock"
)

func TestValuesForStateUsesCorrectLoxoneTypesAndUnits(t *testing.T) {
	state := roborock.InternalDeviceState{
		Slug: "salon", Online: true, UpdatedAt: time.Unix(1700000000, 0),
		CurrentRoom: &roborock.CurrentRoom{ID: 23, Name: "Cuisine"},
		Status: &roborock.PublishedStatus{
			State: "washing_mop", Battery: 82, ErrorCode: 7, Error: "Blocked",
			CleanArea: 12_500_000, CleanTime: 321,
			ConsumablePercents: roborock.ConsumablePercents{MainBrush: 90, SideBrush: 80, Filter: 70, Sensor: 60},
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
		"online": {"1", Digital}, "battery": {"82", Analog},
		"state": {"14", Analog}, "state_text": {"washing_mop", Text},
		"current_room_name": {"Cuisine", Text}, "clean_area": {"12.50", Analog},
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
