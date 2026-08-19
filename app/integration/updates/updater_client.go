package updates

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mqtt-home/roborock-mqtt/updater"
)

type UpdaterClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewUpdaterClient(baseURL, token string) (*UpdaterClient, error) {
	baseURL, token = strings.TrimSuffix(strings.TrimSpace(baseURL), "/"), strings.TrimSpace(token)
	if baseURL == "" {
		return nil, fmt.Errorf("updater URL is missing")
	}
	if len(token) < 32 {
		return nil, fmt.Errorf("updater token is missing or invalid")
	}
	return &UpdaterClient{baseURL: baseURL, token: token, client: &http.Client{Timeout: 12 * time.Second}}, nil
}

func (c *UpdaterClient) SetHTTPClient(client *http.Client) {
	if client != nil {
		c.client = client
	}
}

func (c *UpdaterClient) Status(ctx context.Context) (updater.Operation, error) {
	var operation updater.Operation
	err := c.request(ctx, http.MethodGet, "/v1/operations/current", nil, &operation)
	return operation, err
}

func (c *UpdaterClient) Start(ctx context.Context, request updater.Request) (updater.Operation, error) {
	var operation updater.Operation
	err := c.request(ctx, http.MethodPost, "/v1/updates", request, &operation)
	return operation, err
}

func (c *UpdaterClient) request(ctx context.Context, method, path string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("updater HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.NewDecoder(response.Body).Decode(target)
}
