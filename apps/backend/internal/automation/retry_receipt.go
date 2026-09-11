package automation

import (
	"context"
	"strconv"
)

func (s *Service) AcknowledgeRetryEvent(ctx context.Context, runID string, version int64) error {
	if version == 0 {
		version = 1
	}
	return s.store.AcknowledgeRetryEvent(ctx, runID+":"+strconv.FormatInt(version, 10), runID, version)
}
