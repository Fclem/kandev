package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// newSecretsHandlerService builds a service wired like main.go: the authorizer
// and existence checker classify missing/unauthorized workspaces as
// ErrWorkspaceAccessDenied, and injected raw errors pass through unclassified.
func newSecretsHandlerService(t *testing.T, allowed map[string]bool, authorizerErr, existenceErr error) *Service {
	t.Helper()
	store := newTestSQLiteStore(t)
	svc := NewService(NewUserVisibleStore(store), nil)
	svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		if authorizerErr != nil {
			return authorizerErr
		}
		if allowed[workspaceID] {
			return nil
		}
		return ErrWorkspaceAccessDenied
	})
	svc.SetWorkspaceExistenceChecker(func(_ context.Context, workspaceID string) error {
		if existenceErr != nil {
			return existenceErr
		}
		if allowed[workspaceID] {
			return nil
		}
		return ErrWorkspaceAccessDenied
	})
	return svc
}

func newSecretsHTTPRouter(t *testing.T, svc *Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(svc, testLogger(t)).registerHTTP(router)
	return router
}

func doTransferHTTP(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeItem(t *testing.T, rec *httptest.ResponseRecorder) *SecretListItem {
	t.Helper()
	var item SecretListItem
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode item from %d %s: %v", rec.Code, rec.Body.String(), err)
	}
	return &item
}

func seedGlobalViaService(t *testing.T, svc *Service, name, value string) string {
	t.Helper()
	item := mustCreateViaService(t, svc, name, value, ScopeGlobal, "")
	return item.ID
}

func TestHandlerCopy_GlobalToWorkspaceCreated(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	router := newSecretsHTTPRouter(t, svc)
	id := seedGlobalViaService(t, svc, "API Key", "v")

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+id+"/copy",
		`{"target_scope":"workspace","target_workspace_id":"workspace-a","name":"copied"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	item := decodeItem(t, rec)
	if item.Scope != ScopeWorkspace || item.WorkspaceID != "workspace-a" || item.Name != "copied" {
		t.Fatalf("item = %+v", item)
	}
	if _, err := svc.Get(context.Background(), id); err != nil {
		t.Fatalf("source lost after copy: %v", err)
	}
}

func TestHandlerCopy_WorkspaceSourceRequiresWorkspaceID(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	router := newSecretsHTTPRouter(t, svc)
	w := mustCreateViaService(t, svc, "ws", "v", ScopeWorkspace, "workspace-a")

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+w.ID+"/copy",
		`{"target_scope":"global","name":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (workspace source without workspace_id)", rec.Code)
	}
}

func TestHandlerCopy_OmittedNameUsesSourceName(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	router := newSecretsHTTPRouter(t, svc)
	id := seedGlobalViaService(t, svc, "API Key", "v")

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+id+"/copy",
		`{"target_scope":"workspace","target_workspace_id":"workspace-a"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if item := decodeItem(t, rec); item.Name != "API Key" {
		t.Fatalf("name = %q, want source name", item.Name)
	}
}

func TestHandlerCopy_NullNameBadRequest(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	router := newSecretsHTTPRouter(t, svc)
	id := seedGlobalViaService(t, svc, "g", "v")

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+id+"/copy",
		`{"target_scope":"workspace","target_workspace_id":"workspace-a","name":null}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for null name", rec.Code)
	}
}

func TestHandlerMove_WorkspaceToGlobal(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	router := newSecretsHTTPRouter(t, svc)
	w := mustCreateViaService(t, svc, "ws", "v", ScopeWorkspace, "workspace-a")

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+w.ID+"/move?workspace_id=workspace-a",
		`{"target_scope":"global","name":"moved"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	item := decodeItem(t, rec)
	if item.Scope != ScopeGlobal || item.Name != "moved" {
		t.Fatalf("item = %+v", item)
	}
	if _, err := svc.GetWorkspaceSecret(context.Background(), w.ID, "workspace-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source still present after move: %v", err)
	}
}

func TestHandlerCopy_SameScopeBadRequest(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	router := newSecretsHTTPRouter(t, svc)
	id := seedGlobalViaService(t, svc, "g", "v")

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+id+"/copy",
		`{"target_scope":"global","name":"other"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for same-scope", rec.Code)
	}
}

