package controller

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

type ompAutomaticUpdater struct {
	harnessTestUpdater
	run   func(context.Context, agents.Command, func(string)) error
	probe func(context.Context) (hostutility.AgentCapabilities, error)
}

func (u *ompAutomaticUpdater) RunUpdateTree(ctx context.Context, command agents.Command, output func(string)) error {
	if u.run != nil {
		return u.run(ctx, command, output)
	}
	return u.RunUpdate(ctx, command, output)
}
func (u *ompAutomaticUpdater) Probe(ctx context.Context, name string, command agents.Command) (hostutility.AgentCapabilities, error) {
	if u.probe != nil {
		return u.probe(ctx)
	}
	return u.harnessTestUpdater.Probe(ctx, name, command)
}

func newOMPAutomaticController(t *testing.T, settings *autoMemorySettings) (*Controller, *ompAutomaticUpdater, *updateTerminalBroadcaster) {
	t.Helper()
	u := &ompAutomaticUpdater{}
	u.latest = "1.0.0"
	u.current = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"}
	u.currentFound = true
	u.refreshCaps = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.1.0"}
	c := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP()})
	c.SetRuntimeAutoUpdateStore(managedruntime.NewAutoUpdateStore(settings))
	c.SetRuntimeUpdater(u)
	hub := newUpdateTerminalBroadcaster()
	c.SetJobBroadcaster(hub)
	c.updateJobStore.onRefresh = nil
	t.Cleanup(c.updateJobStore.automaticWorkers.Wait)
	return c, u, hub
}
