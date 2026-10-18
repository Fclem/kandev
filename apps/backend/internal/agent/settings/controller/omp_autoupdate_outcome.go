package controller

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

type automaticHarnessCause string

const (
	automaticChanged          automaticHarnessCause = "changed"
	automaticRepaired         automaticHarnessCause = "repaired"
	automaticNoChange         automaticHarnessCause = "no_change"
	automaticUpdaterError     automaticHarnessCause = "updater_error"
	automaticUpdaterDeadline  automaticHarnessCause = "updater_deadline"
	automaticValidationFailed automaticHarnessCause = "validation_failed"
	automaticCancelled        automaticHarnessCause = "cancelled"
)

type automaticHarnessResult struct {
	Cause           automaticHarnessCause
	PreviousVersion string
	ObservedVersion string
	OutputExcerpt   string
}

var updaterEscapePattern = regexp.MustCompile(`(?:\x1b[\]P^_X][^\x07\x1b]*(?:\x07|\x1b\\|$))|(?:\x1b\[[0-?]*[ -/]*[@-~])|(?:\x{9b}[0-?]*[ -/]*[@-~])|(?:\x1b[ -/]*[0-~])`)

func updaterOutputExcerpt(output string) string {
	output = strings.ReplaceAll(strings.ReplaceAll(output, "\r\n", "\n"), "\r", "\n")
	output = updaterEscapePattern.ReplaceAllString(output, "")
	output = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 32 || (r >= 127 && r <= 159) {
			return -1
		}
		return r
	}, output)
	if len(output) > 2048 {
		start := len(output) - 2048
		for start < len(output) && !utf8.RuneStart(output[start]) {
			start++
		}
		output = output[start:]
	}
	return output
}

func (c *Controller) retainAutomaticHarnessOutcome(job dto.AgentUpdateJobDTO, result automaticHarnessResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c.runtimeAutoUpdateMu.Lock()
	policy, err := c.runtimeAutoUpdateStore.Get(ctx, job.AgentName, job.RuntimeID)
	if err != nil || policy.Outcome == nil || policy.Outcome.ID != job.JobID {
		c.runtimeAutoUpdateMu.Unlock()
		return
	}
	outcome := &managedruntime.UpdateOutcome{
		ID: job.JobID, Status: managedruntime.UpdateOutcomeSucceeded, UpdateMode: string(dto.AgentUpdateModeSelfUpdate),
		PreviousVersion: result.PreviousVersion, ObservedVersion: result.ObservedVersion, OutputExcerpt: result.OutputExcerpt, FinishedAt: time.Now().UTC(),
	}
	switch result.Cause {
	case automaticChanged:
		outcome.Operation = managedruntime.OperationUpdate
		policy.LastChange = &managedruntime.UpdateChange{ID: job.JobID, PreviousVersion: result.PreviousVersion, ObservedVersion: result.ObservedVersion, FinishedAt: outcome.FinishedAt}
		policy.ConsecutiveFailures = 0
	case automaticRepaired:
		outcome.Operation = managedruntime.OperationRepair
		policy.ConsecutiveFailures = 0
	case automaticNoChange:
		outcome.Operation = managedruntime.OperationUpToDate
		policy.ConsecutiveFailures = 0
	default:
		outcome.Status = managedruntime.UpdateOutcomeFailed
		if result.Cause == automaticCancelled {
			outcome.Status = managedruntime.UpdateOutcomeInterrupted
		}
		policy.ConsecutiveFailures++
		policy.Paused = policy.Paused || result.Cause == automaticValidationFailed || policy.ConsecutiveFailures >= 3
	}
	policy.Outcome = outcome
	err = c.runtimeAutoUpdateStore.Save(ctx, job.AgentName, policy)
	c.runtimeAutoUpdateMu.Unlock()
	if err != nil {
		c.logger.Warn("automatic harness outcome could not be persisted")
		return
	}
	ag, found := c.agentRegistry.Get(job.AgentName)
	if !found {
		return
	}
	if c.runtimeUpdateNotifier == nil {
		return
	}
	// Publish notices only: a partial status must not feed the availability observer, which would drop a pending stable-availability notice.
	notice := agents.RuntimeUpdateNotice{AgentID: job.AgentName, RuntimeID: job.RuntimeID, DisplayName: ag.DisplayName(), UpdateMode: string(dto.AgentUpdateModeSelfUpdate), URL: "/settings/agents#runtime-update-" + url.PathEscape(job.AgentName)}
	c.publishAutomaticHarnessNotices(ctx, dto.AgentUpdateStatusDTO{AgentName: job.AgentName, RuntimeID: job.RuntimeID, UpdateMode: dto.AgentUpdateModeSelfUpdate, LastOutcome: outcome, AutoUpdate: policy.Enabled, AutoUpdatePaused: policy.Enabled && policy.Paused, LastChange: policy.LastChange}, notice)
}
