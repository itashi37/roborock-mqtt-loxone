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
}

func New() *Client {
	return &Client{subscriptions: make(map[string]MessageHandler)}
}

func (c *Client) Start(cfg config.MQTTConfig) error {
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("local MQTT broker URL is required")
	}
	c.Stop()
	c.mu.Lock()
	c.config = cfg
	c.diagnostics.Enabled = true
	c.diagnostics.LastAttempt = time.Now()
	c.diagnostics.LastError = ""
	c.mu.Unlock()

	brokerURL := cfg.URL
	if cfg.TLS && strings.HasPrefix(brokerURL, "tcp://") {
		brokerURL = "ssl://" + strings.TrimPrefix(brokerURL, "tcp://")
	}
	options := paho.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(fmt.Sprintf("roborock_mqtt_%08x", rand.Uint32())).
		SetUsername(cfg.Username).
		SetPassword(cfg.Password).
		SetAutoReconnect(true).
		SetConnectRetry(false).
		SetKeepAlive(60 * time.Second).
		SetTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12})
	options.SetOnConnectHandler(func(client paho.Client) { c.onConnect(client) })
	options.SetConnectionLostHandler(func(_ paho.Client, err error) {
		c.mu.Lock()
		c.diagnostics.Connected = false
		c.diagnostics.LastError = err.Error()
		c.mu.Unlock()
	})
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
	c.mu.Unlock()
	for topic, handler := range subscriptions {
		h := handler
		client.Subscribe(topic, qos, func(_ paho.Client, message paho.Message) { h(message.Topic(), message.Payload()) })
	}
}

func (c *Client) Stop() {
	c.mu.Lock()
	client := c.client
	c.client = nil
	c.diagnostics.Connected = false
	c.diagnostics.Enabled = false
	c.mu.Unlock()
	if client != nil && client.IsConnected() {
		client.Disconnect(500)
	}
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
