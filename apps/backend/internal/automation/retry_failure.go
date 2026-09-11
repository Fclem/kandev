package automation

import (
	"context"
)

// FinalizeAutomationRetryFailure is the single service entry point for a
// retry-capable failure. It keeps generation and run identity server-owned.
func (s *Service) FinalizeAutomationRetryFailure(ctx context.Context, runID string, generation int64, raw error, phase string) (*AutomationRun, error) {
	return s.store.FinalizeRetryFailure(ctx, runID, generation, raw, phase)
}
