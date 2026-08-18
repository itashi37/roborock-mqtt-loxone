package roborock

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// LoxoneActivity is the compact, non-retained event contract consumed by
// Loxone. Command fields and event fields intentionally share one topic.
type LoxoneActivity struct {
	Type      string  `json:"type"`
	TS        int64   `json:"ts"`
	ID        string  `json:"id,omitempty"`
	Command   string  `json:"command,omitempty"`
	State     string  `json:"state,omitempty"`
	Error     *string `json:"error,omitempty"`
	Event     string  `json:"event,omitempty"`
	RoomID    int     `json:"room_id,omitempty"`
	RoomName  string  `json:"room_name,omitempty"`
	ErrorCode int     `json:"error_code,omitempty"`
	ErrorText string  `json:"error_text,omitempty"`
}

// LoxoneCommandDecision describes whether an accepted command still needs to
// be sent to the robot. Activities always contains at least one command result.
type LoxoneCommandDecision struct {
	ID         string
	Dispatch   bool
	Activities []LoxoneActivity
}

type loxonePendingCommand struct {
	id      string
	command string
	parsed  LoxoneCommand
}

type loxoneActivityDevice struct {
	online       bool
	hasStatus    bool
	status       PublishedStatus
	room         *CurrentRoom
	cycleActive  bool
	pending      map[string]*loxonePendingCommand
	lastCommands map[string]time.Time
}

// LoxoneActivityTracker correlates asynchronous MQTT commands with later
// Roborock status/map observations. It is safe for concurrent callbacks.
type LoxoneActivityTracker struct {
	mu       sync.Mutex
	devices  map[string]*loxoneActivityDevice
	debounce time.Duration
	sequence uint64
}

func NewLoxoneActivityTracker(debounce time.Duration) *LoxoneActivityTracker {
	return &LoxoneActivityTracker{
		devices:  make(map[string]*loxoneActivityDevice),
		debounce: debounce,
	}
}

func MarshalLoxoneActivity(activity LoxoneActivity) ([]byte, error) {
	return json.Marshal(activity)
}

func (t *LoxoneActivityTracker) UpdateAvailability(slug string, online bool, now time.Time) []LoxoneActivity {
	t.mu.Lock()
	defer t.mu.Unlock()

	device := t.device(slug)
	device.online = online
	if online {
		return nil
	}

	activities := make([]LoxoneActivity, 0, len(device.pending))
	for id, pending := range device.pending {
		activities = append(activities, commandActivity(now, pending, "failed", "robot offline"))
		delete(device.pending, id)
	}
	return activities
}

// BeginCommand validates availability, anti-rebound and current-state
// compatibility. parseErr is reported on /activity with a correlation ID.
func (t *LoxoneActivityTracker) BeginCommand(slug, raw string, parsed LoxoneCommand, parseErr error, online bool, now time.Time) LoxoneCommandDecision {
	t.mu.Lock()
	defer t.mu.Unlock()

	device := t.device(slug)
	device.online = online
	t.sequence++
	id := fmt.Sprintf("cmd-%d-%d", now.UnixMilli(), t.sequence)
	command := strings.TrimSpace(raw)
	pending := &loxonePendingCommand{id: id, command: command, parsed: parsed}
	fail := func(reason string) LoxoneCommandDecision {
		return LoxoneCommandDecision{ID: id, Activities: []LoxoneActivity{commandActivity(now, pending, "failed", reason)}}
	}

	if !online {
		return fail("robot offline")
	}
	if parseErr != nil {
		return fail(parseErr.Error())
	}

	canonical := canonicalLoxoneCommand(command)
	for previousCommand, seenAt := range device.lastCommands {
		if now.Sub(seenAt) >= t.debounce {
			delete(device.lastCommands, previousCommand)
		}
	}
	if previous, ok := device.lastCommands[canonical]; ok && now.Sub(previous) < t.debounce {
		return fail("duplicate command")
	}

	if reason := incompatibleLoxoneCommand(parsed, device); reason != "" {
		return fail(reason)
	}
	device.lastCommands[canonical] = now

	accepted := commandActivity(now, pending, "accepted", "")
	if loxoneCommandAlreadySatisfied(parsed, device) {
		completed := commandActivity(now, pending, "completed", "")
		return LoxoneCommandDecision{ID: id, Activities: []LoxoneActivity{accepted, completed}}
	}

	device.pending[id] = pending
	return LoxoneCommandDecision{ID: id, Dispatch: true, Activities: []LoxoneActivity{accepted}}
}

