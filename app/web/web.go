package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/mqtt-home/roborock-mqtt/config"
	"github.com/mqtt-home/roborock-mqtt/roborock"
	"github.com/philipparndt/go-logger"
	loggerchi "github.com/philipparndt/go-logger/chi"
)

type SSEClient struct {
	ID      string
	Channel chan string
}

type WebServer struct {
	deviceManager  *roborock.DeviceManager
	restClient     *roborock.Client
	scheduleEngine *roborock.ScheduleEngine
	notAtHomeStore *roborock.NotAtHomeStore
	scheduleStore  *roborock.ScheduleStore
	loxone         *LoxoneDependencies
	loxoneMu       sync.RWMutex
	onAuth         func()
	router         *chi.Mux
	sseClients     map[string]*SSEClient
	sseClientsMu   sync.RWMutex
	// Liveness: timestamp at which the bridge first became unhealthy (not
	// authenticated, or no device connected). nil while healthy. The probe fails
	// only once this exceeds the grace window, so transient blips don't restart.
	unhealthySince *time.Time
	unhealthyMu    sync.Mutex
	directLimiter  directCommandLimiter
}

// livenessGrace returns the configured grace window before liveness fails,
// defaulting to 4 minutes when unset.
func (ws *WebServer) livenessGrace() time.Duration {
	if s := config.Get().Web.LivenessGraceSeconds; s > 0 {
		return time.Duration(s) * time.Second
	}
	return 4 * time.Minute
}

func NewWebServer(
	deviceManager *roborock.DeviceManager,
	restClient *roborock.Client,
	onAuth func(),
) *WebServer {
	ws := &WebServer{
		deviceManager: deviceManager,
		restClient:    restClient,
		onAuth:        onAuth,
		router:        chi.NewRouter(),
		sseClients:    make(map[string]*SSEClient),
		directLimiter: directCommandLimiter{windows: make(map[string]directRateWindow)},
	}

	ws.setupRoutes()
	return ws
}

// SetDeviceManager updates the device manager after authentication.
func (ws *WebServer) SetDeviceManager(dm *roborock.DeviceManager) {
	ws.deviceManager = dm
}

// SetScheduleEngine sets the schedule engine after bridge startup.
func (ws *WebServer) SetScheduleEngine(se *roborock.ScheduleEngine) {
	ws.scheduleEngine = se
}

// SetNotAtHomeStore sets the not-at-home store.
func (ws *WebServer) SetNotAtHomeStore(store *roborock.NotAtHomeStore) {
	ws.notAtHomeStore = store
}

// SetScheduleStore sets the schedule store for user-created schedules.
func (ws *WebServer) SetScheduleStore(store *roborock.ScheduleStore) {
	ws.scheduleStore = store
}

// SetLoxoneIntegration wires the optional Loxone UI to the already-running
// bridge without making the web package own the MQTT client.
func (ws *WebServer) SetLoxoneIntegration(dependencies *LoxoneDependencies) {
	ws.loxoneMu.Lock()
	defer ws.loxoneMu.Unlock()
	ws.loxone = dependencies
}

func (ws *WebServer) getLoxoneIntegration() *LoxoneDependencies {
	ws.loxoneMu.RLock()
	defer ws.loxoneMu.RUnlock()
	return ws.loxone
}

