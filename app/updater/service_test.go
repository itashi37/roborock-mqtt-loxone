package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeEngine struct {
	mu            sync.Mutex
	pullError     error
	replaceError  error
	healthError   error
	versionError  error
	rollbackError error
	rolledBack    int
	finalized     int
}

func (f *fakeEngine) CurrentImage(context.Context, string) (string, error) {
	return AllowedImage + ":v1.0.0", nil
}
func (f *fakeEngine) Pull(context.Context, string) error { return f.pullError }
func (f *fakeEngine) Replace(context.Context, string, string) (Replacement, error) {
	return Replacement{ContainerName: "bridge", OldID: "old", NewID: "new", PreviousImage: AllowedImage + ":v1.0.0"}, f.replaceError
}
func (f *fakeEngine) WaitHealthy(context.Context, string, time.Duration) error { return f.healthError }
func (f *fakeEngine) VerifyVersion(context.Context, string, string) error      { return f.versionError }
func (f *fakeEngine) Rollback(context.Context, Replacement) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rolledBack++
	return f.rollbackError
}
func (f *fakeEngine) Finalize(context.Context, Replacement) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finalized++
	return nil
}

func testService(t *testing.T, engine *fakeEngine, free uint64, registryError error) *Service {
	t.Helper()
	service, err := NewService(Dependencies{
		Engine: engine, DataDir: t.TempDir(), HealthURL: "http://bridge/api/system/status", MinimumFree: 100,
		FreeBytes:     func(string) (uint64, error) { return free, nil },
		RegistryCheck: func(context.Context) error { return registryError },
		Backup:        func(_, id string) (string, error) { return "backup-" + id, nil },
		HealthTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func startAndWait(t *testing.T, service *Service) Operation {
	t.Helper()
	if _, err := service.Start(Request{RequestID: "request-1234", Tag: "v1.1.0", ExpectedVersion: "1.1.0"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		operation := service.Status()
		if terminal(operation.Stage) {
			return operation
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("operation did not finish")
	return Operation{}
}

func TestSuccessfulUpdate(t *testing.T) {
	engine := &fakeEngine{}
	operation := startAndWait(t, testService(t, engine, 1000, nil))
	if operation.Stage != StageSuccess || operation.BackupPath == "" || engine.finalized != 1 || engine.rolledBack != 0 {
		t.Fatalf("operation=%+v engine=%+v", operation, engine)
	}
}

func TestUnhealthyUpdateRollsBack(t *testing.T) {
	engine := &fakeEngine{healthError: errors.New("unhealthy")}
	operation := startAndWait(t, testService(t, engine, 1000, nil))
	if operation.Stage != StageFailed || engine.rolledBack != 1 || !strings.Contains(operation.Error, "unhealthy") {
		t.Fatalf("operation=%+v rollback=%d", operation, engine.rolledBack)
	}
}

func TestVersionMismatchRollsBack(t *testing.T) {
	engine := &fakeEngine{versionError: errors.New("wrong version")}
	operation := startAndWait(t, testService(t, engine, 1000, nil))
	if operation.Stage != StageFailed || engine.rolledBack != 1 || !strings.Contains(operation.Error, "version mismatch") {
		t.Fatalf("operation=%+v rollback=%d", operation, engine.rolledBack)
	}
}

func TestNetworkLossAndFullVolumeFailBeforeReplacement(t *testing.T) {
	for name, scenario := range map[string]struct {
		free uint64
		err  error
	}{
		"volume full": {99, nil}, "registry unavailable": {1000, errors.New("offline")},
	} {
		t.Run(name, func(t *testing.T) {
			engine := &fakeEngine{}
			operation := startAndWait(t, testService(t, engine, scenario.free, scenario.err))
			if operation.Stage != StageFailed || engine.rolledBack != 0 {
				t.Fatalf("operation=%+v", operation)
			}
		})
	}
}

func TestUpdaterRestartMarksInterruptedOperationFailed(t *testing.T) {
	directory := t.TempDir()
	state := `{"id":"old-request","stage":"pulling","tag":"v1.1.0"}`
	if err := os.WriteFile(filepath.Join(directory, "update-operation.json"), []byte(state), 0600); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(Dependencies{Engine: &fakeEngine{}, DataDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	operation := service.Status()
	if operation.Stage != StageFailed || !strings.Contains(operation.Error, "restarted") {
		t.Fatalf("operation=%+v", operation)
	}
}

func TestRequestIDReplayIsRejectedAfterRestart(t *testing.T) {
	directory := t.TempDir()
	dependencies := Dependencies{
		Engine: &fakeEngine{}, DataDir: directory, MinimumFree: 1,
		FreeBytes:     func(string) (uint64, error) { return 100, nil },
		RegistryCheck: func(context.Context) error { return nil },
		Backup:        func(_, id string) (string, error) { return "backup-" + id, nil },
	}
	service, err := NewService(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(Request{RequestID: "persistent-id", Tag: "v1.1.0", ExpectedVersion: "1.1.0"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for !terminal(service.Status().Stage) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	reloaded, err := NewService(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reloaded.Start(Request{RequestID: "persistent-id", Tag: "v1.1.0", ExpectedVersion: "1.1.0"}); err == nil {
		t.Fatal("replayed request ID was accepted")
	}
}
