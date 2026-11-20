package api

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/sessioncatalog"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type catalogReadAdapter struct {
	bareAgentAdapter
	onRead func()
	source uint64
}

func (a *catalogReadAdapter) SessionCatalogSourceGeneration() uint64 {
	return a.source
}

func (a *catalogReadAdapter) GetSessionCatalogDiscovery() *sessioncatalog.Discovery {
	if a.onRead != nil {
		a.onRead()
	}
	return &sessioncatalog.Discovery{
		Outcome: sessioncatalog.OutcomeComplete,
		Models:  sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{{ID: "retired-model", Name: "Retired model"}}},
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-002.4
func TestSessionCatalogDiscoveryRejectsProcessReplacement(t *testing.T) {
	for _, sameAdapter := range []bool{false, true} {
		name := "replacement_adapter"
		if sameAdapter {
			name = "retained_adapter"
		}
		t.Run(name, func(t *testing.T) {
			s := newTestServer(t)
			generation := s.procMgr.AdvanceProcessGenerationForTest()
			a := &catalogReadAdapter{bareAgentAdapter: bareAgentAdapter{sessionID: "same-native-id"}, source: 1}
			s.procMgr.SetAdapterForTest(a)
			a.onRead = func() {
				s.procMgr.AdvanceProcessGenerationForTest()
				if !sameAdapter {
					s.procMgr.SetAdapterForTest(&catalogReadAdapter{bareAgentAdapter: bareAgentAdapter{sessionID: "same-native-id"}})
				}
			}
			msg, err := ws.NewRequest("catalog", "agent.session.catalog_discovery", map[string]any{
				"session_id": "same-native-id", "process_generation": generation,
			})
			if err != nil {
				t.Fatal(err)
			}
			payload := errorPayload(t, s.handleAgentStreamRequest(context.Background(), msg))
			if payload.Code != ws.ErrorCodeConflict {
				t.Fatalf("retired process result: code %q, want conflict", payload.Code)
			}
		})
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-002.4
func TestSessionCatalogDiscoveryRejectsSameProcessReload(t *testing.T) {
	s := newTestServer(t)
	generation := s.procMgr.AdvanceProcessGenerationForTest()
	a := &catalogReadAdapter{bareAgentAdapter: bareAgentAdapter{sessionID: "same-native-id"}, source: 1}
	s.procMgr.SetAdapterForTest(a)
	a.onRead = func() { a.source++ }
	msg, err := ws.NewRequest("catalog", "agent.session.catalog_discovery", map[string]any{
		"session_id": "same-native-id", "process_generation": generation,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := errorPayload(t, s.handleAgentStreamRequest(context.Background(), msg))
	if payload.Code != ws.ErrorCodeConflict {
		t.Fatalf("retired session source: code %q, want conflict", payload.Code)
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-002.4
func TestSessionCatalogDiscoveryRejectsUnknownOwnerEvidence(t *testing.T) {
	for _, missing := range []string{"process", "source"} {
		t.Run(missing, func(t *testing.T) {
			s := newTestServer(t)
			generation := s.procMgr.AdvanceProcessGenerationForTest()
			source := uint64(1)
			if missing == "process" {
				generation = 0
			} else {
				source = 0
			}
			s.procMgr.SetAdapterForTest(&catalogReadAdapter{
				bareAgentAdapter: bareAgentAdapter{sessionID: "same-native-id"}, source: source,
			})
			msg, err := ws.NewRequest("catalog", "agent.session.catalog_discovery", map[string]any{
				"session_id": "same-native-id", "process_generation": generation,
			})
			if err != nil {
				t.Fatal(err)
			}
			payload := errorPayload(t, s.handleAgentStreamRequest(context.Background(), msg))
			if payload.Code != ws.ErrorCodeConflict {
				t.Fatalf("unknown %s owner: code %q, want conflict", missing, payload.Code)
			}
		})
	}
}
