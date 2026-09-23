package github

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestRepositoryStarControllerRequiresWorkspaceAndReturnsConfirmedState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := &personalStarClientStub{}
	service := NewService(nil, AuthMethodNone, nil, nil, nil, nil)
	service.SetPersonalStarResolver(&personalStarResolverStub{
		status: PersonalStarConnectionStatus{Available: true, Active: true, Capable: true},
		client: client,
	})
	router := gin.New()
	NewController(service, nil).RegisterHTTPRoutes(router)

	missingWorkspace := httptest.NewRecorder()
	router.ServeHTTP(missingWorkspace, httptest.NewRequest(
		http.MethodPut, "/api/v1/github/user/starred", bytes.NewBufferString(`{"repository_url":"https://github.com/owner/repository"}`),
	))
	if missingWorkspace.Code != http.StatusBadRequest {
		t.Fatalf("missing workspace status = %d, body = %s", missingWorkspace.Code, missingWorkspace.Body.String())
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(
		http.MethodPut, "/api/v1/github/user/starred", bytes.NewBufferString(`{"workspace_id":"workspace-1","repository_url":"https://github.com/owner/repository"}`),
	))
	if response.Code != http.StatusOK || response.Body.String() != `{"starred":true}` {
		t.Fatalf("star response = %d %s", response.Code, response.Body.String())
	}

	invalidURL := httptest.NewRecorder()
	router.ServeHTTP(invalidURL, httptest.NewRequest(
		http.MethodPut, "/api/v1/github/user/starred", bytes.NewBufferString(`{"workspace_id":"workspace-1","repository_url":"https://github.com/owner/repository/extra"}`),
	))
	if invalidURL.Code != http.StatusBadRequest {
		t.Fatalf("invalid URL status = %d, body = %s", invalidURL.Code, invalidURL.Body.String())
	}
	if client.calls != 1 {
		t.Fatalf("provider calls = %d, want one confirmed mutation only", client.calls)
	}
}

func TestRepositoryStarStatusControllerRejectsOversizeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService(nil, AuthMethodNone, nil, nil, nil, nil)
	router := gin.New()
	NewController(service, nil).RegisterHTTPRoutes(router)

	response := httptest.NewRecorder()
	body := bytes.Repeat([]byte("x"), maxRepositoryStarStatusBodyBytes+1)
	router.ServeHTTP(response, httptest.NewRequest(
		http.MethodPost, "/api/v1/github/user/starred/status", bytes.NewReader(body),
	))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversize status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestRepositoryStarControllerRejectsForeignWorkspaceBeforeProviderCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := &personalStarClientStub{}
	service := NewService(nil, AuthMethodNone, nil, nil, nil, nil)
	service.SetWorkspaceAuthorizer(func(context.Context, string) error {
		return repoerrors.ErrWorkspaceNotFound
	})
	service.SetPersonalStarResolver(&personalStarResolverStub{
		status: PersonalStarConnectionStatus{Available: true, Active: true, Capable: true},
		client: client,
	})
	router := gin.New()
	NewController(service, nil).RegisterHTTPRoutes(router)

	for name, request := range map[string]*http.Request{
		"connection status": httptest.NewRequest(
			http.MethodGet, "/api/v1/github/user/starred/connection?workspace_id=foreign", nil),
		"repository status": httptest.NewRequest(
			http.MethodPost, "/api/v1/github/user/starred/status",
			bytes.NewBufferString(`{"workspace_id":"foreign","repository_urls":["https://github.com/owner/repository"]}`)),
		"star": httptest.NewRequest(
			http.MethodPut, "/api/v1/github/user/starred",
			bytes.NewBufferString(`{"workspace_id":"foreign","repository_url":"https://github.com/owner/repository"}`)),
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, body = %s", name, response.Code, response.Body.String())
		}
	}
	if client.calls != 0 {
		t.Fatalf("provider calls = %d, want none for a foreign workspace", client.calls)
	}
}