func (ws *WebServer) setupRoutes() {
	ws.router.Use(loggerchi.LoggerWithLevel(slog.LevelDebug))
	ws.router.Use(middleware.Recoverer)

	ws.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	ws.router.Route("/api", func(r chi.Router) {
		r.Get("/health", ws.healthCheck)
		r.Get("/livez", ws.liveness)

		// Auth endpoints
		r.Get("/auth/status", ws.authStatus)
		r.Post("/auth/request-code", ws.requestCode)
		r.Post("/auth/login", ws.authLogin)
		r.Post("/auth/logout", ws.authLogout)

		// Device endpoints
		r.Get("/devices", ws.getDevices)
		r.Get("/status", ws.getAllStatus)
		r.Route("/devices/{slug}", func(r chi.Router) {
			r.Get("/status", ws.getDeviceStatus)
			r.Post("/start", ws.deviceCommand("start"))
			r.Post("/pause", ws.deviceCommand("pause"))
			r.Post("/dock", ws.deviceCommand("dock"))
			r.Post("/fan-speed", ws.deviceFanSpeed)
			r.Post("/mop-mode", ws.deviceMopMode)
			r.Get("/map", ws.deviceMap)
			r.Get("/map.json", ws.deviceMapJSON)
			r.Get("/scenes", ws.deviceScenes)
			r.Post("/scenes/{id}/execute", ws.executeScene)
			r.Get("/schedule", ws.deviceSchedule)
			r.Post("/schedule", ws.deviceScheduleSave)
			r.Delete("/schedule", ws.deviceScheduleDelete)
			r.Post("/consumables/{name}/reset", ws.resetConsumable)
		})

		r.Get("/schedule/status", ws.scheduleStatus)
		r.Put("/not-at-home", ws.notAtHome)
		r.Get("/events", ws.handleSSE)

		r.Route("/loxone", func(r chi.Router) {
			r.Get("/integration", ws.loxoneIntegration)
			r.Put("/devices/{slug}/rooms/{id}", ws.loxoneRoomOverrideSave)
			r.Delete("/devices/{slug}/rooms/{id}", ws.loxoneRoomOverrideDelete)
			r.Post("/devices/{slug}/command", ws.loxoneCommandTest)
			r.Post("/mqtt-test", ws.loxoneMQTTTest)
			r.Post("/direct/resend", ws.loxoneDirectResend)
			r.Post("/direct/v1/devices/{slug}/commands", ws.directAPI(ws.loxoneDirectCanonicalCommand))
			r.Get("/direct/v1/commands/{id}", ws.directAPI(ws.loxoneDirectCommandStatus))
			for _, method := range []string{http.MethodPost, http.MethodGet} {
				r.MethodFunc(method, "/direct/v1/devices/{slug}/commands/{action}", ws.directAPI(ws.loxoneDirectSimpleCommand))
				r.MethodFunc(method, "/direct/v1/devices/{slug}/commands/rooms/{value}", ws.directAPI(ws.loxoneDirectPathCommand("room")))
				r.MethodFunc(method, "/direct/v1/devices/{slug}/commands/scenes/{value}", ws.directAPI(ws.loxoneDirectPathCommand("scene")))
				r.MethodFunc(method, "/direct/v1/devices/{slug}/commands/fan/{value}", ws.directAPI(ws.loxoneDirectPathCommand("fan")))
				r.MethodFunc(method, "/direct/v1/devices/{slug}/commands/mop/{value}", ws.directAPI(ws.loxoneDirectPathCommand("mop")))
				r.MethodFunc(method, "/direct/v1/devices/{slug}/commands/water/{value}", ws.directAPI(ws.loxoneDirectPathCommand("water")))
			}
			r.Post("/export", ws.loxoneExport)
		})
	})

	// SPA fallback: serve static files, fall back to index.html for client-side routes
	distDir := "./web/dist/"
	fileServer := http.FileServer(http.Dir(distDir))
	ws.router.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the static file
		path := "." + r.URL.Path
		if _, err := http.Dir(distDir).Open(path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Fall back to index.html for client-side routing
		http.ServeFile(w, r, distDir+"index.html")
	})
}

// --- Auth endpoints ---

