package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.1
func TestProviderLimitMarkConcreteOptOutFailureStillMarks(t *testing.T) {
	svc, limits, profile := newProviderLimitMarkService(t)
	reset := time.Now().UTC().Add(time.Hour)
	svc.handleAgentFailed(context.Background(), limitFailureEvent(reset))
	waitForFailureRecovery(t, svc)
	subject := limits.ForProfile(profile, agents.NewOmpACP())
	mark, limited := limits.Lookup(subject, "anthropic/sonnet")
	if !limited || mark.Scope != "account" || !mark.ResetKnown || !mark.Until.Equal(reset) {
		t.Fatalf("opted-out failure did not share its account mark: %+v limited=%t", mark, limited)
	}
	if _, limited := limits.Lookup(subject, "openai-codex/gpt-6.1-sol"); limited {
		t.Fatal("OMP account mark escaped its provider domain")
	}
	session, err := svc.repo.GetTaskSession(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("recording changed the existing recovery surface: %s", session.State)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.5
func TestProviderLimitMarkSuccessfulTurnClearsActualModelAndAccount(t *testing.T) {
	svc, limits, profile := newProviderLimitMarkService(t)
	subject := limits.ForProfile(profile, agents.NewOmpACP())
	ctx := context.Background()
	for _, item := range []struct{ model, scope string }{{"anthropic/live-opus", "account"}, {"anthropic/live-opus", "model"}, {profile.Model, "model"}} {
		if _, err := limits.Record(ctx, subject, item.model, &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: item.scope}, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "live-exec"})
	if _, limited := limits.Lookup(subject, "anthropic/live-opus"); limited {
		t.Fatal("successful actual model remained limited")
	}
	if _, limited := limits.Lookup(subject, "anthropic/sonnet"); limited {
		t.Fatal("successful turn did not clear its applicable account scope")
	}
	if mark, limited := limits.Lookup(subject, profile.Model); !limited || mark.Scope != "model" {
		t.Fatalf("success cleared an unrelated model: %+v limited=%t", mark, limited)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
func TestProviderLimitMarkFailedStorageKeepsExistingRecovery(t *testing.T) {
	svc, limits, profile := newProviderLimitMarkService(t)
	writer := svc.repo.(interface{ DB() *sql.DB }).DB()
	if _, err := writer.Exec(`CREATE TRIGGER reject_limit_mark BEFORE INSERT ON dynamic_resource_circuits BEGIN SELECT RAISE(ABORT,'mark refused'); END`); err != nil {
		t.Fatal(err)
	}
	svc.handleAgentFailed(context.Background(), limitFailureEvent(time.Now().UTC().Add(time.Hour)))
	waitForFailureRecovery(t, svc)
	if _, limited := limits.Lookup(limits.ForProfile(profile, agents.NewOmpACP()), "anthropic/live-opus"); limited {
		t.Fatal("failed storage published a limit")
	}
	session, err := svc.repo.GetTaskSession(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("failed mark write removed existing recovery: %s", session.State)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.1
func TestProviderLimitMarkStaleAndDynamicFailuresDoNotMark(t *testing.T) {
	for _, name := range []string{"rotated execution", "dynamic candidate"} {
		t.Run(name, func(t *testing.T) {
			svc, limits, profile := newProviderLimitMarkService(t)
			data := limitFailureEvent(time.Now().UTC().Add(time.Hour))
			if name == "rotated execution" {
				data.AgentExecutionID = "old-exec"
			} else {
				data.DynamicRouteAttempt = true
			}
			svc.handleAgentFailed(context.Background(), data)
			waitForFailureRecovery(t, svc)
			if _, limited := limits.Lookup(limits.ForProfile(profile, agents.NewOmpACP()), "anthropic/live-opus"); limited {
				t.Fatalf("%s marked a concrete binding", name)
			}
		})
	}
}

func newProviderLimitMarkService(t *testing.T) (*Service, *providerlimit.Service, *settingsmodels.AgentProfile) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	session.AgentProfileID = "limit-profile"
	session.ExecutionProfileID = "limit-profile"
	session.AgentExecutionID = "live-exec"
	session.State = models.TaskSessionStateRunning
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyRuntimeConfig, models.SessionRuntimeConfig{Model: "anthropic/live-opus"}); err != nil {
		t.Fatal(err)
	}
	seedExecutorRunning(t, repo, "s1", "t1", "live-exec")
	profile := &settingsmodels.AgentProfile{ID: "limit-profile", AgentID: "saved-omp", Model: "anthropic/saved-opus", Enabled: true}
	profiles := &limitMarkProfiles{profile: profile}
	limits := providerlimit.NewService(dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(repo)), dynamic.NewCredentialBindingResolver([]byte("installation-limit-test")), profiles, limitMarkCatalog{})
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{repoForExecutionLookup: repo})
	svc.messageCreator = &mockMessageCreator{}
	svc.providerLimits = limits
	t.Cleanup(func() { svc.cancelAllTransientRetries(); svc.stopDynamicSuccessorWorkers() })
	t.Cleanup(svc.stopProviderLimitRecovery)
	return svc, limits, profile
}

func limitFailureEvent(reset time.Time) watcher.AgentEventData {
	return watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "live-exec", AgentProfileID: "limit-profile", AgentID: "omp-acp", ProviderError: &streams.ProviderError{Source: streams.ProviderErrorSourceOMPACP, ProviderID: "omp-acp", ModelID: "anthropic/live-opus", Message: "This request would exceed your account's monthly spend limit. Please try again later.", ResetAt: &reset, OccurredAt: time.Now().UTC()}}
}

type limitMarkCatalog struct{}

func (limitMarkCatalog) Get(name string) (agents.Agent, bool) {
	if name == "omp-acp" {
		return agents.NewOmpACP(), true
	}
	return nil, false
}

type limitMarkProfiles struct{ profile *settingsmodels.AgentProfile }

func (r *limitMarkProfiles) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	if id == r.profile.ID {
		return r.profile, nil
	}
	return nil, errors.New("profile missing")
}
func (r *limitMarkProfiles) GetAgent(_ context.Context, id string) (*settingsmodels.Agent, error) {
	if id == r.profile.AgentID {
		return &settingsmodels.Agent{ID: id, Name: "omp-acp"}, nil
	}
	return nil, errors.New("agent missing")
}
func (r *limitMarkProfiles) ListAgents(context.Context) ([]*settingsmodels.Agent, error) {
	return []*settingsmodels.Agent{{ID: r.profile.AgentID, Name: "omp-acp"}}, nil
}
func (r *limitMarkProfiles) ListAgentProfiles(_ context.Context, id string) ([]*settingsmodels.AgentProfile, error) {
	if id == r.profile.AgentID {
		return []*settingsmodels.AgentProfile{r.profile}, nil
	}
	return nil, nil
}
