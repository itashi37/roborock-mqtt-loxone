package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/mqtt-home/roborock-mqtt/config"
	"github.com/mqtt-home/roborock-mqtt/roborock"
)

const testDirectToken = "0123456789abcdef0123456789abcdef01234567"

func loadDirectAPIConfig(t *testing.T, extra string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "config.json")
	data := `{
		"mqtt":{"enabled":false},
		"roborock":{"username":"test@example.com"},
		"loxone":{"direct":{"enabled":true,"host":"192.168.1.20","api_username":"loxone","api_token":"` + testDirectToken + `","allowed_cidrs":["192.0.2.0/24"],"rate_limit_per_minute":30` + extra + `}},
		"web":{"enabled":true}
	}`
	if err := os.WriteFile(file, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := appconfig.LoadConfig(file); err != nil {
		t.Fatal(err)
	}
}

func authenticatedDirectRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.RemoteAddr = "192.0.2.10:4321"
	request.SetBasicAuth("loxone", testDirectToken)
	return request
}

func TestDirectCanonicalCommandUsesCoordinator(t *testing.T) {
	loadDirectAPIConfig(t, "")
	server := NewWebServer(nil, &roborock.Client{}, nil)
	var gotSlug, gotCommand string
	server.SetLoxoneIntegration(&LoxoneDependencies{SubmitCommand: func(slug, command string) roborock.CommandSubmission {
		gotSlug, gotCommand = slug, command
		return roborock.CommandSubmission{ID: "cmd-1", Command: command, State: "accepted", Accepted: true}
	}})
	request := authenticatedDirectRequest(http.MethodPost, "/api/loxone/direct/v1/devices/robot/commands", `{"command":"dock"}`)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || gotSlug != "robot" || gotCommand != "dock" {
		t.Fatalf("status=%d slug=%q command=%q body=%s", response.Code, gotSlug, gotCommand, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body["id"] != "cmd-1" || body["timestamp"] == nil {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestDirectSimpleRoutesOnlyTranslateCommands(t *testing.T) {
	loadDirectAPIConfig(t, "")
	server := NewWebServer(nil, &roborock.Client{}, nil)
	var commands []string
	server.SetLoxoneIntegration(&LoxoneDependencies{SubmitCommand: func(_, command string) roborock.CommandSubmission {
		commands = append(commands, command)
		return roborock.CommandSubmission{ID: "cmd", Command: command, State: "accepted", Accepted: true}
	}})
	for _, path := range []string{"start", "rooms/23", "scenes/7", "fan/turbo", "mop/deep", "water/moderate"} {
		request := authenticatedDirectRequest(http.MethodPost, "/api/loxone/direct/v1/devices/robot/commands/"+path, "")
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	want := []string{"start", "clean_room_id:23", "scene_id:7", "fan:turbo", "mop:deep", "water:moderate"}
	if strings.Join(commands, "|") != strings.Join(want, "|") {
		t.Fatalf("commands=%v want=%v", commands, want)
	}
}

func TestDirectAPIAuthenticationCIDRAndGETCompatibility(t *testing.T) {
	loadDirectAPIConfig(t, "")
	server := NewWebServer(nil, &roborock.Client{}, nil)
	server.SetLoxoneIntegration(&LoxoneDependencies{SubmitCommand: func(_, command string) roborock.CommandSubmission {
		return roborock.CommandSubmission{ID: "cmd", Command: command, State: "accepted", Accepted: true}
	}})

	unauthenticated := httptest.NewRequest(http.MethodPost, "/api/loxone/direct/v1/devices/robot/commands/start", nil)
	unauthenticated.RemoteAddr = "192.0.2.10:1234"
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, unauthenticated)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", response.Code)
	}

	forbidden := authenticatedDirectRequest(http.MethodPost, "/api/loxone/direct/v1/devices/robot/commands/start", "")
	forbidden.RemoteAddr = "198.51.100.5:1234"
	response = httptest.NewRecorder()
	server.router.ServeHTTP(response, forbidden)
	if response.Code != http.StatusForbidden {
		t.Fatalf("CIDR status=%d", response.Code)
	}

	get := authenticatedDirectRequest(http.MethodGet, "/api/loxone/direct/v1/devices/robot/commands/start", "")
	response = httptest.NewRecorder()
	server.router.ServeHTTP(response, get)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET compatibility status=%d", response.Code)
	}
}

func TestDirectAPIRateLimitAndFailureMapping(t *testing.T) {
	loadDirectAPIConfig(t, `,"rate_limit_per_minute":1`)
	server := NewWebServer(nil, &roborock.Client{}, nil)
	server.SetLoxoneIntegration(&LoxoneDependencies{SubmitCommand: func(_, command string) roborock.CommandSubmission {
		return roborock.CommandSubmission{ID: "cmd", Command: command, State: "failed", Error: "robot offline", Failure: "conflict"}
	}})
	request := authenticatedDirectRequest(http.MethodPost, "/api/loxone/direct/v1/devices/robot/commands/start", "")
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("failure status=%d", response.Code)
	}
	request = authenticatedDirectRequest(http.MethodPost, "/api/loxone/direct/v1/devices/robot/commands/start", "")
	response = httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit status=%d", response.Code)
	}
}

func TestDirectCommandStatusUsesDiagnosticStore(t *testing.T) {
	loadDirectAPIConfig(t, "")
	store := roborock.NewLoxoneDiagnosticStore(10)
	errorText := ""
	store.Record("robot", roborock.LoxoneActivity{Type: "command", ID: "cmd-7", Command: "dock", State: "completed", Error: &errorText})
	server := NewWebServer(nil, &roborock.Client{}, nil)
	server.SetLoxoneIntegration(&LoxoneDependencies{FindCommand: store.FindCommand})
	request := authenticatedDirectRequest(http.MethodGet, "/api/loxone/direct/v1/commands/cmd-7", "")
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"completed"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
