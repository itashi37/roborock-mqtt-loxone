package roborock

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/philipparndt/go-logger"
)

// ManagedDevice represents a single device with its cloud MQTT connection and status.
type ManagedDevice struct {
	Info      DeviceInfo
	Slug      string
	CloudMQTT    *CloudMQTT
	Status       *PublishedStatus
	MapPNG       []byte
	VectorMapJSON []byte
	RoomMappings  []RoomMapping
	Scenes       []Scene
	pollCount    int
	mu           sync.RWMutex
}

func (md *ManagedDevice) GetStatus() *PublishedStatus {
	md.mu.RLock()
	defer md.mu.RUnlock()
	return md.Status
}

func (md *ManagedDevice) SetStatus(s *PublishedStatus) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.Status = s
}

func (md *ManagedDevice) GetVectorMapJSON() []byte {
	md.mu.RLock()
	defer md.mu.RUnlock()
	return md.VectorMapJSON
}

func (md *ManagedDevice) GetMapPNG() []byte {
	md.mu.RLock()
	defer md.mu.RUnlock()
	return md.MapPNG
}

func (md *ManagedDevice) SetMapPNG(data []byte) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.MapPNG = data
}

func (md *ManagedDevice) GetRoomMappings() []RoomMapping {
	md.mu.RLock()
	defer md.mu.RUnlock()
	return append([]RoomMapping(nil), md.RoomMappings...)
}

func (md *ManagedDevice) SetRoomMappings(mappings []RoomMapping) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.RoomMappings = append([]RoomMapping(nil), mappings...)
}

// DeviceManager manages multiple Roborock devices.
type DeviceManager struct {
	devices    []*ManagedDevice
	bySlug     map[string]*ManagedDevice
	loginData  *LoginData
	restClient *Client
	runTracker *RunTracker
	mu             sync.RWMutex
	onStatus       func(slug string, status *PublishedStatus)
	onMap          func(slug string, pngData []byte)
	onAvailability func(slug string, online bool)
}

// NewDeviceManager creates a manager for the given devices.
func NewDeviceManager(loginData *LoginData, devices []DeviceInfo, restClient *Client, dataDir string) *DeviceManager {
	dm := &DeviceManager{
		loginData:  loginData,
		restClient: restClient,
		runTracker: NewRunTracker(dataDir),
		bySlug:     make(map[string]*ManagedDevice),
	}

	usedSlugs := make(map[string]int)
	for _, dev := range devices {
		slug := Slugify(dev.Name)
		usedSlugs[slug]++
		if usedSlugs[slug] > 1 {
			slug = fmt.Sprintf("%s-%d", slug, usedSlugs[slug])
		}

		md := &ManagedDevice{
			Info: dev,
			Slug: slug,
		}
		dm.devices = append(dm.devices, md)
		dm.bySlug[slug] = md
	}

	return dm
}

// SetStatusCallback sets the function called when any device's status changes.
func (dm *DeviceManager) SetStatusCallback(cb func(slug string, status *PublishedStatus)) {
	dm.onStatus = cb
}

// SetMapCallback sets the function called when a device's map is updated.
func (dm *DeviceManager) SetMapCallback(cb func(slug string, pngData []byte)) {
	dm.onMap = cb
}

// SetAvailabilityCallback sets the function called when a device's cloud
// connection transitions online/offline.
func (dm *DeviceManager) SetAvailabilityCallback(cb func(slug string, online bool)) {
	dm.onAvailability = cb
}

// ConnectedCount returns how many devices currently have a live cloud connection.
func (dm *DeviceManager) ConnectedCount() int {
	n := 0
	for _, md := range dm.devices {
		if md.CloudMQTT != nil && md.CloudMQTT.IsConnected() {
			n++
		}
	}
	return n
}

// ConnectionStates returns each device's slug mapped to its live connection state.
func (dm *DeviceManager) ConnectionStates() map[string]bool {
	states := make(map[string]bool, len(dm.devices))
	for _, md := range dm.devices {
		states[md.Slug] = md.CloudMQTT != nil && md.CloudMQTT.IsConnected()
	}
	return states
}

