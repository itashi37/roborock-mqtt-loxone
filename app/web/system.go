package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mqtt-home/roborock-mqtt/integration/updates"
	"github.com/mqtt-home/roborock-mqtt/integration/watchdog"
)

type SystemTransportStatus struct {
	Enabled     bool      `json:"enabled"`
	Connected   bool      `json:"connected"`
	LastSuccess time.Time `json:"last_success,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
}

type DataVolumeStatus struct {
	Path      string `json:"path"`
	Writable  bool   `json:"writable"`
	FreeBytes uint64 `json:"free_bytes"`
	Error     string `json:"error,omitempty"`
}

type SystemStatus struct {
	Product            string                           `json:"product"`
	Version            string                           `json:"version"`
	GitCommit          string                           `json:"git_commit"`
	BuildTime          string                           `json:"build_time"`
	GoVersion          string                           `json:"go_version"`
	Architecture       string                           `json:"architecture"`
	Channel            string                           `json:"channel"`
	UptimeSeconds      int64                            `json:"uptime_seconds"`
	StartedAt          time.Time                        `json:"started_at"`
	LastRestart        time.Time                        `json:"last_restart"`
	LastWatchdogReason string                           `json:"last_watchdog_reason,omitempty"`
	Health             watchdog.Report                  `json:"health"`
	DataVolume         DataVolumeStatus                 `json:"data_volume"`
	Transports         map[string]SystemTransportStatus `json:"transports"`
	Update             updates.Info                     `json:"update"`
}

type SystemDependencies struct {
	Status       func() SystemStatus
	CheckUpdates func(context.Context, string) (updates.Info, error)
}

func (ws *WebServer) SetSystemIntegration(dependencies *SystemDependencies) {
	ws.systemMu.Lock()
	ws.system = dependencies
	ws.systemMu.Unlock()
}

func (ws *WebServer) getSystemIntegration() *SystemDependencies {
	ws.systemMu.RLock()
	defer ws.systemMu.RUnlock()
	return ws.system
}

func (ws *WebServer) systemStatus(w http.ResponseWriter, _ *http.Request) {
	dependencies := ws.getSystemIntegration()
	if dependencies == nil || dependencies.Status == nil {
		http.Error(w, `{"error":"system diagnostics are not ready"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dependencies.Status())
}

func (ws *WebServer) systemCheckUpdates(w http.ResponseWriter, r *http.Request) {
	dependencies := ws.getSystemIntegration()
	if dependencies == nil || dependencies.CheckUpdates == nil {
		http.Error(w, `{"error":"update checks are not ready"}`, http.StatusServiceUnavailable)
		return
	}
	var request struct {
		Channel string `json:"channel"`
	}
	_ = json.NewDecoder(r.Body).Decode(&request)
	channel := strings.ToLower(strings.TrimSpace(request.Channel))
	if channel != "edge" {
		channel = "stable"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	info, err := dependencies.CheckUpdates(ctx, channel)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
	}
	_ = json.NewEncoder(w).Encode(info)
}