func TestHandlerCopy_NameConflict(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	router := newSecretsHTTPRouter(t, svc)
	mustCreateViaService(t, svc, "taken", "other", ScopeWorkspace, "workspace-a")
	id := seedGlobalViaService(t, svc, "g", "v")

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+id+"/copy",
		`{"target_scope":"workspace","target_workspace_id":"workspace-a","name":"taken"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestHandlerCopy_MissingSourceNotFound(t *testing.T) {
	svc := newSecretsHandlerService(t, nil, nil, nil)
	router := newSecretsHTTPRouter(t, svc)
	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/missing/copy",
		`{"target_scope":"global","name":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandlerCopy_UnauthorizedSourceNotFound(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	router := newSecretsHTTPRouter(t, svc)
	w := mustCreateViaService(t, svc, "ws", "v", ScopeWorkspace, "workspace-a")
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return ErrWorkspaceAccessDenied })
	svc.SetWorkspaceExistenceChecker(func(context.Context, string) error { return ErrWorkspaceAccessDenied })

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+w.ID+"/copy?workspace_id=workspace-a",
		`{"target_scope":"global","name":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unauthorized source", rec.Code)
	}
}

func TestHandlerCopy_UnauthorizedDestinationNotFound(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	router := newSecretsHTTPRouter(t, svc)
	id := seedGlobalViaService(t, svc, "g", "v")

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+id+"/copy",
		`{"target_scope":"workspace","target_workspace_id":"workspace-b","name":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unauthorized destination", rec.Code)
	}
}

func TestHandlerCopy_NilExistenceCheckerNotFound(t *testing.T) {
	store := newTestSQLiteStore(t)
	svc := NewService(NewUserVisibleStore(store), nil)
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	router := newSecretsHTTPRouter(t, svc)
	id := seedGlobalViaService(t, svc, "g", "v")

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+id+"/copy",
		`{"target_scope":"workspace","target_workspace_id":"any","name":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (nil checker fails closed)", rec.Code)
	}
}

func TestHandlerCopy_InternalErrorSanitized(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, errors.New("database unreachable"), nil)
	router := newSecretsHTTPRouter(t, svc)
	id := seedGlobalViaService(t, svc, "g", "v")

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+id+"/copy",
		`{"target_scope":"workspace","target_workspace_id":"workspace-a","name":"x"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "database unreachable") {
		t.Fatalf("500 body leaked internal error: %s", rec.Body.String())
	}
}

func TestHandlerCopy_ValidationLikeInternalErrorStays500(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, errors.New("name must be 1-100 characters"), nil)
	router := newSecretsHTTPRouter(t, svc)
	id := seedGlobalViaService(t, svc, "g", "v")

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+id+"/copy",
		`{"target_scope":"workspace","target_workspace_id":"workspace-a","name":"x"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (classification is sentinel-based, not text-based)", rec.Code)
	}
}

func TestHandlerCopy_WhitespaceNameBadRequest(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	router := newSecretsHTTPRouter(t, svc)
	id := seedGlobalViaService(t, svc, "g", "v")

	rec := doTransferHTTP(t, router, http.MethodPost, "/api/v1/secrets/"+id+"/copy",
		`{"target_scope":"workspace","target_workspace_id":"workspace-a","name":"   "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for whitespace name", rec.Code)
	}
}

// --- WebSocket handlers ---

func newWSHandler(t *testing.T, svc *Service) *Handler {
	t.Helper()
	return NewHandler(svc, testLogger(t))
}

func wsTransfer(t *testing.T, h *Handler, action, payload string) (*ws.Message, error) {
	t.Helper()
	msg := &ws.Message{ID: "1", Action: action, Payload: json.RawMessage(payload)}
	switch action {
	case ws.ActionSecretCopy:
		return h.wsCopy(context.Background(), msg)
	case ws.ActionSecretMove:
		return h.wsMove(context.Background(), msg)
	}
	t.Fatalf("unknown action %s", action)
	return nil, nil
}

type wsErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeWSError(t *testing.T, resp *ws.Message) wsErrorPayload {
	t.Helper()
	var errPayload wsErrorPayload
	if err := json.Unmarshal(resp.Payload, &errPayload); err != nil {
		t.Fatalf("decode error payload %s: %v", string(resp.Payload), err)
	}
	return errPayload
}

func decodeWSItem(t *testing.T, resp *ws.Message) *SecretListItem {
	t.Helper()
	var item SecretListItem
	if err := json.Unmarshal(resp.Payload, &item); err != nil {
		t.Fatalf("decode item payload %s: %v", string(resp.Payload), err)
	}
	return &item
}

