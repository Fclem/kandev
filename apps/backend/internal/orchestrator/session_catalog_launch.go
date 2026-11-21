package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

// catalogExecutionOwnerReader reads the accepted execution owner only; the
// saved-source comparison is irrelevant to Resume ACK correlation.
type catalogExecutionOwnerReader interface {
	CatalogRefreshExecution(context.Context, string, string) (sessioncatalog.ExecutionEvidence, error)
}

func (s *Service) acceptedCatalogLaunch(ctx context.Context, execution *executor.TaskExecution, attempt *resumeAttempt) *executor.TaskExecution {
	if execution == nil || attempt == nil || execution.AgentExecutionID == "" {
		return execution
	}
	owner := sessioncatalog.Owner{ExecutionID: execution.AgentExecutionID, AttemptID: attempt.identity()}
	if reader, ok := s.agentManager.(catalogExecutionOwnerReader); ok {
		evidence, err := reader.CatalogRefreshExecution(ctx, execution.SessionID, "")
		if err == nil && evidence.Owner.ExecutionID == owner.ExecutionID && evidence.Owner.AttemptID == owner.AttemptID {
			owner.SourceGeneration = evidence.Owner.SourceGeneration
		}
	}
	response := *execution
	response.AcceptedLaunchOwner = &owner
	return &response
}
