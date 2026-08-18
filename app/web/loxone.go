package web

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mqtt-home/roborock-mqtt/config"
	loxonedirect "github.com/mqtt-home/roborock-mqtt/loxone/direct"
	"github.com/mqtt-home/roborock-mqtt/loxone/templates"
	"github.com/mqtt-home/roborock-mqtt/roborock"
)

const loxoneSubscriptionLimit = 16

type LoxoneMQTTTestStatus struct {
	OK       bool   `json:"ok"`
	Message  string `json:"message"`
	TestedAt int64  `json:"tested_at"`
}

// LoxoneDependencies are injected by main so HTTP handlers can use the same
// Phase 1/2 stores and MQTT connection as the bridge.
type LoxoneDependencies struct {
	Core              *roborock.LoxoneCoreStore
	Diagnostics       *roborock.LoxoneDiagnosticStore
	RoomOverrides     *roborock.LoxoneRoomOverrideStore
	Capabilities      *roborock.CapabilityStore
	PublishCommand    func(slug, command string) error
	TestMQTT          func(context.Context) error
	RefreshRoom       func(slug string)
	DirectDiagnostics func() loxonedirect.SyncDiagnostics
	ResendDirect      func()
	SubmitCommand     func(slug, command string) roborock.CommandSubmission
	FindCommand       func(id string) (slug string, activity roborock.LoxoneActivity, ok bool)
	testMu            sync.RWMutex
	lastMQTTTest      *LoxoneMQTTTestStatus
}

type loxoneIntegrationResponse struct {
	Project               string                        `json:"project"`
	Upstream              string                        `json:"upstream"`
	Enabled               bool                          `json:"enabled"`
	BridgeStarted         bool                          `json:"bridge_started"`
	Topic                 string                        `json:"topic"`
	SubscriptionLimit     int                           `json:"subscription_limit"`
	SubscriptionsPerRobot int                           `json:"subscriptions_per_robot"`
	SubscriptionsRequired int                           `json:"subscriptions_required"`
	ExceedsLimit          bool                          `json:"exceeds_limit"`
	Warning               string                        `json:"warning,omitempty"`
	MQTTTest              *LoxoneMQTTTestStatus         `json:"mqtt_test,omitempty"`
	DirectEnabled         bool                          `json:"direct_enabled"`
	DirectDiagnostics     *loxonedirect.SyncDiagnostics `json:"direct_diagnostics,omitempty"`
	Robots                []loxoneRobotResponse         `json:"robots"`
	Fleet                 *roborock.FleetHealth         `json:"fleet,omitempty"`
	TemplateStatus        templates.Status              `json:"template_status"`
}

type loxoneRobotResponse struct {
	Slug          string                      `json:"slug"`
	Name          string                      `json:"name"`
	Model         string                      `json:"model"`
	Online        bool                        `json:"online"`
	MQTTEnabled   bool                        `json:"mqtt_enabled"`
	DirectEnabled bool                        `json:"direct_enabled"`
	Core          roborock.LoxoneCore         `json:"core"`
	Topics        loxoneTopics                `json:"topics"`
	Rooms         []loxoneRoomResponse        `json:"rooms"`
	Scenes        []loxoneSceneResponse       `json:"scenes"`
	Diagnostics   roborock.LoxoneDiagnostics  `json:"diagnostics"`
	Capabilities  roborock.DeviceCapabilities `json:"capabilities"`
	Health        roborock.DeviceHealth       `json:"health"`
}

type loxoneTopics struct {
	Core        string `json:"core"`
	Activity    string `json:"activity"`
	Command     string `json:"command"`
	LastCommand string `json:"last_command"`
}

type loxoneRoomResponse struct {
	ID            int    `json:"id"`
	RoborockName  string `json:"roborock_name"`
	ConfigName    string `json:"config_name,omitempty"`
	OverrideName  string `json:"override_name,omitempty"`
	EffectiveName string `json:"effective_name"`
	Conflict      bool   `json:"conflict"`
	Command       string `json:"command"`
}

type loxoneSceneResponse struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
}

