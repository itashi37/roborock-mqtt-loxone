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
