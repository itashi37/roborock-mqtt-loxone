package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqtt-home/roborock-mqtt/integration/updates"
	"github.com/mqtt-home/roborock-mqtt/roborock"
)

func TestSystemStatusAndUpdateCheck(t *testing.T) {
	server := NewWebServer(nil, &roborock.Client{}, nil)
	server.SetSystemIntegration(&SystemDependencies{
		Status: func() SystemStatus { return SystemStatus{Product: "roborock-mqtt-loxone", Version: "1.0.0"} },
		CheckUpdates: func(_ context.Context, channel string) (updates.Info, error) {
			return updates.Info{Channel: channel, LatestVersion: "1.1.0", Available: true}, nil
		},
	})

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/system/status", nil)
	statusResponse := httptest.NewRecorder()
	server.router.ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK || !strings.Contains(statusResponse.Body.String(), `"version":"1.0.0"`) {
		t.Fatalf("unexpected status response: %d %s", statusResponse.Code, statusResponse.Body.String())
	}

	checkRequest := httptest.NewRequest(http.MethodPost, "/api/system/updates/check", strings.NewReader(`{"channel":"edge"}`))
	checkResponse := httptest.NewRecorder()
	server.router.ServeHTTP(checkResponse, checkRequest)
	if checkResponse.Code != http.StatusOK || !strings.Contains(checkResponse.Body.String(), `"channel":"edge"`) {
		t.Fatalf("unexpected check response: %d %s", checkResponse.Code, checkResponse.Body.String())
	}
}
