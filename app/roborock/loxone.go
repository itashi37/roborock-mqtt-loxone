package roborock

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LoxoneCommand is the internal representation of a text command received
// through the optional Loxone MQTT contract.
type LoxoneCommand struct {
	Action   string
	Segments []int
	Speed    string
	Mode     string
	Level    string
	SceneID  int
}

// LoxoneCore is the compact retained state recommended for Loxone. Keeping it
// as a fixed struct makes the JSON contract and field order stable.
type LoxoneCore struct {
	Online          int    `json:"online"`
	State           string `json:"state"`
	Battery         int    `json:"battery"`
	CurrentRoomID   int    `json:"current_room_id"`
	CurrentRoomName string `json:"current_room_name"`
	ErrorCode       int    `json:"error_code"`
	LastSeen        int64  `json:"last_seen"`
}

// LoxoneCoreStore merges availability, status, and map updates into one
// coherent per-device snapshot.
type LoxoneCoreStore struct {
	mu      sync.Mutex
	devices map[string]LoxoneCore
}

func NewLoxoneCoreStore() *LoxoneCoreStore {
	return &LoxoneCoreStore{devices: make(map[string]LoxoneCore)}
}

func (s *LoxoneCoreStore) UpdateAvailability(slug string, online bool) LoxoneCore {
	s.mu.Lock()
	defer s.mu.Unlock()

	core := s.get(slug)
	if online {
		core.Online = 1
		if core.State == "offline" {
			core.State = "unknown"
		}
	} else {
		core.Online = 0
		core.State = "offline"
	}
	s.devices[slug] = core
	return core
}

func (s *LoxoneCoreStore) UpdateStatus(slug string, status *PublishedStatus, observedAt time.Time) LoxoneCore {
	s.mu.Lock()
	defer s.mu.Unlock()

	core := s.get(slug)
	if status != nil {
		core.Online = 1
		core.State = NormalizeLoxoneState(status.State)
		core.Battery = status.Battery
		core.ErrorCode = status.ErrorCode
		core.LastSeen = observedAt.Unix()
	}
	s.devices[slug] = core
	return core
}

func (s *LoxoneCoreStore) UpdateCurrentRoom(slug string, room *CurrentRoom) LoxoneCore {
	s.mu.Lock()
	defer s.mu.Unlock()

	core := s.get(slug)
	core.CurrentRoomID = 0
	core.CurrentRoomName = ""
	if room != nil {
		core.CurrentRoomID = room.ID
		core.CurrentRoomName = room.Name
	}
	s.devices[slug] = core
	return core
}

func (s *LoxoneCoreStore) Snapshot(slug string) LoxoneCore {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(slug)
}

func (s *LoxoneCoreStore) get(slug string) LoxoneCore {
	core, ok := s.devices[slug]
	if !ok {
		core.State = "unknown"
	}
	return core
}

func MarshalLoxoneCore(core LoxoneCore) ([]byte, error) {
	return json.Marshal(core)
}

// NormalizeLoxoneState maps model-specific Roborock states to a stable set of
// values while preserving useful dock-service and cleaning states.
func NormalizeLoxoneState(state string) string {
	switch state {
	case "starting":
		return "starting"
	case "charger_disconnected", "idle":
		return "idle"
	case "remote_control_active", "manual_mode", "in_call":
		return "manual"
	case "cleaning", "spot_cleaning", "zoned_cleaning", "segment_cleaning":
		return "cleaning"
	case "returning_home", "docking":
		return "returning"
	case "charging":
		return "charging"
	case "fully_charged":
		return "docked"
	case "paused":
		return "paused"
	case "error", "charging_problem":
		return "error"
	case "shutting_down":
		return "shutting_down"
	case "updating":
		return "updating"
	case "going_to_target":
		return "going_to_target"
	case "emptying_dustbin":
		return "emptying_dustbin"
	case "washing_mop":
		return "washing_mop"
	case "going_to_wash_mop":
		return "servicing_dock"
	case "mapping":
		return "mapping"
	default:
		return "unknown"
	}
}