func (ws *WebServer) loxoneIntegration(w http.ResponseWriter, _ *http.Request) {
	response, err := ws.buildLoxoneIntegration()
	if err != nil {
		writeLoxoneError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeLoxoneJSON(w, http.StatusOK, response)
}

func (ws *WebServer) buildLoxoneIntegration() (loxoneIntegrationResponse, error) {
	cfg := config.Get()
	response := loxoneIntegrationResponse{
		Project:               "roborock-mqtt-loxone",
		Upstream:              "mqtt-home/roborock-mqtt",
		Enabled:               cfg.Loxone.Enabled,
		DirectEnabled:         cfg.Loxone.Direct.Enabled,
		BridgeStarted:         ws.deviceManager != nil,
		Topic:                 cfg.Loxone.Topic,
		SubscriptionLimit:     loxoneSubscriptionLimit,
		SubscriptionsPerRobot: 2,
		Robots:                []loxoneRobotResponse{},
		TemplateStatus:        templates.StatusForCurrentBuild(),
	}
	dependencies := ws.getLoxoneIntegration()
	if dependencies != nil {
		dependencies.testMu.RLock()
		response.MQTTTest = dependencies.lastMQTTTest
		dependencies.testMu.RUnlock()
		if dependencies.DirectDiagnostics != nil {
			diagnostics := dependencies.DirectDiagnostics()
			response.DirectDiagnostics = &diagnostics
		}
	}
	if ws.deviceManager == nil {
		return response, nil
	}
	fleet := ws.deviceManager.FleetHealth()
	response.Fleet = &fleet

	for _, device := range ws.deviceManager.GetDevices() {
		response.Robots = append(response.Robots, ws.buildLoxoneRobot(device))
	}
	mqttRobots := 0
	for _, robot := range response.Robots {
		if robot.MQTTEnabled {
			mqttRobots++
		}
	}
	response.SubscriptionsRequired = mqttRobots * response.SubscriptionsPerRobot
	response.ExceedsLimit = response.SubscriptionsRequired > response.SubscriptionLimit
	if response.ExceedsLimit {
		response.Warning = fmt.Sprintf("Standard configuration requires %d subscriptions and exceeds the Loxone limit of %d; export remains available.", response.SubscriptionsRequired, response.SubscriptionLimit)
	}
	return response, nil
}

func (ws *WebServer) loxoneDirectResend(w http.ResponseWriter, _ *http.Request) {
	dependencies := ws.getLoxoneIntegration()
	if dependencies == nil || dependencies.ResendDirect == nil {
		writeLoxoneError(w, http.StatusConflict, fmt.Errorf("Direct Loxone integration is not running"))
		return
	}
	dependencies.ResendDirect()
	writeLoxoneJSON(w, http.StatusAccepted, map[string]string{"state": "queued"})
}

func (ws *WebServer) buildLoxoneRobot(device *roborock.ManagedDevice) loxoneRobotResponse {
	base := strings.TrimSuffix(config.Get().Loxone.Topic, "/") + "/" + device.Slug
	mqttForDevice, directForDevice := config.Get().Loxone.DeviceModes(device.Info.DID, device.Slug)
	robot := loxoneRobotResponse{
		Slug:          device.Slug,
		Name:          device.Info.Name,
		Model:         device.Info.Model,
		Online:        device.CloudMQTT != nil && device.CloudMQTT.IsConnected(),
		MQTTEnabled:   config.Get().MQTT.IsEnabled() && mqttForDevice,
		DirectEnabled: config.Get().Loxone.Direct.Enabled && directForDevice,
		Health:        ws.deviceManager.DeviceHealth(device.Slug),
		Topics: loxoneTopics{
			Core:        base + "/core",
			Activity:    base + "/activity",
			Command:     base + "/command",
			LastCommand: base + "/last_command",
		},
		Rooms:  []loxoneRoomResponse{},
		Scenes: []loxoneSceneResponse{},
	}
	dependencies := ws.getLoxoneIntegration()
	if dependencies != nil {
		if dependencies.Core != nil {
			robot.Core = dependencies.Core.Snapshot(device.Slug)
		}
		if dependencies.Diagnostics != nil {
			robot.Diagnostics = dependencies.Diagnostics.Snapshot(device.Slug)
		}
		if dependencies.Capabilities != nil {
			robot.Capabilities = dependencies.Capabilities.Get(device.Slug)
		}
	}
	robot.Rooms = ws.loxoneRooms(device)
	for _, scene := range device.Scenes {
		robot.Scenes = append(robot.Scenes, loxoneSceneResponse{ID: scene.ID, Name: scene.Name, Command: fmt.Sprintf("scene_id:%d", scene.ID)})
	}
	sort.Slice(robot.Scenes, func(i, j int) bool {
		return strings.ToLower(robot.Scenes[i].Name) < strings.ToLower(robot.Scenes[j].Name)
	})
	return robot
}

func (ws *WebServer) loxoneRooms(device *roborock.ManagedDevice) []loxoneRoomResponse {
	mappings := device.GetRoomMappings()
	if len(mappings) == 0 {
		return []loxoneRoomResponse{}
	}
	apiNames := ws.restClient.GetRoomNameMap()
	configNames := config.Get().Roborock.RoomNames[device.Info.Name]
	overrides := map[string]string{}
	dependencies := ws.getLoxoneIntegration()
	if dependencies != nil && dependencies.RoomOverrides != nil {
		overrides = dependencies.RoomOverrides.ForDevice(device.Slug)
	}
	effective := roborock.CommandableRoomNames(mappings, apiNames, configNames, overrides)
	counts := make(map[string]int)
	for _, name := range effective {
		counts[strings.ToLower(strings.TrimSpace(name))]++
	}

	rooms := make([]loxoneRoomResponse, 0, len(mappings))
	seen := make(map[int]bool)
	for _, mapping := range mappings {
		if seen[mapping.SegmentID] {
			continue
		}
		seen[mapping.SegmentID] = true
		id := strconv.Itoa(mapping.SegmentID)
		roborockName := strings.TrimSpace(apiNames[mapping.HomeRoomID])
		if roborockName == "" {
			roborockName = fmt.Sprintf("Room %d", mapping.SegmentID)
		}
		effectiveName := strings.TrimSpace(effective[id])
		if effectiveName == "" {
			effectiveName = fmt.Sprintf("Room %d", mapping.SegmentID)
		}
		rooms = append(rooms, loxoneRoomResponse{
			ID:            mapping.SegmentID,
			RoborockName:  roborockName,
			ConfigName:    configNames[id],
			OverrideName:  overrides[id],
			EffectiveName: effectiveName,
			Conflict:      counts[strings.ToLower(effectiveName)] > 1,
			Command:       fmt.Sprintf("clean_room_id:%d", mapping.SegmentID),
		})
	}
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].ID < rooms[j].ID })
	return rooms
}

