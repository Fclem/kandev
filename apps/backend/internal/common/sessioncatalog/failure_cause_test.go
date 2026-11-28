package sessioncatalog

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestFailureCauseNeverExposesErrorText(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, FailureCauseNone},
		{fmt.Errorf("probe: %w", context.DeadlineExceeded), FailureCauseDeadlineExceeded},
		{errors.Join(errors.New("TOKEN=secret"), context.Canceled), FailureCauseCanceled},
		{errors.New("TOKEN=secret"), FailureCauseError},
	} {
		if got := FailureCause(tc.err); got != tc.want {
			t.Errorf("FailureCause(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
