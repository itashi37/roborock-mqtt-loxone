package web

import (
	"net/http"

	"github.com/mqtt-home/roborock-mqtt/loxone/templates"
)

func (ws *WebServer) loxoneTemplateStatus(w http.ResponseWriter, _ *http.Request) {
	writeLoxoneJSON(w, http.StatusOK, templates.StatusForCurrentBuild())
}
