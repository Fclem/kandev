package main

import (
	"math"
	"strconv"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// providerErrorKindKey is the ACP error-data field naming the provider error kind.
const providerErrorKindKey = "errorKind"

type providerLimitScenarioKey struct {
	sessionID    acp.SessionId
	model        string
	milliseconds int64
	quota        bool
}

// Canonical mock providers preserve real provider classification; the plain
// mock-agent ID has no provider-error catalogue and does not claim one.
func (a *mockAgent) handleProviderLimit(sid acp.SessionId, prompt string) (acp.PromptResponse, error, bool) {
	model, milliseconds, quota, ok := parseProviderLimitScenario(prompt)
	if !ok || model != a.sessionModel(sid) {
		return acp.PromptResponse{}, nil, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if a.providerLimitNow != nil {
		now = a.providerLimitNow()
	}
	if a.providerLimitWindows == nil {
		a.providerLimitWindows = make(map[providerLimitScenarioKey]time.Time)
	}
	key := providerLimitScenarioKey{sid, model, milliseconds, quota}
	deadline, exists := a.providerLimitWindows[key]
	if !exists {
		deadline = now.Add(time.Duration(milliseconds) * time.Millisecond)
		a.providerLimitWindows[key] = deadline
	}
	if !now.Before(deadline) {
		return acp.PromptResponse{}, nil, false
	}
	milliseconds = int64((deadline.Sub(now) + time.Millisecond - 1) / time.Millisecond)
	message, kind := "API Error: 429 rate_limit_exceeded: too many requests", "rate_limit_error"
	if quota {
		message, kind = "API Error: 429 enforced_spend_limit_reached: monthly spend limit reached", "quota_limit_error"
	}
	return acp.PromptResponse{}, &acp.RequestError{
		Code:    -32603,
		Message: message,
		Data:    map[string]any{providerErrorKindKey: kind, "retry_after_ms": milliseconds},
	}, true
}

func parseProviderLimitScenario(prompt string) (model string, milliseconds int64, quota, ok bool) {
	parts := strings.Fields(stripKandevSystem(strings.TrimSpace(prompt)))
	if len(parts) != 3 || (parts[0] != "/provider-limit" && parts[0] != "/provider-quota") {
		return "", 0, false, false
	}
	for _, digit := range parts[2] {
		if digit < '0' || digit > '9' {
			return "", 0, false, false
		}
	}
	seconds, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || seconds <= 0 || seconds > math.MaxInt64/int64(time.Second) {
		return "", 0, false, false
	}
	return parts[1], seconds * 1000, parts[0] == "/provider-quota", true
}