// LoxoneStatusScalars converts the existing published status into text values
// that Loxone can consume without JSON parsing.
func LoxoneStatusScalars(status *PublishedStatus, observedAt time.Time) map[string]string {
	if status == nil {
		return nil
	}

	errorText := strings.TrimSpace(status.Error)
	if status.ErrorCode == 0 {
		errorText = ""
	} else if errorText == "" {
		errorText = fmt.Sprintf("error_%d", status.ErrorCode)
	}

	scalars := map[string]string{
		"state":              NormalizeLoxoneState(status.State),
		"battery":            strconv.Itoa(status.Battery),
		"clean_area":         strconv.FormatFloat(float64(status.CleanArea)/1_000_000, 'f', 2, 64),
		"clean_time_seconds": strconv.Itoa(status.CleanTime),
		"error_code":         strconv.Itoa(status.ErrorCode),
		"error_text":         errorText,
		"last_seen":          strconv.FormatInt(observedAt.Unix(), 10),
	}

	// Realtime status messages do not contain consumable data. Avoid replacing
	// known retained percentages with zero until a consumables poll has run.
	c := status.Consumables
	p := status.ConsumablePercents
	hasMaintenance := c.MainBrushWorkTime != 0 || c.SideBrushWorkTime != 0 ||
		c.FilterWorkTime != 0 || c.SensorDirtyTime != 0 || c.DustCollectionWorkTimes != 0 ||
		p.MainBrush != 0 || p.SideBrush != 0 || p.Filter != 0 || p.Sensor != 0 || p.DustCollection != 0
	if hasMaintenance {
		scalars["maintenance/main_brush"] = strconv.Itoa(p.MainBrush)
		scalars["maintenance/side_brush"] = strconv.Itoa(p.SideBrush)
		scalars["maintenance/filter"] = strconv.Itoa(p.Filter)
		scalars["maintenance/sensor"] = strconv.Itoa(p.Sensor)
	}

	return scalars
}

// LoxoneCurrentRoomScalars converts the optional current room into the two
// scalar values used by Loxone.
func LoxoneCurrentRoomScalars(room *CurrentRoom) map[string]string {
	if room == nil {
		return map[string]string{
			"current_room_id":   "0",
			"current_room_name": "",
		}
	}
	return map[string]string{
		"current_room_id":   strconv.Itoa(room.ID),
		"current_room_name": room.Name,
	}
}

