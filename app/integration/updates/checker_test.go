package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckerStableAndEdge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","published_at":"2026-08-19T08:00:00Z","body":"Fixes","html_url":"https://example/release"}`))
		case "/commits/main":
			_, _ = w.Write([]byte(`{"sha":"abcdef1234567890","html_url":"https://example/commit","commit":{"message":"Edge changes","committer":{"date":"2026-08-19T09:00:00Z"}}}`))
		case "/actions/workflows/publish.yml/runs":
			_, _ = w.Write([]byte(`{"workflow_runs":[{"head_sha":"abcdef1234567890","status":"completed","conclusion":"success","html_url":"https://example/run"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	checker := NewChecker("1.2.2", "old")
	checker.SetHTTPClient(server.Client(), server.URL)
	stable, err := checker.Check(context.Background(), "stable")
	if err != nil || !stable.Available || stable.LatestVersion != "1.2.3" {
		t.Fatalf("stable=%+v err=%v", stable, err)
	}
	edge, err := checker.Check(context.Background(), "edge")
	if err != nil || !edge.Available || edge.LatestVersion != "edge-abcdef123456" || !edge.ArtifactReady || edge.LatestCommit != "abcdef1234567890" {
		t.Fatalf("edge=%+v err=%v", edge, err)
	}
}

func TestCheckerEdgeWaitsForMatchingImageWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/commits/main":
			_, _ = w.Write([]byte(`{"sha":"newcommit1234567890","commit":{"message":"Pending","committer":{"date":"2026-08-19T09:00:00Z"}}}`))
		case "/actions/workflows/publish.yml/runs":
			_, _ = w.Write([]byte(`{"workflow_runs":[{"head_sha":"oldcommit1234567890","status":"completed","conclusion":"success"},{"head_sha":"newcommit1234567890","status":"in_progress","conclusion":""}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	checker := NewChecker("edge", "oldcommit")
	checker.SetHTTPClient(server.Client(), server.URL)
	info, err := checker.Check(context.Background(), "edge")
	if err != nil || !info.Available || info.ArtifactReady || info.ArtifactStatus != "in_progress" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestCompareSemVer(t *testing.T) {
	if compareSemVer("1.2.3", "1.2.2") <= 0 || compareSemVer("1.2.3", "1.2.3") != 0 || compareSemVer("1.2.3", "2.0.0") >= 0 {
		t.Fatal("unexpected semantic version ordering")
	}
}
