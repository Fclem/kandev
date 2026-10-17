// Package usage subscribes to the session-prompt-usage bus subject and
// records an append-only task_usage_events ledger row per observed event
// (docs/specs/task-cost-ledger/spec.md). It must not import any
// internal/office/** package (AC-25): Office records its own, separate
// cost_events ledger off the same bus subject, and this package is a
// sibling consumer, not a dependent.
package usage

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/events/bus"
)

// usageEventPayload mirrors lifecycle.SessionPromptUsageEventPayload's JSON
// shape field-for-field (AC-25). It is declared locally rather than
// importing that type because internal/agent/runtime/lifecycle imports
// internal/task/models, so importing it here would point a dependency edge
// back up the stack the wrong way; the wire contract is the JSON, not a
// shared Go type.
type usageEventPayload struct {
	TaskID           string                         `json:"task_id"`
	SessionID        string                         `json:"session_id"`
	AgentID          string                         `json:"agent_id"`
	AgentProfileID   string                         `json:"agent_profile_id,omitempty"`
	AgentType        string                         `json:"agent_type,omitempty"`
	Model            string                         `json:"model,omitempty"`
	Usage            *promptUsagePayload            `json:"usage"`
	Timestamp        string                         `json:"timestamp"`
	TurnID           string                         `json:"turn_id,omitempty"`
	UsageEventID     string                         `json:"usage_event_id,omitempty"`
	UsageObservation *nativeUsageObservationPayload `json:"usage_observation,omitempty"`
}

type nativeUsageObservationPayload struct {
	SchemaVersion            int    `json:"schema_version"`
	Source                   string `json:"source"`
	ProviderThreadID         string `json:"provider_thread_id"`
	ProviderTurnID           string `json:"provider_turn_id"`
	ProviderResponseID       string `json:"provider_response_id,omitempty"`
	Scope                    string `json:"scope"`
	Completeness             string `json:"completeness"`
	Model                    string `json:"model,omitempty"`
	ReasoningOutputTokens    *int64 `json:"reasoning_output_tokens,omitempty"`
	ReportedCacheWriteTokens *int64 `json:"reported_cache_write_tokens,omitempty"`
	ReportedTotalTokens      *int64 `json:"reported_total_tokens,omitempty"`
	PriceSuppressed          bool   `json:"price_suppressed,omitempty"`
}

// Optional numeric fields retain their wire presence; producer presence bits
// also preserve measured zero when the numeric field is omitted.
type promptUsagePayload struct {
	InputTokens                  int64 `json:"input_tokens,omitempty"`
	InputTokensPresent           bool  `json:"input_tokens_present"`
	OutputTokens                 int64 `json:"output_tokens"`
	OutputTokensPresent          bool  `json:"output_tokens_present"`
	CachedReadTokens             int64 `json:"cached_read_tokens,omitempty"`
	CachedWriteTokens            int64 `json:"cached_write_tokens,omitempty"`
	ThoughtTokens                int64 `json:"thought_tokens,omitempty"`
	CachedReadTokensPresent      bool  `json:"cached_read_tokens_present,omitempty"`
	CachedWriteTokensPresent     bool  `json:"cached_write_tokens_present,omitempty"`
	ThoughtTokensPresent         bool  `json:"thought_tokens_present,omitempty"`
	TotalTokens                  int64 `json:"total_tokens,omitempty"`
	ProviderReportedCostSubcents int64 `json:"provider_reported_cost_subcents,omitempty"`
	ProviderReportedCostPresent  bool  `json:"provider_reported_cost_present,omitempty"`
	Estimated                    bool  `json:"estimated,omitempty"`
	PriceSuppressed              bool  `json:"price_suppressed,omitempty"`
}

func (u *promptUsagePayload) UnmarshalJSON(data []byte) error {
	type payload promptUsagePayload
	decoded := struct {
		*payload
		InputTokens       *int64 `json:"input_tokens"`
		CachedReadTokens  *int64 `json:"cached_read_tokens"`
		CachedWriteTokens *int64 `json:"cached_write_tokens"`
		ThoughtTokens     *int64 `json:"thought_tokens"`
	}{payload: (*payload)(u)}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.InputTokens != nil {
		u.InputTokens = *decoded.InputTokens
		u.InputTokensPresent = true
	}
	if decoded.CachedReadTokens != nil {
		u.CachedReadTokens = *decoded.CachedReadTokens
		u.CachedReadTokensPresent = true
	}
	if decoded.CachedWriteTokens != nil {
		u.CachedWriteTokens = *decoded.CachedWriteTokens
		u.CachedWriteTokensPresent = true
	}
	if decoded.ThoughtTokens != nil {
		u.ThoughtTokens = *decoded.ThoughtTokens
		u.ThoughtTokensPresent = true
	}
	return nil
}

// decodePayload extracts a typed usageEventPayload from a bus event via a
// JSON marshal/unmarshal round trip (mirrors office/service.decodeEventData
// for the same wire event, without importing that unexported helper).
func decodePayload(event *bus.Event) (*usageEventPayload, error) {
	raw, err := json.Marshal(event.Data)
	if err != nil {
		return nil, err
	}
	var payload usageEventPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}