func TestWSHandler_CopySuccessAndShape(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	h := newWSHandler(t, svc)
	id := seedGlobalViaService(t, svc, "API Key", "v")

	resp, err := wsTransfer(t, h, ws.ActionSecretCopy,
		`{"id":"`+id+`","target_scope":"workspace","target_workspace_id":"workspace-a","name":"copied"}`)
	if err != nil {
		t.Fatalf("wsCopy: %v", err)
	}
	item := decodeWSItem(t, resp)
	if item.Scope != ScopeWorkspace || item.WorkspaceID != "workspace-a" || item.Name != "copied" {
		t.Fatalf("item = %+v", item)
	}
}

func TestWSHandler_CopyNullNameBadRequest(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	h := newWSHandler(t, svc)
	id := seedGlobalViaService(t, svc, "g", "v")

	resp, err := wsTransfer(t, h, ws.ActionSecretCopy,
		`{"id":"`+id+`","target_scope":"workspace","target_workspace_id":"workspace-a","name":null}`)
	if err != nil {
		t.Fatalf("wsCopy: %v", err)
	}
	if got := decodeWSError(t, resp); got.Code != ws.ErrorCodeBadRequest {
		t.Fatalf("code = %q, want BAD_REQUEST", got.Code)
	}
}

func TestWSHandler_CopyConflict(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	h := newWSHandler(t, svc)
	mustCreateViaService(t, svc, "taken", "other", ScopeWorkspace, "workspace-a")
	id := seedGlobalViaService(t, svc, "g", "v")

	resp, err := wsTransfer(t, h, ws.ActionSecretCopy,
		`{"id":"`+id+`","target_scope":"workspace","target_workspace_id":"workspace-a","name":"taken"}`)
	if err != nil {
		t.Fatalf("wsCopy: %v", err)
	}
	if got := decodeWSError(t, resp); got.Code != ws.ErrorCodeConflict {
		t.Fatalf("code = %q, want CONFLICT", got.Code)
	}
}

func TestWSHandler_CopyMissingSourceNotFound(t *testing.T) {
	svc := newSecretsHandlerService(t, nil, nil, nil)
	h := newWSHandler(t, svc)
	resp, err := wsTransfer(t, h, ws.ActionSecretCopy,
		`{"id":"missing","target_scope":"global","name":"x"}`)
	if err != nil {
		t.Fatalf("wsCopy: %v", err)
	}
	if got := decodeWSError(t, resp); got.Code != ws.ErrorCodeNotFound {
		t.Fatalf("code = %q, want NOT_FOUND", got.Code)
	}
}

func TestWSHandler_CopyUnauthorizedDestinationNotFound(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	h := newWSHandler(t, svc)
	id := seedGlobalViaService(t, svc, "g", "v")

	resp, err := wsTransfer(t, h, ws.ActionSecretCopy,
		`{"id":"`+id+`","target_scope":"workspace","target_workspace_id":"workspace-b","name":"x"}`)
	if err != nil {
		t.Fatalf("wsCopy: %v", err)
	}
	if got := decodeWSError(t, resp); got.Code != ws.ErrorCodeNotFound {
		t.Fatalf("code = %q, want NOT_FOUND", got.Code)
	}
}

func TestWSHandler_CopyInternalErrorSanitized(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, errors.New("database unreachable"), nil)
	h := newWSHandler(t, svc)
	id := seedGlobalViaService(t, svc, "g", "v")

	resp, err := wsTransfer(t, h, ws.ActionSecretCopy,
		`{"id":"`+id+`","target_scope":"workspace","target_workspace_id":"workspace-a","name":"x"}`)
	if err != nil {
		t.Fatalf("wsCopy: %v", err)
	}
	got := decodeWSError(t, resp)
	if got.Code != ws.ErrorCodeInternalError {
		t.Fatalf("code = %q, want INTERNAL_ERROR", got.Code)
	}
	if strings.Contains(got.Message, "database unreachable") {
		t.Fatalf("error message leaked internal detail: %q", got.Message)
	}
}

