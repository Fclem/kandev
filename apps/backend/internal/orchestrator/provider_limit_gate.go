package orchestrator

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/task/models"
)

type providerLimitGateDecision struct {
	subject providerlimit.Subject
	mark    providerlimit.Mark
	model   string
	policy  *agentruntime.StartModelPolicy
	wait    bool
}

func (d *providerLimitGateDecision) launchContext(ctx context.Context) context.Context {
	if d == nil || d.policy == nil {
		return ctx
	}
	return agentruntime.WithStartModelPolicy(ctx, *d.policy)
}

// The gate selects only the profile's authorized fallback; it never changes the profile.
func (s *Service) providerLimitGate(ctx context.Context, profileID, model string, origin launchOrigin) (*providerLimitGateDecision, error) {
	if s.providerLimits == nil || origin != launchOriginAutomatic || profileID == "" {
		return nil, nil
	}
	subject, err := s.providerLimits.ResolveSubject(ctx, profileID)
	if err != nil || subject.Dynamic || subject.Profile == nil {
		return nil, err
	}
	policy := agentruntime.LimitPolicyForProfile(subject.Profile)
	if policy.FallbackModel == "" && !policy.ResumeAfterReset {
		return nil, nil
	}
	if model == "" {
		model = subject.Profile.Model
	}
	mark, limited := s.providerLimits.Lookup(subject, model)
	if !limited {
		return nil, nil
	}
	decision := &providerLimitGateDecision{subject: subject, mark: mark, model: model}
	if policy.FallbackModel != "" && policy.FallbackModel != model {
		if _, limited := s.providerLimits.Lookup(subject, policy.FallbackModel); !limited {
			s.observeKanbanFallback(providerlimit.MetricFallbackSwitched)
			decision.policy = &agentruntime.StartModelPolicy{Model: policy.FallbackModel, RequireExactModel: true}
			return decision, nil
		}
		s.observeKanbanFallback(providerlimit.MetricFallbackMarked)
	}
	decision.wait = policy.ResumeAfterReset && mark.TrustedReset(s.providerLimitNow())
	return decision, nil
}

// mergeExistingProviderLimitLaunch keeps the identity and payload of an
// equivalent launch already deferred for the task, rejecting a conflicting one.
func mergeExistingProviderLimitLaunch(launch *models.ProviderLimitLaunch, existing *models.ProviderLimitLaunch, record map[string]interface{}, candidate models.CeilingDeferral) error {
	if existing != nil {
		if err := requireEquivalentCeilingDeferral(models.CeilingDeferral{Kind: existing.Kind, Payload: existing.Payload}, candidate); err != nil {
			return err
		}
		launch.ID, launch.QueuedAt, launch.Payload, launch.ProbeLease = existing.ID, existing.QueuedAt, existing.Payload, existing.ProbeLease
		return nil
	}
	ceiling, err := models.ReadCeilingDeferral(record)
	if err != nil {
		return nil
	}
	if err := requireEquivalentCeilingDeferral(ceiling, candidate); err != nil {
		return err
	}
	launch.Payload, launch.QueuedAt = ceiling.Payload, ceiling.QueuedAt
	return nil
}

func requireEquivalentCeilingDeferral(existing, candidate models.CeilingDeferral) error {
	equivalent, err := ceilingDeferralsEquivalentForAdmission(existing, candidate)
	if err != nil {
		return err
	}
	if !equivalent {
		return ErrCeilingLaunchConflict
	}
	return nil
}

