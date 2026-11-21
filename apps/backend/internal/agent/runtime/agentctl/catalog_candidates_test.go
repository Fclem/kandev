package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
)

func newCatalogCandidatesClient(t *testing.T, handler http.HandlerFunc, opts ...ClientOption) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	return newClient(server.URL, log, opts...)
}

func catalogProbeRequest() sessioncatalog.TaskProbeRequest {
	return sessioncatalog.TaskProbeRequest{RequestID: "r1", ManagerID: "manager", ProcessGeneration: 3, SessionID: "thread", SessionSource: 2}
}

func catalogProbeReply(request sessioncatalog.TaskProbeRequest, instance string) sessioncatalog.TaskProbeReply {
	return sessioncatalog.TaskProbeReply{
		RequestID: request.RequestID, Capability: "fixture",
		Owner: sessioncatalog.TaskProbeOwner{
			InstanceID: instance, ManagerID: request.ManagerID, ProcessGeneration: request.ProcessGeneration,
			SessionID: request.SessionID, SessionSource: request.SessionSource,
		},
		Result: sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeComplete, Models: sessioncatalog.Surface{
			Present: true, Supported: true, Choices: []sessioncatalog.Choice{{ID: "fresh", Name: "Fresh"}},
		}},
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogTaskDiscoveryClientBindsOwner(t *testing.T) {
	request := catalogProbeRequest()
	var observed atomic.Value
	client := newCatalogCandidatesClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		observed.Store([]string{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-Instance-ID"), string(body)})
		_ = json.NewEncoder(w).Encode(catalogProbeReply(request, "instance-1"))
	}, WithAuthToken("control"), WithExecutionID("instance-1"))
	reply, err := client.DiscoverCatalogCandidates(context.Background(), request)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if reply.Result.Models.Choices[0].ID != "fresh" {
		t.Fatalf("reply = %+v", reply)
	}
	got := observed.Load().([]string)
	want, _ := json.Marshal(request)
	if got[0] != http.MethodPost || got[1] != sessioncatalog.TaskProbePath || got[2] != "Bearer control" || got[3] != "instance-1" || got[4] != string(want) {
		t.Fatalf("request = %q, want authenticated owner-only POST", got)
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogTaskDiscoveryClientRejectsUnprovenReplies(t *testing.T) {
	request := catalogProbeRequest()
	cases := map[string]struct {
		status int
		reply  func() any
		want   error
	}{
		"other instance":       {status: 200, reply: func() any { return catalogProbeReply(request, "instance-2") }, want: sessioncatalog.ErrTaskProbeOwner},
		"other manager":        {status: 200, reply: func() any { r := catalogProbeReply(request, "instance-1"); r.Owner.ManagerID = "x"; return r }, want: sessioncatalog.ErrTaskProbeOwner},
		"other process":        {status: 200, reply: func() any { r := catalogProbeReply(request, "instance-1"); r.Owner.ProcessGeneration++; return r }, want: sessioncatalog.ErrTaskProbeOwner},
		"other session source": {status: 200, reply: func() any { r := catalogProbeReply(request, "instance-1"); r.Owner.SessionSource++; return r }, want: sessioncatalog.ErrTaskProbeOwner},
		"other request":        {status: 200, reply: func() any { r := catalogProbeReply(request, "instance-1"); r.RequestID = "r2"; return r }, want: sessioncatalog.ErrTaskProbeOwner},
		"uncapable complete":   {status: 200, reply: func() any { r := catalogProbeReply(request, "instance-1"); r.Capability = ""; return r }, want: sessioncatalog.ErrTaskProbeResult},
		"choices without support": {status: 200, reply: func() any {
			r := catalogProbeReply(request, "instance-1")
			r.Result.Outcome = sessioncatalog.OutcomeFailed
			return r
		}, want: sessioncatalog.ErrTaskProbeResult},
		"owner conflict":  {status: http.StatusConflict, want: sessioncatalog.ErrTaskProbeOwner},
		"busy":            {status: http.StatusTooManyRequests, want: sessioncatalog.ErrTaskProbeBusy},
		"unauthenticated": {status: http.StatusUnauthorized, want: sessioncatalog.ErrTaskProbeUnavailable},
		"oversized":       {status: 200, reply: func() any { return strings.Repeat("x", sessioncatalog.MaxTaskProbeReplyBytes+1) }, want: sessioncatalog.ErrTaskProbeUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			client := newCatalogCandidatesClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				if tc.reply != nil {
					_ = json.NewEncoder(w).Encode(tc.reply())
				}
			}, WithAuthToken("control"), WithExecutionID("instance-1"))
			if _, err := client.DiscoverCatalogCandidates(context.Background(), request); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
	t.Run("missing instance identity", func(t *testing.T) {
		var calls atomic.Int32
		client := newCatalogCandidatesClient(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) }, WithAuthToken("control"))
		if _, err := client.DiscoverCatalogCandidates(context.Background(), request); !errors.Is(err, sessioncatalog.ErrTaskProbeUnavailable) {
			t.Fatalf("error = %v, want unavailable", err)
		}
		if calls.Load() != 0 {
			t.Fatal("a request without instance identity reached agentctl")
		}
	})
	t.Run("zero generation", func(t *testing.T) {
		zero := request
		zero.ProcessGeneration = 0
		client := newCatalogCandidatesClient(t, func(http.ResponseWriter, *http.Request) {}, WithAuthToken("control"), WithExecutionID("instance-1"))
		if _, err := client.DiscoverCatalogCandidates(context.Background(), zero); !errors.Is(err, sessioncatalog.ErrTaskProbeRequest) {
			t.Fatalf("error = %v, want request rejection", err)
		}
	})
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogTaskDiscoveryBrowserProxyDeniesAction(t *testing.T) {
	var reached atomic.Int32
	var authorization atomic.Value
	client := newCatalogCandidatesClient(t, func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		authorization.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}, WithAuthToken("control"), WithExecutionID("instance-1"))
	transport := client.ProxyTransport()
	for _, target := range []string{
		sessioncatalog.TaskProbePath,
		"/api/v1/port-proxy/3000/../../agent/catalog-candidates/discover",
		"/api/v1/agent/catalog-candidates/discover/",
		"/API/V1/agent/Catalog-Candidates/discover",
		"/api/v1//agent/catalog-candidates%2Fdiscover",
		"/api/v1/agent/catalog-candidates/discover/extra",
	} {
		request, err := http.NewRequest(http.MethodPost, client.BaseURL()+target, strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		response, err := transport.RoundTrip(request)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("%s: status %d, want forbidden", target, response.StatusCode)
		}
	}
	if reached.Load() != 0 {
		t.Fatal("a browser proxy request reached the backend-only action")
	}
	request, _ := http.NewRequest(http.MethodGet, client.BaseURL()+"/api/v1/port-proxy/3000/app", nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("ordinary proxy request: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || reached.Load() != 1 || authorization.Load() != "Bearer control" {
		t.Fatal("ordinary proxied requests must keep their authenticated transport")
	}
}
