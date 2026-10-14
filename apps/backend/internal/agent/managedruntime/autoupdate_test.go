package managedruntime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.1
func TestAutomaticRuntimeConsentPersistsAndDoesNotCrossRuntimeIdentity(t *testing.T) {
	settings := &memorySettings{}
	ctx := context.Background()
	store := NewAutoUpdateStore(settings)
	policy, err := store.Get(ctx, "claude-acp", "npm:claude")
	if err != nil || policy.Enabled {
		t.Fatalf("default policy: %+v, %v", policy, err)
	}
	policy = AutoUpdatePolicy{RuntimeID: "npm:claude", Enabled: true, AttemptedVersion: "1.2.0", Outcome: &UpdateOutcome{ID: "attempt-1", Status: "failed", PreviousVersion: "1.1.0", TargetVersion: "1.2.0", FinishedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}}
	if err := store.Save(ctx, "claude-acp", policy); err != nil {
		t.Fatal(err)
	}
	restarted := NewAutoUpdateStore(settings)
	got, err := restarted.Get(ctx, "claude-acp", "npm:claude")
	if err != nil || !got.Enabled || got.AttemptedVersion != "1.2.0" || got.Outcome == nil || got.Outcome.PreviousVersion != "1.1.0" {
		t.Fatalf("restart policy: %+v, %v", got, err)
	}
	for _, id := range []string{"npm:changed-package", "native:claude"} {
		got, err := restarted.Get(ctx, "claude-acp", id)
		if err != nil || got.Enabled || got.Outcome != nil {
			t.Fatalf("cross-runtime consent: %+v, %v", got, err)
		}
	}
	got, err = restarted.Get(ctx, "codex-acp", "npm:claude")
	if err != nil || got.Enabled {
		t.Fatal("consent crossed agent identity")
	}
}

func TestAutomaticRuntimePolicyFailsClosedOnPersistenceFailure(t *testing.T) {
	settings := &memorySettings{err: errors.New("store offline")}
	store := NewAutoUpdateStore(settings)
	if policy, err := store.Get(context.Background(), "agent", "npm:package"); err == nil || policy.Enabled {
		t.Fatal("failed read authorized automation")
	}
	if err := store.Save(context.Background(), "agent", AutoUpdatePolicy{RuntimeID: "npm:package", Enabled: true}); err == nil {
		t.Fatal("failed save reported success")
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-004.1, AC-AGENTS-RUNTIME-NOTIFY-004.2
func TestOMPAutomaticPolicyPreservesDurableFieldsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	raw := []byte(`{"runtime_id":"omp-acp","enabled":true,"last_attempt_started_at":"2026-10-10T12:00:00Z","consecutive_failures":2,"paused":true,"last_change":{"id":"changed","previous_version":"1.0.0","observed_version":"1.1.0","finished_at":"2026-10-10T12:01:00Z"},"outcome":{"id":"check","status":"succeeded","update_mode":"self_update","operation":"up_to_date","observed_version":"1.1.0","output_excerpt":"declined\nchannel"}}`)
	settings := &memorySettings{values: map[string][]byte{"agent_runtime.auto.omp-acp": raw}}
	policy, err := NewAutoUpdateStore(settings).Get(ctx, "omp-acp", "omp-acp")
	if err != nil {
		t.Fatal(err)
	}
	if err := NewAutoUpdateStore(settings).Save(ctx, "omp-acp", policy); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(settings.values["agent_runtime.auto.omp-acp"], &got); err != nil {
		t.Fatal(err)
	}
	if got["last_attempt_started_at"] != "2026-10-10T12:00:00Z" || got["consecutive_failures"] != float64(2) || got["paused"] != true || got["last_change"] == nil {
		t.Fatalf("durable check policy lost on restart: %+v", got)
	}
	outcome := got["outcome"].(map[string]any)
	if outcome["update_mode"] != "self_update" || outcome["operation"] != "up_to_date" || outcome["observed_version"] != "1.1.0" || outcome["output_excerpt"] != "declined\nchannel" {
		t.Fatalf("durable observation lost: %+v", outcome)
	}
}
