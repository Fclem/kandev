package controller

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/common/logger"
)

func newDefaultRegistryController(t *testing.T) *Controller {
	t.Helper()
	t.Setenv("KANDEV_E2E_MOCK", "false")
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	disc, err := discovery.LoadRegistry(context.Background(), reg, log)
	if err != nil {
		t.Fatal(err)
	}
	return &Controller{agentRegistry: reg, discovery: disc, logger: log}
}

// The production registry includes virtual agents whose IsInstalled is
// unsupported; status reads must keep using cached discovery, not per-agent probes.
func TestRuntimeUpdateStatusesSucceedWithDefaultRegistryAndDiscovery(t *testing.T) {
	c := newDefaultRegistryController(t)
	resp, err := c.ListAgentUpdateStatuses(context.Background())
	if err != nil || len(resp.Statuses) == 0 {
		t.Fatalf("statuses = %d, err = %v", len(resp.Statuses), err)
	}
}

func TestAgentAvailableTreatsVirtualAgentsAsUnavailable(t *testing.T) {
	c := newDefaultRegistryController(t)
	found := false
	for _, ag := range c.agentRegistry.List() {
		if !agents.IsVirtualAgent(ag) {
			continue
		}
		found = true
		if ok, err := c.agentAvailable(context.Background(), ag); ok || err != nil {
			t.Fatalf("virtual agent %s: available=%v err=%v", ag.ID(), ok, err)
		}
	}
	if !found {
		t.Fatal("default registry has no virtual agent to exercise")
	}
}

func TestOMPAutomaticNoChangeSkipsRefreshSideEffects(t *testing.T) {
	c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	var refreshes, invalidations atomic.Int32
	c.updateJobStore.onRefresh = func() { refreshes.Add(1) }
	c.updateJobStore.SetStatusInvalidator(func(string) { invalidations.Add(1) })
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	u.refreshCaps = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"}
	t.Setenv("KANDEV_E2E_MOCK", "true")
	status := dto.AgentUpdateStatusDTO{AgentName: "omp-acp", RuntimeID: "omp-acp", Available: true, Enabled: true, CurrentVersion: "1.0.0"}
	if err := c.enqueueAutomaticHarness(ctx, status, time.Now()); err != nil {
		t.Fatal(err)
	}
	jobs := c.ListAgentUpdateJobs()
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d", len(jobs))
	}
	waitForUpdateStatus(t, hub.completed, jobs[0].JobID, dto.AgentUpdateJobStatusSucceeded)
	c.updateJobStore.automaticWorkers.Wait()
	p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if p.Outcome == nil || p.Outcome.Operation != managedruntime.OperationUpToDate {
		t.Fatalf("outcome = %+v", p.Outcome)
	}
	if refreshes.Load() != 0 || invalidations.Load() != 0 {
		t.Fatalf("no-change check refreshed=%d invalidated=%d", refreshes.Load(), invalidations.Load())
	}
}

// A manual update can finish between the status snapshot and admission; the
// automatic check must compare against the version current under the claim.
func TestOMPAutomaticPreviousVersionIsReadUnderClaim(t *testing.T) {
	c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KANDEV_E2E_MOCK", "true")
	// Snapshot says 1.0.0, but a manual update already published 1.1.0; the automatic updater changes nothing.
	u.current = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.1.0"}
	u.refreshCaps = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.1.0"}
	stale := dto.AgentUpdateStatusDTO{AgentName: "omp-acp", RuntimeID: "omp-acp", Available: true, Enabled: true, CurrentVersion: "1.0.0"}
	if err := c.enqueueAutomaticHarness(ctx, stale, time.Now()); err != nil {
		t.Fatal(err)
	}
	jobs := c.ListAgentUpdateJobs()
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d", len(jobs))
	}
	waitForUpdateStatus(t, hub.completed, jobs[0].JobID, dto.AgentUpdateJobStatusSucceeded)
	c.updateJobStore.automaticWorkers.Wait()
	p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if p.LastChange != nil || p.Outcome == nil || p.Outcome.Operation != managedruntime.OperationUpToDate || p.Outcome.PreviousVersion != "1.1.0" {
		t.Fatalf("stale snapshot reported a change: %+v change=%+v", p.Outcome, p.LastChange)
	}
}
