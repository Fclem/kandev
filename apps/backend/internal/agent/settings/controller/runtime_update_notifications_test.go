package controller

import (
	"context"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type runtimeNoticeCapture struct {
	mu      sync.Mutex
	notices []agents.RuntimeUpdateNotice
	batches [][]agents.RuntimeUpdateNotice
}

func (n *runtimeNoticeCapture) HandleAgentRuntimeUpdate(_ context.Context, notice agents.RuntimeUpdateNotice) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notices = append(n.notices, notice)
}

func (n *runtimeNoticeCapture) HandleAgentRuntimeUpdates(_ context.Context, notices []agents.RuntimeUpdateNotice) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.batches = append(n.batches, append([]agents.RuntimeUpdateNotice(nil), notices...))
	n.notices = append(n.notices, notices...)
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.3, AC-AGENTS-RUNTIME-NOTIFY-002.6
func TestRuntimeBackgroundPassNamesManualRuntimeAndRetainedOutcome(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ag := agents.NewKimiACP()
		c := newTestController(map[string]agents.Agent{ag.ID(): ag})
		c.SetRuntimeUpdater(&recoveryRuntimeUpdater{currentFound: true, current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"}})
		c.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { return "2.0.0", nil })
		n := &runtimeNoticeCapture{}
		c.SetRuntimeUpdateNotifier(n)
		stop := c.StartRuntimeUpdateBackground(context.Background())
		defer stop()
		synctest.Wait()
		if n.count() != 0 || n.batchCount() != 0 {
			individual, batches := n.snapshot()
			t.Fatalf("availability bypassed its collection window: individual=%+v batches=%+v", individual, batches)
		}

		time.Sleep(runtimeUpdateAvailabilityWindow)
		synctest.Wait()
		n.mu.Lock()
		batches := append([][]agents.RuntimeUpdateNotice(nil), n.batches...)
		n.mu.Unlock()
		if len(batches) != 1 || len(batches[0]) != 1 {
			t.Fatalf("background pass did not deliver one collected runtime summary: %+v", batches)
		}
		if notice := batches[0][0]; notice.AgentID != ag.ID() || notice.PreviousVersion != "1.0.0" || notice.Version != "2.0.0" || notice.URL != "/settings/agents#runtime-update-"+ag.ID() {
			t.Fatalf("manual runtime summary lost its current details: %+v", notice)
		}

		runtimeID := agents.RuntimeUpdateCapabilities(ag).RuntimeID
		c.publishRuntimeStatus(context.Background(), dto.AgentUpdateStatusDTO{
			AgentName: ag.ID(), DisplayName: "Kimi", RuntimeID: runtimeID,
			Available: true, Enabled: true, CheckState: dto.AgentUpdateCheckStateUnknown,
			LastOutcome: &managedruntime.UpdateOutcome{ID: "attempt", Status: "failed", PreviousVersion: "1.0.0", TargetVersion: "2.0.0"},
		})
		individual, _ := n.snapshot()
		if len(individual) != 2 || individual[1].Status != "failed" || individual[1].PreviousVersion != "1.0.0" {
			t.Fatalf("retained failure was not delivered individually: %+v", individual)
		}
	})
}

func (n *runtimeNoticeCapture) count() int { n.mu.Lock(); defer n.mu.Unlock(); return len(n.notices) }

func (n *runtimeNoticeCapture) batchCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.batches)
}

func (n *runtimeNoticeCapture) snapshot() ([]agents.RuntimeUpdateNotice, [][]agents.RuntimeUpdateNotice) {
	n.mu.Lock()
	defer n.mu.Unlock()
	individual := append([]agents.RuntimeUpdateNotice(nil), n.notices...)
	batches := make([][]agents.RuntimeUpdateNotice, len(n.batches))
	for i, batch := range n.batches {
		batches[i] = append([]agents.RuntimeUpdateNotice(nil), batch...)
	}
	return individual, batches
}

func TestOMPAutomaticChangedNoticeReplaysAfterLaterNoChangeCheck(t *testing.T) {
	c, _, _ := newOMPAutomaticController(t, &autoMemorySettings{})
	n := &runtimeNoticeCapture{}
	c.SetRuntimeUpdateNotifier(n)
	status := dto.AgentUpdateStatusDTO{
		AgentName: "omp-acp", RuntimeID: "omp-acp", UpdateMode: dto.AgentUpdateModeSelfUpdate,
		LastChange:  &managedruntime.UpdateChange{ID: "changed", PreviousVersion: "1.0.0", ObservedVersion: "1.1.0"},
		LastOutcome: &managedruntime.UpdateOutcome{ID: "later", Status: "succeeded", UpdateMode: "self_update", Operation: managedruntime.OperationUpToDate},
	}
	c.publishRuntimeStatus(context.Background(), status)
	notices, _ := n.snapshot()
	if len(notices) != 1 || notices[0].Version != "1.1.0" || notices[0].OccurrenceID != runtimeNoticeKey("omp-acp", "omp-acp", "1.1.0", "succeeded", "changed") {
		t.Fatalf("no-change erased changed replay: %+v", notices)
	}
}

func TestOMPAutomaticNoticesSuppressRepairNoChangeAndNonPausingFailure(t *testing.T) {
	c, _, _ := newOMPAutomaticController(t, &autoMemorySettings{})
	n := &runtimeNoticeCapture{}
	c.SetRuntimeUpdateNotifier(n)
	for _, outcome := range []*managedruntime.UpdateOutcome{
		{ID: "repair", Status: "succeeded", UpdateMode: "self_update", Operation: managedruntime.OperationRepair},
		{ID: "same", Status: "succeeded", UpdateMode: "self_update", Operation: managedruntime.OperationUpToDate},
		{ID: "retry", Status: "failed", UpdateMode: "self_update"},
	} {
		c.publishRuntimeStatus(context.Background(), dto.AgentUpdateStatusDTO{AgentName: "omp-acp", RuntimeID: "omp-acp", UpdateMode: dto.AgentUpdateModeSelfUpdate, LastOutcome: outcome})
	}
	if n.count() != 0 {
		t.Fatal("automatic check generated noisy outcome notice")
	}
	c.publishRuntimeStatus(context.Background(), dto.AgentUpdateStatusDTO{AgentName: "omp-acp", RuntimeID: "omp-acp", UpdateMode: dto.AgentUpdateModeSelfUpdate, AutoUpdatePaused: true, LastOutcome: &managedruntime.UpdateOutcome{ID: "pause", Status: "failed", UpdateMode: "self_update"}})
	if n.count() != 1 {
		t.Fatal("versionless pausing failure was dropped")
	}
}
