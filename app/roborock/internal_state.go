package roborock

import (
	"sync"
	"time"
)

type DeviceStateChange string

const (
	DeviceStateSeed         DeviceStateChange = "seed"
	DeviceStateStatus       DeviceStateChange = "status"
	DeviceStateAvailability DeviceStateChange = "availability"
	DeviceStateRoom         DeviceStateChange = "current_room"
	DeviceStateInventory    DeviceStateChange = "inventory"
	DeviceStateHealth       DeviceStateChange = "health"
)

// InternalDeviceState is the transport-independent source of truth consumed by
// MQTT today and by Direct Loxone in the following phase.
type InternalDeviceState struct {
	DeviceID     string             `json:"device_id"`
	Slug         string             `json:"slug"`
	Name         string             `json:"name"`
	Model        string             `json:"model"`
	Online       bool               `json:"online"`
	Status       *PublishedStatus   `json:"status,omitempty"`
	CurrentRoom  *CurrentRoom       `json:"current_room,omitempty"`
	RoomMappings []RoomMapping      `json:"room_mappings,omitempty"`
	Scenes       []Scene            `json:"scenes,omitempty"`
	Capabilities DeviceCapabilities `json:"capabilities"`
	Health       DeviceHealth       `json:"health"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

func (s *DeviceStateStore) UpdateHealth(slug string, health DeviceHealth, now time.Time) {
	s.update(slug, DeviceStateHealth, func(state *InternalDeviceState) {
		state.Health = health
		state.UpdatedAt = now
	})
}

type DeviceStateUpdate struct {
	Change DeviceStateChange
	State  InternalDeviceState
}

type DeviceStateStore struct {
	mu          sync.RWMutex
	devices     map[string]InternalDeviceState
	subscribers []func(DeviceStateUpdate)
}

func NewDeviceStateStore() *DeviceStateStore {
	return &DeviceStateStore{devices: make(map[string]InternalDeviceState)}
}

func (s *DeviceStateStore) Subscribe(callback func(DeviceStateUpdate)) {
	if callback == nil {
		return
	}
	s.mu.Lock()
	s.subscribers = append(s.subscribers, callback)
	s.mu.Unlock()
}

func (s *DeviceStateStore) Seed(device *ManagedDevice, capabilities DeviceCapabilities, now time.Time) {
	if device == nil {
		return
	}
	s.update(device.Slug, DeviceStateSeed, func(state *InternalDeviceState) {
		state.DeviceID = device.Info.DID
		state.Slug = device.Slug
		state.Name = device.Info.Name
		state.Model = device.Info.Model
		state.Scenes = append([]Scene(nil), device.Scenes...)
		state.RoomMappings = device.GetRoomMappings()
		state.Capabilities = capabilities
		state.UpdatedAt = now
	})
}

func (s *DeviceStateStore) UpdateStatus(slug string, status *PublishedStatus, capabilities DeviceCapabilities, now time.Time) {
	s.update(slug, DeviceStateStatus, func(state *InternalDeviceState) {
		state.Status = clonePublishedStatus(status)
		state.Capabilities = capabilities
		state.UpdatedAt = now
	})
}

func (s *DeviceStateStore) UpdateAvailability(slug string, online bool, now time.Time) {
	s.update(slug, DeviceStateAvailability, func(state *InternalDeviceState) {
		state.Online = online
		state.UpdatedAt = now
	})
}

func (s *DeviceStateStore) UpdateCurrentRoom(slug string, room *CurrentRoom, now time.Time) {
	s.update(slug, DeviceStateRoom, func(state *InternalDeviceState) {
		if room == nil {
			state.CurrentRoom = nil
		} else {
			copy := *room
			state.CurrentRoom = &copy
		}
		state.UpdatedAt = now
	})
}

func (s *DeviceStateStore) UpdateInventory(slug string, mappings []RoomMapping, scenes []Scene, capabilities DeviceCapabilities, now time.Time) {
	s.update(slug, DeviceStateInventory, func(state *InternalDeviceState) {
		state.RoomMappings = append([]RoomMapping(nil), mappings...)
		state.Scenes = append([]Scene(nil), scenes...)
		state.Capabilities = capabilities
		state.UpdatedAt = now
	})
}

func (s *DeviceStateStore) Get(slug string) (InternalDeviceState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.devices[slug]
	return cloneInternalDeviceState(state), ok
}

func (s *DeviceStateStore) All() []InternalDeviceState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]InternalDeviceState, 0, len(s.devices))
	for _, state := range s.devices {
		result = append(result, cloneInternalDeviceState(state))
	}
	return result
}

func (s *DeviceStateStore) update(slug string, change DeviceStateChange, mutate func(*InternalDeviceState)) {
	s.mu.Lock()
	state := s.devices[slug]
	state.Slug = slug
	mutate(&state)
	s.devices[slug] = state
	update := DeviceStateUpdate{Change: change, State: cloneInternalDeviceState(state)}
	subscribers := append([]func(DeviceStateUpdate){}, s.subscribers...)
	s.mu.Unlock()
	for _, subscriber := range subscribers {
		subscriber(update)
	}
}

func clonePublishedStatus(status *PublishedStatus) *PublishedStatus {
	if status == nil {
		return nil
	}
	copy := *status
	return &copy
}

func cloneInternalDeviceState(state InternalDeviceState) InternalDeviceState {
	state.Status = clonePublishedStatus(state.Status)
	if state.CurrentRoom != nil {
		room := *state.CurrentRoom
		state.CurrentRoom = &room
	}
	state.RoomMappings = append([]RoomMapping(nil), state.RoomMappings...)
	state.Scenes = append([]Scene(nil), state.Scenes...)
	return state
}
