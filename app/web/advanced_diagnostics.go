package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mqtt-home/roborock-mqtt/roborock"
)

type advancedDiagnosticsResponse struct {
	Diagnostics  roborock.AdvancedDiagnostics `json:"diagnostics"`
	Capabilities roborock.DeviceCapabilities  `json:"capabilities"`
}

func (ws *WebServer) deviceAdvancedDiagnostics(w http.ResponseWriter, r *http.Request) {
	if ws.deviceManager == nil {
		writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("bridge is not started"))
		return
	}
	slug := chi.URLParam(r, "slug")
	device := ws.deviceManager.GetDevice(slug)
	if device == nil {
		writeLoxoneError(w, http.StatusNotFound, fmt.Errorf("unknown robot %q", slug))
		return
	}
	if device.CloudMQTT == nil || !device.CloudMQTT.IsConnected() {
		writeLoxoneError(w, http.StatusConflict, fmt.Errorf("robot offline"))
		return
	}
	diagnostics, err := device.CloudMQTT.PollAdvancedDiagnostics()
	if err != nil {
		writeLoxoneError(w, http.StatusBadGateway, err)
		return
	}
	capabilities := roborock.InitialDeviceCapabilities(time.Now())
	if dependencies := ws.getLoxoneIntegration(); dependencies != nil && dependencies.Capabilities != nil {
		capabilities = dependencies.Capabilities.ObserveAdvancedDiagnostics(slug, diagnostics, time.Now())
	}
	w.Header().Set("Cache-Control", "no-store")
	writeLoxoneJSON(w, http.StatusOK, advancedDiagnosticsResponse{Diagnostics: diagnostics, Capabilities: capabilities})
}
