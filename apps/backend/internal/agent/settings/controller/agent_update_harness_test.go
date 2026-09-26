package controller

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// AC-AGENTS-RUNTIME-UPDATES-003.1, AC-AGENTS-RUNTIME-UPDATES-003.7.
func TestHarnessUpdateStatusesCompareACPVersionAndIsolateFailures(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP(), "claude-acp": agents.NewClaudeACP()})
	updater := &fakeRuntimeUpdater{current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"}, currentFound: true}
	ctrl.SetRuntimeUpdater(updater)
	ctrl.SetRuntimeUpdateStatusResolver(func(_ context.Context, pkg string) (string, error) {
		if pkg == "@oh-my-pi/pi-coding-agent" {
			return "1.1.0", nil
		}
		return "0.100.0", nil
	})
	result, err := ctrl.ListAgentUpdateStatuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Statuses) != 2 {
		t.Fatalf("status count = %d, want two", len(result.Statuses))
	}
	for _, status := range result.Statuses {
		if status.AgentName != "omp-acp" {
			continue
		}
		if status.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
			t.Errorf("OMP check = %s", status.CheckState)
		}
		payload, _ := json.Marshal(status)
		var wire map[string]any
		_ = json.Unmarshal(payload, &wire)
		if wire["update_mode"] != "self_update" {
			t.Errorf("OMP mode = %v", wire["update_mode"])
		}
		if wire["effective_version"] != "1.0.0" {
			t.Errorf("OMP effective = %v", wire["effective_version"])
		}
		return
	}
	t.Fatal("OMP missing from status response")
}

func TestHarnessUpdateStatusRegistryFailureKeepsOtherAgent(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP(), "claude-acp": agents.NewClaudeACP()})
	ctrl.SetRuntimeUpdater(&fakeRuntimeUpdater{current: hostutility.AgentCapabilities{AgentVersion: "1.0.0"}, currentFound: true})
	ctrl.SetRuntimeUpdateStatusResolver(func(_ context.Context, pkg string) (string, error) {
		if pkg == "@oh-my-pi/pi-coding-agent" {
			return "", errors.New("registry unavailable")
		}
		return "0.100.0", nil
	})
	result, err := ctrl.ListAgentUpdateStatuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Statuses) != 2 {
		t.Fatalf("statuses = %d", len(result.Statuses))
	}
	for _, status := range result.Statuses {
		if status.AgentName == "omp-acp" && status.CheckState != dto.AgentUpdateCheckStateUnknown {
			t.Errorf("OMP = %s", status.CheckState)
		}
		if status.AgentName == "claude-acp" && status.CheckState == dto.AgentUpdateCheckStateUnknown {
			t.Errorf("Claude status changed to unknown")
		}
	}
}

type harnessTestUpdater struct {
	fakeRuntimeUpdater
	latest    string
	latestErr error
	published hostutility.AgentCapabilities
	probeErr  error
}

func (u *harnessTestUpdater) ResolveHarnessLatest(context.Context, string) (string, error) {
	return u.latest, u.latestErr
}

func (u *harnessTestUpdater) Probe(_ context.Context, _ string, command agents.Command) (hostutility.AgentCapabilities, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.refreshCalls++
	u.refreshCommand = append([]string(nil), command.Args()...)
	return u.refreshCaps, u.probeErr
}

func (u *harnessTestUpdater) CurrentCapabilities(string) (hostutility.AgentCapabilities, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.current, u.currentFound
}

func (u *harnessTestUpdater) PublishCapabilities(_ string, caps hostutility.AgentCapabilities) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.published = caps
	u.current = caps
}

