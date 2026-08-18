package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mqtt-home/roborock-mqtt/config"
	"github.com/mqtt-home/roborock-mqtt/integration/localmqtt"
	loxonedirect "github.com/mqtt-home/roborock-mqtt/loxone/direct"
)

type IntegrationSettingsDependencies struct {
	Apply      func(config.RuntimeSettings) error
	TestMQTT   func(context.Context, config.MQTTConfig) error
	TestDirect func(context.Context, config.DirectLoxoneConfig) error
	MQTTStatus func() localmqtt.Diagnostics
}

type setupSettingsResponse struct {
	SetupComplete     bool                          `json:"setup_complete"`
	Authenticated     bool                          `json:"authenticated"`
	RoborockUsername  string                        `json:"roborock_username"`
	MQTT              sanitizedMQTTConfig           `json:"mqtt"`
	Loxone            sanitizedLoxoneConfig         `json:"loxone"`
	MQTTDiagnostics   localmqtt.Diagnostics         `json:"mqtt_diagnostics"`
	DirectDiagnostics *loxonedirect.SyncDiagnostics `json:"direct_diagnostics,omitempty"`
}

type sanitizedMQTTConfig struct {
	Enabled            bool   `json:"enabled"`
	URL                string `json:"url"`
	Retain             bool   `json:"retain"`
	Topic              string `json:"topic"`
	QoS                byte   `json:"qos"`
	Username           string `json:"username"`
	TLS                bool   `json:"tls"`
	PasswordConfigured bool   `json:"password_configured"`
}

type sanitizedLoxoneConfig struct {
	Enabled bool                                      `json:"enabled"`
	Topic   string                                    `json:"topic"`
	Direct  sanitizedDirectLoxoneConfig               `json:"direct"`
	Devices map[string]config.DeviceIntegrationConfig `json:"devices,omitempty"`
}

type sanitizedDirectLoxoneConfig struct {
	Enabled            bool                         `json:"enabled"`
	Scheme             string                       `json:"scheme"`
	Host               string                       `json:"host"`
	Port               int                          `json:"port"`
	Username           string                       `json:"username"`
	TimeoutSeconds     int                          `json:"timeout_seconds"`
	MaxRetries         int                          `json:"max_retries"`
	RetryDelayMS       int                          `json:"retry_delay_ms"`
	InputPrefix        string                       `json:"input_prefix"`
	Inputs             map[string]map[string]string `json:"inputs,omitempty"`
	APIUsername        string                       `json:"api_username"`
	AllowedCIDRs       []string                     `json:"allowed_cidrs,omitempty"`
	AllowGETCommands   bool                         `json:"allow_get_commands"`
	RateLimitPerMinute int                          `json:"rate_limit_per_minute"`
	PasswordConfigured bool                         `json:"password_configured"`
	APITokenConfigured bool                         `json:"api_token_configured"`
}

type settingsUpdateRequest struct {
	SetupComplete    *bool               `json:"setup_complete,omitempty"`
	RoborockUsername string              `json:"roborock_username"`
	MQTT             config.MQTTConfig   `json:"mqtt"`
	Loxone           config.LoxoneConfig `json:"loxone"`
}

func (ws *WebServer) SetIntegrationSettings(dependencies *IntegrationSettingsDependencies) {
	ws.settingsMu.Lock()
	defer ws.settingsMu.Unlock()
	ws.settings = dependencies
}

func (ws *WebServer) getIntegrationSettings() *IntegrationSettingsDependencies {
	ws.settingsMu.RLock()
	defer ws.settingsMu.RUnlock()
	return ws.settings
}

func sanitizeSettings() setupSettingsResponse {
	settings := config.RuntimeSettingsSnapshot()
	direct := settings.Loxone.Direct
	response := setupSettingsResponse{
		SetupComplete:    config.SetupComplete(),
		RoborockUsername: settings.RoborockUsername,
		MQTT: sanitizedMQTTConfig{
			Enabled: settings.MQTT.IsEnabled(), URL: settings.MQTT.URL, Retain: settings.MQTT.Retain,
			Topic: settings.MQTT.Topic, QoS: settings.MQTT.QoS, Username: settings.MQTT.Username,
			TLS: settings.MQTT.TLS, PasswordConfigured: settings.MQTT.Password != "",
		},
		Loxone: sanitizedLoxoneConfig{
			Enabled: settings.Loxone.Enabled, Topic: settings.Loxone.Topic, Devices: settings.Loxone.Devices,
			Direct: sanitizedDirectLoxoneConfig{
				Enabled: direct.Enabled, Scheme: direct.Scheme, Host: direct.Host, Port: direct.Port,
				Username: direct.Username, TimeoutSeconds: direct.TimeoutSeconds, MaxRetries: direct.MaxRetries,
				RetryDelayMS: direct.RetryDelayMS, InputPrefix: direct.InputPrefix, Inputs: direct.Inputs,
				APIUsername: direct.APIUsername, AllowedCIDRs: direct.AllowedCIDRs,
				AllowGETCommands: direct.AllowGETCommands, RateLimitPerMinute: direct.RateLimitPerMinute,
				PasswordConfigured: direct.Password != "", APITokenConfigured: direct.APIToken != "",
			},
		},
	}
	return response
}

