package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

const ProviderLimitWaitsKey = "provider_limit_waits"

// ProviderLimitProbeLease preserves the acquired token, not a reconstructed expiry.
type ProviderLimitProbeLease struct {
	Key   string
	Until time.Time
}

type ProviderLimitWait struct {
	SessionID          string
	TurnID             string
	MarkKey            string
	Model              string
	WorkflowStepID     string
	Payload            map[string]interface{}
	NotBefore          time.Time
	QueuedAt           time.Time
	FailureExecutionID string
	FailureGeneration  uint64
	ProbeLease         *ProviderLimitProbeLease
	ProbeTurnID        string
	ProbeExecutionID   string
	ProbeGeneration    uint64
	ProbeSucceeded     bool
}

func ProviderLimitWaitIdentity(sessionID, turnID string) string {
	return sessionID + "|" + turnID
}

func ReadProviderLimitWaits(record map[string]interface{}) (map[string]ProviderLimitWait, error) {
	entries, err := providerLimitWaitEntries(record)
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	waits := make(map[string]ProviderLimitWait, len(entries))
	for identity, entry := range entries {
		object, ok := entry.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("provider limit wait is not an object")
		}
		wait, err := readProviderLimitWait(object)
		if err != nil {
			return nil, err
		}
		if identity != ProviderLimitWaitIdentity(wait.SessionID, wait.TurnID) {
			return nil, fmt.Errorf("provider limit wait identity does not match its owner")
		}
		waits[identity] = wait
	}
	return waits, nil
}

func PutProviderLimitWait(record map[string]interface{}, wait ProviderLimitWait) (map[string]interface{}, error) {
	if wait.SessionID == "" || wait.TurnID == "" || wait.MarkKey == "" || wait.Model == "" || wait.NotBefore.IsZero() || wait.QueuedAt.IsZero() {
		return nil, fmt.Errorf("provider limit wait lacks its durable identity or timing")
	}
	if _, err := providerLimitWaitEntries(record); err != nil {
		return nil, err
	}
	updated, entries := cloneProviderLimitWaitRecord(record)
	entries[ProviderLimitWaitIdentity(wait.SessionID, wait.TurnID)] = providerLimitWaitRecord(wait)
	updated[ProviderLimitWaitsKey] = entries
	return updated, nil
}

func RemoveProviderLimitWait(record map[string]interface{}, sessionID, turnID string) (map[string]interface{}, error) {
	if _, err := providerLimitWaitEntries(record); err != nil {
		return nil, err
	}
	updated, entries := cloneProviderLimitWaitRecord(record)
	delete(entries, ProviderLimitWaitIdentity(sessionID, turnID))
	if len(entries) == 0 {
		delete(updated, ProviderLimitWaitsKey)
	} else {
		updated[ProviderLimitWaitsKey] = entries
	}
	return updated, nil
}

func providerLimitWaitEntries(record map[string]interface{}) (map[string]interface{}, error) {
	value, exists := record[ProviderLimitWaitsKey]
	if !exists {
		return nil, nil
	}
	entries, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("provider limit wait collection is not an object")
	}
	return entries, nil
}

func cloneProviderLimitWaitRecord(record map[string]interface{}) (map[string]interface{}, map[string]interface{}) {
	updated := make(map[string]interface{}, len(record)+1)
	for key, value := range record {
		updated[key] = value
	}
	current, _ := record[ProviderLimitWaitsKey].(map[string]interface{})
	entries := make(map[string]interface{}, len(current)+1)
	for key, value := range current {
		entries[key] = value
	}
	return updated, entries
}

func providerLimitWaitRecord(wait ProviderLimitWait) map[string]interface{} {
	object := map[string]interface{}{
		recordFieldSessionID: wait.SessionID, "turn_id": wait.TurnID,
		"mark_key": wait.MarkKey, recordFieldModel: wait.Model,
		recordFieldWorkflowStepID: wait.WorkflowStepID, "payload": wait.Payload,
		"not_before":           wait.NotBefore.UTC().Format(time.RFC3339Nano),
		"queued_at":            wait.QueuedAt.UTC().Format(time.RFC3339Nano),
		"failure_execution_id": wait.FailureExecutionID, "failure_generation": wait.FailureGeneration,
		"probe_turn_id": wait.ProbeTurnID, "probe_execution_id": wait.ProbeExecutionID, "probe_generation": wait.ProbeGeneration,
		"probe_succeeded": wait.ProbeSucceeded,
	}
	if wait.ProbeLease != nil {
		object["probe_lease"] = map[string]interface{}{recordFieldKey: wait.ProbeLease.Key, recordFieldUntil: wait.ProbeLease.Until.UTC().Format(time.RFC3339Nano)}
	}
	return object
}

func readProviderLimitWait(object map[string]interface{}) (ProviderLimitWait, error) {
	var wait ProviderLimitWait
	wait.SessionID, _ = object[recordFieldSessionID].(string)
	wait.TurnID, _ = object["turn_id"].(string)
	wait.MarkKey, _ = object["mark_key"].(string)
	wait.Model, _ = object[recordFieldModel].(string)
	wait.WorkflowStepID, _ = object[recordFieldWorkflowStepID].(string)
	wait.Payload, _ = object["payload"].(map[string]interface{})
	wait.FailureExecutionID, _ = object["failure_execution_id"].(string)
	wait.ProbeTurnID, _ = object["probe_turn_id"].(string)
	wait.ProbeExecutionID, _ = object["probe_execution_id"].(string)
	wait.ProbeSucceeded, _ = object["probe_succeeded"].(bool)
	var err error
	if wait.NotBefore, err = providerLimitRecordTime(object["not_before"]); err != nil {
		return wait, err
	}
	if wait.QueuedAt, err = providerLimitRecordTime(object["queued_at"]); err != nil {
		return wait, err
	}
	if wait.FailureGeneration, err = providerLimitRecordGeneration(object["failure_generation"]); err != nil {
		return wait, err
	}
	if wait.ProbeGeneration, err = providerLimitRecordGeneration(object["probe_generation"]); err != nil {
		return wait, err
	}
	if raw, exists := object["probe_lease"]; exists {
		lease, ok := raw.(map[string]interface{})
		if !ok {
			return wait, fmt.Errorf("provider limit probe token is not an object")
		}
		key, _ := lease[recordFieldKey].(string)
		until, err := providerLimitRecordTime(lease[recordFieldUntil])
		if err != nil || key == "" || key != wait.MarkKey {
			return wait, fmt.Errorf("provider limit probe token does not match its mark")
		}
		wait.ProbeLease = &ProviderLimitProbeLease{Key: key, Until: until}
	}
	if wait.SessionID == "" || wait.TurnID == "" || wait.MarkKey == "" || wait.Model == "" {
		return wait, fmt.Errorf("provider limit wait lacks its owner or mark")
	}
	return wait, nil
}

func providerLimitRecordTime(value interface{}) (time.Time, error) {
	text, ok := value.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("provider limit timestamp is absent")
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil || parsed.IsZero() {
		return time.Time{}, fmt.Errorf("provider limit timestamp is invalid")
	}
	return parsed.UTC(), nil
}

func providerLimitRecordGeneration(value interface{}) (uint64, error) {
	switch number := value.(type) {
	case nil:
		return 0, nil
	case uint64:
		return number, nil
	case json.Number:
		return strconv.ParseUint(number.String(), 10, 64)
	case float64:
		if number >= 0 && number < 1<<53 && number == float64(uint64(number)) {
			return uint64(number), nil
		}
	}
	return 0, fmt.Errorf("provider limit prompt generation is invalid")
}
