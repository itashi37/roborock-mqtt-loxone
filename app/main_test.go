package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mqtt-home/roborock-mqtt/config"
	"github.com/mqtt-home/roborock-mqtt/roborock"
)

func loadCommandTestConfig(t *testing.T, mqttEnabled, loxoneMQTTEnabled, directEnabled bool, devices map[string]config.DeviceIntegrationConfig) {
	t.Helper()
	settings := config.Config{
		MQTT: config.MQTTConfig{Enabled: &mqttEnabled},
		Loxone: config.LoxoneConfig{
			Enabled: loxoneMQTTEnabled,
			Direct:  config.DirectLoxoneConfig{Enabled: directEnabled},
			Devices: devices,
		},
	}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadConfig(file); err != nil {
		t.Fatal(err)
	}
}

func TestWebCommandsRespectEachRobotsEnabledLoxoneTransport(t *testing.T) {
	previousManager, previousCoordinator := deviceManager, commandCoordinator
	t.Cleanup(func() { deviceManager, commandCoordinator = previousManager, previousCoordinator })
	deviceManager = roborock.NewDeviceManager(nil, []roborock.DeviceInfo{{DID: "did-robot", Name: "Robot"}}, nil, t.TempDir())
	commandCoordinator = nil
	trueValue, falseValue := true, false
	tests := []struct {
		name                     string
		mqtt, loxoneMQTT, direct bool
		device                   config.DeviceIntegrationConfig
		allowed                  bool
	}{
		{name: "direct only", direct: true, allowed: true},
		{name: "legacy MQTT", mqtt: true, loxoneMQTT: true, allowed: true},
		{name: "both transports", mqtt: true, loxoneMQTT: true, direct: true, allowed: true},
		{name: "disabled globally"},
		{name: "local MQTT disabled", loxoneMQTT: true},
		{name: "device MQTT disabled", mqtt: true, loxoneMQTT: true, device: config.DeviceIntegrationConfig{MQTT: &falseValue}},
		{name: "device Direct disabled", direct: true, device: config.DeviceIntegrationConfig{Direct: &falseValue}},
		{name: "device cannot enable global Direct", device: config.DeviceIntegrationConfig{Direct: &trueValue}},
		{name: "device Direct still available", mqtt: true, loxoneMQTT: true, direct: true, device: config.DeviceIntegrationConfig{MQTT: &falseValue}, allowed: true},
		{name: "device MQTT still available", mqtt: true, loxoneMQTT: true, direct: true, device: config.DeviceIntegrationConfig{Direct: &falseValue}, allowed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loadCommandTestConfig(t, test.mqtt, test.loxoneMQTT, test.direct, map[string]config.DeviceIntegrationConfig{"did-robot": test.device})
			err := publishLoxoneWebCommand("robot", "start")
			want := "Loxone is disabled for this robot"
			if test.allowed {
				want = "command coordinator is not ready"
			}
			if err == nil || err.Error() != want {
				t.Fatalf("error = %v, want %q", err, want)
			}
		})
	}
}

