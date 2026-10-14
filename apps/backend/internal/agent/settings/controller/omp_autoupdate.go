package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (c *Controller) enqueueAutomaticHarness(ctx context.Context, status dto.AgentUpdateStatusDTO, started time.Time) error {
	ag, found := c.agentRegistry.Get(status.AgentName)
	if !found || !status.Available || !status.Enabled {
		return nil
	}
	available, err := c.agentAvailable(ctx, ag)
	if err != nil || !available {
		return err
	}
	s := c.updateJobStore
	if s == nil {
		return nil
	}
	select {
	case s.semaphore <- struct{}{}:
	default:
		return nil
	}
	admitted := false
	defer func() {
		if !admitted {
			<-s.semaphore
		}
	}()
	admission, err := c.prepareAutomaticHarness(ctx, status, started)
	if err != nil || admission.job == nil {
		return err
	}
	admitted = true
	s.automaticWorkers.Add(1)
	s.broadcast(ws.ActionAgentUpdateStarted, admission.job.snapshot())
	go func() {
		defer s.automaticWorkers.Done()
		defer func() { <-s.semaphore }()
		s.runAutomaticHarness(admission.job, admission.recipe, admission.probe, admission.ref)
	}()
	return nil
}

type automaticHarnessAdmission struct {
	job    *AgentUpdateJob
	recipe agents.HarnessUpdateSpec
	probe  agents.Command
	ref    MaintenanceJobRef
}

func (c *Controller) prepareAutomaticHarness(ctx context.Context, status dto.AgentUpdateStatusDTO, started time.Time) (automaticHarnessAdmission, error) {
	c.runtimeAutoUpdateMu.Lock()
	defer c.runtimeAutoUpdateMu.Unlock()
	cap, err := c.automaticRuntimeCapability(status.AgentName, status.RuntimeID)
	if err != nil || cap.AutomaticHarness == nil {
		return automaticHarnessAdmission{}, err
	}
	ag, _ := c.agentRegistry.Get(status.AgentName)
	policy, err := c.runtimeAutoUpdateStore.Get(ctx, ag.ID(), cap.RuntimeID)
	if err != nil || !policy.Enabled || ctx.Err() != nil {
		return automaticHarnessAdmission{}, err
	}
	if policy.Paused || !automaticHarnessDue(started, policy.LastAttemptStartedAt) {
		return automaticHarnessAdmission{}, nil
	}
	job := &AgentUpdateJob{
		ID: uuid.NewString(), AgentName: ag.ID(), Package: cap.AutomaticHarness.Package,
		RuntimeID: cap.RuntimeID, Automatic: true, UpdateMode: dto.AgentUpdateModeSelfUpdate,
		Status: dto.AgentUpdateJobStatusQueued, Output: newRingBuffer(jobOutputRingSize),
		StartedAt: started, PreviousVersion: status.CurrentVersion, context: ctx,
	}
	ref, err := c.updateJobStore.admitAutomaticHarness(job, func() error {
		policy.LastAttemptStartedAt = started
		policy.Outcome = &managedruntime.UpdateOutcome{ID: job.ID, Status: managedruntime.UpdateOutcomeRunning, UpdateMode: string(job.UpdateMode), PreviousVersion: job.PreviousVersion}
		return c.runtimeAutoUpdateStore.Save(ctx, ag.ID(), policy)
	})
	if err != nil || ref.JobID == "" {
		return automaticHarnessAdmission{}, err
	}
	return automaticHarnessAdmission{job: job, recipe: *cap.AutomaticHarness, probe: ag.Runtime().Cmd, ref: ref}, nil
}

