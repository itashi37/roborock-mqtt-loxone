package roborock

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type AdvancedDiagnostics struct {
	Method      string         `json:"method"`
	CollectedAt time.Time      `json:"collected_at"`
	Fields      map[string]any `json:"fields"`
}

var diagnosticExactFields = map[string]bool{
	"dock_type": true, "charge_status": true, "dock_error_status": true,
	"dust_collection_status": true, "wash_status": true, "dry_status": true,
	"locate": true, "find_me": true, "stop": true, "app_stop": true,
	"dock_empty": true, "dust_collection": true, "mop_wash": true,
	"wash_mop": true, "mop_dry": true, "dry_mop": true,
}

func ParseAdvancedDiagnostics(data []byte, now time.Time) (AdvancedDiagnostics, error) {
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return AdvancedDiagnostics{}, fmt.Errorf("parse app_get_init_status: %w", err)
	}
	fields := make(map[string]any)
	collectDiagnosticFields(decoded, fields, 0)
	return AdvancedDiagnostics{Method: "app_get_init_status", CollectedAt: now, Fields: fields}, nil
}

func collectDiagnosticFields(value any, result map[string]any, depth int) {
	if depth > 8 {
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lower := strings.ToLower(key)
			if safeDiagnosticKey(lower) {
				if scrubbed, ok := scrubDiagnosticValue(child, depth+1); ok {
					result[lower] = scrubbed
				}
			}
			if !sensitiveDiagnosticKey(lower) {
				collectDiagnosticFields(child, result, depth+1)
			}
		}
	case []any:
		for _, child := range typed {
			collectDiagnosticFields(child, result, depth+1)
		}
	}
}

func safeDiagnosticKey(key string) bool {
	if sensitiveDiagnosticKey(key) {
		return false
	}
	return diagnosticExactFields[key] || strings.Contains(key, "feature") || strings.HasPrefix(key, "support_")
}

func sensitiveDiagnosticKey(key string) bool {
	for _, fragment := range []string{"token", "secret", "password", "credential", "localkey", "local_key", "mqtt", "nonce", "security", "endpoint", "userid", "user_id"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func scrubDiagnosticValue(value any, depth int) (any, bool) {
	if depth > 8 {
		return nil, false
	}
	switch typed := value.(type) {
	case nil, bool, float64, string:
		return typed, true
	case []any:
		result := make([]any, 0, len(typed))
		for _, child := range typed {
			if item, ok := scrubDiagnosticValue(child, depth+1); ok {
				result = append(result, item)
			}
		}
		return result, true
	case map[string]any:
		result := make(map[string]any)
		for key, child := range typed {
			if sensitiveDiagnosticKey(strings.ToLower(key)) {
				continue
			}
			if item, ok := scrubDiagnosticValue(child, depth+1); ok {
				result[key] = item
			}
		}
		return result, true
	default:
		return nil, false
	}
}
