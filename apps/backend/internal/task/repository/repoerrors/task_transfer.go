package repoerrors

import "errors"

// TaskTransferConflictReason classifies an ErrTaskTransferConflict without
// string matching. Adapters disclose a reason only to a caller already
// authorized for both workspaces; the MCP surface never discloses it.
type TaskTransferConflictReason string

const (
	TaskTransferSourceChanged             TaskTransferConflictReason = "source_changed"
	TaskTransferDestinationUnavailable    TaskTransferConflictReason = "destination_unavailable"
	TaskTransferStepIncompatible          TaskTransferConflictReason = "step_incompatible"
	TaskTransferDestinationFull           TaskTransferConflictReason = "destination_full"
	TaskTransferWorkspaceRelationUnmapped TaskTransferConflictReason = "workspace_relation_unmapped"
	TaskTransferProfileMappingRequired    TaskTransferConflictReason = "profile_mapping_required"
	TaskTransferPendingStepUnmapped       TaskTransferConflictReason = "pending_step_unmapped"
	TaskTransferCleanupActive             TaskTransferConflictReason = "cleanup_active"
	TaskTransferRelationsBusy             TaskTransferConflictReason = "relations_busy"
	TaskTransferIdempotencyConflict       TaskTransferConflictReason = "idempotency_conflict"
	TaskTransferOperationRejected         TaskTransferConflictReason = "operation_rejected"
	TaskTransferUnsupported               TaskTransferConflictReason = "transfer_unsupported"
)

// TaskTransferConflictError is an ErrTaskTransferConflict carrying its typed
// reason. errors.Is(err, ErrTaskTransferConflict) holds for every instance.
type TaskTransferConflictError struct {
	Reason TaskTransferConflictReason
	detail string
}

// NewTaskTransferConflict returns a typed transfer conflict. The detail is
// internal log text and is never part of an adapter response.
func NewTaskTransferConflict(reason TaskTransferConflictReason, detail string) error {
	return &TaskTransferConflictError{Reason: reason, detail: detail}
}

func (e *TaskTransferConflictError) Error() string {
	if e.detail == "" {
		return ErrTaskTransferConflict.Error()
	}
	return ErrTaskTransferConflict.Error() + ": " + e.detail
}

func (e *TaskTransferConflictError) Unwrap() error { return ErrTaskTransferConflict }

// TaskTransferConflictReasonOf reports the typed reason of a transfer
// conflict. An untyped transfer conflict reports TaskTransferUnsupported;
// ok is false when err is not a transfer conflict at all.
func TaskTransferConflictReasonOf(err error) (TaskTransferConflictReason, bool) {
	if !errors.Is(err, ErrTaskTransferConflict) {
		return "", false
	}
	var typed *TaskTransferConflictError
	if errors.As(err, &typed) {
		return typed.Reason, true
	}
	return TaskTransferUnsupported, true
}

// RetagTaskTransferConflict replaces the reason of a transfer conflict and
// returns every other error unchanged.
func RetagTaskTransferConflict(err error, reason TaskTransferConflictReason) error {
	if !errors.Is(err, ErrTaskTransferConflict) {
		return err
	}
	detail := ""
	var typed *TaskTransferConflictError
	if errors.As(err, &typed) {
		detail = typed.detail
	}
	return NewTaskTransferConflict(reason, detail)
}
