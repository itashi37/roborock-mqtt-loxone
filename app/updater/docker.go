package updater

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mqtt-home/roborock-mqtt/supervisor"
)

const dockerAPIVersion = "v1.43"

type DockerEngine struct {
	client     *http.Client
	apiBase    string
	httpClient *http.Client
}

func NewDockerEngine(socketPath string) *DockerEngine {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socketPath)
	}}
	return &DockerEngine{
		client:     &http.Client{Transport: transport, Timeout: 10 * time.Minute},
		apiBase:    "http://docker/" + dockerAPIVersion,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type dockerInspect struct {
	ID         string         `json:"Id"`
	Name       string         `json:"Name"`
	Config     map[string]any `json:"Config"`
	HostConfig map[string]any `json:"HostConfig"`
	State      struct {
		Running bool `json:"Running"`
		Health  *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	NetworkSettings struct {
		Networks map[string]struct {
			IPAMConfig any               `json:"IPAMConfig,omitempty"`
			Links      []string          `json:"Links,omitempty"`
			Aliases    []string          `json:"Aliases,omitempty"`
			DriverOpts map[string]string `json:"DriverOpts,omitempty"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

func (d *DockerEngine) CurrentArtifact(ctx context.Context, container string) (string, error) {
	inspect, err := d.inspect(ctx, container)
	if err != nil {
		return "", err
	}
	image, _ := inspect.Config["Image"].(string)
	if image == "" {
		return "", fmt.Errorf("current image is missing")
	}
	return image, nil
}

func (d *DockerEngine) Fetch(ctx context.Context, image string) error {
	if !strings.HasPrefix(image, AllowedImage+":") {
		return fmt.Errorf("image is outside the allowlist")
	}
	values := url.Values{"fromImage": {AllowedImage}, "tag": {strings.TrimPrefix(image, AllowedImage+":")}}
	response, err := d.request(ctx, http.MethodPost, "/images/create?"+values.Encode(), nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return dockerError(response)
	}
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		var event struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Error != "" {
			return fmt.Errorf("Docker pull failed: %s", event.Error)
		}
	}
	return scanner.Err()
}

func (d *DockerEngine) Prepare(ctx context.Context, container, image string) (supervisor.Replacement, error) {
	inspect, err := d.inspect(ctx, container)
	if err != nil {
		return supervisor.Replacement{}, err
	}
	previousImage, _ := inspect.Config["Image"].(string)
	return supervisor.Replacement{ServiceName: container, PreviousInstance: inspect.ID, PreviousArtifact: previousImage, TargetArtifact: image, PreviousName: container + "-rollback-" + time.Now().UTC().Format("20060102T150405")}, nil
}

func (d *DockerEngine) Activate(ctx context.Context, replacement *supervisor.Replacement) error {
	inspect, err := d.inspect(ctx, replacement.PreviousInstance)
	if err != nil {
		return err
	}
	if err := d.simple(ctx, http.MethodPost, "/containers/"+url.PathEscape(inspect.ID)+"/stop?t=20"); err != nil {
		return err
	}
	if err := d.simple(ctx, http.MethodPost, "/containers/"+url.PathEscape(inspect.ID)+"/rename?name="+url.QueryEscape(replacement.PreviousName)); err != nil {
		_ = d.simple(ctx, http.MethodPost, "/containers/"+url.PathEscape(inspect.ID)+"/start")
		return err
	}
	inspect.Config["Image"] = replacement.TargetArtifact
	endpoints := make(map[string]any, len(inspect.NetworkSettings.Networks))
	for name, endpoint := range inspect.NetworkSettings.Networks {
		endpoints[name] = endpoint
	}
	body := map[string]any{"Image": replacement.TargetArtifact, "HostConfig": inspect.HostConfig, "NetworkingConfig": map[string]any{"EndpointsConfig": endpoints}}
	for key, value := range inspect.Config {
		body[key] = value
	}
	var created struct {
		ID string `json:"Id"`
	}
	if err := d.jsonRequest(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(replacement.ServiceName), body, &created); err != nil {
		return err
	}
	replacement.NewInstance = created.ID
	if err := d.simple(ctx, http.MethodPost, "/containers/"+url.PathEscape(created.ID)+"/start"); err != nil {
		_ = d.simple(context.Background(), http.MethodDelete, "/containers/"+url.PathEscape(created.ID)+"?force=true")
		return err
	}
	return nil
}

func (d *DockerEngine) WaitHealthy(ctx context.Context, container string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		inspect, err := d.inspect(ctx, container)
		if err == nil && inspect.State.Running && inspect.State.Health != nil {
			switch inspect.State.Health.Status {
			case "healthy":
				return nil
			case "unhealthy":
				return fmt.Errorf("Docker health status is unhealthy")
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return fmt.Errorf("healthcheck timed out after %s", timeout)
}

func (d *DockerEngine) VerifyVersion(ctx context.Context, endpoint, expectedVersion, expectedCommit string) error {
	if endpoint == "" {
		return fmt.Errorf("bridge status URL is missing")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := d.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("bridge status returned HTTP %d", response.StatusCode)
	}
	var status struct {
		Version   string `json:"version"`
		GitCommit string `json:"git_commit"`
	}
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return err
	}
	if strings.TrimPrefix(status.Version, "v") != strings.TrimPrefix(expectedVersion, "v") {
		return fmt.Errorf("got version %q, expected %q", status.Version, expectedVersion)
	}
	actualCommit := strings.ToLower(strings.TrimSpace(status.GitCommit))
	expectedCommit = strings.ToLower(strings.TrimSpace(expectedCommit))
	if expectedCommit != "" {
		if actualCommit == "" || !strings.HasPrefix(actualCommit, expectedCommit) && !strings.HasPrefix(expectedCommit, actualCommit) {
			return fmt.Errorf("got commit %q, expected %q", status.GitCommit, expectedCommit)
		}
	}
	return nil
}

func (d *DockerEngine) Rollback(ctx context.Context, replacement supervisor.Replacement) error {
	if current, err := d.inspect(ctx, replacement.ServiceName); err == nil && current.ID != replacement.PreviousInstance {
		_ = d.simple(ctx, http.MethodPost, "/containers/"+url.PathEscape(current.ID)+"/stop?t=10")
		if err := d.simple(ctx, http.MethodDelete, "/containers/"+url.PathEscape(current.ID)+"?force=true"); err != nil {
			return err
		}
	}
	previous, err := d.inspect(ctx, replacement.PreviousInstance)
	if err != nil {
		return err
	}
	if strings.TrimPrefix(previous.Name, "/") != replacement.ServiceName {
		if err := d.simple(ctx, http.MethodPost, "/containers/"+url.PathEscape(replacement.PreviousInstance)+"/rename?name="+url.QueryEscape(replacement.ServiceName)); err != nil {
			return err
		}
	}
	if previous.State.Running {
		return nil
	}
	return d.simple(ctx, http.MethodPost, "/containers/"+url.PathEscape(replacement.PreviousInstance)+"/start")
}

func (d *DockerEngine) Finalize(ctx context.Context, replacement supervisor.Replacement) error {
	return d.simple(ctx, http.MethodDelete, "/containers/"+url.PathEscape(replacement.PreviousInstance)+"?force=true")
}

func (d *DockerEngine) inspect(ctx context.Context, container string) (dockerInspect, error) {
	var result dockerInspect
	err := d.jsonRequest(ctx, http.MethodGet, "/containers/"+url.PathEscape(container)+"/json", nil, &result)
	return result, err
}

func (d *DockerEngine) simple(ctx context.Context, method, path string) error {
	response, err := d.request(ctx, method, path, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return dockerError(response)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	return nil
}

func (d *DockerEngine) jsonRequest(ctx context.Context, method, path string, body any, target any) error {
	response, err := d.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return dockerError(response)
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func (d *DockerEngine) request(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		pipeReader, pipeWriter := io.Pipe()
		go func() { pipeWriter.CloseWithError(json.NewEncoder(pipeWriter).Encode(body)) }()
		reader = pipeReader
	}
	request, err := http.NewRequestWithContext(ctx, method, d.apiBase+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return d.client.Do(request)
}

func dockerError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	return fmt.Errorf("Docker API HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
}
