package acp

import (
	"encoding/json"
	"testing"

	sdk "github.com/coder/acp-go-sdk"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.7
func TestProviderErrorFromErrorRetryAfterMs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    any
		message string
		want    int64
	}{
		{"exact milliseconds", map[string]any{"retry_after_ms": float64(274579000)}, "rate_limit_error", 274579000},
		{"camel case", map[string]any{"retryAfterMs": float64(41501)}, "rate_limit_error", 41501},
		{"header milliseconds precede seconds", map[string]any{"retry-after": "1", "headers": map[string]any{"retry-after-ms": "274579000"}}, "rate_limit_error", 274579000},
		{"fractional seconds", map[string]any{"retry-after": "41.5"}, "rate_limit_error", 41500},
		{"Gemini duration", map[string]any{"retryDelay": "34s"}, "RESOURCE_EXHAUSTED", 34000},
		{"text milliseconds", nil, "rate_limit_error retry-after-ms=274579000", 274579000},
		{"text seconds", nil, "Please retry in 41.5s", 41500},
		{"eight days", map[string]any{"retry_after_ms": float64(691200000)}, "rate_limit_error", 691200000},
		{"zero", map[string]any{"retry_after_ms": float64(0)}, "rate_limit_error", 0},
		{"negative", map[string]any{"retry_after_ms": float64(-1)}, "rate_limit_error", 0},
		{"overflow", map[string]any{"retry_after_ms": "9223372036854775807"}, "rate_limit_error", 0},
		{"fractional millisecond", map[string]any{"retry_after_ms": 1.5}, "rate_limit_error", 0},
		{"malformed", map[string]any{"retry-after": "NaN"}, "rate_limit_error", 0},
		{"missing duration unit", map[string]any{"retryDelay": "34"}, "RESOURCE_EXHAUSTED", 0},
		{"submillisecond delay", map[string]any{"retry-after": "0.0001"}, "rate_limit_error", 0},
		{"unallowlisted field", map[string]any{"delay": "34s"}, "rate_limit_error", 0},
		{"oversized integer text", nil, "rate_limit_error retry-after-ms=9999999999999", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ProviderErrorFromError(&sdk.RequestError{Code: -32603, Message: tc.message, Data: tc.data}, "claude-acp", "opus")
			if got == nil {
				t.Fatal("missing provider diagnostic")
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			number, _ := fields["retry_after_ms"].(float64)
			if int64(number) != tc.want {
				t.Fatalf("retry_after_ms = %v, want %d", fields["retry_after_ms"], tc.want)
			}
		})
	}
}
