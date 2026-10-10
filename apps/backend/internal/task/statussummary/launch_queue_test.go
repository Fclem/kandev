package statussummary

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestLaunchQueueSummaryFromTaskProjectsProviderLimitDeadlineWithoutPayload(t *testing.T) {
	reset := time.Date(2026, 10, 4, 12, 0, 0, 123456789, time.UTC)
	launch, err := models.PutProviderLimitLaunch(nil, models.ProviderLimitLaunch{
		ID: "launch-one", Kind: models.CeilingLaunchStart, Origin: "automatic",
		WorkflowStepID: "step-one", MarkKey: "private-binding-key", Model: "vendor/model",
		NotBefore: reset, QueuedAt: reset.Add(-time.Minute), SessionID: "session-one",
		Payload: map[string]interface{}{"agent_profile_id": "profile-one", "prompt": "private prompt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	summary := LaunchQueueSummaryFromTask(&models.Task{ID: "task-one", Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: launch}})
	if summary == nil || summary.Reason != LaunchQueueReasonProviderLimit || summary.Model != "vendor/model" || summary.RetryAt == nil || !summary.RetryAt.Equal(reset) || summary.SessionID != "session-one" || summary.AgentProfileID != "profile-one" {
		t.Fatalf("provider limit queue projection lost its bounded wait identity: %+v", summary)
	}
	if summary.Capacity != nil {
		t.Fatalf("provider wait unexpectedly projected session capacity: %+v", summary.Capacity)
	}
	if err := validateLaunchQueue(summary); err != nil {
		t.Fatalf("provider limit wait summary rejected as an unknown queue reason: %v", err)
	}
}
