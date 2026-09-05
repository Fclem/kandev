package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
	"sync"
	"time"
)

const (
	// queueErrorCodeEntryNotFound is surfaced when an edit/remove targets an entry
	// that has already been drained (atomic-take won the race).
	queueErrorCodeEntryNotFound       = "entry_not_found"
	queueErrorCodeSessionBusy         = "session_busy"
	queueErrorCodeNotPromptable       = "session_not_promptable"
	queueErrorCodeSendNowQueueEmpty   = "queue_empty"
	queueErrorCodeSendNowQueueChanged = "queue_changed"
	// queueErrorCodeQueueChanged is the shared "your snapshot is stale" signal
	// for reorder drift; the wire value matches the send-now code so clients
	// reconcile with one handler.
	queueErrorCodeQueueChanged              = "queue_changed"
	queueErrorCodeSendNowConflict           = "send_now_conflict"
	queueErrorCodeSendNowTurnChanged        = "turn_changed"
	queueErrorCodeSendNowAttachmentOverflow = "send_now_attachment_overflow"
	queueErrorCodeSendNowReferenceOverflow  = "send_now_reference_overflow"
	// queueErrorCodeMergeReferenceOverflow is surfaced when a merge would push
	// the combined entity references past the per-message cap; the merge is
	// rejected atomically instead of dropping persisted references.
	queueErrorCodeMergeReferenceOverflow = "merge_reference_overflow"
	// queueErrorCodeMergeDisabled is surfaced when queued-message merging is
	// disabled via the message queue system setting.
	queueErrorCodeMergeDisabled = "merge_disabled"
	queueInvalidReferences      = "Invalid entity references"
	queueAccessDenied           = "Session not found"

	// Payload field names — extracted to satisfy goconst (≥3 occurrences).
	fieldSessionID = "session_id"
	fieldEntryID   = "entry_id"
	fieldQueueSize = "queue_size"
	fieldMax       = "max"
)

// QueueService is the surface the handlers depend on. Real implementation lives
// in messagequeue.Service.
type QueueService interface {
	QueueMessageWithMetadata(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment, metadata map[string]interface{}) (*messagequeue.QueuedMessage, error)
	QueueMessageWithMetadataAfterInsert(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment, metadata map[string]interface{}, afterInsert func(context.Context, *messagequeue.QueuedMessage) error) (*messagequeue.QueuedMessage, error)
	AppendContent(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, bool, error)
	GetEntry(ctx context.Context, sessionID, entryID string) (*messagequeue.QueuedMessage, error)
	UpdateMessageWithMetadata(ctx context.Context, sessionID, entryID, content string, attachments []messagequeue.MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string) error
	RemoveEntry(ctx context.Context, sessionID, entryID string) error
	MergeIntoAbove(ctx context.Context, sessionID, entryID, queuedBy string) (*messagequeue.QueuedMessage, error)
	ReorderEntries(ctx context.Context, sessionID string, orderedIDs []string) error
	CancelAll(ctx context.Context, sessionID string) (int, error)
	GetStatus(ctx context.Context, sessionID string) *messagequeue.QueueStatus
}

// QueueDrainer drains a single queued entry when the session is promptable.
type QueueDrainer interface {
	DrainQueuedMessage(ctx context.Context, sessionID string) (bool, error)
}

// QueueAutoRunController persists queue policy and may immediately dispatch
// one FIFO head when enabling an eligible session.
type QueueAutoRunController interface {
	SetQueueAutoRun(ctx context.Context, sessionID string, enabled bool) (autoRun bool, dispatched bool, err error)
}

// QueueEditLeaseController owns target-bound queued-message edit leases.
type QueueEditLeaseController interface {
	BeginEdit(context.Context, string, string, string) (*messagequeue.QueueEditLease, error)
	RenewEdit(context.Context, string, string, string, string) (*messagequeue.QueueEditLease, error)
	EndEdit(context.Context, string, string, string, string) error
	UpdateMessageWithLease(context.Context, string, string, string, string, string, int64, string, []messagequeue.MessageAttachment, map[string]interface{}) (int64, error)
}

type queueEditAdmissionController interface {
	WithSessionAdmission(context.Context, string, func(context.Context) error) error
}
type queueEditLeaseStateReader interface {
	GetEditLease(context.Context, string, string) (*messagequeue.QueueEditLease, error)
}

type queueEntryLocator interface {
	FindEntryByID(context.Context, string) (*messagequeue.QueuedMessage, error)
}

type queueAttachmentReferenceChecker interface {
	ReferencedQueueAttachmentIDs(context.Context, string, string, []string) (map[string]struct{}, error)
}
type queueBatchCanceller interface {
	CancelAllWithEntries(context.Context, string) ([]messagequeue.QueuedMessage, error)
}
type queueAttachmentCleanupStore interface {
	AttachmentCleanupPersistenceAvailable() bool
	UpsertAttachmentCleanup(context.Context, messagequeue.AttachmentCleanup) error
	DeleteAttachmentCleanup(context.Context, string, string, string) error
	ListAttachmentCleanups(context.Context) ([]messagequeue.AttachmentCleanup, error)
}

type queueEntryRemover interface {
	RemoveEntryWithEntry(context.Context, string, string) (*messagequeue.QueuedMessage, error)
}

type queueEditAttachmentController interface {
	UpdateMessageWithLeaseAfterValidationAndFinalize(context.Context, string, string, string, string, string, int64, string, []messagequeue.MessageAttachment, map[string]interface{}, func(context.Context) error, func(context.Context) error, func(context.Context, *messagequeue.QueuedMessage) error) (int64, error)
}

// QueueSendNowDispatcher is implemented by the orchestrator service. It is
// kept separate from QueueDrainer so queue-focused handlers can retain their
// small test doubles while the new action gets the replacement-turn contract.
type QueueSendNowDispatcher interface {
	SendQueuedNow(ctx context.Context, sessionID, scope, entryID string) (int, error)
}

// QueueAccessAuthorizer scopes queue reads and mutations to visible sessions.
type QueueAccessAuthorizer interface {
	AuthorizeSessionAccess(ctx context.Context, sessionID string) error
	AuthorizeTaskSessionAccess(ctx context.Context, taskID, sessionID string) error
}

// SessionTaskResolver returns the task that owns a session. It enriches the
// message.queue.status_changed event with task_id so task-scoped consumers
// (e.g. the status summary projector) can refresh per-task queued counts.
// An empty result omits the field; an error is logged and also omits it.
type SessionTaskResolver func(ctx context.Context, sessionID string) (string, error)

// QueueAttachmentClaimer binds staged file descriptors to the task/session
// after a queue entry has been durably accepted.
type QueueAttachmentClaimer interface {
	ClaimMessageAttachments(ctx context.Context, taskID, sessionID string, attachments []v1.MessageAttachment) error
}

type QueueAttachmentReleaser interface {
	ReleaseMessageAttachments(ctx context.Context, taskID, sessionID string, attachments []v1.MessageAttachment) error
}

type queueEntryTaker interface {
	TakeQueuedEntry(context.Context, string, string) (*messagequeue.QueuedMessage, bool, error)
}
type pendingQueueAttachmentCleanupKey struct {
	sessionID   string
	entryID     string
	operationID string
}

type pendingQueueAttachmentCleanup struct {
	key      pendingQueueAttachmentCleanupKey
	req      wsUpdateMessageRequest
	previous *messagequeue.QueuedMessage
	releaser QueueAttachmentReleaser
	authCtx  context.Context
	wake     chan struct{}
}

// QueueHandlers handles WebSocket message-queue operations.
type QueueHandlers struct {
	queueService             QueueService
	queueDrainer             QueueDrainer
	queueAutoRun             QueueAutoRunController
	queueDispatcher          QueueSendNowDispatcher
	queueEdit                QueueEditLeaseController
	accessAuthorizer         QueueAccessAuthorizer
	sessionTaskResolver      SessionTaskResolver
	eventBus                 bus.EventBus
	logger                   *logger.Logger
	referenceValidator       entityrefs.SubmissionValidator
	attachmentClaimer        QueueAttachmentClaimer
	attachmentCleanupMu      sync.Mutex
	pendingAttachmentCleanup map[pendingQueueAttachmentCleanupKey]*pendingQueueAttachmentCleanup
	attachmentCleanupCtx     context.Context
	attachmentCleanupCancel  context.CancelFunc
	attachmentCleanupWG      sync.WaitGroup
	attachmentCleanupStarted bool
	attachmentCleanupStopped bool
}

