package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mqtt-home/roborock-mqtt/config"
	"github.com/mqtt-home/roborock-mqtt/integration/autoupdate"
	"github.com/mqtt-home/roborock-mqtt/integration/updates"
	"github.com/mqtt-home/roborock-mqtt/integration/watchdog"
	"github.com/mqtt-home/roborock-mqtt/updater"
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
	UpdateSettings     config.UpdateConfig              `json:"update_settings"`
	AutoUpdate         autoupdate.Diagnostics           `json:"auto_update"`
}

type SystemDependencies struct {
	Status             func() SystemStatus
	CheckUpdates       func(context.Context, string) (updates.Info, error)
	UpdaterStatus      func(context.Context) (updater.Operation, error)
	InstallUpdate      func(context.Context, string) (updater.Operation, error)
	SaveUpdateSettings func(config.UpdateConfig) error
}

func (ws *WebServer) systemSaveUpdateSettings(w http.ResponseWriter, r *http.Request) {
	dependencies := ws.getSystemIntegration()
	if dependencies == nil || dependencies.SaveUpdateSettings == nil {
		http.Error(w, `{"error":"update settings are not ready"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Header.Get("X-Roborock-Intent") != "save-update-settings" || !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		http.Error(w, `{"error":"missing settings confirmation header"}`, http.StatusForbidden)
		return
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && !sameOrigin(origin, r.Host) {
		http.Error(w, `{"error":"cross-origin settings request rejected"}`, http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var settings config.UpdateConfig
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		writeJSONError(w, "invalid update settings", http.StatusBadRequest)
		return
	}
	if err := dependencies.SaveUpdateSettings(settings); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(config.Get().Updates)
}

func (ws *WebServer) systemUpdateOperation(w http.ResponseWriter, r *http.Request) {
	dependencies := ws.getSystemIntegration()
	if dependencies == nil || dependencies.UpdaterStatus == nil {
		http.Error(w, `{"error":"isolated updater is not configured"}`, http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	operation, err := dependencies.UpdaterStatus(ctx)
	if err != nil {
		http.Error(w, `{"error":"isolated updater is unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operation)
}

func (ws *WebServer) systemInstallUpdate(w http.ResponseWriter, r *http.Request) {
	dependencies := ws.getSystemIntegration()
	if dependencies == nil || dependencies.InstallUpdate == nil {
		http.Error(w, `{"error":"isolated updater is not configured"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Header.Get("X-Roborock-Intent") != "install-update" || !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		http.Error(w, `{"error":"missing update confirmation header"}`, http.StatusForbidden)
		return
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && !sameOrigin(origin, r.Host) {
		http.Error(w, `{"error":"cross-origin update request rejected"}`, http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var request struct {
		Channel string `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	channel := strings.ToLower(strings.TrimSpace(request.Channel))
	if channel != "edge" {
		channel = "stable"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	operation, err := dependencies.InstallUpdate(ctx, channel)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(operation)
}

func sameOrigin(origin, host string) bool {
	origin = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://"), "/")
	return strings.EqualFold(origin, host)
}

func writeJSONError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
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