// ConnectAll establishes cloud MQTT connections for all devices.
func (dm *DeviceManager) ConnectAll() {
	for _, md := range dm.devices {
		dev := md // capture
		cloudMQTT := NewCloudMQTT(dm.loginData, &dev.Info)
		cloudMQTT.SetStatusCallback(func(status *DeviceStatus) {
			published := &PublishedStatus{
				State:      status.StateName,
				Battery:    status.Battery,
				FanSpeed:   status.FanSpeedName,
				MopMode:    status.MopModeName,
				WaterBox:   status.WaterBoxName,
				CleanTime:  status.CleanTime,
				CleanArea:  status.CleanArea,
				ErrorCode:  status.ErrorCode,
				Error:      status.ErrorName,
				InCleaning: status.InCleaning > 0,
			}
			if status.InCleaning > 0 {
				published.CleanPercent = status.CleanPercent
			}
			dm.runTracker.Update(dev.Slug, status, published)
			dev.SetStatus(published)
			if dm.onStatus != nil {
				dm.onStatus(dev.Slug, published)
			}
		})
		cloudMQTT.SetAvailabilityCallback(func(online bool) {
			if dm.onAvailability != nil {
				dm.onAvailability(dev.Slug, online)
			}
		})

		if err := cloudMQTT.Connect(); err != nil {
			logger.Error("Failed to connect device", "device", dev.Info.Name, "slug", dev.Slug, "error", err)
			continue
		}

		dev.CloudMQTT = cloudMQTT
		logger.Info("Connected device", "device", dev.Info.Name, "slug", dev.Slug)

		// Fetch scenes for this device
		if dm.restClient != nil {
			scenes, err := dm.restClient.GetScenes(dev.Info.DID)
			if err != nil {
				logger.Warn("Failed to fetch scenes", "device", dev.Slug, "error", err)
			} else {
				dev.Scenes = scenes
				logger.Info("Fetched scenes", "device", dev.Slug, "count", len(scenes))
			}
		}
	}
}

func (dm *DeviceManager) mapCacheDir() string {
	if dm.restClient == nil || dm.restClient.sessionDir == "" {
		return ""
	}
	return filepath.Join(dm.restClient.sessionDir, "maps")
}

// SaveMapCache writes a device's map PNG to disk.
func (dm *DeviceManager) SaveMapCache(slug string, pngData []byte) {
	dir := dm.mapCacheDir()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		logger.Debug("Failed to create map cache dir", "error", err)
		return
	}
	file := filepath.Join(dir, slug+".png")
	if err := os.WriteFile(file, pngData, 0600); err != nil {
		logger.Debug("Failed to save map cache", "device", slug, "error", err)
	}
}

// LoadMapCaches loads cached map PNGs from disk into each device's MapPNG.
func (dm *DeviceManager) LoadMapCaches() {
	dir := dm.mapCacheDir()
	if dir == "" {
		return
	}
	for _, md := range dm.devices {
		file := filepath.Join(dir, md.Slug+".png")
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		md.MapPNG = data
		logger.Info("Loaded cached map", "device", md.Slug, "size", len(data))
	}
}

// ExecuteScene triggers a scene via the REST API.
func (dm *DeviceManager) ExecuteScene(sceneID int) error {
	if dm.restClient == nil {
		return fmt.Errorf("no REST client available")
	}
	return dm.restClient.ExecuteScene(sceneID)
}

// NoteSceneStarted associates an upcoming cleaning run on a device with a
// triggered scene so its ETA is estimated from previous runs of the same scene.
func (dm *DeviceManager) NoteSceneStarted(slug string, sceneID int) {
	dm.runTracker.NoteSceneStarted(slug, sceneID)
}

// NoteSegmentClean associates an upcoming cleaning run with a triggered segment
// clean so its ETA is estimated from previous runs of the same rooms.
func (dm *DeviceManager) NoteSegmentClean(slug string, segments []int) {
	dm.runTracker.NoteSegmentClean(slug, segments)
}

// SceneRecordedMinutes returns the recorded run duration (minutes) for a scene
// on a device, or 0 if none has been recorded yet.
func (dm *DeviceManager) SceneRecordedMinutes(slug string, sceneID int) int {
	return dm.runTracker.DurationMinutes(slug, fmt.Sprintf("scene:%d", sceneID))
}

// DisconnectAll disconnects all devices.
func (dm *DeviceManager) DisconnectAll() {
	for _, md := range dm.devices {
		if md.CloudMQTT != nil {
			md.CloudMQTT.Disconnect()
		}
	}
}

