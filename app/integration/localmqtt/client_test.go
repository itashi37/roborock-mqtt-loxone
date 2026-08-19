package localmqtt

import (
	"testing"

	"github.com/mqtt-home/roborock-mqtt/config"
)

func TestClientRejectsEmptyBrokerWithoutChangingState(t *testing.T) {
	client := New()
	if err := client.Start(config.MQTTConfig{}); err == nil {
		t.Fatal("expected empty broker URL to be rejected")
	}
	if client.IsConnected() || client.Diagnostics().Enabled {
		t.Fatalf("unexpected diagnostics after rejected start: %+v", client.Diagnostics())
	}
}

func TestSubscriptionsCanBeRebuiltWhileDisconnected(t *testing.T) {
	client := New()
	if err := client.Subscribe("home/robot/command", func(string, []byte) {}); err != nil {
		t.Fatal(err)
	}
	if got := client.Diagnostics().Subscriptions; got != 1 {
		t.Fatalf("subscriptions = %d", got)
	}
	client.ClearSubscriptions()
	if got := client.Diagnostics().Subscriptions; got != 0 {
		t.Fatalf("subscriptions after clear = %d", got)
	}
}

func TestClientOptionsConfigureRetainedBridgeLastWill(t *testing.T) {
	client := New()
	options := client.clientOptions(config.MQTTConfig{QoS: 1}, "tcp://broker:1883", &LastWill{
		Topic: "loxone/roborock/_bridge/bridge_alive", OfflinePayload: "0", OnlinePayload: "1", Retained: true,
	})
	if !options.WillEnabled || options.WillTopic != "loxone/roborock/_bridge/bridge_alive" || string(options.WillPayload) != "0" || options.WillQos != 1 || !options.WillRetained {
		t.Fatalf("unexpected MQTT Last Will options: %+v", options)
	}
}
