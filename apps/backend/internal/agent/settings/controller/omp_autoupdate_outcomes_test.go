package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-004.4, AC-AGENTS-RUNTIME-NOTIFY-004.5, AC-AGENTS-RUNTIME-NOTIFY-004.7
func TestOMPAutomaticTypedCauseReachesHook(t *testing.T) {
	cases := []struct {
		name, previous, observed  string
		status                    hostutility.Status
		runErr                    error
		operation                 managedruntime.Operation
		cause                     automaticHarnessCause
		failed, paused, published bool
	}{
		{name: "TestOMPAutomaticChangedVersionPublishesObservedCapabilities", previous: "1.0.0", observed: "1.1.0", status: hostutility.StatusOK, operation: managedruntime.OperationUpdate, cause: automaticChanged, published: true},
		{name: "TestOMPAutomaticUnknownPreviousVersionRecordsRepair", observed: "1.1.0", status: hostutility.StatusOK, operation: managedruntime.OperationRepair, cause: automaticRepaired, published: true},
		{name: "TestOMPAutomaticNoChangeRetainsOutputExcerpt", previous: "1.0.0", observed: "1.0.0", status: hostutility.StatusOK, operation: managedruntime.OperationUpToDate, cause: automaticNoChange},
		{name: "updater_error", previous: "1.0.0", observed: "1.1.0", status: hostutility.StatusOK, runErr: errors.New("validation failed: context canceled"), cause: automaticUpdaterError, failed: true},
		{name: "TestOMPAutomaticMissingVersionFails", previous: "1.0.0", status: hostutility.StatusOK, cause: automaticValidationFailed, failed: true, paused: true},
		{name: "invalid_version", previous: "1.0.0", observed: "unknown", status: hostutility.StatusOK, cause: automaticValidationFailed, failed: true, paused: true},
		{name: "TestOMPAutomaticValidationFailurePauses", previous: "1.0.0", observed: "1.1.0", status: hostutility.StatusFailed, cause: automaticValidationFailed, failed: true, paused: true},
		{name: "TestOMPAutomaticAuthRequiredProbePauses", previous: "1.0.0", observed: "1.1.0", status: hostutility.StatusAuthRequired, cause: automaticValidationFailed, failed: true, paused: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
			causes := make(chan automaticHarnessCause, 1)
			c.updateJobStore.onAutomaticFinished = func(job dto.AgentUpdateJobDTO, result automaticHarnessResult) {
				causes <- result.Cause
				c.retainAutomaticHarnessOutcome(job, result)
			}
			u.current.AgentVersion = tc.previous
			u.refreshCaps = hostutility.AgentCapabilities{Status: tc.status, AgentVersion: tc.observed}
			u.runErr = tc.runErr
			u.updateOutput = "\x1b[31mdeclined\x1b[0m\r\nchannel\x00\n"
			ctx := context.Background()
			if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
				t.Fatal(err)
			}
			if err := c.RunRuntimeUpdatePass(ctx); err != nil {
				t.Fatal(err)
			}
			expected := dto.AgentUpdateJobStatusSucceeded
			if tc.failed {
				expected = dto.AgentUpdateJobStatusFailed
			}
			job := waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, expected)
			c.updateJobStore.automaticWorkers.Wait()
			if cause := <-causes; cause != tc.cause {
				t.Fatalf("cause=%s, want %s", cause, tc.cause)
			}
			policy, err := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
			if err != nil {
				t.Fatal(err)
			}
			if policy.Paused != tc.paused || policy.Outcome.Operation != tc.operation || policy.Outcome.UpdateMode != "self_update" || policy.Outcome.TargetVersion != "" {
				t.Fatalf("typed outcome policy: %+v outcome=%+v", policy, policy.Outcome)
			}
			if policy.Outcome.OutputExcerpt != "declined\nchannel\n" {
				t.Fatalf("excerpt=%q", policy.Outcome.OutputExcerpt)
			}
			if tc.operation == managedruntime.OperationUpdate && (policy.LastChange == nil || policy.LastChange.ObservedVersion != tc.observed || policy.LastChange.ID != job.JobID) {
				t.Fatalf("change evidence=%+v", policy.LastChange)
			}
			if tc.operation == managedruntime.OperationRepair && policy.LastChange != nil {
				t.Fatal("unknown previous version claimed a change")
			}
			u.mu.Lock()
			defer u.mu.Unlock()
			if (u.publishCalls > 0) != tc.published {
				t.Fatalf("publication count=%d", u.publishCalls)
			}
			if !tc.published && u.current.AgentVersion != tc.previous {
				t.Fatal("failure/no-change replaced catalogue")
			}
		})
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-004.6
func TestOMPAutomaticUpdaterErrorRetriesThenPausesAtThree(t *testing.T) {
	c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	u.runErr = errors.New("offline")
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	c.SetRuntimeUpdateStatusClock(func() time.Time { return now })
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		if err := c.RunRuntimeUpdatePass(ctx); err != nil {
			t.Fatal(err)
		}
		jobs := c.ListAgentUpdateJobs()
		var id string
		for _, job := range jobs {
			if job.StartedAt.Equal(now) {
				id = job.JobID
			}
		}
		if id == "" {
			t.Fatalf("retry %d was not admitted", i)
		}
		waitForUpdateStatus(t, hub.completed, id, dto.AgentUpdateJobStatusFailed)
		c.updateJobStore.automaticWorkers.Wait()
		policy, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
		if policy.ConsecutiveFailures != i || policy.Paused != (i == 3) {
			t.Fatalf("retry %d policy=%+v", i, policy)
		}
		now = now.Add(15 * time.Minute)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	if len(c.ListAgentUpdateJobs()) != 3 {
		t.Fatal("paused updater ran again")
	}
}

