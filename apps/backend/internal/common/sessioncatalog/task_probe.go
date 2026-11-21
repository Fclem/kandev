package sessioncatalog

import (
	"errors"
	"time"
)

// TaskProbePath is the backend-only authenticated agentctl action that reads a
// fresh catalog in one task's sealed execution context. Browser proxies must
// never reach it.
const TaskProbePath = "/api/v1/agent/catalog-candidates/discover"

// SourceTaskProbeV1 marks host candidates acquired by that task's own fresh
// helper. Candidates without it are not task-owned and are never selectable.
const SourceTaskProbeV1 = "task_probe_v1"

const (
	MaxTaskProbePages           = 16
	MaxTaskProbeChoices         = 1024
	MaxTaskProbePayloadBytes    = 1 << 20
	MaxTaskProbeRequestIDLength = 128
	MaxTaskProbeReplyBytes      = 2 << 20
	TaskProbeHelperTimeout      = 15 * time.Second
)

// TaskProbeRequest names the expected live owner. It carries no launch input,
// command, credential, catalog or protocol frame.
type TaskProbeRequest struct {
	RequestID         string `json:"request_id"`
	ManagerID         string `json:"manager_id"`
	ProcessGeneration uint64 `json:"process_generation"`
	SessionID         string `json:"session_id"`
	SessionSource     uint64 `json:"session_source"`
}

// TaskProbeOwner is the actual agentctl owner observed before and after the
// helper ran.
type TaskProbeOwner struct {
	InstanceID        string `json:"instance_id"`
	ManagerID         string `json:"manager_id"`
	ProcessGeneration uint64 `json:"process_generation"`
	SessionID         string `json:"session_id"`
	SessionSource     uint64 `json:"session_source"`
}

// TaskProbeReply is the sanitized helper outcome. Launch material, paths,
// stderr and provider defaults never leave agentctl.
type TaskProbeReply struct {
	RequestID  string         `json:"request_id"`
	Owner      TaskProbeOwner `json:"owner"`
	Capability string         `json:"capability,omitempty"`
	Result     Discovery      `json:"result"`
}

var (
	ErrTaskProbeRequest     = errors.New("invalid task catalog probe request")
	ErrTaskProbeOwner       = errors.New("task catalog probe owner changed")
	ErrTaskProbeResult      = errors.New("invalid task catalog probe result")
	ErrTaskProbeBusy        = errors.New("a task catalog probe is already running")
	ErrTaskProbeUnavailable = errors.New("task catalog probe is unavailable")
)

// MockCatalogProbeMetaKey is the deterministic mock-agent fixture's read-only
// initialize extension. It is test support, never provider qualification.
const MockCatalogProbeMetaKey = "kandev.dev/catalogProbe"

// Validate rejects absent or wildcard ownership; zero generations are never evidence.
func (r TaskProbeRequest) Validate() error {
	if r.RequestID == "" || len(r.RequestID) > MaxTaskProbeRequestIDLength || r.ManagerID == "" ||
		r.ProcessGeneration == 0 || r.SessionID == "" || r.SessionSource == 0 {
		return ErrTaskProbeRequest
	}
	return nil
}

// ValidateTaskProbeReply binds a reply to the request and the instance that
// authenticated it, then checks the bounded per-surface result.
func ValidateTaskProbeReply(request TaskProbeRequest, instanceID string, reply TaskProbeReply) error {
	if err := request.Validate(); err != nil {
		return err
	}
	owner := reply.Owner
	if instanceID == "" || reply.RequestID != request.RequestID || owner.InstanceID != instanceID ||
		owner.ManagerID != request.ManagerID || owner.ProcessGeneration != request.ProcessGeneration ||
		owner.SessionID != request.SessionID || owner.SessionSource != request.SessionSource {
		return ErrTaskProbeOwner
	}
	if err := ValidateDiscovery(&reply.Result); err != nil {
		return errors.Join(ErrTaskProbeResult, err)
	}
	if reply.Result.Outcome == OutcomeComplete && reply.Capability == "" {
		return ErrTaskProbeResult
	}
	if len(reply.Result.Models.Choices)+len(reply.Result.Modes.Choices) > MaxTaskProbeChoices {
		return ErrTaskProbeResult
	}
	return nil
}
