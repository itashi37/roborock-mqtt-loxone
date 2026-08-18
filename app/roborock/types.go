package roborock

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// FlexInt handles JSON values that can be either int or string.
type FlexInt int

func (fi *FlexInt) UnmarshalJSON(b []byte) error {
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		*fi = FlexInt(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		parsed, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("cannot parse %q as int", s)
		}
		*fi = FlexInt(parsed)
		return nil
	}
	return fmt.Errorf("cannot unmarshal %s as int or string", string(b))
}

// LoginResponse represents the Roborock cloud API login response.
type LoginResponse struct {
	Code FlexInt   `json:"code"`
	Data LoginData `json:"data"`
}

type LoginData struct {
	UID      int    `json:"uid"`
	Token    string `json:"token"`
	RRIoT    RRIoT  `json:"rriot"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Nickname string `json:"nickname"`
}

type RRIoT struct {
	Remote    RRIoTRemote `json:"r"`
	SessionID string      `json:"s"`
	UserID    string      `json:"u"`
	HMACKey   string      `json:"h"`
	MQTTKey   string      `json:"k"`
}

type RRIoTRemote struct {
	APIURL     string `json:"a"`
	MQTTServer string `json:"m"`
	LogServer  string `json:"l"`
}

// HomeDetailResponse represents the response from getHomeDetail (base URL).
// Returns only the home ID, not devices.
type HomeDetailResponse struct {
	Code FlexInt        `json:"code"`
	Data HomeDetailData `json:"data"`
}

type HomeDetailData struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	RRHomeID int    `json:"rrHomeId"`
}

// HomeDataResponse represents the response from /user/homes/{id} (RRIOT API URL).
// Returns full home data including devices.
type HomeDataResponse struct {
	Success bool     `json:"success"`
	Result  HomeData `json:"result"`
}

type RoomInfo struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type HomeData struct {
	ID              int           `json:"id"`
	Name            string        `json:"name"`
	Devices         []DeviceInfo  `json:"devices"`
	ReceivedDevices []DeviceInfo  `json:"receivedDevices"`
	Products        []ProductInfo `json:"products"`
	Rooms           []RoomInfo    `json:"rooms"`
}

type DeviceInfo struct {
	DID       string `json:"duid"`
	Name      string `json:"name"`
	Model     string `json:"model"`
	Firmware  string `json:"fv"`
	Online    bool   `json:"online"`
	ProductID string `json:"productId"`
	DeviceKey string `json:"localKey"`
	PV        string `json:"pv"`
}

type ProductInfo struct {
	ID    string `json:"id"`
	Model string `json:"model"`
	Name  string `json:"name"`
}

// Scene represents a cleaning program/routine configured in the Roborock app.
type Scene struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"`
	Param any    `json:"param,omitempty"`
}

// ScenesResponse represents the response from the scenes API.
type ScenesResponse struct {
	Success bool    `json:"success"`
	Result  []Scene `json:"result"`
}

// DeviceStatus represents the current state of the vacuum.
type DeviceStatus struct {
	State     int `json:"state"`
	Battery   int `json:"battery"`
	CleanTime int `json:"clean_time"`
	CleanArea int `json:"clean_area"`
	// CleanPercent is the run completion percentage the robot reports directly.
	// Only newer models send it, so it is a pointer: nil means "not reported".
	CleanPercent         *int   `json:"clean_percent"`
	ErrorCode            int    `json:"error_code"`
	MapPresent           int    `json:"map_present"`
	InCleaning           int    `json:"in_cleaning"`
	InReturning          int    `json:"in_returning"`
	InFreshState         int    `json:"in_fresh_state"`
	FanPower             int    `json:"fan_power"`
	DNDEnabled           int    `json:"dnd_enabled"`
	WaterBoxStatus       int    `json:"water_box_status"`
	WaterBoxMode         int    `json:"water_box_custom_mode"`
	MopMode              int    `json:"mop_mode"`
	StateName            string `json:"state_name"`
	FanSpeedName         string `json:"fan_speed_name"`
	MopModeName          string `json:"mop_mode_name"`
	WaterBoxName         string `json:"water_box_name"`
	ErrorName            string `json:"error_name"`
	DockType             *int   `json:"dock_type,omitempty"`
	ChargeStatus         *int   `json:"charge_status,omitempty"`
	DockErrorStatus      *int   `json:"dock_error_status,omitempty"`
	DustCollectionStatus *int   `json:"dust_collection_status,omitempty"`
	WashStatus           *int   `json:"wash_status,omitempty"`
	DryStatus            *int   `json:"dry_status,omitempty"`
}