func TestWSHandler_WorkspaceSourceWithoutWorkspaceIDNotFound(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	h := newWSHandler(t, svc)
	w := mustCreateViaService(t, svc, "ws", "v", ScopeWorkspace, "workspace-a")

	resp, err := wsTransfer(t, h, ws.ActionSecretCopy,
		`{"id":"`+w.ID+`","target_scope":"global","name":"x"}`)
	if err != nil {
		t.Fatalf("wsCopy: %v", err)
	}
	if got := decodeWSError(t, resp); got.Code != ws.ErrorCodeNotFound {
		t.Fatalf("code = %q, want NOT_FOUND (workspace source without workspace_id)", got.Code)
	}
}

func TestWSHandler_ReusedEnvelopeResetsAllFields(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true, "workspace-b": true}, nil, nil)
	h := newWSHandler(t, svc)
	wsSecretA := mustCreateViaService(t, svc, "ws-a", "va", ScopeWorkspace, "workspace-a")
	global2 := seedGlobalViaService(t, svc, "g2", "vg")

	// First message: workspace source, workspace target, explicit null name.
	first, err := wsTransfer(t, h, ws.ActionSecretCopy,
		`{"id":"`+wsSecretA.ID+`","workspace_id":"workspace-a","target_scope":"workspace","target_workspace_id":"workspace-b","name":null}`)
	if err != nil {
		t.Fatalf("first message: %v", err)
	}
	if got := decodeWSError(t, first); got.Code != ws.ErrorCodeBadRequest {
		t.Fatalf("first code = %q, want BAD_REQUEST", got.Code)
	}

	// Second message: a different source (Global), workspace target A, omitted
	// name, NO source workspace_id. If the decoder leaked the stale source
	// workspace_id, the Global source would be mis-dispatched as a workspace
	// source (404); a clean reset succeeds with the default source name.
	second, err := wsTransfer(t, h, ws.ActionSecretCopy,
		`{"id":"`+global2+`","target_scope":"workspace","target_workspace_id":"workspace-a"}`)
	if err != nil {
		t.Fatalf("second message: %v", err)
	}
	item := decodeWSItem(t, second)
	if item.Scope != ScopeWorkspace || item.WorkspaceID != "workspace-a" || item.Name != "g2" {
		t.Fatalf("second item = %+v (stale fields leaked between decodes)", item)
	}

	// Decoder-level reuse: one wsTransferPayload instance decodes BOTH
	// messages. The second decode must fully reset id, workspace_id, scope,
	// and the name presence markers.
	var payload wsTransferPayload
	firstBody := `{"id":"` + wsSecretA.ID + `","workspace_id":"workspace-a","target_scope":"workspace","target_workspace_id":"workspace-b","name":null}`
	if err := json.Unmarshal([]byte(firstBody), &payload); err != nil {
		t.Fatalf("decode first into shared payload: %v", err)
	}
	// Local alias for the embedded request: promoted selectors for
	// WorkspaceID would be ambiguous (the envelope's source workspace id
	// shadows the embedded target workspace id).
	target := &payload.CopySecretRequest
	if !target.nameNull || payload.ID != wsSecretA.ID {
		t.Fatalf("first shared decode = %+v", payload)
	}
	secondBody := `{"id":"` + global2 + `","target_scope":"workspace","target_workspace_id":"workspace-a"}`
	if err := json.Unmarshal([]byte(secondBody), &payload); err != nil {
		t.Fatalf("decode second into shared payload: %v", err)
	}
	if payload.ID != global2 || payload.WorkspaceID != "" ||
		target.Scope != ScopeWorkspace || target.WorkspaceID != "workspace-a" ||
		target.Name != nil || target.nameSet || target.nameNull {
		t.Fatalf("shared payload after second decode = %+v (stale fields leaked)", payload)
	}

	// A malformed second decode must leave the receiver fully reset, never
	// carrying state from the first decode.
	staleName := "stale"
	bad := wsTransferPayload{
		ID:          "stale-id",
		WorkspaceID: "stale-ws",
		CopySecretRequest: CopySecretRequest{
			Scope:       ScopeWorkspace,
			WorkspaceID: "stale-ws",
			Name:        &staleName,
			nameSet:     true,
		},
	}
	if err := bad.UnmarshalJSON([]byte(`{"id":`)); err == nil {
		t.Fatal("malformed decode succeeded")
	}
	badTarget := &bad.CopySecretRequest
	if bad.ID != "" || bad.WorkspaceID != "" || badTarget.Scope != "" ||
		badTarget.WorkspaceID != "" || badTarget.Name != nil ||
		badTarget.nameSet || badTarget.nameNull {
		t.Fatalf("malformed decode left stale state: %+v", bad)
	}
}
