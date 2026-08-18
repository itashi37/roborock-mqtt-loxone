package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/philipparndt/go-logger"
)

var cfg Config
var cfgMu sync.RWMutex
var loadedConfigFile string
var envVariablePattern = regexp.MustCompile(`\$\{([^}]+)\}`)

const runtimeSettingsFile = "integration-settings.json"

// EnsureConfigFile creates the minimal browser-first configuration used by a
// fresh Docker volume. Existing files are never changed.
func EnsureConfigFile(file string) error {
	if _, err := os.Stat(file); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	minimal := Config{
		MQTT:     MQTTConfig{Enabled: boolConfig(false), Topic: "home/roborock", QoS: 1, Retain: true},
		Roborock: RoborockConfig{BaseURL: "https://euiot.roborock.com", PollingInterval: 30},
		Loxone:   LoxoneConfig{Topic: "loxone/roborock"},
		Web:      WebConfig{Enabled: true, Port: 8080}, LogLevel: "info",
	}
	data, err := json.MarshalIndent(minimal, "", "  ")
	if err != nil {
		return err
	}
	handle, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return fmt.Errorf("create initial config: %w", err)
	}
	defer handle.Close()
	if _, err := handle.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write initial config: %w", err)
	}
	return nil
}

func boolConfig(value bool) *bool { return &value }

type RuntimeSettings struct {
	MQTT             MQTTConfig   `json:"mqtt"`
	Loxone           LoxoneConfig `json:"loxone"`
	RoborockUsername string       `json:"roborock_username,omitempty"`
	SetupComplete    *bool        `json:"setup_complete,omitempty"`
}

type Config struct {
	MQTT          MQTTConfig         `json:"mqtt"`
	Roborock      RoborockConfig     `json:"roborock"`
	Loxone        LoxoneConfig       `json:"loxone,omitempty"`
	Web           WebConfig          `json:"web"`
	Notifications NotificationConfig `json:"notifications,omitempty"`
	LogLevel      string             `json:"loglevel,omitempty"`
}

// MQTTConfig controls the optional local/home-automation MQTT adapter. Enabled
// is a pointer so existing configurations that predate integration modes keep
// their historical behaviour (enabled by default).
type MQTTConfig struct {
	Enabled  *bool  `json:"enabled,omitempty"`
	URL      string `json:"url"`
	Retain   bool   `json:"retain"`
	Topic    string `json:"topic"`
	QoS      byte   `json:"qos"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	TLS      bool   `json:"tls,omitempty"`
}

func (m MQTTConfig) IsEnabled() bool {
	return m.Enabled == nil || *m.Enabled
}

type LoxoneConfig struct {
	Enabled               bool                               `json:"enabled"`
	Topic                 string                             `json:"topic,omitempty"`
	CommandDebounceMS     int                                `json:"command_debounce_ms,omitempty"`
	CommandTimeoutSeconds int                                `json:"command_timeout_seconds,omitempty"`
	Direct                DirectLoxoneConfig                 `json:"direct,omitempty"`
	Devices               map[string]DeviceIntegrationConfig `json:"devices,omitempty"`
}

type DeviceIntegrationConfig struct {
	MQTT   *bool `json:"mqtt,omitempty"`
	Direct *bool `json:"direct,omitempty"`
}

func (l LoxoneConfig) DeviceModes(deviceID, slug string) (mqttEnabled, directEnabled bool) {
	mqttEnabled = true
	directEnabled = l.Direct.Enabled
	device, ok := l.Devices[deviceID]
	if !ok {
		device, ok = l.Devices[slug]
	}
	if ok {
		if device.MQTT != nil {
			mqttEnabled = *device.MQTT
		}
		if device.Direct != nil {
			directEnabled = *device.Direct
		}
	}
	return mqttEnabled, directEnabled
}

type DirectLoxoneConfig struct {
	Enabled            bool                         `json:"enabled"`
	Scheme             string                       `json:"scheme,omitempty"`
	Host               string                       `json:"host,omitempty"`
	Port               int                          `json:"port,omitempty"`
	Username           string                       `json:"username,omitempty"`
	Password           string                       `json:"password,omitempty"`
	TimeoutSeconds     int                          `json:"timeout_seconds,omitempty"`
	MaxRetries         int                          `json:"max_retries,omitempty"`
	RetryDelayMS       int                          `json:"retry_delay_ms,omitempty"`
	InputPrefix        string                       `json:"input_prefix,omitempty"`
	Inputs             map[string]map[string]string `json:"inputs,omitempty"`
	APIUsername        string                       `json:"api_username,omitempty"`
	APIToken           string                       `json:"api_token,omitempty"`
	AllowedCIDRs       []string                     `json:"allowed_cidrs,omitempty"`
	AllowGETCommands   bool                         `json:"allow_get_commands,omitempty"`
	RateLimitPerMinute int                          `json:"rate_limit_per_minute,omitempty"`
}

type TimeSlot struct {
	Time    string `json:"time"`
	Action  string `json:"action"`
	SceneID int    `json:"scene_id,omitempty"`
}

type DeviceSchedule struct {
	Normal    []TimeSlot `json:"normal,omitempty"`
	Weekend   []TimeSlot `json:"weekend,omitempty"`
	Free      []TimeSlot `json:"free,omitempty"`
	NotAtHome []TimeSlot `json:"notAtHome,omitempty"`
}

type ScheduleSignals struct {
	PublicHoliday string `json:"public_holiday,omitempty"`
	Vacation      string `json:"vacation,omitempty"`
}

type ConsumableLifetimes struct {
	MainBrush      int `json:"main_brush,omitempty"`
	SideBrush      int `json:"side_brush,omitempty"`
	Filter         int `json:"filter,omitempty"`
	Sensor         int `json:"sensor,omitempty"`
	DustCollection int `json:"dust_collection,omitempty"`
}

type EmailConfig struct {
	Enabled  bool   `json:"enabled"`
	SMTPHost string `json:"smtp_host,omitempty"`
	SMTPPort int    `json:"smtp_port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
}

