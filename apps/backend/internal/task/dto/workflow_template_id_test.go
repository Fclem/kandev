package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestFromWorkflowIncludesTemplateID(t *testing.T) {
	templateID := "improve-kandev"
	encoded, err := json.Marshal(FromWorkflow(&models.Workflow{
		ID:                 "workflow-1",
		WorkflowTemplateID: &templateID,
	}))
	if err != nil {
		t.Fatalf("marshal workflow DTO: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal workflow DTO: %v", err)
	}
	if payload["workflow_template_id"] != templateID {
		t.Fatalf("workflow_template_id = %#v, want %q", payload["workflow_template_id"], templateID)
	}
}
