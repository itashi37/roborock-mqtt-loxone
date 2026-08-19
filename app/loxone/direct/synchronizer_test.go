package direct

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mqtt-home/roborock-mqtt/roborock"
)

type recordingPusher struct {
	mu       sync.Mutex
	calls    []StateValue
	failures int
}

func (p *recordingPusher) Push(_ context.Context, input, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, StateValue{Input: input, Value: value})
	if p.failures > 0 {
		p.failures--
		return errors.New("connection unavailable")
	}
	return nil
}

func (p *recordingPusher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func waitForCalls(t *testing.T, p *recordingPusher, count int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for p.count() < count && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := p.count(); got < count {
		t.Fatalf("got %d pushes, want at least %d", got, count)
	}
}

func TestSynchronizerSendsChangesOnlyAndSupportsFullResync(t *testing.T) {
	state := roborock.InternalDeviceState{Slug: "robot", Online: true, UpdatedAt: time.Unix(10, 0), Status: &roborock.PublishedStatus{State: "idle", Battery: 80}}
	states := []roborock.InternalDeviceState{state}
	pusher := &recordingPusher{}
	synchronizer := NewSynchronizer(pusher, InputMapping{Prefix: "RR"}, 0, time.Millisecond, func() []roborock.InternalDeviceState { return states })
	defer synchronizer.Close()

	synchronizer.Update(state)
	waitForCalls(t, pusher, 16)
	initial := pusher.count()
	synchronizer.Update(state)
	time.Sleep(30 * time.Millisecond)
	if got := pusher.count(); got != initial {
		t.Fatalf("unchanged state was resent: %d -> %d", initial, got)
	}

	state.Status.Battery = 79
	state.UpdatedAt = time.Unix(11, 0)
	states[0] = state
	synchronizer.Update(state)
	waitForCalls(t, pusher, initial+2) // battery and last_seen
	changed := pusher.count()
	synchronizer.ResendAll()
	waitForCalls(t, pusher, changed+16)
}

func TestSynchronizerResendsInstallationHealth(t *testing.T) {
	pusher := &recordingPusher{}
	synchronizer := NewSynchronizer(pusher, InputMapping{Prefix: "RR"}, 0, time.Millisecond, nil)
	defer synchronizer.Close()
	synchronizer.SetInstallationValues(func() []StateValue {
		return []StateValue{InstallationValue("bridge_alive", Digital, "1", InputMapping{Prefix: "RR"})}
	})
	synchronizer.ResendAll()
	waitForCalls(t, pusher, 1)
}

func TestSynchronizerRetriesAndRecordsDiagnostics(t *testing.T) {
	pusher := &recordingPusher{failures: 1}
	synchronizer := NewSynchronizer(pusher, InputMapping{Prefix: "RR"}, 2, time.Millisecond, nil)
	defer synchronizer.Close()
	synchronizer.Update(roborock.InternalDeviceState{Slug: "robot", UpdatedAt: time.Unix(1, 0)})
	waitForCalls(t, pusher, 17)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		diagnostics := synchronizer.Diagnostics()
		if !diagnostics.LastTransmission.IsZero() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("successful retry was not recorded")
}