type ThresholdConfig struct {
	WarnPercent     int `json:"warn_percent,omitempty"`
	CriticalPercent int `json:"critical_percent,omitempty"`
}

type NotificationConfig struct {
	Email                        EmailConfig         `json:"email,omitempty"`
	Thresholds                   ThresholdConfig     `json:"thresholds,omitempty"`
	ConsumableLifetimes          ConsumableLifetimes `json:"consumable_lifetimes,omitempty"`
	DisableScheduleOnMaintenance *bool               `json:"disable_schedule_on_maintenance,omitempty"`
}

// ShouldDisableScheduleOnMaintenance returns whether schedules should be paused
// when maintenance is pending. Defaults to true.
func (n NotificationConfig) ShouldDisableScheduleOnMaintenance() bool {
	if n.DisableScheduleOnMaintenance == nil {
		return true
	}
	return *n.DisableScheduleOnMaintenance
}

type RoborockConfig struct {
	Username        string                       `json:"username"`
	Password        string                       `json:"password"`
	ClientID        string                       `json:"client_id"`
	BaseURL         string                       `json:"base_url"`
	PollingInterval int                          `json:"polling_interval"`
	Schedules       map[string]DeviceSchedule    `json:"schedules,omitempty"`
	ScheduleSignals ScheduleSignals              `json:"schedule_signals,omitempty"`
	RoomNames       map[string]map[string]string `json:"room_names,omitempty"`
}

type WebConfig struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
	// LivenessGraceSeconds is how long the bridge may stay unhealthy (not
	// authenticated, or no device connected) before the /livez probe fails.
	// Defaults to 240 (4 min) when unset.
	LivenessGraceSeconds int `json:"liveness_grace_seconds,omitempty"`
}

func LoadConfig(file string) (Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		logger.Error("Error reading config file", "error", err)
		return Config{}, err
	}

	data = envVariablePattern.ReplaceAllFunc(data, func(match []byte) []byte {
		return []byte(os.Getenv(string(match[2 : len(match)-1])))
	})

	var loaded Config
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		logger.Error("Unmarshaling JSON", "error", err)
		return Config{}, err
	}
	settingsFile := filepath.Join(filepath.Dir(file), runtimeSettingsFile)
	if settingsData, readErr := os.ReadFile(settingsFile); readErr == nil {
		settingsData = envVariablePattern.ReplaceAllFunc(settingsData, func(match []byte) []byte {
			return []byte(os.Getenv(string(match[2 : len(match)-1])))
		})
		var settings RuntimeSettings
		if decodeErr := json.Unmarshal(settingsData, &settings); decodeErr != nil {
			return Config{}, fmt.Errorf("parse runtime integration settings: %w", decodeErr)
		}
		loaded.MQTT = settings.MQTT
		loaded.Loxone = settings.Loxone
		if strings.TrimSpace(settings.RoborockUsername) != "" {
			loaded.Roborock.Username = settings.RoborockUsername
		}
	} else if !os.IsNotExist(readErr) {
		return Config{}, fmt.Errorf("read runtime integration settings: %w", readErr)
	}
	applyDefaults(&loaded)
	cfgMu.Lock()
	cfg = loaded
	loadedConfigFile = file
	cfgMu.Unlock()
	return loaded, nil
}