// PollAll polls status, consumables, and maps for all connected devices.
func (dm *DeviceManager) PollAll() {
	for _, md := range dm.devices {
		if md.CloudMQTT == nil || !md.CloudMQTT.IsConnected() {
			continue
		}

		status, err := md.CloudMQTT.PollStatus()
		if err != nil {
			logger.Error("Failed to poll status", "device", md.Slug, "error", err)
			continue
		}

		published := &PublishedStatus{
			State:      status.StateName,
			Battery:    status.Battery,
			FanSpeed:   status.FanSpeedName,
			MopMode:    status.MopModeName,
			WaterBox:   status.WaterBoxName,
			CleanTime:  status.CleanTime,
			CleanArea:  status.CleanArea,
			ErrorCode:  status.ErrorCode,
			Error:      status.ErrorName,
			InCleaning: status.InCleaning > 0,
		}
		if status.InCleaning > 0 {
			published.CleanPercent = status.CleanPercent
		}

		consumables, err := md.CloudMQTT.PollConsumables()
		if err != nil {
			logger.Debug("Failed to poll consumables, keeping last known values", "device", md.Slug, "error", err)
			// Preserve last known consumable data
			if prev := md.GetStatus(); prev != nil {
				published.Consumables = prev.Consumables
				published.ConsumablePercents = prev.ConsumablePercents
			}
		} else {
			published.Consumables = *consumables
			published.ConsumablePercents = ComputeConsumablePercents(consumables)
		}

		dm.runTracker.Update(md.Slug, status, published)
		md.SetStatus(published)
		if dm.onStatus != nil {
			dm.onStatus(md.Slug, published)
		}

		// Map polling: first cycle, every cycle during cleaning, every 5th cycle when idle
		shouldPollMap := published.InCleaning || md.pollCount == 0 || md.pollCount%5 == 0
		md.pollCount++
		if shouldPollMap {
			mappings, mappingErr := md.CloudMQTT.PollRoomMappings()
			if mappingErr != nil {
				md.SetRoomMappings(nil)
				logger.Warn("Failed to refresh active room mapping", "device", md.Slug, "error", mappingErr)
			} else {
				md.SetRoomMappings(mappings)
			}
			mapPNG, mapData, err := md.CloudMQTT.PollMap()
			if err != nil {
				logger.Debug("Failed to poll map", "device", md.Slug, "error", err)
			} else if mapPNG != nil {
				md.SetMapPNG(mapPNG)
				go dm.SaveMapCache(md.Slug, mapPNG)
				if mapData != nil {
					vectorJSON, err := MapToVectorJSON(mapData)
					if err == nil && vectorJSON != nil {
						md.mu.Lock()
						md.VectorMapJSON = vectorJSON
						md.mu.Unlock()
					}
				}
				if dm.onMap != nil {
					dm.onMap(md.Slug, mapPNG)
				}
			}
		}
	}
}

// StartPolling starts periodic polling for all devices.
func (dm *DeviceManager) StartPolling(interval time.Duration, stop chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			dm.PollAll()
		case <-stop:
			return
		}
	}
}

// GetDevice returns a managed device by slug.
func (dm *DeviceManager) GetDevice(slug string) *ManagedDevice {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.bySlug[slug]
}

// GetDevices returns all managed devices.
func (dm *DeviceManager) GetDevices() []*ManagedDevice {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.devices
}

// DeviceSummary is used in API responses.
type DeviceSummary struct {
	Slug   string           `json:"slug"`
	Name   string           `json:"name"`
	Model  string           `json:"model"`
	Online bool             `json:"online"`
	Status *PublishedStatus `json:"status,omitempty"`
	Scenes []Scene          `json:"scenes,omitempty"`
}

// GetSummaries returns a list of device summaries for the API, sorted by name.
func (dm *DeviceManager) GetSummaries() []DeviceSummary {
	var summaries []DeviceSummary
	for _, md := range dm.devices {
		summaries = append(summaries, DeviceSummary{
			Slug:   md.Slug,
			Name:   md.Info.Name,
			Model:  md.Info.Model,
			Online: md.CloudMQTT != nil && md.CloudMQTT.IsConnected(),
			Status: md.GetStatus(),
			Scenes: md.Scenes,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Name < summaries[j].Name
	})
	return summaries
}
