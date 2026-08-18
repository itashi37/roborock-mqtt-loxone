package direct

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mqtt-home/roborock-mqtt/roborock"
)

type ValueKind string

const (
	Digital ValueKind = "digital"
	Analog  ValueKind = "analog"
	Text    ValueKind = "text"
)

type StateValue struct {
	Robot string    `json:"robot"`
	Field string    `json:"field"`
	Input string    `json:"input"`
	Kind  ValueKind `json:"kind"`
	Value string    `json:"value"`
}

type InputMapping struct {
	Prefix    string
	Overrides map[string]map[string]string
}

var invalidInputName = regexp.MustCompile(`[^A-Za-z0-9_]+`)

func ValuesForState(state roborock.InternalDeviceState, mapping InputMapping) []StateValue {
	status := state.Status
	stateText := "unknown"
	battery, errorCode, cleanArea, cleanTime := 0, 0, 0, 0
	errorText := ""
	mainBrush, sideBrush, filter, sensor := 0, 0, 0, 0
	if status != nil {
		stateText = roborock.NormalizeLoxoneState(status.State)
		battery = status.Battery
		errorCode = status.ErrorCode
		errorText = status.Error
		cleanArea = status.CleanArea
		cleanTime = status.CleanTime
		mainBrush = status.ConsumablePercents.MainBrush
		sideBrush = status.ConsumablePercents.SideBrush
		filter = status.ConsumablePercents.Filter
		sensor = status.ConsumablePercents.Sensor
	}
	roomID, roomName := 0, ""
	if state.CurrentRoom != nil {
		roomID, roomName = state.CurrentRoom.ID, state.CurrentRoom.Name
	}
	online := "0"
	if state.Online {
		online = "1"
	}
	values := []struct {
		field string
		kind  ValueKind
		value string
	}{
		{"online", Digital, online},
		{"battery", Analog, strconv.Itoa(battery)},
		{"state", Analog, strconv.Itoa(DirectStateCode(stateText))},
		{"state_text", Text, stateText},
		{"current_room_id", Analog, strconv.Itoa(roomID)},
		{"current_room_name", Text, roomName},
		{"error_code", Analog, strconv.Itoa(errorCode)},
		{"error_text", Text, errorText},
		{"clean_area", Analog, strconv.FormatFloat(float64(cleanArea)/1_000_000, 'f', 2, 64)},
		{"clean_time_seconds", Analog, strconv.Itoa(cleanTime)},
		{"last_seen", Analog, strconv.FormatInt(state.UpdatedAt.Unix(), 10)},
		{"main_brush", Analog, strconv.Itoa(mainBrush)},
		{"side_brush", Analog, strconv.Itoa(sideBrush)},
		{"filter", Analog, strconv.Itoa(filter)},
		{"sensor", Analog, strconv.Itoa(sensor)},
	}
	result := make([]StateValue, 0, len(values))
	for _, value := range values {
		result = append(result, StateValue{Robot: state.Slug, Field: value.field, Input: inputName(state.Slug, value.field, mapping), Kind: value.kind, Value: value.value})
	}
	return result
}

func inputName(slug, field string, mapping InputMapping) string {
	if robot := mapping.Overrides[slug]; robot != nil {
		if override := strings.TrimSpace(robot[field]); override != "" {
			return override
		}
	}
	prefix := strings.TrimSpace(mapping.Prefix)
	if prefix == "" {
		prefix = "RR"
	}
	name := fmt.Sprintf("%s_%s_%s", prefix, slug, field)
	name = invalidInputName.ReplaceAllString(name, "_")
	return strings.Trim(name, "_")
}

func DirectStateCode(state string) int {
	codes := map[string]int{
		"unknown": 0, "idle": 1, "starting": 2, "cleaning": 3,
		"paused": 4, "returning": 5, "charging": 6, "docked": 7,
		"manual": 8, "error": 9, "shutting_down": 10, "updating": 11,
		"going_to_target": 12, "emptying_dustbin": 13, "washing_mop": 14,
		"servicing_dock": 15, "mapping": 16,
	}
	return codes[state]
}
