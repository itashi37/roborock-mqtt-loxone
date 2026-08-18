package web

import (
	"fmt"
	"net/http"
)

func (ws *WebServer) fleetHealth(w http.ResponseWriter, _ *http.Request) {
	if ws.deviceManager == nil {
		writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("bridge is not started"))
		return
	}
	writeLoxoneJSON(w, http.StatusOK, ws.deviceManager.FleetHealth())
}
