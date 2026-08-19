package updater

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUpdaterAPIAuthorizationValidationAndRateLimit(t *testing.T) {
	service := testService(t, &fakeEngine{}, 1000, nil)
	server, err := NewServer(service, strings.Repeat("a", 32), 1, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/operations/current", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized=%d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/updates", strings.NewReader(`{"request_id":"request-5678","tag":"evil/image","expected_version":"1.0.0"}`))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	invalid := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalid, request)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid=%d body=%s", invalid.Code, invalid.Body.String())
	}

	second := httptest.NewRequest(http.MethodPost, "/v1/updates", strings.NewReader(`{"request_id":"request-9876","tag":"v1.1.0","expected_version":"1.1.0"}`))
	second.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	limited := httptest.NewRecorder()
	server.Handler().ServeHTTP(limited, second)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("limited=%d", limited.Code)
	}
}
