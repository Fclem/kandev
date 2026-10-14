package controller

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-004.6, AC-AGENTS-RUNTIME-NOTIFY-004.7
func TestOMPAutomaticShutdownDuringProbeIsInterruptedNotPaused(t *testing.T) {
	c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	probing := make(chan struct{})
	u.probe = func(ctx context.Context) (hostutility.AgentCapabilities, error) {
		close(probing)
		<-ctx.Done()
		return hostutility.AgentCapabilities{Status: hostutility.StatusFailed}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	waitForRuntimeSignal(t, probing, "probe")
	cancel()
	waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusFailed)
	p, _ := c.runtimeAutoUpdateStore.Get(context.Background(), "omp-acp", "omp-acp")
	if p.Outcome.Status != "interrupted" || p.Paused || p.ConsecutiveFailures != 1 {
		t.Fatalf("shutdown accounting=%+v outcome=%+v", p, p.Outcome)
	}
}

func TestOMPAutomaticUpdaterDeadlineCountsAsFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
		u.run = func(ctx context.Context, _ agents.Command, _ func(string)) error { <-ctx.Done(); return ctx.Err() }
		ctx := context.Background()
		if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
			t.Fatal(err)
		}
		if err := c.RunRuntimeUpdatePass(ctx); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		time.Sleep(jobHardTimeout)
		synctest.Wait()
		waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusFailed)
		p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
		if p.Outcome.Status != "failed" || p.Paused || p.ConsecutiveFailures != 1 {
			t.Fatalf("deadline classified as interruption: %+v", p)
		}
	})
}

func TestOMPAutomaticStaleThirdFailurePausesBeforeAdmission(t *testing.T) {
	c, _, _ := newOMPAutomaticController(t, &autoMemorySettings{})
	ctx := context.Background()
	p := managedruntime.AutoUpdatePolicy{RuntimeID: "omp-acp", Enabled: true, ConsecutiveFailures: 2, LastAttemptStartedAt: time.Date(2026, 10, 10, 1, 0, 0, 0, time.UTC), Outcome: &managedruntime.UpdateOutcome{ID: "lost", Status: "running", UpdateMode: "self_update"}}
	if err := c.runtimeAutoUpdateStore.Save(ctx, "omp-acp", p); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	p, _ = c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if !p.Paused || p.ConsecutiveFailures != 3 || len(c.ListAgentUpdateJobs()) != 0 {
		t.Fatalf("stale third attempt admitted a fourth check: %+v", p)
	}
}

type ompTerminalFailSettings struct {
	autoMemorySettings
	reject atomic.Bool
}

func (s *ompTerminalFailSettings) Save(ctx context.Context, key string, value []byte) error {
	if s.reject.Load() {
		return errors.New("terminal persistence unavailable")
	}
	return s.autoMemorySettings.Save(ctx, key, value)
}