func (ws *WebServer) setupStatus(w http.ResponseWriter, _ *http.Request) {
	response := sanitizeSettings()
	response.Authenticated = ws.restClient.IsAuthenticated()
	if dependencies := ws.getIntegrationSettings(); dependencies != nil && dependencies.MQTTStatus != nil {
		response.MQTTDiagnostics = dependencies.MQTTStatus()
	}
	if dependencies := ws.getLoxoneIntegration(); dependencies != nil && dependencies.DirectDiagnostics != nil {
		diagnostics := dependencies.DirectDiagnostics()
		response.DirectDiagnostics = &diagnostics
	}
	w.Header().Set("Cache-Control", "no-store")
	writeLoxoneJSON(w, http.StatusOK, response)
}

func mergeSecrets(request settingsUpdateRequest) config.RuntimeSettings {
	current := config.RuntimeSettingsSnapshot()
	if request.MQTT.Password == "" {
		request.MQTT.Password = current.MQTT.Password
	}
	if request.Loxone.Direct.Password == "" {
		request.Loxone.Direct.Password = current.Loxone.Direct.Password
	}
	if request.Loxone.Direct.APIToken == "" {
		request.Loxone.Direct.APIToken = current.Loxone.Direct.APIToken
	}
	if strings.TrimSpace(request.RoborockUsername) == "" {
		request.RoborockUsername = current.RoborockUsername
	}
	return config.RuntimeSettings{MQTT: request.MQTT, Loxone: request.Loxone, RoborockUsername: request.RoborockUsername, SetupComplete: request.SetupComplete}
}

func (ws *WebServer) setupSave(w http.ResponseWriter, r *http.Request) {
	var request settingsUpdateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeLoxoneError(w, http.StatusBadRequest, fmt.Errorf("invalid settings: %w", err))
		return
	}
	settings := mergeSecrets(request)
	dependencies := ws.getIntegrationSettings()
	if dependencies == nil || dependencies.Apply == nil {
		writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("settings service is not ready"))
		return
	}
	if err := dependencies.Apply(settings); err != nil {
		writeLoxoneError(w, http.StatusBadRequest, err)
		return
	}
	ws.setupStatus(w, r)
}

func (ws *WebServer) mqttConfigTest(w http.ResponseWriter, r *http.Request) {
	var mqttConfig config.MQTTConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&mqttConfig); err != nil {
		writeLoxoneError(w, http.StatusBadRequest, err)
		return
	}
	if mqttConfig.Password == "" {
		mqttConfig.Password = config.Get().MQTT.Password
	}
	dependencies := ws.getIntegrationSettings()
	if dependencies == nil || dependencies.TestMQTT == nil {
		writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("MQTT test is unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	if err := dependencies.TestMQTT(ctx, mqttConfig); err != nil {
		writeLoxoneError(w, http.StatusBadGateway, err)
		return
	}
	writeLoxoneJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (ws *WebServer) directConfigTest(w http.ResponseWriter, r *http.Request) {
	var direct config.DirectLoxoneConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&direct); err != nil {
		writeLoxoneError(w, http.StatusBadRequest, err)
		return
	}
	if direct.Password == "" {
		direct.Password = config.Get().Loxone.Direct.Password
	}
	dependencies := ws.getIntegrationSettings()
	if dependencies == nil || dependencies.TestDirect == nil {
		writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("Direct Loxone test is unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	if err := dependencies.TestDirect(ctx, direct); err != nil {
		writeLoxoneError(w, http.StatusBadGateway, err)
		return
	}
	writeLoxoneJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (ws *WebServer) rotateDirectToken(w http.ResponseWriter, _ *http.Request) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		writeLoxoneError(w, http.StatusInternalServerError, err)
		return
	}
	token := hex.EncodeToString(buffer)
	settings := config.RuntimeSettingsSnapshot()
	settings.Loxone.Direct.APIToken = token
	dependencies := ws.getIntegrationSettings()
	if dependencies == nil || dependencies.Apply == nil {
		writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("settings service is not ready"))
		return
	}
	if err := dependencies.Apply(settings); err != nil {
		writeLoxoneError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeLoxoneJSON(w, http.StatusOK, map[string]string{"token": token})
}
