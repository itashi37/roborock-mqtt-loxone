package roborock

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

// Fan speed levels
const (
	FanQuiet    = 101
	FanBalanced = 102
	FanTurbo    = 103
	FanMax      = 104
	FanOff      = 105
)

// Mop mode levels
const (
	MopStandard = 300
	MopDeep     = 301
	MopDeepPlus = 303
)

// Water box levels
const (
	WaterOff      = 200
	WaterMild     = 201
	WaterModerate = 202
	WaterIntense  = 203
)

var fanSpeedMap = map[string]int{
	"quiet":    FanQuiet,
	"balanced": FanBalanced,
	"turbo":    FanTurbo,
	"max":      FanMax,
	"off":      FanOff,
}

var mopModeMap = map[string]int{
	"standard":  MopStandard,
	"deep":      MopDeep,
	"deep_plus": MopDeepPlus,
}

var waterBoxMap = map[string]int{
	"off":      WaterOff,
	"mild":     WaterMild,
	"moderate": WaterModerate,
	"intense":  WaterIntense,
}

var fanSpeedNames = map[int]string{
	FanQuiet:    "quiet",
	FanBalanced: "balanced",
	FanTurbo:    "turbo",
	FanMax:      "max",
	FanOff:      "off",
}

var mopModeNames = map[int]string{
	MopStandard: "standard",
	MopDeep:     "deep",
	MopDeepPlus: "deep_plus",
}

var waterBoxNames = map[int]string{
	WaterOff:      "off",
	WaterMild:     "mild",
	WaterModerate: "moderate",
	WaterIntense:  "intense",
}

var stateNames = map[int]string{
	1:  "starting",
	2:  "charger_disconnected",
	3:  "idle",
	4:  "remote_control_active",
	5:  "cleaning",
	6:  "returning_home",
	7:  "manual_mode",
	8:  "charging",
	9:  "charging_problem",
	10: "paused",
	11: "spot_cleaning",
	12: "error",
	13: "shutting_down",
	14: "updating",
	15: "docking",
	16: "going_to_target",
	17: "zoned_cleaning",
	18: "segment_cleaning",
	22: "emptying_dustbin",
	23: "washing_mop",
	26: "going_to_wash_mop",
	28: "in_call",
	29: "mapping",
	100: "fully_charged",
}

func nextRequestID() int {
	return 10000 + rand.Intn(22767)
}

// FanSpeedName returns the human-readable name for a fan speed value.
func FanSpeedName(speed int) string {
	if speed == 0 {
		return ""
	}
	if name, ok := fanSpeedNames[speed]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", speed)
}

// MopModeName returns the human-readable name for a mop mode value.
func MopModeName(mode int) string {
	if mode == 0 {
		return ""
	}
	if name, ok := mopModeNames[mode]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", mode)
}

// WaterBoxName returns the human-readable name for a water box level.
func WaterBoxName(level int) string {
	if level == 0 {
		return ""
	}
	if name, ok := waterBoxNames[level]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", level)
}

// StateName returns the human-readable name for a device state.
func StateName(state int) string {
	if state == 0 {
		return ""
	}
	if name, ok := stateNames[state]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", state)
}

// buildIPCPayload creates a full MQTT message payload for an IPC request.
// The DPS "101" value must be a JSON-encoded string, not a nested object.
func buildIPCPayload(method string, params any) ([]byte, int, error) {
	id := nextRequestID()
	ipcReq := IPCRequest{
		ID:     id,
		Method: method,
		Params: params,
	}

	// DPS "101" value is a JSON string, not a nested object
	ipcJSON, err := json.Marshal(ipcReq)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal IPC request: %w", err)
	}

	msg := MQTTMessage{
		DPS: map[string]any{
			"101": string(ipcJSON),
		},
		T: time.Now().Unix(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal MQTT message: %w", err)
	}

	return data, id, nil
}

// BuildStartPayload creates the payload for APP_START.
func BuildStartPayload() ([]byte, int, error) {
	return buildIPCPayload("app_start", []any{map[string]any{"clean_mop": 0}})
}

// BuildPausePayload creates the payload for APP_PAUSE.
func BuildPausePayload() ([]byte, int, error) {
	return buildIPCPayload("app_pause", []any{})
}

// BuildDockPayload creates the payload for APP_CHARGE.
func BuildDockPayload() ([]byte, int, error) {
	return buildIPCPayload("app_charge", []any{})
}