type loxoneRoomOverrideRequest struct {
	Name string `json:"name"`
}

func (ws *WebServer) loxoneRoomOverrideSave(w http.ResponseWriter, r *http.Request) {
	device, roomID, ok := ws.loxoneRoomTarget(w, r)
	if !ok {
		return
	}
	var request loxoneRoomOverrideRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
		writeLoxoneError(w, http.StatusBadRequest, fmt.Errorf("invalid request"))
		return
	}
	apiNames := ws.restClient.GetRoomNameMap()
	configNames := config.Get().Roborock.RoomNames[device.Info.Name]
	baseNames := roborock.CommandableRoomNames(device.GetRoomMappings(), apiNames, configNames, nil)
	dependencies := ws.getLoxoneIntegration()
	if err := dependencies.RoomOverrides.Set(device.Slug, roomID, request.Name, baseNames); err != nil {
		writeLoxoneError(w, http.StatusConflict, err)
		return
	}
	if dependencies.RefreshRoom != nil {
		dependencies.RefreshRoom(device.Slug)
	}
	writeLoxoneJSON(w, http.StatusOK, map[string]any{"status": "success", "rooms": ws.loxoneRooms(device)})
}

func (ws *WebServer) loxoneRoomOverrideDelete(w http.ResponseWriter, r *http.Request) {
	device, roomID, ok := ws.loxoneRoomTarget(w, r)
	if !ok {
		return
	}
	dependencies := ws.getLoxoneIntegration()
	if err := dependencies.RoomOverrides.Delete(device.Slug, roomID); err != nil {
		writeLoxoneError(w, http.StatusInternalServerError, err)
		return
	}
	if dependencies.RefreshRoom != nil {
		dependencies.RefreshRoom(device.Slug)
	}
	writeLoxoneJSON(w, http.StatusOK, map[string]any{"status": "success", "rooms": ws.loxoneRooms(device)})
}

