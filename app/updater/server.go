package updater

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Server struct {
	service  *Service
	token    string
	mu       sync.Mutex
	requests []time.Time
	maximum  int
	window   time.Duration
}

func NewServer(service *Service, token string, maximum int, window time.Duration) (*Server, error) {
	token = strings.TrimSpace(token)
	if len(token) < 32 {
		return nil, fmt.Errorf("UPDATER_TOKEN must contain at least 32 characters")
	}
	if maximum <= 0 {
		maximum = 3
	}
	if window <= 0 {
		window = time.Hour
	}
	return &Server{service: service, token: token, maximum: maximum, window: window}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.health)
	mux.HandleFunc("GET /v1/operations/current", s.auth(s.current))
	mux.HandleFunc("POST /v1/updates", s.auth(s.rateLimit(s.start)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) current(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.service.Status())
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request Request
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	operation, err := s.service.Start(request)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "already") || strings.Contains(err.Error(), "running") {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]any{"error": err.Error(), "operation": operation})
		return
	}
	slog.Info("update accepted", "operation", operation.ID, "tag", operation.Tag)
	writeJSON(w, http.StatusAccepted, operation)
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(provided) != len(s.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) rateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		cutoff := now.Add(-s.window)
		s.mu.Lock()
		kept := s.requests[:0]
		for _, request := range s.requests {
			if request.After(cutoff) {
				kept = append(kept, request)
			}
		}
		s.requests = kept
		if len(s.requests) >= s.maximum {
			s.mu.Unlock()
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "update rate limit exceeded"})
			return
		}
		s.requests = append(s.requests, now)
		s.mu.Unlock()
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
