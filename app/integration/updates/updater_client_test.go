package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqtt-home/roborock-mqtt/updater"
)

func TestUpdaterClientAuthenticatesAndDoesNotExposeToken(t *testing.T) {
	token := strings.Repeat("s", 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatal("missing updater authorization")
		}
		_, _ = w.Write([]byte(`{"id":"operation","stage":"preparing"}`))
	}))
	defer server.Close()
	client, err := NewUpdaterClient(server.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := client.Start(context.Background(), updater.Request{RequestID: "request-123", Tag: "v1.0.0", ExpectedVersion: "1.0.0"})
	if err != nil || operation.ID != "operation" {
		t.Fatalf("operation=%+v err=%v", operation, err)
	}
}