func applyDefaults(cfg *Config) {
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	if cfg.Roborock.BaseURL == "" {
		cfg.Roborock.BaseURL = "https://euiot.roborock.com"
	}

	if cfg.Roborock.PollingInterval == 0 {
		cfg.Roborock.PollingInterval = 60
	}

	if cfg.Web.Port == 0 {
		cfg.Web.Port = 8080
	}

	cfg.Loxone.Topic = strings.TrimSuffix(strings.TrimSpace(cfg.Loxone.Topic), "/")
	if cfg.Loxone.Topic == "" {
		cfg.Loxone.Topic = "loxone/roborock"
	}
	if cfg.Loxone.CommandDebounceMS <= 0 {
		cfg.Loxone.CommandDebounceMS = 2000
	}
	if cfg.Loxone.CommandTimeoutSeconds <= 0 {
		cfg.Loxone.CommandTimeoutSeconds = 90
	}
	cfg.Loxone.Direct.Scheme = strings.ToLower(strings.TrimSpace(cfg.Loxone.Direct.Scheme))
	if cfg.Loxone.Direct.Scheme == "" {
		cfg.Loxone.Direct.Scheme = "http"
	}
	if cfg.Loxone.Direct.Port <= 0 {
		if cfg.Loxone.Direct.Scheme == "https" {
			cfg.Loxone.Direct.Port = 443
		} else {
			cfg.Loxone.Direct.Port = 80
		}
	}
	if cfg.Loxone.Direct.TimeoutSeconds <= 0 {
		cfg.Loxone.Direct.TimeoutSeconds = 5
	}
	if cfg.Loxone.Direct.MaxRetries <= 0 {
		cfg.Loxone.Direct.MaxRetries = 3
	}
	if cfg.Loxone.Direct.RetryDelayMS <= 0 {
		cfg.Loxone.Direct.RetryDelayMS = 500
	}
	if strings.TrimSpace(cfg.Loxone.Direct.InputPrefix) == "" {
		cfg.Loxone.Direct.InputPrefix = "RR"
	}
	if strings.TrimSpace(cfg.Loxone.Direct.APIUsername) == "" {
		cfg.Loxone.Direct.APIUsername = "loxone"
	}
	if cfg.Loxone.Direct.RateLimitPerMinute <= 0 {
		cfg.Loxone.Direct.RateLimitPerMinute = 30
	}

	if cfg.Roborock.ScheduleSignals.PublicHoliday == "" {
		cfg.Roborock.ScheduleSignals.PublicHoliday = "rules/public-holiday"
	}
	if cfg.Roborock.ScheduleSignals.Vacation == "" {
		cfg.Roborock.ScheduleSignals.Vacation = "rules/free-day"
	}

	// Notification defaults
	if cfg.Notifications.Thresholds.WarnPercent == 0 {
		cfg.Notifications.Thresholds.WarnPercent = 20
	}
	if cfg.Notifications.Thresholds.CriticalPercent == 0 {
		cfg.Notifications.Thresholds.CriticalPercent = 10
	}
	if cfg.Notifications.ConsumableLifetimes.MainBrush == 0 {
		cfg.Notifications.ConsumableLifetimes.MainBrush = 1080000
	}
	if cfg.Notifications.ConsumableLifetimes.SideBrush == 0 {
		cfg.Notifications.ConsumableLifetimes.SideBrush = 720000
	}
	if cfg.Notifications.ConsumableLifetimes.Filter == 0 {
		cfg.Notifications.ConsumableLifetimes.Filter = 540000
	}
	if cfg.Notifications.ConsumableLifetimes.Sensor == 0 {
		cfg.Notifications.ConsumableLifetimes.Sensor = 108000
	}
	if cfg.Notifications.ConsumableLifetimes.DustCollection == 0 {
		cfg.Notifications.ConsumableLifetimes.DustCollection = 20 // cycle count, not seconds
	}
	if cfg.Notifications.Email.SMTPPort == 0 {
		cfg.Notifications.Email.SMTPPort = 587
	}

}

func Get() Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cfg
}

func SaveRuntimeSettings(settings RuntimeSettings) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	if loadedConfigFile == "" {
		return fmt.Errorf("configuration has not been loaded")
	}
	if strings.TrimSpace(settings.RoborockUsername) == "" {
		settings.RoborockUsername = cfg.Roborock.Username
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal runtime integration settings: %w", err)
	}
	path := filepath.Join(filepath.Dir(loadedConfigFile), runtimeSettingsFile)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("write runtime integration settings: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace runtime integration settings: %w", err)
	}
	cfg.MQTT = settings.MQTT
	cfg.Loxone = settings.Loxone
	cfg.Roborock.Username = settings.RoborockUsername
	applyDefaults(&cfg)
	return nil
}

func RuntimeSettingsSnapshot() RuntimeSettings {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	settings := RuntimeSettings{MQTT: cfg.MQTT, Loxone: cfg.Loxone, RoborockUsername: cfg.Roborock.Username}
	if loadedConfigFile != "" {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(loadedConfigFile), runtimeSettingsFile))
		if err == nil {
			var persisted RuntimeSettings
			if json.Unmarshal(data, &persisted) == nil {
				settings.SetupComplete = persisted.SetupComplete
			}
		}
	}
	return settings
}

// SetupComplete preserves backward compatibility: installations configured
// before the browser wizard are considered complete when an account exists.
func SetupComplete() bool {
	settings := RuntimeSettingsSnapshot()
	if settings.SetupComplete != nil {
		return *settings.SetupComplete
	}
	return strings.TrimSpace(settings.RoborockUsername) != ""
}
