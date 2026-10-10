package orchestrator

import (
	"context"
	"errors"
	"expvar"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.2
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.3
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.6
func TestProviderLimitFallbackContinuesSameSessionWithOneDurableWarning(t *testing.T) {
	ctx := context.Background()
	svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
	beforeSwitched := providerLimitFallbackMetric(t, providerlimit.MetricContextKanban, providerlimit.MetricFallbackSwitched)
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	// A replayed failure from the original prompt generation cannot recover again.
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	if delta := providerLimitFallbackMetric(t, providerlimit.MetricContextKanban, providerlimit.MetricFallbackSwitched) - beforeSwitched; delta != 1 {
		t.Fatalf("accepted fallback metric delta = %d, want 1", delta)
	}
	manager.mu.Lock()
	selected := append([]sessionModelCall(nil), manager.setSessionModelCalls...)
	prompts := append([]string(nil), manager.capturedPrompts...)
	manager.mu.Unlock()
	if len(selected) != 1 || selected[0].SessionID != "s1" || selected[0].ModelID != profile.FallbackModel {
		t.Fatalf("same-session fallback selection = %+v", selected)
	}
	if len(prompts) != 1 || prompts[0] != "original user input" {
		t.Fatalf("never-started turn replay = %+v", prompts)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if session.Metadata["provider_limit_fallback_turn"] != "limited-turn" {
		t.Fatalf("failed turn did not own the durable fallback marker: %+v", session.Metadata)
	}
	if session.AgentProfileSnapshot["model"] != profile.FallbackModel {
		t.Fatalf("fallback did not remain selected: %+v", session.AgentProfileSnapshot)
	}
	rows, err := repo.ListMessages(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	warnings := 0
	for _, row := range rows {
		if row.Metadata["reason"] == "provider_limit" {
			warnings++
			if row.Metadata["requested_model"] != "anthropic/live-opus" || row.Metadata["effective_model"] != profile.FallbackModel || row.Metadata["decision_id"] == "" {
				t.Fatalf("durable warning lost decision context: %+v", row.Metadata)
			}
		}
	}
	if warnings != 1 {
		t.Fatalf("durable fallback warnings = %d", warnings)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.2
func TestProviderLimitFallbackPersistsOutputFromNewPromptGeneration(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(map[bool]string{true: "known", false: "unknown"}[known], func(t *testing.T) {
			ctx := context.Background()
			svc, _, _, _, data := newProviderLimitFallbackFixture(t)
			if !known {
				svc.clearPromptAttemptEvidence("s1", "live-exec", 1)
				data.EvidenceKnown = false
			}
			svc.handleAgentFailed(ctx, data)
			waitForFailureRecovery(t, svc)
			svc.beginPromptAttempt("s1", "live-exec", 2, false)
			messages := svc.messageCreator.(*providerLimitPersistedMessages).mockMessageCreator
			for _, generation := range []uint64{1, 2} {
				svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
					TaskID: "t1", SessionID: "s1", ExecutionID: "live-exec",
					Data: &lifecycle.AgentStreamEventData{
						Type: agentEventComplete, Text: "fallback output", PromptGeneration: generation,
					},
				})
				if generation == 1 && messages.agentMessageWrites != 0 {
					t.Fatal("late failed-prompt completion was persisted")
				}
			}
			if messages.agentMessageWrites != 1 || messages.agentMessages[0].content != "fallback output" {
				t.Fatalf("fallback output was not persisted: %+v", messages.agentMessages)
			}
		})
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.5
func TestProviderLimitFallbackLateFailureCannotSettleNewPrompt(t *testing.T) {
	ctx := context.Background()
	svc, _, _, repo, data := newProviderLimitFallbackFixture(t)
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	svc.beginPromptAttempt("s1", "live-exec", 2, false)
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil || session.State != models.TaskSessionStateRunning {
		t.Fatalf("late failed prompt settled its successor: %+v, %v", session, err)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.5
func TestProviderLimitFallbackLateFailureAfterCompletionDoesNotAddRecovery(t *testing.T) {
	ctx := context.Background()
	svc, _, _, repo, data := newProviderLimitFallbackFixture(t)
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "t1", SessionID: "s1", ExecutionID: "live-exec",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete, Text: "fallback output", PromptGeneration: 2,
		},
	})
	before, err := repo.ListMessages(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	after, err := repo.ListMessages(ctx, "s1")
	if err != nil || len(after) != len(before) {
		t.Fatalf("late failure added a recovery surface after successful fallback: before=%+v after=%+v err=%v", before, after, err)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.2
func TestProviderLimitFallbackReportedPromptFailurePreservesTurnUntilActorRecovery(t *testing.T) {
	ctx := context.Background()
	svc, _, manager, _, data := newProviderLimitFallbackFixture(t)
	reported := errors.Join(lifecycle.ErrAgentReported, errors.New(data.ProviderError.Message))
	if err := svc.handlePromptError(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, reported); !errors.Is(err, lifecycle.ErrAgentReported) {
		t.Fatalf("terminal provider evidence was lost: %v", err)
	}
	turnID, err := svc.peekActiveTurnID(ctx, "s1")
	if err != nil || turnID != "limited-turn" {
		t.Fatalf("synchronous failure settled the actor-owned failed turn: %q, %v", turnID, err)
	}
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.setSessionModelCalls) != 1 || len(manager.capturedPrompts) != 1 || manager.capturedPrompts[0] != "original user input" {
		t.Fatalf("reported failure was not recovered once: models=%+v prompts=%+v", manager.setSessionModelCalls, manager.capturedPrompts)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.3
func TestProviderLimitFallbackLaterDecisionHasItsOwnDurableWarning(t *testing.T) {
	ctx := context.Background()
	svc, _, _, repo, data := newProviderLimitFallbackFixture(t)
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	svc.completeTurnForSession(ctx, "s1")
	now := time.Now().UTC()
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "later-limited-turn", TaskSessionID: "s1", TaskID: "t1", StartedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc.activeTurns.Store("s1", "later-limited-turn")
	if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateRunning, ""); err != nil {
		t.Fatal(err)
	}
	svc.beginPromptAttempt("s1", "live-exec", 2, false)
	data.PromptGeneration = 2
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	rows, err := repo.ListMessages(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	decisions := map[string]string{}
	for _, row := range rows {
		if row.Metadata["reason"] == "provider_limit" {
			id, ok := row.Metadata["decision_id"].(string)
			if !ok || id == "" {
				t.Fatalf("missing durable decision on %s", row.TurnID)
			}
			decisions[row.TurnID] = id
		}
	}
	if len(decisions) != 2 || decisions["limited-turn"] == decisions["later-limited-turn"] {
		t.Fatalf("separate failed turns reused a warning: %+v", decisions)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.5
func TestProviderLimitFallbackLimitedFallbackDoesNotSwitchAgain(t *testing.T) {
	ctx := context.Background()
	svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	svc.agentManager.(*providerLimitCatalogManager).state.CurrentModelID = profile.FallbackModel
	data.ProviderError.ModelID = profile.FallbackModel
	svc.beginPromptAttempt("s1", "live-exec", 2, false)
	data.PromptGeneration = 2
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.setSessionModelCalls) != 1 || len(manager.capturedPrompts) != 1 {
		t.Fatal("limited fallback caused another automatic switch or replay")
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("limited fallback did not retain recovery surface: %s", session.State)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.5
func TestProviderLimitFallbackDispatchAndWarningFailuresRemainRecoverable(t *testing.T) {
	for _, failure := range []string{"prompt refused", "warning write refused"} {
		t.Run(failure, func(t *testing.T) {
			svc, _, manager, repo, data := newProviderLimitFallbackFixture(t)
			switch failure {
			case "prompt refused":
				manager.promptErr = errors.New("provider rejected fallback prompt")
			case "warning write refused":
				if _, err := repo.DB().Exec(`CREATE TRIGGER reject_fallback_warning BEFORE INSERT ON task_session_messages WHEN json_extract(NEW.metadata,'$.reason') = 'provider_limit' BEGIN SELECT RAISE(ABORT,'warning refused'); END`); err != nil {
					t.Fatal(err)
				}
			}
			svc.handleAgentFailed(context.Background(), data)
			waitForFailureRecovery(t, svc)
			session, err := repo.GetTaskSession(context.Background(), "s1")
			if err != nil {
				t.Fatal(err)
			}
			if session.State != models.TaskSessionStateWaitingForInput {
				t.Fatalf("%s stranded session in %s", failure, session.State)
			}
			if failure == "warning write refused" {
				manager.mu.Lock()
				defer manager.mu.Unlock()
				if len(manager.setSessionModelCalls) != 0 || len(manager.capturedPrompts) != 0 {
					t.Fatal("fallback proceeded without a durable warning")
				}
			}
		})
	}
}

func newProviderLimitFallbackFixture(t *testing.T) (*Service, *settingsmodels.AgentProfile, *mockAgentManager, *taskrepo.Repository, watcher.AgentEventData) {
	t.Helper()
	svc, _, profile := newProviderLimitMarkService(t)
	profile.LimitFallback = true
	profile.FallbackModel = "openai-codex/gpt-6.1-sol"
	manager := svc.agentManager.(*mockAgentManager)
	manager.isAgentRunning = true
	manager.setSessionModelSupported = true
	manager.resolveProfileInfo = &executor.AgentProfileInfo{Model: "anthropic/live-opus", SupportsMCP: true}
	svc.agentManager = &providerLimitCatalogManager{mockAgentManager: manager, state: &lifecycle.CachedModelState{CurrentModelID: "anthropic/live-opus", Models: []streams.SessionModelInfo{{ModelID: profile.FallbackModel}}}}
	repo := svc.repo.(*taskrepo.Repository)
	svc.turnService = &providerLimitTurnService{repoTurnService: &repoTurnService{repo: repo}}
	now := time.Now().UTC()
	if err := repo.CreateTurn(context.Background(), &models.Turn{ID: "limited-turn", TaskSessionID: "s1", TaskID: "t1", StartedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc.activeTurns.Store("s1", "limited-turn")
	svc.lastTurnPrompt.Store("s1", capturedPrompt{text: "original user input", model: "anthropic/live-opus"})
	svc.messageCreator = &providerLimitPersistedMessages{mockMessageCreator: &mockMessageCreator{}, repo: repo}
	svc.beginPromptAttempt("s1", "live-exec", 1, false)
	data := limitFailureEvent(now.Add(time.Hour))
	data.EvidenceKnown = true
	data.PromptGeneration = 1
	return svc, profile, manager, repo, data
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.4
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.5
func TestProviderLimitFallbackIneligibleDecisionsKeepRecoverySurface(t *testing.T) {
	for _, name := range []string{"opted out", "strict", "automatic fallback", "empty fallback", "missing catalog", "unadvertised", "marked", "already used", "mark write fails", "ownership write fails"} {
		t.Run(name, func(t *testing.T) {
			svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
			ctx := context.Background()
			expectedOutcome := ""
			catalog := svc.agentManager.(*providerLimitCatalogManager)
			switch name {
			case "opted out":
				profile.LimitFallback = false
			case "strict":
				profile.RequireExactModel = true
			case "automatic fallback":
				profile.AutoFallback = true
			case "empty fallback":
				profile.FallbackModel = " "
			case "missing catalog":
				catalog.state = nil
				expectedOutcome = providerlimit.MetricFallbackFailed
			case "unadvertised":
				catalog.state.Models = []streams.SessionModelInfo{{ModelID: "host-only-model"}}
				expectedOutcome = providerlimit.MetricFallbackNotAdvertised
			case "marked":
				subject := svc.providerLimits.ForProfile(profile, agents.NewOmpACP())
				if _, err := svc.providerLimits.Record(ctx, subject, profile.FallbackModel, &routingerr.Error{Code: routingerr.CodeRateLimited}, time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
				expectedOutcome = providerlimit.MetricFallbackMarked
			case "already used":
				if err := repo.SetSessionMetadataKey(ctx, "s1", providerLimitFallbackTurnKey, "limited-turn"); err != nil {
					t.Fatal(err)
				}
			case "mark write fails":
				if _, err := repo.DB().Exec(`CREATE TRIGGER reject_fallback_mark BEFORE INSERT ON dynamic_resource_circuits BEGIN SELECT RAISE(ABORT,'mark refused'); END`); err != nil {
					t.Fatal(err)
				}
			case "ownership write fails":
				if _, err := repo.DB().Exec(`CREATE TRIGGER reject_fallback_ownership BEFORE UPDATE OF metadata ON task_sessions WHEN json_extract(NEW.metadata,'$.provider_limit_fallback_turn') IS NOT NULL BEGIN SELECT RAISE(ABORT,'ownership refused'); END`); err != nil {
					t.Fatal(err)
				}
			}
			beforeFallbackMetric := providerLimitFallbackMetric(t, providerlimit.MetricContextKanban, expectedOutcome)
			svc.handleAgentFailed(ctx, data)
			waitForFailureRecovery(t, svc)
			if expectedOutcome != "" {
				if after := providerLimitFallbackMetric(t, providerlimit.MetricContextKanban, expectedOutcome); after-beforeFallbackMetric != 1 {
					t.Fatalf("%s fallback metric delta = %d, want 1", name, after-beforeFallbackMetric)
				}
			}
			manager.mu.Lock()
			switches, prompts := len(manager.setSessionModelCalls), len(manager.capturedPrompts)
			manager.mu.Unlock()
			if switches != 0 || prompts != 0 {
				t.Fatalf("%s switched or resent: switches=%d prompts=%d", name, switches, prompts)
			}
			session, err := repo.GetTaskSession(ctx, "s1")
			if err != nil {
				t.Fatal(err)
			}
			if session.State != models.TaskSessionStateWaitingForInput {
				t.Fatalf("%s removed the existing recovery state: %s", name, session.State)
			}
		})
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.2
func TestProviderLimitFallbackNeverReplaysInputAfterTurnEvents(t *testing.T) {
	for _, observed := range []string{"output", "effect", "unknown"} {
		t.Run(observed, func(t *testing.T) {
			svc, _, manager, _, data := newProviderLimitFallbackFixture(t)
			switch observed {
			case "output":
				data.OutputObserved = true
			case "effect":
				data.EffectObserved = true
			case "unknown":
				data.EvidenceKnown = false
				svc.clearPromptAttemptEvidence("s1", "live-exec", 1)
			}
			svc.handleAgentFailed(context.Background(), data)
			waitForFailureRecovery(t, svc)
			manager.mu.Lock()
			defer manager.mu.Unlock()
			if len(manager.capturedPrompts) != 1 || manager.capturedPrompts[0] != providerLimitContinuationPrompt {
				t.Fatalf("%s evidence resent prior input: %+v", observed, manager.capturedPrompts)
			}
		})
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.2
func TestProviderLimitFallbackRecoversPersistedInputForTheFailedTurnOnly(t *testing.T) {
	svc, _, manager, repo, data := newProviderLimitFallbackFixture(t)
	svc.lastTurnPrompt.Delete("s1")
	if err := repo.CreateMessage(context.Background(), &models.Message{TaskSessionID: "s1", TaskID: "t1", TurnID: "limited-turn", AuthorType: "user", Content: "persisted exact turn input"}); err != nil {
		t.Fatal(err)
	}
	svc.handleAgentFailed(context.Background(), data)
	waitForFailureRecovery(t, svc)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.capturedPrompts) != 1 || manager.capturedPrompts[0] != "persisted exact turn input" {
		t.Fatalf("persisted failed-turn replay=%+v", manager.capturedPrompts)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.6
func TestProviderLimitFallbackCancellationAndArchiveInvalidatePreparedContinuation(t *testing.T) {
	for _, mutation := range []string{"cancel", "archive", "stop", "delete"} {
		t.Run(mutation, func(t *testing.T) {
			svc, _, manager, repo, data := newProviderLimitFallbackFixture(t)
			continuation := svc.handleAgentFailedLocked(context.Background(), data)
			if continuation == nil {
				t.Fatal("eligible failure did not prepare continuation")
			}
			switch mutation {
			case "cancel":
				finish := svc.beginCancellationProjection("s1")
				finish()
			case "archive":
				if err := repo.ArchiveTask(context.Background(), "t1"); err != nil {
					t.Fatal(err)
				}
			case "stop":
				if err := repo.UpdateTaskSessionState(context.Background(), "s1", models.TaskSessionStateCancelled, ""); err != nil {
					t.Fatal(err)
				}
			case "delete":
				if err := repo.DeleteTask(context.Background(), "t1"); err != nil {
					t.Fatal(err)
				}
			}
			continuation(context.Background())
			manager.mu.Lock()
			defer manager.mu.Unlock()
			if len(manager.setSessionModelCalls) != 0 || len(manager.capturedPrompts) != 0 {
				t.Fatalf("%s did not invalidate continuation", mutation)
			}
		})
	}
}

type providerLimitCatalogManager struct {
	*mockAgentManager
	state *lifecycle.CachedModelState
}

func (m *providerLimitCatalogManager) GetModelStateForSession(string) *lifecycle.CachedModelState {
	return m.state
}
func (m *providerLimitCatalogManager) GetPromptGenerationForSession(context.Context, string) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return uint64(len(m.capturedPrompts)) + 1, nil
}

type providerLimitTurnService struct{ *repoTurnService }

func (s *providerLimitTurnService) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	now := time.Now().UTC()
	turn := &models.Turn{ID: uuid.NewString(), TaskSessionID: sessionID, TaskID: "t1", StartedAt: now, CreatedAt: now, UpdatedAt: now}
	return turn, s.repo.CreateTurn(ctx, turn)
}
func (s *providerLimitTurnService) ReserveTurn(ctx context.Context, sessionID string, _ *models.PromptDispatchRecovery) (*models.Turn, error) {
	return s.StartTurn(ctx, sessionID)
}

type providerLimitPersistedMessages struct {
	*mockMessageCreator
	repo *taskrepo.Repository
}

func (m *providerLimitPersistedMessages) CreateSessionMessage(ctx context.Context, taskID, content, sessionID, messageType, turnID string, metadata map[string]interface{}, requestsInput bool) error {
	return m.repo.CreateMessage(ctx, &models.Message{TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID, Content: content, Type: models.MessageType(messageType), AuthorType: models.MessageAuthorType("agent"), Metadata: metadata, RequestsInput: requestsInput})
}
func (m *providerLimitPersistedMessages) CreateSessionMessageIdempotent(ctx context.Context, messageID, taskID, content, sessionID, messageType, turnID string, metadata map[string]interface{}, requestsInput bool) error {
	if message, err := m.repo.GetMessage(ctx, messageID); err == nil && message != nil {
		return nil
	}
	return m.repo.CreateMessage(ctx, &models.Message{ID: messageID, TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID, Content: content, Type: models.MessageType(messageType), AuthorType: models.MessageAuthorType("agent"), Metadata: metadata, RequestsInput: requestsInput})
}
func providerLimitFallbackMetric(t *testing.T, context, outcome string) int64 {
	t.Helper()
	value := expvar.Get("provider_limit_fallback_total")
	if value == nil {
		t.Fatal("provider limit fallback expvar map is not registered")
	}
	entry := value.(*expvar.Map).Get("context=" + context + ";outcome=" + outcome)
	if entry == nil {
		return 0
	}
	return entry.(*expvar.Int).Value()
}