func TestOMPAutomaticFailedTerminalSaveRemainsInterruptedEvidence(t *testing.T) {
	c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	settings := &ompTerminalFailSettings{}
	c.SetRuntimeAutoUpdateStore(managedruntime.NewAutoUpdateStore(settings))
	ctx := context.Background()
	p := managedruntime.AutoUpdatePolicy{RuntimeID: "omp-acp", Enabled: true, ConsecutiveFailures: 2}
	if err := c.runtimeAutoUpdateStore.Save(ctx, "omp-acp", p); err != nil {
		t.Fatal(err)
	}
	u.probe = func(context.Context) (hostutility.AgentCapabilities, error) {
		settings.reject.Store(true)
		return hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.1.0"}, nil
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	id := c.ListAgentUpdateJobs()[0].JobID
	waitForUpdateStatus(t, hub.completed, id, dto.AgentUpdateJobStatusSucceeded)
	c.updateJobStore.automaticWorkers.Wait()
	p, _ = c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if p.Outcome == nil || p.Outcome.ID != id || p.Outcome.Status != managedruntime.UpdateOutcomeRunning || p.ConsecutiveFailures != 2 || p.LastChange != nil {
		t.Fatalf("failed terminal write destroyed marker: %+v", p)
	}
	settings.reject.Store(false)
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	p, _ = c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if p.Outcome.Status != managedruntime.UpdateOutcomeInterrupted || !p.Paused || p.ConsecutiveFailures != 3 || len(c.ListAgentUpdateJobs()) != 1 {
		t.Fatalf("failed-save restart admitted fourth attempt: %+v", p)
	}
}

func TestOMPAutomaticDeadlineAtUpdaterEdgeIsNotInterrupted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
		causes := make(chan automaticHarnessCause, 1)
		c.updateJobStore.onAutomaticFinished = func(job dto.AgentUpdateJobDTO, result automaticHarnessResult) {
			causes <- result.Cause
			c.retainAutomaticHarnessOutcome(job, result)
		}
		u.run = func(ctx context.Context, _ agents.Command, _ func(string)) error { <-ctx.Done(); return nil }
		ctx := context.Background()
		if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
			t.Fatal(err)
		}
		if err := c.RunRuntimeUpdatePass(ctx); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		time.Sleep(jobHardTimeout)
		synctest.Wait()
		waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusFailed)
		if cause := <-causes; cause != automaticUpdaterDeadline {
			t.Fatalf("deadline-edge cause=%s", cause)
		}
		p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
		if p.Outcome.Status != managedruntime.UpdateOutcomeFailed || p.ConsecutiveFailures != 1 || p.Paused {
			t.Fatalf("deadline edge=%+v", p)
		}
	})
}

func TestOMPAutomaticSlowUpdaterStillLeavesProbeItsFullCap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
		u.run = func(context.Context, agents.Command, func(string)) error {
			time.Sleep(jobHardTimeout - time.Second)
			return nil
		}
		u.probe = func(ctx context.Context) (hostutility.AgentCapabilities, error) {
			time.Sleep(time.Minute)
			return hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.1.0"}, ctx.Err()
		}
		ctx := context.Background()
		if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
			t.Fatal(err)
		}
		if err := c.RunRuntimeUpdatePass(ctx); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		time.Sleep(jobHardTimeout + time.Minute)
		synctest.Wait()
		waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusSucceeded)
		p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
		if p.Outcome.ObservedVersion != "1.1.0" || p.Paused {
			t.Fatalf("probe inherited updater deadline: %+v", p)
		}
	})
}

func TestOMPAutomaticCancelledAttemptPersistsInterruptedThroughFinishPath(t *testing.T) {
	c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	running := make(chan struct{})
	u.run = func(ctx context.Context, _ agents.Command, _ func(string)) error {
		close(running)
		<-ctx.Done()
		return errors.New("updater failed")
	}
	generic := make(chan struct{}, 1)
	c.updateJobStore.onFinished = func(dto.AgentUpdateJobDTO) { generic <- struct{}{} }
	causes := make(chan automaticHarnessCause, 1)
	c.updateJobStore.onAutomaticFinished = func(job dto.AgentUpdateJobDTO, result automaticHarnessResult) {
		causes <- result.Cause
		c.retainAutomaticHarnessOutcome(job, result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	<-running
	cancel()
	waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusFailed)
	c.updateJobStore.automaticWorkers.Wait()
	if cause := <-causes; cause != automaticCancelled {
		t.Fatalf("shutdown cause=%s", cause)
	}
	select {
	case <-generic:
		t.Fatal("generic finish hook overwrote typed outcome")
	default:
	}
	p, _ := c.runtimeAutoUpdateStore.Get(context.Background(), "omp-acp", "omp-acp")
	if p.Outcome.Status != managedruntime.UpdateOutcomeInterrupted || p.ConsecutiveFailures != 1 || p.Paused {
		t.Fatalf("finish path=%+v", p)
	}
	if err := c.SetAgentAutomaticUpdates(context.Background(), "omp-acp", false); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(context.Background()); err != nil {
		t.Fatal(err)
	}
	p, _ = c.runtimeAutoUpdateStore.Get(context.Background(), "omp-acp", "omp-acp")
	if p.ConsecutiveFailures != 1 {
		t.Fatalf("completed interruption counted again: %+v", p)
	}
}
