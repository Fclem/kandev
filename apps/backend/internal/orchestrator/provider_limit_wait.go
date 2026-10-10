package orchestrator

import (
	"context"
	"maps"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const providerLimitWaitIdentityKey = "provider_limit_wait_identity"
const providerLimitWaitCounterKey = "provider_limit_waits"
const providerLimitMaximumWaits = 3

type providerLimitWaitArmer interface {
	ArmProviderLimitWait(context.Context, *models.TaskSession, models.TaskSessionState, time.Time, map[string]interface{}, models.ProviderLimitWait) (bool, error)
}

func (s *Service) prepareProviderLimitWait(ctx context.Context, data watcher.AgentEventData, session *models.TaskSession, subject providerlimit.Subject, model string, mark providerlimit.Mark) func(context.Context) {
	if !subject.Profile.ResumeAfterReset || !mark.TrustedReset(s.providerLimitNow()) || s.messageCreator == nil {
		return nil
	}
	armer, capable := s.repo.(providerLimitWaitArmer)
	if !capable {
		return nil
	}
	turnID, waits, ok := s.providerLimitWaitState(ctx, session)
	if !ok {
		return nil
	}
	identity := models.ProviderLimitWaitIdentity(session.ID, turnID)
	if current, exists := waits[identity]; exists && current.FailureExecutionID == data.AgentExecutionID && current.FailureGeneration == data.PromptGeneration && session.Metadata[providerLimitWaitIdentityKey] == identity {
		return func(runCtx context.Context) { s.publishTaskUpdatedByID(runCtx, session.TaskID) }
	}
	count := models.CeilingRecordInt(session.Metadata[providerLimitWaitCounterKey])
	if count >= providerLimitMaximumWaits {
		providerlimit.ObserveWait(s.logger.Zap(), providerlimit.MetricContextKanban, providerlimit.MetricWaitExhausted)
		return nil
	}
	wait, ok := s.newProviderLimitWait(ctx, data, session, turnID, model, mark)
	if !ok {
		return nil
	}
	if previous, exists := waits[identity]; exists {
		wait.QueuedAt = previous.QueuedAt
	}
	messageID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("provider-limit-wait:"+identity+":"+strconv.FormatUint(data.PromptGeneration, 10)+":"+strconv.Itoa(count+1))).String()
	notice := providerLimitWaitNotice(session, identity, model, mark)
	if err := s.messageCreator.CreateSessionMessageIdempotent(ctx, messageID, session.TaskID, "Waiting for the provider limit reset.", session.ID, string(v1.MessageTypeStatus), turnID, notice, false); err != nil {
		s.logger.Warn("persist provider limit wait notice failed", zap.String("session_id", session.ID), zap.Error(err))
		return nil
	}
	if !s.armProviderLimitWait(ctx, armer, session, identity, count+1, wait) {
		return nil
	}
	return func(runCtx context.Context) {
		s.publishTaskUpdatedByID(runCtx, session.TaskID)
		s.ReconcileProviderLimitWaits(runCtx)
	}
}

// providerLimitWaitState returns the session's active turn and the task's
// existing provider-limit waits.
func (s *Service) providerLimitWaitState(ctx context.Context, session *models.TaskSession) (string, map[string]models.ProviderLimitWait, bool) {
	turnID, err := s.peekActiveTurnID(ctx, session.ID)
	if err != nil || turnID == "" {
		return "", nil, false
	}
	record, _, err := s.repo.GetTaskDeferredLaunch(ctx, session.TaskID)
	if err != nil {
		return "", nil, false
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil {
		return "", nil, false
	}
	return turnID, waits, true
}

// newProviderLimitWait builds the durable wait that replays the failed turn's
// prompt once the mark resets.
func (s *Service) newProviderLimitWait(ctx context.Context, data watcher.AgentEventData, session *models.TaskSession, turnID, model string, mark providerlimit.Mark) (models.ProviderLimitWait, bool) {
	prompt, safe := s.providerLimitFallbackPrompt(ctx, data, turnID)
	if !safe {
		return models.ProviderLimitWait{}, false
	}
	task, err := s.repo.GetTask(ctx, session.TaskID)
	if err != nil || task == nil || task.ArchivedAt != nil {
		return models.ProviderLimitWait{}, false
	}
	return models.ProviderLimitWait{
		SessionID: session.ID, TurnID: turnID, MarkKey: mark.Key, Model: model,
		WorkflowStepID: task.WorkflowStepID, NotBefore: mark.Until, QueuedAt: time.Now().UTC(),
		FailureExecutionID: data.AgentExecutionID, FailureGeneration: data.PromptGeneration,
		Payload: map[string]interface{}{
			metaKeySessionID: session.ID, metaKeyPrompt: prompt.text, sessionModelConfigKey: model,
			metaKeyPlanMode: prompt.planMode, metaKeyAttachments: prompt.attachments,
			"dispatch_only": true,
		},
	}, true
}

// providerLimitWaitNotice is the status-message metadata for an armed wait,
// including the action that cancels it.
func providerLimitWaitNotice(session *models.TaskSession, identity, model string, mark providerlimit.Mark) map[string]interface{} {
	cancel := wsRecoveryAction(session.TaskID, session.ID, recoverActionCancelRetry, "Cancel", "x", "", recoveryCancelRetryButtonTestID)
	cancel["params"].(map[string]interface{})["payload"].(map[string]interface{})["limit_wait_identity"] = identity
	return map[string]interface{}{
		metaKeyVariant: metaVariantWarning, metaKeyRetrying: true, "limit_wait": true,
		metaKeyRetryAt: mark.Until.UTC().Format(time.RFC3339Nano), metaKeyModelID: model,
		"failure_code": string(mark.Code), "actions": []map[string]interface{}{cancel},
		"limit_wait_identity": identity,
	}
}

// armProviderLimitWait moves the session to WAITING_FOR_INPUT with the wait as
// its owner and publishes the transition. It reports whether the wait armed.
func (s *Service) armProviderLimitWait(ctx context.Context, armer providerLimitWaitArmer, session *models.TaskSession, identity string, count int, wait models.ProviderLimitWait) bool {
	next := *session
	next.State, next.ErrorMessage = models.TaskSessionStateWaitingForInput, ""
	metadata := maps.Clone(session.Metadata)
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata[providerLimitWaitCounterKey] = count
	metadata[providerLimitWaitIdentityKey] = identity
	changed, err := armer.ArmProviderLimitWait(ctx, &next, session.State, session.UpdatedAt, metadata, wait)
	if err != nil {
		s.logger.Warn("persist provider limit wait ownership failed", zap.String("session_id", session.ID), zap.Error(err))
		return false
	}
	if !changed {
		return false
	}
	providerlimit.ObserveWait(s.logger.Zap(), providerlimit.MetricContextKanban, providerlimit.MetricWaitArmed)
	next.Metadata = metadata
	s.releaseCeilingIfLeftPopulation(session.ID, session.State, next.State)
	s.publishTaskSessionStateChanged(ctx, session.TaskID, session.ID, session.State, next.State, "", &next.UpdatedAt, &next)
	s.republishTaskActivityOnSettle(ctx, session.TaskID, session.State, next.State)
	s.writeTaskReviewState(ctx, session.TaskID, session.ID)
	return true
}
