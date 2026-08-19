package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqtt-home/roborock-mqtt/integration/updates"
	"github.com/mqtt-home/roborock-mqtt/roborock"
	"github.com/mqtt-home/roborock-mqtt/updater"
)

func TestSystemStatusAndUpdateCheck(t *testing.T) {
	server := NewWebServer(nil, &roborock.Client{}, nil)
	server.SetSystemIntegration(&SystemDependencies{
		Status: func() SystemStatus { return SystemStatus{Product: "roborock-mqtt-loxone", Version: "1.0.0"} },
		CheckUpdates: func(_ context.Context, channel string) (updates.Info, error) {
			return updates.Info{Channel: channel, LatestVersion: "1.1.0", Available: true}, nil
		},
		UpdaterStatus: func(context.Context) (updater.Operation, error) {
			return updater.Operation{ID: "current", Stage: updater.StagePulling}, nil
		},
		InstallUpdate: func(_ context.Context, channel string) (updater.Operation, error) {
			return updater.Operation{ID: channel, Stage: updater.StagePreparing}, nil
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

func TestSystemInstallRequiresSameOriginConfirmation(t *testing.T) {
	server := NewWebServer(nil, &roborock.Client{}, nil)
	server.SetSystemIntegration(&SystemDependencies{InstallUpdate: func(_ context.Context, channel string) (updater.Operation, error) {
		return updater.Operation{ID: channel, Stage: updater.StagePreparing}, nil
	}})
	body := `{"channel":"stable"}`

	missing := httptest.NewRecorder()
	server.router.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/api/system/updates/install", strings.NewReader(body)))
	if missing.Code != http.StatusForbidden {
		t.Fatalf("missing confirmation=%d", missing.Code)
	}

	crossOriginRequest := httptest.NewRequest(http.MethodPost, "/api/system/updates/install", strings.NewReader(body))
	crossOriginRequest.Header.Set("Content-Type", "application/json")
	crossOriginRequest.Header.Set("X-Roborock-Intent", "install-update")
	crossOriginRequest.Header.Set("Origin", "https://evil.example")
	crossOrigin := httptest.NewRecorder()
	server.router.ServeHTTP(crossOrigin, crossOriginRequest)
	if crossOrigin.Code != http.StatusForbidden {
		t.Fatalf("cross origin=%d", crossOrigin.Code)
	}

	acceptedRequest := httptest.NewRequest(http.MethodPost, "/api/system/updates/install", strings.NewReader(body))
	acceptedRequest.Header.Set("Content-Type", "application/json")
	acceptedRequest.Header.Set("X-Roborock-Intent", "install-update")
	acceptedRequest.Header.Set("Origin", "http://example.com")
	accepted := httptest.NewRecorder()
	server.router.ServeHTTP(accepted, acceptedRequest)
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("accepted=%d body=%s", accepted.Code, accepted.Body.String())
	}
}
