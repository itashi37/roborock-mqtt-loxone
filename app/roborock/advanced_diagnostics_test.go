package roborock

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseAdvancedDiagnosticsAllowListsFieldsAndRemovesSecrets(t *testing.T) {
	input := []byte(`[{"dock_type":3,"charge_status":1,"feature_flags":{"support_find_me":1,"support_dryer":true,"token":"leak"},"localKey":"leak","mqtt":{"password":"leak"},"unrelated":"private"}]`)
	diagnostics, err := ParseAdvancedDiagnostics(input, time.Unix(10, 0))
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Fields["dock_type"] != float64(3) || diagnostics.Fields["support_find_me"] != float64(1) {
		t.Fatalf("expected fields missing: %+v", diagnostics.Fields)
	}
	serialized, _ := json.Marshal(diagnostics)
	for _, forbidden := range []string{"leak", "localKey", "mqtt", "unrelated"} {
		if string(serialized) != "" && containsString(string(serialized), forbidden) {
			t.Fatalf("diagnostics leaked %q: %s", forbidden, serialized)
		}
	}
}

func containsString(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

func TestAdvancedCommandPayloadMethodsAndDryerParameter(t *testing.T) {
	tests := []struct {
		name   string
		build  func() ([]byte, int, error)
		method string
	}{
		{"stop", BuildStopPayload, "app_stop"}, {"locate", BuildLocatePayload, "find_me"},
		{"empty", BuildStartDustCollectionPayload, "app_start_collect_dust"},
		{"stop empty", BuildStopDustCollectionPayload, "app_stop_collect_dust"},
		{"wash", BuildStartWashPayload, "app_start_wash"}, {"stop wash", BuildStopWashPayload, "app_stop_wash"},
		{"init", BuildAppGetInitStatusPayload, "app_get_init_status"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, _, err := test.build()
			if err != nil {
				t.Fatal(err)
			}
			request := decodeIPCRequest(t, payload)
			if request.Method != test.method {
				t.Fatalf("method = %q", request.Method)
			}
		})
	}
	payload, _, err := BuildSetDryerStatusPayload(true)
	if err != nil {
		t.Fatal(err)
	}
	request := decodeIPCRequest(t, payload)
	if request.Method != "app_set_dryer_status" {
		t.Fatalf("dryer method = %q", request.Method)
	}
	params, _ := json.Marshal(request.Params)
	if string(params) != `[{"status":1}]` {
		t.Fatalf("dryer params = %s", params)
	}
}

func decodeIPCRequest(t *testing.T, payload []byte) IPCRequest {
	t.Helper()
	var message MQTTMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatal(err)
	}
	raw, ok := message.DPS["101"].(string)
	if !ok {
		t.Fatalf("missing IPC payload: %+v", message)
	}
	var request IPCRequest
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		t.Fatal(err)
	}
	return request
}
