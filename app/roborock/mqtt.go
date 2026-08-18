package roborock

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/philipparndt/go-logger"
)

const protocolMap = 301

const maxConsecutiveTimeouts = 3

// CloudMQTT manages the MQTT connection to the Roborock cloud broker.
type CloudMQTT struct {
	client              pahomqtt.Client
	loginData           *LoginData
	device              *DeviceInfo
	mqttUsername        string
	tracker             *RequestTracker
	mapSecurity         *MapSecurityData
	mapChan             chan []byte
	onStatus            func(*DeviceStatus)
	onAvailability      func(online bool)
	mu                  sync.Mutex
	connected           bool
	available           bool
	availKnown          bool
	consecutiveTimeouts int
	stopCh              chan struct{}
}

// NewCloudMQTT creates a new Roborock cloud MQTT client.
func NewCloudMQTT(loginData *LoginData, device *DeviceInfo) *CloudMQTT {
	return &CloudMQTT{
		loginData: loginData,
		device:    device,
		tracker:   NewRequestTracker(),
		stopCh:    make(chan struct{}),
	}
}

// SetStatusCallback sets the function called when device status updates arrive.
func (cm *CloudMQTT) SetStatusCallback(cb func(*DeviceStatus)) {
	cm.onStatus = cb
}

// SetAvailabilityCallback sets the function called whenever the device's
// cloud-connection state transitions (online/offline). It fires only on change.
func (cm *CloudMQTT) SetAvailabilityCallback(cb func(online bool)) {
	cm.onAvailability = cb
}

// setAvailability records the cloud-connection state and, on a transition (or the
// first observation), invokes the availability callback outside the lock.
func (cm *CloudMQTT) setAvailability(online bool) {
	cm.mu.Lock()
	changed := !cm.availKnown || cm.available != online
	cm.available = online
	cm.availKnown = true
	cb := cm.onAvailability
	cm.mu.Unlock()
	if changed && cb != nil {
		cb(online)
	}
}

// Connect establishes the MQTT connection to the Roborock cloud broker.
func (cm *CloudMQTT) Connect() error {
	mqttServer := cm.loginData.RRIoT.Remote.MQTTServer
	username, password := deriveMQTTCredentials(
		cm.loginData.RRIoT.UserID,
		cm.loginData.RRIoT.SessionID,
		cm.loginData.RRIoT.MQTTKey,
	)

	cm.mqttUsername = username
	clientID := fmt.Sprintf("rrb_%06d", rand.Intn(1000000))

	logger.Debug("Cloud MQTT connection",
		"server", mqttServer,
		"username", username,
		"clientID", clientID,
		"device", cm.device.DID,
	)

	opts := pahomqtt.NewClientOptions().
		AddBroker(mqttServer).
		SetClientID(clientID).
		SetUsername(username).
		SetPassword(password).
		SetKeepAlive(60 * time.Second).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(5 * time.Minute).
		SetOnConnectHandler(cm.onConnect).
		SetConnectionLostHandler(cm.onConnectionLost).
		SetTLSConfig(&tls.Config{})

	cm.client = pahomqtt.NewClient(opts)

	token := cm.client.Connect()
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("connect to Roborock MQTT: %w", token.Error())
	}

	// Start periodic cleanup of stale requests
	go cm.cleanupLoop()

	return nil
}

func (cm *CloudMQTT) onConnect(client pahomqtt.Client) {
	cm.mu.Lock()
	cm.connected = true
	cm.mu.Unlock()

	topic := fmt.Sprintf("rr/m/o/%s/%s/%s",
		cm.loginData.RRIoT.UserID,
		cm.mqttUsername,
		cm.device.DID,
	)

	logger.Info("Connected to Roborock cloud MQTT, subscribing", "topic", topic)

	token := client.Subscribe(topic, 0, cm.handleMessage)
	if token.Wait() && token.Error() != nil {
		logger.Error("Failed to subscribe", "error", token.Error())
		return
	}
	cm.setAvailability(true)
}

func (cm *CloudMQTT) onConnectionLost(_ pahomqtt.Client, err error) {
	cm.mu.Lock()
	cm.connected = false
	cm.mu.Unlock()
	logger.Warn("Roborock cloud MQTT connection lost", "error", err)
	cm.setAvailability(false)
}

