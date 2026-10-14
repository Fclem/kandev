package controller

import (
	"context"
	"os"

	"github.com/kandev/kandev/internal/agent/agents"
)

func (c *Controller) agentAvailable(ctx context.Context, ag agents.Agent) (bool, error) {
	if c.runtimeAgentAvailable != nil {
		return c.runtimeAgentAvailable(ctx, ag)
	}
	if c.discovery == nil {
		return true, ctx.Err()
	}
	if os.Getenv("KANDEV_E2E_MOCK") == "true" || (ag.ID() == "mock-agent" && ag.Enabled()) {
		return true, ctx.Err()
	}
	result, err := ag.IsInstalled(ctx)
	return result != nil && result.Available, err
}