// SetAttachmentClaimer wires task-owned attachment claiming into queue edits.
func (h *QueueHandlers) SetAttachmentClaimer(claimer QueueAttachmentClaimer) {
	h.attachmentClaimer = claimer
}

// NewQueueHandlers creates a new QueueHandlers instance. sessionTaskResolver
// enriches published queue status events with the owning task_id; nil keeps
// the payload unchanged.
func NewQueueHandlers(
	queueService QueueService,
	eventBus bus.EventBus,
	log *logger.Logger,
	queueDrainer QueueDrainer,
	accessAuthorizer QueueAccessAuthorizer,
	sessionTaskResolver SessionTaskResolver,
	validators ...entityrefs.SubmissionValidator,
) *QueueHandlers {
	var referenceValidator entityrefs.SubmissionValidator
	if len(validators) > 0 {
		referenceValidator = validators[0]
	}
	handlers := &QueueHandlers{
		queueService:             queueService,
		queueDrainer:             queueDrainer,
		accessAuthorizer:         accessAuthorizer,
		sessionTaskResolver:      sessionTaskResolver,
		eventBus:                 eventBus,
		logger:                   log.WithFields(zap.String("component", "queue-handlers")),
		referenceValidator:       referenceValidator,
		pendingAttachmentCleanup: make(map[pendingQueueAttachmentCleanupKey]*pendingQueueAttachmentCleanup),
	}
	if controller, ok := queueService.(QueueEditLeaseController); ok {
		handlers.queueEdit = controller
	}
	if dispatcher, ok := queueDrainer.(QueueSendNowDispatcher); ok {
		handlers.queueDispatcher = dispatcher
	}
	if controller, ok := queueDrainer.(QueueAutoRunController); ok {
		handlers.queueAutoRun = controller
	}

	return handlers
}

// Start owns the context used by pending attachment cleanup retries and reloads
// durable obligations left by an earlier process.
func (h *QueueHandlers) Start(ctx context.Context) {
	h.attachmentCleanupMu.Lock()
	if h.attachmentCleanupStarted || h.attachmentCleanupStopped {
		h.attachmentCleanupMu.Unlock()
		return
	}
	h.attachmentCleanupCtx, h.attachmentCleanupCancel = context.WithCancel(ctx)
	h.attachmentCleanupStarted = true
	h.loadPendingAttachmentCleanupsLocked(context.WithoutCancel(ctx))
	for _, pending := range h.pendingAttachmentCleanup {
		h.attachmentCleanupWG.Add(1)
		go h.retryPendingAttachmentCleanup(pending)
	}
	h.attachmentCleanupMu.Unlock()
}

func (h *QueueHandlers) loadPendingAttachmentCleanupsLocked(ctx context.Context) {
	store, ok := h.attachmentCleanupStore()
	releaser, canRelease := h.attachmentClaimer.(QueueAttachmentReleaser)
	if !ok || !canRelease {
		return
	}
	cleanups, err := store.ListAttachmentCleanups(ctx)
	if err != nil {
		h.logger.Error("failed to reload pending queue attachment cleanup", zap.Error(err))
		return
	}
	for _, cleanup := range cleanups {
		key := pendingQueueAttachmentCleanupKey{
			sessionID: cleanup.SessionID, entryID: cleanup.EntryID, operationID: cleanup.OperationID,
		}
		if _, exists := h.pendingAttachmentCleanup[key]; exists {
			continue
		}
		h.pendingAttachmentCleanup[key] = &pendingQueueAttachmentCleanup{
			key: key,
			req: wsUpdateMessageRequest{
				SessionID:   cleanup.SessionID,
				EntryID:     cleanup.EntryID,
				LeaseID:     cleanup.LeaseID,
				OperationID: cleanup.OperationID,
			},
			previous: &messagequeue.QueuedMessage{
				ID: cleanup.EntryID, SessionID: cleanup.SessionID, TaskID: cleanup.TaskID,
				Attachments: cleanup.Attachments,
			},
			releaser: releaser,
			authCtx:  context.WithoutCancel(ctx),
			wake:     make(chan struct{}, 1),
		}
	}
}

func (h *QueueHandlers) attachmentCleanupStore() (queueAttachmentCleanupStore, bool) {
	store, ok := h.queueService.(queueAttachmentCleanupStore)
	return store, ok && store.AttachmentCleanupPersistenceAvailable()
}

// Stop cancels active retries after their obligations are durable. A later
// handler instance reloads unresolved work before accepting queue operations.
func (h *QueueHandlers) Stop() {
	h.attachmentCleanupMu.Lock()
	if !h.attachmentCleanupStopped {
		h.attachmentCleanupStopped = true
		if h.attachmentCleanupCancel != nil {
			h.attachmentCleanupCancel()
		}
	}
	h.attachmentCleanupMu.Unlock()
	h.attachmentCleanupWG.Wait()
}

// RegisterHandlers registers queue handlers with the dispatcher.
func (h *QueueHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionMessageQueueAdd, h.wsQueueMessage)
	d.RegisterFunc(ws.ActionMessageQueueCancel, h.wsCancelAll)
	d.RegisterFunc(ws.ActionMessageQueueGet, h.wsGetQueueStatus)
	d.RegisterFunc(ws.ActionMessageQueueUpdate, h.wsUpdateMessage)
	d.RegisterFunc(ws.ActionMessageQueueEditBegin, h.wsBeginEdit)
	d.RegisterFunc(ws.ActionMessageQueueEditRenew, h.wsRenewEdit)
	d.RegisterFunc(ws.ActionMessageQueueEditEnd, h.wsEndEdit)
	d.RegisterFunc(ws.ActionMessageQueueAppend, h.wsAppendToQueue)
	d.RegisterFunc(ws.ActionMessageQueueDrain, h.wsDrainQueue)
	d.RegisterFunc(ws.ActionMessageQueueSendNow, h.wsSendNow)
	d.RegisterFunc(ws.ActionMessageQueueAutoRunSet, h.wsSetAutoRun)
	d.RegisterFunc(ws.ActionMessageQueueRemove, h.wsRemoveEntry)
	d.RegisterFunc(ws.ActionMessageQueueMerge, h.wsMergeIntoAbove)
	d.RegisterFunc(ws.ActionMessageQueueReorder, h.wsReorder)
}

type wsQueueMessageRequest struct {
	SessionID        string                           `json:"session_id"`
	TaskID           string                           `json:"task_id"`
	Content          string                           `json:"content"`
	Model            string                           `json:"model,omitempty"`
	PlanMode         bool                             `json:"plan_mode,omitempty"`
	Attachments      []messagequeue.MessageAttachment `json:"attachments,omitempty"`
	ContextFiles     []v1.ContextFileMeta             `json:"context_files,omitempty"`
	EntityReferences []v1.EntityReference             `json:"entity_references,omitempty"`
	UserID           string                           `json:"user_id,omitempty"`
}

