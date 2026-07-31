package plugins

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// InvokeWebhook routes an inbound webhook to id's live subprocess via the
// runtime manager's RemotePlugin.HandleWebhook RPC. Used by
// POST/GET /api/plugins/:id/webhooks/:key.
func (s *Service) InvokeWebhook(ctx context.Context, id string, req *pluginsdk.WebhookRequest) (*pluginsdk.WebhookResponse, error) {
	remote, ok := s.pluginRemote(id)
	if !ok {
		return nil, fmt.Errorf("plugins: plugin %q is not running", id)
	}
	return remote.HandleWebhook(ctx, req)
}

// InvokeAction routes an already-authenticated, host-verified browser action
// to id's live subprocess. HTTP authorization, body limits, and response
// filtering belong to the action handler; this method only owns RPC dispatch.
func (s *Service) InvokeAction(
	ctx context.Context, id string, req *pluginsdk.PluginActionRequest,
) (*pluginsdk.PluginActionResponse, error) {
	remote, ok := s.pluginRemote(id)
	if !ok {
		return nil, fmt.Errorf("plugins: plugin %q is not running", id)
	}
	return remote.HandleAction(ctx, req)
}

// pluginRemote returns the live RemotePlugin for id, if the runtime manager
// is wired and currently tracking a running process for it.
func (s *Service) pluginRemote(id string) (*pluginsdk.RemotePlugin, bool) {
	if s.runtime == nil {
		return nil, false
	}
	return s.runtime.Get(id)
}