func (ws *WebServer) authStatus(w http.ResponseWriter, _ *http.Request) {
	authenticated := ws.restClient.IsAuthenticated()
	resp := map[string]any{
		"authenticated": authenticated,
	}
	if authenticated {
		resp["user"] = ws.restClient.GetLoginData().Nickname
		resp["devices"] = len(ws.restClient.GetDevices())
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (ws *WebServer) requestCode(w http.ResponseWriter, _ *http.Request) {
	if err := ws.restClient.RequestCode(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	ws.jsonOK(w)
}

type CodeLoginRequest struct {
	Code string `json:"code"`
}

func (ws *WebServer) authLogin(w http.ResponseWriter, r *http.Request) {
	var req CodeLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		http.Error(w, `{"error":"code is required"}`, http.StatusBadRequest)
		return
	}

	if err := ws.restClient.CodeLogin(req.Code); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Save login data immediately
	if err := ws.restClient.SaveSession(); err != nil {
		logger.Warn("Failed to save session after login", "error", err)
	}

	if err := ws.restClient.DiscoverDevice(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Login succeeded but failed to discover devices: " + err.Error()})
		return
	}

	// Save again with devices
	if err := ws.restClient.SaveSession(); err != nil {
		logger.Warn("Failed to save session after discovery", "error", err)
	}

	// Start the bridge (synchronous so devices are ready for the response)
	if ws.onAuth != nil {
		ws.onAuth()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "success",
		"user":    ws.restClient.GetLoginData().Nickname,
		"devices": len(ws.restClient.GetDevices()),
	})
}

func (ws *WebServer) authLogout(w http.ResponseWriter, _ *http.Request) {
	ws.restClient.ClearSession()
	ws.jsonOK(w)
}

// --- Device endpoints ---

func (ws *WebServer) getDevices(w http.ResponseWriter, _ *http.Request) {
	if ws.deviceManager == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ws.deviceManager.GetSummaries())
}