// wsQueueMessage handles ActionMessageQueueAdd, appending a new entry to the session queue.
func (h *QueueHandlers) wsQueueMessage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsQueueMessageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if denied := h.authorizeTaskSession(ctx, msg, req.TaskID, req.SessionID); denied != nil {
		return denied, nil
	}
	if req.Content == "" && len(req.Attachments) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content or attachments are required", nil)
	}
	if invalid := firstInvalidDeliveryMode(req.Attachments); invalid >= 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "attachment delivery_mode must be prompt or path",
			map[string]interface{}{"attachment_index": invalid})
	}
	if invalid := firstInvalidAttachment(req.Attachments); invalid >= 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "attachment metadata is invalid",
			map[string]interface{}{"attachment_index": invalid})
	}
	if messagequeue.IsReservedQueuedBy(req.UserID) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, reservedIdentityError(req.UserID), nil)
	}
	references, err := h.validateSubmittedReferences(ctx, req.SessionID, req.TaskID, req.EntityReferences)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, queueInvalidReferences, nil)
	}
	req.EntityReferences = references

	// Default empty user_id to QueuedByUser so the entry has a non-empty owner;
	// the UpdateMessage handler relies on this so its filter against agent
	// entries (queued_by="agent") is always meaningful.
	queuedBy := req.UserID
	if queuedBy == "" {
		queuedBy = messagequeue.QueuedByUser
	}
	metadata := orchestrator.NewUserMessageMeta().
		WithContextFiles(req.ContextFiles).
		WithEntityReferences(req.EntityReferences).
		ToMap()
	queued, err := h.admitQueuedMessage(ctx, &req, queuedBy, metadata)
	if err != nil {
		if errors.Is(err, messagequeue.ErrQueueFull) {
			status := h.queueService.GetStatus(ctx, req.SessionID)
			return ws.NewError(msg.ID, msg.Action, messagequeue.QueueFullErrorCode, "Queue is full",
				map[string]interface{}{
					fieldQueueSize: status.Count,
					fieldMax:       status.Max,
				})
		}
		if errors.Is(err, messagequeue.ErrTaskInactive) {
			// The task was archived or deleted between the caller's
			// authorization and the queue admission; do not queue a message
			// that would be orphaned behind the task's purge.
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Task is no longer active", nil)
		}
		if errors.Is(err, errQueuedAttachmentRollback) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to roll back queued attachment", nil)
		}
		if errors.Is(err, errQueuedAttachmentUnavailable) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Attachment is no longer available", nil)
		}
		h.logger.Error("failed to queue message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to queue message", nil)
	}

	h.publishStatus(ctx, req.SessionID, queued)
	return ws.NewResponse(msg.ID, msg.Action, queued)
}

var (
	errQueuedAttachmentUnavailable = errors.New("queued attachment unavailable")
	errQueuedAttachmentRollback    = errors.New("queued attachment rollback failed")
)

func (h *QueueHandlers) admitQueuedMessage(ctx context.Context, req *wsQueueMessageRequest, queuedBy string, metadata map[string]interface{}) (*messagequeue.QueuedMessage, error) {
	if h.attachmentClaimer == nil || len(req.Attachments) == 0 {
		return h.queueService.QueueMessageWithMetadata(
			ctx, req.SessionID, req.TaskID, req.Content, req.Model, queuedBy, req.PlanMode, req.Attachments, metadata,
		)
	}
	return h.queueService.QueueMessageWithMetadataAfterInsert(
		ctx, req.SessionID, req.TaskID, req.Content, req.Model, queuedBy, req.PlanMode, req.Attachments, metadata,
		func(admittedCtx context.Context, source *messagequeue.QueuedMessage) error {
			claimErr := h.attachmentClaimer.ClaimMessageAttachments(
				admittedCtx, req.TaskID, req.SessionID, queueAttachmentsToV1(req.Attachments),
			)
			if claimErr == nil {
				return nil
			}
			if rollbackErr := h.rollbackQueuedAttachmentClaim(admittedCtx, req.SessionID, source.ID); rollbackErr != nil {
				h.logger.Error("failed to roll back queued attachment", zap.Error(rollbackErr))
				return fmt.Errorf("%w: %v", errQueuedAttachmentRollback, rollbackErr)
			}
			return fmt.Errorf("%w: %v", errQueuedAttachmentUnavailable, claimErr)
		},
	)
}

type wsCancelAllRequest struct {
	SessionID string `json:"session_id"`
}

// wsCancelAll handles ActionMessageQueueCancel, clearing every pending entry for a session.
func (h *QueueHandlers) wsCancelAll(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCancelAllRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}

	var removed int
	var removedEntries []messagequeue.QueuedMessage
	var countRemovedEntries bool
	var err error
	cancel := func(cancelCtx context.Context) error {
		var err error
		if batchCanceller, ok := h.queueService.(queueBatchCanceller); ok {
			removedEntries, err = batchCanceller.CancelAllWithEntries(cancelCtx, req.SessionID)
			countRemovedEntries = true
		} else {
			status := h.queueService.GetStatus(cancelCtx, req.SessionID)
			removed, err = h.queueService.CancelAll(cancelCtx, req.SessionID)
			if err == nil && status != nil {
				removedEntries = status.Entries
			}
		}
		if err != nil {
			return err
		}
		for i := range removedEntries {
			if !removedEntries[i].IsReservedInFlight() {
				if countRemovedEntries {
					removed++
				}
				h.releaseQueuedAttachments(cancelCtx, &removedEntries[i])
			}
		}
		return nil
	}
	if admission, ok := h.queueService.(queueEditAdmissionController); ok {
		err = admission.WithSessionAdmission(ctx, req.SessionID, cancel)
	} else {
		err = cancel(ctx)
	}
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		"removed":      removed,
	})
}

type wsDrainQueueRequest struct {
	SessionID string `json:"session_id"`
}

// wsDrainQueue handles ActionMessageQueueDrain, dispatching one queued entry when the session is promptable.
func (h *QueueHandlers) wsDrainQueue(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsDrainQueueRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if h.queueDrainer == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue drain is unavailable", nil)
	}

	drained, err := h.queueDrainer.DrainQueuedMessage(ctx, req.SessionID)
	if err != nil {
		switch {
		case errors.Is(err, orchestrator.ErrAgentPromptInProgress):
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeSessionBusy, "Session is busy", nil)
		case errors.Is(err, orchestrator.ErrSessionNotPromptable):
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeNotPromptable, "Session is not ready for input", nil)
		default:
			h.logger.Error("failed to drain queued message", zap.String(fieldSessionID, req.SessionID), zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to drain queued message", nil)
		}
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		"drained":      drained,
	})
}

type wsSetAutoRunRequest struct {
	SessionID string `json:"session_id"`
	Enabled   *bool  `json:"enabled"`
}

// wsSetAutoRun persists automatic queue processing and optionally starts the
// promptable FIFO head when enabling it.
func (h *QueueHandlers) wsSetAutoRun(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsSetAutoRunRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.Enabled == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "enabled is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if h.queueAutoRun == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue Auto-run is unavailable", nil)
	}
	autoRun, dispatched, err := h.queueAutoRun.SetQueueAutoRun(ctx, req.SessionID, *req.Enabled)
	if err != nil {
		h.logger.Error("failed to set queue Auto-run", zap.String(fieldSessionID, req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to set queue Auto-run", nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		"auto_run":     autoRun,
		"dispatched":   dispatched,
	})
}

type wsSendNowRequest struct {
	SessionID string `json:"session_id"`
	Scope     string `json:"scope"`
	EntryID   string `json:"entry_id,omitempty"`
}

// wsSendNow handles ActionMessageQueueSendNow, interrupting the active turn with an exact queue selection.
func (h *QueueHandlers) wsSendNow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsSendNowRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if validation := validateSendNowRequest(req); validation != "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, validation, nil)
	}
	if h.queueDispatcher == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue send-now is unavailable", nil)
	}

	sentCount, err := h.queueDispatcher.SendQueuedNow(ctx, req.SessionID, req.Scope, req.EntryID)
	if err != nil {
		return h.sendNowErrorResponse(msg, req.SessionID, err)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		"dispatched":   true,
		"sent_count":   sentCount,
	})
}

// validateSendNowRequest validates the send-now scope and entry id combination.
func validateSendNowRequest(req wsSendNowRequest) string {
	switch {
	case req.Scope != orchestrator.QueueSendNowScopeEntry && req.Scope != orchestrator.QueueSendNowScopeAll:
		return "scope must be entry or all"
	case req.Scope == orchestrator.QueueSendNowScopeEntry && req.EntryID == "":
		return "entry_id is required for entry scope"
	case req.Scope == orchestrator.QueueSendNowScopeAll && req.EntryID != "":
		return "entry_id is not allowed for all scope"
	default:
		return ""
	}
}

