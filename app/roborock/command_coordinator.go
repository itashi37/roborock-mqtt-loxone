package roborock

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CommandContext struct {
	Slug         string
	Online       bool
	RoomNames    map[string]string
	Scenes       []Scene
	Capabilities DeviceCapabilities
}

type CommandSubmission struct {
	ID       string                `json:"id"`
	Command  string                `json:"command"`
	State    string                `json:"state"`
	Error    string                `json:"error,omitempty"`
	Accepted bool                  `json:"accepted"`
	Failure  string                `json:"-"`
	Decision LoxoneCommandDecision `json:"-"`
}

type CommandCoordinator struct {
	tracker       *LoxoneActivityTracker
	timeout       time.Duration
	resolve       func(string) (CommandContext, bool)
	dispatch      func(CommandContext, LoxoneCommand) error
	onActivity    func(string, []LoxoneActivity)
	diagnosticsMu sync.RWMutex
	inFlight      map[string]time.Time
	lastCompleted time.Time
}

func NewCommandCoordinator(
	debounce, timeout time.Duration,
	resolve func(string) (CommandContext, bool),
	dispatch func(CommandContext, LoxoneCommand) error,
	onActivity func(string, []LoxoneActivity),
) *CommandCoordinator {
	return &CommandCoordinator{
		tracker: NewLoxoneActivityTracker(debounce), timeout: timeout, inFlight: make(map[string]time.Time),
		resolve: resolve, dispatch: dispatch, onActivity: onActivity,
	}
}

func (c *CommandCoordinator) Tracker() *LoxoneActivityTracker { return c.tracker }

func (c *CommandCoordinator) SubmitText(slug, raw string) CommandSubmission {
	context, ok := c.resolveContext(slug)
	if !ok {
		return c.submitUnknownDevice(slug, raw)
	}
	command, err := ParseLoxoneCommand(raw, context.RoomNames, context.Scenes)
	return c.SubmitParsed(context, raw, command, err)
}

func (c *CommandCoordinator) SubmitParsed(context CommandContext, raw string, command LoxoneCommand, parseErr error) CommandSubmission {
	if parseErr == nil {
		parseErr = validateCommandInventory(command, context)
	}
	now := time.Now()
	decision := c.tracker.BeginCommand(context.Slug, raw, command, parseErr, context.Online, now)
	c.emit(context.Slug, decision.Activities)
	result := submissionFromDecision(raw, decision)
	if !decision.Dispatch {
		return result
	}
	c.diagnosticsMu.Lock()
	c.inFlight[decision.ID] = now
	c.diagnosticsMu.Unlock()

	time.AfterFunc(c.timeout, func() {
		if activity := c.tracker.ExpireCommand(context.Slug, decision.ID, time.Now()); activity != nil {
			c.emit(context.Slug, []LoxoneActivity{*activity})
		}
	})
	go func() {
		defer c.completeDispatch(decision.ID)
		if err := c.dispatch(context, command); err != nil {
			if activity := c.tracker.MarkFailed(context.Slug, decision.ID, err.Error(), time.Now()); activity != nil {
				c.emit(context.Slug, []LoxoneActivity{*activity})
			}
			return
		}
		if activity := c.tracker.MarkRunning(context.Slug, decision.ID, time.Now()); activity != nil {
			c.emit(context.Slug, []LoxoneActivity{*activity})
		}
		if command.Action == "locate" {
			if activity := c.tracker.MarkCompleted(context.Slug, decision.ID, time.Now()); activity != nil {
				c.emit(context.Slug, []LoxoneActivity{*activity})
			}
		}
	}()
	return result
}

type DispatcherDiagnostics struct {
	InFlight      int       `json:"in_flight"`
	Oldest        time.Time `json:"oldest,omitempty"`
	LastCompleted time.Time `json:"last_completed,omitempty"`
}

func (c *CommandCoordinator) Diagnostics() DispatcherDiagnostics {
	if c == nil {
		return DispatcherDiagnostics{}
	}
	c.diagnosticsMu.RLock()
	defer c.diagnosticsMu.RUnlock()
	result := DispatcherDiagnostics{InFlight: len(c.inFlight), LastCompleted: c.lastCompleted}
	for _, started := range c.inFlight {
		if result.Oldest.IsZero() || started.Before(result.Oldest) {
			result.Oldest = started
		}
	}
	return result
}

func (c *CommandCoordinator) completeDispatch(id string) {
	c.diagnosticsMu.Lock()
	delete(c.inFlight, id)
	c.lastCompleted = time.Now()
	c.diagnosticsMu.Unlock()
}

func (c *CommandCoordinator) UpdateStatus(slug string, status *PublishedStatus, now time.Time) {
	c.emit(slug, c.tracker.UpdateStatus(slug, status, now))
}