func (s *Service) deferProviderLimitLaunch(ctx context.Context, taskID, sessionID string, kind models.CeilingLaunchKind, payload map[string]interface{}, decision *providerLimitGateDecision) error {
	admissionCtx, release := s.lockCeilingEntryAdmission(ctx, taskID)
	defer release()
	task, err := s.repo.GetTask(admissionCtx, taskID)
	if err != nil {
		return err
	}
	if task == nil || task.ArchivedAt != nil {
		return fmt.Errorf("provider limit launch task is unavailable")
	}
	payload = s.enrichCeilingLaunchPayload(admissionCtx, taskID, sessionID, payload)
	for range deferredLaunchCASRetryBudget {
		launch := models.ProviderLimitLaunch{ID: uuid.NewString(), Kind: kind, Payload: payload, Origin: string(launchOriginAutomatic), WorkflowStepID: task.WorkflowStepID, MarkKey: decision.mark.Key, Model: decision.model, NotBefore: decision.mark.Until, QueuedAt: s.providerLimitNow(), SessionID: sessionID}
		stored, armed, err := s.storeProviderLimitLaunch(admissionCtx, taskID, launch)
		if err != nil {
			return err
		}
		if stored {
			if armed {
				providerlimit.ObserveWait(s.logger.Zap(), providerlimit.MetricContextKanban, providerlimit.MetricWaitArmed)
			}
			s.scheduleProviderLimitWake(launch.NotBefore)
			return nil
		}
	}
	return fmt.Errorf("provider limit launch admission could not persist ownership")
}

// storeProviderLimitLaunch performs one compare-and-set write of launch into
// the task's deferred-launch record. It reports whether the write landed and
// whether it armed a new wait rather than refreshing an existing one.
func (s *Service) storeProviderLimitLaunch(ctx context.Context, taskID string, launch models.ProviderLimitLaunch) (stored, armed bool, err error) {
	record, prior, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
	if err != nil {
		return false, false, err
	}
	existing, err := models.ReadProviderLimitLaunch(record)
	if err != nil {
		return false, false, err
	}
	candidate := models.CeilingDeferral{Kind: launch.Kind, Payload: launch.Payload}
	if err := mergeExistingProviderLimitLaunch(&launch, existing, record, candidate); err != nil {
		return false, false, err
	}
	updated, err := models.PutProviderLimitLaunch(record, launch)
	if err != nil {
		return false, false, err
	}
	stored, lostCompare, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, updated)
	if err != nil {
		return false, false, err
	}
	if !stored && !lostCompare {
		return false, false, fmt.Errorf("provider limit launch write returned no outcome")
	}
	return stored, existing == nil, nil
}

func (s *Service) gateProviderLimitLaunch(ctx context.Context, taskID, sessionID string, origin launchOrigin, kind models.CeilingLaunchKind, payload map[string]interface{}) (*providerLimitGateDecision, bool, error) {
	if s.providerLimits == nil || origin != launchOriginAutomatic || kind == models.CeilingLaunchDynamicRelaunch {
		return nil, false, nil
	}
	decision, err := s.providerLimitLaunchDecision(ctx, taskID, sessionID, origin, payload)
	if err != nil || decision == nil || !decision.wait {
		return decision, false, err
	}
	if err := s.deferProviderLimitLaunch(ctx, taskID, sessionID, kind, payload, decision); err != nil {
		return nil, false, err
	}
	s.reconcileQueuedTaskState(ctx, taskID)
	s.publishTaskUpdatedByID(ctx, taskID)
	return decision, true, nil
}

func (s *Service) providerLimitLaunchDecision(ctx context.Context, taskID, sessionID string, origin launchOrigin, payload map[string]interface{}) (*providerLimitGateDecision, error) {
	if s.providerLimits == nil || origin != launchOriginAutomatic {
		return nil, nil
	}
	office, err := s.lookupOfficeTask(ctx, taskID)
	if err != nil || office {
		return nil, err
	}
	profileID, _ := payload[metaKeyAgentProfileID].(string)
	model, _ := payload[sessionModelConfigKey].(string)
	if profileID == "" && sessionID != "" {
		profileID, model, err = s.providerLimitSessionProfile(ctx, sessionID, model)
		if err != nil {
			return nil, err
		}
	}
	return s.providerLimitGate(ctx, profileID, model, origin)
}

// providerLimitSessionProfile resolves the profile and, when model is empty,
// the model of the session a launch resumes.
func (s *Service) providerLimitSessionProfile(ctx context.Context, sessionID, model string) (string, string, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return "", "", err
	}
	if session == nil {
		return "", "", fmt.Errorf("provider limit launch session is unavailable")
	}
	if model == "" {
		_, model, err = s.providerLimitSubjectForSession(ctx, session)
		if err != nil {
			return "", "", err
		}
	}
	return session.AgentProfileID, model, nil
}
