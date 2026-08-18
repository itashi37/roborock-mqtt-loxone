package direct

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestClientPushUsesBasicAuthAndEscapedVirtualInput(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		path = request.URL.EscapedPath()
		username, password, ok := request.BasicAuth()
		if !ok || username != "bridge" || password != "secret" {
			t.Fatalf("unexpected basic auth: %q %q %v", username, password, ok)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<LL control="dev/sps/io/Test" value="1" Code="200"/>`))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(parsed.Port())
	client, err := NewClient(ClientConfig{Scheme: "http", Host: parsed.Hostname(), Port: port, Username: "bridge", Password: "secret", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Push(context.Background(), "Pièce robot", "Cuisine/Salon"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, "Pi%C3%A8ce%20robot") || !strings.Contains(path, "Cuisine%2FSalon") {
		t.Fatalf("path was not safely escaped: %s", path)
	}
}

func TestClientPushRejectsLoxoneErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<LL Code="401"/>`))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(parsed.Port())
	client, err := NewClient(ClientConfig{Scheme: "http", Host: parsed.Hostname(), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Push(context.Background(), "Input", "1"); err == nil || strings.Contains(err.Error(), "401") {
		// The response body may contain sensitive server details and must not be
		// copied into diagnostics; expose only a generic rejection.
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientTestDoesNotWriteVirtualInput(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(parsed.Port())
	client, err := NewClient(ClientConfig{Scheme: "http", Host: parsed.Hostname(), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Test(context.Background()); err != nil {
		t.Fatal(err)
	}
	if path != "/dev/sps/state" {
		t.Fatalf("test path = %q", path)
	}
}
