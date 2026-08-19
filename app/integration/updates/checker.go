package updates

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultRepositoryAPI = "https://api.github.com/repos/itashi37/roborock-mqtt-loxone"

type Info struct {
	Channel        string    `json:"channel"`
	CurrentVersion string    `json:"current_version"`
	LatestVersion  string    `json:"latest_version,omitempty"`
	LatestCommit   string    `json:"latest_commit,omitempty"`
	ArtifactReady  bool      `json:"artifact_ready"`
	ArtifactStatus string    `json:"artifact_status,omitempty"`
	PublishedAt    time.Time `json:"published_at,omitempty"`
	ReleaseNotes   string    `json:"release_notes,omitempty"`
	ReleaseURL     string    `json:"release_url,omitempty"`
	Available      bool      `json:"available"`
	CheckedAt      time.Time `json:"checked_at,omitempty"`
	Error          string    `json:"error,omitempty"`
}

type Checker struct {
	client         *http.Client
	repositoryAPI  string
	currentVersion string
	currentCommit  string
	mu             sync.RWMutex
	last           Info
}

func NewChecker(currentVersion, currentCommit string) *Checker {
	return &Checker{
		client: &http.Client{Timeout: 8 * time.Second}, repositoryAPI: defaultRepositoryAPI,
		currentVersion: strings.TrimSpace(currentVersion), currentCommit: strings.TrimSpace(currentCommit),
	}
}

func (c *Checker) SetHTTPClient(client *http.Client, repositoryAPI string) {
	if client != nil {
		c.client = client
	}
	if strings.TrimSpace(repositoryAPI) != "" {
		c.repositoryAPI = strings.TrimSuffix(repositoryAPI, "/")
	}
}

func (c *Checker) Last() Info {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.last
}

func (c *Checker) Check(ctx context.Context, channel string) (Info, error) {
	channel = normalizeChannel(channel)
	info := Info{Channel: channel, CurrentVersion: c.currentVersion, CheckedAt: time.Now().UTC()}
	var err error
	if channel == "edge" {
		err = c.checkEdge(ctx, &info)
	} else {
		err = c.checkStable(ctx, &info)
	}
	if err != nil {
		info.Error = err.Error()
	}
	c.mu.Lock()
	c.last = info
	c.mu.Unlock()
	return info, err
}

func normalizeChannel(channel string) string {
	if strings.EqualFold(strings.TrimSpace(channel), "edge") || strings.EqualFold(strings.TrimSpace(channel), "beta") {
		return "edge"
	}
	return "stable"
}

func (c *Checker) checkStable(ctx context.Context, info *Info) error {
	var release struct {
		TagName     string    `json:"tag_name"`
		PublishedAt time.Time `json:"published_at"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
	}
	if err := c.getJSON(ctx, c.repositoryAPI+"/releases/latest", &release); err != nil {
		return fmt.Errorf("check latest stable release: %w", err)
	}
	info.LatestVersion = strings.TrimPrefix(release.TagName, "v")
	info.PublishedAt, info.ReleaseURL = release.PublishedAt, release.HTMLURL
	info.ReleaseNotes = limitText(release.Body, 20000)
	info.Available = compareSemVer(info.LatestVersion, strings.TrimPrefix(c.currentVersion, "v")) > 0
	// Stable releases are created only after the corresponding image has been
	// published by the release workflow.
	info.ArtifactReady, info.ArtifactStatus = true, "published"
	return nil
}

func (c *Checker) checkEdge(ctx context.Context, info *Info) error {
	var commit struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message   string `json:"message"`
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := c.getJSON(ctx, c.repositoryAPI+"/commits/main", &commit); err != nil {
		return fmt.Errorf("check latest edge build: %w", err)
	}
	short := commit.SHA
	if len(short) > 12 {
		short = short[:12]
	}
	info.LatestVersion = "edge-" + short
	info.LatestCommit = commit.SHA
	info.PublishedAt, info.ReleaseURL = commit.Commit.Committer.Date, commit.HTMLURL
	info.ReleaseNotes = limitText(commit.Commit.Message, 20000)
	info.Available = c.currentCommit == "" || c.currentCommit == "unknown" || !strings.HasPrefix(commit.SHA, c.currentCommit) && !strings.HasPrefix(c.currentCommit, commit.SHA)

	var runs struct {
		WorkflowRuns []struct {
			HeadSHA    string `json:"head_sha"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HTMLURL    string `json:"html_url"`
		} `json:"workflow_runs"`
	}
	endpoint := c.repositoryAPI + "/actions/workflows/publish.yml/runs?branch=main&event=push&per_page=20"
	if err := c.getJSON(ctx, endpoint, &runs); err != nil {
		return fmt.Errorf("verify latest edge image publication: %w", err)
	}
	info.ArtifactStatus = "not_found"
	for _, run := range runs.WorkflowRuns {
		if run.HeadSHA != commit.SHA {
			continue
		}
		if run.HTMLURL != "" {
			info.ReleaseURL = run.HTMLURL
		}
		if run.Status == "completed" {
			info.ArtifactStatus = run.Conclusion
			info.ArtifactReady = run.Conclusion == "success"
		} else {
			info.ArtifactStatus = run.Status
		}
		break
	}
	return nil
}

func (c *Checker) getJSON(ctx context.Context, endpoint string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "roborock-mqtt-loxone")
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("GitHub returned HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func compareSemVer(left, right string) int {
	a := semVerParts(left)
	b := semVerParts(right)
	for index := 0; index < 3; index++ {
		if a[index] < b[index] {
			return -1
		}
		if a[index] > b[index] {
			return 1
		}
	}
	return 0
}

func semVerParts(value string) [3]int {
	value = strings.SplitN(value, "-", 2)[0]
	pieces := strings.Split(value, ".")
	var result [3]int
	for index := 0; index < len(pieces) && index < len(result); index++ {
		result[index], _ = strconv.Atoi(pieces[index])
	}
	return result
}

func limitText(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum] + "…"
}