func (cm *CloudMQTT) handleMessage(_ pahomqtt.Client, mqttMsg pahomqtt.Message) {
	logger.Debug("Received cloud MQTT message", "topic", mqttMsg.Topic(), "len", len(mqttMsg.Payload()))
	cm.resetTimeouts()

	header, payload, err := DecodeMessage(mqttMsg.Payload(), cm.device.DeviceKey)
	if err != nil {
		logger.Warn("Failed to decode message", "error", err)
		return
	}

	if payload == nil {
		return
	}

	logger.Debug("Decoded message", "protocol", header.Protocol, "seq", header.SequenceNumber, "len", len(payload))

	// Protocol 301 = map data
	if header.Protocol == protocolMap {
		cm.handleMapResponse(payload)
		return
	}

	// Parse as MQTTMessage to extract DPS
	var msg MQTTMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		logger.Debug("Could not parse message", "error", err)
		return
	}

	// Look for IPC response in DPS "102"
	for key, value := range msg.DPS {
		if key == "102" {
			strVal, ok := value.(string)
			if !ok {
				continue
			}
			var ipcResp IPCResponse
			if err := json.Unmarshal([]byte(strVal), &ipcResp); err != nil {
				continue
			}

			// Try to correlate with pending request by IPC request ID
			resultData, _ := json.Marshal(ipcResp.Result)
			if cm.tracker.Complete(ipcResp.ID, resultData) {
				logger.Debug("Matched response to request", "id", ipcResp.ID)
				return
			}

			// Unsolicited — process as status
			cm.processIPCResult(ipcResp.Result)
		}
	}
}

func (cm *CloudMQTT) processIPCResult(result any) {
	if cm.onStatus == nil {
		return
	}

	// Only try to parse dict/list results as status — skip strings
	switch result.(type) {
	case string:
		return
	case nil:
		return
	}

	data, err := json.Marshal(result)
	if err != nil {
		return
	}

	status, err := ParseStatusFromProps(data)
	if err != nil {
		logger.Debug("Could not parse status from IPC result", "error", err)
		return
	}

	// Enrich with human-readable names
	status.StateName = StateName(status.State)
	status.FanSpeedName = FanSpeedName(status.FanPower)
	status.MopModeName = MopModeName(status.MopMode)
	status.WaterBoxName = WaterBoxName(status.WaterBoxMode)

	cm.onStatus(status)
}

// SendCommand sends an IPC command to the device and waits for a response.
// The requestID is the IPC request ID from the JSON payload, used for response correlation.
func (cm *CloudMQTT) SendCommand(payload []byte, requestID int) ([]byte, error) {
	encoded, _, err := EncodeMessage(payload, cm.device.DeviceKey, protocolIPC)
	if err != nil {
		return nil, fmt.Errorf("encode message: %w", err)
	}

	req := cm.tracker.Add(requestID, "")

	topic := fmt.Sprintf("rr/m/i/%s/%s/%s",
		cm.loginData.RRIoT.UserID,
		cm.mqttUsername,
		cm.device.DID,
	)

	logger.Debug("Sending command", "topic", topic, "requestID", requestID, "len", len(encoded))

	token := cm.client.Publish(topic, 0, false, encoded)
	if token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("publish command: %w", token.Error())
	}

	select {
	case resp, ok := <-req.Response:
		if !ok {
			cm.recordTimeout()
			return nil, fmt.Errorf("request timed out")
		}
		cm.resetTimeouts()
		return resp, nil
	case <-time.After(15 * time.Second):
		cm.recordTimeout()
		return nil, fmt.Errorf("command timed out after 15s")
	}
}

func (cm *CloudMQTT) recordTimeout() {
	cm.mu.Lock()
	cm.consecutiveTimeouts++
	count := cm.consecutiveTimeouts
	cm.mu.Unlock()

	if count >= maxConsecutiveTimeouts {
		logger.Warn("Too many consecutive timeouts, forcing reconnect", "device", cm.device.Name, "timeouts", count)
		go cm.forceReconnect()
	}
}

func (cm *CloudMQTT) resetTimeouts() {
	cm.mu.Lock()
	cm.consecutiveTimeouts = 0
	cm.mu.Unlock()
}

func (cm *CloudMQTT) forceReconnect() {
	cm.mu.Lock()
	cm.consecutiveTimeouts = 0
	cm.connected = false
	cm.mu.Unlock()

	if cm.client != nil {
		cm.client.Disconnect(500)
		time.Sleep(2 * time.Second)
		token := cm.client.Connect()
		if token.Wait() && token.Error() != nil {
			logger.Error("Failed to reconnect", "device", cm.device.Name, "error", token.Error())
			cm.setAvailability(false)
		} else {
			logger.Info("Reconnected to Roborock cloud MQTT", "device", cm.device.Name)
		}
	}
}