// ParseLoxoneCommand parses the stable text command contract and resolves room
// and scene names against data already discovered by the bridge.
func ParseLoxoneCommand(payload string, roomNames map[string]string, scenes []Scene) (LoxoneCommand, error) {
	command := strings.TrimSpace(payload)
	if command == "" {
		return LoxoneCommand{}, fmt.Errorf("empty command")
	}

	verb, argument, hasArgument := strings.Cut(command, ":")
	verb = strings.ToLower(strings.TrimSpace(verb))
	argument = strings.TrimSpace(argument)

	switch verb {
	case "start", "pause", "dock":
		if hasArgument {
			return LoxoneCommand{}, fmt.Errorf("%s does not accept an argument", verb)
		}
		return LoxoneCommand{Action: verb}, nil
	case "clean_room":
		if !hasArgument || argument == "" {
			return LoxoneCommand{}, fmt.Errorf("clean_room requires a room name")
		}
		id, err := resolveUniqueRoomName(argument, roomNames)
		if err != nil {
			return LoxoneCommand{}, err
		}
		return LoxoneCommand{Action: "segment_clean", Segments: []int{id}}, nil
	case "clean_rooms":
		if !hasArgument || argument == "" {
			return LoxoneCommand{}, fmt.Errorf("clean_rooms requires room names")
		}
		names := splitNonEmpty(argument)
		if len(names) == 0 {
			return LoxoneCommand{}, fmt.Errorf("clean_rooms requires room names")
		}
		ids := make([]int, 0, len(names))
		for _, name := range names {
			id, err := resolveUniqueRoomName(name, roomNames)
			if err != nil {
				return LoxoneCommand{}, err
			}
			ids = appendUniqueInt(ids, id)
		}
		return LoxoneCommand{Action: "segment_clean", Segments: ids}, nil
	case "clean_room_id":
		if !hasArgument || argument == "" {
			return LoxoneCommand{}, fmt.Errorf("clean_room_id requires a room ID")
		}
		id, err := parsePositiveID(argument, "room")
		if err != nil {
			return LoxoneCommand{}, err
		}
		if err := validateCommandableRoomID(id, roomNames); err != nil {
			return LoxoneCommand{}, err
		}
		return LoxoneCommand{Action: "segment_clean", Segments: []int{id}}, nil
	case "clean_room_ids":
		if !hasArgument || argument == "" {
			return LoxoneCommand{}, fmt.Errorf("clean_room_ids requires room IDs")
		}
		parts := splitNonEmpty(argument)
		ids := make([]int, 0, len(parts))
		for _, part := range parts {
			id, err := parsePositiveID(part, "room")
			if err != nil {
				return LoxoneCommand{}, err
			}
			if err := validateCommandableRoomID(id, roomNames); err != nil {
				return LoxoneCommand{}, err
			}
			ids = appendUniqueInt(ids, id)
		}
		if len(ids) == 0 {
			return LoxoneCommand{}, fmt.Errorf("clean_room_ids requires room IDs")
		}
		return LoxoneCommand{Action: "segment_clean", Segments: ids}, nil
	case "scene":
		if !hasArgument || argument == "" {
			return LoxoneCommand{}, fmt.Errorf("scene requires a scene name")
		}
		id, err := resolveUniqueSceneName(argument, scenes)
		if err != nil {
			return LoxoneCommand{}, err
		}
		return LoxoneCommand{Action: "scene", SceneID: id}, nil
	case "scene_id":
		if !hasArgument || argument == "" {
			return LoxoneCommand{}, fmt.Errorf("scene_id requires a scene ID")
		}
		id, err := parsePositiveID(argument, "scene")
		if err != nil {
			return LoxoneCommand{}, err
		}
		return LoxoneCommand{Action: "scene", SceneID: id}, nil
	case "fan":
		if !hasArgument || argument == "" {
			return LoxoneCommand{}, fmt.Errorf("fan requires a speed")
		}
		speed := strings.ToLower(argument)
		if _, ok := fanSpeedMap[speed]; !ok {
			return LoxoneCommand{}, fmt.Errorf("unknown fan speed %q", argument)
		}
		return LoxoneCommand{Action: "set_fan_speed", Speed: speed}, nil
	case "mop":
		if !hasArgument || argument == "" {
			return LoxoneCommand{}, fmt.Errorf("mop requires a mode")
		}
		mode := strings.ToLower(argument)
		if _, ok := mopModeMap[mode]; !ok {
			return LoxoneCommand{}, fmt.Errorf("unknown mop mode %q", argument)
		}
		return LoxoneCommand{Action: "set_mop_mode", Mode: mode}, nil
	default:
		return LoxoneCommand{}, fmt.Errorf("unknown command %q", verb)
	}
}

func validateCommandableRoomID(id int, roomNames map[string]string) error {
	if _, ok := roomNames[strconv.Itoa(id)]; !ok {
		return fmt.Errorf("room %d is not commandable on the active map", id)
	}
	return nil
}

func resolveUniqueRoomName(name string, roomNames map[string]string) (int, error) {
	var matches []int
	for idText, candidate := range roomNames {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(name)) {
			id, err := strconv.Atoi(idText)
			if err == nil && id > 0 {
				matches = appendUniqueInt(matches, id)
			}
		}
	}
	if len(matches) == 0 {
		return 0, fmt.Errorf("unknown room %q", name)
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("ambiguous room %q", name)
	}
	return matches[0], nil
}

func resolveUniqueSceneName(name string, scenes []Scene) (int, error) {
	var matches []int
	for _, scene := range scenes {
		if strings.EqualFold(strings.TrimSpace(scene.Name), strings.TrimSpace(name)) {
			matches = appendUniqueInt(matches, scene.ID)
		}
	}
	if len(matches) == 0 {
		return 0, fmt.Errorf("unknown scene %q", name)
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("ambiguous scene %q", name)
	}
	return matches[0], nil
}

func splitNonEmpty(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parsePositiveID(value, kind string) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid %s ID %q", kind, value)
	}
	return id, nil
}

func appendUniqueInt(values []int, value int) []int {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
