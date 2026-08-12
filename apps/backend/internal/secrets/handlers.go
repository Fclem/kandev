package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Handler provides HTTP and WebSocket handlers for secrets CRUD.
type Handler struct {
	service *Service
	logger  *logger.Logger
}

// NewHandler creates a new secrets handler.
func NewHandler(svc *Service, log *logger.Logger) *Handler {
	return &Handler{service: svc, logger: log}
}

// RegisterRoutes registers both HTTP and WS handlers.
func RegisterRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	h := NewHandler(svc, log)
	h.registerHTTP(router)
	h.registerWS(dispatcher)
}

func (h *Handler) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.POST("/secrets", h.httpCreateSecret)
	api.GET("/secrets", h.httpListSecrets)
	api.GET("/secrets/:id", h.httpGetSecret)
	api.PUT("/secrets/:id", h.httpUpdateSecret)
	api.DELETE("/secrets/:id", h.httpDeleteSecret)
	api.POST("/secrets/:id/reveal", h.httpRevealSecret)
	api.POST("/secrets/:id/copy", h.httpCopySecret)
	api.POST("/secrets/:id/move", h.httpMoveSecret)
}

func (h *Handler) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionSecretList, h.wsList)
	dispatcher.RegisterFunc(ws.ActionSecretCreate, h.wsCreate)
	dispatcher.RegisterFunc(ws.ActionSecretUpdate, h.wsUpdate)
	dispatcher.RegisterFunc(ws.ActionSecretDelete, h.wsDelete)
	dispatcher.RegisterFunc(ws.ActionSecretReveal, h.wsReveal)
	dispatcher.RegisterFunc(ws.ActionSecretCopy, h.wsCopy)
	dispatcher.RegisterFunc(ws.ActionSecretMove, h.wsMove)
}

// HTTP handlers

func (h *Handler) httpCreateSecret(c *gin.Context) {
	var req CreateSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	item, err := h.service.Create(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("failed to create secret", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *Handler) httpListSecrets(c *gin.Context) {
	opts := SecretListOptions{
		Scope:         SecretScope(c.Query("scope")),
		WorkspaceID:   c.Query("workspace_id"),
		IncludeGlobal: c.Query("include_global") == "true",
	}
	if opts.Scope == "" && opts.WorkspaceID != "" {
		opts.Scope = ScopeWorkspace
	}
	items, err := h.service.ListScoped(c.Request.Context(), opts)
	if err != nil {
		h.logger.Error("failed to list secrets", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list secrets"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) httpGetSecret(c *gin.Context) {
	id := c.Param("id")
	secret, err := h.getSecret(c, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, secret)
}

func (h *Handler) httpUpdateSecret(c *gin.Context) {
	id := c.Param("id")
	var req UpdateSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	item, err := h.updateSecret(c, id, &req)
	if err != nil {
		h.logger.Error("failed to update secret", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *Handler) httpDeleteSecret(c *gin.Context) {
	id := c.Param("id")
	if err := h.deleteSecret(c, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) httpRevealSecret(c *gin.Context) {
	id := c.Param("id")
	value, err := h.revealSecret(c, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RevealSecretResponse{Value: value})
}

// WS handlers

func (h *Handler) wsList(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		Scope         SecretScope `json:"scope"`
		WorkspaceID   string      `json:"workspace_id"`
		IncludeGlobal bool        `json:"include_global"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}
	if req.Scope == "" && req.WorkspaceID != "" {
		req.Scope = ScopeWorkspace
	}
	items, err := h.service.ListScoped(ctx, SecretListOptions{
		Scope: req.Scope, WorkspaceID: req.WorkspaceID, IncludeGlobal: req.IncludeGlobal,
	})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, items)
}

func (h *Handler) wsCreate(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req CreateSecretRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	item, err := h.service.Create(ctx, &req)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, item)
}

func (h *Handler) wsUpdate(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspace_id"`
		UpdateSecretRequest
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	item, err := h.updateSecretForWorkspace(ctx, payload.ID, payload.WorkspaceID, &payload.UpdateSecretRequest)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, item)
}

func (h *Handler) wsDelete(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	if err := h.deleteSecretForWorkspace(ctx, payload.ID, payload.WorkspaceID); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"success": true})
}

func (h *Handler) wsReveal(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	value, err := h.revealSecretForWorkspace(ctx, payload.ID, payload.WorkspaceID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, RevealSecretResponse{Value: value})
}

func (h *Handler) getSecret(c *gin.Context, id string) (*Secret, error) {
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		return h.service.GetWorkspaceSecret(c.Request.Context(), id, workspaceID)
	}
	return h.service.Get(c.Request.Context(), id)
}

func (h *Handler) updateSecret(c *gin.Context, id string, req *UpdateSecretRequest) (*SecretListItem, error) {
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		return h.service.UpdateWorkspaceSecret(c.Request.Context(), id, workspaceID, req)
	}
	return h.service.Update(c.Request.Context(), id, req)
}

func (h *Handler) updateSecretForWorkspace(ctx context.Context, id, workspaceID string, req *UpdateSecretRequest) (*SecretListItem, error) {
	if workspaceID != "" {
		return h.service.UpdateWorkspaceSecret(ctx, id, workspaceID, req)
	}
	return h.service.Update(ctx, id, req)
}

func (h *Handler) deleteSecret(c *gin.Context, id string) error {
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		return h.service.DeleteWorkspaceSecret(c.Request.Context(), id, workspaceID)
	}
	return h.service.Delete(c.Request.Context(), id)
}

func (h *Handler) deleteSecretForWorkspace(ctx context.Context, id, workspaceID string) error {
	if workspaceID != "" {
		return h.service.DeleteWorkspaceSecret(ctx, id, workspaceID)
	}
	return h.service.Delete(ctx, id)
}

func (h *Handler) revealSecret(c *gin.Context, id string) (string, error) {
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		return h.service.RevealWorkspaceSecret(c.Request.Context(), id, workspaceID)
	}
	return h.service.Reveal(c.Request.Context(), id)
}