// SendCommandNoWait sends a command without waiting for a response.
func (cm *CloudMQTT) SendCommandNoWait(payload []byte) error {
	encoded, _, err := EncodeMessage(payload, cm.device.DeviceKey, protocolIPC)
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}

	topic := fmt.Sprintf("rr/m/i/%s/%s/%s",
		cm.loginData.RRIoT.UserID,
		cm.mqttUsername,
		cm.device.DID,
	)

	token := cm.client.Publish(topic, 0, false, encoded)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("publish command: %w", token.Error())
	}

	return nil
}

func (cm *CloudMQTT) cleanupLoop() {
	cleanupTicker := time.NewTicker(30 * time.Second)
	reconnectTicker := time.NewTicker(90 * time.Minute)
	defer cleanupTicker.Stop()
	defer reconnectTicker.Stop()

	for {
		select {
		case <-cleanupTicker.C:
			cm.tracker.Cleanup(60 * time.Second)
		case <-reconnectTicker.C:
			logger.Info("Proactive reconnect to keep session fresh", "device", cm.device.Name)
			cm.forceReconnect()
		case <-cm.stopCh:
			return
		}
	}
}

// Disconnect cleanly disconnects from the cloud MQTT broker.
func (cm *CloudMQTT) Disconnect() {
	close(cm.stopCh)
	if cm.client != nil && cm.client.IsConnected() {
		cm.client.Disconnect(1000)
	}
	cm.setAvailability(false)
}

// IsConnected returns whether the cloud MQTT connection is active.
func (cm *CloudMQTT) IsConnected() bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.connected
}

