package direct

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var loxoneSuccessCode = regexp.MustCompile(`(?i)(?:code(?:&quot;|["'])?\s*[:=]\s*(?:["'])?200\b)`)

type ClientConfig struct {
	Scheme   string
	Host     string
	Port     int
	Username string
	Password string
	Timeout  time.Duration
}

type ValuePusher interface {
	Push(context.Context, string, string) error
}

type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

func NewClient(config ClientConfig) (*Client, error) {
	scheme := strings.ToLower(strings.TrimSpace(config.Scheme))
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("unsupported Loxone scheme %q", config.Scheme)
	}
	host := strings.Trim(strings.TrimSpace(config.Host), "[]")
	if host == "" {
		return nil, fmt.Errorf("Loxone Miniserver host is required")
	}
	if config.Port <= 0 || config.Port > 65535 {
		return nil, fmt.Errorf("invalid Loxone Miniserver port %d", config.Port)
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL:  scheme + "://" + net.JoinHostPort(host, strconv.Itoa(config.Port)),
		username: config.Username, password: config.Password,
		http: &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) Push(ctx context.Context, input, value string) error {
	endpoint := c.baseURL + "/dev/sps/io/" + url.PathEscape(input) + "/" + url.PathEscape(value)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create Loxone request: %w", err)
	}
	if c.username != "" || c.password != "" {
		request.SetBasicAuth(c.username, c.password)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("send Loxone value: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("read Loxone response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Loxone HTTP status %d", response.StatusCode)
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return fmt.Errorf("empty Loxone response")
	}
	if !loxoneSuccessCode.Match(trimmed) {
		return fmt.Errorf("Loxone rejected value")
	}
	return nil
}

// Test verifies that the Miniserver is reachable and accepts the configured
// credentials without changing a Virtual Input.
func (c *Client) Test(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/dev/sps/state", nil)
	if err != nil {
		return fmt.Errorf("create Loxone test request: %w", err)
	}
	if c.username != "" || c.password != "" {
		request.SetBasicAuth(c.username, c.password)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("connect to Loxone Miniserver: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Loxone HTTP status %d", response.StatusCode)
	}
	return nil
}
