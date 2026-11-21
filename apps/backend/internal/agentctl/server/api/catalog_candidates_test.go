package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
)

const catalogCandidatesTestBody = `{"request_id":"r1","manager_id":"m1","process_generation":1,"session_id":"thread","session_source":1}`

func newCatalogCandidatesServer(t *testing.T, token string) *Server {
	t.Helper()
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AuthToken: token, InstanceID: "instance-1"}
	return NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogTaskDiscoveryAPIAuthorization(t *testing.T) {
	cases := []struct {
		name     string
		token    string
		auth     string
		instance string
		body     string
		want     int
	}{
		{name: "missing credential", token: "control", instance: "instance-1", body: catalogCandidatesTestBody, want: http.StatusUnauthorized},
		{name: "wrong credential", token: "control", auth: "Bearer other", instance: "instance-1", body: catalogCandidatesTestBody, want: http.StatusUnauthorized},
		{name: "auth-disabled instance", instance: "instance-1", body: catalogCandidatesTestBody, want: http.StatusForbidden},
		{name: "missing instance identity", token: "control", auth: "Bearer control", body: catalogCandidatesTestBody, want: http.StatusForbidden},
		{name: "foreign instance identity", token: "control", auth: "Bearer control", instance: "instance-2", body: catalogCandidatesTestBody, want: http.StatusNotFound},
		{name: "caller-supplied launch input", token: "control", auth: "Bearer control", instance: "instance-1", body: `{"request_id":"r1","manager_id":"m1","process_generation":1,"session_id":"thread","session_source":1,"env":{"HOME":"/tmp"}}`, want: http.StatusBadRequest},
		{name: "zero generation wildcard", token: "control", auth: "Bearer control", instance: "instance-1", body: `{"request_id":"r1","manager_id":"m1","process_generation":0,"session_id":"thread","session_source":1}`, want: http.StatusBadRequest},
		{name: "unproven owner", token: "control", auth: "Bearer control", instance: "instance-1", body: catalogCandidatesTestBody, want: http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newCatalogCandidatesServer(t, tc.token)
			request := httptest.NewRequest(http.MethodPost, sessioncatalog.TaskProbePath, strings.NewReader(tc.body))
			if tc.auth != "" {
				request.Header.Set("Authorization", tc.auth)
			}
			if tc.instance != "" {
				request.Header.Set(InstanceIDHeader, tc.instance)
			}
			recorder := httptest.NewRecorder()
			server.Router().ServeHTTP(recorder, request)
			if recorder.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, tc.want, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "HOME") {
				t.Fatal("refusal echoed caller-supplied launch input")
			}
		})
	}
}
