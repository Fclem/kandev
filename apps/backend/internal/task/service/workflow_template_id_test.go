package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func TestService_CreateWorkflowEventIncludesTemplateID(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-1", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	templateID := "report-kandev-issue"
	if _, err := svc.CreateWorkflow(ctx, &CreateWorkflowRequest{
		WorkspaceID:        "workspace-1",
		Name:               "Open issue",
		WorkflowTemplateID: &templateID,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	published := eventBus.GetPublishedEvents()
	var payload map[string]interface{}
	for _, event := range published {
		if event.Type == events.WorkflowCreated {
			payload, _ = event.Data.(map[string]interface{})
		}
	}
	got, ok := payload["workflow_template_id"].(*string)
	if !ok || got == nil || *got != templateID {
		t.Fatalf("workflow.created payload = %#v, want workflow_template_id %q", payload, templateID)
	}
}