func (ws *WebServer) getAllStatus(w http.ResponseWriter, _ *http.Request) {
	if ws.deviceManager == nil {
		http.Error(w, "Not connected", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ws.deviceManager.GetSummaries())
}

func (ws *WebServer) getDeviceStatus(w http.ResponseWriter, r *http.Request) {
	dev := ws.getDeviceFromRequest(w, r)
	if dev == nil {
		return
	}
	status := dev.GetStatus()
	if status == nil {
		http.Error(w, "No status available", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (ws *WebServer) deviceCommand(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dev := ws.getDeviceFromRequest(w, r)
		if dev == nil || dev.CloudMQTT == nil {
			return
		}

		var err error
		switch action {
		case "start":
			err = dev.CloudMQTT.Start()
		case "pause":
			err = dev.CloudMQTT.Pause()
		case "dock":
			err = dev.CloudMQTT.Dock()
		}

		if err != nil {
			logger.Error("Command failed", "device", dev.Slug, "action", action, "error", err)
		}
		ws.jsonOK(w)
	}
}

type FanSpeedRequest struct {
	Speed string `json:"speed"`
}

func (ws *WebServer) deviceFanSpeed(w http.ResponseWriter, r *http.Request) {
	dev := ws.getDeviceFromRequest(w, r)
	if dev == nil || dev.CloudMQTT == nil {
		return
	}
	var req FanSpeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	go func() {
		if err := dev.CloudMQTT.SetFanSpeed(req.Speed); err != nil {
			logger.Error("Failed to set fan speed", "device", dev.Slug, "error", err)
		}
	}()
	ws.jsonOK(w)
}

type MopModeRequest struct {
	Mode string `json:"mode"`
}

func (ws *WebServer) deviceMopMode(w http.ResponseWriter, r *http.Request) {
	dev := ws.getDeviceFromRequest(w, r)
	if dev == nil || dev.CloudMQTT == nil {
		return
	}
	var req MopModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	go func() {
		if err := dev.CloudMQTT.SetMopMode(req.Mode); err != nil {
			logger.Error("Failed to set mop mode", "device", dev.Slug, "error", err)
		}
	}()
	ws.jsonOK(w)
}

func (ws *WebServer) deviceScenes(w http.ResponseWriter, r *http.Request) {
	dev := ws.getDeviceFromRequest(w, r)
	if dev == nil {
		return
	}

	// Enrich each scene with the recorded run duration (if any) so the UI can
	// show how long the program takes before it is started.
	type sceneResp struct {
		ID              int    `json:"id"`
		Name            string `json:"name"`
		RecordedMinutes *int   `json:"recorded_minutes,omitempty"`
	}
	resp := make([]sceneResp, 0, len(dev.Scenes))
	for _, s := range dev.Scenes {
		item := sceneResp{ID: s.ID, Name: s.Name}
		if m := ws.deviceManager.SceneRecordedMinutes(dev.Slug, s.ID); m > 0 {
			item.RecordedMinutes = &m
		}
		resp = append(resp, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (ws *WebServer) executeScene(w http.ResponseWriter, r *http.Request) {
	if ws.deviceManager == nil {
		http.Error(w, `{"error":"not connected"}`, http.StatusServiceUnavailable)
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid scene id"}`, http.StatusBadRequest)
		return
	}
	// Tag the upcoming run on this device with the scene id so its duration is
	// recorded under the same program key as the MQTT/schedule paths.
	ws.deviceManager.NoteSceneStarted(chi.URLParam(r, "slug"), id)
	if err := ws.deviceManager.ExecuteScene(id); err != nil {
		logger.Error("Failed to execute scene", "id", id, "error", err)
		http.Error(w, `{"error":"failed to execute scene"}`, http.StatusInternalServerError)
		return
	}
	ws.jsonOK(w)
}

func (ws *WebServer) deviceMapJSON(w http.ResponseWriter, r *http.Request) {
	dev := ws.getDeviceFromRequest(w, r)
	if dev == nil {
		return
	}
	data := dev.GetVectorMapJSON()
	if data == nil {
		http.Error(w, "No map available", http.StatusNotFound)
		return
	}

	// Inject names keyed by the active map's commandable segment IDs. The cloud
	// room IDs are joined through get_room_mapping and are never exposed as
	// segment IDs.
	apiNames := ws.restClient.GetRoomNameMap()
	cfg := config.Get()
	cfgNames := cfg.Roborock.RoomNames[dev.Info.Name]
	roomNames := roborock.CommandableRoomNames(dev.GetRoomMappings(), apiNames, cfgNames, nil)
	if len(roomNames) > 0 {
		var vm roborock.VectorMap
		if err := json.Unmarshal(data, &vm); err == nil {
			vm.RoomNames = roomNames
			if enriched, err := json.Marshal(vm); err == nil {
				data = enriched
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

func (ws *WebServer) deviceMap(w http.ResponseWriter, r *http.Request) {
	dev := ws.getDeviceFromRequest(w, r)
	if dev == nil {
		return
	}
	mapPNG := dev.GetMapPNG()
	if mapPNG == nil {
		http.Error(w, "No map available", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(mapPNG)
}

func (ws *WebServer) getDeviceFromRequest(w http.ResponseWriter, r *http.Request) *roborock.ManagedDevice {
	if ws.deviceManager == nil {
		http.Error(w, `{"error":"not connected"}`, http.StatusServiceUnavailable)
		return nil
	}
	slug := chi.URLParam(r, "slug")
	dev := ws.deviceManager.GetDevice(slug)
	if dev == nil {
		http.Error(w, `{"error":"device not found"}`, http.StatusNotFound)
		return nil
	}
	return dev
}

// healthCheck is the always-200 human/diagnostic endpoint. It reports auth and
// per-device connection state in the body but never fails — use /livez for the
// k8s liveness probe.
func (ws *WebServer) healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":        "ok",
		"goroutines":    runtime.NumGoroutine(),
		"authenticated": ws.restClient.IsAuthenticated(),
		"devices":       ws.connectionStates(),
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	})
}

// connectionStates returns each device's slug → live connection state, or an
// empty map before the bridge has started.
func (ws *WebServer) connectionStates() map[string]bool {
	if ws.deviceManager == nil {
		return map[string]bool{}
	}
	return ws.deviceManager.ConnectionStates()
}

// liveness is the k8s liveness probe. It returns 200 while the bridge is healthy
// (authenticated AND at least one device connected) or has been unhealthy for
// less than the grace window; it returns 503 once the bridge has been
// unrecoverably stuck for longer than the grace window, prompting a pod restart
// — the proven recovery for an expired cloud session. Transient cloud blips stay
// within the grace window and do not trigger a restart loop.
func (ws *WebServer) liveness(w http.ResponseWriter, _ *http.Request) {
	authenticated := ws.restClient.IsAuthenticated()
	connected := ws.deviceManager != nil && ws.deviceManager.ConnectedCount() > 0
	healthy := authenticated && connected

	now := time.Now()
	grace := ws.livenessGrace()

	ws.unhealthyMu.Lock()
	statusCode, newSince, stuckFor := evaluateLiveness(healthy, ws.unhealthySince, now, grace)
	ws.unhealthySince = newSince
	ws.unhealthyMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]any{
		"healthy":         healthy,
		"authenticated":   authenticated,
		"devicesOnline":   ws.deviceManagerConnectedCount(),
		"stuckForSeconds": int(stuckFor.Seconds()),
		"graceSeconds":    int(grace.Seconds()),
		"timestamp":       now.UTC().Format(time.RFC3339),
	})
}

// evaluateLiveness is the pure liveness decision. Given the current health, the
// timestamp the bridge first became unhealthy (nil while healthy), the current
// time and the grace window, it returns the HTTP status, the updated
// unhealthy-since timestamp (cleared when healthy), and how long it has been
// stuck. The probe fails (503) only once the bridge has been continuously
// unhealthy for longer than the grace window.
func evaluateLiveness(healthy bool, unhealthySince *time.Time, now time.Time, grace time.Duration) (statusCode int, newSince *time.Time, stuckFor time.Duration) {
	if healthy {
		return http.StatusOK, nil, 0
	}
	if unhealthySince == nil {
		t := now
		unhealthySince = &t
	}
	stuckFor = now.Sub(*unhealthySince)
	statusCode = http.StatusOK
	if stuckFor > grace {
		statusCode = http.StatusServiceUnavailable
	}
	return statusCode, unhealthySince, stuckFor
}

func (ws *WebServer) deviceManagerConnectedCount() int {
	if ws.deviceManager == nil {
		return 0
	}
	return ws.deviceManager.ConnectedCount()
}

func (ws *WebServer) resetConsumable(w http.ResponseWriter, r *http.Request) {
	dev := ws.getDeviceFromRequest(w, r)
	if dev == nil || dev.CloudMQTT == nil {
		return
	}
	name := chi.URLParam(r, "name")
	if _, ok := roborock.ConsumableFieldNames[name]; !ok {
		http.Error(w, `{"error":"unknown consumable"}`, http.StatusBadRequest)
		return
	}
	if err := dev.CloudMQTT.ResetConsumable(name); err != nil {
		logger.Error("Failed to reset consumable", "device", dev.Slug, "consumable", name, "error", err)
		http.Error(w, `{"error":"reset failed"}`, http.StatusInternalServerError)
		return
	}
	logger.Info("Consumable reset", "device", dev.Slug, "consumable", name)

	// Re-poll consumables to get updated values
	consumables, err := dev.CloudMQTT.PollConsumables()
	if err != nil {
		logger.Debug("Failed to re-poll consumables after reset", "error", err)
		ws.jsonOK(w)
		return
	}
	percents := roborock.ComputeConsumablePercents(consumables)

	// Update the device's stored status and broadcast via SSE
	if current := dev.GetStatus(); current != nil {
		updated := *current
		updated.Consumables = *consumables
		updated.ConsumablePercents = percents
		dev.SetStatus(&updated)
		ws.BroadcastDeviceStatus(dev.Slug, &updated)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":              "success",
		"consumables":         consumables,
		"consumable_percents": percents,
	})
}

func (ws *WebServer) jsonOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// --- Schedule endpoints ---

func (ws *WebServer) deviceSchedule(w http.ResponseWriter, r *http.Request) {
	dev := ws.getDeviceFromRequest(w, r)
	if dev == nil {
		return
	}
	if ws.scheduleEngine == nil || !ws.scheduleEngine.HasAnyScheduleForSlug(dev.Slug) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"configured": false, "source": "none"})
		return
	}
	state := ws.scheduleEngine.GetScheduleStateForSlug(dev.Slug)
	sched := ws.scheduleEngine.GetDeviceScheduleForSlug(dev.Slug)
	source := "none"
	if state != nil {
		source = string(state.Source)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"configured": true,
		"source":     source,
		"state":      state,
		"schedule":   sched,
	})
}

func (ws *WebServer) deviceScheduleSave(w http.ResponseWriter, r *http.Request) {
	dev := ws.getDeviceFromRequest(w, r)
	if dev == nil {
		return
	}
	if ws.scheduleStore == nil || ws.scheduleEngine == nil {
		http.Error(w, `{"error":"schedules not configured"}`, http.StatusServiceUnavailable)
		return
	}
	var sched config.DeviceSchedule
	if err := json.NewDecoder(r.Body).Decode(&sched); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if err := ws.scheduleStore.Save(dev.Info.Name, sched); err != nil {
		logger.Error("Failed to save user schedule", "device", dev.Slug, "error", err)
		http.Error(w, `{"error":"failed to save schedule"}`, http.StatusInternalServerError)
		return
	}
	ws.scheduleEngine.RebuildSchedules()
	state := ws.scheduleEngine.GetScheduleStateForSlug(dev.Slug)
	if state != nil {
		ws.BroadcastScheduleState(dev.Slug, state)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "success",
		"source":   "user",
		"state":    state,
		"schedule": &sched,
	})
}

func (ws *WebServer) deviceScheduleDelete(w http.ResponseWriter, r *http.Request) {
	dev := ws.getDeviceFromRequest(w, r)
	if dev == nil {
		return
	}
	if ws.scheduleStore == nil || ws.scheduleEngine == nil {
		http.Error(w, `{"error":"schedules not configured"}`, http.StatusServiceUnavailable)
		return
	}
	if err := ws.scheduleStore.Delete(dev.Info.Name); err != nil {
		http.Error(w, `{"error":"no user schedule to delete"}`, http.StatusNotFound)
		return
	}
	ws.scheduleEngine.RebuildSchedules()
	state := ws.scheduleEngine.GetScheduleStateForSlug(dev.Slug)
	if state != nil {
		ws.BroadcastScheduleState(dev.Slug, state)
	}
	source := "none"
	if state != nil {
		source = string(state.Source)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "success",
		"source": source,
		"state":  state,
	})
}