// ConsumableStatus represents consumable wear levels.
type ConsumableStatus struct {
	MainBrushWorkTime       int `json:"main_brush_work_time"`
	SideBrushWorkTime       int `json:"side_brush_work_time"`
	FilterWorkTime          int `json:"filter_work_time"`
	SensorDirtyTime         int `json:"sensor_dirty_time"`
	DustCollectionWorkTimes int `json:"dust_collection_work_times"`
}

// MQTTMessage represents the structure of messages sent/received via the Roborock MQTT protocol.
type MQTTMessage struct {
	DPS map[string]interface{} `json:"dps"`
	T   int64                  `json:"t"`
}

// IPCRequest represents an IPC request sent to the device.
type IPCRequest struct {
	ID       int         `json:"id"`
	Method   string      `json:"method"`
	Params   interface{} `json:"params"`
	Security interface{} `json:"security,omitempty"`
}

// IPCResponse represents an IPC response from the device.
type IPCResponse struct {
	ID     int         `json:"id"`
	Result interface{} `json:"result"`
}

// ConsumablePercents represents remaining percentage for each consumable.
type ConsumablePercents struct {
	MainBrush      int `json:"main_brush"`
	SideBrush      int `json:"side_brush"`
	Filter         int `json:"filter"`
	Sensor         int `json:"sensor"`
	DustCollection int `json:"dust_collection"`
}

// PublishedStatus is the status published to the local MQTT broker.
type PublishedStatus struct {
	State                string `json:"state"`
	Battery              int    `json:"battery"`
	FanSpeed             string `json:"fan_speed"`
	MopMode              string `json:"mop_mode"`
	WaterBox             string `json:"water_box"`
	CleanTime            int    `json:"clean_time"`
	CleanArea            int    `json:"clean_area"`
	ErrorCode            int    `json:"error_code"`
	Error                string `json:"error"`
	InCleaning           bool   `json:"in_cleaning"`
	DockType             *int   `json:"dock_type,omitempty"`
	ChargeStatus         *int   `json:"charge_status,omitempty"`
	DockErrorStatus      *int   `json:"dock_error_status,omitempty"`
	DustCollectionStatus *int   `json:"dust_collection_status,omitempty"`
	WashStatus           *int   `json:"wash_status,omitempty"`
	DryStatus            *int   `json:"dry_status,omitempty"`
	// CleanPercent is the run completion percentage reported directly by the
	// robot (newer models only). Unlike RemainingMinutes below it is not an
	// estimate; set only while cleaning, omitted when the robot does not report it.
	CleanPercent       *int               `json:"clean_percent,omitempty"`
	Consumables        ConsumableStatus   `json:"consumables"`
	ConsumablePercents ConsumablePercents `json:"consumable_percents"`
	// Program identifies the current cleaning run (scene id, room segments, or
	// cleaning mode); set while cleaning. RemainingMinutes, TimeCompleted and
	// RecordedMinutes are estimated from how long previous runs of the same
	// program took (Roborock reports no ETA) and are only set once a reference
	// run has been recorded. TimeCompleted is an RFC3339 timestamp with timezone
	// offset; RecordedMinutes is the reference duration the estimate is based on.
	Program          *string `json:"program,omitempty"`
	RemainingMinutes *int    `json:"remaining_minutes,omitempty"`
	TimeCompleted    *string `json:"time_completed,omitempty"`
	RecordedMinutes  *int    `json:"recorded_minutes,omitempty"`
}