func (t *LoxoneActivityTracker) MarkRunning(slug, id string, now time.Time) *LoxoneActivity {
	t.mu.Lock()
	defer t.mu.Unlock()
	pending := t.device(slug).pending[id]
	if pending == nil {
		return nil
	}
	activity := commandActivity(now, pending, "running", "")
	return &activity
}

func (t *LoxoneActivityTracker) MarkFailed(slug, id, reason string, now time.Time) *LoxoneActivity {
	t.mu.Lock()
	defer t.mu.Unlock()
	device := t.device(slug)
	pending := device.pending[id]
	if pending == nil {
		return nil
	}
	delete(device.pending, id)
	activity := commandActivity(now, pending, "failed", reason)
	return &activity
}

func (t *LoxoneActivityTracker) ExpireCommand(slug, id string, now time.Time) *LoxoneActivity {
	return t.MarkFailed(slug, id, "confirmation timeout", now)
}

// UpdateStatus emits only transitions observed after a baseline status. It
// also confirms pending commands from the robot's reported state.
func (t *LoxoneActivityTracker) UpdateStatus(slug string, status *PublishedStatus, now time.Time) []LoxoneActivity {
	if status == nil {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	device := t.device(slug)
	device.online = true
	if !device.hasStatus {
		device.status = *status
		device.hasStatus = true
		device.cycleActive = isLoxoneCleaningState(status) || NormalizeLoxoneState(status.State) == "paused"
		return t.confirmPending(device, status, now)
	}

	previous := device.status
	prevState := NormalizeLoxoneState(previous.State)
	state := NormalizeLoxoneState(status.State)
	prevCleaning := isLoxoneCleaningState(&previous)
	cleaning := isLoxoneCleaningState(status)
	activities := make([]LoxoneActivity, 0, 4)

	if !prevCleaning && cleaning {
		if prevState == "paused" {
			activities = append(activities, eventActivity(now, "resumed"))
		} else if !device.cycleActive {
			activities = append(activities, eventActivity(now, "cleaning_started"))
			device.room = nil
		}
		device.cycleActive = true
	}
	if prevState != "paused" && state == "paused" {
		activities = append(activities, eventActivity(now, "paused"))
		device.cycleActive = true
	}
	if device.cycleActive && status.ErrorCode == 0 && state != "error" &&
		(state == "returning" || state == "idle" || state == "charging" || state == "docked") {
		activities = append(activities, eventActivity(now, "cleaning_completed"))
		device.cycleActive = false
	}
	if prevState == "returning" && (state == "charging" || state == "docked") {
		activities = append(activities, eventActivity(now, "returned_to_dock"))
	}
	if status.ErrorCode != 0 && (previous.ErrorCode != status.ErrorCode || prevState != "error" && state == "error") {
		activity := eventActivity(now, "error")
		activity.ErrorCode = status.ErrorCode
		activity.ErrorText = loxoneErrorText(status)
		activities = append(activities, activity)
		device.cycleActive = false
	} else if status.ErrorCode == 0 && prevState != "error" && state == "error" {
		activity := eventActivity(now, "error")
		activity.ErrorText = loxoneErrorText(status)
		activities = append(activities, activity)
		device.cycleActive = false
	}
	activities = append(activities, t.confirmPending(device, status, now)...)
	device.status = *status
	return activities
}

func (t *LoxoneActivityTracker) UpdateRoom(slug string, room *CurrentRoom, now time.Time) []LoxoneActivity {
	t.mu.Lock()
	defer t.mu.Unlock()
	device := t.device(slug)
	previousID := 0
	if device.room != nil {
		previousID = device.room.ID
	}
	if room == nil {
		device.room = nil
		return nil
	}
	copyRoom := *room
	device.room = &copyRoom
	if !device.hasStatus || !isLoxoneCleaningState(&device.status) || room.ID == 0 || room.ID == previousID {
		return nil
	}
	activity := eventActivity(now, "room_entered")
	activity.RoomID = room.ID
	activity.RoomName = room.Name
	return []LoxoneActivity{activity}
}

func (t *LoxoneActivityTracker) confirmPending(device *loxoneActivityDevice, status *PublishedStatus, now time.Time) []LoxoneActivity {
	activities := make([]LoxoneActivity, 0)
	state := NormalizeLoxoneState(status.State)
	for id, pending := range device.pending {
		confirmed := false
		switch pending.parsed.Action {
		case "start", "segment_clean", "scene":
			confirmed = isLoxoneCleaningState(status)
		case "pause":
			confirmed = state == "paused"
		case "dock":
			// A dock command is complete only on an actual charging/docked
			// observation. Dock-service states such as washing_mop are not
			// sufficient because they also occur mid-cleaning.
			confirmed = state == "charging" || state == "docked"
		case "set_fan_speed":
			confirmed = strings.EqualFold(status.FanSpeed, pending.parsed.Speed)
		case "set_mop_mode":
			confirmed = strings.EqualFold(status.MopMode, pending.parsed.Mode)
		}
		if confirmed {
			activities = append(activities, commandActivity(now, pending, "completed", ""))
			delete(device.pending, id)
		}
	}
	return activities
}

func (t *LoxoneActivityTracker) device(slug string) *loxoneActivityDevice {
	device := t.devices[slug]
	if device == nil {
		device = &loxoneActivityDevice{
			pending:      make(map[string]*loxonePendingCommand),
			lastCommands: make(map[string]time.Time),
		}
		t.devices[slug] = device
	}
	return device
}

func commandActivity(now time.Time, pending *loxonePendingCommand, state, reason string) LoxoneActivity {
	return LoxoneActivity{Type: "command", TS: now.Unix(), ID: pending.id, Command: pending.command, State: state, Error: &reason}
}

func eventActivity(now time.Time, event string) LoxoneActivity {
	return LoxoneActivity{Type: "event", TS: now.Unix(), Event: event}
}

func canonicalLoxoneCommand(command string) string {
	parts := strings.Split(command, ":")
	for i := range parts {
		parts[i] = strings.ToLower(strings.TrimSpace(parts[i]))
	}
	return strings.Join(parts, ":")
}

func incompatibleLoxoneCommand(command LoxoneCommand, device *loxoneActivityDevice) string {
	if !device.hasStatus {
		return "robot status unavailable"
	}
	state := NormalizeLoxoneState(device.status.State)
	switch command.Action {
	case "pause":
		if state != "paused" && !isLoxoneCleaningState(&device.status) {
			return "pause incompatible with current state"
		}
	case "segment_clean", "scene":
		if isLoxoneCleaningState(&device.status) || state == "paused" || state == "returning" ||
			state == "washing_mop" || state == "servicing_dock" || state == "emptying_dustbin" ||
			state == "updating" || state == "mapping" {
			return "cleaning command incompatible with current state"
		}
	case "start":
		if state == "returning" || state == "washing_mop" || state == "servicing_dock" ||
			state == "emptying_dustbin" || state == "updating" || state == "mapping" ||
			state == "error" || state == "shutting_down" {
			return "start incompatible with current state"
		}
	case "dock":
		if state == "updating" || state == "mapping" || state == "error" || state == "shutting_down" {
			return "dock incompatible with current state"
		}
	}
	return ""
}

func loxoneCommandAlreadySatisfied(command LoxoneCommand, device *loxoneActivityDevice) bool {
	if !device.hasStatus {
		return false
	}
	state := NormalizeLoxoneState(device.status.State)
	switch command.Action {
	case "start":
		return isLoxoneCleaningState(&device.status)
	case "pause":
		return state == "paused"
	case "dock":
		return state == "charging" || state == "docked"
	case "set_fan_speed":
		return strings.EqualFold(device.status.FanSpeed, command.Speed)
	case "set_mop_mode":
		return strings.EqualFold(device.status.MopMode, command.Mode)
	default:
		return false
	}
}

func isLoxoneCleaningState(status *PublishedStatus) bool {
	return status != nil && (status.InCleaning || NormalizeLoxoneState(status.State) == "cleaning")
}

func loxoneErrorText(status *PublishedStatus) string {
	text := strings.TrimSpace(status.Error)
	if text == "" && status.ErrorCode != 0 {
		return fmt.Sprintf("error_%d", status.ErrorCode)
	}
	if text == "" {
		return "robot error"
	}
	return text
}
