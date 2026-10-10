package providerretry

import (
	"math"
	"testing"
	"time"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.7
func TestDurationRepresentableBoundary(t *testing.T) {
	max := math.MaxInt64 / int64(time.Millisecond)
	if got := Duration(&max); got != time.Duration(max)*time.Millisecond {
		t.Fatalf("maximum delay = %v", got)
	}
	invalid := max + 1
	if got := Duration(&invalid); got != 0 {
		t.Fatalf("overflow delay = %v", got)
	}
	if got := Milliseconds(invalid); got != nil {
		t.Fatalf("overflow accepted: %v", *got)
	}
}