// sendNowErrorResponse maps send-now failures to their stable websocket error codes.
func (h *QueueHandlers) sendNowErrorResponse(msg *ws.Message, sessionID string, err error) (*ws.Message, error) {
	switch {
	case errors.Is(err, orchestrator.ErrSendNowEntryNotFound):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry is no longer pending", nil)
	case errors.Is(err, orchestrator.ErrSendNowQueueEmpty):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowQueueEmpty, "Queue is empty", nil)
	case errors.Is(err, orchestrator.ErrSendNowQueueChanged):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowQueueChanged, "Queue changed before Send Now could start", nil)
	case errors.Is(err, orchestrator.ErrSendNowConflict):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowConflict, "Another cancellation or Send Now operation is in progress", nil)
	case errors.Is(err, orchestrator.ErrSendNowTurnChanged):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowTurnChanged, "The active turn changed before Send Now could start", nil)
	case errors.Is(err, messagequeue.ErrSendNowAttachmentOverflow):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowAttachmentOverflow, "Combined attachments exceed the message limits", nil)
	case errors.Is(err, messagequeue.ErrSendNowReferenceOverflow):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowReferenceOverflow, "Combined entity references exceed the message limit", nil)
	case errors.Is(err, orchestrator.ErrSessionNotPromptable):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeNotPromptable, "Session is not ready for input", nil)
	default:
		h.logger.Error("failed to send queued message now", zap.String(fieldSessionID, sessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to send queued message now", nil)
	}
}

type wsGetQueueStatusRequest struct {
	SessionID string `json:"session_id"`
}

// wsGetQueueStatus handles ActionMessageQueueGet, returning the pending list and capacity.
func (h *QueueHandlers) wsGetQueueStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetQueueStatusRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}

	status := h.queueService.GetStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, status)
}

type wsQueueEditRequest struct {
	SessionID string `json:"session_id"`
	EntryID   string `json:"entry_id"`
	LeaseID   string `json:"lease_id"`
}

func queueEditError(msg *ws.Message, err error) *ws.Message {
	code := queueErrorCodeEntryNotFound
	message := "Queue entry was already drained or is no longer editable"
	if errors.Is(err, messagequeue.ErrEditConflict) {
		code, message = "edit_conflict", "Queue entry is being edited by another view"
	} else if errors.Is(err, messagequeue.ErrEditRevisionConflict) {
		code, message = "queue_conflict", "Queue entry changed while it was being edited"
	}
	response, _ := ws.NewError(msg.ID, msg.Action, code, message, nil)
	return response
}
func queueEditLeaseError(msg *ws.Message, err error) *ws.Message {
	if errors.Is(err, messagequeue.ErrEditLeaseNotFound) {
		response, _ := ws.NewError(msg.ID, msg.Action, "edit_conflict", "Queue entry edit lease is no longer valid", nil)
		return response
	}
	return queueEditError(msg, err)
}

func (h *QueueHandlers) wsBeginEdit(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsQueueEditRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" || req.EntryID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id and entry_id are required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if h.queueEdit == nil || ws.ConnectionID(ctx) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "Queue editing requires a WebSocket connection", nil)
	}
	lease, err := h.queueEdit.BeginEdit(ctx, req.SessionID, req.EntryID, ws.ConnectionID(ctx))
	if err != nil {
		return queueEditError(msg, err), nil
	}
	return ws.NewResponse(msg.ID, msg.Action, lease)
}

func (h *QueueHandlers) wsRenewEdit(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsQueueEditRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" || req.EntryID == "" || req.LeaseID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id, entry_id, and lease_id are required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if h.queueEdit == nil || ws.ConnectionID(ctx) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "Queue editing requires a WebSocket connection", nil)
	}
	lease, err := h.queueEdit.RenewEdit(ctx, req.SessionID, req.EntryID, req.LeaseID, ws.ConnectionID(ctx))
	if err != nil {
		return queueEditLeaseError(msg, err), nil
	}
	return ws.NewResponse(msg.ID, msg.Action, lease)
}

func (h *QueueHandlers) wsEndEdit(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsQueueEditRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" || req.EntryID == "" || req.LeaseID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id, entry_id, and lease_id are required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if h.queueEdit == nil || ws.ConnectionID(ctx) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "Queue editing requires a WebSocket connection", nil)
	}
	if err := h.queueEdit.EndEdit(ctx, req.SessionID, req.EntryID, req.LeaseID, ws.ConnectionID(ctx)); err != nil {
		return queueEditLeaseError(msg, err), nil
	}
	h.signalPendingAttachmentCleanup(req.SessionID, req.EntryID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]string{fieldSessionID: req.SessionID, fieldEntryID: req.EntryID})
}

type wsUpdateMessageRequest struct {
	SessionID        string                           `json:"session_id"`
	EntryID          string                           `json:"entry_id"`
	LeaseID          string                           `json:"lease_id,omitempty"`
	OperationID      string                           `json:"operation_id,omitempty"`
	ExpectedRevision *int64                           `json:"expected_target_revision,omitempty"`
	Content          string                           `json:"content"`
	Attachments      []messagequeue.MessageAttachment `json:"attachments,omitempty"`
	EntityReferences []v1.EntityReference             `json:"entity_references,omitempty"`
	UserID           string                           `json:"user_id,omitempty"`
}

// wsUpdateMessage handles ActionMessageQueueUpdate, replacing a queued entry's content.
func (h *QueueHandlers) wsUpdateMessage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateMessageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		// Required so publishStatus can broadcast the post-update list to other
		// connected clients; without it they'd be left with a stale view.
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if req.EntryID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "entry_id is required", nil)
	}
	if req.Content == "" && len(req.Attachments) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content or attachments are required", nil)
	}
	if invalid := firstInvalidDeliveryMode(req.Attachments); invalid >= 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "attachment delivery_mode must be prompt or path",
			map[string]interface{}{"attachment_index": invalid})
	}
	if invalid := firstInvalidAttachment(req.Attachments); invalid >= 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "attachment metadata is invalid",
			map[string]interface{}{"attachment_index": invalid})
	}

	// Reject any client-supplied identity that would impersonate the agent.
	// Without this guard a hostile WS client could send user_id="agent" to
	// satisfy the `WHERE queued_by = ?` filter on inter-task entries and
	// overwrite their content. The reserved sentinel must be settable only
	// from the inter-task dispatch path inside the backend.
	if messagequeue.IsReservedQueuedBy(req.UserID) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, reservedIdentityError(req.UserID), nil)
	}
	connectionID := ws.ConnectionID(ctx)
	if connectionID != "" && (h.queueEdit == nil || req.LeaseID == "" || req.OperationID == "" || req.ExpectedRevision == nil) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"lease_id, operation_id, and expected_target_revision are required", nil)
	}
	referencesProvided := req.EntityReferences != nil
	references, err := h.validateSubmittedReferences(ctx, req.SessionID, "", req.EntityReferences)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, queueInvalidReferences, nil)
	}
	// Default empty user_id to QueuedByUser so the UpdateContent guard always
	// runs against a non-empty owner. Agent entries (queued_by="agent") then
	// fail the filter, mirroring the canEdit UI gate at the WS layer.
	req.EntityReferences = references
	queuedBy := req.UserID
	if queuedBy == "" {
		queuedBy = messagequeue.QueuedByUser
	}
	var metadataUpdates map[string]interface{}
	if referencesProvided {
		var referenceMetadata interface{}
		if len(req.EntityReferences) > 0 {
			referenceMetadata = req.EntityReferences
		}
		metadataUpdates = map[string]interface{}{messagequeue.MetadataEntityReferences: referenceMetadata}
	}
	var previous *messagequeue.QueuedMessage
	var releaseClaims QueueAttachmentReleaser
	var newlyAdded []messagequeue.MessageAttachment
	if h.attachmentClaimer != nil {
		var err error
		previous, err = h.queueService.GetEntry(ctx, req.SessionID, req.EntryID)
		if err != nil {
			if errors.Is(err, messagequeue.ErrEntryNotFound) {
				return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
			}
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		newlyAdded = newlyAddedQueueAttachments(previous.Attachments, req.Attachments)
		releaseClaims, _ = h.attachmentClaimer.(QueueAttachmentReleaser)
	}
	var revision int64
	var updateErr error
	applyUpdate := func(updateCtx context.Context) (int64, error) {
		attachmentsToClaim := newlyAdded
		newlyAdded = nil
		if h.attachmentClaimer != nil {
			if err := h.attachmentClaimer.ClaimMessageAttachments(updateCtx, previous.TaskID, req.SessionID, queueAttachmentsToV1(attachmentsToClaim)); err != nil {
				h.releaseQueuedAttachmentUpdateFailure(updateCtx, previous, req.SessionID, attachmentsToClaim, releaseClaims)
				return 0, fmt.Errorf("%w: %v", errQueuedAttachmentUnavailable, err)
			}
		}
		if connectionID != "" {
			revision, updateErr = h.queueEdit.UpdateMessageWithLease(updateCtx, req.SessionID, req.EntryID,
				req.LeaseID, req.OperationID, connectionID, *req.ExpectedRevision, req.Content,
				req.Attachments, metadataUpdates)
		} else {
			updateErr = h.queueService.UpdateMessageWithMetadata(updateCtx, req.SessionID, req.EntryID,
				req.Content, req.Attachments, metadataUpdates, queuedBy)
		}
		if updateErr != nil && h.attachmentClaimer != nil {
			h.releaseQueuedAttachmentUpdateFailure(updateCtx, previous, req.SessionID, attachmentsToClaim, releaseClaims)
		}
		return revision, updateErr
	}
	if connectionID != "" && h.attachmentClaimer != nil {
		revision, updateErr = h.updateMessageWithAttachmentLease(
			ctx, req, connectionID, previous, releaseClaims, &newlyAdded, metadataUpdates, applyUpdate,
		)
	} else {
		revision, updateErr = applyUpdate(ctx)
	}
	if updateErr != nil {
		return h.queueUpdateFailure(ctx, msg, req, updateErr)
	}
	if releaseClaims != nil && previous != nil && (connectionID == "" || h.attachmentClaimer == nil) {
		h.releaseSupersededQueueAttachments(ctx, req, previous, releaseClaims)
	}
	response := map[string]interface{}{fieldEntryID: req.EntryID}
	if req.OperationID != "" {
		response["operation_id"] = req.OperationID
		response["target_revision"] = revision
	}
	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, response)
}

