package health

import (
	"sync"
	"testing"
	"time"
)

func TestPublisherEmitsPeriodicHeartbeatAndCurrentHealth(t *testing.T) {
	var mu sync.Mutex
	cloud := true
	robot := true
	updates := make(chan Snapshot, 8)
	publisher := NewPublisher(10*time.Millisecond, func(time.Time) Snapshot {
		mu.Lock()
		defer mu.Unlock()
		return Snapshot{BridgeAlive: true, CloudConnected: cloud, RobotOnline: map[string]bool{"robot": robot}}
	}, func(snapshot Snapshot) { updates <- snapshot })
	publisher.Start()
	defer publisher.Stop()

	first := waitSnapshot(t, updates)
	second := waitSnapshot(t, updates)
	if first.Heartbeat == 0 || second.Heartbeat == 0 || !second.BridgeAlive || !second.CloudConnected || !second.RobotOnline["robot"] {
		t.Fatalf("unexpected initial snapshots: first=%+v second=%+v", first, second)
	}

	mu.Lock()
	cloud, robot = false, false
	mu.Unlock()
	publisher.PublishNow()
	lost := waitSnapshot(t, updates)
	if lost.CloudConnected || lost.RobotOnline["robot"] {
		t.Fatalf("lost cloud/robot state not emitted: %+v", lost)
	}

	mu.Lock()
	cloud, robot = true, true
	mu.Unlock()
	publisher.PublishNow()
	reconnected := waitSnapshot(t, updates)
	if !reconnected.CloudConnected || !reconnected.RobotOnline["robot"] {
		t.Fatalf("reconnection state not emitted: %+v", reconnected)
	}
}

func waitSnapshot(t *testing.T, updates <-chan Snapshot) Snapshot {
	t.Helper()
	select {
	case snapshot := <-updates:
		return snapshot
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for health snapshot")
		return Snapshot{}
	}
}
