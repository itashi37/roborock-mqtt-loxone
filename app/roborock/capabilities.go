package roborock

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type CapabilityConfidence string

const (
	CapabilityConfirmed CapabilityConfidence = "confirmed"
	CapabilityReported  CapabilityConfidence = "reported"
	CapabilityObserved  CapabilityConfidence = "observed"
	CapabilityUnknown   CapabilityConfidence = "unknown"
)

// Capability uses a nullable boolean deliberately: true and false are facts;
// null means the bridge has insufficient evidence and must not guess.
type Capability struct {
	Supported   *bool                `json:"supported"`
	Source      string               `json:"source"`
	Confidence  CapabilityConfidence `json:"confidence"`
	Values      []string             `json:"values,omitempty"`
	LastChecked time.Time            `json:"last_checked"`
	Reason      string               `json:"reason,omitempty"`
}

type DeviceCapabilities struct {
	Rooms     Capability `json:"rooms"`
	Scenes    Capability `json:"scenes"`
	Fan       Capability `json:"fan"`
	Mop       Capability `json:"mop"`
	Water     Capability `json:"water"`
	Locate    Capability `json:"locate"`
	Dock      Capability `json:"dock"`
	DockEmpty Capability `json:"dock_empty"`
	MopWash   Capability `json:"mop_wash"`
	MopDry    Capability `json:"mop_dry"`
}

func unknownCapability(now time.Time, reason string) Capability {
	return Capability{Supported: nil, Source: "not_detected", Confidence: CapabilityUnknown, LastChecked: now, Reason: reason}
}

func observedCapability(now time.Time, source string, values ...string) Capability {
	supported := true
	return Capability{Supported: &supported, Source: source, Confidence: CapabilityObserved, Values: values, LastChecked: now}
}

func InitialDeviceCapabilities(now time.Time) DeviceCapabilities {
	reason := "the robot has not exposed reliable capability data yet"
	return DeviceCapabilities{
		Rooms: unknownCapability(now, reason), Scenes: unknownCapability(now, reason),
		Fan: unknownCapability(now, reason), Mop: unknownCapability(now, reason), Water: unknownCapability(now, reason),
		Locate: unknownCapability(now, reason), Dock: unknownCapability(now, reason),
		DockEmpty: unknownCapability(now, reason), MopWash: unknownCapability(now, reason), MopDry: unknownCapability(now, reason),
	}
}

// CapabilityStore incrementally records only evidence already parsed by the
// Go backend. Advanced commands remain unknown until feature flags/dock status
// are implemented and verified in later phases.
type CapabilityStore struct {
	mu      sync.RWMutex
	devices map[string]DeviceCapabilities
}

func NewCapabilityStore() *CapabilityStore {
	return &CapabilityStore{devices: make(map[string]DeviceCapabilities)}
}

func (s *CapabilityStore) Ensure(slug string, now time.Time) DeviceCapabilities {
	s.mu.Lock()
	defer s.mu.Unlock()
	capabilities, ok := s.devices[slug]
	if !ok {
		capabilities = InitialDeviceCapabilities(now)
		s.devices[slug] = capabilities
	}
	return capabilities
}

func (s *CapabilityStore) UpdateInventory(slug string, mappings []RoomMapping, roomsKnown bool, scenes []Scene, scenesKnown bool, now time.Time) DeviceCapabilities {
	s.mu.Lock()
	defer s.mu.Unlock()
	capabilities, ok := s.devices[slug]
	if !ok {
		capabilities = InitialDeviceCapabilities(now)
	}
	if roomsKnown {
		capabilities.Rooms = observedCapability(now, "get_room_mapping")
		if len(mappings) == 0 {
			supported := false
			capabilities.Rooms.Supported = &supported
			capabilities.Rooms.Reason = "get_room_mapping returned no active segments"
		}
	}
	if scenesKnown {
		capabilities.Scenes = observedCapability(now, "roborock_scenes_api")
		if len(scenes) == 0 {
			supported := false
			capabilities.Scenes.Supported = &supported
			capabilities.Scenes.Reason = "the scenes API returned no scenes"
		}
	}
	s.devices[slug] = capabilities
	return capabilities
}

func (s *CapabilityStore) ObserveStatus(slug string, status *PublishedStatus, now time.Time) DeviceCapabilities {
	s.mu.Lock()
	defer s.mu.Unlock()
	capabilities, ok := s.devices[slug]
	if !ok {
		capabilities = InitialDeviceCapabilities(now)
	}
	if status != nil {
		if value := strings.TrimSpace(status.FanSpeed); value != "" && value != "unknown" {
			capabilities.Fan = mergeObservedValue(capabilities.Fan, now, "get_status.fan_power", value)
		}
		if value := strings.TrimSpace(status.MopMode); value != "" && value != "unknown" {
			capabilities.Mop = mergeObservedValue(capabilities.Mop, now, "get_status.mop_mode", value)
		}
		if value := strings.TrimSpace(status.WaterBox); value != "" && value != "unknown" {
			capabilities.Water = mergeObservedValue(capabilities.Water, now, "get_status.water_box_custom_mode", value)
		}
		state := NormalizeLoxoneState(status.State)
		if state == "charging" || state == "returning" || state == "docked" {
			capabilities.Dock = observedCapability(now, "get_status.state", state)
		}
	}
	s.devices[slug] = capabilities
	return capabilities
}

func mergeObservedValue(current Capability, now time.Time, source, value string) Capability {
	values := append([]string(nil), current.Values...)
	found := false
	for _, existing := range values {
		if existing == value {
			found = true
			break
		}
	}
	if !found {
		values = append(values, value)
		sort.Strings(values)
	}
	return observedCapability(now, source, values...)
}

func (s *CapabilityStore) Get(slug string) DeviceCapabilities {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.devices[slug]
}
