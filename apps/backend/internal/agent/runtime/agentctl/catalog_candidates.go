package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/kandev/kandev/internal/common/sessioncatalog"
)

// catalogSessionSource binds an adapter handshake epoch to its native session
// and the process generation that produced it.
type catalogSessionSource struct {
	sessionID         string
	processGeneration uint64
	source            uint64
}

func (c *Client) recordSessionSource(sessionID string, source uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionSource = catalogSessionSource{sessionID: sessionID, processGeneration: c.processGeneration, source: source}
}

// CatalogSessionSource returns the accepted handshake epoch for sessionID in
// the current process generation, or zero when that evidence is absent.
func (c *Client) CatalogSessionSource(sessionID string) (processGeneration, source uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	recorded := c.sessionSource
	if sessionID == "" || recorded.sessionID != sessionID || recorded.processGeneration != c.processGeneration ||
		recorded.processGeneration == 0 || recorded.source == 0 {
		return 0, 0
	}
	return recorded.processGeneration, recorded.source
}

// CurrentProcessManagerID reads the manager lifetime from the authenticated
// current instance descriptor, never from a discovery reply.
func (c *Client) CurrentProcessManagerID(ctx context.Context) (string, error) {
	status, err := c.GetDeliveryStatus(ctx, "")
	if err != nil {
		return "", errors.Join(sessioncatalog.ErrTaskProbeUnavailable, err)
	}
	if status == nil || status.ProcessManagerID == "" {
		return "", sessioncatalog.ErrTaskProbeUnavailable
	}
	return status.ProcessManagerID, nil
}

// DiscoverCatalogCandidates asks this authenticated instance for one fresh
// task-context catalog observation. The request names only the expected
// owner; the reply must echo this instance and that owner exactly.
func (c *Client) DiscoverCatalogCandidates(ctx context.Context, request sessioncatalog.TaskProbeRequest) (sessioncatalog.TaskProbeReply, error) {
	var reply sessioncatalog.TaskProbeReply
	if err := request.Validate(); err != nil {
		return reply, err
	}
	if c.executionID == "" {
		return reply, sessioncatalog.ErrTaskProbeUnavailable
	}
	body, err := json.Marshal(request)
	if err != nil {
		return reply, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+sessioncatalog.TaskProbePath, bytes.NewReader(body))
	if err != nil {
		return reply, sessioncatalog.ErrTaskProbeUnavailable
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Instance-ID", c.executionID)
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return reply, ctxErr
		}
		return reply, sessioncatalog.ErrTaskProbeUnavailable
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, sessioncatalog.MaxTaskProbeReplyBytes+1))
	if err != nil || len(data) > sessioncatalog.MaxTaskProbeReplyBytes {
		return reply, sessioncatalog.ErrTaskProbeUnavailable
	}
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusConflict:
		return reply, sessioncatalog.ErrTaskProbeOwner
	case http.StatusTooManyRequests:
		return reply, sessioncatalog.ErrTaskProbeBusy
	default:
		return reply, sessioncatalog.ErrTaskProbeUnavailable
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		return sessioncatalog.TaskProbeReply{}, sessioncatalog.ErrTaskProbeResult
	}
	if err := sessioncatalog.ValidateTaskProbeReply(request, c.executionID, reply); err != nil {
		return sessioncatalog.TaskProbeReply{}, err
	}
	return reply, nil
}

// browserProxyTransport is the transport handed to browser-facing proxies.
// Those proxies inject this instance's credentials, so backend-only actions
// are refused here before any request reaches agentctl.
type browserProxyTransport struct {
	base http.RoundTripper
}

func (t browserProxyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if backendOnlyAgentctlAction(request.URL) {
		if request.Body != nil {
			_ = request.Body.Close()
		}
		return &http.Response{
			StatusCode: http.StatusForbidden, Status: http.StatusText(http.StatusForbidden),
			Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
			Header: http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
			Body:   io.NopCloser(strings.NewReader("forbidden\n")), Request: request,
		}, nil
	}
	return t.base.RoundTrip(request)
}

func backendOnlyAgentctlAction(target *url.URL) bool {
	if target == nil {
		return false
	}
	candidates := []string{target.Path, target.RawPath}
	if decoded, err := url.PathUnescape(target.EscapedPath()); err == nil {
		candidates = append(candidates, decoded)
	}
	protected := strings.ToLower(sessioncatalog.TaskProbePath)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		cleaned := strings.ToLower(path.Clean("/" + strings.ReplaceAll(candidate, `\`, "/")))
		if cleaned == protected || strings.HasPrefix(cleaned, protected+"/") {
			return true
		}
	}
	return false
}
