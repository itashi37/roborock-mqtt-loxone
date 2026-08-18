package roborock

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/philipparndt/go-logger"
)

// Client handles authentication and REST API calls to the Roborock cloud.
type Client struct {
	baseURL    string
	username   string
	password   string
	clientID   string
	httpClient *http.Client
	loginData  *LoginData
	device     *DeviceInfo
	devices    []DeviceInfo
	rooms      []RoomInfo
	sessionDir string
}

// SavedSession contains the data persisted between restarts.
type SavedSession struct {
	LoginData LoginData    `json:"login_data"`
	Devices   []DeviceInfo `json:"devices,omitempty"`
	Rooms     []RoomInfo   `json:"rooms,omitempty"`
}

func NewClient(baseURL, username, password, clientID string) *Client {
	return &Client{
		baseURL:    baseURL,
		username:   username,
		password:   password,
		clientID:   clientID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetSessionDir sets the directory for persisting session data.
func (c *Client) SetSessionDir(dir string) {
	c.sessionDir = dir
}

// SetUsername updates the Roborock login identifier used by the setup wizard.
// It deliberately does not alter an existing cloud session.
func (c *Client) SetUsername(username string) {
	c.username = strings.TrimSpace(username)
}

func (c *Client) sessionFile() string {
	if c.sessionDir == "" {
		return ""
	}
	return filepath.Join(c.sessionDir, "session.json")
}

// SaveSession persists the current login data and device info to disk.
func (c *Client) SaveSession() error {
	file := c.sessionFile()
	if file == "" || c.loginData == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	session := SavedSession{
		LoginData: *c.loginData,
		Devices:   c.devices,
		Rooms:     c.rooms,
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	return os.WriteFile(file, data, 0600)
}

// LoadSession tries to restore a previous session from disk.
func (c *Client) LoadSession() bool {
	file := c.sessionFile()
	if file == "" {
		return false
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}

	var session SavedSession
	if err := json.Unmarshal(data, &session); err != nil {
		logger.Warn("Failed to parse saved session", "error", err)
		return false
	}

	c.loginData = &session.LoginData
	if len(session.Devices) > 0 {
		c.devices = session.Devices
		c.device = &c.devices[0]
	}
	c.rooms = session.Rooms
	logger.Info("Restored session from disk",
		"user", c.loginData.Nickname,
		"devices", len(c.devices),
		"rooms", len(c.rooms),
	)
	return true
}

// ClearSession removes the saved session file.
func (c *Client) ClearSession() {
	file := c.sessionFile()
	if file != "" {
		os.Remove(file)
	}
	c.loginData = nil
	c.device = nil
}

// IsAuthenticated returns whether the client has valid login data.
func (c *Client) IsAuthenticated() bool {
	return c.loginData != nil && len(c.devices) > 0
}

// headerClientID computes the header_clientid value.
func (c *Client) headerClientID() string {
	if c.clientID != "" {
		return c.clientID
	}
	h := md5.New()
	h.Write([]byte(c.username))
	h.Write([]byte(c.clientID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// RequestCode sends a verification code to the user's email.
func (c *Client) RequestCode() error {
	if c.username == "" {
		return fmt.Errorf("username is not configured - check your config file")
	}

	params := url.Values{}
	params.Set("username", c.username)
	params.Set("type", "auth")

	reqURL := fmt.Sprintf("%s/api/v1/sendEmailCode?%s", c.baseURL, params.Encode())

	req, err := http.NewRequest("POST", reqURL, nil)
	if err != nil {
		return fmt.Errorf("create code request: %w", err)
	}
	req.Header.Set("header_clientid", c.headerClientID())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("code request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read code response: %w", err)
	}

	var codeResp LoginResponse
	if err := json.Unmarshal(body, &codeResp); err != nil {
		return fmt.Errorf("parse code response: %w", err)
	}

	if codeResp.Code != 200 {
		return fmt.Errorf("request code failed with code %d: %s", codeResp.Code, string(body))
	}

	logger.Info("Verification code sent to email", "email", c.username)
	return nil
}

// CodeLogin authenticates using the email verification code.
func (c *Client) CodeLogin(code string) error {
	params := url.Values{}
	params.Set("username", c.username)
	params.Set("verifycode", code)
	params.Set("verifycodetype", "AUTH_EMAIL_CODE")

	reqURL := fmt.Sprintf("%s/api/v1/loginWithCode", c.baseURL)

	req, err := http.NewRequest("POST", reqURL, strings.NewReader(params.Encode()))
	if err != nil {
		return fmt.Errorf("create code login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("header_clientid", c.headerClientID())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("code login request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read code login response: %w", err)
	}

	logger.Debug("Code login response", "body", string(body))

	var loginResp LoginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return fmt.Errorf("parse code login response: %w", err)
	}

	if loginResp.Code != 200 {
		switch loginResp.Code {
		case 2018:
			return fmt.Errorf("invalid verification code")
		default:
			return fmt.Errorf("code login failed with code %d: %s", loginResp.Code, string(body))
		}
	}

	c.loginData = &loginResp.Data
	logger.Info("Code login successful",
		"user", loginResp.Data.Nickname,
		"region", loginResp.Data.Region,
		"rriot_api", loginResp.Data.RRIoT.Remote.APIURL,
		"rriot_mqtt", loginResp.Data.RRIoT.Remote.MQTTServer,
		"rriot_user", loginResp.Data.RRIoT.UserID,
	)

	return nil
}

// Login authenticates with the Roborock cloud API using password.
func (c *Client) Login() error {
	params := url.Values{}
	params.Set("username", c.username)
	params.Set("password", c.password)
	params.Set("needtwostepauth", "false")

	reqURL := fmt.Sprintf("%s/api/v1/login?%s", c.baseURL, params.Encode())

	req, err := http.NewRequest("POST", reqURL, nil)
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("header_clientid", c.headerClientID())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read login response: %w", err)
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return fmt.Errorf("parse login response: %w", err)
	}

	if loginResp.Code != 200 {
		switch loginResp.Code {
		case 2008:
			return fmt.Errorf("login failed: unknown user")
		case 2012:
			return fmt.Errorf("login failed: incorrect password")
		case 2031:
			return fmt.Errorf("two-step authentication required")
		default:
			return fmt.Errorf("login failed with code %d: %s", loginResp.Code, string(body))
		}
	}

	c.loginData = &loginResp.Data
	logger.Info("Login successful", "user", loginResp.Data.Nickname, "region", loginResp.Data.Region)

	return nil
}

// hawkAuth generates a Hawk authentication header for REST API requests.
// Matches the python-roborock implementation: key is raw UTF-8 bytes,
// message format is u:s:nonce:ts:md5(url):::
func (c *Client) hawkAuth(urlPath string) string {
	nonce := uuid.New().String()[:8]
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	urlHash := md5.Sum([]byte(urlPath))
	urlHashHex := hex.EncodeToString(urlHash[:])

	// Format: u:s:nonce:ts:md5(url):params:formdata
	message := fmt.Sprintf("%s:%s:%s:%s:%s::",
		c.loginData.RRIoT.UserID,
		c.loginData.RRIoT.SessionID,
		nonce,
		ts,
		urlHashHex,
	)

	// HMAC key is raw UTF-8 bytes of rriot.h, NOT base64-decoded
	keyBytes := []byte(c.loginData.RRIoT.HMACKey)
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(message))
	macStr := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf(`Hawk id="%s",s="%s",ts="%s",nonce="%s",mac="%s"`,
		c.loginData.RRIoT.UserID,
		c.loginData.RRIoT.SessionID,
		ts,
		nonce,
		macStr,
	)
}

// authenticatedGet performs a GET request with Hawk authentication against the base URL.
func (c *Client) authenticatedGet(path string) ([]byte, error) {
	fullURL := c.baseURL + path

	logger.Debug("Authenticated GET", "url", fullURL)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.hawkAuth(path))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	logger.Debug("Authenticated GET response", "status", resp.StatusCode, "body", string(body))

	if resp.StatusCode == 401 {
		logger.Warn("Authentication expired, re-logging in...")
		if err := c.Login(); err != nil {
			return nil, fmt.Errorf("re-login: %w", err)
		}

		req2, _ := http.NewRequest("GET", fullURL, nil)
		req2.Header.Set("Authorization", c.hawkAuth(path))
		resp2, err := c.httpClient.Do(req2)
		if err != nil {
			return nil, fmt.Errorf("retry request: %w", err)
		}
		defer resp2.Body.Close()
		return io.ReadAll(resp2.Body)
	}

	return body, nil
}

// GetHomeDetail retrieves home and device information.
// Uses token-based auth against the base URL (euiot.roborock.com).
func (c *Client) GetHomeDetail() (*HomeDetailData, error) {
	fullURL := c.baseURL + "/api/v1/getHomeDetail"

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.loginData.Token)
	req.Header.Set("header_clientid", c.headerClientID())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	logger.Debug("getHomeDetail response", "status", resp.StatusCode, "body", string(body))

	var homeResp HomeDetailResponse
	if err := json.Unmarshal(body, &homeResp); err != nil {
		return nil, fmt.Errorf("parse home detail: %w", err)
	}

	if homeResp.Code != 200 {
		return nil, fmt.Errorf("getHomeDetail failed with code %d", homeResp.Code)
	}

	return &homeResp.Data, nil
}

// GetHomeData retrieves full home data including devices from the RRIOT API.
// Uses Hawk authentication against api-eu.roborock.com.
func (c *Client) GetHomeData(homeID int) (*HomeData, error) {
	path := fmt.Sprintf("/user/homes/%d", homeID)
	apiURL := c.loginData.RRIoT.Remote.APIURL
	fullURL := apiURL + path

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.hawkAuth(path))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	logger.Debug("getHomeData response", "status", resp.StatusCode, "body", string(body))

	var homeResp HomeDataResponse
	if err := json.Unmarshal(body, &homeResp); err != nil {
		return nil, fmt.Errorf("parse home data: %w", err)
	}

	if !homeResp.Success {
		return nil, fmt.Errorf("getHomeData failed")
	}

	return &homeResp.Result, nil
}

