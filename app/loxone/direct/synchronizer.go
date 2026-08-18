package direct

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mqtt-home/roborock-mqtt/roborock"
)

type InputDiagnostic struct {
	Robot              string    `json:"robot"`
	Field              string    `json:"field"`
	Input              string    `json:"input"`
	Kind               ValueKind `json:"kind"`
	LastValue          string    `json:"last_value,omitempty"`
	LastAttempt        time.Time `json:"last_attempt,omitempty"`
	LastSuccess        time.Time `json:"last_success,omitempty"`
	LastError          string    `json:"last_error,omitempty"`
	ConsecutiveRetries int       `json:"consecutive_retries"`
}

type SyncDiagnostics struct {
	LastTransmission time.Time         `json:"last_transmission,omitempty"`
	LastError        string            `json:"last_error,omitempty"`
	Inputs           []InputDiagnostic `json:"inputs"`
}

type queuedValue struct {
	value    StateValue
	attempts int
	force    bool
}

type Synchronizer struct {
	client     ValuePusher
	mapping    InputMapping
	maxRetries int
	retryDelay time.Duration
	stateAll   func() []roborock.InternalDeviceState

	mu           sync.Mutex
	pending      map[string]queuedValue
	lastSuccess  map[string]string
	diagnostics  map[string]InputDiagnostic
	disconnected bool
	wake         chan struct{}
	stop         chan struct{}
	done         chan struct{}
}

func NewSynchronizer(client ValuePusher, mapping InputMapping, maxRetries int, retryDelay time.Duration, stateAll func() []roborock.InternalDeviceState) *Synchronizer {
	if maxRetries < 0 {
		maxRetries = 0
	}
	if retryDelay <= 0 {
		retryDelay = 500 * time.Millisecond
	}
	synchronizer := &Synchronizer{
		client: client, mapping: mapping, maxRetries: maxRetries, retryDelay: retryDelay, stateAll: stateAll,
		pending: make(map[string]queuedValue), lastSuccess: make(map[string]string), diagnostics: make(map[string]InputDiagnostic),
		wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}),
	}
	go synchronizer.run()
	return synchronizer
}

func (s *Synchronizer) Update(state roborock.InternalDeviceState) {
	s.enqueueState(state, false)
}

func (s *Synchronizer) ResendAll() {
	if s.stateAll == nil {
		return
	}
	for _, state := range s.stateAll() {
		s.enqueueState(state, true)
	}
}

func (s *Synchronizer) Close() {
	select {
	case <-s.stop:
		return
	default:
		close(s.stop)
		<-s.done
	}
}

func (s *Synchronizer) Diagnostics() SyncDiagnostics {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := SyncDiagnostics{Inputs: make([]InputDiagnostic, 0, len(s.diagnostics))}
	for _, diagnostic := range s.diagnostics {
		result.Inputs = append(result.Inputs, diagnostic)
		if diagnostic.LastSuccess.After(result.LastTransmission) {
			result.LastTransmission = diagnostic.LastSuccess
		}
		if diagnostic.LastError != "" {
			result.LastError = diagnostic.LastError
		}
	}
	sort.Slice(result.Inputs, func(i, j int) bool {
		if result.Inputs[i].Robot == result.Inputs[j].Robot {
			return result.Inputs[i].Field < result.Inputs[j].Field
		}
		return result.Inputs[i].Robot < result.Inputs[j].Robot
	})
	return result
}

func (s *Synchronizer) enqueueState(state roborock.InternalDeviceState, force bool) {
	for _, value := range ValuesForState(state, s.mapping) {
		s.enqueue(queuedValue{value: value, force: force})
	}
}

func (s *Synchronizer) enqueue(item queuedValue) {
	key := item.value.Robot + "\x00" + item.value.Field
	s.mu.Lock()
	if previous, ok := s.lastSuccess[key]; ok && !item.force && previous == item.value.Value {
		s.mu.Unlock()
		return
	}
	if current, ok := s.pending[key]; ok {
		if item.attempts > 0 && current.value.Value != item.value.Value {
			s.mu.Unlock()
			return // a newer observation wins over this retry
		}
		if current.value.Value == item.value.Value && !item.force {
			s.mu.Unlock()
			return
		}
	}
	s.pending[key] = item
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Synchronizer) run() {
	defer close(s.done)
	for {
		select {
		case <-s.wake:
			for s.processNext() {
			}
		case <-s.stop:
			return
		}
	}
}

func (s *Synchronizer) processNext() bool {
	s.mu.Lock()
	keys := make([]string, 0, len(s.pending))
	for key := range s.pending {
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		s.mu.Unlock()
		return false
	}
	sort.Strings(keys)
	key := keys[0]
	item := s.pending[key]
	delete(s.pending, key)
	diagnostic := s.diagnostics[key]
	diagnostic.Robot, diagnostic.Field, diagnostic.Input, diagnostic.Kind = item.value.Robot, item.value.Field, item.value.Input, item.value.Kind
	diagnostic.LastAttempt = time.Now()
	s.diagnostics[key] = diagnostic
	s.mu.Unlock()

	err := s.client.Push(context.Background(), item.value.Input, item.value.Value)
	now := time.Now()
	s.mu.Lock()
	diagnostic = s.diagnostics[key]
	if err == nil {
		wasDisconnected := s.disconnected
		s.disconnected = false
		s.lastSuccess[key] = item.value.Value
		diagnostic.LastValue = item.value.Value
		diagnostic.LastSuccess = now
		diagnostic.LastError = ""
		diagnostic.ConsecutiveRetries = 0
		s.diagnostics[key] = diagnostic
		s.mu.Unlock()
		if wasDisconnected {
			go s.ResendAll()
		}
		return true
	}
	s.disconnected = true
	diagnostic.LastError = err.Error()
	diagnostic.ConsecutiveRetries = item.attempts + 1
	s.diagnostics[key] = diagnostic
	s.mu.Unlock()
	if item.attempts < s.maxRetries {
		item.attempts++
		delay := s.retryDelay * time.Duration(1<<(item.attempts-1))
		time.AfterFunc(delay, func() { s.enqueue(item) })
	}
	return true
}

func (s *Synchronizer) Test(ctx context.Context, input string) error {
	if input == "" {
		return fmt.Errorf("test Virtual Input is required")
	}
	return s.client.Push(ctx, input, "0")
}
