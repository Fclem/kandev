package providerlimit

import (
	"context"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
)

func (s *Service) Get(key string) (Mark, bool) {
	if s.circuits == nil || !strings.HasPrefix(key, keyPrefix) {
		return Mark{}, false
	}
	snapshot, exists := s.circuits.Get(key)
	if !exists || (snapshot.State == dynamic.CircuitClosed && snapshot.ProbeUntil.IsZero()) {
		return Mark{}, false
	}
	return markFromSnapshot(snapshot)
}

func (s *Service) List() []Mark {
	if s.circuits == nil {
		return nil
	}
	now := s.now()
	var marks []Mark
	for _, snapshot := range s.circuits.List(keyPrefix) {
		mark, ok := markFromSnapshot(snapshot)
		if ok && !mark.Cleared && mark.Until.After(now) {
			marks = append(marks, mark)
		}
	}
	return marks
}

func (s *Service) AcquireProbe(ctx context.Context, key string) (dynamic.ProbeLease, bool, error) {
	if _, exists := s.Get(key); !exists {
		return dynamic.ProbeLease{}, false, ErrSubjectUnavailable
	}
	return s.circuits.AcquireProbeDurable(ctx, key, ProbeLifetime)
}

func (s *Service) ReleaseProbe(ctx context.Context, lease dynamic.ProbeLease, success bool) (bool, error) {
	if s.circuits == nil || !strings.HasPrefix(lease.Key, keyPrefix) {
		return false, ErrSubjectUnavailable
	}
	released, err := s.circuits.ReleaseProbeDurable(ctx, lease, success, 0)
	if err == nil && released && success && s.onChange != nil {
		s.onChange(ctx)
	}
	return released, err
}

func markFromSnapshot(snapshot dynamic.CircuitSnapshot) (Mark, bool) {
	_, resource, ok := strings.Cut(strings.TrimPrefix(snapshot.Key, keyPrefix), "|")
	if !ok {
		return Mark{}, false
	}
	scope, detail, _ := strings.Cut(resource, "|")
	if scope != ScopeAccount && scope != ScopeModel {
		return Mark{}, false
	}
	model := ""
	if scope == ScopeModel {
		model = detail
	}
	return Mark{Key: snapshot.Key, Scope: scope, Model: model, Until: snapshot.Until, ResetKnown: snapshot.ResetKnown, Code: snapshot.Code, ProbeUntil: snapshot.ProbeUntil, Cleared: snapshot.State == dynamic.CircuitClosed}, true
}

// ProfileLimit is the public projection. Binding and ownership identities stay internal.
type ProfileLimit struct {
	ProfileID  string    `json:"profile_id"`
	Model      string    `json:"model"`
	Scope      string    `json:"scope"`
	Until      time.Time `json:"until"`
	ResetKnown bool      `json:"reset_known"`
}

func (s *Service) ListProfiles(ctx context.Context) ([]ProfileLimit, error) {
	limits := make([]ProfileLimit, 0)
	if s.profiles == nil {
		return nil, ErrSubjectUnavailable
	}
	agentRows, err := s.profiles.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	for _, agent := range agentRows {
		var definition agents.Agent
		if s.catalog != nil {
			definition, _ = s.catalog.Get(agent.Name)
		}
		profiles, err := s.profiles.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return nil, err
		}
		for _, profile := range profiles {
			if profile.DeletedAt != nil || profile.WorkspaceID != "" {
				continue
			}
			mark, limited := s.Lookup(s.ForProfile(profile, definition), profile.Model)
			if limited {
				limits = append(limits, ProfileLimit{ProfileID: profile.ID, Model: profile.Model, Scope: mark.Scope, Until: mark.Until, ResetKnown: mark.ResetKnown})
			}
		}
	}
	return limits, nil
}
