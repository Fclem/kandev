package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestToTaskUsageTotalsDTO_ZeroUsage_NullTimestamps(t *testing.T) {
	totals := &models.TaskUsageTotals{OutputTokensComplete: true}

	got := ToTaskUsageTotalsDTO(TaskUsageTotalsScopeTask, "task-1", totals)
	if got.Scope != TaskUsageTotalsScopeTask || got.ScopeID != "task-1" {
		t.Fatalf("scope = (%q, %q), want (task, task-1)", got.Scope, got.ScopeID)
	}
	if got.EventCount != 0 || got.TokensTotal != 0 {
		t.Fatalf("got = %+v, want every sum zero", got)
	}
	if !got.OutputTokensComplete {
		t.Error("OutputTokensComplete = false, want true")
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	firstEventAt, ok := decoded["first_event_at"]
	if !ok {
		t.Fatal("first_event_at must be present in the JSON body, not omitted")
	}
	if firstEventAt != nil {
		t.Errorf("first_event_at = %v, want JSON null", firstEventAt)
	}
	lastEventAt, ok := decoded["last_event_at"]
	if !ok {
		t.Fatal("last_event_at must be present in the JSON body, not omitted")
	}
	if lastEventAt != nil {
		t.Errorf("last_event_at = %v, want JSON null", lastEventAt)
	}
}
