package localmqtt

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/mqtt-home/roborock-mqtt/config"
)

type MessageHandler func(topic string, payload []byte)

// LastWill configures the bridge availability message published by the broker
// when this client disappears without a clean MQTT disconnect.
type LastWill struct {
	Topic          string
	OfflinePayload string
	OnlinePayload  string
	Retained       bool
}

type Diagnostics struct {
	Enabled       bool      `json:"enabled"`
	Connected     bool      `json:"connected"`
	ConnectedAt   time.Time `json:"connected_at,omitempty"`
	LastAttempt   time.Time `json:"last_attempt,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	Reconnects    int       `json:"reconnects"`
	Subscriptions int       `json:"subscriptions"`
}

type Client struct {
	mu            sync.RWMutex
	client        paho.Client
	config        config.MQTTConfig
	subscriptions map[string]MessageHandler
	diagnostics   Diagnostics
	lastWill      *LastWill
}

func New() *Client {
	return &Client{subscriptions: make(map[string]MessageHandler)}
}

func (c *Client) Start(cfg config.MQTTConfig) error {
	return c.StartWithWill(cfg, nil)
}

// StartWithWill starts the client and configures an optional retained bridge
// availability marker. Stop publishes the offline marker explicitly; an
// ungraceful loss is handled by the broker-side Last Will.
func (c *Client) StartWithWill(cfg config.MQTTConfig, lastWill *LastWill) error {
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("local MQTT broker URL is required")
	}
	c.Stop()
	c.mu.Lock()
	c.config = cfg
	c.lastWill = cloneLastWill(lastWill)
	c.diagnostics.Enabled = true
	c.diagnostics.LastAttempt = time.Now()
	c.diagnostics.LastError = ""
	c.mu.Unlock()

	brokerURL := cfg.URL
	if cfg.TLS && strings.HasPrefix(brokerURL, "tcp://") {
		brokerURL = "ssl://" + strings.TrimPrefix(brokerURL, "tcp://")
	}
	options := c.clientOptions(cfg, brokerURL, lastWill)
	client := paho.NewClient(options)
	c.mu.Lock()
	c.client = client
	c.mu.Unlock()
	token := client.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		c.recordError("local MQTT connection timed out")
		client.Disconnect(250)
		return fmt.Errorf("local MQTT connection timed out")
	}
	if err := token.Error(); err != nil {
		c.recordError(err.Error())
		return fmt.Errorf("connect local MQTT: %w", err)
	}
	c.Publish(strings.TrimSuffix(cfg.Topic, "/")+"/bridge/state", "online", true)
	return nil
}

func (c *Client) clientOptions(cfg config.MQTTConfig, brokerURL string, lastWill *LastWill) *paho.ClientOptions {
	options := paho.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(fmt.Sprintf("roborock_mqtt_%08x", rand.Uint32())).
		SetUsername(cfg.Username).
		SetPassword(cfg.Password).
		SetAutoReconnect(true).
		SetConnectRetry(false).
		SetKeepAlive(60 * time.Second).
		SetTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12})
	if lastWill != nil && strings.TrimSpace(lastWill.Topic) != "" {
		options.SetWill(lastWill.Topic, lastWill.OfflinePayload, cfg.QoS, lastWill.Retained)
	}
	options.SetOnConnectHandler(func(client paho.Client) { c.onConnect(client) })
	options.SetConnectionLostHandler(func(_ paho.Client, err error) {
		c.mu.Lock()
		c.diagnostics.Connected = false
		c.diagnostics.LastError = err.Error()
		c.mu.Unlock()
	})
	return options
}

func (c *Client) onConnect(client paho.Client) {
	c.mu.Lock()
	if !c.diagnostics.Connected && !c.diagnostics.ConnectedAt.IsZero() {
		c.diagnostics.Reconnects++
	}
	c.diagnostics.Connected = true
	c.diagnostics.ConnectedAt = time.Now()
	c.diagnostics.LastError = ""
	subscriptions := make(map[string]MessageHandler, len(c.subscriptions))
	for topic, handler := range c.subscriptions {
		subscriptions[topic] = handler
	}
	qos := c.config.QoS
	lastWill := cloneLastWill(c.lastWill)
	c.mu.Unlock()
	if lastWill != nil && strings.TrimSpace(lastWill.Topic) != "" {
		client.Publish(lastWill.Topic, qos, lastWill.Retained, lastWill.OnlinePayload).Wait()
	}
	for topic, handler := range subscriptions {
		h := handler
		client.Subscribe(topic, qos, func(_ paho.Client, message paho.Message) { h(message.Topic(), message.Payload()) })
	}
}

func (c *Client) Stop() {
	c.mu.Lock()
	client := c.client
	lastWill := cloneLastWill(c.lastWill)
	qos := c.config.QoS
	c.client = nil
	c.lastWill = nil
	c.diagnostics.Connected = false
	c.diagnostics.Enabled = false
	c.mu.Unlock()
	if client != nil && client.IsConnected() {
		if lastWill != nil && strings.TrimSpace(lastWill.Topic) != "" {
			client.Publish(lastWill.Topic, qos, lastWill.Retained, lastWill.OfflinePayload).Wait()
		}
		client.Disconnect(500)
	}
}

func cloneLastWill(value *LastWill) *LastWill {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (c *Client) ClearSubscriptions() {
	c.mu.Lock()
	c.subscriptions = make(map[string]MessageHandler)
	c.diagnostics.Subscriptions = 0
	c.mu.Unlock()
}

func (c *Client) Subscribe(topic string, handler MessageHandler) error {
	if strings.TrimSpace(topic) == "" || handler == nil {
		return fmt.Errorf("invalid local MQTT subscription")
	}
	c.mu.Lock()
	c.subscriptions[topic] = handler
	c.diagnostics.Subscriptions = len(c.subscriptions)
	client := c.client
	qos := c.config.QoS
	c.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return nil
	}
	token := client.Subscribe(topic, qos, func(_ paho.Client, message paho.Message) { handler(message.Topic(), message.Payload()) })
	token.Wait()
	return token.Error()
}

func (c *Client) Publish(topic string, payload any, retained bool) error {
	c.mu.RLock()
	client := c.client
	qos := c.config.QoS
	c.mu.RUnlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("local MQTT broker is disconnected")
	}
	token := client.Publish(topic, qos, retained, payload)
	token.Wait()
	return token.Error()
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client != nil && c.client.IsConnected()
}

func (c *Client) Diagnostics() Diagnostics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.diagnostics
}

func (c *Client) Test(ctx context.Context, topic string) error {
	nonce := fmt.Sprintf("%x", time.Now().UnixNano())
	received := make(chan struct{}, 1)
	if err := c.Subscribe(topic, func(_ string, payload []byte) {
		if string(payload) == nonce {
			select {
			case received <- struct{}{}:
			default:
			}
		}
	}); err != nil {
		return err
	}
	if err := c.Publish(topic, nonce, false); err != nil {
		return err
	}
	select {
	case <-received:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("local MQTT loopback timed out: %w", ctx.Err())
	}
}

func (c *Client) recordError(message string) {
	c.mu.Lock()
	c.diagnostics.Connected = false
	c.diagnostics.LastError = message
	c.mu.Unlock()
}