func (h *QueueHandlers) releaseQueuedAttachmentUpdateFailure(
	ctx context.Context,
	previous *messagequeue.QueuedMessage,
	sessionID string,
	newlyAdded []messagequeue.MessageAttachment,
	releaseClaims QueueAttachmentReleaser,
) {
	if releaseClaims == nil || previous == nil || len(newlyAdded) == 0 {
		return
	}
	cleanupCtx := context.WithoutCancel(ctx)
	candidates := h.unreferencedQueueAttachments(cleanupCtx, sessionID, previous.ID, newlyAdded)
	if len(candidates) == 0 {
		return
	}
	if releaseErr := releaseClaims.ReleaseMessageAttachments(cleanupCtx, previous.TaskID, sessionID, queueAttachmentsToV1(candidates)); releaseErr != nil {
		h.logger.Warn("failed to release attachments after queue update failure", zap.Error(releaseErr))
	}
}

// unreferencedQueueAttachments prevents cleanup from deleting a claim that a
// different pending entry in the same session still uses. A reference lookup
// failure fails closed because releasing in that case can destroy another
// queued prompt's attachment.
func (h *QueueHandlers) unreferencedQueueAttachments(
	ctx context.Context,
	sessionID, excludedEntryID string,
	candidates []messagequeue.MessageAttachment,
) []messagequeue.MessageAttachment {
	ctx = context.WithoutCancel(ctx)
	if len(candidates) == 0 {
		return nil
	}
	unique := make([]messagequeue.MessageAttachment, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, attachment := range candidates {
		if attachment.AttachmentID == "" {
			continue
		}
		if _, ok := seen[attachment.AttachmentID]; ok {
			continue
		}
		seen[attachment.AttachmentID] = struct{}{}
		unique = append(unique, attachment)
	}
	if len(unique) == 0 {
		return nil
	}
	checker, ok := h.queueService.(queueAttachmentReferenceChecker)
	if !ok {
		return unique
	}
	ids := make([]string, 0, len(unique))
	for _, attachment := range unique {
		ids = append(ids, attachment.AttachmentID)
	}
	referenced, err := checker.ReferencedQueueAttachmentIDs(ctx, sessionID, excludedEntryID, ids)
	if err != nil {
		h.logger.Warn("failed to inspect queue attachment references", zap.Error(err))
		return nil
	}
	unreferenced := make([]messagequeue.MessageAttachment, 0, len(unique))
	for _, attachment := range unique {
		if _, ok := referenced[attachment.AttachmentID]; !ok {
			unreferenced = append(unreferenced, attachment)
		}
	}
	return unreferenced
}

// releaseSupersededQueueAttachments serializes attachment cleanup with later
// edits and recomputes the retained set from the current queue row. A second
// edit may reclaim an attachment after this update commits but before its
// cleanup runs; releasing from the stale request snapshot would then destroy
// a claim still referenced by the row.
func (h *QueueHandlers) releaseSupersededQueueAttachments(
	ctx context.Context,
	req wsUpdateMessageRequest,
	previous *messagequeue.QueuedMessage,
	releaser QueueAttachmentReleaser,
) {
	release := func(admittedCtx context.Context) error {
		return h.releaseSupersededQueueAttachmentsAdmitted(admittedCtx, req, previous, releaser)
	}
	if admission, ok := h.queueService.(queueEditAdmissionController); ok {
		if err := admission.WithSessionAdmission(context.WithoutCancel(ctx), req.SessionID, release); err != nil {
			h.logger.Warn("failed to serialize superseded queue attachment cleanup", zap.Error(err))
		}
		return
	}
	if err := release(context.WithoutCancel(ctx)); err != nil {
		h.logger.Warn("failed to release superseded queue attachments", zap.Error(err))
	}
}

func (h *QueueHandlers) releaseSupersededQueueAttachmentsAdmitted(
	ctx context.Context,
	req wsUpdateMessageRequest,
	previous *messagequeue.QueuedMessage,
	releaser QueueAttachmentReleaser,
) error {
	cleanupCtx := context.WithoutCancel(ctx)
	current, err := h.queueService.GetEntry(cleanupCtx, req.SessionID, req.EntryID)
	if errors.Is(err, messagequeue.ErrEntryNotFound) {
		if locator, ok := h.queueService.(queueEntryLocator); ok {
			current, err = locator.FindEntryByID(cleanupCtx, req.EntryID)
			if err == nil && current != nil {
				req.SessionID = current.SessionID
			}
		}
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return h.releaseQueueAttachmentCandidates(
				cleanupCtx, previous.TaskID, req, previous.Attachments, releaser,
			)
		}
	}
	if err != nil {
		h.logger.Warn("failed to reload queue entry before attachment cleanup", zap.Error(err))
		return err
	}
	if current == nil {
		return nil
	}
	return h.releaseQueueAttachmentCandidates(
		cleanupCtx, previous.TaskID, req, supersededQueueAttachments(previous.Attachments, current.Attachments), releaser,
	)
}

func (h *QueueHandlers) releaseQueueAttachmentCandidates(
	ctx context.Context,
	taskID string,
	req wsUpdateMessageRequest,
	candidates []messagequeue.MessageAttachment,
	releaser QueueAttachmentReleaser,
) error {
	cleanupCtx := context.WithoutCancel(ctx)
	unreferenced := h.unreferencedQueueAttachments(cleanupCtx, req.SessionID, req.EntryID, candidates)
	if len(unreferenced) == 0 {
		return nil
	}
	if err := releaser.ReleaseMessageAttachments(
		cleanupCtx, taskID, req.SessionID, queueAttachmentsToV1(unreferenced),
	); err != nil {
		h.logger.Warn("failed to release superseded queue attachments", zap.Error(err))
		return err
	}
	return nil
}
func (h *QueueHandlers) queuePendingAttachmentCleanup(
	ctx context.Context,
	req wsUpdateMessageRequest,
	previous *messagequeue.QueuedMessage,
	releaser QueueAttachmentReleaser,
) {
	key := pendingQueueAttachmentCleanupKey{
		sessionID: req.SessionID, entryID: req.EntryID, operationID: req.OperationID,
	}
	cleanupCtx := context.WithoutCancel(ctx)
	if store, ok := h.attachmentCleanupStore(); ok {
		if err := store.UpsertAttachmentCleanup(cleanupCtx, messagequeue.AttachmentCleanup{
			SessionID: req.SessionID, EntryID: req.EntryID, OperationID: req.OperationID,
			TaskID: previous.TaskID, LeaseID: req.LeaseID,
			Attachments: append([]messagequeue.MessageAttachment(nil), previous.Attachments...),
			CreatedAt:   time.Now().UTC(),
		}); err != nil {
			h.logger.Error("failed to persist queue attachment cleanup", zap.Error(err))
		}
	}
	pending := &pendingQueueAttachmentCleanup{
		key: key, req: req, previous: previous, releaser: releaser,
		authCtx: cleanupCtx, wake: make(chan struct{}, 1),
	}
	h.attachmentCleanupMu.Lock()
	if h.attachmentCleanupStopped {
		h.attachmentCleanupMu.Unlock()
		return
	}
	if _, exists := h.pendingAttachmentCleanup[key]; exists {
		h.attachmentCleanupMu.Unlock()
		return
	}
	h.pendingAttachmentCleanup[key] = pending
	if !h.attachmentCleanupStarted {
		h.attachmentCleanupMu.Unlock()
		return
	}
	h.attachmentCleanupWG.Add(1)
	h.attachmentCleanupMu.Unlock()
	go h.retryPendingAttachmentCleanup(pending)
}

