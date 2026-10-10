package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const providerLimitContinuationPrompt = "Continue the current task from where the previous turn stopped. Preserve the existing conversation and completed work; do not repeat completed actions."
const providerLimitFallbackTurnKey = "provider_limit_fallback_turn"

const (
	providerLimitReason       = "provider_limit"
	modelSelectionWarningKind = "model_selection_warning"
)

// Reported terminal errors already have an agent.failed event. Its actor owns
// the opted-in recovery decision and the failed turn, in either delivery order.
func (s *Service) providerLimitRecoveryOwnsReportedFailure(ctx context.Context, sessionID string, failure error) bool {
	if s.providerLimits == nil || !errors.Is(failure, agentruntime.ErrAgentReported) || errors.Is(failure, agentruntime.ErrCancelEscalated) {
		return false
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.RouteGeneration > 0 {
		return false
	}
	subject, _, err := s.providerLimitSubjectForSession(ctx, session)
	if err != nil || subject.Dynamic {
		return false
	}
	policy := agentruntime.LimitPolicyForProfile(subject.Profile)
	if policy.FallbackModel == "" && !policy.ResumeAfterReset {
		return false
	}
	office, err := s.lookupOfficeTask(ctx, session.TaskID)
	return err == nil && !office
}

// The returned continuation runs only after the session failure guard is released.
func (s *Service) handleProviderLimitFailure(ctx context.Context, data watcher.AgentEventData) func(context.Context) {
	if s.providerLimits == nil || data.SessionID == "" || data.DynamicRouteAttempt || data.UserInitiated {
		return nil
	}
	failure := classifyKanbanFailure(data)
	if failure.Code != routingerr.CodeQuotaLimited && failure.Code != routingerr.CodeRateLimited {
		return nil
	}
	session, subject, model, ok := s.providerLimitFailureTarget(ctx, data)
	if !ok {
		return nil
	}
	model, observedAt := providerLimitObservation(data, model)
	mark, err := s.providerLimits.Record(ctx, subject, model, failure, observedAt)
	if err != nil {
		s.logger.Warn("persist concrete provider limit failed", zap.String("task_id", data.TaskID), zap.String("session_id", data.SessionID), zap.Error(err))
		return nil
	}
	if fallback := s.prepareProviderLimitFallback(ctx, data, session, subject, model); fallback != nil {
		return fallback
	}
	return s.prepareProviderLimitWait(ctx, data, session, subject, model, mark)
}

// providerLimitFailureTarget resolves the concrete Kanban session a limit
// failure belongs to and retires any probe that the failure ended.
func (s *Service) providerLimitFailureTarget(ctx context.Context, data watcher.AgentEventData) (*models.TaskSession, providerlimit.Subject, string, bool) {
	office, err := s.lookupOfficeTask(ctx, data.TaskID)
	if err != nil || office {
		return nil, providerlimit.Subject{}, "", false
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil || session.RouteGeneration > 0 {
		return nil, providerlimit.Subject{}, "", false
	}
	subject, model, err := s.providerLimitSubjectForSession(ctx, session)
	if err != nil || subject.Dynamic {
		return nil, providerlimit.Subject{}, "", false
	}
	session, err = s.retireFailedProviderLimitProbe(ctx, data, session)
	if err != nil || session == nil {
		s.logger.Warn("retire failed provider limit probe failed", zap.String("session_id", data.SessionID), zap.Error(err))
		return nil, providerlimit.Subject{}, "", false
	}
	return session, subject, model, true
}

// providerLimitObservation prefers the model and time the provider reported.
func providerLimitObservation(data watcher.AgentEventData, model string) (string, time.Time) {
	observedAt := time.Now().UTC()
	if data.ProviderError == nil {
		return model, observedAt
	}
	if data.ProviderError.ModelID != "" {
		model = data.ProviderError.ModelID
	}
	if !data.ProviderError.OccurredAt.IsZero() {
		observedAt = data.ProviderError.OccurredAt
	}
	return model, observedAt
}

func (s *Service) observeKanbanFallback(outcome string) {
	providerlimit.ObserveFallback(s.logger.Zap(), providerlimit.MetricContextKanban, outcome)
}

func (s *Service) prepareProviderLimitFallback(ctx context.Context, data watcher.AgentEventData, session *models.TaskSession, subject providerlimit.Subject, model string) func(context.Context) {
	fallback, ok := s.providerLimitFallbackCandidate(session, subject, model)
	if !ok {
		return nil
	}
	turnID, prompt, ok := s.claimProviderLimitFallbackTurn(ctx, data, session)
	if !ok {
		return nil
	}
	_, revision := s.CancellationPendingSnapshot(session.ID)
	fence := &promptCancellationFence{revision: revision}
	hash := sha256.Sum256([]byte(strings.Join([]string{session.ID, turnID, model, providerLimitReason, fallback}, "\x00")))
	warning := streams.ModelSelectionWarning{Kind: modelSelectionWarningKind, DecisionID: hex.EncodeToString(hash[:]), Reason: providerLimitReason, RequestedModel: model, EffectiveModel: fallback, FallbackModel: fallback, AgentID: data.AgentID}
	return func(runCtx context.Context) {
		s.continueProviderLimitFallback(runCtx, data, turnID, prompt, fallback, fence, warning)
	}
}

// providerLimitFallbackCandidate returns the profile's authorized fallback when
// the live execution advertises it and it is not itself limited.
func (s *Service) providerLimitFallbackCandidate(session *models.TaskSession, subject providerlimit.Subject, model string) (string, bool) {
	fallback := agentruntime.LimitPolicyForProfile(subject.Profile).FallbackModel
	if fallback == "" || fallback == model {
		return "", false
	}
	if s.messageCreator == nil {
		s.observeKanbanFallback(providerlimit.MetricFallbackFailed)
		return "", false
	}
	advertised, known := s.sessionAdvertisesModel(session.ID, fallback)
	if !known {
		s.observeKanbanFallback(providerlimit.MetricFallbackFailed)
		return "", false
	}
	if !advertised {
		s.observeKanbanFallback(providerlimit.MetricFallbackNotAdvertised)
		return "", false
	}
	if _, limited := s.providerLimits.Lookup(subject, fallback); limited {
		s.observeKanbanFallback(providerlimit.MetricFallbackMarked)
		return "", false
	}
	return fallback, true
}

// sessionAdvertisesModel reports whether the live execution's model catalog
// contains modelID; known is false when no catalog is available.
func (s *Service) sessionAdvertisesModel(sessionID, modelID string) (advertised, known bool) {
	getter, ok := s.agentManager.(interface {
		GetModelStateForSession(string) *agentruntime.CachedModelState
	})
	if !ok {
		return false, false
	}
	state := getter.GetModelStateForSession(sessionID)
	if state == nil {
		return false, false
	}
	for _, item := range state.Models {
		if item.ModelID == modelID {
			return true, true
		}
	}
	return false, true
}

// claimProviderLimitFallbackTurn durably records that the active turn owns one
// fallback continuation and returns the prompt that continuation replays.
func (s *Service) claimProviderLimitFallbackTurn(ctx context.Context, data watcher.AgentEventData, session *models.TaskSession) (string, capturedPrompt, bool) {
	turnID, err := s.peekActiveTurnID(ctx, session.ID)
	if err != nil {
		s.observeKanbanFallback(providerlimit.MetricFallbackFailed)
		return "", capturedPrompt{}, false
	}
	if turnID == "" || session.Metadata[providerLimitFallbackTurnKey] == turnID {
		return "", capturedPrompt{}, false
	}
	prompt, ok := s.providerLimitFallbackPrompt(ctx, data, turnID)
	if !ok {
		s.observeKanbanFallback(providerlimit.MetricFallbackFailed)
		return "", capturedPrompt{}, false
	}
	updater, ok := s.repo.(interface {
		UpdateTaskSessionIfCurrentSnapshot(context.Context, *models.TaskSession, models.TaskSessionState, time.Time, map[string]interface{}) (bool, error)
	})
	if !ok {
		s.observeKanbanFallback(providerlimit.MetricFallbackFailed)
		return "", capturedPrompt{}, false
	}
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	session.Metadata[providerLimitFallbackTurnKey] = turnID
	changed, err := updater.UpdateTaskSessionIfCurrentSnapshot(ctx, session, session.State, session.UpdatedAt, session.Metadata)
	if err != nil {
		s.logger.Warn("persist provider limit fallback ownership failed", zap.String("session_id", session.ID), zap.Error(err))
		s.observeKanbanFallback(providerlimit.MetricFallbackFailed)
		return "", capturedPrompt{}, false
	}
	return turnID, prompt, changed
}

func (s *Service) providerLimitFallbackPrompt(ctx context.Context, data watcher.AgentEventData, turnID string) (capturedPrompt, bool) {
	prompt := capturedPrompt{text: providerLimitContinuationPrompt}
	if cached, exists := s.lastTurnPrompt.Load(data.SessionID); exists {
		if captured, ok := cached.(capturedPrompt); ok {
			prompt = captured
		}
	}
	if !data.EvidenceKnown || data.OutputObserved || data.EffectObserved {
		prompt.text = providerLimitContinuationPrompt
		prompt.attachments = nil
		return prompt, true
	}
	if strings.TrimSpace(prompt.text) != "" && prompt.text != providerLimitContinuationPrompt {
		return prompt, true
	}
	messages, err := s.repo.ListMessages(ctx, data.SessionID)
	if err != nil {
		return capturedPrompt{}, false
	}
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.TurnID == turnID && message.AuthorType == models.MessageAuthorUser && strings.TrimSpace(message.Content) != "" {
			prompt.text = message.Content
			return prompt, true
		}
	}
	return capturedPrompt{}, false
}

// providerLimitFallbackSession re-reads the session under its cancellation
// guard and returns it only while the claimed turn still owns the fallback.
func (s *Service) providerLimitFallbackSession(ctx context.Context, data watcher.AgentEventData, turnID string, fence *promptCancellationFence) (*models.TaskSession, bool) {
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil {
		return nil, false
	}
	task, err := s.repo.GetTask(ctx, data.TaskID)
	if err != nil || task == nil || task.ArchivedAt != nil {
		return nil, false
	}
	currentTurn, err := s.peekActiveTurnID(ctx, data.SessionID)
	if err != nil || currentTurn != turnID {
		return nil, false
	}
	if s.isCancelInFlight(data.SessionID) || s.validatePromptCancellationFence(data.SessionID, fence) != nil {
		return nil, false
	}
	owned := session.AgentExecutionID == data.AgentExecutionID &&
		session.Metadata[providerLimitFallbackTurnKey] == turnID &&
		session.State == models.TaskSessionStateRunning
	return session, owned
}

// recordProviderLimitFallbackWarning persists and publishes the fallback
// decision while the session cancellation guard is held.
func (s *Service) recordProviderLimitFallbackWarning(ctx context.Context, data watcher.AgentEventData, turnID string, fence *promptCancellationFence, warning streams.ModelSelectionWarning) (bool, error) {
	lock, release := s.acquireCancelInFlightGuard(data.SessionID)
	lock.Lock()
	defer func() {
		lock.Unlock()
		release()
	}()
	session, ok := s.providerLimitFallbackSession(ctx, data, turnID, fence)
	if !ok {
		return false, nil
	}
	messageID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("provider-limit-warning:"+warning.DecisionID)).String()
	err := s.messageCreator.CreateSessionMessageIdempotent(ctx, messageID, data.TaskID, "The provider limit recovery selected the fallback model.", data.SessionID, string(v1.MessageTypeStatus), turnID, modelSelectionWarningMetadata(warning), false)
	if err != nil {
		return true, err
	}
	s.publishModelSelectionWarning(ctx, data.TaskID, data.SessionID, warning)
	s.setSessionWaitingForInput(ctx, data.TaskID, data.SessionID, session)
	return true, nil
}