func (h *Handler) revealSecretForWorkspace(ctx context.Context, id, workspaceID string) (string, error) {
	if workspaceID != "" {
		return h.service.RevealWorkspaceSecret(ctx, id, workspaceID)
	}
	return h.service.Reveal(ctx, id)
}

// HTTP copy/move handlers

func (h *Handler) httpCopySecret(c *gin.Context) {
	h.httpTransferSecret(c, false)
}

func (h *Handler) httpMoveSecret(c *gin.Context) {
	h.httpTransferSecret(c, true)
}

func (h *Handler) httpTransferSecret(c *gin.Context, move bool) {
	id := c.Param("id")
	var req CopySecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	item, err := h.transferSecret(c.Request.Context(), id, c.Query("workspace_id"), &req, move)
	if err != nil {
		status, message := h.transferError(err)
		c.JSON(status, gin.H{"error": message})
		return
	}
	c.JSON(http.StatusCreated, item)
}

// transferSecret dispatches copy/move using the existing source-scoping
// convention: a non-empty workspace_id selects the workspace-scoped source.
func (h *Handler) transferSecret(ctx context.Context, id, sourceWorkspaceID string, req *CopySecretRequest, move bool) (*SecretListItem, error) {
	if move {
		return h.service.Move(ctx, id, sourceWorkspaceID, req)
	}
	return h.service.Copy(ctx, id, sourceWorkspaceID, req)
}

// transferError classifies a transfer error into an HTTP status and a safe
// message. Only the known sentinels map to 400/404/409; unexpected failures
// become a sanitized 500 with the details logged server-side.
func (h *Handler) transferError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrSecretValidation):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrWorkspaceAccessDenied):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, ErrSecretNameConflict):
		return http.StatusConflict, err.Error()
	default:
		h.logger.Error("secret transfer failed", zap.Error(err))
		return http.StatusInternalServerError, "internal error"
	}
}

// WS copy/move handlers

func (h *Handler) wsCopy(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.wsTransfer(ctx, msg, false)
}

func (h *Handler) wsMove(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.wsTransfer(ctx, msg, true)
}

// wsTransferPayload is the WS envelope for secrets.copy / secrets.move. It
// decodes id/workspace_id separately, then delegates the transfer fields to
// the presence-aware CopySecretRequest decoder (a flat struct would silently
// collapse an explicit null name into "omitted").
type wsTransferPayload struct {
	ID          string
	WorkspaceID string
	CopySecretRequest
}

// UnmarshalJSON implements the presence-aware envelope decode. The
// CopySecretRequest decoder runs on the whole payload and resets every field,
// including its presence markers, so a reused envelope cannot leak state
// between messages.
func (p *wsTransferPayload) UnmarshalJSON(data []byte) error {
	// Reset every field before parsing so a failed decode can never leave
	// stale state from an earlier message on a reused envelope.
	p.ID = ""
	p.WorkspaceID = ""
	p.CopySecretRequest = CopySecretRequest{}
	var aux struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	p.ID = aux.ID
	p.WorkspaceID = aux.WorkspaceID
	return json.Unmarshal(data, &p.CopySecretRequest)
}

func (h *Handler) wsTransfer(ctx context.Context, msg *ws.Message, move bool) (*ws.Message, error) {
	var payload wsTransferPayload
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	item, err := h.transferSecret(ctx, payload.ID, payload.WorkspaceID, &payload.CopySecretRequest, move)
	if err != nil {
		code, message := h.transferWSError(err)
		return ws.NewError(msg.ID, msg.Action, code, message, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, item)
}

func (h *Handler) transferWSError(err error) (string, string) {
	switch {
	case errors.Is(err, ErrSecretValidation):
		return ws.ErrorCodeBadRequest, err.Error()
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrWorkspaceAccessDenied):
		return ws.ErrorCodeNotFound, err.Error()
	case errors.Is(err, ErrSecretNameConflict):
		return ws.ErrorCodeConflict, err.Error()
	default:
		h.logger.Error("secret transfer failed", zap.Error(err))
		return ws.ErrorCodeInternalError, "internal error"
	}
}