func (ws *WebServer) loxoneRoomTarget(w http.ResponseWriter, r *http.Request) (*roborock.ManagedDevice, int, bool) {
	dependencies := ws.getLoxoneIntegration()
	if dependencies == nil || dependencies.RoomOverrides == nil {
		writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("Loxone integration unavailable"))
		return nil, 0, false
	}
	device := ws.getDeviceFromRequest(w, r)
	if device == nil {
		return nil, 0, false
	}
	roomID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || roomID <= 0 {
		writeLoxoneError(w, http.StatusBadRequest, fmt.Errorf("invalid room ID"))
		return nil, 0, false
	}
	found := false
	for _, room := range ws.loxoneRooms(device) {
		if room.ID == roomID {
			found = true
			break
		}
	}
	if !found {
		writeLoxoneError(w, http.StatusNotFound, fmt.Errorf("room not found on current map"))
		return nil, 0, false
	}
	return device, roomID, true
}

type loxoneCommandRequest struct {
	Command string `json:"command"`
}

func (ws *WebServer) loxoneCommandTest(w http.ResponseWriter, r *http.Request) {
	device := ws.getDeviceFromRequest(w, r)
	if device == nil {
		return
	}
	dependencies := ws.getLoxoneIntegration()
	if dependencies == nil || dependencies.PublishCommand == nil {
		writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("Loxone MQTT command publisher unavailable"))
		return
	}
	var request loxoneCommandRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
		writeLoxoneError(w, http.StatusBadRequest, fmt.Errorf("invalid request"))
		return
	}
	request.Command = strings.TrimSpace(request.Command)
	if request.Command == "" || len(request.Command) > 512 {
		writeLoxoneError(w, http.StatusBadRequest, fmt.Errorf("command is empty or too long"))
		return
	}
	if err := dependencies.PublishCommand(device.Slug, request.Command); err != nil {
		writeLoxoneError(w, http.StatusBadGateway, err)
		return
	}
	writeLoxoneJSON(w, http.StatusAccepted, map[string]string{"status": "published", "topic": strings.TrimSuffix(config.Get().Loxone.Topic, "/") + "/" + device.Slug + "/command"})
}

