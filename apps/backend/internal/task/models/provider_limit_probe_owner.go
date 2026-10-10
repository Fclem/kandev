package models

import (
	"fmt"
	"maps"
	"time"
)

const ProviderLimitProbeOwnersKey = "provider_limit_probe_owners"

// ProviderLimitProbeOwner outlives the automatic launch slot and fences one admitted turn.
type ProviderLimitProbeOwner struct {
	LaunchID       string
	SessionID      string
	TurnID         string
	WorkflowStepID string
	Model          string
	ExecutionID    string
	Generation     uint64
	Lease          ProviderLimitProbeLease
	Succeeded      bool
}

func ReadProviderLimitProbeOwners(record map[string]interface{}) (map[string]ProviderLimitProbeOwner, error) {
	raw, exists := record[ProviderLimitProbeOwnersKey]
	if !exists {
		return nil, nil
	}
	entries, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("provider limit probe owners is not an object")
	}
	owners := make(map[string]ProviderLimitProbeOwner, len(entries))
	for identity, raw := range entries {
		object, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("provider limit probe owner is not an object")
		}
		var owner ProviderLimitProbeOwner
		owner.LaunchID, _ = object["launch_id"].(string)
		owner.SessionID, _ = object[recordFieldSessionID].(string)
		owner.TurnID, _ = object["turn_id"].(string)
		owner.WorkflowStepID, _ = object[recordFieldWorkflowStepID].(string)
		owner.Model, _ = object[recordFieldModel].(string)
		owner.ExecutionID, _ = object[recordFieldExecutionID].(string)
		owner.Succeeded, _ = object["succeeded"].(bool)
		var err error
		owner.Generation, err = providerLimitRecordGeneration(object["generation"])
		if err != nil {
			return nil, err
		}
		lease, ok := object["lease"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("provider limit probe owner lacks its exact token")
		}
		owner.Lease.Key, _ = lease[recordFieldKey].(string)
		owner.Lease.Until, err = providerLimitRecordTime(lease[recordFieldUntil])
		if err != nil {
			return nil, err
		}
		if !validProviderLimitProbeOwner(owner) || identity != ProviderLimitWaitIdentity(owner.SessionID, owner.TurnID) {
			return nil, fmt.Errorf("provider limit probe owner lacks its dispatch identity")
		}
		owners[identity] = owner
	}
	return owners, nil
}

func PutProviderLimitProbeOwner(record map[string]interface{}, owner ProviderLimitProbeOwner) (map[string]interface{}, error) {
	if !validProviderLimitProbeOwner(owner) {
		return nil, fmt.Errorf("provider limit probe owner lacks its dispatch identity")
	}
	if _, err := ReadProviderLimitProbeOwners(record); err != nil {
		return nil, err
	}
	updated := maps.Clone(record)
	if updated == nil {
		updated = make(map[string]interface{})
	}
	current, _ := record[ProviderLimitProbeOwnersKey].(map[string]interface{})
	entries := maps.Clone(current)
	if entries == nil {
		entries = make(map[string]interface{})
	}
	entries[ProviderLimitWaitIdentity(owner.SessionID, owner.TurnID)] = map[string]interface{}{
		"launch_id": owner.LaunchID, recordFieldSessionID: owner.SessionID, "turn_id": owner.TurnID,
		recordFieldWorkflowStepID: owner.WorkflowStepID, recordFieldModel: owner.Model,
		recordFieldExecutionID: owner.ExecutionID, "generation": owner.Generation, "succeeded": owner.Succeeded,
		"lease": map[string]interface{}{recordFieldKey: owner.Lease.Key, recordFieldUntil: owner.Lease.Until.UTC().Format(time.RFC3339Nano)},
	}
	updated[ProviderLimitProbeOwnersKey] = entries
	return updated, nil
}

func RemoveProviderLimitProbeOwner(record map[string]interface{}, identity string) (map[string]interface{}, error) {
	if _, err := ReadProviderLimitProbeOwners(record); err != nil {
		return nil, err
	}
	updated := maps.Clone(record)
	current, _ := record[ProviderLimitProbeOwnersKey].(map[string]interface{})
	entries := maps.Clone(current)
	delete(entries, identity)
	if len(entries) == 0 {
		delete(updated, ProviderLimitProbeOwnersKey)
	} else {
		updated[ProviderLimitProbeOwnersKey] = entries
	}
	return updated, nil
}

func validProviderLimitProbeOwner(owner ProviderLimitProbeOwner) bool {
	return owner.LaunchID != "" && owner.SessionID != "" && owner.TurnID != "" && owner.WorkflowStepID != "" && owner.Model != "" && owner.ExecutionID != "" && owner.Generation != 0 && owner.Lease.Key != "" && !owner.Lease.Until.IsZero()
}
