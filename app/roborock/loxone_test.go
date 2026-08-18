package roborock

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestLoxoneCoreStoreMergesUpdates(t *testing.T) {
	store := NewLoxoneCoreStore()

	core := store.UpdateAvailability("vacuum", false)
	if core.Online != 0 || core.State != "offline" {
		t.Fatalf("unexpected offline core: %+v", core)
	}

	core = store.UpdateCurrentRoom("vacuum", &CurrentRoom{ID: 23, Name: "Cuisine"})
	if core.CurrentRoomID != 23 || core.CurrentRoomName != "Cuisine" || core.State != "offline" {
		t.Fatalf("room update lost existing core state: %+v", core)
	}

	observedAt := time.Unix(1_700_000_000, 0)
	core = store.UpdateStatus("vacuum", &PublishedStatus{
		State:     "washing_mop",
		Battery:   82,
		ErrorCode: 7,
	}, observedAt)
	want := LoxoneCore{
		Online:          1,
		State:           "washing_mop",
		Battery:         82,
		CurrentRoomID:   23,
		CurrentRoomName: "Cuisine",
		ErrorCode:       7,
		LastSeen:        1_700_000_000,
	}
	if core != want {
		t.Fatalf("got %+v, want %+v", core, want)
	}

	data, err := MarshalLoxoneCore(core)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	wantJSON := `{"online":1,"state":"washing_mop","battery":82,"current_room_id":23,"current_room_name":"Cuisine","error_code":7,"last_seen":1700000000}`
	if string(data) != wantJSON {
		t.Fatalf("got %s, want %s", data, wantJSON)
	}

	core = store.UpdateAvailability("vacuum", false)
	if core.State != "offline" || core.Online != 0 || core.CurrentRoomID != 23 || core.LastSeen != observedAt.Unix() {
		t.Fatalf("offline update did not preserve last known data: %+v", core)
	}
	core = store.UpdateAvailability("vacuum", true)
	if core.State != "unknown" || core.Online != 1 {
		t.Fatalf("online transition should await a fresh status: %+v", core)
	}
}

func TestLoxoneCoreStoreConcurrentUpdates(t *testing.T) {
	store := NewLoxoneCoreStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			store.UpdateAvailability("vacuum", i%2 == 0)
		}(i)
		go func(i int) {
			defer wg.Done()
			store.UpdateStatus("vacuum", &PublishedStatus{State: "cleaning", Battery: i}, time.Unix(int64(i), 0))
		}(i)
		go func(i int) {
			defer wg.Done()
			store.UpdateCurrentRoom("vacuum", &CurrentRoom{ID: i, Name: "Room"})
		}(i)
	}
	wg.Wait()

	core := store.Snapshot("vacuum")
	if core.State == "" {
		t.Fatal("expected a valid core snapshot")
	}
}

