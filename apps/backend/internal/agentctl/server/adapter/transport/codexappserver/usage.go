package codexappserver

import (
	"encoding/json"
	"fmt"
	"math"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

const (
	nativeUsageSchemaVersion   = 1
	nativeRawOutputKey         = "outputTokens"
	nativeRawCacheWriteKey     = "cacheWriteInputTokens"
	nativeRawInputKey          = "inputTokens"
	nativeRawCacheReadKey      = "cachedInputTokens"
	nativeRawReasoningKey      = "reasoningOutputTokens"
	nativeRawTotalKey          = "totalTokens"
	nativeTurnAttributionLimit = 128
)

type nativeTurnAttribution struct {
	model      string
	generation uint64
}

func (a *Adapter) resetNativeAttributionLocked() {
	clear(a.turnAttributions)
	clear(a.turnAttributionKeys[:])
	a.turnAttributionNext = 0
}

func (a *Adapter) beginProviderTurn(threadID, turnID string) {
	if threadID == "" || turnID == "" {
		return
	}
	key := nativeTurnKey(threadID, turnID)
	a.mu.Lock()
	if _, exists := a.turnAttributions[key]; exists {
		a.mu.Unlock()
		return
	}
	model := a.modelID
	generation := a.activeGeneration
	if child, ok := a.children[threadID]; ok {
		model = child.model
		generation = child.generation
	}
	if expired := a.turnAttributionKeys[a.turnAttributionNext]; expired != "" {
		delete(a.turnAttributions, expired)
	}
	a.turnAttributions[key] = nativeTurnAttribution{model: model, generation: generation}
	a.turnAttributionKeys[a.turnAttributionNext] = key
	a.turnAttributionNext = (a.turnAttributionNext + 1) % nativeTurnAttributionLimit
	a.mu.Unlock()
}

func (a *Adapter) handleRawResponseCompleted(params map[string]any) {
	notification, ok := decodeRawResponse(params)
	if !ok {
		return
	}
	rootThreadID, parentToolCallID, isChild := a.eventScope(notification.ThreadID)
	if notification.ThreadID != rootThreadID && !isChild {
		// A provider thread without a known parent must not be attributed to the
		// root conversation by guessing from its arrival time.
		return
	}
	key := nativeTurnKey(notification.ThreadID, notification.TurnID)
	a.mu.RLock()
	attribution, retained := a.turnAttributions[key]
	a.mu.RUnlock()
	if !retained {
		if a.log != nil {
			a.log.Warn("ignoring native response without retained turn attribution", zap.String("thread_id", notification.ThreadID), zap.String("turn_id", notification.TurnID))
		}
		return
	}

	usage, observation := responseUsageOrGap(notification, isChild, attribution.model)
	a.emit(streams.AgentEvent{
		Type:             streams.EventTypeUsageObservation,
		SessionID:        rootThreadID,
		OperationID:      notification.TurnID,
		PromptGeneration: attribution.generation,
		ParentToolCallID: parentToolCallID,
		Usage:            usage,
		UsageObservation: observation,
	})
}

func decodeRawResponse(params map[string]any) (protocol.RawResponseCompleted, bool) {
	encoded, err := json.Marshal(params)
	if err != nil {
		return protocol.RawResponseCompleted{}, false
	}
	var notification protocol.RawResponseCompleted
	if json.Unmarshal(encoded, &notification) != nil {
		var identity struct {
			ThreadID   string `json:"threadId"`
			TurnID     string `json:"turnId"`
			ResponseID string `json:"responseId"`
		}
		if json.Unmarshal(encoded, &identity) != nil {
			return protocol.RawResponseCompleted{}, false
		}
		notification = protocol.RawResponseCompleted{
			ThreadID: identity.ThreadID, TurnID: identity.TurnID, ResponseID: identity.ResponseID,
		}
	}
	if notification.Usage != nil && !completeRawUsageFields(params["usage"]) {
		notification.Usage = nil
	}
	return notification, notification.ThreadID != "" && notification.TurnID != "" && notification.ResponseID != ""
}

func completeRawUsageFields(raw any) bool {
	fields, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range [...]string{
		nativeRawInputKey, nativeRawCacheReadKey,
		nativeRawOutputKey, nativeRawReasoningKey, nativeRawTotalKey,
	} {
		if fields[key] == nil {
			return false
		}
	}
	// Cache-write is optional with a protocol default of zero, but null is invalid.
	if value, present := fields[nativeRawCacheWriteKey]; present && value == nil {
		return false
	}
	return true
}

func responseUsageOrGap(notification protocol.RawResponseCompleted, isChild bool, model string) (*streams.PromptUsage, *streams.NativeUsageObservation) {
	if notification.Usage != nil {
		usage, observation, ok := normalizeRawResponseUsage(*notification.Usage, notification.ThreadID, notification.TurnID, notification.ResponseID, isChild, model)
		if ok {
			return usage, observation
		}
	}
	scope := streams.NativeUsageScopeDirect
	if isChild {
		scope = streams.NativeUsageScopeChild
	}
	return nil, &streams.NativeUsageObservation{
		SchemaVersion: nativeUsageSchemaVersion, Source: streams.NativeUsageSourceResponse,
		ProviderThreadID: notification.ThreadID, ProviderTurnID: notification.TurnID,
		ProviderResponseID: notification.ResponseID, Scope: scope, Completeness: streams.NativeUsageCompletenessPartial, Model: model,
	}
}

func normalizeRawResponseUsage(raw protocol.TokenUsageBreakdown, threadID, turnID, responseID string, child bool, model string) (*streams.PromptUsage, *streams.NativeUsageObservation, bool) {
	if !validNativeTokenCounts(&raw) {
		return nil, nil, false
	}
	input := raw.InputTokens - raw.CachedInputTokens
	total, ok := sumNativeTokens(input, raw.CachedInputTokens, raw.OutputTokens)
	if !ok {
		return nil, nil, false
	}
	reasoning := raw.ReasoningOutputTokens
	cacheWrite := raw.CacheWriteInputTokens
	reportedTotal := raw.TotalTokens
	scope := "direct"
	if child {
		scope = "child"
	}
	usage := &streams.PromptUsage{
		InputTokens:              input,
		InputTokensPresent:       true,
		CachedReadTokens:         raw.CachedInputTokens,
		OutputTokens:             raw.OutputTokens,
		OutputTokensPresent:      true,
		CachedReadTokensPresent:  true,
		CachedWriteTokensPresent: raw.CacheWriteInputTokens == 0,
		TotalTokens:              total,
		PriceSuppressed:          raw.CacheWriteInputTokens > 0,
	}
	observation := &streams.NativeUsageObservation{
		SchemaVersion:            nativeUsageSchemaVersion,
		Source:                   "response",
		ProviderThreadID:         threadID,
		ProviderTurnID:           turnID,
		ProviderResponseID:       responseID,
		Scope:                    scope,
		Completeness:             "exact",
		Model:                    model,
		ReasoningOutputTokens:    &reasoning,
		ReportedCacheWriteTokens: &cacheWrite,
		ReportedTotalTokens:      &reportedTotal,
		PriceSuppressed:          cacheWrite > 0,
	}
	return usage, observation, true
}

func validNativeTokenCounts(raw *protocol.TokenUsageBreakdown) bool {
	return raw.InputTokens >= 0 && raw.CachedInputTokens >= 0 && raw.CacheWriteInputTokens >= 0 &&
		raw.OutputTokens >= 0 && raw.ReasoningOutputTokens >= 0 && raw.TotalTokens >= 0 &&
		raw.CachedInputTokens <= raw.InputTokens && raw.ReasoningOutputTokens <= raw.OutputTokens
}

func sumNativeTokens(values ...int64) (int64, bool) {
	var total int64
	for _, value := range values {
		if value < 0 || total > math.MaxInt64-value {
			return 0, false
		}
		total += value
	}
	return total, true
}

func nativeTurnKey(threadID, turnID string) string {
	return fmt.Sprintf("%s\x00%s", threadID, turnID)
}
