package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mqtt-home/roborock-mqtt/integration/watchdog"
	"github.com/mqtt-home/roborock-mqtt/roborock"
)

func TestHealthEndpointsUseWatchdogSemantics(t *testing.T) {
	server := NewWebServer(nil, &roborock.Client{}, nil)
	report := watchdog.Report{Status: "degraded", Live: true, Ready: false}
	server.SetHealthProvider(func() watchdog.Report { return report })

	tests := []struct {
		path string
		want int
	}{
		{"/api/health", http.StatusServiceUnavailable},
		{"/api/live", http.StatusOK},
		{"/api/livez", http.StatusOK},
		{"/api/ready", http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Errorf("%s returned %d, want %d", test.path, response.Code, test.want)
		}
	}
}
