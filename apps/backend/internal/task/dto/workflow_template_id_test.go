package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestFromWorkflowIncludesTemplateIDAndKeepsVisibilityFields(t *testing.T) {
	templateID := "improve-kandev"
	encoded, err := json.Marshal(FromWorkflow(&models.Workflow{
		ID:                 "workflow-1",
		WorkflowTemplateID: &templateID,
		SortOrder:          3,
		Hidden:             true,
		Style:              "kanban",
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
	if payload["sort_order"] != float64(3) {
		t.Fatalf("sort_order = %#v, want 3", payload["sort_order"])
	}
	if payload["hidden"] != true {
		t.Fatalf("hidden = %#v, want true", payload["hidden"])
	}
	if payload["style"] != "kanban" {
		t.Fatalf("style = %#v, want kanban", payload["style"])
	}
}