func (h *QueueHandlers) retryPendingAttachmentCleanup(pending *pendingQueueAttachmentCleanup) {
	defer h.attachmentCleanupWG.Done()
	delay := 10 * time.Millisecond
	for {
		if !h.editLeaseActive(pending) {
			if err := h.runPendingAttachmentCleanup(pending); err == nil {
				if err := h.deletePendingAttachmentCleanup(pending); err != nil {
					h.logger.Warn("failed to acknowledge queue attachment cleanup", zap.Error(err))
				} else {
					h.attachmentCleanupMu.Lock()
					delete(h.pendingAttachmentCleanup, pending.key)
					h.attachmentCleanupMu.Unlock()
					return
				}
			}
		}
		if !h.waitForAttachmentCleanupRetry(delay, pending.wake) {
			return
		}
		if delay < time.Second {
			delay *= 2
		}
	}
}

func (h *QueueHandlers) deletePendingAttachmentCleanup(pending *pendingQueueAttachmentCleanup) error {
	store, ok := h.attachmentCleanupStore()
	if !ok {
		return nil
	}
	return store.DeleteAttachmentCleanup(
		pending.authCtx, pending.key.sessionID, pending.key.entryID, pending.key.operationID,
	)
}

func (h *QueueHandlers) editLeaseActive(pending *pendingQueueAttachmentCleanup) bool {
	reader, ok := h.queueService.(queueEditLeaseStateReader)
	if !ok {
		return false
	}
	lease, err := reader.GetEditLease(
		pending.authCtx, pending.req.SessionID, pending.req.EntryID,
	)
	if err != nil {
		return !errors.Is(err, messagequeue.ErrEditLeaseNotFound)
	}
	return lease != nil && lease.LeaseID == pending.req.LeaseID
}

func (h *QueueHandlers) waitForAttachmentCleanupRetry(delay time.Duration, wake <-chan struct{}) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-h.attachmentCleanupCtx.Done():
		return false
	case <-wake:
		return true
	case <-timer.C:
		return true
	}
}

func (h *QueueHandlers) signalPendingAttachmentCleanup(sessionID, entryID string) {
	h.attachmentCleanupMu.Lock()
	defer h.attachmentCleanupMu.Unlock()
	for key, pending := range h.pendingAttachmentCleanup {
		if key.sessionID == sessionID && key.entryID == entryID {
			select {
			case pending.wake <- struct{}{}:
			default:
			}
		}
	}
}

func (h *QueueHandlers) runPendingAttachmentCleanup(pending *pendingQueueAttachmentCleanup) error {
	cleanup := func(ctx context.Context) error {
		return h.releaseSupersededQueueAttachmentsAdmitted(
			ctx, pending.req, pending.previous, pending.releaser,
		)
	}
	if admission, ok := h.queueService.(queueEditAdmissionController); ok {
		return admission.WithSessionAdmission(pending.authCtx, pending.req.SessionID, cleanup)
	}
	return cleanup(pending.authCtx)
}

func newlyAddedQueueAttachments(previous, replacement []messagequeue.MessageAttachment) []messagequeue.MessageAttachment {
	retained := make(map[string]struct{}, len(previous))
	for _, attachment := range previous {
		if attachment.AttachmentID != "" {
			retained[attachment.AttachmentID] = struct{}{}
		}
	}
	var newlyAdded []messagequeue.MessageAttachment
	for _, attachment := range replacement {
		if attachment.AttachmentID == "" {
			continue
		}
		if _, ok := retained[attachment.AttachmentID]; !ok {
			newlyAdded = append(newlyAdded, attachment)
		}
	}
	return newlyAdded
}

func (h *QueueHandlers) updateMessageWithAttachmentLease(
	ctx context.Context,
	req wsUpdateMessageRequest,
	connectionID string,
	previous *messagequeue.QueuedMessage,
	releaseClaims QueueAttachmentReleaser,
	newlyAdded *[]messagequeue.MessageAttachment,
	metadataUpdates map[string]interface{},
	applyUpdate func(context.Context) (int64, error),
) (int64, error) {
	controller, ok := h.queueEdit.(queueEditAttachmentController)
	if !ok {
		return h.updateMessageWithAttachmentAdmissionFallback(
			ctx, req, previous, releaseClaims, applyUpdate,
		)
	}

	var claimedAttachments []messagequeue.MessageAttachment
	var claimAttempted bool
	return controller.UpdateMessageWithLeaseAfterValidationAndFinalize(
		ctx, req.SessionID, req.EntryID, req.LeaseID, req.OperationID, connectionID,
		*req.ExpectedRevision, req.Content, req.Attachments, metadataUpdates,
		func(prepareCtx context.Context) error {
			claimedAttachments = append(claimedAttachments[:0], *newlyAdded...)
			*newlyAdded = nil
			claimAttempted = true
			if err := h.attachmentClaimer.ClaimMessageAttachments(prepareCtx, previous.TaskID, req.SessionID, queueAttachmentsToV1(claimedAttachments)); err != nil {
				return fmt.Errorf("%w: %v", errQueuedAttachmentUnavailable, err)
			}
			return nil
		},
		func(rollbackCtx context.Context) error {
			if claimAttempted && releaseClaims != nil && previous != nil {
				if len(claimedAttachments) == 0 {
					// Invoke the release callback after every attempted
					// claim, including a no-op claim, so rollback ordering
					// remains observable to attachment lifecycle owners.
					_ = releaseClaims.ReleaseMessageAttachments(
						context.WithoutCancel(rollbackCtx), previous.TaskID, req.SessionID, nil,
					)
				} else {
					h.releaseQueuedAttachmentUpdateFailure(
						rollbackCtx, previous, req.SessionID, claimedAttachments, releaseClaims,
					)
				}
			}
			claimedAttachments = nil
			return nil
		},
		func(finalizeCtx context.Context, cleanupPrevious *messagequeue.QueuedMessage) error {
			if releaseClaims == nil || cleanupPrevious == nil {
				return nil
			}
			if err := h.releaseSupersededQueueAttachmentsAdmitted(finalizeCtx, req, cleanupPrevious, releaseClaims); err != nil {
				h.queuePendingAttachmentCleanup(finalizeCtx, req, cleanupPrevious, releaseClaims)
			}
			return nil
		},
	)
}

