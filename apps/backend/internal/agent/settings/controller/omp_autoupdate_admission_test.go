package controller

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestOMPAutomaticReenableKeepsCadenceTimestamp(t *testing.T) {
	c, _, _ := newOMPAutomaticController(t, &autoMemorySettings{})
	ctx := context.Background()
	started := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	p := managedruntime.AutoUpdatePolicy{RuntimeID: "omp-acp", Enabled: true, Paused: true, ConsecutiveFailures: 3, LastAttemptStartedAt: started}
	if err := c.runtimeAutoUpdateStore.Save(ctx, "omp-acp", p); err != nil {
		t.Fatal(err)
	}
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	got, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if !got.Paused || got.ConsecutiveFailures != 3 {
		t.Fatal("idempotent enable reset pause")
	}
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", false); err != nil {
		t.Fatal(err)
	}
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	got, _ = c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if got.Paused || got.ConsecutiveFailures != 0 || !got.LastAttemptStartedAt.Equal(started) {
		t.Fatalf("saved recovery=%+v", got)
	}
}

func TestOMPAutomaticFullSemaphoreDefersWithoutJob(t *testing.T) {
	c, _, _ := newOMPAutomaticController(t, &autoMemorySettings{})
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	for range cap(c.updateJobStore.semaphore) {
		c.updateJobStore.semaphore <- struct{}{}
	}
	defer func() {
		for len(c.updateJobStore.semaphore) > 0 {
			<-c.updateJobStore.semaphore
		}
	}()
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if len(c.ListAgentUpdateJobs()) != 0 || p.Outcome != nil || !p.LastAttemptStartedAt.IsZero() {
		t.Fatalf("full semaphore consumed admission=%+v", p)
	}
}

