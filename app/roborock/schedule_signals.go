package roborock

import (
	"strings"
	"sync"

	"github.com/philipparndt/go-logger"
)

// SignalListener subscribes to MQTT topics for public holiday and vacation signals.
type SignalListener struct {
	holidayTopic  string
	vacationTopic string
	holiday       bool
	vacation      bool
	mu            sync.RWMutex
	onChange      func()
	subscribe     func(string, func(string, []byte)) error
}

// NewSignalListener creates a listener for the given MQTT signal topics.
func NewSignalListener(holidayTopic, vacationTopic string, subscribe ...func(string, func(string, []byte)) error) *SignalListener {
	var subscriber func(string, func(string, []byte)) error
	if len(subscribe) > 0 {
		subscriber = subscribe[0]
	}
	return &SignalListener{
		holidayTopic:  holidayTopic,
		vacationTopic: vacationTopic,
		subscribe:     subscriber,
	}
}

// SetOnChange sets a callback invoked when any signal value changes.
func (sl *SignalListener) SetOnChange(cb func()) {
	sl.onChange = cb
}

// Subscribe starts listening on the configured MQTT topics.
func (sl *SignalListener) Subscribe() {
	if sl.subscribe == nil {
		logger.Warn("Schedule signal subscription unavailable")
		return
	}
	logger.Info("Subscribing to schedule signals", "holiday", sl.holidayTopic, "vacation", sl.vacationTopic)

	_ = sl.subscribe(sl.holidayTopic, func(topic string, payload []byte) {
		val := parseBool(string(payload))
		sl.mu.Lock()
		changed := sl.holiday != val
		sl.holiday = val
		sl.mu.Unlock()
		logger.Debug("Holiday signal", "topic", topic, "value", val)
		if changed && sl.onChange != nil {
			sl.onChange()
		}
	})

	_ = sl.subscribe(sl.vacationTopic, func(topic string, payload []byte) {
		val := parseBool(string(payload))
		sl.mu.Lock()
		changed := sl.vacation != val
		sl.vacation = val
		sl.mu.Unlock()
		logger.Debug("Vacation signal", "topic", topic, "value", val)
		if changed && sl.onChange != nil {
			sl.onChange()
		}
	})
}

// IsHoliday returns the current public holiday signal value.
func (sl *SignalListener) IsHoliday() bool {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.holiday
}

// IsVacation returns the current vacation signal value.
func (sl *SignalListener) IsVacation() bool {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.vacation
}

func parseBool(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "true"
}