// AC-AGENTS-RUNTIME-UPDATES-003.2, .11, .13.
func TestHarnessUpdatePreviewReferenceAndRepair(t *testing.T) {
	for _, tc := range []struct{ current, operation string }{{"1.0.0", "update"}, {"", "repair"}, {"1.1.0", "up_to_date"}} {
		t.Run(tc.operation, func(t *testing.T) {
			updater := &harnessTestUpdater{latest: "1.1.0"}
			updater.current = hostutility.AgentCapabilities{AgentVersion: tc.current}
			updater.currentFound = true
			ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP()})
			ctrl.SetRuntimeUpdater(updater)
			preview, err := ctrl.PreviewAgentUpdate(context.Background(), "omp-acp")
			if err != nil {
				t.Fatal(err)
			}
			if preview.UpdateMode != dto.AgentUpdateModeSelfUpdate || preview.StableLatestVersion != "1.1.0" || preview.TargetVersion != "" || preview.Operation != tc.operation || len(preview.AvailableVersions) != 0 {
				t.Errorf("self-update preview = %+v", preview)
			}
			if !reflect.DeepEqual(preview.Command, []string{"omp", "update"}) {
				t.Errorf("command = %v", preview.Command)
			}
			if jobs := ctrl.ListAgentUpdateJobs(); len(jobs) != 0 {
				t.Errorf("preview created jobs: %v", jobs)
			}
		})
	}
}

func TestHarnessUpdatePreviewRegistryFailureNoJob(t *testing.T) {
	updater := &harnessTestUpdater{latestErr: errors.New("registry unavailable")}
	ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP()})
	ctrl.SetRuntimeUpdater(updater)
	_, err := ctrl.PreviewAgentUpdate(context.Background(), "omp-acp")
	if !errors.Is(err, ErrRuntimeUpdatePreviewFailed) {
		t.Errorf("preview error = %v", err)
	}
	if jobs := ctrl.ListAgentUpdateJobs(); len(jobs) != 0 {
		t.Errorf("preview created jobs: %v", jobs)
	}
}

// AC-AGENTS-RUNTIME-UPDATES-003.11: status uses harness metadata, not npm.
func TestHarnessUpdateStatusUsesDirectResolver(t *testing.T) {
	updater := &harnessTestUpdater{latest: "1.1.0"}
	updater.current = hostutility.AgentCapabilities{AgentVersion: "1.0.0"}
	updater.currentFound = true
	ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP()})
	ctrl.SetRuntimeUpdater(updater)
	result, err := ctrl.ListAgentUpdateStatuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Statuses) != 1 || result.Statuses[0].CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
		t.Errorf("OMP status = %+v", result.Statuses)
	}
}

// AC-AGENTS-RUNTIME-UPDATES-003.6: preflight returns a terminal result without a job.
func TestHarnessUpdateApprovalUpToDateHasNoJob(t *testing.T) {
	updater := &harnessTestUpdater{latest: "1.1.0"}
	updater.current = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.1.0"}
	updater.currentFound = true
	ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP()})
	ctrl.SetRuntimeUpdater(updater)
	ctrl.SetJobBroadcaster(newUpdateTerminalBroadcaster())
	job, err := ctrl.EnqueueAgentUpdate(context.Background(), "omp-acp", "")
	if err != nil {
		t.Fatal(err)
	}
	if job.JobID != "" || job.Status != dto.AgentUpdateJobStatusSucceeded || job.Operation != "up_to_date" || job.CurrentVersion != "1.1.0" || job.UpdateMode != dto.AgentUpdateModeSelfUpdate {
		t.Errorf("no-job result = %+v", job)
	}
	if len(ctrl.ListAgentUpdateJobs()) != 0 {
		t.Fatal("preflight created a retained job")
	}
	updater.mu.Lock()
	defer updater.mu.Unlock()
	if updater.runCalls != 0 || updater.refreshCalls != 0 {
		t.Errorf("preflight invoked update/probe: %d/%d", updater.runCalls, updater.refreshCalls)
	}
}

