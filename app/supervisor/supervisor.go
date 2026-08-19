package supervisor

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Replacement is an opaque update transaction understood by a platform
// supervisor. Docker uses container IDs; a Synology package can use package
// versions and staging paths without changing the update coordinator.
type Replacement struct {
	ServiceName      string `json:"service_name,omitempty"`
	PreviousInstance string `json:"previous_instance,omitempty"`
	PreviousName     string `json:"previous_name,omitempty"`
	NewInstance      string `json:"new_instance,omitempty"`
	PreviousArtifact string `json:"previous_artifact,omitempty"`
	TargetArtifact   string `json:"target_artifact,omitempty"`
}

// UpdateSupervisor is the minimal platform contract required by the safe
// update coordinator. Implementations must provide transactional rollback.
type UpdateSupervisor interface {
	CurrentArtifact(context.Context, string) (string, error)
	Fetch(context.Context, string) error
	Prepare(context.Context, string, string) (Replacement, error)
	Activate(context.Context, *Replacement) error
	WaitHealthy(context.Context, string, time.Duration) error
	VerifyVersion(context.Context, string, string) error
	Rollback(context.Context, Replacement) error
	Finalize(context.Context, Replacement) error
}

type RuntimeStatus struct {
	Kind             string `json:"kind"`
	DataDir          string `json:"data_dir"`
	LogMode          string `json:"log_mode"`
	RestartSupported bool   `json:"restart_supported"`
}

// Runtime controls only the current service process lifecycle. The external
// Docker or DSM service manager remains responsible for actually restarting it.
type Runtime struct {
	status   RuntimeStatus
	restarts chan string
	once     sync.Once
}

func NewRuntime(kind, dataDir string) *Runtime {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		kind = "docker"
	}
	return &Runtime{status: RuntimeStatus{Kind: kind, DataDir: dataDir, LogMode: "stdout", RestartSupported: true}, restarts: make(chan string, 1)}
}

func (r *Runtime) Status() RuntimeStatus          { return r.status }
func (r *Runtime) RestartRequests() <-chan string { return r.restarts }

func (r *Runtime) RequestRestart(reason string) {
	r.once.Do(func() { r.restarts <- reason })
}