func (c *CommandCoordinator) UpdateRoom(slug string, room *CurrentRoom, now time.Time) {
	c.emit(slug, c.tracker.UpdateRoom(slug, room, now))
}

func (c *CommandCoordinator) UpdateAvailability(slug string, online bool, now time.Time) {
	c.emit(slug, c.tracker.UpdateAvailability(slug, online, now))
}

func (c *CommandCoordinator) resolveContext(slug string) (CommandContext, bool) {
	if c == nil || c.resolve == nil {
		return CommandContext{}, false
	}
	return c.resolve(slug)
}

func (c *CommandCoordinator) submitUnknownDevice(slug, raw string) CommandSubmission {
	// Keep availability true here so the more precise resolution error is not
	// masked by the tracker's generic offline protection.
	context := CommandContext{Slug: slug, Online: true}
	return c.SubmitParsed(context, raw, LoxoneCommand{}, fmt.Errorf("unknown robot %q", slug))
}

func (c *CommandCoordinator) emit(slug string, activities []LoxoneActivity) {
	if len(activities) > 0 && c.onActivity != nil {
		c.onActivity(slug, activities)
	}
}

func submissionFromDecision(raw string, decision LoxoneCommandDecision) CommandSubmission {
	result := CommandSubmission{ID: decision.ID, Command: strings.TrimSpace(raw), Accepted: decision.Dispatch, Decision: decision}
	if len(decision.Activities) == 0 {
		return result
	}
	activity := decision.Activities[len(decision.Activities)-1]
	result.State = activity.State
	if activity.Error != nil {
		result.Error = *activity.Error
	}
	if result.State == "completed" {
		result.Accepted = true
	}
	if result.State == "failed" {
		result.Failure = classifyCommandFailure(result.Error)
	}
	return result
}

func classifyCommandFailure(message string) string {
	message = strings.ToLower(message)
	switch {
	case strings.Contains(message, "unknown robot"), strings.Contains(message, "unknown scene"),
		strings.Contains(message, "unknown or inactive room"), strings.Contains(message, "not supported"),
		strings.Contains(message, "capability"):
		return "not_found"
	case strings.Contains(message, "offline"), strings.Contains(message, "duplicate"),
		strings.Contains(message, "incompatible"), strings.Contains(message, "status unavailable"):
		return "conflict"
	default:
		return "invalid"
	}
}

func validateCommandInventory(command LoxoneCommand, context CommandContext) error {
	switch command.Action {
	case "segment_clean":
		for _, segment := range command.Segments {
			if _, ok := context.RoomNames[strconv.Itoa(segment)]; !ok {
				return fmt.Errorf("unknown or inactive room segment %d", segment)
			}
		}
		if context.Capabilities.Rooms.Supported != nil && !*context.Capabilities.Rooms.Supported {
			return fmt.Errorf("room cleaning is not supported by this robot")
		}
	case "scene":
		found := false
		for _, scene := range context.Scenes {
			if scene.ID == command.SceneID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown scene %d", command.SceneID)
		}
		if context.Capabilities.Scenes.Supported != nil && !*context.Capabilities.Scenes.Supported {
			return fmt.Errorf("scenes are not supported by this robot")
		}
	case "locate":
		if context.Capabilities.Locate.Supported == nil || !*context.Capabilities.Locate.Supported {
			return fmt.Errorf("capability locate is not confirmed")
		}
	case "stop":
		if context.Capabilities.Stop.Supported == nil || !*context.Capabilities.Stop.Supported {
			return fmt.Errorf("capability stop is not confirmed")
		}
	case "empty_dustbin", "stop_emptying":
		if context.Capabilities.DockEmpty.Supported == nil || !*context.Capabilities.DockEmpty.Supported {
			return fmt.Errorf("capability dock_empty is not confirmed")
		}
	case "wash_mop", "stop_washing":
		if context.Capabilities.MopWash.Supported == nil || !*context.Capabilities.MopWash.Supported {
			return fmt.Errorf("capability mop_wash is not confirmed")
		}
	case "dry_mop", "stop_drying":
		if context.Capabilities.MopDry.Supported == nil || !*context.Capabilities.MopDry.Supported {
			return fmt.Errorf("capability mop_dry is not confirmed")
		}
	case "set_fan_speed":
		if context.Capabilities.Fan.Supported != nil && !*context.Capabilities.Fan.Supported {
			return fmt.Errorf("fan control is not supported by this robot")
		}
	case "set_mop_mode":
		if context.Capabilities.Mop.Supported != nil && !*context.Capabilities.Mop.Supported {
			return fmt.Errorf("mop control is not supported by this robot")
		}
	case "set_water_box":
		if context.Capabilities.Water.Supported != nil && !*context.Capabilities.Water.Supported {
			return fmt.Errorf("water control is not supported by this robot")
		}
	}
	return nil
}