func (ws *WebServer) scheduleStatus(w http.ResponseWriter, _ *http.Request) {
	if ws.scheduleEngine == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"devices": []any{}})
		return
	}
	states := ws.scheduleEngine.GetAllScheduleStates()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"devices": states})
}

type NotAtHomeRequest struct {
	Enabled bool `json:"enabled"`
}

func (ws *WebServer) notAtHome(w http.ResponseWriter, r *http.Request) {
	if ws.notAtHomeStore == nil || ws.scheduleEngine == nil {
		http.Error(w, `{"error":"schedules not configured"}`, http.StatusServiceUnavailable)
		return
	}
	var req NotAtHomeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	ws.notAtHomeStore.Set(req.Enabled)
	logger.Info("Not-at-home toggled", "enabled", req.Enabled)

	// Broadcast updated state for all devices with schedules
	for _, state := range ws.scheduleEngine.GetAllScheduleStates() {
		ws.BroadcastScheduleState(state.Device, state)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":      "success",
		"not_at_home": req.Enabled,
	})
}

// --- SSE ---

// BroadcastScheduleState sends a schedule state update to all SSE clients.
func (ws *WebServer) BroadcastScheduleState(slug string, state *roborock.ScheduleState) {
	payload := struct {
		Type   string                  `json:"type"`
		Device string                  `json:"device"`
		State  *roborock.ScheduleState `json:"state"`
	}{
		Type:   "schedule",
		Device: slug,
		State:  state,
	}

	message, err := json.Marshal(payload)
	if err != nil {
		return
	}
	messageStr := string(message)

	ws.sseClientsMu.RLock()
	for _, client := range ws.sseClients {
		select {
		case client.Channel <- messageStr:
		default:
		}
	}
	ws.sseClientsMu.RUnlock()
}

