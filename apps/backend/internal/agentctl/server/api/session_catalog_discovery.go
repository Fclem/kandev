package api

import (
	"context"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func sessionCatalogDiscovery(agentAdapter adapter.AgentAdapter) *sessioncatalog.Discovery {
	provider, ok := agentAdapter.(interface {
		GetSessionCatalogDiscovery() *sessioncatalog.Discovery
	})
	if !ok {
		return nil
	}
	return provider.GetSessionCatalogDiscovery()
}

func sessionCatalogSourceGeneration(agentAdapter adapter.AgentAdapter) uint64 {
	provider, ok := agentAdapter.(interface{ SessionCatalogSourceGeneration() uint64 })
	if !ok {
		return 0
	}
	return provider.SessionCatalogSourceGeneration()
}

func (s *Server) handleWSSessionCatalogDiscovery(_ context.Context, msg *ws.Message) *ws.Message {
	var request struct {
		SessionID         string `json:"session_id"`
		ProcessGeneration uint64 `json:"process_generation"`
	}
	if err := msg.ParsePayload(&request); err != nil || request.SessionID == "" {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "session catalog source is required", nil)
		return response
	}
	agentAdapter, current := s.procMgr.GetAdapterForGeneration(request.ProcessGeneration)
	if request.ProcessGeneration == 0 || !current || agentAdapter == nil || agentAdapter.GetSessionID() != request.SessionID {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "session catalog source changed", nil)
		return response
	}
	source := sessionCatalogSourceGeneration(agentAdapter)
	if source == 0 {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "session catalog source is unknown", nil)
		return response
	}
	discovery := sessionCatalogDiscovery(agentAdapter)
	if discovery != nil {
		if err := sessioncatalog.ValidateDiscovery(discovery); err != nil {
			response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "invalid session catalog result", nil)
			return response
		}
	}
	currentAdapter, current := s.procMgr.GetAdapterForGeneration(request.ProcessGeneration)
	if !current || currentAdapter != agentAdapter || agentAdapter.GetSessionID() != request.SessionID ||
		sessionCatalogSourceGeneration(agentAdapter) != source {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "session catalog source changed", nil)
		return response
	}
	response, _ := ws.NewResponse(msg.ID, msg.Action, struct {
		SessionID        string                    `json:"session_id"`
		CatalogDiscovery *sessioncatalog.Discovery `json:"catalog_discovery,omitempty"`
	}{SessionID: request.SessionID, CatalogDiscovery: discovery})
	return response
}
