package controller

import (
	"context"
	"os"

	"github.com/kandev/kandev/internal/agent/agents"
)

const (
	mockAgentID = "mock-agent"
	envTrue     = "true"
)

// agentAvailable is the single-agent availability check used by OMP admission.
// The status path uses the cached multi-agent detection instead. Virtual and
// disabled agents are unavailable, and a per-agent probe failure that is not a
// context error means unavailable rather than failing the caller.
func (c *Controller) agentAvailable(ctx context.Context, ag agents.Agent) (bool, error) {
	if c.runtimeAgentAvailable != nil {
		return c.runtimeAgentAvailable(ctx, ag)
	}
	if c.discovery == nil {
		return true, ctx.Err()
	}
	if os.Getenv("KANDEV_E2E_MOCK") == envTrue || (ag.ID() == mockAgentID && ag.Enabled()) {
		return true, ctx.Err()
	}
	if agents.IsVirtualAgent(ag) || !ag.Enabled() {
		return false, ctx.Err()
	}
	result, err := ag.IsInstalled(ctx)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, ctxErr
	}
	if err != nil {
		return false, nil
	}
	return result != nil && result.Available, nil
}
