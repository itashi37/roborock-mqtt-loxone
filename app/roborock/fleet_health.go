package roborock

import (
	"sort"
	"sync"
	"time"
)

type DeviceHealth struct {
	Slug                string    `json:"slug"`
	Online              bool      `json:"online"`
	InError             bool      `json:"in_error"`
	ErrorCode           int       `json:"error_code"`
	DockState           string    `json:"dock_state"`
	LastPollAttempt     time.Time `json:"last_poll_attempt,omitempty"`
	LastPollSuccess     time.Time `json:"last_poll_success,omitempty"`
	LastCommunication   time.Time `json:"last_communication,omitempty"`
	StatusLatencyMS     int64     `json:"status_latency_ms"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	BackoffSeconds      int       `json:"backoff_seconds"`
	NextPollAt          time.Time `json:"next_poll_at,omitempty"`
	LastError           string    `json:"last_error,omitempty"`
}

type FleetHealth struct {
	Health       string         `json:"health"`
	UpdatedAt    time.Time      `json:"updated_at"`
	Robots       int            `json:"robots"`
	Online       int            `json:"online"`
	InError      int            `json:"in_error"`
	PollFailures int            `json:"poll_failures"`
	Devices      []DeviceHealth `json:"devices"`
}

type FleetHealthStore struct {
	mu      sync.RWMutex
	devices map[string]DeviceHealth
}

func NewFleetHealthStore(slugs []string) *FleetHealthStore {
	store := &FleetHealthStore{devices: make(map[string]DeviceHealth)}
	for _, slug := range slugs {
		store.devices[slug] = DeviceHealth{Slug: slug, DockState: "unknown"}
	}
	return store
}

func (s *FleetHealthStore) ShouldPoll(slug string, now time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	health := s.devices[slug]
	return health.NextPollAt.IsZero() || !now.Before(health.NextPollAt)
}

func (s *FleetHealthStore) MarkAttempt(slug string, now time.Time) DeviceHealth {
	return s.update(slug, func(health *DeviceHealth) { health.LastPollAttempt = now })
}

func (s *FleetHealthStore) MarkSuccess(slug string, status *PublishedStatus, latency time.Duration, now time.Time) DeviceHealth {
	return s.update(slug, func(health *DeviceHealth) {
		health.Online = true
		health.LastPollSuccess = now
		health.LastCommunication = now
		health.StatusLatencyMS = latency.Milliseconds()
		health.ConsecutiveFailures = 0
		health.BackoffSeconds = 0
		health.NextPollAt = time.Time{}
		health.LastError = ""
		if status != nil {
			health.ErrorCode = status.ErrorCode
			health.InError = status.ErrorCode != 0 || NormalizeLoxoneState(status.State) == "error"
			health.DockState = DockState(status)
		}
	})
}

func (s *FleetHealthStore) MarkFailure(slug, message string, now time.Time) DeviceHealth {
	return s.update(slug, func(health *DeviceHealth) {
		health.ConsecutiveFailures++
		backoff := 30 * (1 << minInt(health.ConsecutiveFailures-1, 5))
		if backoff > 900 {
			backoff = 900
		}
		health.BackoffSeconds = backoff
		health.NextPollAt = now.Add(time.Duration(backoff) * time.Second)
		health.LastError = message
	})
}

func (s *FleetHealthStore) MarkOnline(slug string, online bool, now time.Time) DeviceHealth {
	return s.update(slug, func(health *DeviceHealth) {
		health.Online = online
		if online {
			health.LastCommunication = now
		}
	})
}

func (s *FleetHealthStore) MarkCommunication(slug string, status *PublishedStatus, now time.Time) DeviceHealth {
	return s.update(slug, func(health *DeviceHealth) {
		health.Online = true
		health.LastCommunication = now
		if status != nil {
			health.ErrorCode = status.ErrorCode
			health.InError = status.ErrorCode != 0 || NormalizeLoxoneState(status.State) == "error"
			health.DockState = DockState(status)
		}
	})
}

func (s *FleetHealthStore) update(slug string, mutate func(*DeviceHealth)) DeviceHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	health := s.devices[slug]
	health.Slug = slug
	mutate(&health)
	s.devices[slug] = health
	return health
}

func (s *FleetHealthStore) Snapshot(now time.Time) FleetHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := FleetHealth{UpdatedAt: now, Robots: len(s.devices), Devices: make([]DeviceHealth, 0, len(s.devices))}
	for _, health := range s.devices {
		result.Devices = append(result.Devices, health)
		if health.Online {
			result.Online++
		}
		if health.InError {
			result.InError++
		}
		if health.ConsecutiveFailures > 0 {
			result.PollFailures++
		}
	}
	sort.Slice(result.Devices, func(i, j int) bool { return result.Devices[i].Slug < result.Devices[j].Slug })
	switch {
	case result.Robots == 0 || result.Online == 0:
		result.Health = "offline"
	case result.Online < result.Robots || result.InError > 0 || result.PollFailures > 0:
		result.Health = "degraded"
	default:
		result.Health = "healthy"
	}
	return result
}

func (s *FleetHealthStore) Get(slug string) DeviceHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.devices[slug]
}

func DockState(status *PublishedStatus) string {
	if status == nil {
		return "unknown"
	}
	if status.DockErrorStatus != nil && *status.DockErrorStatus != 0 {
		return "error"
	}
	if status.DustCollectionStatus != nil && *status.DustCollectionStatus > 0 {
		return "emptying"
	}
	if status.WashStatus != nil && *status.WashStatus > 0 {
		return "washing"
	}
	if status.DryStatus != nil && *status.DryStatus > 0 {
		return "drying"
	}
	state := NormalizeLoxoneState(status.State)
	if state == "charging" || state == "docked" {
		return "docked"
	}
	if status.DockType != nil && *status.DockType > 0 {
		return "available"
	}
	return "unknown"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
