package models

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.9
func TestProviderLimitDeferralMutationPreservesSiblingCeilingAndDependency(t *testing.T) {
	reset := time.Date(2026, 10, 4, 12, 0, 0, 123456789, time.UTC)
	first := ProviderLimitWait{SessionID: "s1", TurnID: "t1", MarkKey: "limit|binding|model|one", Model: "one", NotBefore: reset, QueuedAt: reset.Add(-time.Hour), Payload: map[string]interface{}{"prompt": "first input"}}
	second := first
	second.SessionID, second.TurnID, second.Model = "s2", "t2", "two"
	second.Payload = map[string]interface{}{"prompt": "second input"}
	ceiling := CeilingRecordKeys(sampleDeferral())
	ceiling[DeferredLaunchStartWhenUnblockedKey] = true
	record, err := PutProviderLimitWait(ceiling, first)
	if err != nil {
		t.Fatal(err)
	}
	record, err = PutProviderLimitWait(record, second)
	if err != nil {
		t.Fatal(err)
	}
	before := record
	record, err = RemoveProviderLimitWait(record, first.SessionID, first.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	waits, err := ReadProviderLimitWaits(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(waits) != 1 || !reflect.DeepEqual(waits[ProviderLimitWaitIdentity("s2", "t2")], second) {
		t.Fatalf("sibling wait was replaced or cancelled: %+v", waits)
	}
	if got, err := ReadCeilingDeferral(record); err != nil || !reflect.DeepEqual(got.Payload, sampleDeferral().Payload) || record[DeferredLaunchStartWhenUnblockedKey] != true {
		t.Fatalf("ceiling or dependency intent was lost: %+v, %v", record, err)
	}
	original, err := ReadProviderLimitWaits(before)
	if err != nil || len(original) != 2 {
		t.Fatalf("mutation changed its CAS snapshot: %+v, %v", original, err)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.6
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.2
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.4
func TestProviderLimitDeferralRestartKeepsNanosecondsPayloadAndExactLease(t *testing.T) {
	reset := time.Date(2026, 10, 4, 12, 0, 0, 123456789, time.UTC)
	wait := ProviderLimitWait{SessionID: "s1", TurnID: "failed-turn", MarkKey: "limit|binding|account", Model: "one", WorkflowStepID: "entry", NotBefore: reset, QueuedAt: reset.Add(-time.Hour), FailureExecutionID: "failed-execution", FailureGeneration: 4, ProbeTurnID: "resumed-turn", ProbeExecutionID: "probe-execution", ProbeGeneration: 5, ProbeLease: &ProviderLimitProbeLease{Key: "limit|binding|account", Until: reset.Add(10 * time.Minute)}, Payload: map[string]interface{}{"prompt": "continue", "model": "one", "attachments": []interface{}{map[string]interface{}{"name": "input.txt", "data": "aGVsbG8="}}}}
	record, err := PutProviderLimitWait(nil, wait)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var restored map[string]interface{}
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	waits, err := ReadProviderLimitWaits(restored)
	if err != nil {
		t.Fatal(err)
	}
	if got := waits[ProviderLimitWaitIdentity("s1", "failed-turn")]; !reflect.DeepEqual(got, wait) {
		t.Fatalf("restart changed exact wait/probe identity or replay input: got=%+v want=%+v", got, wait)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.5
func TestProviderLimitDeferralStaleCancellationCannotRemoveSuccessor(t *testing.T) {
	reset := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	wait := ProviderLimitWait{SessionID: "s1", TurnID: "successor", MarkKey: "limit|binding|account", Model: "one", NotBefore: reset, QueuedAt: reset}
	record, err := PutProviderLimitWait(nil, wait)
	if err != nil {
		t.Fatal(err)
	}
	record, err = RemoveProviderLimitWait(record, "s1", "predecessor")
	if err != nil {
		t.Fatal(err)
	}
	waits, err := ReadProviderLimitWaits(record)
	if err != nil || !reflect.DeepEqual(waits[ProviderLimitWaitIdentity("s1", "successor")], wait) {
		t.Fatalf("stale cancellation removed successor: %+v, %v", waits, err)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
func TestProviderLimitDeferralPublicProjectionExcludesReplayAndProbeTokens(t *testing.T) {
	reset := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	wait := ProviderLimitWait{SessionID: "s1", TurnID: "failed", MarkKey: "limit|private-binding|account", Model: "one", NotBefore: reset, QueuedAt: reset, ProbeLease: &ProviderLimitProbeLease{Key: "limit|private-binding|account", Until: reset.Add(10 * time.Minute)}, Payload: map[string]interface{}{"prompt": "private replay input", "attachments": "private attachment"}}
	record, err := PutProviderLimitWait(CeilingRecordKeys(sampleDeferral()), wait)
	if err != nil {
		t.Fatal(err)
	}
	public := PublicTaskMetadata(map[string]interface{}{MetaKeyDeferredLaunch: record})
	deferred := public[MetaKeyDeferredLaunch].(map[string]interface{})
	if _, exposed := deferred[ProviderLimitWaitsKey]; exposed {
		t.Fatal("public task metadata exposed private replay input and exact probe ownership")
	}
	if deferred[CeilingDeferredKey] != true {
		t.Fatal("redaction removed the public ceiling state")
	}
	if _, retained := record[ProviderLimitWaitsKey]; !retained {
		t.Fatal("public projection mutated the durable wait")
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.2
func TestProviderLimitDeferralLaunchProjectionKeepsPrivateLeaseAndReplayServerSide(t *testing.T) {
	reset := time.Date(2026, 10, 4, 12, 0, 0, 123456789, time.UTC)
	launch := ProviderLimitLaunch{
		ID: "launch-one", Kind: CeilingLaunchStart, Origin: "automatic", WorkflowStepID: "step-one",
		MarkKey: "limit|private-binding|account", Model: "anthropic/one",
		NotBefore: reset, QueuedAt: reset.Add(-time.Hour),
		ProbeLease: &ProviderLimitProbeLease{Key: "limit|private-binding|account", Until: reset.Add(time.Minute)},
		Payload:    map[string]interface{}{"prompt": "private input", "env": map[string]string{"SECRET": "private credential"}},
	}
	record, err := PutProviderLimitLaunch(map[string]interface{}{DeferredLaunchStartWhenUnblockedKey: true}, launch)
	if err != nil {
		t.Fatal(err)
	}
	public := PublicTaskMetadata(map[string]interface{}{MetaKeyDeferredLaunch: record})
	deferred := public[MetaKeyDeferredLaunch].(map[string]interface{})
	if _, exposed := deferred[ProviderLimitLaunchKey]; exposed {
		t.Fatal("public launch state exposed the credential binding and exact probe token")
	}
	if _, exposed := deferred[CeilingLaunchPayloadKey]; exposed {
		t.Fatal("public launch state exposed the private prompt and environment")
	}
	if deferred["provider_limit_deferred"] != true || deferred["provider_limit_model"] != launch.Model || deferred["provider_limit_retry_at"] != reset.Format(time.RFC3339Nano) || deferred[DeferredLaunchStartWhenUnblockedKey] != true {
		t.Fatalf("public projection lost the deadline or independent dependency intent: %+v", deferred)
	}
	restored, err := ReadProviderLimitLaunch(record)
	if err != nil || restored == nil || restored.ProbeLease == nil || !restored.ProbeLease.Until.Equal(launch.ProbeLease.Until) || !restored.NotBefore.Equal(reset) || restored.Payload["prompt"] != "private input" {
		t.Fatalf("projection mutated exact durable launch ownership: %+v error=%v", restored, err)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.3
func TestProviderLimitProbeOwnerSurvivesLaunchSlotRemovalWithoutPublicTokenLeak(t *testing.T) {
	until := time.Date(2026, 10, 4, 12, 0, 0, 123456789, time.UTC)
	owner := ProviderLimitProbeOwner{LaunchID: "original", SessionID: "s1", TurnID: "first-turn", WorkflowStepID: "step-one", Model: "one", ExecutionID: "execution-one", Generation: 7, Lease: ProviderLimitProbeLease{Key: "private-binding-account", Until: until}}
	record, err := PutProviderLimitProbeOwner(map[string]interface{}{DeferredLaunchStartWhenUnblockedKey: true}, owner)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := ReadProviderLimitProbeOwners(record)
	if err != nil || !reflect.DeepEqual(restored[ProviderLimitWaitIdentity(owner.SessionID, owner.TurnID)], owner) {
		t.Fatalf("exact dispatch ownership changed across the durable codec: %+v error=%v", restored, err)
	}
	public := PublicTaskMetadata(map[string]interface{}{MetaKeyDeferredLaunch: record})
	projected := public[MetaKeyDeferredLaunch].(map[string]interface{})
	if _, leaked := projected[ProviderLimitProbeOwnersKey]; leaked {
		t.Fatal("public task metadata exposed the post-launch credential binding and exact lease")
	}
	if projected[DeferredLaunchStartWhenUnblockedKey] != true {
		t.Fatal("probe owner projection dropped independent dependency work")
	}
}
