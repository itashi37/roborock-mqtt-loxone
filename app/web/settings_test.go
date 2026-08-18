package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqtt-home/roborock-mqtt/config"
	"github.com/mqtt-home/roborock-mqtt/roborock"
)

func loadSecretSettings(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	base := filepath.Join(dir, "config.json")
	data := `{
		"mqtt":{"enabled":true,"url":"tcp://broker:1883","topic":"home/roborock","password":"mqtt-secret"},
		"roborock":{"username":"owner@example.com"},
		"loxone":{"direct":{"enabled":true,"host":"192.168.1.10","password":"loxone-secret","api_token":"api-secret"}},
		"web":{}
	}`
	if err := os.WriteFile(base, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadConfig(base); err != nil {
		t.Fatal(err)
	}
}

func TestSetupStatusNeverReturnsSecrets(t *testing.T) {
	loadSecretSettings(t)
	server := NewWebServer(nil, roborock.NewClient("http://example.invalid", "owner@example.com", "", ""), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, secret := range []string{"mqtt-secret", "loxone-secret", "api-secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("setup response leaked %q: %s", secret, body)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	mqtt := decoded["mqtt"].(map[string]any)
	if mqtt["password_configured"] != true {
		t.Fatalf("expected password presence marker: %v", mqtt)
	}
}

func TestMergeSecretsKeepsOmittedCredentials(t *testing.T) {
	loadSecretSettings(t)
	merged := mergeSecrets(settingsUpdateRequest{
		RoborockUsername: "owner@example.com",
		MQTT:             config.MQTTConfig{URL: "tcp://new-broker:1883"},
		Loxone:           config.LoxoneConfig{Direct: config.DirectLoxoneConfig{Host: "192.168.1.11"}},
	})
	if merged.MQTT.Password != "mqtt-secret" || merged.Loxone.Direct.Password != "loxone-secret" || merged.Loxone.Direct.APIToken != "api-secret" {
		t.Fatalf("credentials were not preserved: %+v", merged)
	}
}
