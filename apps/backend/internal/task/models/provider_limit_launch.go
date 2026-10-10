package models

import (
	"fmt"
	"maps"
	"time"
)

const (
	ProviderLimitLaunchKey   = "provider_limit_launch"
	ProviderLimitDeferredKey = "provider_limit_deferred"
	ProviderLimitModelKey    = "provider_limit_model"
	ProviderLimitRetryAtKey  = "provider_limit_retry_at"
)

// Field names shared by the durable provider-limit launch, wait, and probe-owner records.
const (
	recordFieldModel          = "model"
	recordFieldSessionID      = "session_id"
	recordFieldKey            = "key"
	recordFieldUntil          = "until"
	recordFieldExecutionID    = "execution_id"
	recordFieldWorkflowStepID = "workflow_step_id"
)

// ClearProviderLimitLaunch removes the deferred provider-limit launch from
// record, keeping the shared ceiling launch fields only when a ceiling
// deferral still owns them.
func ClearProviderLimitLaunch(record map[string]interface{}) {
	delete(record, ProviderLimitLaunchKey)
	delete(record, ProviderLimitDeferredKey)
	delete(record, ProviderLimitModelKey)
	delete(record, ProviderLimitRetryAtKey)
	if _, err := ReadCeilingDeferral(record); err != nil {
		delete(record, CeilingLaunchKindKey)
		delete(record, CeilingLaunchPayloadKey)
		delete(record, CeilingLaunchOriginKey)
	}
}

// IsProviderLimitRecordKey identifies recovery state that dependency claims cannot consume.
func IsProviderLimitRecordKey(key string) bool {
	switch key {
	case ProviderLimitWaitsKey, ProviderLimitLaunchKey, ProviderLimitProbeOwnersKey, ProviderLimitDeferredKey, ProviderLimitModelKey, ProviderLimitRetryAtKey:
		return true
	default:
		return false
	}
}

// ProviderLimitLaunch owns the shared automatic launch slot, independently of turn waits.
type ProviderLimitLaunch struct {
	ID             string
	Kind           CeilingLaunchKind
	Payload        map[string]interface{}
	Origin         string
	WorkflowStepID string
	MarkKey        string
	Model          string
	NotBefore      time.Time
	QueuedAt       time.Time
	ProbeLease     *ProviderLimitProbeLease
	SessionID      string
}

func ReadProviderLimitLaunch(record map[string]interface{}) (*ProviderLimitLaunch, error) {
	raw, exists := record[ProviderLimitLaunchKey]
	if !exists {
		return nil, nil
	}
	object, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("provider limit launch is not an object")
	}
	launch := &ProviderLimitLaunch{}
	launch.ID, _ = object["id"].(string)
	launch.MarkKey, _ = object["mark_key"].(string)
	launch.Model, _ = object[recordFieldModel].(string)
	launch.WorkflowStepID, _ = object[recordFieldWorkflowStepID].(string)
	launch.SessionID, _ = object[recordFieldSessionID].(string)
	kind, _ := record[CeilingLaunchKindKey].(string)
	launch.Kind = CeilingLaunchKind(kind)
	launch.Payload, _ = record[CeilingLaunchPayloadKey].(map[string]interface{})
	launch.Origin, _ = record[CeilingLaunchOriginKey].(string)
	var err error
	launch.NotBefore, err = providerLimitRecordTime(object["not_before"])
	if err != nil {
		return nil, err
	}
	launch.QueuedAt, err = providerLimitRecordTime(object["queued_at"])
	if err != nil {
		return nil, err
	}
	if launch.ID == "" || launch.MarkKey == "" || launch.Model == "" || launch.WorkflowStepID == "" || !IsCeilingLaunchKind(launch.Kind) || launch.Payload == nil {
		return nil, fmt.Errorf("provider limit launch lacks its identity or replay payload")
	}
	if raw, exists := object["probe_lease"]; exists {
		lease, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("provider limit launch probe is not an object")
		}
		key, _ := lease[recordFieldKey].(string)
		until, err := providerLimitRecordTime(lease[recordFieldUntil])
		if err != nil || key != launch.MarkKey {
			return nil, fmt.Errorf("provider limit launch probe does not match its mark")
		}
		launch.ProbeLease = &ProviderLimitProbeLease{Key: key, Until: until}
	}
	return launch, nil
}

func PutProviderLimitLaunch(record map[string]interface{}, launch ProviderLimitLaunch) (map[string]interface{}, error) {
	if launch.ID == "" || launch.MarkKey == "" || launch.Model == "" || launch.WorkflowStepID == "" || launch.NotBefore.IsZero() || launch.QueuedAt.IsZero() || !IsCeilingLaunchKind(launch.Kind) || launch.Payload == nil {
		return nil, fmt.Errorf("provider limit launch lacks its durable identity or timing")
	}
	updated := maps.Clone(record)
	if updated == nil {
		updated = make(map[string]interface{})
	}
	object := map[string]interface{}{
		"id": launch.ID, "mark_key": launch.MarkKey, recordFieldModel: launch.Model,
		recordFieldWorkflowStepID: launch.WorkflowStepID, recordFieldSessionID: launch.SessionID,
		"not_before": launch.NotBefore.UTC().Format(time.RFC3339Nano),
		"queued_at":  launch.QueuedAt.UTC().Format(time.RFC3339Nano),
	}
	if launch.ProbeLease != nil {
		if launch.ProbeLease.Key != launch.MarkKey || launch.ProbeLease.Until.IsZero() {
			return nil, fmt.Errorf("provider limit launch probe does not match its mark")
		}
		object["probe_lease"] = map[string]interface{}{recordFieldKey: launch.ProbeLease.Key, recordFieldUntil: launch.ProbeLease.Until.UTC().Format(time.RFC3339Nano)}
	}
	updated[ProviderLimitLaunchKey] = object
	updated[ProviderLimitDeferredKey] = true
	updated[ProviderLimitModelKey] = launch.Model
	updated[ProviderLimitRetryAtKey] = object["not_before"]
	updated[CeilingLaunchKindKey] = string(launch.Kind)
	updated[CeilingLaunchPayloadKey] = launch.Payload
	updated[CeilingLaunchOriginKey] = launch.Origin
	delete(updated, CeilingDeferredKey)
	return updated, nil
}