func (s *AgentUpdateJobStore) admitAutomaticHarness(job *AgentUpdateJob, beforeStart func() error) (MaintenanceJobRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeByAgt[job.AgentName] != nil {
		return MaintenanceJobRef{}, nil
	}
	// Do not expose the claim until its durable marker and job are registered.
	s.maintenance.mu.Lock()
	defer s.maintenance.mu.Unlock()
	ref, claimed, err := s.maintenance.claimLocked(job.AgentName, MaintenanceKindUpdate, job.ID)
	if err != nil || !claimed {
		return MaintenanceJobRef{}, err
	}
	if err := beforeStart(); err != nil {
		delete(s.maintenance.byAgent, job.AgentName)
		return MaintenanceJobRef{}, err
	}
	s.jobs[job.ID], s.activeByAgt[job.AgentName] = job, job
	return ref, nil
}

func (s *AgentUpdateJobStore) runAutomaticHarness(job *AgentUpdateJob, spec agents.HarnessUpdateSpec, probe agents.Command, ref MaintenanceJobRef) {
	ctx, cancel := context.WithTimeout(job.context, jobHardTimeout)
	defer cancel()
	s.setStatus(job, dto.AgentUpdateJobStatusUpdating)
	flusher := newUpdateOutputFlusher(s, job)
	err := s.updater.(RuntimeTreeUpdater).RunUpdateTree(ctx, spec.UpdateCommand, flusher.append)
	flusher.flush()
	result := automaticHarnessResult{PreviousVersion: job.PreviousVersion, OutputExcerpt: updaterOutputExcerpt(job.Output.String())}
	if errors.Is(job.context.Err(), context.Canceled) {
		s.failAutomaticHarness(job, ctx, ref, result, automaticCancelled, context.Canceled)
		return
	}
	if err != nil || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		cause := automaticUpdaterError
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			cause, err = automaticUpdaterDeadline, context.DeadlineExceeded
		}
		s.failAutomaticHarness(job, ctx, ref, result, cause, err)
		return
	}
	s.setStatus(job, dto.AgentUpdateJobStatusRefreshing)
	candidate := s.updater.(RuntimeCandidateUpdater)
	caps, err := candidate.Probe(job.context, job.AgentName, probe)
	if errors.Is(job.context.Err(), context.Canceled) {
		s.failAutomaticHarness(job, job.context, ref, result, automaticCancelled, context.Canceled)
		return
	}
	if err != nil || caps.Status != hostutility.StatusOK || harnessUpdateOperation(caps.AgentVersion) == string(managedruntime.OperationRepair) {
		if err == nil {
			err = fmt.Errorf("validate updated harness: %s", capabilityRefreshError(caps))
		}
		s.failAutomaticHarness(job, job.context, ref, result, automaticValidationFailed, err)
		return
	}
	result.ObservedVersion, result.Cause = caps.AgentVersion, automaticChanged
	operation := managedruntime.OperationUpdate
	if harnessUpdateOperation(job.PreviousVersion) == string(managedruntime.OperationRepair) {
		result.Cause, operation = automaticRepaired, managedruntime.OperationRepair
	} else {
		previous, _ := semver.NewVersion(job.PreviousVersion)
		observed, _ := semver.NewVersion(caps.AgentVersion)
		if previous.Equal(observed) {
			result.Cause, operation = automaticNoChange, managedruntime.OperationUpToDate
		}
	}
	if result.Cause != automaticNoChange {
		candidate.PublishCapabilities(job.AgentName, caps)
	}
	s.mu.Lock()
	job.automaticResult = result
	job.Operation, job.EffectiveVersion = operation, caps.AgentVersion
	s.mu.Unlock()
	s.finishRefresh(job, job.context, caps, nil, ref)
}

func (s *AgentUpdateJobStore) failAutomaticHarness(job *AgentUpdateJob, ctx context.Context, ref MaintenanceJobRef, result automaticHarnessResult, cause automaticHarnessCause, err error) {
	result.Cause = cause
	s.mu.Lock()
	job.automaticResult = result
	s.mu.Unlock()
	s.finishFailed(job, ctx, err, ref)
}

func automaticHarnessDue(started, previous time.Time) bool {
	elapsed := started.Sub(previous)
	return previous.IsZero() || elapsed < 0 || elapsed >= 14*time.Minute
}