// DiscoverDevice finds the first device after login.
// Step 1: getHomeDetail (base URL + token) -> home ID
// Step 2: /user/homes/{id} (RRIOT API + Hawk auth) -> devices
func (c *Client) DiscoverDevice() error {
	homeDetail, err := c.GetHomeDetail()
	if err != nil {
		return fmt.Errorf("get home detail: %w", err)
	}

	logger.Info("Found home", "id", homeDetail.ID, "name", homeDetail.Name)

	homeID := homeDetail.RRHomeID
	if homeID == 0 {
		homeID = homeDetail.ID
	}
	homeData, err := c.GetHomeData(homeID)
	if err != nil {
		return fmt.Errorf("get home data: %w", err)
	}

	allDevices := append(homeData.Devices, homeData.ReceivedDevices...)
	if len(allDevices) == 0 {
		return fmt.Errorf("no devices found in account")
	}

	c.devices = allDevices
	c.device = &allDevices[0]
	c.rooms = homeData.Rooms

	for _, dev := range allDevices {
		logger.Info("Discovered device",
			"name", dev.Name,
			"model", dev.Model,
			"duid", dev.DID,
			"online", dev.Online,
			"slug", Slugify(dev.Name),
		)
	}
	for _, room := range c.rooms {
		logger.Info("Discovered room", "id", room.ID, "name", room.Name)
	}

	return nil
}

