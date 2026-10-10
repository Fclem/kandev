package routingerr

import (
	"strings"
	"testing"
	"time"
)

func limitTestInput(provider, source, text string, delay time.Duration) Input {
	return Input{ProviderID: provider, DiagnosticSource: source, RetryAfter: delay,
		Phase: PhaseStreaming, Stderr: text,
		OccurredAt: time.Date(2026, 10, 3, 12, 0, 0, 123000000, time.UTC)}
}

func assertLimitScope(t *testing.T, got *Error, want string) {
	t.Helper()
	if got.LimitScope != want {
		t.Fatalf("scope = %s, want %s", got.LimitScope, want)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.2
func TestClassifyAnthropicSpendLimit(t *testing.T) {
	for _, text := range []string{
		`429 {"type":"error","error":{"type":"rate_limit_error","message":"This request would exceed your account's monthly spend limit."},"request_id":"req_011CfL9vJs9DYV45eqL6jbh9"} retry-after-ms=274579000`,
		`rate_limit_error: enforced_spend_limit_reached`,
	} {
		got := Classify(limitTestInput("claude-acp", "", text, 0))
		if got.Code != CodeQuotaLimited {
			t.Errorf("spend limit: got %s, want quota_limited", got.Code)
		}
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.3
func TestClassifyLimitScope(t *testing.T) {
	for _, tc := range []struct{ provider, text, scope string }{
		{"claude-acp", "You've hit your session limit", "account"},
		{"claude-acp", "insufficient credits", "account"},
		{"codex-acp", "usageLimitExceeded", "account"},
		{"opencode-acp", "5-hour usage limit reached", "account"},
		{"opencode-acp", "out of credits", "account"},
		{"opencode-acp", "Error: quota exceeded", "model"},
		{"copilot-acp", "Rate limit exceeded", "model"},
	} {
		t.Run(tc.provider+tc.text, func(t *testing.T) {
			assertLimitScope(t, Classify(limitTestInput(tc.provider, "", tc.text, 0)), tc.scope)
		})
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.7
func TestClassifyRetryAfterPrecedence(t *testing.T) {
	for _, delay := range []time.Duration{274579000 * time.Millisecond, 8 * 24 * time.Hour, 41500 * time.Millisecond} {
		in := limitTestInput("claude-acp", "", "rate_limit_error Please retry in 2s", delay)
		want := in.OccurredAt.Add(delay)
		got := Classify(in)
		if got.ResetHint == nil || !got.ResetHint.Equal(want) {
			t.Errorf("delay %v: reset %v, want %v", delay, got.ResetHint, want)
		}
		absolute := in.OccurredAt.Add(time.Hour)
		in.ResetHint = &absolute
		got = Classify(in)
		if got.ResetHint == nil || !got.ResetHint.Equal(absolute) {
			t.Errorf("absolute precedence: %v", got.ResetHint)
		}
	}
	for _, text := range []string{"rate_limit_error retry-after-ms=0", "rate_limit_error retry-after-ms=-2", "rate_limit_error retry-after-ms=9223372036854775807", "rate_limit_error Please retry in nonsense"} {
		if got := Classify(limitTestInput("claude-acp", "", text, 0)); got.ResetHint != nil {
			t.Errorf("invalid timing %q: %v", text, got.ResetHint)
		}
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.1
func TestClassifyGeminiResourceExhausted(t *testing.T) {
	in := limitTestInput("gemini", "", `429 RESOURCE_EXHAUSTED: exceeded your current quota. retryDelay "34s"`, 0)
	got := Classify(in)
	if got.Code != CodeQuotaLimited {
		t.Fatalf("got %s, want quota_limited", got.Code)
	}
	want := in.OccurredAt.Add(34 * time.Second)
	if got.ResetHint == nil || !got.ResetHint.Equal(want) {
		t.Fatalf("reset = %v, want %v", got.ResetHint, want)
	}
	assertLimitScope(t, got, "model")
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.2
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.4
func TestClassifyOMPAnthropicEnvelope(t *testing.T) {
	for _, tc := range []struct {
		message string
		code    Code
		scope   string
	}{
		{"This request would exceed your account's monthly spend limit", CodeQuotaLimited, "account"},
		{"Too many requests", CodeRateLimited, "model"},
	} {
		in := limitTestInput("omp-acp", "omp_acp", tc.message, 274579000*time.Millisecond)
		got := Classify(in)
		if got.Code != tc.code {
			t.Errorf("converted OMP: got %s, want %s", got.Code, tc.code)
		} else {
			assertLimitScope(t, got, tc.scope)
		}
		prose := Classify(limitTestInput("omp-acp", "", tc.message, 0))
		if prose.Code != CodeAgentRuntime {
			t.Errorf("ordinary prose classified as %s", prose.Code)
		}
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.6
func TestSanitizeRedactsRequestID(t *testing.T) {
	got := Sanitize(`monthly spend limit request_id=req_short retry-after-ms=274579000`)
	if strings.Contains(got, "req_short") {
		t.Fatalf("request identifier leaked: %s", got)
	}
	if !strings.Contains(got, "274579000") {
		t.Fatalf("retry delay lost: %s", got)
	}
}
