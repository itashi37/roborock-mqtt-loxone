package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigLoxoneDefaults(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.json")
	data := []byte(`{
		"mqtt":{"url":"tcp://localhost:1883","topic":"home/roborock"},
		"roborock":{"username":"user@example.com"},
		"loxone":{"enabled":true,"topic":""},
		"web":{"enabled":true}
	}`)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !loaded.Loxone.Enabled {
		t.Fatal("expected Loxone mode to be enabled")
	}
	if !loaded.MQTT.IsEnabled() {
		t.Fatal("legacy configuration must keep local MQTT enabled")
	}
	if loaded.Loxone.Topic != "loxone/roborock" {
		t.Fatalf("got topic %q, want loxone/roborock", loaded.Loxone.Topic)
	}
	if loaded.Loxone.CommandDebounceMS != 2000 {
		t.Fatalf("got debounce %d, want 2000", loaded.Loxone.CommandDebounceMS)
	}
	if loaded.Loxone.CommandTimeoutSeconds != 90 {
		t.Fatalf("got timeout %d, want 90", loaded.Loxone.CommandTimeoutSeconds)
	}
}

func TestLoadConfigCanDisableLocalMQTT(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{
		"mqtt":{"enabled":false,"url":"tcp://localhost:1883","topic":"home/roborock"},
		"roborock":{"username":"user@example.com"},
		"web":{"enabled":true}
	}`)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.MQTT.IsEnabled() {
		t.Fatal("expected local MQTT to be disabled")
	}
	if got := loaded.MQTT.GatewayConfig().URL; got != "tcp://localhost:1883" {
		t.Fatalf("gateway URL = %q", got)
	}
}

func TestLoadConfigDirectLoxoneDefaults(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{
		"mqtt":{"enabled":false},
		"roborock":{"username":"user@example.com"},
		"loxone":{"direct":{"enabled":true,"host":"192.168.1.20"}},
		"web":{"enabled":true}
	}`)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	direct := loaded.Loxone.Direct
	if !direct.Enabled || direct.Scheme != "http" || direct.Port != 80 || direct.TimeoutSeconds != 5 || direct.MaxRetries != 3 || direct.InputPrefix != "RR" {
		t.Fatalf("unexpected Direct defaults: %+v", direct)
	}
}

func TestDeviceIntegrationModesPreferStableDeviceID(t *testing.T) {
	falseValue, trueValue := false, true
	config := LoxoneConfig{
		Direct: DirectLoxoneConfig{Enabled: true},
		Devices: map[string]DeviceIntegrationConfig{
			"did-a": {MQTT: &falseValue, Direct: &trueValue},
			"robot": {MQTT: &trueValue, Direct: &falseValue},
		},
	}
	mqttEnabled, directEnabled := config.DeviceModes("did-a", "robot")
	if mqttEnabled || !directEnabled {
		t.Fatalf("DUID modes not preferred: mqtt=%v direct=%v", mqttEnabled, directEnabled)
	}
	mqttEnabled, directEnabled = config.DeviceModes("did-unknown", "unknown")
	if !mqttEnabled || !directEnabled {
		t.Fatalf("unexpected defaults: mqtt=%v direct=%v", mqttEnabled, directEnabled)
	}
}

func TestLoadConfigLoxoneCustomTopic(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.json")
	data := []byte(`{
		"mqtt":{"url":"tcp://localhost:1883","topic":"home/roborock"},
		"roborock":{"username":"user@example.com"},
		"loxone":{"enabled":true,"topic":" house/vacuum/ ","command_debounce_ms":3500,"command_timeout_seconds":120},
		"web":{"enabled":true}
	}`)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.Loxone.Topic != "house/vacuum" {
		t.Fatalf("got topic %q, want house/vacuum", loaded.Loxone.Topic)
	}
	if loaded.Loxone.CommandDebounceMS != 3500 {
		t.Fatalf("got debounce %d, want 3500", loaded.Loxone.CommandDebounceMS)
	}
	if loaded.Loxone.CommandTimeoutSeconds != 120 {
		t.Fatalf("got timeout %d, want 120", loaded.Loxone.CommandTimeoutSeconds)
	}
}