func (h *QueueHandlers) updateMessageWithAttachmentAdmissionFallback(
	ctx context.Context,
	req wsUpdateMessageRequest,
	previous *messagequeue.QueuedMessage,
	releaseClaims QueueAttachmentReleaser,
	applyUpdate func(context.Context) (int64, error),
) (int64, error) {
	admission, ok := h.queueEdit.(queueEditAdmissionController)
	if !ok {
		return applyUpdate(ctx)
	}
	var revision int64
	err := admission.WithSessionAdmission(ctx, req.SessionID, func(admittedCtx context.Context) error {
		var err error
		revision, err = applyUpdate(admittedCtx)
		if err != nil {
			return err
		}
		if cleanupErr := h.releaseSupersededQueueAttachmentsAdmitted(
			admittedCtx, req, previous, releaseClaims,
		); cleanupErr != nil {
			h.queuePendingAttachmentCleanup(admittedCtx, req, previous, releaseClaims)
		}
		return nil
	})
	return revision, err
}
func (h *QueueHandlers) queueUpdateFailure(
	_ context.Context,
	msg *ws.Message,
	req wsUpdateMessageRequest,
	updateErr error,
) (*ws.Message, error) {
	if errors.Is(updateErr, errQueuedAttachmentUnavailable) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Attachment is no longer available", nil)
	}
	if errors.Is(updateErr, messagequeue.ErrEditConflict) ||
		errors.Is(updateErr, messagequeue.ErrEditLeaseNotFound) ||
		errors.Is(updateErr, messagequeue.ErrEditRevisionConflict) {
		return queueEditLeaseError(msg, updateErr), nil
	}
	if errors.Is(updateErr, messagequeue.ErrEntryNotFound) {
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, updateErr.Error(), nil)
}

// supersededQueueAttachments returns attachment descriptors dropped by the replacement.
func supersededQueueAttachments(previous, replacement []messagequeue.MessageAttachment) []messagequeue.MessageAttachment {
	retained := make(map[string]struct{}, len(replacement))
	for _, attachment := range replacement {
		if attachment.AttachmentID != "" {
			retained[attachment.AttachmentID] = struct{}{}
		}
	}
	var superseded []messagequeue.MessageAttachment
	for _, attachment := range previous {
		if attachment.AttachmentID == "" {
			continue
		}
		if _, ok := retained[attachment.AttachmentID]; !ok {
			superseded = append(superseded, attachment)
		}
	}
	return superseded
}

// validateSubmittedReferences runs the entity-reference submission validator when configured.
func (h *QueueHandlers) validateSubmittedReferences(
	ctx context.Context,
	sessionID, taskID string,
	references []v1.EntityReference,
) ([]v1.EntityReference, error) {
	if len(references) == 0 {
		return nil, nil
	}
	if h.referenceValidator == nil {
		return nil, entityrefs.ErrUnauthorizedReference
	}
	return h.referenceValidator.ValidateForSubmission(ctx, sessionID, taskID, references)
}

// rollbackQueuedAttachmentClaim releases attachments claimed for a queue entry that failed to persist.
func (h *QueueHandlers) rollbackQueuedAttachmentClaim(ctx context.Context, sessionID, entryID string) error {
	rollbackCtx := context.WithoutCancel(ctx)
	if err := h.queueService.RemoveEntry(rollbackCtx, sessionID, entryID); err == nil {
		return nil
	} else {
		h.logger.Error("failed to remove queue entry after attachment claim failure",
			zap.String("entry_id", entryID), zap.Error(err))
	}
	taker, ok := h.queueService.(queueEntryTaker)
	if !ok {
		return errors.New("queue service cannot atomically remove a queued entry")
	}
	_, _, err := taker.TakeQueuedEntry(rollbackCtx, sessionID, entryID)
	return err
}

// releaseQueuedAttachments releases descriptors no longer referenced by the
// remaining queue entries. The queue entry has already been removed.
func (h *QueueHandlers) releaseQueuedAttachments(ctx context.Context, entry *messagequeue.QueuedMessage) {
	if entry == nil || len(entry.Attachments) == 0 || h.attachmentClaimer == nil {
		return
	}
	releaser, ok := h.attachmentClaimer.(QueueAttachmentReleaser)
	if !ok {
		return
	}
	cleanupCtx := context.WithoutCancel(ctx)
	candidates := h.unreferencedQueueAttachments(cleanupCtx, entry.SessionID, entry.ID, entry.Attachments)
	if len(candidates) == 0 {
		return
	}
	if err := releaser.ReleaseMessageAttachments(cleanupCtx, entry.TaskID, entry.SessionID, queueAttachmentsToV1(candidates)); err != nil {
		h.logger.Warn("failed to release attachments after queue entry removal", zap.Error(err))
		h.queuePendingAttachmentCleanup(cleanupCtx, wsUpdateMessageRequest{
			SessionID: entry.SessionID,
			EntryID:   entry.ID,
		}, entry, releaser)
	}
}

// firstInvalidDeliveryMode returns the index of the first attachment with an unknown delivery mode.
func firstInvalidDeliveryMode(attachments []messagequeue.MessageAttachment) int {
	for i, att := range attachments {
		if att.DeliveryMode != "" && att.DeliveryMode != "prompt" && att.DeliveryMode != "path" {
			return i
		}
	}
	return -1
}

// firstInvalidAttachment returns the index of the first structurally invalid attachment.
func firstInvalidAttachment(attachments []messagequeue.MessageAttachment) int {
	if len(attachments) > models.MaxMessageAttachmentCount {
		return models.MaxMessageAttachmentCount
	}
	var total int64
	for i, attachment := range attachments {
		if attachment.Type != "image" && attachment.Type != "audio" && attachment.Type != "resource" {
			return i
		}
		bytes, valid := attachmentPayloadBytes(attachment)
		if !valid {
			return i
		}
		total += bytes
		if total > models.MaxMessageAttachmentBytes {
			return i
		}
	}
	return -1
}

// attachmentPayloadBytes returns the decoded payload size and whether the base64 is valid.
func attachmentPayloadBytes(attachment messagequeue.MessageAttachment) (int64, bool) {
	if attachment.AttachmentID != "" {
		if attachment.Data != "" || attachment.Name == "" || attachment.MimeType == "" {
			return 0, false
		}
		if attachment.SizeBytes < 0 || attachment.SizeBytes > models.MaxMessageAttachmentBytes {
			return 0, false
		}
		return attachment.SizeBytes, true
	}
	if attachment.Data == "" || len(attachment.Data) > 10*1024*1024 {
		return 0, false
	}
	decoded, err := base64.StdEncoding.DecodeString(attachment.Data)
	if err != nil {
		return 0, false
	}
	return int64(len(decoded)), true
}

// queueAttachmentsToV1 converts queue attachments to the API v1 representation.
func queueAttachmentsToV1(attachments []messagequeue.MessageAttachment) []v1.MessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	converted := make([]v1.MessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		converted = append(converted, v1.MessageAttachment{
			AttachmentID: attachment.AttachmentID,
			Type:         attachment.Type,
			Data:         attachment.Data,
			MimeType:     attachment.MimeType,
			Name:         attachment.Name,
			SizeBytes:    attachment.SizeBytes,
			DeliveryMode: attachment.DeliveryMode,
		})
	}
	return converted
}

type wsRemoveEntryRequest struct {
	SessionID string `json:"session_id"`
	EntryID   string `json:"entry_id"`
}

// wsRemoveEntry handles ActionMessageQueueRemove, deleting a single queued entry.
func (h *QueueHandlers) wsRemoveEntry(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsRemoveEntryRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		// Required so publishStatus can broadcast the post-removal list.
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if req.EntryID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "entry_id is required", nil)
	}

	var entry *messagequeue.QueuedMessage
	var err error
	remove := func(removeCtx context.Context) error {
		var err error
		if remover, ok := h.queueService.(queueEntryRemover); ok {
			entry, err = remover.RemoveEntryWithEntry(removeCtx, req.SessionID, req.EntryID)
		} else {
			entry, err = h.queueService.GetEntry(removeCtx, req.SessionID, req.EntryID)
			if err == nil {
				err = h.queueService.RemoveEntry(removeCtx, req.SessionID, req.EntryID)
			}
		}
		if err != nil {
			return err
		}
		h.releaseQueuedAttachments(removeCtx, entry)
		return nil
	}
	if admission, ok := h.queueService.(queueEditAdmissionController); ok {
		err = admission.WithSessionAdmission(ctx, req.SessionID, remove)
	} else {
		err = remove(ctx)
	}
	if err != nil {
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry is no longer pending", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{fieldEntryID: req.EntryID})
}