func TestNormalizeLoxoneState(t *testing.T) {
	tests := map[string]string{
		"idle":                  "idle",
		"cleaning":              "cleaning",
		"segment_cleaning":      "cleaning",
		"spot_cleaning":         "cleaning",
		"returning_home":        "returning",
		"docking":               "returning",
		"charging":              "charging",
		"fully_charged":         "docked",
		"washing_mop":           "washing_mop",
		"going_to_wash_mop":     "servicing_dock",
		"emptying_dustbin":      "emptying_dustbin",
		"mapping":               "mapping",
		"charging_problem":      "error",
		"unknown(999)":          "unknown",
		"a_future_vendor_state": "unknown",
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := NormalizeLoxoneState(input); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestLoxoneStatusScalars(t *testing.T) {
	observedAt := time.Unix(1_700_000_000, 0)
	status := &PublishedStatus{
		State:     "washing_mop",
		Battery:   82,
		CleanTime: 367,
		CleanArea: 42_500_000,
		ErrorCode: 0,
		Error:     "ignored when code is zero",
		ConsumablePercents: ConsumablePercents{
			MainBrush: 75,
			SideBrush: 64,
			Filter:    53,
			Sensor:    42,
		},
	}

	got := LoxoneStatusScalars(status, observedAt)
	want := map[string]string{
		"state":                  "washing_mop",
		"battery":                "82",
		"clean_area":             "42.50",
		"clean_time_seconds":     "367",
		"error_code":             "0",
		"error_text":             "",
		"last_seen":              "1700000000",
		"maintenance/main_brush": "75",
		"maintenance/side_brush": "64",
		"maintenance/filter":     "53",
		"maintenance/sensor":     "42",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestLoxoneStatusScalarsFallsBackForUnknownErrorText(t *testing.T) {
	got := LoxoneStatusScalars(&PublishedStatus{ErrorCode: 12}, time.Unix(1, 0))
	if got["error_text"] != "error_12" {
		t.Fatalf("got %q, want error_12", got["error_text"])
	}
}

func TestLoxoneStatusScalarsDoesNotOverwriteUnknownMaintenance(t *testing.T) {
	got := LoxoneStatusScalars(&PublishedStatus{State: "idle"}, time.Unix(1, 0))
	for _, suffix := range []string{
		"maintenance/main_brush",
		"maintenance/side_brush",
		"maintenance/filter",
		"maintenance/sensor",
	} {
		if _, exists := got[suffix]; exists {
			t.Fatalf("unexpected scalar %q when maintenance has not been polled", suffix)
		}
	}
}

func TestLoxoneCurrentRoomScalars(t *testing.T) {
	tests := []struct {
		name string
		room *CurrentRoom
		want map[string]string
	}{
		{
			name: "known room",
			room: &CurrentRoom{ID: 23, Name: "Cuisine"},
			want: map[string]string{"current_room_id": "23", "current_room_name": "Cuisine"},
		},
		{
			name: "unknown room",
			room: nil,
			want: map[string]string{"current_room_id": "0", "current_room_name": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LoxoneCurrentRoomScalars(tt.room); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseLoxoneCommand(t *testing.T) {
	rooms := map[string]string{"12": "Salon", "23": "Cuisine", "24": "Entrée"}
	scenes := []Scene{{ID: 101, Name: "Après les repas"}, {ID: 102, Name: "Nuit: silencieuse"}}

	tests := []struct {
		name    string
		payload string
		want    LoxoneCommand
	}{
		{name: "start", payload: " start ", want: LoxoneCommand{Action: "start"}},
		{name: "pause", payload: "pause", want: LoxoneCommand{Action: "pause"}},
		{name: "dock", payload: "dock", want: LoxoneCommand{Action: "dock"}},
		{name: "locate", payload: "locate", want: LoxoneCommand{Action: "locate"}},
		{name: "stop", payload: "stop", want: LoxoneCommand{Action: "stop"}},
		{name: "empty", payload: "empty_dustbin", want: LoxoneCommand{Action: "empty_dustbin"}},
		{name: "stop empty", payload: "stop_emptying", want: LoxoneCommand{Action: "stop_emptying"}},
		{name: "wash", payload: "wash_mop", want: LoxoneCommand{Action: "wash_mop"}},
		{name: "stop wash", payload: "stop_washing", want: LoxoneCommand{Action: "stop_washing"}},
		{name: "dry", payload: "dry_mop", want: LoxoneCommand{Action: "dry_mop"}},
		{name: "stop dry", payload: "stop_drying", want: LoxoneCommand{Action: "stop_drying"}},
		{name: "room by name case insensitive", payload: "clean_room:cuisine", want: LoxoneCommand{Action: "segment_clean", Segments: []int{23}}},
		{name: "rooms by name", payload: "clean_rooms: Cuisine, Salon, Cuisine ", want: LoxoneCommand{Action: "segment_clean", Segments: []int{23, 12}}},
		{name: "room by id", payload: "clean_room_id:23", want: LoxoneCommand{Action: "segment_clean", Segments: []int{23}}},
		{name: "rooms by ids", payload: "clean_room_ids:23,12,23", want: LoxoneCommand{Action: "segment_clean", Segments: []int{23, 12}}},
		{name: "scene by name", payload: "scene:après les repas", want: LoxoneCommand{Action: "scene", SceneID: 101}},
		{name: "scene name may contain colon", payload: "scene:Nuit: silencieuse", want: LoxoneCommand{Action: "scene", SceneID: 102}},
		{name: "scene by id", payload: "scene_id:101", want: LoxoneCommand{Action: "scene", SceneID: 101}},
		{name: "fan", payload: "fan:Turbo", want: LoxoneCommand{Action: "set_fan_speed", Speed: "turbo"}},
		{name: "mop", payload: "mop:Deep", want: LoxoneCommand{Action: "set_mop_mode", Mode: "deep"}},
		{name: "water", payload: "water:Moderate", want: LoxoneCommand{Action: "set_water_box", Level: "moderate"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLoxoneCommand(tt.payload, rooms, scenes)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseLoxoneCommandRejectsInvalidOrAmbiguousNames(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		rooms   map[string]string
		scenes  []Scene
	}{
		{name: "empty", payload: ""},
		{name: "unknown command", payload: "launch"},
		{name: "argument on simple command", payload: "start:now"},
		{name: "unknown room", payload: "clean_room:Bureau", rooms: map[string]string{"23": "Cuisine"}},
		{name: "ambiguous room", payload: "clean_room:Salon", rooms: map[string]string{"12": "Salon", "13": "salon"}},
		{name: "invalid room id", payload: "clean_room_id:abc"},
		{name: "cloud room id is not commandable", payload: "clean_room_id:23364799", rooms: map[string]string{"23": "Cuisine"}},
		{name: "old segment is not commandable", payload: "clean_room_id:15", rooms: map[string]string{"23": "Cuisine"}},
		{name: "room command fails closed without mapping", payload: "clean_room_id:23", rooms: map[string]string{}},
		{name: "mixed room ids reject inactive segment", payload: "clean_room_ids:23,15", rooms: map[string]string{"23": "Cuisine"}},
		{name: "unknown scene", payload: "scene:Repas", scenes: []Scene{{ID: 1, Name: "Nuit"}}},
		{name: "ambiguous scene", payload: "scene:Nuit", scenes: []Scene{{ID: 1, Name: "Nuit"}, {ID: 2, Name: "nuit"}}},
		{name: "invalid scene id", payload: "scene_id:0"},
		{name: "missing fan", payload: "fan:"},
		{name: "unknown fan", payload: "fan:warp"},
		{name: "missing mop", payload: "mop:"},
		{name: "unknown mop", payload: "mop:polish"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseLoxoneCommand(tt.payload, tt.rooms, tt.scenes); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