func (ws *WebServer) loxoneMQTTTest(w http.ResponseWriter, r *http.Request) {
	dependencies := ws.getLoxoneIntegration()
	if dependencies == nil || dependencies.TestMQTT == nil {
		writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("MQTT diagnostic unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	result := &LoxoneMQTTTestStatus{TestedAt: time.Now().Unix()}
	status := http.StatusOK
	if err := dependencies.TestMQTT(ctx); err != nil {
		result.Message = err.Error()
		status = http.StatusServiceUnavailable
	} else {
		result.OK = true
		result.Message = "MQTT publish/subscribe loopback succeeded"
	}
	dependencies.testMu.Lock()
	dependencies.lastMQTTTest = result
	dependencies.testMu.Unlock()
	writeLoxoneJSON(w, status, result)
}

type loxoneExportRequest struct {
	Robots []loxoneExportSelection `json:"robots"`
}

type loxoneExportSelection struct {
	Slug     string `json:"slug"`
	RoomIDs  []int  `json:"room_ids"`
	SceneIDs []int  `json:"scene_ids"`
}

type loxoneExportPack struct {
	Schema                string              `json:"schema"`
	Project               string              `json:"project"`
	Upstream              string              `json:"upstream"`
	GeneratedAt           string              `json:"generated_at"`
	LoxoneTopic           string              `json:"loxone_topic"`
	SubscriptionsPerRobot int                 `json:"subscriptions_per_robot"`
	SubscriptionsRequired int                 `json:"subscriptions_required"`
	SubscriptionLimit     int                 `json:"subscription_limit"`
	ExceedsLimit          bool                `json:"exceeds_limit"`
	Warning               string              `json:"warning,omitempty"`
	Robots                []loxoneExportRobot `json:"robots"`
	TemplateStatus        templates.Status    `json:"native_template_status"`
}

type loxoneExportRobot struct {
	Slug          string                `json:"slug"`
	Name          string                `json:"name"`
	MQTTEnabled   bool                  `json:"mqtt_enabled"`
	DirectEnabled bool                  `json:"direct_enabled"`
	Topics        loxoneTopics          `json:"topics"`
	Rooms         []loxoneRoomResponse  `json:"rooms"`
	Scenes        []loxoneSceneResponse `json:"scenes"`
	DirectInputs  []loxoneDirectInput   `json:"direct_inputs,omitempty"`
	DirectOutputs []loxoneDirectOutput  `json:"direct_outputs,omitempty"`
}

type loxoneDirectInput struct {
	Name  string `json:"name"`
	Field string `json:"field"`
	Kind  string `json:"kind"`
}

type loxoneDirectOutput struct {
	Name    string `json:"name"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Command string `json:"command"`
	Level   string `json:"level"`
}

func (ws *WebServer) loxoneExport(w http.ResponseWriter, r *http.Request) {
	var request loxoneExportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeLoxoneError(w, http.StatusBadRequest, fmt.Errorf("invalid export request"))
		return
	}
	pack, err := ws.buildLoxoneExport(request)
	if err != nil {
		writeLoxoneError(w, http.StatusBadRequest, err)
		return
	}
	data, err := createLoxoneExportZIP(pack)
	if err != nil {
		writeLoxoneError(w, http.StatusInternalServerError, err)
		return
	}
	filename := "roborock-mqtt-loxone-integration-" + time.Now().UTC().Format("20060102-150405") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (ws *WebServer) buildLoxoneExport(request loxoneExportRequest) (loxoneExportPack, error) {
	integration, err := ws.buildLoxoneIntegration()
	if err != nil {
		return loxoneExportPack{}, err
	}
	bySlug := make(map[string]loxoneRobotResponse, len(integration.Robots))
	for _, robot := range integration.Robots {
		bySlug[robot.Slug] = robot
	}
	if len(request.Robots) == 0 {
		for _, robot := range integration.Robots {
			selection := loxoneExportSelection{Slug: robot.Slug}
			for _, room := range robot.Rooms {
				selection.RoomIDs = append(selection.RoomIDs, room.ID)
			}
			for _, scene := range robot.Scenes {
				selection.SceneIDs = append(selection.SceneIDs, scene.ID)
			}
			request.Robots = append(request.Robots, selection)
		}
	}
	pack := loxoneExportPack{
		Schema:                "roborock-mqtt-loxone-integration/v2",
		Project:               "roborock-mqtt-loxone",
		Upstream:              "https://github.com/mqtt-home/roborock-mqtt",
		GeneratedAt:           time.Now().UTC().Format(time.RFC3339),
		LoxoneTopic:           integration.Topic,
		SubscriptionsPerRobot: 2,
		SubscriptionLimit:     loxoneSubscriptionLimit,
		Robots:                []loxoneExportRobot{},
		TemplateStatus:        templates.StatusForCurrentBuild(),
	}
	seen := make(map[string]bool)
	for _, selection := range request.Robots {
		robot, ok := bySlug[selection.Slug]
		if !ok {
			return loxoneExportPack{}, fmt.Errorf("unknown robot %q", selection.Slug)
		}
		if seen[selection.Slug] {
			return loxoneExportPack{}, fmt.Errorf("robot %q selected more than once", selection.Slug)
		}
		seen[selection.Slug] = true
		exported := loxoneExportRobot{Slug: robot.Slug, Name: robot.Name, MQTTEnabled: robot.MQTTEnabled, DirectEnabled: robot.DirectEnabled, Topics: robot.Topics, Rooms: []loxoneRoomResponse{}, Scenes: []loxoneSceneResponse{}}
		for _, id := range uniqueInts(selection.RoomIDs) {
			room, found := findLoxoneRoom(robot.Rooms, id)
			if !found {
				return loxoneExportPack{}, fmt.Errorf("unknown room %d for robot %q", id, selection.Slug)
			}
			exported.Rooms = append(exported.Rooms, room)
		}
		for _, id := range uniqueInts(selection.SceneIDs) {
			scene, found := findLoxoneScene(robot.Scenes, id)
			if !found {
				return loxoneExportPack{}, fmt.Errorf("unknown scene %d for robot %q", id, selection.Slug)
			}
			exported.Scenes = append(exported.Scenes, scene)
		}
		if exported.DirectEnabled {
			exported.DirectInputs, exported.DirectOutputs = ws.directExportPlan(robot, exported.Rooms, exported.Scenes)
		}
		pack.Robots = append(pack.Robots, exported)
	}
	mqttRobots := 0
	for _, robot := range pack.Robots {
		if robot.MQTTEnabled {
			mqttRobots++
		}
	}
	pack.SubscriptionsRequired = mqttRobots * pack.SubscriptionsPerRobot
	pack.ExceedsLimit = pack.SubscriptionsRequired > pack.SubscriptionLimit
	if pack.ExceedsLimit {
		pack.Warning = fmt.Sprintf("WARNING: the standard configuration needs %d MQTT subscriptions, exceeding Loxone's limit of %d. This pack was generated as requested; split the integration across MQTT plugins/Miniservers or reduce the selected robots.", pack.SubscriptionsRequired, pack.SubscriptionLimit)
	}
	return pack, nil
}

func (ws *WebServer) directExportPlan(robot loxoneRobotResponse, rooms []loxoneRoomResponse, scenes []loxoneSceneResponse) ([]loxoneDirectInput, []loxoneDirectOutput) {
	device := ws.deviceManager.GetDevice(robot.Slug)
	state := roborock.InternalDeviceState{Slug: robot.Slug, UpdatedAt: time.Now()}
	if device != nil {
		state.Status = device.GetStatus()
	}
	mapping := loxonedirect.InputMapping{Prefix: config.Get().Loxone.Direct.InputPrefix, Overrides: config.Get().Loxone.Direct.Inputs}
	values := loxonedirect.ValuesForState(state, mapping)
	inputs := make([]loxoneDirectInput, 0, len(values))
	for _, value := range values {
		inputs = append(inputs, loxoneDirectInput{Name: value.Input, Field: value.Field, Kind: string(value.Kind)})
	}
	base := "/api/loxone/direct/v1/devices/" + robot.Slug + "/commands/"
	outputs := []loxoneDirectOutput{
		{Name: "Start", Method: "POST", Path: base + "start", Command: "start", Level: "safe"},
		{Name: "Pause", Method: "POST", Path: base + "pause", Command: "pause", Level: "safe"},
		{Name: "Dock", Method: "POST", Path: base + "dock", Command: "dock", Level: "safe"},
	}
	for _, room := range rooms {
		outputs = append(outputs, loxoneDirectOutput{Name: "Clean " + room.EffectiveName, Method: "POST", Path: fmt.Sprintf("%srooms/%d", base, room.ID), Command: room.Command, Level: "safe"})
	}
	for _, scene := range scenes {
		outputs = append(outputs, loxoneDirectOutput{Name: "Scene " + scene.Name, Method: "POST", Path: fmt.Sprintf("%sscenes/%d", base, scene.ID), Command: scene.Command, Level: "safe"})
	}
	advanced := []struct {
		name, command string
		capability    roborock.Capability
	}{
		{"Stop", "stop", robot.Capabilities.Stop}, {"Locate", "locate", robot.Capabilities.Locate},
		{"Empty dustbin", "empty_dustbin", robot.Capabilities.DockEmpty}, {"Stop emptying", "stop_emptying", robot.Capabilities.DockEmpty},
		{"Wash mop", "wash_mop", robot.Capabilities.MopWash}, {"Stop washing", "stop_washing", robot.Capabilities.MopWash},
		{"Dry mop", "dry_mop", robot.Capabilities.MopDry}, {"Stop drying", "stop_drying", robot.Capabilities.MopDry},
	}
	for _, item := range advanced {
		if item.capability.Supported != nil && *item.capability.Supported {
			outputs = append(outputs, loxoneDirectOutput{Name: item.name, Method: "POST", Path: base + item.command, Command: item.command, Level: "advanced"})
		}
	}
	return inputs, outputs
}

func createLoxoneExportZIP(pack loxoneExportPack) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	integrationJSON, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := addZIPFile(writer, "integration.json", append(integrationJSON, '\n')); err != nil {
		return nil, err
	}
	if err := addZIPFile(writer, "topics.csv", loxoneTopicsCSV(pack)); err != nil {
		return nil, err
	}
	if err := addZIPFile(writer, "command-recognition.csv", loxoneRecognitionCSV(pack)); err != nil {
		return nil, err
	}
	if err := addZIPFile(writer, "direct-inputs.csv", loxoneDirectInputsCSV(pack)); err != nil {
		return nil, err
	}
	if err := addZIPFile(writer, "direct-outputs.csv", loxoneDirectOutputsCSV(pack)); err != nil {
		return nil, err
	}
	templateStatus, err := json.MarshalIndent(pack.TemplateStatus, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := addZIPFile(writer, "template-status.json", append(templateStatus, '\n')); err != nil {
		return nil, err
	}
	if err := addZIPFile(writer, "TEMPLATE-SAMPLES-NEEDED.md", []byte(loxoneTemplateRequirementsMarkdown(pack.TemplateStatus))); err != nil {
		return nil, err
	}
	if err := addZIPFile(writer, "SETUP.md", []byte(loxoneSetupMarkdown(pack))); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func loxoneDirectInputsCSV(pack loxoneExportPack) []byte {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	_ = writer.Write([]string{"Robot", "Virtual Input", "Field", "Type"})
	for _, robot := range pack.Robots {
		for _, input := range robot.DirectInputs {
			_ = writer.Write([]string{safeCSV(robot.Name), safeCSV(input.Name), input.Field, input.Kind})
		}
	}
	writer.Flush()
	return buffer.Bytes()
}

func loxoneDirectOutputsCSV(pack loxoneExportPack) []byte {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	_ = writer.Write([]string{"Robot", "Name", "Method", "Relative path", "Command", "Safety level"})
	for _, robot := range pack.Robots {
		for _, output := range robot.DirectOutputs {
			_ = writer.Write([]string{safeCSV(robot.Name), safeCSV(output.Name), output.Method, output.Path, output.Command, output.Level})
		}
	}
	writer.Flush()
	return buffer.Bytes()
}

func loxoneTemplateRequirementsMarkdown(status templates.Status) string {
	var builder strings.Builder
	builder.WriteString("# Native Loxone Config template generation\n\n")
	builder.WriteString("Native generation is deliberately disabled: " + status.Reason + ".\n\n")
	builder.WriteString("Export these minimal templates from the exact Loxone Config version used by the installation, without credentials or real IP addresses:\n\n")
	for _, sample := range status.RequiredSamples {
		builder.WriteString("- `" + string(sample) + "`\n")
	}
	builder.WriteString("\nThe files will be used as fixtures to validate element names, namespaces, identifiers, encodings and import behavior before a generator is enabled.\n")
	return builder.String()
}

func addZIPFile(writer *zip.Writer, name string, data []byte) error {
	file, err := writer.Create(name)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	return err
}

func loxoneTopicsCSV(pack loxoneExportPack) []byte {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	_ = writer.Write([]string{"Robot", "Direction", "Topic", "Retained", "Purpose"})
	for _, robot := range pack.Robots {
		if !robot.MQTTEnabled {
			continue
		}
		_ = writer.Write([]string{safeCSV(robot.Name), "Subscribe", robot.Topics.Core, "yes", "Compact current state"})
		_ = writer.Write([]string{safeCSV(robot.Name), "Subscribe", robot.Topics.Activity, "no", "Command progress and robot events"})
		_ = writer.Write([]string{safeCSV(robot.Name), "Publish", robot.Topics.Command, "no", "Text commands"})
	}
	writer.Flush()
	return buffer.Bytes()
}

func loxoneRecognitionCSV(pack loxoneExportPack) []byte {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	_ = writer.Write([]string{"Source", "Value", "Command recognition", "Notes"})
	for _, field := range []string{"online", "battery", "current_room_id", "error_code", "last_seen"} {
		_ = writer.Write([]string{"core", field, `\i"` + field + `":\i\v`, "Numeric value"})
	}
	for _, state := range []string{"offline", "idle", "cleaning", "paused", "returning", "charging", "docked", "washing_mop", "servicing_dock", "error"} {
		_ = writer.Write([]string{"core", "state=" + state, `"state":"` + state + `"`, "Text match"})
	}
	for _, event := range []string{"cleaning_started", "room_entered", "cleaning_completed", "returned_to_dock", "paused", "resumed", "error"} {
		_ = writer.Write([]string{"activity", "event=" + event, `"type":"event"\i"event":"` + event + `"`, "Event match"})
	}
	for _, robot := range pack.Robots {
		for _, room := range robot.Rooms {
			_ = writer.Write([]string{"core", "room=" + safeCSV(room.EffectiveName), fmt.Sprintf(`"current_room_id":%d`, room.ID), "Room ID match"})
		}
	}
	writer.Flush()
	return buffer.Bytes()
}

func loxoneSetupMarkdown(pack loxoneExportPack) string {
	var builder strings.Builder
	builder.WriteString("# roborock-mqtt-loxone integration\n\n")
	builder.WriteString("Generated by roborock-mqtt-loxone, a fork based on mqtt-home/roborock-mqtt.\n\n")
	builder.WriteString("This archive is a safe configuration assistant, not a native Loxone XML or .LoxPLAN file. Loxone does not publish a stable external schema for generating MQTT templates.\n\n")
	if pack.Warning != "" {
		builder.WriteString("## Subscription warning\n\n**" + pack.Warning + "**\n\n")
	}
	builder.WriteString(fmt.Sprintf("The selected MQTT configuration uses **%d subscriptions** (%d per MQTT-enabled robot). The documented Loxone limit is %d. Direct HTTP uses no MQTT subscriptions.\n\n", pack.SubscriptionsRequired, pack.SubscriptionsPerRobot, pack.SubscriptionLimit))
	builder.WriteString("## MQTT setup\n\nFor MQTT-enabled robots, add the two Subscribe objects and one non-retained Publish object from `topics.csv`, then use `command-recognition.csv`. Broker credentials are never included.\n\n")
	builder.WriteString("## Direct HTTP setup\n\nCreate Virtual Inputs from `direct-inputs.csv`. Create authenticated HTTP POST Virtual Outputs from `direct-outputs.csv`, prefixing each relative path with the bridge address. Enter the dedicated command API credentials manually in Loxone Config.\n\n")
	builder.WriteString("## Native template status\n\nNo XML is generated. See `TEMPLATE-SAMPLES-NEEDED.md`; native generation stays locked until real exports from the target Loxone Config version pass fixture and round-trip validation.\n\n")
	for _, robot := range pack.Robots {
		builder.WriteString("## " + robot.Name + " (`" + robot.Slug + "`)\n\n")
		if robot.MQTTEnabled {
			builder.WriteString("- Core: `" + robot.Topics.Core + "`\n- Activity: `" + robot.Topics.Activity + "`\n- Command: `" + robot.Topics.Command + "`\n")
		}
		if robot.DirectEnabled {
			builder.WriteString(fmt.Sprintf("- Direct plan: %d Virtual Inputs, %d Virtual Outputs\n", len(robot.DirectInputs), len(robot.DirectOutputs)))
		}
		for _, room := range robot.Rooms {
			builder.WriteString("- Room " + room.EffectiveName + ": `" + room.Command + "`\n")
		}
		for _, scene := range robot.Scenes {
			builder.WriteString("- Scene " + scene.Name + ": `" + scene.Command + "`\n")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("## Official references\n\n- https://www.loxone.com/enus/kb/mqtt/\n- https://www.loxone.com/enus/kb/command-recognition/\n- https://www.loxone.com/enus/kb/templates/\n")
	return builder.String()
}

func safeCSV(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "@") {
		return "'" + trimmed
	}
	return trimmed
}

func uniqueInts(values []int) []int {
	seen := make(map[int]bool)
	result := make([]int, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func findLoxoneRoom(rooms []loxoneRoomResponse, id int) (loxoneRoomResponse, bool) {
	for _, room := range rooms {
		if room.ID == id {
			return room, true
		}
	}
	return loxoneRoomResponse{}, false
}

func findLoxoneScene(scenes []loxoneSceneResponse, id int) (loxoneSceneResponse, bool) {
	for _, scene := range scenes {
		if scene.ID == id {
			return scene, true
		}
	}
	return loxoneSceneResponse{}, false
}

func writeLoxoneJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeLoxoneError(w http.ResponseWriter, status int, err error) {
	writeLoxoneJSON(w, status, map[string]string{"error": err.Error()})
}
