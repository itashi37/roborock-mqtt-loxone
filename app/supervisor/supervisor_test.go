package supervisor

import "testing"

func TestRuntimePublishesOnlyOneRestartRequest(t *testing.T) {
	runtime := NewRuntime("synology", "/volume1/data")
	runtime.RequestRestart("stuck")
	runtime.RequestRestart("duplicate")
	if reason := <-runtime.RestartRequests(); reason != "stuck" {
		t.Fatalf("reason=%q", reason)
	}
	select {
	case reason := <-runtime.RestartRequests():
		t.Fatalf("unexpected duplicate restart: %s", reason)
	default:
	}
	if status := runtime.Status(); status.Kind != "synology" || status.DataDir != "/volume1/data" {
		t.Fatalf("status=%+v", status)
	}
}