// Start starts a vacuum cleaning.
func (cm *CloudMQTT) Start() error {
	payload, _, err := BuildStartPayload()
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

// Pause pauses a vacuum cleaning.
func (cm *CloudMQTT) Pause() error {
	payload, _, err := BuildPausePayload()
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

// Dock sends the vacuum back to the dock.
func (cm *CloudMQTT) Dock() error {
	payload, _, err := BuildDockPayload()
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

func (cm *CloudMQTT) Stop() error {
	payload, _, err := BuildStopPayload()
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

// Locate waits for the robot RPC acknowledgement because the audible response
// has no reliable state transition that the bridge can observe.
func (cm *CloudMQTT) Locate() error {
	payload, requestID, err := BuildLocatePayload()
	if err != nil {
		return err
	}
	_, err = cm.SendCommand(payload, requestID)
	return err
}

func (cm *CloudMQTT) StartDustCollection() error {
	payload, _, err := BuildStartDustCollectionPayload()
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

func (cm *CloudMQTT) StopDustCollection() error {
	payload, _, err := BuildStopDustCollectionPayload()
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

func (cm *CloudMQTT) StartWash() error {
	payload, _, err := BuildStartWashPayload()
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

func (cm *CloudMQTT) StopWash() error {
	payload, _, err := BuildStopWashPayload()
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

func (cm *CloudMQTT) SetDryerEnabled(enabled bool) error {
	payload, _, err := BuildSetDryerStatusPayload(enabled)
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

// SegmentClean cleans specific room segments.
func (cm *CloudMQTT) SegmentClean(segments []int) error {
	payload, _, err := BuildSegmentCleanPayload(segments)
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

// SetFanSpeed sets the suction power level.
func (cm *CloudMQTT) SetFanSpeed(speed string) error {
	payload, _, err := BuildSetFanSpeedPayload(speed)
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

// SetMopMode sets the mop intensity mode.
func (cm *CloudMQTT) SetMopMode(mode string) error {
	payload, _, err := BuildSetMopModePayload(mode)
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

// SetWaterBox sets the water box level.
func (cm *CloudMQTT) SetWaterBox(level string) error {
	payload, _, err := BuildSetWaterBoxPayload(level)
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

// ResetConsumable resets a consumable counter on the device.
func (cm *CloudMQTT) ResetConsumable(name string) error {
	payload, _, err := BuildResetConsumablePayload(name)
	if err != nil {
		return err
	}
	return cm.SendCommandNoWait(payload)
}

// PollStatus requests the current device status.
func (cm *CloudMQTT) PollStatus() (*DeviceStatus, error) {
	payload, requestID, err := BuildGetStatusPayload()
	if err != nil {
		return nil, err
	}

	resp, err := cm.SendCommand(payload, requestID)
	if err != nil {
		return nil, fmt.Errorf("poll status: %w", err)
	}

	status, err := ParseStatusFromProps(resp)
	if err != nil {
		return nil, err
	}

	status.StateName = StateName(status.State)
	status.FanSpeedName = FanSpeedName(status.FanPower)
	status.MopModeName = MopModeName(status.MopMode)
	status.WaterBoxName = WaterBoxName(status.WaterBoxMode)

	return status, nil
}

// PollConsumables requests the current consumable status.
func (cm *CloudMQTT) PollConsumables() (*ConsumableStatus, error) {
	payload, requestID, err := BuildGetConsumablePayload()
	if err != nil {
		return nil, err
	}

	resp, err := cm.SendCommand(payload, requestID)
	if err != nil {
		return nil, fmt.Errorf("poll consumables: %w", err)
	}

	return ParseConsumableStatus(resp)
}

// PollRoomMappings requests the commandable rooms for the currently active
// Roborock map.
func (cm *CloudMQTT) PollRoomMappings() ([]RoomMapping, error) {
	payload, requestID, err := BuildGetRoomMappingPayload()
	if err != nil {
		return nil, err
	}
	response, err := cm.SendCommand(payload, requestID)
	if err != nil {
		return nil, fmt.Errorf("poll room mapping: %w", err)
	}
	return ParseRoomMappings(response)
}

func (cm *CloudMQTT) PollAdvancedDiagnostics() (AdvancedDiagnostics, error) {
	payload, requestID, err := BuildAppGetInitStatusPayload()
	if err != nil {
		return AdvancedDiagnostics{}, err
	}
	response, err := cm.SendCommand(payload, requestID)
	if err != nil {
		return AdvancedDiagnostics{}, fmt.Errorf("app_get_init_status: %w", err)
	}
	return ParseAdvancedDiagnostics(response, time.Now())
}

// handleMapResponse processes a Protocol 301 map data message.
func (cm *CloudMQTT) handleMapResponse(encrypted []byte) {
	if cm.mapSecurity == nil {
		logger.Debug("Received map data but no pending map request")
		return
	}

	// Map response has a 24-byte header: 8 bytes endpoint, 8 bytes padding, 2 bytes request_id, 6 bytes padding
	if len(encrypted) < 24 {
		logger.Warn("Map data too short for header", "len", len(encrypted))
		return
	}

	mapBody := encrypted[24:]
	logger.Debug("Map data body", "headerLen", 24, "bodyLen", len(mapBody))

	// Decrypt with CBC using the nonce
	decrypted, err := cm.mapSecurity.DecryptMapData(mapBody)
	if err != nil {
		logger.Warn("Failed to decrypt map data", "error", err)
		return
	}

	// Decompress gzip
	reader, err := gzip.NewReader(bytes.NewReader(decrypted))
	if err != nil {
		logger.Warn("Failed to decompress map data", "error", err)
		return
	}
	defer reader.Close()

	mapBytes, err := io.ReadAll(reader)
	if err != nil {
		logger.Warn("Failed to read decompressed map data", "error", err)
		return
	}

	logger.Debug("Map data received", "size", len(mapBytes))

	// Send to map channel
	if cm.mapChan != nil {
		select {
		case cm.mapChan <- mapBytes:
		default:
		}
	}
}

// PollMap requests and returns the current map as PNG and parsed data.
func (cm *CloudMQTT) PollMap() ([]byte, *MapData, error) {
	payload, _, security, err := BuildGetMapPayload()
	if err != nil {
		return nil, nil, err
	}

	cm.mapChan = make(chan []byte, 1)
	cm.mapSecurity = security
	defer func() {
		cm.mapSecurity = nil
		cm.mapChan = nil
	}()

	if err := cm.SendCommandNoWait(payload); err != nil {
		return nil, nil, fmt.Errorf("send map request: %w", err)
	}

	select {
	case mapBytes := <-cm.mapChan:
		mapData, err := ParseMapData(mapBytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parse map: %w", err)
		}

		pngData, err := RenderMapPNG(mapData)
		if err != nil {
			return nil, nil, fmt.Errorf("render map: %w", err)
		}

		return pngData, mapData, nil
	case <-time.After(30 * time.Second):
		return nil, nil, fmt.Errorf("map request timed out after 30s")
	}
}
