package sessioncatalog

import (
	"context"
	"errors"
)

// Sanitized publication failure causes for logs. Errors from storage, runtime
// and provider paths can embed private detail, so callers log one of these
// categories instead of the error text.
const (
	FailureCauseNone             = "none"
	FailureCauseDeadlineExceeded = "deadline_exceeded"
	FailureCauseCanceled         = "canceled"
	FailureCauseError            = "error"
)

// FailureCause classifies err without exposing its message.
func FailureCause(err error) string {
	switch {
	case err == nil:
		return FailureCauseNone
	case errors.Is(err, context.DeadlineExceeded):
		return FailureCauseDeadlineExceeded
	case errors.Is(err, context.Canceled):
		return FailureCauseCanceled
	default:
		return FailureCauseError
	}
}
