package web

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	appconfig "github.com/mqtt-home/roborock-mqtt/config"
	"github.com/mqtt-home/roborock-mqtt/loxone/templates"
	"github.com/mqtt-home/roborock-mqtt/roborock"
)

func TestCreateLoxoneExportZIPContainsSafeDocumentedPack(t *testing.T) {
	pack := loxoneExportPack{
		Schema:                "roborock-mqtt-loxone-integration/v1",
		Project:               "roborock-mqtt-loxone",
		Upstream:              "https://github.com/mqtt-home/roborock-mqtt",
		GeneratedAt:           "2026-08-18T10:00:00Z",
		LoxoneTopic:           "loxone/roborock",
		SubscriptionsPerRobot: 2,
		SubscriptionsRequired: 18,
		SubscriptionLimit:     16,
		ExceedsLimit:          true,
		Warning:               "WARNING: 18 exceeds 16",
		TemplateStatus:        templates.StatusForCurrentBuild(),
		Robots: []loxoneExportRobot{{
			Slug: "vacuum", MQTTEnabled: true,
			Name: "=Formula Robot",
			Topics: loxoneTopics{
				Core: "loxone/roborock/vacuum/core", Activity: "loxone/roborock/vacuum/activity", Command: "loxone/roborock/vacuum/command",
			},
			Rooms:  []loxoneRoomResponse{{ID: 23, EffectiveName: "Cuisine", Command: "clean_room_id:23"}},
			Scenes: []loxoneSceneResponse{{ID: 101, Name: "Dinner", Command: "scene_id:101"}},
		}},
	}
	data, err := createLoxoneExportZIP(pack)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string]string)
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name] = string(content)
	}
	for _, name := range []string{"integration.json", "topics.csv", "command-recognition.csv", "direct-inputs.csv", "direct-outputs.csv", "template-status.json", "TEMPLATE-SAMPLES-NEEDED.md", "SETUP.md"} {
		if _, ok := files[name]; !ok {
			t.Fatalf("missing %s", name)
		}
	}
	if !strings.Contains(files["SETUP.md"], "18 subscriptions") || !strings.Contains(files["SETUP.md"], "not a native Loxone XML") {
		t.Fatalf("setup lacks warning/safety explanation: %s", files["SETUP.md"])
	}
	records, err := csv.NewReader(strings.NewReader(files["topics.csv"])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) < 2 || records[1][0] != "'=Formula Robot" {
		t.Fatalf("CSV formula prefix was not neutralized: %+v", records)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(files["integration.json"]), &decoded); err != nil {
		t.Fatal(err)
	}
	serialized := files["integration.json"]
	for _, forbidden := range []string{"password", "username", "localKey", "token", "mqtt_url"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("export unexpectedly contains secret field %q", forbidden)
		}
	}
}

func TestTemplateStatusEndpointFailsClosed(t *testing.T) {
	server := NewWebServer(nil, roborock.NewClient("", "", "", ""), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/loxone/templates/status", nil)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"native_generation":false`) || !strings.Contains(response.Body.String(), "virtual_output_http_post") {
		t.Fatalf("unexpected template status: %d %s", response.Code, response.Body.String())
	}
}

