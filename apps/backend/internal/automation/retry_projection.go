package automation

import (
	"context"
	"encoding/json"
	"strings"
)

// SafeRetryTriggerProjection retains only fields that are stable trigger
// provenance. Provider payloads, headers, credentials, and signatures never
// enter retry snapshots.

const (
	retryTriggerTypeKey = "trigger_type"
	retryTriggerIDKey   = "trigger_id"
)

func SafeRetryTriggerProjection(triggerType TriggerType, triggerID string, raw json.RawMessage, dedupKey string) json.RawMessage {
	projection := map[string]any{
		"projection_version": int64(1),
		retryTriggerTypeKey:  triggerType,
		retryTriggerIDKey:    triggerID,
	}
	if dedupKey != "" {
		projection["dedup_key"] = dedupKey
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) == nil {
		allowed := map[string]struct{}{
			"observation_id": {}, "observed_at": {}, "provider": {}, "event_type": {},
			"delivery_id": {}, "item_id": {}, "repository_id": {}, "action": {},
			"schedule_id": {}, "occurrence_timestamp": {}, "server_request_id": {},
			"payload": {}, automationRepoKey: {}, automationHeadBranchKey: {},
			automationBaseBranchKey: {}, automationTaskIDKey: {}, automationPRNumberKey: {},
			automationMergedPRNumberKey: {}, automationTitleKey: {}, automationHTMLURLKey: {},
			automationAuthorLoginKey: {}, automationBodyKey: {}, automationBranchKey: {},
			automationSHAKey: {}, automationMessageKey: {}, automationCheckNameKey: {},
			automationConclusionKey: {},
		}
		for key, value := range fields {
			if _, ok := allowed[strings.ToLower(key)]; ok {
				projection[key] = value
			}
		}
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return json.RawMessage(`{"projection_version":1}`)
	}
	return encoded
}

func (s *Store) ensureRetryIndexes() error {
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_automation_runs_retry_due ON automation_runs(retry_state, retry_scheduled_at, retry_group_generation)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_automation_runs_retry_group_attempt ON automation_runs(retry_group_id, attempt_number) WHERE retry_group_id <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_automation_retry_outbox_pending ON automation_retry_outbox(state, enqueued_at)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_retry_operations_state ON automation_run_operations(state, lease_expires_at)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(s.db.Rebind(statement)); err != nil {
			return err
		}
	}
	return nil
}

// PendingRetrySummary is bounded so a busy automation cannot make summary
// reads unbounded or expose private claim and operation tokens.
func (s *Store) PendingRetrySummary(ctx context.Context, automationID string) (PendingRetrySummary, error) {
	const limit = 32
	var count int
	if err := s.ro.GetContext(ctx, &count, s.ro.Rebind(`
		SELECT COUNT(*) FROM automation_runs ar
		JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id
		WHERE ar.automation_id = ? AND rg.state = ? AND ar.retry_state IN (?, ?)`),
		automationID, RetryGroupLive, RetryStateScheduled, RetryStateClaimed); err != nil {
		return PendingRetrySummary{}, err
	}
	var items []*AutomationRun
	if err := s.ro.SelectContext(ctx, &items, s.ro.Rebind(`
		SELECT ar.* FROM automation_runs ar
		JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id
		WHERE ar.automation_id = ? AND rg.state = ? AND ar.retry_state IN (?, ?)
		ORDER BY COALESCE(ar.retry_scheduled_at, ar.created_at) ASC, ar.id ASC LIMIT ?`),
		automationID, RetryGroupLive, RetryStateScheduled, RetryStateClaimed, limit); err != nil {
		return PendingRetrySummary{}, err
	}
	for _, item := range items {
		item.TriggerData = json.RawMessage(item.TriggerDataJSON)
	}
	return PendingRetrySummary{Count: count, Items: items, Limit: limit}, nil
}