func TestOMPAutomaticAndManualNeverOverlap(t *testing.T) {
	c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	started, release := make(chan struct{}), make(chan struct{})
	u.run = func(ctx context.Context, _ agents.Command, _ func(string)) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Cleanup(cancel)
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	waitForRuntimeSignal(t, started, "updater")
	if _, err := c.EnqueueAgentUpdate(ctx, "omp-acp", ""); err == nil {
		t.Fatal("manual updater competed with automatic updater")
	} else {
		var conflict *MaintenanceConflictError
		if !errors.As(err, &conflict) {
			t.Fatal(err)
		}
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	if len(c.ListAgentUpdateJobs()) != 1 {
		t.Fatal("competing check registered")
	}
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", false); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusSucceeded)
	p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if p.Enabled || p.Outcome.Status != "succeeded" {
		t.Fatalf("post-admission withdrawal erased result: %+v", p)
	}
}

func TestOMPManualFinishNeverCallsAutomaticHook(t *testing.T) {
	c, _, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	called := make(chan struct{}, 1)
	c.updateJobStore.onAutomaticFinished = func(dto.AgentUpdateJobDTO, automaticHarnessResult) { called <- struct{}{} }
	ctx := context.Background()
	p := managedruntime.AutoUpdatePolicy{RuntimeID: "omp-acp", Enabled: true, Paused: true, ConsecutiveFailures: 3}
	if err := c.runtimeAutoUpdateStore.Save(ctx, "omp-acp", p); err != nil {
		t.Fatal(err)
	}
	job, err := c.EnqueueAgentUpdate(ctx, "omp-acp", "")
	if err != nil {
		t.Fatal(err)
	}
	waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
	select {
	case <-called:
		t.Fatal("manual finish dispatched automatic hook")
	default:
	}
	got, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if !got.Enabled || !got.Paused || got.ConsecutiveFailures != 3 {
		t.Fatalf("manual update changed consent/pause: %+v", got)
	}
}

func TestOMPAutomaticGuardRejectionPersistsNothing(t *testing.T) {
	c, _, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	c.discovery, _ = discovery.LoadRegistry(ctx, c.agentRegistry, c.logger)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("KANDEV_E2E_MOCK", "false")
	stale := dto.AgentUpdateStatusDTO{AgentName: "omp-acp", RuntimeID: "omp-acp", Available: true, Enabled: true, CurrentVersion: "1.0.0"}
	if err := c.enqueueAutomaticHarness(ctx, stale, time.Now()); err != nil {
		t.Fatal(err)
	}
	c.updateJobStore.automaticWorkers.Wait()
	p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if len(c.ListAgentUpdateJobs()) != 0 || p.Outcome != nil || !p.LastAttemptStartedAt.IsZero() {
		t.Fatalf("removed executable admitted check: %+v", p)
	}
	// The same availability boundary honours the E2E registry contract.
	t.Setenv("KANDEV_E2E_MOCK", "true")
	if err := c.enqueueAutomaticHarness(ctx, stale, time.Now()); err != nil {
		t.Fatal(err)
	}
	jobs := c.ListAgentUpdateJobs()
	if len(jobs) != 1 {
		t.Fatal("mock-mode availability not honoured")
	}
	waitForUpdateStatus(t, hub.completed, jobs[0].JobID, dto.AgentUpdateJobStatusSucceeded)
}

func TestOMPAutomaticAvailabilityCannotBlockWithdrawal(t *testing.T) {
	c, _, _ := newOMPAutomaticController(t, &autoMemorySettings{})
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	checking := make(chan struct{})
	release := make(chan struct{})
	c.runtimeAgentAvailable = func(context.Context, agents.Agent) (bool, error) {
		close(checking)
		<-release
		return true, nil
	}
	done := make(chan error, 1)
	go func() {
		done <- c.enqueueAutomaticHarness(ctx, dto.AgentUpdateStatusDTO{AgentName: "omp-acp", RuntimeID: "omp-acp", Available: true, Enabled: true}, time.Now())
	}()
	<-checking
	withdrawn := make(chan error, 1)
	go func() { withdrawn <- c.SetAgentAutomaticUpdates(ctx, "omp-acp", false) }()
	select {
	case err := <-withdrawn:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		close(release)
		<-done
		t.Fatal("availability held consent mutex")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if p.Enabled || p.Outcome != nil || !p.LastAttemptStartedAt.IsZero() || len(c.ListAgentUpdateJobs()) != 0 {
		t.Fatalf("pre-start withdrawal admitted a job: %+v", p)
	}
}

type ompAdmissionSettings struct {
	autoMemorySettings
	beforeSave func() error
}

func (s *ompAdmissionSettings) Save(ctx context.Context, key string, value []byte) error {
	if s.beforeSave != nil {
		if err := s.beforeSave(); err != nil {
			return err
		}
	}
	return s.autoMemorySettings.Save(ctx, key, value)
}

func TestOMPAutomaticClaimNeverVisibleWithoutRegisteredJob(t *testing.T) {
	c, u, _ := newOMPAutomaticController(t, &autoMemorySettings{})
	settings := &ompAdmissionSettings{}
	c.SetRuntimeAutoUpdateStore(managedruntime.NewAutoUpdateStore(settings))
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	t.Cleanup(func() { close(gate) })
	u.run = func(ctx context.Context, _ agents.Command, _ func(string)) error {
		select {
		case <-gate:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	type claimResult struct {
		ref     MaintenanceJobRef
		claimed bool
		err     error
	}
	competing := make(chan claimResult, 1)
	exposed := false
	settings.beforeSave = func() error {
		go func() {
			ref, claimed, err := c.maintenance.claim("omp-acp", MaintenanceKindInstall, "install")
			competing <- claimResult{ref, claimed, err}
		}()
		select {
		case result := <-competing:
			if result.err != nil {
				_, registered := c.updateJobStore.jobs[result.ref.JobID]
				exposed = !registered
			}
			competing <- result
		case <-time.After(100 * time.Millisecond):
		}
		return nil
	}
	stale := dto.AgentUpdateStatusDTO{AgentName: "omp-acp", RuntimeID: "omp-acp", Available: true, Enabled: true, CurrentVersion: "1.0.0"}
	if err := c.enqueueAutomaticHarness(ctx, stale, time.Now()); err != nil {
		t.Fatal(err)
	}
	settings.beforeSave = nil
	result := <-competing
	if exposed {
		t.Fatal("maintenance claimant observed an automatic claim before its job was registered")
	}
	if result.claimed {
		t.Fatal("install overlapped admitted updater")
	}
	job, found := c.updateJobStore.Get(result.ref.JobID)
	if !found || !job.Automatic {
		t.Fatalf("conflict cannot hydrate an admitted job: %+v", result)
	}
}

type ompAdmissionBroadcaster struct{ events atomic.Int32 }

func (b *ompAdmissionBroadcaster) Broadcast(*ws.Message) { b.events.Add(1) }

func TestOMPAutomaticRefusedAdmissionCreatesNoJobAndBroadcastsNothing(t *testing.T) {
	c, _, _ := newOMPAutomaticController(t, &autoMemorySettings{})
	settings := &ompTerminalFailSettings{}
	c.SetRuntimeAutoUpdateStore(managedruntime.NewAutoUpdateStore(settings))
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	hub := &ompAdmissionBroadcaster{}
	c.SetJobBroadcaster(hub)
	settings.reject.Store(true)
	status := dto.AgentUpdateStatusDTO{AgentName: "omp-acp", RuntimeID: "omp-acp", Available: true, Enabled: true}
	if err := c.enqueueAutomaticHarness(ctx, status, time.Now()); err == nil {
		t.Fatal("marker write unexpectedly succeeded")
	}
	if _, found := c.updateJobStore.GetActive("omp-acp"); found || len(c.updateJobStore.ListAll()) != 0 {
		t.Fatal("unadmitted job visible")
	}
	p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if p.Outcome != nil || !p.LastAttemptStartedAt.IsZero() {
		t.Fatalf("refused marker consumed cadence: %+v", p)
	}
	if hub.events.Load() != 0 {
		t.Fatal("unadmitted job broadcast an event")
	}
	ref, claimed, err := c.maintenance.claim("omp-acp", MaintenanceKindInstall, "install")
	if err != nil || !claimed {
		t.Fatalf("failed persistence leaked claim: %v", err)
	}
	c.maintenance.release("omp-acp", ref)
}

func TestOMPAutomaticRequiresTreeUpdater(t *testing.T) {
	c, u, _ := newOMPAutomaticController(t, &autoMemorySettings{})
	c.SetRuntimeUpdater(&u.harnessTestUpdater)
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err == nil {
		t.Fatal("unbounded updater accepted automatic consent")
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.runCalls != 0 || len(c.ListAgentUpdateJobs()) != 0 {
		t.Fatal("automatic path fell back to unbounded manual updater")
	}
}
