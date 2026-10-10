package providerlimit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	models "github.com/kandev/kandev/internal/agent/settings/models"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.2
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.4
func TestProviderLimitBindingAndModelScopes(t *testing.T) {
	svc, _, now := newLimitFixture()
	one := svc.ForProfile(&models.AgentProfile{ID: "one", AgentID: "saved-claude", BillingType: "api_key"}, nil)
	same := svc.ForProfile(&models.AgentProfile{ID: "two", AgentID: "saved-claude", BillingType: "api_key"}, nil)
	other := svc.ForProfile(&models.AgentProfile{ID: "other", AgentID: "saved-claude", BillingType: "subscription"}, nil)
	reset := now.Add(time.Hour)
	mark, err := svc.Record(context.Background(), one, "opus", &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account", ResetHint: &reset}, *now)
	if err != nil {
		t.Fatal(err)
	}
	if got, limited := svc.Lookup(same, "sonnet"); !limited || got.Key != mark.Key || got.Scope != "account" {
		t.Fatalf("shared account not limited: %+v limited=%t", got, limited)
	}
	if _, limited := svc.Lookup(other, "opus"); limited {
		t.Fatal("a different credential binding inherited the mark")
	}
	unknownOne := svc.ForProfile(&models.AgentProfile{ID: "unknown-one", AgentID: "saved-claude"}, nil)
	unknownTwo := svc.ForProfile(&models.AgentProfile{ID: "unknown-two", AgentID: "saved-claude"}, nil)
	if _, err := svc.Record(context.Background(), unknownOne, "opus", &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account"}, *now); err != nil {
		t.Fatal(err)
	}
	if _, limited := svc.Lookup(unknownTwo, "opus"); limited {
		t.Fatal("unprovable bindings shared an account mark")
	}
	if _, err := svc.ClearOnSuccess(context.Background(), one, "opus"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Record(context.Background(), one, "opus", &routingerr.Error{Code: routingerr.CodeRateLimited}, *now); err != nil {
		t.Fatal(err)
	}
	if got, limited := svc.Lookup(same, "opus"); !limited || got.Scope != "model" {
		t.Fatalf("shared model not limited: %+v limited=%t", got, limited)
	}
	if _, limited := svc.Lookup(same, "sonnet"); limited {
		t.Fatal("a model mark limited another model")
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.9
func TestProviderLimitAccountProviderDomainsUseAgentCapability(t *testing.T) {
	for _, definition := range []agents.Agent{agents.NewOmpACP(), agents.NewOpenCodeACP()} {
		t.Run(definition.ID(), func(t *testing.T) {
			svc, _, now := newLimitFixture()
			subject := svc.ForProfile(&models.AgentProfile{ID: "multi", AgentID: "saved-multi", BillingType: "api_key"}, definition)
			mark, err := svc.Record(context.Background(), subject, "anthropic/opus", &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account"}, *now)
			if err != nil {
				t.Fatal(err)
			}
			if mark.Scope != "account" {
				t.Fatalf("recognized provider account scope=%s", mark.Scope)
			}
			if _, limited := svc.Lookup(subject, "anthropic/sonnet"); !limited {
				t.Fatal("same provider escaped the account mark")
			}
			if _, limited := svc.Lookup(subject, "openai-codex/gpt-6.1-sol"); limited {
				t.Fatal("another provider inherited the account mark")
			}
			unknown, err := svc.Record(context.Background(), subject, "unqualified", &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account"}, *now)
			if err != nil {
				t.Fatal(err)
			}
			if unknown.Scope != "model" {
				t.Fatalf("unknown provider prefix acquired account scope: %+v", unknown)
			}
			if _, limited := svc.Lookup(subject, "another-unqualified"); limited {
				t.Fatal("unprovable provider domain limited other models")
			}
		})
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.3
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
func TestProviderLimitPersistenceExpiryKnowledgeAndDurablePublication(t *testing.T) {
	svc, persist, now := newLimitFixture()
	subject := svc.ForProfile(&models.AgentProfile{ID: "one"}, nil)
	failure := &routingerr.Error{Code: routingerr.CodeRateLimited, RawExcerpt: "private provider diagnostic req_never-persist"}
	persist.fail = true
	if _, err := svc.Record(context.Background(), subject, "opus", failure, *now); err == nil {
		t.Fatal("failed mark write accepted")
	}
	if _, limited := svc.Lookup(subject, "opus"); limited {
		t.Fatal("unpersisted mark authorized recovery")
	}
	persist.fail = false
	mark, err := svc.Record(context.Background(), subject, "opus", failure, *now)
	if err != nil {
		t.Fatal(err)
	}
	if mark.ResetKnown || !mark.Until.Equal(now.Add(30*time.Minute)) || mark.TrustedReset(*now) {
		t.Fatalf("unknown reset acquired trust or wrong expiry: %+v", mark)
	}
	reset := now.Add(8 * 24 * time.Hour)
	failure.ResetHint = &reset
	mark, err = svc.Record(context.Background(), subject, "opus", failure, *now)
	if err != nil {
		t.Fatal(err)
	}
	if !mark.ResetKnown || !mark.Until.Equal(reset) || mark.TrustedReset(*now) {
		t.Fatalf("eight-day reset truncated or trusted: %+v", mark)
	}
	earlier := now.Add(time.Hour)
	failure.ResetHint = &earlier
	if _, err := svc.Record(context.Background(), subject, "opus", failure, *now); err != nil {
		t.Fatal(err)
	}
	mark, limited := svc.Lookup(subject, "opus")
	if !limited || !mark.Until.Equal(reset) {
		t.Fatalf("earlier observation shortened accepted expiry: %+v", mark)
	}
	encoded, err := json.Marshal(persist.rows)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private provider diagnostic") || strings.Contains(string(encoded), "req_never-persist") {
		t.Fatalf("provider text persisted: %s", encoded)
	}
	registry := dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(persist), dynamic.WithCircuitClock(func() time.Time { return *now }))
	if err := registry.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	restarted := NewService(registry, dynamic.NewCredentialBindingResolver([]byte("limit-test-installation")), nil, nil, WithClock(func() time.Time { return *now }))
	restoredSubject := restarted.ForProfile(&models.AgentProfile{ID: "one"}, nil)
	restored, active := restarted.Lookup(restoredSubject, "opus")
	if !active || restored.Key != mark.Key || !restored.ResetKnown || !restored.Until.Equal(reset) || restored.TrustedReset(*now) {
		t.Fatalf("restart changed the accepted long-reset mark: %+v (active %v)", restored, active)
	}
	*now = reset
	if _, limited := svc.Lookup(subject, "opus"); limited {
		t.Fatal("expired mark still limits a model")
	}
	if marks := svc.List(); len(marks) != 0 {
		t.Fatalf("expired marks remain visible: %+v", marks)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.5
func TestProviderLimitSuccessClearIsAtomicAndFailureBlocksBothScopes(t *testing.T) {
	svc, persist, now := newLimitFixture()
	subject := svc.ForProfile(&models.AgentProfile{ID: "one", AgentID: "saved-claude", BillingType: "api_key"}, nil)
	account, err := svc.Record(context.Background(), subject, "opus", &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account"}, *now)
	if err != nil {
		t.Fatal(err)
	}
	model, err := svc.Record(context.Background(), subject, "opus", &routingerr.Error{Code: routingerr.CodeRateLimited}, *now)
	if err != nil {
		t.Fatal(err)
	}
	persist.fail = true
	if _, err := svc.ClearOnSuccess(context.Background(), subject, "opus"); err == nil {
		t.Fatal("failed clear succeeded")
	}
	for _, key := range []string{account.Key, model.Key} {
		if _, active := svc.Get(key); !active {
			t.Fatalf("failed clear released %s", key)
		}
	}
	persist.fail = false
	if _, err := svc.ClearOnSuccess(context.Background(), subject, "opus"); err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"opus", "sonnet"} {
		if _, limited := svc.Lookup(subject, model); limited {
			t.Fatalf("successful atomic clear still limits %s", model)
		}
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.1
func TestProviderLimitDynamicProfileNeverMarksConcreteBindings(t *testing.T) {
	svc, _, now := newLimitFixture()
	subject := svc.ForProfile(&models.AgentProfile{ID: "dynamic", AgentID: agents.DynamicAgentID, BillingType: "api_key"}, agents.NewDynamicAgent())
	if _, err := svc.Record(context.Background(), subject, "opus", &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account"}, *now); err == nil {
		t.Fatal("dynamic profile admitted a concrete limit mark")
	}
	concrete := svc.ForProfile(&models.AgentProfile{ID: "concrete", AgentID: "family", BillingType: "api_key"}, nil)
	if _, limited := svc.Lookup(concrete, "opus"); limited {
		t.Fatal("dynamic failure marked a concrete binding")
	}
}

func newLimitFixture() (*Service, *limitPersistence, *time.Time) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	persist := &limitPersistence{rows: make(map[string]dynamic.CircuitSnapshot)}
	registry := dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(persist), dynamic.WithCircuitClock(func() time.Time { return now }))
	return NewService(registry, dynamic.NewCredentialBindingResolver([]byte("limit-test-installation")), nil, nil, WithClock(func() time.Time { return now })), persist, &now
}

type limitPersistence struct {
	rows map[string]dynamic.CircuitSnapshot
	fail bool
}

func (p *limitPersistence) SaveCircuit(_ context.Context, snapshot dynamic.CircuitSnapshot) error {
	if p.fail {
		return errors.New("persist rejected")
	}
	p.rows[snapshot.Key] = snapshot
	return nil
}
func (p *limitPersistence) SaveCircuits(_ context.Context, snapshots []dynamic.CircuitSnapshot) error {
	if p.fail {
		return errors.New("batch rejected")
	}
	for _, snapshot := range snapshots {
		p.rows[snapshot.Key] = snapshot
	}
	return nil
}
func (p *limitPersistence) LoadCircuits(context.Context) ([]dynamic.CircuitSnapshot, error) {
	var rows []dynamic.CircuitSnapshot
	for _, snapshot := range p.rows {
		if snapshot.State != dynamic.CircuitClosed || !snapshot.ProbeUntil.IsZero() {
			rows = append(rows, snapshot)
		}
	}
	return rows, nil
}
