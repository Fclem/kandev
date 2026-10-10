package acp

import (
	"encoding/json"
	"strings"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/providerretry"
)

const (
	ompAgentID = "omp-acp"
	// anthropicErrorEnvelopeType is the top-level type of an Anthropic API error body.
	anthropicErrorEnvelopeType  = "error"
	anthropicRateLimitErrorType = "rate_limit_error"
)

func (a *Adapter) observeOMPLimitEvidence(n sdk.SessionNotification, generation uint64) bool {
	if a.agentID != ompAgentID || n.Update.AgentMessageChunk == nil || generation == 0 {
		return false
	}
	turn := a.currentPromptTurn()
	if turn == nil || turn.promptGeneration != generation {
		return false
	}
	a.mu.RLock()
	active := a.sessionID == string(n.SessionId) && !a.closed
	modelID := currentModelFromConfig(a.availableConfigOptions)
	a.mu.RUnlock()
	if !active {
		return false
	}
	var candidate *streams.ProviderError
	if content := n.Update.AgentMessageChunk.Content.Text; content != nil {
		candidate = ompAnthropicLimitEnvelope(content.Text)
		if candidate != nil {
			candidate.ModelID = modelID
			candidate.OccurredAt = time.Now().UTC()
		}
	}
	// Every subsequent assistant chunk replaces this turn's final-chunk evidence.
	turn.evidenceMu.Lock()
	turn.ompLimit = candidate
	turn.evidenceMu.Unlock()
	return candidate != nil
}

func ompAnthropicLimitEnvelope(text string) *streams.ProviderError {
	body, ok := strings.CutPrefix(strings.TrimSpace(text), "429 ")
	if !ok {
		return nil
	}
	var envelope struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decoder := json.NewDecoder(strings.NewReader(body))
	if err := decoder.Decode(&envelope); err != nil || envelope.Type != anthropicErrorEnvelopeType || envelope.Error.Type != anthropicRateLimitErrorType {
		return nil
	}
	message := streams.SanitizeProviderMessage(envelope.Error.Message)
	if message == "" {
		return nil
	}
	var delay *int64
	if suffix := strings.TrimSpace(body[decoder.InputOffset():]); suffix != "" {
		digits, valid := strings.CutPrefix(suffix, "retry-after-ms=")
		if !valid || digits == "" {
			return nil
		}
		for _, ch := range digits {
			if ch < '0' || ch > '9' {
				return nil
			}
		}
		delay = providerretry.Milliseconds(digits)
	}
	return &streams.ProviderError{
		Source:       streams.ProviderErrorSourceOMPACP,
		ProviderID:   ompAgentID,
		Message:      message,
		RetryAfterMs: delay,
	}
}

func (t *promptTurnState) ompLimitFailure() *streams.ProviderError {
	if t == nil {
		return nil
	}
	t.evidenceMu.Lock()
	defer t.evidenceMu.Unlock()
	if t.ompLimit == nil {
		return nil
	}
	copy := *t.ompLimit
	return &copy
}
