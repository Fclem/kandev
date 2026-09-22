package github

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// rateLimitSeedTimeout caps the startup `gh api rate_limit` probe so a slow
// CLI cannot delay backend boot.
const rateLimitSeedTimeout = 5 * time.Second

// attachRateTracker wires the rate tracker into whichever client
// implementation we ended up with, then seeds initial snapshots. The seed is
// best-effort — a missing CLI or network blip should not block startup.
func attachRateTracker(client Client, tracker *RateTracker, log *logger.Logger) {
	wireRateTracker(client, tracker)
	c, ok := client.(*GHClient)
	if !ok {
		return
	}
	seedCtx, cancel := context.WithTimeout(context.Background(), rateLimitSeedTimeout)
	defer cancel()
	if err := c.FetchRateLimit(seedCtx); err != nil {
		log.Debug("seed gh rate limit failed", zap.Error(err))
	}
}

func wireRateTracker(client Client, tracker *RateTracker) {
	switch c := client.(type) {
	case *PATClient:
		c.WithRateTracker(tracker)
	case *GHClient:
		c.WithRateTracker(tracker)
	}
}

func wireRateAdmission(client Client, admission *RateAdmission) {
	switch c := client.(type) {
	case *PATClient:
		c.withRateAdmission(admission)
	case *GHClient:
		c.withRateAdmission(admission)
	}
}

func clientRateTracker(client Client) *RateTracker {
	switch c := client.(type) {
	case *PATClient:
		if c != nil {
			return c.rateTracker
		}
	case *GHClient:
		if c != nil {
			return c.rateTracker
		}
	}
	return nil
}