func TestDirectOnlyWebCommandUsesSharedCoordinator(t *testing.T) {
	previousManager, previousCoordinator := deviceManager, commandCoordinator
	t.Cleanup(func() { deviceManager, commandCoordinator = previousManager, previousCoordinator })
	deviceManager = nil
	loadCommandTestConfig(t, false, false, true, nil)
	dispatched := make(chan string, 1)
	commandCoordinator = roborock.NewCommandCoordinator(time.Second, time.Second,
		func(slug string) (roborock.CommandContext, bool) {
			return roborock.CommandContext{Slug: slug, Online: true}, slug == "robot"
		},
		func(context roborock.CommandContext, command roborock.LoxoneCommand) error {
			dispatched <- context.Slug + ":" + command.Action
			return nil
		}, nil,
	)
	commandCoordinator.UpdateStatus("robot", &roborock.PublishedStatus{State: "idle"}, time.Now())
	if err := publishLoxoneWebCommand("robot", "start"); err != nil {
		t.Fatalf("Direct-only command rejected: %v", err)
	}
	select {
	case got := <-dispatched:
		if got != "robot:start" {
			t.Fatalf("dispatch = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("Direct-only command did not reach the shared coordinator")
	}
	if err := publishLoxoneWebCommand("robot", "invalid-command"); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("coordinator validation was bypassed: %v", err)
	}
}

func TestStartupRefreshDiscoversNewRobotsAndPreservesExistingAddresses(t *testing.T) {
	requests := 0
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/getHomeDetail":
			_, _ = w.Write([]byte(`{"code":200,"data":{"id":7}}`))
		case "/user/homes/7":
			_, _ = w.Write([]byte(`{"success":true,"result":{"devices":[{"duid":"original","name":"Renamed robot"},{"duid":"new","name":"Roborock Qrevo Curv"}]}}`))
		default:
			t.Errorf("unexpected cloud request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer cloud.Close()
	directory := t.TempDir()
	cached := []roborock.DeviceInfo{{DID: "original", Name: "Roborock Qrevo Curv"}}
	previous := roborock.NewDeviceManager(nil, cached, nil, directory)
	writeStartupSession(t, directory, cloud.URL, cached)
	client := roborock.NewClient(cloud.URL, "", "", "")
	client.SetSessionDir(directory)
	if !restoreBridgeSession(client) {
		t.Fatal("saved session failed to start after successful discovery")
	}
	if requests != 2 || len(client.GetDevices()) != 2 {
		t.Fatalf("cloud requests = %d, devices = %d", requests, len(client.GetDevices()))
	}
	next := roborock.NewDeviceManager(client.GetLoginData(), client.GetDevices(), nil, directory)
	if next.GetDevices()[0].Slug != previous.GetDevices()[0].Slug || next.GetDevices()[0].Slug == next.GetDevices()[1].Slug {
		t.Fatalf("existing addresses changed or collided: %v", next.GetSummaries())
	}
	persisted := roborock.NewClient(cloud.URL, "", "", "")
	persisted.SetSessionDir(directory)
	if !persisted.LoadSession() || len(persisted.GetDevices()) != 2 || persisted.GetLoginData().Token != "existing-token" {
		t.Fatal("refreshed devices and original session were not persisted")
	}
}

func TestStartupRefreshKeepsCachedRobotsWhenCloudUnavailable(t *testing.T) {
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":503}`))
	}))
	defer cloud.Close()
	directory := t.TempDir()
	cached := []roborock.DeviceInfo{{DID: "original", Name: "Roborock Qrevo Curv"}}
	writeStartupSession(t, directory, cloud.URL, cached)
	file := filepath.Join(directory, "session.json")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	client := roborock.NewClient(cloud.URL, "", "", "")
	client.SetSessionDir(directory)
	if !restoreBridgeSession(client) || len(client.GetDevices()) != 1 || client.GetDevices()[0].DID != "original" {
		t.Fatal("cloud failure discarded a working cached robot")
	}
	after, err := os.ReadFile(file)
	if err != nil || string(before) != string(after) {
		t.Fatal("cloud failure replaced the saved session")
	}
}

func TestStartupWithoutSavedSessionStillRequiresLogin(t *testing.T) {
	client := roborock.NewClient("http://example.invalid", "", "", "")
	client.SetSessionDir(t.TempDir())
	if restoreBridgeSession(client) {
		t.Fatal("missing saved session unexpectedly authenticated")
	}
}

func writeStartupSession(t *testing.T, directory, cloudURL string, devices []roborock.DeviceInfo) {
	t.Helper()
	session := roborock.SavedSession{
		LoginData: roborock.LoginData{Token: "existing-token", RRIoT: roborock.RRIoT{Remote: roborock.RRIoTRemote{APIURL: cloudURL}}},
		Devices:   devices,
	}
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "session.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
