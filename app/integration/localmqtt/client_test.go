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
