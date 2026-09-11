package automation

import "context"

func (s *Service) MarkAutomationRetrySucceeded(ctx context.Context, runID string, generation int64) error {
	return s.store.MarkRetrySucceeded(ctx, runID, generation)
}