func (s *Service) continueProviderLimitFallback(ctx context.Context, data watcher.AgentEventData, turnID string, prompt capturedPrompt, fallback string, fence *promptCancellationFence, warning streams.ModelSelectionWarning) {
	if ctx.Err() != nil {
		return
	}
	proceed, err := s.recordProviderLimitFallbackWarning(ctx, data, turnID, fence, warning)
	if !proceed {
		return
	}
	if err != nil {
		s.observeKanbanFallback(providerlimit.MetricFallbackFailed)
		s.logger.Warn("persist provider limit warning failed", zap.String("session_id", data.SessionID), zap.Error(err))
		s.finalizeAutomationRun(ctx, data.TaskID, false, data.ErrorMessage)
		s.handleRecoverableFailure(ctx, data)
		return
	}
	if _, err := s.promptTask(ctx, data.TaskID, data.SessionID, prompt.text, fallback, prompt.planMode, prompt.attachments, true, launchOriginAutomatic, promptTaskOptions{cancellationFence: fence, expectedCurrentTurnID: turnID, requireNonterminalSession: true, onAccepted: prompt.onAccepted}); err != nil {
		if ctx.Err() != nil || s.validatePromptCancellationFence(data.SessionID, fence) != nil {
			return
		}
		s.observeKanbanFallback(providerlimit.MetricFallbackFailed)
		s.logger.Warn("provider limit fallback failed", zap.String("session_id", data.SessionID), zap.Error(err))
		s.finalizeAutomationRun(ctx, data.TaskID, false, err.Error())
		s.handleRecoverableFailure(ctx, data)
		return
	}
	s.observeKanbanFallback(providerlimit.MetricFallbackSwitched)
}
