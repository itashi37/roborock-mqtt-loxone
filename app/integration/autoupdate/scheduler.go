package autoupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mqtt-home/roborock-mqtt/config"
	"github.com/mqtt-home/roborock-mqtt/integration/updates"
	"github.com/mqtt-home/roborock-mqtt/updater"
)

type GuardState struct {
	RobotActive      bool `json:"robot_active"`
	Cleaning         bool `json:"cleaning"`
	CommandsInFlight int  `json:"commands_in_flight"`
}

type Diagnostics struct {
	LastCheck     time.Time  `json:"last_check,omitempty"`
	LastDecision  string     `json:"last_decision,omitempty"`
	LastVersion   string     `json:"last_version,omitempty"`
	LastAttempt   time.Time  `json:"last_attempt,omitempty"`
	LastOperation string     `json:"last_operation,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	Guard         GuardState `json:"guard"`
	NextWindow    time.Time  `json:"next_window,omitempty"`
}

type Dependencies struct {
	Settings      func() config.UpdateConfig
	Check         func(context.Context, string) (updates.Info, error)
	Install       func(context.Context, string) (updater.Operation, error)
	Operation     func(context.Context) (updater.Operation, error)
	Guard         func() GuardState
	DataDir       string
	CheckInterval time.Duration
	RetryCooldown time.Duration
}

type Scheduler struct {
	dependencies Dependencies
	mu           sync.RWMutex
	diagnostics  Diagnostics
	latest       updates.Info
	stop         chan struct{}
	done         chan struct{}
	statePath    string
}

func NewScheduler(dependencies Dependencies) *Scheduler {
	if dependencies.CheckInterval <= 0 {
		dependencies.CheckInterval = 6 * time.Hour
	}
	if dependencies.RetryCooldown <= 0 {
		dependencies.RetryCooldown = 6 * time.Hour
	}
	scheduler := &Scheduler{dependencies: dependencies, statePath: filepath.Join(dependencies.DataDir, "autoupdate-state.json")}
	if data, err := os.ReadFile(scheduler.statePath); err == nil {
		_ = json.Unmarshal(data, &scheduler.diagnostics)
	}
	return scheduler
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.stop != nil {
		s.mu.Unlock()
		return
	}
	s.stop, s.done = make(chan struct{}), make(chan struct{})
	stop, done := s.stop, s.done
	s.mu.Unlock()
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				s.Step(context.Background(), now)
			case <-stop:
				return
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.stop == nil {
		s.mu.Unlock()
		return
	}
	stop, done := s.stop, s.done
	s.stop, s.done = nil, nil
	close(stop)
	s.mu.Unlock()
	<-done
}

func (s *Scheduler) Diagnostics() Diagnostics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.diagnostics
}

func (s *Scheduler) Step(ctx context.Context, now time.Time) Diagnostics {
	settings := s.dependencies.Settings()
	if settings.Mode == "off" {
		return s.record(now, "updates disabled", "", GuardState{}, time.Time{})
	}
	previous := s.Diagnostics()
	s.mu.RLock()
	info := s.latest
	s.mu.RUnlock()
	if info.Channel != settings.Channel || previous.LastCheck.IsZero() || now.Sub(previous.LastCheck) >= s.dependencies.CheckInterval {
		var err error
		info, err = s.dependencies.Check(ctx, settings.Channel)
		if err != nil {
			return s.record(now, "update check failed", err.Error(), GuardState{}, time.Time{})
		}
		s.mu.Lock()
		s.diagnostics.LastCheck, s.diagnostics.LastVersion, s.latest = now, info.LatestVersion, info
		s.persistLocked()
		s.mu.Unlock()
	}
	if !info.Available {
		return s.record(now, "up to date", "", GuardState{}, time.Time{})
	}
	if settings.Mode == "notify" {
		return s.record(now, "update available; notification only", "", GuardState{}, time.Time{})
	}
	if settings.Channel == "edge" && !settings.AllowEdgeAutomatic {
		return s.record(now, "edge automatic updates require explicit authorization", "", GuardState{}, time.Time{})
	}
	if !info.PublishedAt.IsZero() && now.Before(info.PublishedAt.Add(time.Duration(settings.DelayHours)*time.Hour)) {
		return s.record(now, "waiting for publication delay", "", GuardState{}, nextEligibleWindow(now, settings))
	}
	if !allowedDay(now, settings.AllowedDays) || !insideWindow(now, settings.WindowStart, settings.WindowEnd) {
		return s.record(now, "outside the allowed update window", "", GuardState{}, nextEligibleWindow(now, settings))
	}
	guard := s.dependencies.Guard()
	if settings.BlocksCleaning() && guard.Cleaning {
		return s.record(now, "deferred: robot is cleaning", "", guard, nextEligibleWindow(now.Add(24*time.Hour), settings))
	}
	if settings.BlocksRobotActive() && guard.RobotActive {
		return s.record(now, "deferred: robot is active", "", guard, nextEligibleWindow(now.Add(24*time.Hour), settings))
	}
	if settings.BlocksCommands() && guard.CommandsInFlight > 0 {
		return s.record(now, "deferred: command in progress", "", guard, nextEligibleWindow(now.Add(time.Hour), settings))
	}
	if s.dependencies.Operation != nil {
		operation, operationErr := s.dependencies.Operation(ctx)
		if operationErr != nil {
			return s.record(now, "deferred: updater unavailable", operationErr.Error(), guard, nextEligibleWindow(now.Add(time.Hour), settings))
		}
		if operation.Stage != updater.StageIdle && operation.Stage != updater.StageSuccess && operation.Stage != updater.StageFailed {
			return s.record(now, "deferred: update operation already active", "", guard, nextEligibleWindow(now.Add(time.Hour), settings))
		}
	}
	if previous.LastVersion == info.LatestVersion && !previous.LastAttempt.IsZero() && now.Sub(previous.LastAttempt) < s.dependencies.RetryCooldown {
		return s.record(now, "waiting before retrying this version", "", guard, nextEligibleWindow(now.Add(s.dependencies.RetryCooldown), settings))
	}
	operation, err := s.dependencies.Install(ctx, settings.Channel)
	if err != nil {
		return s.recordAttempt(now, info.LatestVersion, "automatic update failed to start", err.Error(), "")
	}
	return s.recordAttempt(now, info.LatestVersion, "automatic update started", "", operation.ID)
}

func (s *Scheduler) record(now time.Time, decision, errorMessage string, guard GuardState, next time.Time) Diagnostics {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.diagnostics.LastDecision, s.diagnostics.LastError, s.diagnostics.Guard, s.diagnostics.NextWindow = decision, errorMessage, guard, next
	s.persistLocked()
	return s.diagnostics
}

func (s *Scheduler) recordAttempt(now time.Time, version, decision, errorMessage, operation string) Diagnostics {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.diagnostics.LastAttempt, s.diagnostics.LastVersion = now, version
	s.diagnostics.LastDecision, s.diagnostics.LastError, s.diagnostics.LastOperation = decision, errorMessage, operation
	s.persistLocked()
	return s.diagnostics
}

func (s *Scheduler) persistLocked() {
	data, err := json.MarshalIndent(s.diagnostics, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.statePath), 0700)
	temporary := s.statePath + ".tmp"
	if os.WriteFile(temporary, append(data, '\n'), 0600) == nil {
		_ = os.Rename(temporary, s.statePath)
	}
}

func allowedDay(now time.Time, days []int) bool {
	for _, day := range days {
		if day == int(now.Weekday()) {
			return true
		}
	}
	return false
}

func insideWindow(now time.Time, start, end string) bool {
	startMinute, startErr := minuteOfDay(start)
	endMinute, endErr := minuteOfDay(end)
	if startErr != nil || endErr != nil {
		return false
	}
	current := now.Hour()*60 + now.Minute()
	if startMinute == endMinute {
		return true
	}
	if startMinute < endMinute {
		return current >= startMinute && current < endMinute
	}
	return current >= startMinute || current < endMinute
}

func minuteOfDay(value string) (int, error) {
	pieces := strings.Split(value, ":")
	if len(pieces) != 2 {
		return 0, fmt.Errorf("invalid time")
	}
	hour, hourErr := strconv.Atoi(pieces[0])
	minute, minuteErr := strconv.Atoi(pieces[1])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("invalid time")
	}
	return hour*60 + minute, nil
}

func nextEligibleWindow(now time.Time, settings config.UpdateConfig) time.Time {
	start, err := minuteOfDay(settings.WindowStart)
	if err != nil {
		return time.Time{}
	}
	for offset := 0; offset <= 8; offset++ {
		day := now.AddDate(0, 0, offset)
		candidate := time.Date(day.Year(), day.Month(), day.Day(), start/60, start%60, 0, 0, now.Location())
		if candidate.Before(now) {
			continue
		}
		if allowedDay(candidate, settings.AllowedDays) {
			return candidate
		}
	}
	return time.Time{}
}