// BroadcastDeviceStatus sends a per-device status update to all SSE clients.
func (ws *WebServer) BroadcastDeviceStatus(slug string, status *roborock.PublishedStatus) {
	payload := struct {
		Device string `json:"device"`
		*roborock.PublishedStatus
	}{
		Device:          slug,
		PublishedStatus: status,
	}

	message, err := json.Marshal(payload)
	if err != nil {
		return
	}
	messageStr := string(message)

	ws.sseClientsMu.RLock()
	for _, client := range ws.sseClients {
		select {
		case client.Channel <- messageStr:
		default:
		}
	}
	ws.sseClientsMu.RUnlock()
}

// BroadcastDeviceAvailability sends a per-device online/offline transition to all
// SSE clients so the web UI's connection indicator updates live.
func (ws *WebServer) BroadcastDeviceAvailability(slug string, online bool) {
	payload := struct {
		Type   string `json:"type"`
		Device string `json:"device"`
		Online bool   `json:"online"`
	}{
		Type:   "availability",
		Device: slug,
		Online: online,
	}

	message, err := json.Marshal(payload)
	if err != nil {
		return
	}
	messageStr := string(message)

	ws.sseClientsMu.RLock()
	for _, client := range ws.sseClients {
		select {
		case client.Channel <- messageStr:
		default:
		}
	}
	ws.sseClientsMu.RUnlock()
}