// wsMergeIntoAboveRequest is the payload for ActionMessageQueueMerge: the
// session whose queue is modified and the id of the entry to fold into the
// entry directly above it. user_id is forwarded for ownership checks and is
// optional (the server defaults to the reserved "user" identity).
type wsMergeIntoAboveRequest struct {
	SessionID string `json:"session_id"`
	EntryID   string `json:"entry_id"`
	UserID    string `json:"user_id,omitempty"`
}

// wsMergeIntoAbove handles ActionMessageQueueMerge, folding the referenced
// queued entry into the entry above it and broadcasting the updated queue.
func (h *QueueHandlers) wsMergeIntoAbove(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsMergeIntoAboveRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		// Required so publishStatus can broadcast the post-merge list.
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if req.EntryID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "entry_id is required", nil)
	}
	if messagequeue.IsReservedQueuedBy(req.UserID) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, reservedIdentityError(req.UserID), nil)
	}
	// Default empty user_id to QueuedByUser so the merge ownership guard runs
	// against a non-empty owner, mirroring wsUpdateMessage.
	queuedBy := req.UserID
	if queuedBy == "" {
		queuedBy = messagequeue.QueuedByUser
	}

	merged, err := h.queueService.MergeIntoAbove(ctx, req.SessionID, req.EntryID, queuedBy)
	if err != nil {
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
		}
		if errors.Is(err, messagequeue.ErrNoMergeTarget) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "No mergeable message above this entry", nil)
		}
		if errors.Is(err, messagequeue.ErrMergeReferenceOverflow) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeMergeReferenceOverflow, err.Error(), nil)
		}
		if errors.Is(err, messagequeue.ErrMergeDisabled) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeMergeDisabled, "Message merging is disabled", nil)
		}
		h.logger.Error("failed to merge queued message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to merge queued message", nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{fieldEntryID: merged.ID})
}

// wsReorderRequest is the payload for ActionMessageQueueReorder: the session
// whose queue is modified and the complete ordered list of visible pending
// entry ids the caller wants as the new FIFO order.
type wsReorderRequest struct {
	SessionID  string   `json:"session_id"`
	OrderedIDs []string `json:"ordered_ids"`
}

// wsReorder handles ActionMessageQueueReorder, rewriting the session's visible
// pending order to match ordered_ids and broadcasting the updated queue. Any
// drift from the persisted visible set (a drain/remove/merge raced the drag)
// is rejected atomically with queue_changed so the client refetches.
func (h *QueueHandlers) wsReorder(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsReorderRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if len(req.OrderedIDs) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "ordered_ids is required", nil)
	}
	if hasDuplicateIDs(req.OrderedIDs) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "ordered_ids must not contain duplicates", nil)
	}

	if err := h.queueService.ReorderEntries(ctx, req.SessionID, req.OrderedIDs); err != nil {
		if errors.Is(err, messagequeue.ErrQueueChanged) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeQueueChanged, "Queue changed before the reorder could be applied", nil)
		}
		if errors.Is(err, messagequeue.ErrEditConflict) {
			return queueEditLeaseError(msg, err), nil
		}
		h.logger.Error("failed to reorder queued messages", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to reorder queued messages", nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		"reordered":    len(req.OrderedIDs),
	})
}

type wsAppendToQueueRequest struct {
	SessionID string `json:"session_id"`
	TaskID    string `json:"task_id"`
	Content   string `json:"content"`
	Model     string `json:"model,omitempty"`
	PlanMode  bool   `json:"plan_mode,omitempty"`
	UserID    string `json:"user_id,omitempty"`
}

// wsAppendToQueue handles ActionMessageQueueAppend, appending or inserting a user message.
func (h *QueueHandlers) wsAppendToQueue(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsAppendToQueueRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if denied := h.authorizeTaskSession(ctx, msg, req.TaskID, req.SessionID); denied != nil {
		return denied, nil
	}
	if req.Content == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content is required", nil)
	}
	if messagequeue.IsReservedQueuedBy(req.UserID) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, reservedIdentityError(req.UserID), nil)
	}

	queuedBy := req.UserID
	if queuedBy == "" {
		queuedBy = messagequeue.QueuedByUser
	}
	queued, appended, err := h.queueService.AppendContent(ctx, req.SessionID, req.TaskID, req.Content, req.Model, queuedBy, req.PlanMode, nil)
	if err != nil {
		if errors.Is(err, messagequeue.ErrQueueFull) {
			status := h.queueService.GetStatus(ctx, req.SessionID)
			return ws.NewError(msg.ID, msg.Action, messagequeue.QueueFullErrorCode, "Queue is full",
				map[string]interface{}{
					fieldQueueSize: status.Count,
					fieldMax:       status.Max,
				})
		}
		h.logger.Error("failed to append to queue", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to queue message", nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldEntryID: queued.ID,
		"was_append": appended,
	})
}

// hasDuplicateIDs reports whether ids contains any id more than once.
func hasDuplicateIDs(ids []string) bool {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			return true
		}
		seen[id] = struct{}{}
	}
	return false
}

// reservedIdentityError builds the validation message for reserved caller identities.
func reservedIdentityError(queuedBy string) string {
	if queuedBy == messagequeue.QueuedByAgent {
		return "user_id may not impersonate the agent identity"
	}
	return "user_id may not impersonate a reserved identity"
}

// authorizeSession denies the request when the caller cannot access the session.
func (h *QueueHandlers) authorizeSession(ctx context.Context, msg *ws.Message, sessionID string) *ws.Message {
	if h.accessAuthorizer == nil {
		return queueAccessDeniedResponse(msg)
	}
	if err := h.accessAuthorizer.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		return queueAccessDeniedResponse(msg)
	}
	return nil
}

// authorizeTaskSession denies the request when the caller cannot access the task/session pair.
func (h *QueueHandlers) authorizeTaskSession(
	ctx context.Context,
	msg *ws.Message,
	taskID, sessionID string,
) *ws.Message {
	if h.accessAuthorizer == nil {
		return queueAccessDeniedResponse(msg)
	}
	if err := h.accessAuthorizer.AuthorizeTaskSessionAccess(ctx, taskID, sessionID); err != nil {
		return queueAccessDeniedResponse(msg)
	}
	return nil
}

// queueAccessDeniedResponse builds the non-enumerating session-not-found error response.
func queueAccessDeniedResponse(msg *ws.Message) *ws.Message {
	response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, queueAccessDenied, nil)
	return response
}

// publishStatus emits the latest QueueStatus on the event bus so the frontend
// updates its store after every mutation.
func (h *QueueHandlers) publishStatus(ctx context.Context, sessionID string, admitted ...*messagequeue.QueuedMessage) {
	if h.eventBus == nil {
		return
	}
	// A committed queue mutation still needs an authoritative snapshot when
	// the initiating request has already been cancelled.
	ctx = context.WithoutCancel(ctx)
	if admission, ok := h.queueService.(queueEditAdmissionController); ok {
		_ = admission.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
			h.publishStatusSnapshot(admittedCtx, sessionID, admitted...)
			return nil
		})
		return
	}
	h.publishStatusSnapshot(ctx, sessionID, admitted...)
}

func (h *QueueHandlers) publishStatusSnapshot(ctx context.Context, sessionID string, admitted ...*messagequeue.QueuedMessage) {
	status := h.queueService.GetStatus(ctx, sessionID)
	eventData := map[string]interface{}{
		fieldSessionID:  sessionID,
		"entries":       status.Entries,
		"count":         status.Count,
		fieldMax:        status.Max,
		"auto_run":      status.AutoRun,
		"merge_enabled": status.MergeEnabled,
	}
	if len(admitted) > 0 && admitted[0] != nil && admitted[0].QueuedBy != "" && !messagequeue.IsReservedQueuedBy(admitted[0].QueuedBy) {
		eventData["queued_by"] = admitted[0].QueuedBy
		eventData["queued_at"] = admitted[0].QueuedAt
	}
	if h.sessionTaskResolver != nil {
		if taskID, err := h.sessionTaskResolver(ctx, sessionID); err != nil {
			h.logger.Warn("resolve session task for queue status event",
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else if taskID != "" {
			eventData["task_id"] = taskID
		}
	}
	_ = h.eventBus.Publish(ctx, events.MessageQueueStatusChanged, bus.NewEvent(
		events.MessageQueueStatusChanged,
		"queue-handlers",
		eventData,
	))
}