// AC-AGENTS-RUNTIME-UPDATES-003.13: mode comes from trusted capability, not request.
func TestHarnessUpdateApprovalRejectsVersionAndDefault(t *testing.T) {
	updater := &harnessTestUpdater{latest: "1.1.0"}
	ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP()})
	ctrl.SetRuntimeUpdater(updater)
	ctrl.SetJobBroadcaster(newUpdateTerminalBroadcaster())
	for _, attempt := range []func() (*dto.AgentUpdateJobDTO, error){
		func() (*dto.AgentUpdateJobDTO, error) {
			return ctrl.EnqueueAgentUpdate(context.Background(), "omp-acp", "1.2.3")
		},
		func() (*dto.AgentUpdateJobDTO, error) {
			return ctrl.EnqueueAgentUpdateUseDefault(context.Background(), "omp-acp")
		},
	} {
		if _, err := attempt(); !errors.Is(err, ErrRuntimeUpdateTargetInvalid) {
			t.Errorf("version/default rejection = %v", err)
		}
	}
	if len(ctrl.ListAgentUpdateJobs()) != 0 {
		t.Fatal("rejected approval created a job")
	}
}

// AC-AGENTS-RUNTIME-UPDATES-003.3, .4, .14.
func TestHarnessUpdateJobRunsTrustedCommandAndPublishesProbedChannelVersion(t *testing.T) {
	updater := &harnessTestUpdater{latest: "1.1.0"}
	updater.current = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"}
	updater.currentFound = true
	updater.refreshCaps = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.2.0"}
	updater.updateOutput = "updated to channel version 1.2.0\n"
	ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP()})
	ctrl.SetRuntimeUpdater(updater)
	hub := newUpdateTerminalBroadcaster()
	ctrl.SetJobBroadcaster(hub)
	ctrl.updateJobStore.onRefresh = nil
	job, err := ctrl.EnqueueAgentUpdate(context.Background(), "omp-acp", "")
	if err != nil {
		t.Fatal(err)
	}
	if job.JobID == "" {
		t.Fatal("outdated approval returned no job")
	}
	result := waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
	if result.UpdateMode != dto.AgentUpdateModeSelfUpdate || result.CurrentVersion != "1.2.0" || result.TargetVersion != "" || result.ActiveVersion != "" || result.DefaultVersion != "" {
		t.Errorf("completed job = %+v", result)
	}
	if result.Output != updater.updateOutput {
		t.Errorf("job output = %q", result.Output)
	}
	updater.mu.Lock()
	defer updater.mu.Unlock()
	if !reflect.DeepEqual(updater.runCommand, []string{"omp", "update"}) || updater.runCalls != 1 || !reflect.DeepEqual(updater.refreshCommand, []string{"omp", "acp"}) || updater.published.AgentVersion != "1.2.0" || updater.invalidateCalls != 0 {
		t.Errorf("update/probe/publication = command %v calls %d probe %v published %+v cache %d", updater.runCommand, updater.runCalls, updater.refreshCommand, updater.published, updater.invalidateCalls)
	}
}

// AC-AGENTS-RUNTIME-UPDATES-003.5, .8.
func TestHarnessUpdateJobFailuresDoNotPublishOrFallback(t *testing.T) {
	for _, tc := range []struct {
		name                string
		updateErr, probeErr error
	}{
		{"updater", errors.New("nix-managed installation"), nil},
		{"probe", nil, errors.New("ACP probe failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			updater := &harnessTestUpdater{latest: "1.1.0", probeErr: tc.probeErr}
			updater.current = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"}
			updater.currentFound = true
			updater.runErr = tc.updateErr
			updater.updateOutput = "updater own output\n"
			updater.refreshCaps = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.2.0"}
			ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP()})
			ctrl.SetRuntimeUpdater(updater)
			hub := newUpdateTerminalBroadcaster()
			ctrl.SetJobBroadcaster(hub)
			ctrl.updateJobStore.onRefresh = nil
			job, err := ctrl.EnqueueAgentUpdate(context.Background(), "omp-acp", "")
			if err != nil {
				t.Fatal(err)
			}
			result := waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusFailed)
			if result.Output != updater.updateOutput || result.CurrentVersion != "1.0.0" || result.Error == "" {
				t.Errorf("failure result = %+v", result)
			}
			updater.mu.Lock()
			defer updater.mu.Unlock()
			if updater.runCalls != 1 || updater.invalidateCalls != 0 || updater.published.AgentVersion != "" || !reflect.DeepEqual(updater.runCommand, []string{"omp", "update"}) {
				t.Errorf("failure mutated beyond trusted updater: %d/%d published %+v argv %v", updater.runCalls, updater.invalidateCalls, updater.published, updater.runCommand)
			}
			if tc.updateErr != nil && updater.refreshCalls != 0 {
				t.Error("probe ran after failed updater")
			}
		})
	}
}

