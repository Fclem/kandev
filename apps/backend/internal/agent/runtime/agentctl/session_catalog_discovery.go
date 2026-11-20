package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/common/sessioncatalog"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// ReadSessionCatalogDiscovery observes the current adapter after settings settle;
// it does not initialize a session or mutate provider selection.
func (c *Client) ReadSessionCatalogDiscovery(ctx context.Context, nativeSessionID string) (*sessioncatalog.Discovery, error) {
	if nativeSessionID == "" {
		return nil, errors.New("session catalog source is required")
	}
	processGeneration := c.ProcessGeneration()
	response, err := c.sendStreamRequest(ctx, "agent.session.catalog_discovery", struct {
		SessionID         string `json:"session_id"`
		ProcessGeneration uint64 `json:"process_generation"`
	}{SessionID: nativeSessionID, ProcessGeneration: processGeneration})
	if err != nil {
		return nil, fmt.Errorf("read session catalog: %w", err)
	}
	if response.Type == ws.MessageTypeError {
		return nil, errors.New("session catalog observation failed")
	}
	var result struct {
		SessionID        string                    `json:"session_id"`
		CatalogDiscovery *sessioncatalog.Discovery `json:"catalog_discovery,omitempty"`
	}
	if err := response.ParsePayload(&result); err != nil {
		return nil, fmt.Errorf("decode session catalog: %w", err)
	}
	if c.ProcessGeneration() != processGeneration || result.SessionID != nativeSessionID {
		return nil, errors.New("session catalog source changed")
	}
	if result.CatalogDiscovery != nil {
		if err := sessioncatalog.ValidateDiscovery(result.CatalogDiscovery); err != nil {
			return nil, err
		}
	}
	return result.CatalogDiscovery, nil
}