func TestDirectExportPlanContainsNoCredentialsAndNoMQTTSubscriptions(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.json")
	data := []byte(`{
		"mqtt":{"enabled":false,"topic":"home/roborock"},
		"roborock":{"username":"test@example.com"},
		"loxone":{"direct":{"enabled":true,"host":"192.168.1.10","username":"secret-user","password":"secret-password","api_token":"secret-token"}},
		"web":{}
	}`)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := appconfig.LoadConfig(file); err != nil {
		t.Fatal(err)
	}
	client := roborock.NewClient("", "", "", "")
	manager := roborock.NewDeviceManager(&roborock.LoginData{}, []roborock.DeviceInfo{{Name: "Robot", DID: "did-1"}}, client, dir)
	server := NewWebServer(manager, client, nil)
	server.SetLoxoneIntegration(&LoxoneDependencies{Core: roborock.NewLoxoneCoreStore(), Diagnostics: roborock.NewLoxoneDiagnosticStore(5), Capabilities: roborock.NewCapabilityStore()})
	pack, err := server.buildLoxoneExport(loxoneExportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if pack.SubscriptionsRequired != 0 || len(pack.Robots) != 1 || len(pack.Robots[0].DirectInputs) < 15 || len(pack.Robots[0].DirectOutputs) != 3 {
		t.Fatalf("unexpected Direct plan: %+v", pack)
	}
	serialized, _ := json.Marshal(pack)
	for _, secret := range []string{"secret-user", "secret-password", "secret-token", "192.168.1.10"} {
		if strings.Contains(string(serialized), secret) {
			t.Fatalf("export leaked %q", secret)
		}
	}
}

func TestSafeCSV(t *testing.T) {
	for _, value := range []string{"=1+1", "+cmd", "-2", "@formula"} {
		if got := safeCSV(value); !strings.HasPrefix(got, "'") {
			t.Fatalf("safeCSV(%q) = %q", value, got)
		}
	}
	if got := safeCSV("Cuisine"); got != "Cuisine" {
		t.Fatalf("safe name changed to %q", got)
	}
}

func TestFleetHealthEndpointReportsOfflineEmptyFleet(t *testing.T) {
	rest := roborock.NewClient("http://example.invalid", "", "", "")
	manager := roborock.NewDeviceManager(nil, nil, rest, t.TempDir())
	server := NewWebServer(manager, rest, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/fleet/health", nil)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"health":"offline"`) {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body.String())
	}
}

func TestLoxoneCommandEndpointUsesInjectedPublisher(t *testing.T) {
	loadLoxoneTestConfig(t)
	manager := roborock.NewDeviceManager(&roborock.LoginData{}, []roborock.DeviceInfo{{Name: "Vacuum"}}, nil, t.TempDir())
	server := NewWebServer(manager, &roborock.Client{}, nil)
	var gotSlug, gotCommand string
	server.SetLoxoneIntegration(&LoxoneDependencies{PublishCommand: func(slug, command string) error {
		gotSlug, gotCommand = slug, command
		return nil
	}})

	request := httptest.NewRequest(http.MethodPost, "/api/loxone/devices/vacuum/command", strings.NewReader(`{"command":"dock"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("got status %d: %s", response.Code, response.Body.String())
	}
	if gotSlug != "vacuum" || gotCommand != "dock" {
		t.Fatalf("publisher got slug=%q command=%q", gotSlug, gotCommand)
	}
}

func TestDirectResendUsesInjectedSynchronizer(t *testing.T) {
	loadLoxoneTestConfig(t)
	server := NewWebServer(nil, &roborock.Client{}, nil)
	called := false
	server.SetLoxoneIntegration(&LoxoneDependencies{ResendDirect: func() { called = true }})
	request := httptest.NewRequest(http.MethodPost, "/api/loxone/direct/resend", nil)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !called {
		t.Fatalf("resend status=%d called=%v body=%s", response.Code, called, response.Body.String())
	}
}

func TestLoxoneRoomsExposeOnlyActiveCommandableSegments(t *testing.T) {
	loadLoxoneTestConfig(t)
	client := loxoneTestClientWithRooms(t, []roborock.RoomInfo{
		{ID: 23364799, Name: "Cuisine"},
		{ID: 23364800, Name: "Salon"},
		{ID: 23364801, Name: "Entrée"},
		{ID: 99999999, Name: "Ancienne pièce"},
	})
	manager := roborock.NewDeviceManager(&roborock.LoginData{}, []roborock.DeviceInfo{{Name: "Vacuum"}}, client, t.TempDir())
	device := manager.GetDevice("vacuum")
	device.SetRoomMappings([]roborock.RoomMapping{
		{SegmentID: 7, HomeRoomID: "23364799"},
		{SegmentID: 8, HomeRoomID: "23364800"},
		{SegmentID: 23, HomeRoomID: "23364801"},
	})
	// The geometry may still contain an old segment. It must remain usable by
	// current_room but must not enter the commandable inventory.
	device.VectorMapJSON = []byte(`{"rooms":[{"id":7},{"id":8},{"id":15},{"id":23},{"id":191}]}`)

	server := NewWebServer(manager, client, nil)
	rooms := server.loxoneRooms(device)
	if len(rooms) != 3 {
		t.Fatalf("got %d rooms: %+v", len(rooms), rooms)
	}
	wantIDs := []int{7, 8, 23}
	wantNames := []string{"Cuisine", "Salon", "Entrée"}
	for index, room := range rooms {
		if room.ID != wantIDs[index] || room.RoborockName != wantNames[index] || room.EffectiveName != wantNames[index] {
			t.Fatalf("room %d = %+v, want ID %d named %q", index, room, wantIDs[index], wantNames[index])
		}
		if room.Command != "clean_room_id:"+strconv.Itoa(room.ID) {
			t.Fatalf("wrong command for room %+v", room)
		}
	}
}

func TestLoxoneRoomConflictsOnlyUseCommandableSegments(t *testing.T) {
	loadLoxoneTestConfig(t)
	client := loxoneTestClientWithRooms(t, []roborock.RoomInfo{
		{ID: 23364799, Name: "Cuisine"},
		{ID: 99999999, Name: "Cuisine"}, // historical duplicate, not mapped
		{ID: 23364800, Name: "Salon"},
	})
	manager := roborock.NewDeviceManager(&roborock.LoginData{}, []roborock.DeviceInfo{{Name: "Vacuum"}}, client, t.TempDir())
	device := manager.GetDevice("vacuum")
	device.SetRoomMappings([]roborock.RoomMapping{{SegmentID: 7, HomeRoomID: "23364799"}, {SegmentID: 8, HomeRoomID: "23364800"}})
	server := NewWebServer(manager, client, nil)
	for _, room := range server.loxoneRooms(device) {
		if room.Conflict {
			t.Fatalf("historical cloud room caused conflict: %+v", room)
		}
	}

	device.SetRoomMappings([]roborock.RoomMapping{{SegmentID: 7, HomeRoomID: "23364799"}, {SegmentID: 8, HomeRoomID: "99999999"}})
	rooms := server.loxoneRooms(device)
	if len(rooms) != 2 || !rooms[0].Conflict || !rooms[1].Conflict {
		t.Fatalf("commandable duplicate was not reported: %+v", rooms)
	}
}

func TestLoxoneRoomsFailClosedWithoutActiveMapping(t *testing.T) {
	loadLoxoneTestConfig(t)
	client := loxoneTestClientWithRooms(t, []roborock.RoomInfo{{ID: 23364799, Name: "Cuisine"}})
	manager := roborock.NewDeviceManager(&roborock.LoginData{}, []roborock.DeviceInfo{{Name: "Vacuum"}}, client, t.TempDir())
	device := manager.GetDevice("vacuum")
	device.VectorMapJSON = []byte(`{"rooms":[{"id":23},{"id":191}]}`)
	server := NewWebServer(manager, client, nil)
	if rooms := server.loxoneRooms(device); len(rooms) != 0 {
		t.Fatalf("geometry/cloud fallback exposed rooms: %+v", rooms)
	}
}

func TestLoxoneExportBeyondLimitWarnsButSucceeds(t *testing.T) {
	loadLoxoneTestConfig(t)
	devices := make([]roborock.DeviceInfo, 0, 9)
	for i := 1; i <= 9; i++ {
		devices = append(devices, roborock.DeviceInfo{Name: "Robot " + strconv.Itoa(i)})
	}
	manager := roborock.NewDeviceManager(&roborock.LoginData{}, devices, nil, t.TempDir())
	server := NewWebServer(manager, &roborock.Client{}, nil)
	server.SetLoxoneIntegration(&LoxoneDependencies{Core: roborock.NewLoxoneCoreStore(), Diagnostics: roborock.NewLoxoneDiagnosticStore(10)})
	request := httptest.NewRequest(http.MethodPost, "/api/loxone/export", strings.NewReader(`{"robots":[]}`))
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("export was blocked with status %d: %s", response.Code, response.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var integration []byte
	for _, file := range reader.File {
		if file.Name != "integration.json" {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		integration, err = io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	var pack loxoneExportPack
	if err := json.Unmarshal(integration, &pack); err != nil {
		t.Fatal(err)
	}
	if !pack.ExceedsLimit || pack.SubscriptionsRequired != 18 || pack.Warning == "" {
		t.Fatalf("missing over-limit warning: %+v", pack)
	}
}

func loadLoxoneTestConfig(t *testing.T) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{
		"mqtt":{"url":"tcp://localhost:1883","topic":"home/roborock"},
		"roborock":{"username":"test@example.com"},
		"loxone":{"enabled":true,"topic":"loxone/roborock"},
		"web":{"enabled":true}
	}`)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := appconfig.LoadConfig(file); err != nil {
		t.Fatal(err)
	}
}

func loxoneTestClientWithRooms(t *testing.T, rooms []roborock.RoomInfo) *roborock.Client {
	t.Helper()
	dir := t.TempDir()
	client := roborock.NewClient("", "", "", "")
	client.SetSessionDir(dir)
	data, err := json.Marshal(map[string]any{
		"login_data": roborock.LoginData{},
		"rooms":      rooms,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if !client.LoadSession() {
		t.Fatal("failed to load test room session")
	}
	return client
}