// AC-AGENTS-RUNTIME-UPDATES-003.6: worker observation keeps the accepted job.
func TestHarnessUpdateWorkerUpToDateRetainsJobWithoutCommands(t *testing.T) {
	updater := &harnessTestUpdater{latest: "1.1.0"}
	updater.current = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"}
	updater.currentFound = true
	ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP()})
	ctrl.SetRuntimeUpdater(updater)
	hub := newUpdateTerminalBroadcaster()
	ctrl.SetJobBroadcaster(hub)
	for range cap(ctrl.updateJobStore.semaphore) {
		ctrl.updateJobStore.semaphore <- struct{}{}
	}
	job, err := ctrl.EnqueueAgentUpdate(context.Background(), "omp-acp", "")
	if err != nil {
		t.Fatal(err)
	}
	if job.JobID == "" {
		t.Fatal("preflight did not enqueue")
	}
	updater.mu.Lock()
	updater.current.AgentVersion = "1.1.0"
	updater.mu.Unlock()
	<-ctrl.updateJobStore.semaphore
	result := waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
	if result.Operation != "up_to_date" || result.CurrentVersion != "1.1.0" || len(ctrl.ListAgentUpdateJobs()) != 1 {
		t.Errorf("retained no-op job = %+v", result)
	}
	updater.mu.Lock()
	defer updater.mu.Unlock()
	if updater.runCalls != 0 || updater.refreshCalls != 0 {
		t.Errorf("worker ran updater/probe: %d/%d", updater.runCalls, updater.refreshCalls)
	}
}

func TestHarnessUpdateApprovalRegistryFailureDoesNotEnqueue(t *testing.T) {
	updater := &harnessTestUpdater{latestErr: errors.New("registry unavailable")}
	ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP()})
	ctrl.SetRuntimeUpdater(updater)
	ctrl.SetJobBroadcaster(newUpdateTerminalBroadcaster())
	_, err := ctrl.EnqueueAgentUpdate(context.Background(), "omp-acp", "")
	if !errors.Is(err, ErrRuntimeUpdatePreviewFailed) {
		t.Errorf("approval failure = %v", err)
	}
	if len(ctrl.ListAgentUpdateJobs()) != 0 {
		t.Fatal("registry failure created a job")
	}
}

// AC-AGENTS-RUNTIME-UPDATES-003.14: a faster configured channel is not downgraded.
func TestHarnessUpdatePreviewTreatsNewerCanaryAsUpToDate(t *testing.T) {
	updater := &harnessTestUpdater{latest: "1.1.0"}
	updater.current = hostutility.AgentCapabilities{AgentVersion: "1.2.0-canary.1"}
	updater.currentFound = true
	ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP()})
	ctrl.SetRuntimeUpdater(updater)
	preview, err := ctrl.PreviewAgentUpdate(context.Background(), "omp-acp")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Operation != "up_to_date" {
		t.Errorf("configured channel operation = %q", preview.Operation)
	}
}

func TestHarnessUpdateStatusTreatsNewerCanaryAsUpToDate(t *testing.T) {
	updater := &harnessTestUpdater{latest: "1.1.0"}
	updater.current = hostutility.AgentCapabilities{AgentVersion: "1.2.0-canary.1"}
	updater.currentFound = true
	ctrl := newTestController(map[string]agents.Agent{"omp-acp": agents.NewOmpACP()})
	ctrl.SetRuntimeUpdater(updater)
	result, err := ctrl.ListAgentUpdateStatuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Statuses) != 1 || result.Statuses[0].CheckState != dto.AgentUpdateCheckStateUpToDate {
		t.Errorf("configured-channel status = %+v", result.Statuses)
	}
}
