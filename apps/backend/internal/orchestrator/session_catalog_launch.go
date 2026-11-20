package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

func (s *Service) acceptedCatalogLaunch(ctx context.Context, execution *executor.TaskExecution, attempt *resumeAttempt) *executor.TaskExecution {
	if execution == nil || attempt == nil || execution.AgentExecutionID == "" {
		return execution
	}
	owner := sessioncatalog.Owner{ExecutionID: execution.AgentExecutionID, AttemptID: attempt.identity()}
	if reader, ok := s.agentManager.(catalogRefreshExecutionReader); ok {
		evidence, err := reader.CatalogRefreshExecution(ctx, execution.SessionID, sessioncatalog.LaunchContext{})
		if err == nil && evidence.Owner.ExecutionID == owner.ExecutionID && evidence.Owner.AttemptID == owner.AttemptID {
			owner.SourceGeneration = evidence.Owner.SourceGeneration
		}
	}
	response := *execution
	response.AcceptedLaunchOwner = &owner
	return &response
}