// BuildSegmentCleanPayload creates the payload for APP_SEGMENT_CLEAN.
func BuildSegmentCleanPayload(segments []int) ([]byte, int, error) {
	return buildIPCPayload("app_segment_clean", []any{map[string]any{"segments": segments}})
}

// BuildSetFanSpeedPayload creates the payload for SET_CUSTOM_MODE.
func BuildSetFanSpeedPayload(speed string) ([]byte, int, error) {
	value, ok := fanSpeedMap[speed]
	if !ok {
		return nil, 0, fmt.Errorf("unknown fan speed: %s", speed)
	}
	return buildIPCPayload("set_custom_mode", []int{value})
}

// BuildSetMopModePayload creates the payload for setting mop mode.
func BuildSetMopModePayload(mode string) ([]byte, int, error) {
	value, ok := mopModeMap[mode]
	if !ok {
		return nil, 0, fmt.Errorf("unknown mop mode: %s", mode)
	}
	return buildIPCPayload("set_mop_mode", []int{value})
}

// BuildSetWaterBoxPayload creates the payload for setting water box level.
func BuildSetWaterBoxPayload(level string) ([]byte, int, error) {
	value, ok := waterBoxMap[level]
	if !ok {
		return nil, 0, fmt.Errorf("unknown water box level: %s", level)
	}
	return buildIPCPayload("set_water_box_custom_mode", []int{value})
}

// ConsumableFieldNames maps short names to the IPC field names for reset.
var ConsumableFieldNames = map[string]string{
	"main_brush":      "main_brush_work_time",
	"side_brush":      "side_brush_work_time",
	"filter":          "filter_work_time",
	"sensor":          "sensor_dirty_time",
	"dust_collection": "dust_collection_work_times",
}

// BuildResetConsumablePayload creates the payload for reset_consumable.
func BuildResetConsumablePayload(name string) ([]byte, int, error) {
	fieldName, ok := ConsumableFieldNames[name]
	if !ok {
		return nil, 0, fmt.Errorf("unknown consumable: %s", name)
	}
	return buildIPCPayload("reset_consumable", []string{fieldName})
}

// BuildGetStatusPayload creates the payload for GET_PROP.
func BuildGetStatusPayload() ([]byte, int, error) {
	return buildIPCPayload("get_prop", []string{"get_status"})
}

// BuildGetConsumablePayload creates the payload for GET_CONSUMABLE.
func BuildGetConsumablePayload() ([]byte, int, error) {
	return buildIPCPayload("get_consumable", []any{})
}

// BuildGetRoomMappingPayload creates the request for the active map's
// commandable segment-to-home-room mapping.
func BuildGetRoomMappingPayload() ([]byte, int, error) {
	return buildIPCPayload("get_room_mapping", []any{})
}

// BuildGetMapPayload creates the payload for GET_MAP_V1 with security nonce.
func BuildGetMapPayload() ([]byte, int, *MapSecurityData, error) {
	security := GenerateMapSecurity()
	id := nextRequestID()
	ipcReq := IPCRequest{
		ID:     id,
		Method: "get_map_v1",
		Params: []any{},
		Security: map[string]string{
			"endpoint": security.Endpoint,
			"nonce":    security.Nonce,
		},
	}

	ipcJSON, err := json.Marshal(ipcReq)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("marshal IPC request: %w", err)
	}

	msg := MQTTMessage{
		DPS: map[string]any{
			"101": string(ipcJSON),
		},
		T: time.Now().Unix(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("marshal MQTT message: %w", err)
	}

	return data, id, security, nil
}

// ParseStatusFromProps parses device status from a GET_PROP response.
func ParseStatusFromProps(data []byte) (*DeviceStatus, error) {
	var result []DeviceStatus
	if err := json.Unmarshal(data, &result); err != nil {
		// Try single object
		var single DeviceStatus
		if err2 := json.Unmarshal(data, &single); err2 != nil {
			return nil, fmt.Errorf("parse device status: %w", err)
		}
		return &single, nil
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("empty status response")
	}
	return &result[0], nil
}

// ParseConsumableStatus parses consumable data from a GET_CONSUMABLE response.
func ParseConsumableStatus(data []byte) (*ConsumableStatus, error) {
	var result []ConsumableStatus
	if err := json.Unmarshal(data, &result); err != nil {
		var single ConsumableStatus
		if err2 := json.Unmarshal(data, &single); err2 != nil {
			return nil, fmt.Errorf("parse consumable status: %w", err)
		}
		return &single, nil
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("empty consumable response")
	}
	return &result[0], nil
}
