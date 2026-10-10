// Package providerlimit tracks concrete-profile limits independently of dynamic routing.
package providerlimit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	models "github.com/kandev/kandev/internal/agent/settings/models"
	"go.uber.org/zap"
)

const (
	UnknownResetLifetime = 30 * time.Minute
	ProbeLifetime        = 10 * time.Minute
	MaximumAutomaticWait = 7 * 24 * time.Hour
	keyPrefix            = "limit|"

	// ScopeAccount marks a limit shared by every model on one provider account.
	ScopeAccount = routingerr.LimitScopeAccount
	// ScopeModel marks a limit on one model.
	ScopeModel = routingerr.LimitScopeModel
)

var (
	ErrSubjectUnavailable = errors.New("concrete provider limit subject unavailable")
	ErrNotLimit           = errors.New("failure is not a provider limit")
)

type ProfileRepository interface {
	GetAgentProfile(context.Context, string) (*models.AgentProfile, error)
	GetAgent(context.Context, string) (*models.Agent, error)
	ListAgents(context.Context) ([]*models.Agent, error)
	ListAgentProfiles(context.Context, string) ([]*models.AgentProfile, error)
}

type AgentCatalog interface {
	Get(string) (agents.Agent, bool)
}

type Subject struct {
	Profile                 *models.AgentProfile
	BindingKey              string
	ProviderQualifiedModels bool
	Dynamic                 bool
}

type Mark struct {
	Key        string          `json:"-"`
	Scope      string          `json:"scope"`
	Model      string          `json:"model,omitempty"`
	Until      time.Time       `json:"until"`
	ResetKnown bool            `json:"reset_known"`
	Code       routingerr.Code `json:"-"`
	ProbeUntil time.Time       `json:"-"`
	Cleared    bool            `json:"-"`
}

func (m Mark) TrustedReset(now time.Time) bool {
	return !m.Cleared && m.ResetKnown && m.Until.After(now) && !m.Until.After(now.Add(MaximumAutomaticWait))
}

type Service struct {
	circuits *dynamic.CircuitRegistry
	bindings *dynamic.CredentialBindingResolver
	profiles ProfileRepository
	catalog  AgentCatalog
	now      func() time.Time
	onChange func(context.Context)
	logger   *zap.Logger
}

type Option func(*Service)

func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }
func WithOnChange(changed func(context.Context)) Option {
	return func(s *Service) { s.onChange = changed }
}
func WithLogger(logger *zap.Logger) Option { return func(s *Service) { s.logger = logger } }

func NewService(circuits *dynamic.CircuitRegistry, bindings *dynamic.CredentialBindingResolver, profiles ProfileRepository, catalog AgentCatalog, options ...Option) *Service {
	s := &Service{circuits: circuits, bindings: bindings, profiles: profiles, catalog: catalog, now: time.Now, logger: zap.NewNop()}
	for _, option := range options {
		option(s)
	}
	return s
}

func (s *Service) ForProfile(profile *models.AgentProfile, definition agents.Agent) Subject {
	if profile == nil {
		return Subject{}
	}
	billing := profile.BillingType
	if billing == "" && definition != nil {
		billing = string(definition.BillingType())
	}
	subject := Subject{Profile: profile, BindingKey: s.bindings.Resolve(dynamic.ProfileCredentialBindingDescriptor(profile, billing), profile.ID), Dynamic: profile.AgentID == agents.DynamicAgentID || agents.IsVirtualAgent(definition)}
	if qualified, ok := definition.(agents.ProviderQualifiedModelAgent); ok {
		subject.ProviderQualifiedModels = qualified.SupportsProviderQualifiedModels()
	}
	return subject
}

func (s *Service) ResolveSubject(ctx context.Context, profileID string) (Subject, error) {
	if s.profiles == nil {
		return Subject{}, ErrSubjectUnavailable
	}
	profile, err := s.profiles.GetAgentProfile(ctx, profileID)
	if err != nil {
		return Subject{}, fmt.Errorf("load provider limit profile: %w", err)
	}
	if profile == nil {
		return Subject{}, ErrSubjectUnavailable
	}
	agent, err := s.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return Subject{}, fmt.Errorf("load provider limit agent: %w", err)
	}
	if agent == nil {
		return Subject{}, ErrSubjectUnavailable
	}
	var definition agents.Agent
	if s.catalog != nil {
		definition, _ = s.catalog.Get(agent.Name)
	}
	return s.ForProfile(profile, definition), nil
}