// BroadcastLoxoneActivity forwards the Phase 2 stream to the diagnostic page.
func (ws *WebServer) BroadcastLoxoneActivity(slug string, activity roborock.LoxoneActivity) {
	payload := struct {
		Type     string                  `json:"type"`
		Device   string                  `json:"device"`
		Activity roborock.LoxoneActivity `json:"activity"`
	}{Type: "loxone_activity", Device: slug, Activity: activity}
	message, err := json.Marshal(payload)
	if err != nil {
		return
	}
	ws.broadcastSSE(string(message))
}

func (ws *WebServer) broadcastSSE(message string) {
	ws.sseClientsMu.RLock()
	defer ws.sseClientsMu.RUnlock()
	for _, client := range ws.sseClients {
		select {
		case client.Channel <- message:
		default:
		}
	}
}

func (ws *WebServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	clientID := fmt.Sprintf("%d", time.Now().UnixNano())
	channel := make(chan string, 10)

	ws.sseClientsMu.Lock()
	ws.sseClients[clientID] = &SSEClient{ID: clientID, Channel: channel}
	ws.sseClientsMu.Unlock()

	// Send initial state for all devices
	if ws.deviceManager != nil {
		for _, summary := range ws.deviceManager.GetSummaries() {
			// Current connection state, so the indicator is correct on first paint.
			avail := struct {
				Type   string `json:"type"`
				Device string `json:"device"`
				Online bool   `json:"online"`
			}{Type: "availability", Device: summary.Slug, Online: summary.Online}
			availMsg, _ := json.Marshal(avail)
			fmt.Fprintf(w, "data: %s\n\n", string(availMsg))

			if summary.Status != nil {
				payload := struct {
					Device string `json:"device"`
					*roborock.PublishedStatus
				}{Device: summary.Slug, PublishedStatus: summary.Status}
				msg, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", string(msg))
			}
		}
	}

	// Send initial schedule state
	if ws.scheduleEngine != nil {
		for _, state := range ws.scheduleEngine.GetAllScheduleStates() {
			payload := struct {
				Type   string                  `json:"type"`
				Device string                  `json:"device"`
				State  *roborock.ScheduleState `json:"state"`
			}{Type: "schedule", Device: state.Device, State: state}
			msg, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", string(msg))
		}
	}

	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}

	defer func() {
		ws.sseClientsMu.Lock()
		delete(ws.sseClients, clientID)
		close(channel)
		ws.sseClientsMu.Unlock()
	}()

	for {
		select {
		case msg := <-channel:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
				return
			}
			if ok {
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (ws *WebServer) Start(port int) error {
	addr := ":" + strconv.Itoa(port)
	logger.Info("Starting web server", "address", addr)
	return http.ListenAndServe(addr, ws.router)
}
