package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
)

const (
	taskTransferLocalHumanActorID   = "local-human"
	taskTransferMaxRequestBytes     = 16 << 10
	taskTransferAdapterAuditTimeout = 2 * time.Second
	taskTransferErrorConflict       = "task transfer conflict"
	taskTransferErrorUnavailable    = "task transfer unavailable"
	taskTransferErrorForbidden      = "insufficient permissions for this action"
	taskTransferErrorInvalid        = "invalid task transfer request"
	taskTransferErrorFailed         = "task transfer failed"
	taskTransferCodeKey             = "code"
	taskTransferAuditResultDenied   = "denied"
	taskTransferAuditResultConflict = "conflict"
)

// httpTaskTransferRequest is the complete Web transfer body. Task identity is
// route-owned; actor, owner, policy, audit identity, and step-entry options
// are server-owned and rejected as unknown fields.
type httpTaskTransferRequest struct {
	ExpectedSourceWorkspaceID string `json:"expected_source_workspace_id"`
	ExpectedSourceWorkflowID  string `json:"expected_source_workflow_id"`
	ExpectedSourceStepID      string `json:"expected_source_workflow_step_id"`
	ExpectedTaskUpdatedAt     string `json:"expected_task_updated_at"`
	DestinationWorkspaceID    string `json:"destination_workspace_id"`
	DestinationWorkflowID     string `json:"destination_workflow_id"`
	DestinationStepID         string `json:"destination_workflow_step_id"`
	IdempotencyKey            string `json:"idempotency_key"`
}

type httpTaskTransferResponse struct {
	Receipt *models.TaskTransferReceipt `json:"receipt"`
}

// httpTransferTask exposes the audited cross-workspace transfer command to a
// human Web caller. It never patches placement or composes same-workspace moves.
func (h *TaskHandlers) httpTransferTask(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx := c.Request.Context()
	var request httpTaskTransferRequest
	decodeErr := decodeHTTPTaskTransferRequest(c.Request.Body, &request)
	command, commandErr := httpTaskTransferCommand(c.Param("id"), request)
	actor, attested := httpTaskTransferActor(ctx)
	command.Actor = actor
	if !attested {
		h.auditRejectedHTTPTaskTransfer(ctx, command, taskTransferAuditResultDenied)
		c.JSON(http.StatusNotFound, gin.H{dependencyKeyError: taskTransferErrorUnavailable})
		return
	}
	if decodeErr != nil || commandErr != nil {
		h.auditRejectedHTTPTaskTransfer(ctx, command, taskTransferAuditResultConflict)
		c.JSON(http.StatusBadRequest, gin.H{dependencyKeyError: taskTransferErrorInvalid})
		return
	}
	receipt, err := h.service.TransferTask(ctx, command)
	if err != nil {
		h.writeHTTPTaskTransferError(c, err)
		return
	}
	c.JSON(http.StatusOK, httpTaskTransferResponse{Receipt: receipt})
}