func (s *Service) Record(ctx context.Context, subject Subject, model string, failure *routingerr.Error, observedAt time.Time) (Mark, error) {
	if subject.Profile == nil || subject.Profile.ID == "" || subject.Dynamic || subject.BindingKey == "" || s.circuits == nil {
		return Mark{}, ErrSubjectUnavailable
	}
	if failure == nil || (failure.Code != routingerr.CodeRateLimited && failure.Code != routingerr.CodeQuotaLimited) {
		return Mark{}, ErrNotLimit
	}
	key, scope := markKeyFor(subject, model, failure.LimitScope)
	if observedAt.IsZero() {
		observedAt = s.now()
	}
	until, known := markUntil(failure, observedAt)
	if err := s.circuits.OpenDurable(ctx, key, until, failure.Code, known); err != nil {
		return Mark{}, err
	}
	mark, _ := s.Get(key)
	if mark.Scope != "" {
		scope = mark.Scope
	}
	ObserveMark(s.logger, scope, string(failure.Code))
	if s.onChange != nil {
		s.onChange(ctx)
	}
	return mark, nil
}

// markKeyFor selects the account-scoped key when the provider reported an
// account limit and the subject has an account resource, otherwise the model key.
func markKeyFor(subject Subject, model, limitScope string) (key, scope string) {
	accountKey, modelKey := resourceKeys(subject, model)
	if limitScope == ScopeAccount && accountKey != "" {
		return accountKey, ScopeAccount
	}
	return modelKey, ScopeModel
}

// markUntil uses the provider's reset hint when present, otherwise a bounded
// unknown-reset lifetime from observedAt.
func markUntil(failure *routingerr.Error, observedAt time.Time) (time.Time, bool) {
	if failure.ResetHint != nil && !failure.ResetHint.IsZero() {
		return *failure.ResetHint, true
	}
	return observedAt.Add(UnknownResetLifetime), false
}

func (s *Service) Lookup(subject Subject, model string) (Mark, bool) {
	if subject.Profile == nil || subject.Dynamic || subject.BindingKey == "" || s.circuits == nil {
		return Mark{}, false
	}
	accountKey, modelKey := resourceKeys(subject, model)
	now := s.now()
	var selected Mark
	for _, key := range []string{accountKey, modelKey} {
		mark, ok := s.Get(key)
		if !ok || mark.Cleared || !mark.Until.After(now) {
			continue
		}
		if selected.Key == "" || mark.Until.After(selected.Until) {
			selected = mark
		}
	}
	return selected, selected.Key != ""
}

func (s *Service) ClearOnSuccess(ctx context.Context, subject Subject, model string) (bool, error) {
	if subject.Profile == nil || subject.Dynamic || subject.BindingKey == "" || s.circuits == nil {
		return false, ErrSubjectUnavailable
	}
	accountKey, modelKey := resourceKeys(subject, model)
	keys := []string{modelKey}
	if accountKey != "" {
		keys = append(keys, accountKey)
	}
	changed := false
	for _, key := range keys {
		if mark, ok := s.Get(key); ok && !mark.Cleared {
			changed = true
			break
		}
	}
	if !changed {
		return false, nil
	}
	if err := s.circuits.CloseManyDurable(ctx, keys); err != nil {
		return false, err
	}
	if s.onChange != nil {
		s.onChange(ctx)
	}
	return true, nil
}

func resourceKeys(subject Subject, model string) (string, string) {
	prefix := keyPrefix + subject.BindingKey + "|"
	account := prefix + ScopeAccount
	if subject.ProviderQualifiedModels {
		provider, rest, ok := strings.Cut(model, "/")
		if !ok || strings.TrimSpace(provider) == "" || strings.TrimSpace(rest) == "" {
			account = ""
		} else {
			account += "|" + provider
		}
	}
	return account, prefix + ScopeModel + "|" + model
}
