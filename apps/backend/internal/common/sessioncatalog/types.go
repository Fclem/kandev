// Package sessioncatalog defines catalog observations, separate from effective settings.
package sessioncatalog

import "github.com/kandev/kandev/internal/common/modelcatalog"

const MetadataKey = "profile_catalog_refresh"

const (
	OutcomeComplete    = "complete"
	OutcomeUnsupported = "unsupported"
	OutcomeFailed      = "failed"

	DispositionHostCandidates      = "host_candidates"
	DispositionRediscoveryRequired = "rediscovery_required"
	DispositionCleared             = "cleared"

	ReasonNewCandidates     = "new_candidates"
	ReasonDiscoveryComplete = "discovery_complete"
	// ReasonExecutorScoped marks remote and isolated executors; they never
	// receive host choices or an automatic task probe.
	ReasonExecutorScoped = "executor_scoped"
	// ReasonContextUnverified covers unknown locality, legacy evidence, a
	// changed saved source and legacy generic candidates.
	ReasonContextUnverified = "context_unverified"
	// ReasonTaskDiscoveryUnsupported and ReasonTaskDiscoveryFailed describe a
	// task probe outcome; neither is provider startup evidence.
	ReasonTaskDiscoveryUnsupported = "task_discovery_unsupported"
	ReasonTaskDiscoveryFailed      = "task_discovery_failed"

	PolicyStrict           = "strict"
	PolicyProviderRestored = "provider_restored"
)

// Owner correlates discovery to one accepted session launch source.
type Owner struct {
	ExecutionID      string `json:"execution_id"`
	AttemptID        string `json:"attempt_id"`
	SourceGeneration uint64 `json:"source_generation"`
}

// Choice is an exact provider value with bounded presentation data only.
type Choice struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Surface distinguishes unsupported discovery from supported empty discovery.
type Surface struct {
	Present   bool                  `json:"present"`
	Supported bool                  `json:"supported"`
	Choices   []Choice              `json:"choices"`
	Catalog   *modelcatalog.Catalog `json:"catalog,omitempty"`
}

// Discovery is emitted only after a terminal handshake/settings outcome.
type Discovery struct {
	Outcome    string  `json:"outcome"`
	Generation uint64  `json:"generation,omitempty"`
	Models     Surface `json:"models"`
	Modes      Surface `json:"modes"`
}

// Observation is revisioned session metadata. A cleared disposition is a tombstone.
// Host candidates are selectable only with Source SourceTaskProbeV1: Probe is
// that target's own complete per-surface result, never another session's or
// the generic settings probe.
type Observation struct {
	Version     int                   `json:"version"`
	Revision    uint64                `json:"revision"`
	ProfileID   string                `json:"profile_id"`
	Digest      string                `json:"digest"`
	Owner       Owner                 `json:"owner"`
	Disposition string                `json:"disposition"`
	Reason      string                `json:"reason"`
	Source      string                `json:"source,omitempty"`
	Models      []Choice              `json:"models,omitempty"`
	Modes       []Choice              `json:"modes,omitempty"`
	Catalog     *modelcatalog.Catalog `json:"catalog,omitempty"`
	Probe       *Discovery            `json:"probe,omitempty"`
	Result      *Discovery            `json:"result,omitempty"`
}

// TaskOwnedCandidates reports whether the record may supply selectable host
// candidates. Legacy generic-origin records never qualify.
func (o *Observation) TaskOwnedCandidates() bool {
	return o.Disposition == DispositionHostCandidates && o.Source == SourceTaskProbeV1 && o.Probe != nil &&
		o.Probe.Outcome == OutcomeComplete
}

// ExecutionEvidence describes the actual initialized execution being refreshed.
// SavedSourceCurrent means it was launched under the profile's current saved
// launch-input revision; it is not launch-context equivalence with a utility.
type ExecutionEvidence struct {
	Owner              Owner
	ProfileID          string
	AgentID            string
	ExecutorType       string
	Initialized        bool
	SavedSourceCurrent bool
}

// Update is delivered only to subscribers of the owning task session.
type Update struct {
	TaskID    string      `json:"task_id"`
	SessionID string      `json:"session_id"`
	Record    Observation `json:"record"`
}

func (u Update) GetSessionID() string { return u.SessionID }

const LaunchMetadataKey = "profile_catalog_discovery_owner"

// Launch binds a replacement's discovery to the notice present at its start.
// A refresh committed after this reservation cannot be settled by that launch.
type Launch struct {
	Owner          Owner  `json:"owner"`
	NoticeRevision uint64 `json:"notice_revision"`
}