func decodeHTTPTaskTransferRequest(body io.Reader, request *httpTaskTransferRequest) error {
	if body == nil {
		return errors.New("task transfer body is required")
	}
	payload, err := io.ReadAll(io.LimitReader(body, taskTransferMaxRequestBytes+1))
	if err != nil {
		return err
	}
	if len(payload) > taskTransferMaxRequestBytes {
		return errors.New("task transfer body is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(request); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("task transfer body must contain one JSON value")
	}
	return nil
}

func httpTaskTransferCommand(taskID string, request httpTaskTransferRequest) (models.TaskTransferCommand, error) {
	command := models.TaskTransferCommand{
		TaskID:                    taskID,
		ExpectedSourceWorkspaceID: request.ExpectedSourceWorkspaceID,
		ExpectedSourceWorkflowID:  request.ExpectedSourceWorkflowID,
		ExpectedSourceStepID:      request.ExpectedSourceStepID,
		DestinationWorkspaceID:    request.DestinationWorkspaceID,
		DestinationWorkflowID:     request.DestinationWorkflowID,
		DestinationStepID:         request.DestinationStepID,
		IdempotencyKey:            request.IdempotencyKey,
		PreservationPolicy:        models.TaskTransferPreservationPolicyV1,
		DurableRejection:          true,
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, request.ExpectedTaskUpdatedAt)
	if err != nil {
		return command, err
	}
	command.ExpectedTaskUpdatedAt = updatedAt
	if taskID == "" || request.ExpectedSourceWorkspaceID == "" || request.ExpectedSourceWorkflowID == "" ||
		request.ExpectedSourceStepID == "" || request.DestinationWorkspaceID == "" ||
		request.DestinationWorkflowID == "" || request.DestinationStepID == "" || request.IdempotencyKey == "" {
		return command, errors.New("task transfer request is incomplete")
	}
	return command, nil
}

// httpTaskTransferActor attests the human from the auth middleware identity.
// An absent identity is never internal authority.
func httpTaskTransferActor(ctx context.Context) (models.TaskTransferActor, bool) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		return models.TaskTransferActor{Kind: models.TaskTransferActorRejected}, false
	}
	actorID := identity.UserID
	if actorID == "" && identity.Synthetic {
		actorID = taskTransferLocalHumanActorID
	}
	if actorID == "" {
		return models.TaskTransferActor{Kind: models.TaskTransferActorRejected}, false
	}
	return models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: actorID}, true
}

// auditRejectedHTTPTaskTransfer records an adapter-level rejection. Its
// result never acts as a failed-key barrier, so a malformed body cannot
// claim the key of a reviewed request.
func (h *TaskHandlers) auditRejectedHTTPTaskTransfer(
	ctx context.Context,
	command models.TaskTransferCommand,
	result string,
) {
	command.Actor.Kind = models.TaskTransferActorRejected
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), taskTransferAdapterAuditTimeout)
	defer cancel()
	if err := h.service.AuditTaskTransferAttempt(auditCtx, command, result); err != nil {
		h.logger.Error("audit rejected task transfer", zap.Error(err))
	}
}

func (h *TaskHandlers) writeHTTPTaskTransferError(c *gin.Context, err error) {
	switch {
	case isClientDisconnect(err):
		abortClientDisconnect(c)
	case errors.Is(err, repoerrors.ErrTaskTransferConflict):
		reason, _ := repoerrors.TaskTransferConflictReasonOf(err)
		c.JSON(http.StatusConflict, gin.H{dependencyKeyError: taskTransferErrorConflict, taskTransferCodeKey: httpTaskTransferConflictCode(reason)})
	case service.IsForbidden(err):
		c.JSON(http.StatusForbidden, gin.H{dependencyKeyError: taskTransferErrorForbidden})
	case isNotFound(err), errors.Is(err, repoerrors.ErrWorkspaceNotFound), errors.Is(err, repoerrors.ErrWorkflowNotFound):
		c.JSON(http.StatusNotFound, gin.H{dependencyKeyError: taskTransferErrorUnavailable})
	default:
		h.logger.Error("task transfer failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{dependencyKeyError: taskTransferErrorFailed})
	}
}

// httpTaskTransferConflictCode allowlists the disclosed reason vocabulary.
func httpTaskTransferConflictCode(reason repoerrors.TaskTransferConflictReason) string {
	switch reason {
	case repoerrors.TaskTransferSourceChanged, repoerrors.TaskTransferDestinationUnavailable,
		repoerrors.TaskTransferStepIncompatible, repoerrors.TaskTransferDestinationFull,
		repoerrors.TaskTransferWorkspaceRelationUnmapped, repoerrors.TaskTransferProfileMappingRequired,
		repoerrors.TaskTransferPendingStepUnmapped, repoerrors.TaskTransferCleanupActive,
		repoerrors.TaskTransferIdempotencyConflict, repoerrors.TaskTransferOperationRejected:
		return string(reason)
	default:
		return string(repoerrors.TaskTransferUnsupported)
	}
}
