package roborock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const loxoneRoomOverridesFile = "loxone-room-overrides.json"

// LoxoneRoomOverrideStore persists UI-managed display names separately from
// config.json. Values are keyed by device slug and Roborock room ID.
type LoxoneRoomOverrideStore struct {
	mu      sync.RWMutex
	path    string
	devices map[string]map[string]string
}

func NewLoxoneRoomOverrideStore(dataDir string) (*LoxoneRoomOverrideStore, error) {
	store := &LoxoneRoomOverrideStore{
		path:    filepath.Join(dataDir, loxoneRoomOverridesFile),
		devices: make(map[string]map[string]string),
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("read Loxone room overrides: %w", err)
	}
	if err := json.Unmarshal(data, &store.devices); err != nil {
		return nil, fmt.Errorf("parse Loxone room overrides: %w", err)
	}
	return store, nil
}

func (s *LoxoneRoomOverrideStore) ForDevice(slug string) map[string]string {
	if s == nil {
		return map[string]string{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]string, len(s.devices[slug]))
	for id, name := range s.devices[slug] {
		result[id] = name
	}
	return result
}

// Set validates that the effective room names remain unique before persisting
// an override. baseNames must contain the API/config names before UI overrides.
func (s *LoxoneRoomOverrideStore) Set(slug string, roomID int, name string, baseNames map[string]string) error {
	if s == nil {
		return fmt.Errorf("Loxone room override store unavailable")
	}
	name = strings.TrimSpace(name)
	if roomID <= 0 {
		return fmt.Errorf("invalid room ID")
	}
	if name == "" {
		return fmt.Errorf("room name is required")
	}
	if len([]rune(name)) > 80 {
		return fmt.Errorf("room name must not exceed 80 characters")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	candidate := make(map[string]string, len(s.devices[slug])+1)
	for id, existing := range s.devices[slug] {
		candidate[id] = existing
	}
	candidate[strconv.Itoa(roomID)] = name
	if err := validateUniqueLoxoneRoomNames(baseNames, candidate); err != nil {
		return err
	}
	if s.devices[slug] == nil {
		s.devices[slug] = make(map[string]string)
	}
	id := strconv.Itoa(roomID)
	previous, existed := s.devices[slug][id]
	s.devices[slug][id] = name
	if err := s.saveLocked(); err != nil {
		if existed {
			s.devices[slug][id] = previous
		} else {
			delete(s.devices[slug], id)
		}
		return err
	}
	return nil
}

func (s *LoxoneRoomOverrideStore) Delete(slug string, roomID int) error {
	if s == nil {
		return fmt.Errorf("Loxone room override store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rooms := s.devices[slug]
	id := strconv.Itoa(roomID)
	previous, existed := "", false
	if rooms != nil {
		previous, existed = rooms[id]
		delete(rooms, id)
		if len(rooms) == 0 {
			delete(s.devices, slug)
		}
	}
	if err := s.saveLocked(); err != nil {
		if existed {
			if s.devices[slug] == nil {
				s.devices[slug] = make(map[string]string)
			}
			s.devices[slug][id] = previous
		}
		return err
	}
	return nil
}

func (s *LoxoneRoomOverrideStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create override directory: %w", err)
	}
	data, err := json.MarshalIndent(s.devices, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal room overrides: %w", err)
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("write room overrides: %w", err)
	}
	if err := os.Rename(temporary, s.path); err != nil {
		return fmt.Errorf("replace room overrides: %w", err)
	}
	return nil
}

func validateUniqueLoxoneRoomNames(baseNames, overrides map[string]string) error {
	ids := make([]string, 0, len(baseNames))
	for id := range baseNames {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	seen := make(map[string]string)
	for _, id := range ids {
		name := strings.TrimSpace(baseNames[id])
		if override := strings.TrimSpace(overrides[id]); override != "" {
			name = override
		}
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if previous, exists := seen[key]; exists && previous != id {
			return fmt.Errorf("room name %q is already used by room %s", name, previous)
		}
		seen[key] = id
	}
	return nil
}

// LoxoneDiagnostics is a read-only snapshot used by the integration UI.
type LoxoneDiagnostics struct {
	LastActivity *LoxoneActivity  `json:"last_activity,omitempty"`
	LastCommand  *LoxoneActivity  `json:"last_command,omitempty"`
	Recent       []LoxoneActivity `json:"recent,omitempty"`
}

// LoxoneDiagnosticStore keeps a bounded in-memory activity history. The
// retained last_command MQTT topic remains the persistent source of truth.
type LoxoneDiagnosticStore struct {
	mu      sync.RWMutex
	limit   int
	devices map[string]LoxoneDiagnostics
}

func NewLoxoneDiagnosticStore(limit int) *LoxoneDiagnosticStore {
	if limit <= 0 {
		limit = 50
	}
	return &LoxoneDiagnosticStore{limit: limit, devices: make(map[string]LoxoneDiagnostics)}
}

func (s *LoxoneDiagnosticStore) Record(slug string, activity LoxoneActivity) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	diagnostic := s.devices[slug]
	copyActivity := activity
	diagnostic.LastActivity = &copyActivity
	if activity.Type == "command" {
		diagnostic.LastCommand = &copyActivity
	}
	diagnostic.Recent = append(diagnostic.Recent, activity)
	if len(diagnostic.Recent) > s.limit {
		diagnostic.Recent = append([]LoxoneActivity(nil), diagnostic.Recent[len(diagnostic.Recent)-s.limit:]...)
	}
	s.devices[slug] = diagnostic
}

func (s *LoxoneDiagnosticStore) Snapshot(slug string) LoxoneDiagnostics {
	if s == nil {
		return LoxoneDiagnostics{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	diagnostic := s.devices[slug]
	result := LoxoneDiagnostics{Recent: append([]LoxoneActivity(nil), diagnostic.Recent...)}
	if diagnostic.LastActivity != nil {
		copyActivity := *diagnostic.LastActivity
		result.LastActivity = &copyActivity
	}
	if diagnostic.LastCommand != nil {
		copyCommand := *diagnostic.LastCommand
		result.LastCommand = &copyCommand
	}
	return result
}

// FindCommand returns the newest lifecycle state recorded for a command ID.
func (s *LoxoneDiagnosticStore) FindCommand(id string) (string, LoxoneActivity, bool) {
	if s == nil || strings.TrimSpace(id) == "" {
		return "", LoxoneActivity{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for slug, diagnostic := range s.devices {
		for index := len(diagnostic.Recent) - 1; index >= 0; index-- {
			activity := diagnostic.Recent[index]
			if activity.Type == "command" && activity.ID == id {
				return slug, activity, true
			}
		}
	}
	return "", LoxoneActivity{}, false
}

// RestoreLastCommand seeds diagnostics from the retained MQTT topic without
// adding a duplicate entry to the current-session activity history.
func (s *LoxoneDiagnosticStore) RestoreLastCommand(slug string, activity LoxoneActivity) {
	if s == nil || activity.Type != "command" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	diagnostic := s.devices[slug]
	copyActivity := activity
	diagnostic.LastCommand = &copyActivity
	s.devices[slug] = diagnostic
}
