package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-004.1, AC-AGENTS-RUNTIME-NOTIFY-004.3
func TestOMPAutomaticChannelChecksIgnoreStableStatus(t *testing.T) {
	for _, latest := range []string{"", "1.0.0", "0.9.0", "2.0.0"} {
		t.Run(latest, func(t *testing.T) {
			c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
			u.latest = latest
			if latest == "" {
				u.latestErr = errors.New("offline")
			}
			ctx := context.Background()
			if err := c.RunRuntimeUpdatePass(ctx); err != nil {
				t.Fatal(err)
			}
			if len(c.ListAgentUpdateJobs()) != 0 {
				t.Fatal("default-off check ran")
			}
			if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
				t.Fatalf("trusted OMP consent rejected: %v", err)
			}
			if err := c.RunRuntimeUpdatePass(ctx); err != nil {
				t.Fatal(err)
			}
			jobs := c.ListAgentUpdateJobs()
			if len(jobs) != 1 {
				t.Fatalf("checks = %d; stable metadata must not gate configured-channel execution", len(jobs))
			}
			job := waitForUpdateStatus(t, hub.completed, jobs[0].JobID, dto.AgentUpdateJobStatusSucceeded)
			u.mu.Lock()
			defer u.mu.Unlock()
			if u.runCalls != 1 || u.publishCalls != 1 || u.published.AgentVersion != "1.1.0" || job.TargetVersion != "" {
				t.Fatalf("command/catalogue result: calls=%d publish=%+v job=%+v", u.runCalls, u.published, job)
			}
		})
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-004.2
func TestOMPAutomaticCadenceUsesStartTimeAndSurvivesRestart(t *testing.T) {
	settings := &autoMemorySettings{}
	c, _, hub := newOMPAutomaticController(t, settings)
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	c.SetRuntimeUpdateStatusClock(func() time.Time { return now })
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	job := c.ListAgentUpdateJobs()[0]
	waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
	c.updateJobStore.automaticWorkers.Wait()
	restarted, _, secondHub := newOMPAutomaticController(t, settings)
	restarted.SetRuntimeUpdateStatusClock(func() time.Time { return now })
	now = now.Add(13*time.Minute + 59*time.Second)
	if err := restarted.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	if len(restarted.ListAgentUpdateJobs()) != 0 {
		t.Fatal("restart admitted check before fourteen-minute spacing")
	}
	now = now.Add(time.Second)
	if err := restarted.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	jobs := restarted.ListAgentUpdateJobs()
	if len(jobs) != 1 {
		t.Fatalf("fourteen-minute boundary admitted %d checks", len(jobs))
	}
	waitForUpdateStatus(t, secondHub.completed, jobs[0].JobID, dto.AgentUpdateJobStatusSucceeded)
}

func TestOMPAutomaticClockStepBackIsEligible(t *testing.T) {
	c, _, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	ctx := context.Background()
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	c.SetRuntimeUpdateStatusClock(func() time.Time { return now })
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusSucceeded)
	c.updateJobStore.automaticWorkers.Wait()
	now = now.Add(-time.Minute)
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	for _, job := range c.ListAgentUpdateJobs() {
		if job.StartedAt.Equal(now) {
			waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
			return
		}
	}
	t.Fatal("clock step back stranded periodic checks")
}

func TestOMPAutomaticBusyPassDoesNotConsumeInterval(t *testing.T) {
	c, _, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	ref, _, err := c.maintenance.claim("omp-acp", MaintenanceKindInstall, "install")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if p.Outcome != nil || !p.LastAttemptStartedAt.IsZero() || len(c.ListAgentUpdateJobs()) != 0 {
		t.Fatalf("busy admission consumed interval: %+v", p)
	}
	c.maintenance.release("omp-acp", ref)
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	jobs := c.ListAgentUpdateJobs()
	if len(jobs) != 1 {
		t.Fatal("busy deferral blocked the next eligible pass")
	}
	waitForUpdateStatus(t, hub.completed, jobs[0].JobID, dto.AgentUpdateJobStatusSucceeded)
}
