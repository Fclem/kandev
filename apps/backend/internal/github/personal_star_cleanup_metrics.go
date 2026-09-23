package github

import "expvar"

// personalStarCleanupAttempts counts star-secret deletion attempts by outcome.
// The terminal outcome is the operator-visible signal that a generation-specific
// token could not be deleted and stays in the secret store for diagnosis.
var personalStarCleanupAttempts = expvar.NewMap("personal_star_cleanup_attempts_total")

// personalStar cleanup attempt outcomes.
const (
	personalStarCleanupOutcomeDeleted  = "deleted"
	personalStarCleanupOutcomeRetry    = "retry"
	personalStarCleanupOutcomeTerminal = "terminal"
)

// recordPersonalStarCleanupAttempt publishes one attempt outcome. The expvar
// map is process-global, so tests that record attempts still work: expvar
// counters only ever increase.
func recordPersonalStarCleanupAttempt(outcome string) {
	personalStarCleanupAttempts.Add(outcome, 1)
}
