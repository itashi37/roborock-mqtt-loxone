package web

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mqtt-home/roborock-mqtt/config"
)

const directCommandBodyLimit = 4096

type directRateWindow struct {
	started time.Time
	count   int
}

type directCommandLimiter struct {
	mu      sync.Mutex
	windows map[string]directRateWindow
}

func (l *directCommandLimiter) allow(key string, limit int, now time.Time) bool {
	if limit <= 0 {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	window := l.windows[key]
	if window.started.IsZero() || now.Sub(window.started) >= time.Minute {
		window = directRateWindow{started: now}
	}
	if window.count >= limit {
		l.windows[key] = window
		return false
	}
	window.count++
	l.windows[key] = window
	return true
}

func (ws *WebServer) directAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		cfg := config.Get().Loxone.Direct
		if !cfg.Enabled {
			writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("Direct Loxone integration is disabled"))
			return
		}
		if len(cfg.APIToken) < 32 {
			writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("Direct command API token is not configured securely"))
			return
		}
		if !directRemoteAllowed(request.RemoteAddr, cfg.AllowedCIDRs) {
			writeLoxoneError(w, http.StatusForbidden, fmt.Errorf("source address is not allowed"))
			return
		}
		if !directAuthenticated(request, cfg.APIUsername, cfg.APIToken) {
			w.Header().Set("WWW-Authenticate", `Basic realm="roborock-mqtt-loxone"`)
			writeLoxoneError(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
			return
		}
		next(w, request)
	}
}

func directAuthenticated(request *http.Request, username, token string) bool {
	if basicUser, basicPassword, ok := request.BasicAuth(); ok {
		return constantTimeEqual(basicUser, username) && constantTimeEqual(basicPassword, token)
	}
	authorization := strings.TrimSpace(request.Header.Get("Authorization"))
	if len(authorization) > 7 && strings.EqualFold(authorization[:7], "Bearer ") {
		return constantTimeEqual(strings.TrimSpace(authorization[7:]), token)
	}
	return false
}

func constantTimeEqual(actual, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func directRemoteAllowed(remoteAddress string, allowedCIDRs []string) bool {
	if len(allowedCIDRs) == 0 {
		return true
	}
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		host = remoteAddress
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	for _, raw := range allowedCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(raw))
		if err != nil {
			return false // fail closed on an invalid security configuration
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (ws *WebServer) loxoneDirectCanonicalCommand(w http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(w, request.Body, directCommandBodyLimit)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var body struct {
		Command string `json:"command"`
	}
	if err := decoder.Decode(&body); err != nil || strings.TrimSpace(body.Command) == "" {
		writeLoxoneError(w, http.StatusBadRequest, fmt.Errorf("a valid command is required"))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeLoxoneError(w, http.StatusBadRequest, fmt.Errorf("request must contain one JSON object"))
		return
	}
	ws.submitDirectCommand(w, request, body.Command)
}

func (ws *WebServer) loxoneDirectSimpleCommand(w http.ResponseWriter, request *http.Request) {
	action := strings.ToLower(strings.TrimSpace(chi.URLParam(request, "action")))
	switch action {
	case "start", "pause", "dock", "locate":
		ws.submitDirectCommand(w, request, action)
	default:
		writeLoxoneError(w, http.StatusNotFound, fmt.Errorf("unknown command"))
	}
}

func (ws *WebServer) loxoneDirectPathCommand(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		value := strings.TrimSpace(chi.URLParam(request, "value"))
		if value == "" || len(value) > 80 {
			writeLoxoneError(w, http.StatusBadRequest, fmt.Errorf("invalid command value"))
			return
		}
		var command string
		switch kind {
		case "room", "scene":
			id, err := strconv.Atoi(value)
			if err != nil || id <= 0 {
				writeLoxoneError(w, http.StatusBadRequest, fmt.Errorf("invalid numeric ID"))
				return
			}
			command = map[string]string{"room": "clean_room_id:", "scene": "scene_id:"}[kind] + strconv.Itoa(id)
		case "fan", "mop", "water":
			command = kind + ":" + value
		default:
			writeLoxoneError(w, http.StatusNotFound, fmt.Errorf("unknown command"))
			return
		}
		ws.submitDirectCommand(w, request, command)
	}
}

func (ws *WebServer) submitDirectCommand(w http.ResponseWriter, request *http.Request, command string) {
	if request.Method == http.MethodGet && !config.Get().Loxone.Direct.AllowGETCommands {
		w.Header().Set("Allow", http.MethodPost)
		writeLoxoneError(w, http.StatusMethodNotAllowed, fmt.Errorf("GET command compatibility is disabled"))
		return
	}
	dependencies := ws.getLoxoneIntegration()
	if dependencies == nil || dependencies.SubmitCommand == nil {
		writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("command coordinator is not ready"))
		return
	}
	slug := chi.URLParam(request, "slug")
	remote, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		remote = request.RemoteAddr
	}
	key := remote + "\x00" + slug
	if !ws.directLimiter.allow(key, config.Get().Loxone.Direct.RateLimitPerMinute, time.Now()) {
		writeLoxoneError(w, http.StatusTooManyRequests, fmt.Errorf("command rate limit exceeded"))
		return
	}
	result := dependencies.SubmitCommand(slug, command)
	if result.State == "failed" {
		status := http.StatusBadRequest
		switch result.Failure {
		case "not_found":
			status = http.StatusNotFound
		case "conflict":
			status = http.StatusConflict
		}
		writeLoxoneJSON(w, status, map[string]any{"id": result.ID, "command": result.Command, "state": result.State, "error": result.Error, "timestamp": time.Now().Unix()})
		return
	}
	writeLoxoneJSON(w, http.StatusAccepted, map[string]any{"id": result.ID, "command": result.Command, "state": result.State, "timestamp": time.Now().Unix()})
}

func (ws *WebServer) loxoneDirectCommandStatus(w http.ResponseWriter, request *http.Request) {
	dependencies := ws.getLoxoneIntegration()
	if dependencies == nil || dependencies.FindCommand == nil {
		writeLoxoneError(w, http.StatusServiceUnavailable, fmt.Errorf("command diagnostics are unavailable"))
		return
	}
	id := strings.TrimSpace(chi.URLParam(request, "id"))
	slug, activity, ok := dependencies.FindCommand(id)
	if !ok {
		writeLoxoneError(w, http.StatusNotFound, fmt.Errorf("unknown command ID"))
		return
	}
	writeLoxoneJSON(w, http.StatusOK, map[string]any{"robot": slug, "activity": activity})
}