func TestOMPAutomaticExcerptKeepsNewlinesStripsEscapesAndCapsAtRuneBoundary(t *testing.T) {
	c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	u.refreshCaps = u.current
	u.updateOutput = strings.Repeat("é", 1500) + "\x1b]0;secret\x07\x1b(Bone\x1b7two\x1b8\x1bPq#0\x1b\\three\u009b31mred\r\nfinal\u0085\tline\x1b[0m"
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusSucceeded)
	p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	excerpt := p.Outcome.OutputExcerpt
	if len(excerpt) > 2048 || !strings.HasSuffix(excerpt, "onetwothreered\nfinal\tline") || strings.Contains(excerpt, "secret") || strings.ContainsRune(excerpt, '�') {
		t.Fatalf("bounded plain-text excerpt=%q (%d bytes)", excerpt, len(excerpt))
	}
}

func TestOMPAutomaticLastChangeSurvivesNoChange(t *testing.T) {
	c, u, hub := newOMPAutomaticController(t, &autoMemorySettings{})
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	c.SetRuntimeUpdateStatusClock(func() time.Time { return now })
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "omp-acp", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	id := c.ListAgentUpdateJobs()[0].JobID
	waitForUpdateStatus(t, hub.completed, id, dto.AgentUpdateJobStatusSucceeded)
	c.updateJobStore.automaticWorkers.Wait()
	now = now.Add(15 * time.Minute)
	u.mu.Lock()
	u.refreshCaps = u.current
	u.mu.Unlock()
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	for _, job := range c.ListAgentUpdateJobs() {
		if job.JobID != id {
			waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
		}
	}
	p, _ := c.runtimeAutoUpdateStore.Get(ctx, "omp-acp", "omp-acp")
	if p.LastChange == nil || p.LastChange.ID != id || p.Outcome.ID == id || p.Outcome.Operation != managedruntime.OperationUpToDate {
		t.Fatalf("no-change erased change history: %+v", p)
	}
}
