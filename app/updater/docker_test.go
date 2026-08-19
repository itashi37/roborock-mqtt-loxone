package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyVersionChecksExactEdgeCommit(t *testing.T) {
	commit := "abcdef1234567890abcdef1234567890abcdef12"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"edge","git_commit":"` + commit + `"}`))
	}))
	defer server.Close()

	engine := NewDockerEngine("/unused")
	engine.httpClient = server.Client()
	if err := engine.VerifyVersion(context.Background(), server.URL, "edge", commit[:12]); err != nil {
		t.Fatalf("matching commit rejected: %v", err)
	}
	if err := engine.VerifyVersion(context.Background(), server.URL, "edge", "1234567890ab"); err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("wrong commit was not rejected: %v", err)
	}
}

func TestVerifyVersionRejectsMissingCommit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"edge"}`))
	}))
	defer server.Close()

	engine := NewDockerEngine("/unused")
	engine.httpClient = server.Client()
	if err := engine.VerifyVersion(context.Background(), server.URL, "edge", "abcdef123456"); err == nil {
		t.Fatal("missing commit was accepted")
	}
}
