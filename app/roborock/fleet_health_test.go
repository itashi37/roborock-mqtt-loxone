package roborock

import (
	"testing"
	"time"
)

func TestFleetHealthBackoffAndSuccessfulRecovery(t *testing.T) {
	store := NewFleetHealthStore([]string{"alpha", "beta"})
	now := time.Unix(1_700_000_000, 0)
	first := store.MarkFailure("alpha", "status poll failed", now)
	if first.ConsecutiveFailures != 1 || first.BackoffSeconds != 30 || !first.NextPollAt.Equal(now.Add(30*time.Second)) {
		t.Fatalf("unexpected first failure: %+v", first)
	}
	if store.ShouldPoll("alpha", now.Add(29*time.Second)) {
		t.Fatal("poll should be suppressed during backoff")
	}
	second := store.MarkFailure("alpha", "status poll failed", now.Add(30*time.Second))
	if second.BackoffSeconds != 60 {
		t.Fatalf("second backoff = %d", second.BackoffSeconds)
	}
	recovered := store.MarkSuccess("alpha", &PublishedStatus{State: "charging"}, 125*time.Millisecond, now.Add(90*time.Second))
	if recovered.ConsecutiveFailures != 0 || recovered.BackoffSeconds != 0 || recovered.StatusLatencyMS != 125 || recovered.DockState != "docked" {
		t.Fatalf("unexpected recovery: %+v", recovered)
	}
}

func TestFleetGlobalHealthAndDockPriority(t *testing.T) {
	store := NewFleetHealthStore([]string{"alpha", "beta"})
	now := time.Now()
	dockType, dockError, active := 3, 2, 1
	store.MarkSuccess("alpha", &PublishedStatus{DockType: &dockType, DryStatus: &active}, time.Millisecond, now)
	if got := store.Snapshot(now).Health; got != "degraded" {
		t.Fatalf("one of two online should be degraded, got %s", got)
	}
	store.MarkSuccess("beta", &PublishedStatus{DockType: &dockType}, time.Millisecond, now)
	if got := store.Snapshot(now).Health; got != "healthy" {
		t.Fatalf("all online should be healthy, got %s", got)
	}
	if got := DockState(&PublishedStatus{DockType: &dockType, DockErrorStatus: &dockError, DryStatus: &active}); got != "error" {
		t.Fatalf("dock error must take priority, got %s", got)
	}
}

func TestDeviceHealthIsPartOfInternalState(t *testing.T) {
	store := NewDeviceStateStore()
	store.UpdateHealth("alpha", DeviceHealth{Slug: "alpha", Online: true, StatusLatencyMS: 42}, time.Unix(5, 0))
	state, ok := store.Get("alpha")
	if !ok || !state.Health.Online || state.Health.StatusLatencyMS != 42 {
		t.Fatalf("health not stored: %+v", state)
	}
}
