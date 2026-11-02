package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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

func TestSetupSavePreservesExistingLoxoneContract(t *testing.T) {
	loadSecretSettings(t)
	settings := config.RuntimeSettingsSnapshot()
	complete, enabled, disabled := true, true, false
	settings.SetupComplete = &complete
	settings.Loxone.Enabled = true
	settings.Loxone.Topic = "my-existing/loxone/robots"
	settings.Loxone.CommandDebounceMS = 4500
	settings.Loxone.CommandTimeoutSeconds = 180
	settings.Loxone.Devices = map[string]config.DeviceIntegrationConfig{
		"did-first": {MQTT: &disabled, Direct: &enabled},
		"old-robot": {MQTT: &enabled, Direct: &disabled},
	}
	settings.Loxone.Direct.InputPrefix = "EXISTING"
	settings.Loxone.Direct.Inputs = map[string]map[string]string{
		"old-robot": {"running": "My existing running VI", "current_room_id": "Room unchanged"},
	}
	settings.Loxone.Direct.APIUsername = "existing-api-user"
	settings.Loxone.Direct.AllowedCIDRs = []string{"192.168.20.0/24"}
	settings.Loxone.Direct.AllowGETCommands = true
	settings.Loxone.Direct.RateLimitPerMinute = 70
	if err := config.SaveRuntimeSettings(settings); err != nil {
		t.Fatal(err)
	}

	// Simulate the setup form's sanitized round trip. Presence markers are UI
	// metadata, not writable settings; credentials remain absent from the body.
	data, err := json.Marshal(sanitizeSettings())
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	var request settingsUpdateRequest
	if err := json.Unmarshal(response["mqtt"], &request.MQTT); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(response["loxone"], &request.Loxone); err != nil {
		t.Fatal(err)
	}
	request.Loxone.Direct.Host = "192.168.1.11"
	merged := mergeSecrets(request)
	want := settings
	want.Loxone.Direct.Host = "192.168.1.11"
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("setup round trip changed existing integration settings:\n got: %+v\nwant: %+v", merged, want)
	}
	if err := config.SaveRuntimeSettings(merged); err != nil {
		t.Fatal(err)
	}
	if got := config.RuntimeSettingsSnapshot(); !reflect.DeepEqual(got, want) {
		t.Fatal("saved settings changed the existing Loxone contract")
	}
}

func TestMergeSecretsPreservesOmittedAdvancedLoxoneSettings(t *testing.T) {
	loadSecretSettings(t)
	settings := config.RuntimeSettingsSnapshot()
	enabled := true
	settings.Loxone.CommandDebounceMS = 4500
	settings.Loxone.CommandTimeoutSeconds = 180
	settings.Loxone.Devices = map[string]config.DeviceIntegrationConfig{"old-robot": {Direct: &enabled}}
	settings.Loxone.Direct.InputPrefix = "EXISTING"
	settings.Loxone.Direct.Inputs = map[string]map[string]string{"old-robot": {"running": "Existing VI"}}
	if err := config.SaveRuntimeSettings(settings); err != nil {
		t.Fatal(err)
	}
	merged := mergeSecrets(settingsUpdateRequest{Loxone: config.LoxoneConfig{Direct: config.DirectLoxoneConfig{Host: "192.168.1.11"}}})
	if merged.Loxone.CommandDebounceMS != 4500 || merged.Loxone.CommandTimeoutSeconds != 180 || merged.Loxone.Direct.InputPrefix != "EXISTING" || !reflect.DeepEqual(merged.Loxone.Direct.Inputs, settings.Loxone.Direct.Inputs) || !reflect.DeepEqual(merged.Loxone.Devices, settings.Loxone.Devices) {
		t.Fatal("omitted advanced Loxone settings were replaced")
	}
	merged = mergeSecrets(settingsUpdateRequest{Loxone: config.LoxoneConfig{
		Devices: map[string]config.DeviceIntegrationConfig{},
		Direct:  config.DirectLoxoneConfig{Inputs: map[string]map[string]string{}},
	}})
	if len(merged.Loxone.Devices) != 0 || len(merged.Loxone.Direct.Inputs) != 0 {
		t.Fatal("explicit empty mappings must still clear overrides")
	}
}