// GetLoginData returns the login data after authentication.
func (c *Client) GetLoginData() *LoginData {
	return c.loginData
}

// GetDevice returns the first discovered device.
func (c *Client) GetDevice() *DeviceInfo {
	return c.device
}

// GetDevices returns all discovered devices.
func (c *Client) GetDevices() []DeviceInfo {
	return c.devices
}

// GetRooms returns the rooms discovered from the home data.
func (c *Client) GetRooms() []RoomInfo {
	return c.rooms
}

// GetRoomNameMap returns home-level cloud room IDs mapped to names. These IDs
// are not segment IDs; get_room_mapping is required to join them to the active
// map's app_segment_clean IDs.
func (c *Client) GetRoomNameMap() map[string]string {
	names := make(map[string]string)
	for _, r := range c.rooms {
		names[fmt.Sprintf("%d", r.ID)] = r.Name
	}
	return names
}

// GetScenes fetches cleaning scenes/programs for a device using Hawk auth against the RRIOT API.
func (c *Client) GetScenes(deviceID string) ([]Scene, error) {
	path := "/user/scene/device/" + deviceID
	apiURL := c.loginData.RRIoT.Remote.APIURL
	fullURL := apiURL + path

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.hawkAuth(path))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	logger.Debug("GetScenes response", "device", deviceID, "status", resp.StatusCode, "body", string(body))

	var scenesResp ScenesResponse
	if err := json.Unmarshal(body, &scenesResp); err != nil {
		return nil, fmt.Errorf("parse scenes: %w", err)
	}

	if !scenesResp.Success {
		return nil, fmt.Errorf("getScenes failed")
	}

	return scenesResp.Result, nil
}

// ExecuteScene triggers a cleaning scene/program using Hawk auth against the RRIOT API.
func (c *Client) ExecuteScene(sceneID int) error {
	path := fmt.Sprintf("/user/scene/%d/execute", sceneID)
	apiURL := c.loginData.RRIoT.Remote.APIURL
	fullURL := apiURL + path

	req, err := http.NewRequest("POST", fullURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.hawkAuth(path))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	logger.Debug("ExecuteScene response", "sceneID", sceneID, "status", resp.StatusCode, "body", string(body))

	return nil
}

func init() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
}
