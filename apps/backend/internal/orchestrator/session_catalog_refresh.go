package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/common/modelcatalog"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	catalogRefreshPageSize       = 128
	catalogRefreshConcurrency    = 2
	catalogRefreshFanoutDeadline = 60 * time.Second
)

var errCatalogPublication = errors.New("session catalog publication could not be completed")

type catalogRefreshStore interface {
	ListCatalogRefreshSessions(context.Context, string, string, int) ([]*models.TaskSession, error)
	PublishSessionCatalog(context.Context, string, string, string, any, sessioncatalog.Observation) (*sessioncatalog.Observation, error)
}

// catalogRefreshRuntime is the lifecycle-owned capability the publisher uses.
// The target of a task probe is always derived from the execution's runtime.
type catalogRefreshRuntime interface {
	CurrentCatalogSavedSource(context.Context, string) (string, error)
	CatalogRefreshExecution(context.Context, string, string) (sessioncatalog.ExecutionEvidence, error)
	DiscoverTaskCatalogCandidates(context.Context, string, sessioncatalog.Owner) (sessioncatalog.TaskProbeReply, error)
}

// catalogCommit runs a write behind the saved-source fences captured by the
// settings request: refresh generation, saved mutation, secret and runtime.
type catalogCommit func(context.Context, func() error) error

type catalogRefresh struct {
	service     *Service
	store       catalogRefreshStore
	runtime     catalogRefreshRuntime
	profileID   string
	agentName   string
	savedSource string
	commit      catalogCommit
	failed      atomic.Bool
}

// PublishProfileCatalog runs only after a successful saved-profile probe. That
// probe gates the work but supplies no candidates: each authorized eligible
// host execution is probed in its own sealed context, at most two at a time
// within one bounded request; every other session receives an honest notice.
func (s *Service) PublishProfileCatalog(ctx context.Context, profileID, agentName string, commit func(context.Context, func() error) error) error {
	if profileID == "" || commit == nil {
		return errors.New("invalid catalog publication")
	}
	store, storeOK := s.repo.(catalogRefreshStore)
	runtime, runtimeOK := s.agentManager.(catalogRefreshRuntime)
	if !storeOK || !runtimeOK {
		return errors.New("catalog publication is unavailable")
	}
	savedSource, err := runtime.CurrentCatalogSavedSource(ctx, profileID)
	if err != nil {
		savedSource = ""
	}
	fanout, cancel := context.WithTimeout(ctx, catalogRefreshFanoutDeadline)
	defer cancel()
	refresh := &catalogRefresh{service: s, store: store, runtime: runtime, profileID: profileID, agentName: agentName, savedSource: savedSource, commit: commit}
	for after := ""; ; {
		if err := fanout.Err(); err != nil {
			return errors.Join(errCatalogPublication, err)
		}
		sessions, err := store.ListCatalogRefreshSessions(fanout, profileID, after, catalogRefreshPageSize)
		if err != nil {
			return errors.Join(errCatalogPublication, err)
		}
		refresh.publishPage(fanout, sessions)
		if len(sessions) < catalogRefreshPageSize {
			break
		}
		after = sessions[len(sessions)-1].ID
	}
	if err := fanout.Err(); err != nil {
		return errors.Join(errCatalogPublication, err)
	}
	if refresh.failed.Load() {
		return errCatalogPublication
	}
	return nil
}

// publishPage authorizes each owning task before any task-side request and
// admits at most catalogRefreshConcurrency sessions; the request owns them all.
func (r *catalogRefresh) publishPage(ctx context.Context, sessions []*models.TaskSession) {
	slots := make(chan struct{}, catalogRefreshConcurrency)
	var wg sync.WaitGroup
	defer wg.Wait()
	for _, session := range sessions {
		if ctx.Err() != nil {
			return
		}
		if err := r.service.authorizeTask(ctx, session.TaskID); err != nil {
			continue
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		wg.Add(1)
		go func(session *models.TaskSession) {
			defer wg.Done()
			defer func() { <-slots }()
			if err := r.publishSession(ctx, session); err != nil {
				r.failed.Store(true)
			}
		}(session)
	}
}

func (r *catalogRefresh) publishSession(ctx context.Context, observed *models.TaskSession) error {
	evidence, err := r.runtime.CatalogRefreshExecution(ctx, observed.ID, r.savedSource)
	if err != nil {
		return err
	}
	if !evidence.Initialized || evidence.ProfileID != r.profileID || evidence.Owner.ExecutionID != observed.AgentExecutionID {
		return nil
	}
	projection := r.acquire(ctx, observed.ID, &evidence)
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.commitSession(ctx, observed, &evidence, projection)
}

// acquire classifies the actual execution. Only a host process launched under
// the current saved source enters fresh task-side acquisition; remote,
// isolated and unknown executors never receive an automatic probe.
func (r *catalogRefresh) acquire(ctx context.Context, sessionID string, evidence *sessioncatalog.ExecutionEvidence) sessioncatalog.Observation {
	projection := sessioncatalog.Observation{
		Version: 1, ProfileID: r.profileID, Owner: evidence.Owner,
		Disposition: sessioncatalog.DispositionRediscoveryRequired, Reason: sessioncatalog.ReasonContextUnverified,
	}
	switch catalogExecutorLocality(evidence.ExecutorType) {
	case catalogLocalityRemote:
		projection.Reason = sessioncatalog.ReasonExecutorScoped
		return projection
	case catalogLocalityUnknown:
		return projection
	}
	if !evidence.SavedSourceCurrent || evidence.AgentID != r.agentName {
		return projection
	}
	probeCtx, cancel := context.WithTimeout(ctx, sessioncatalog.TaskProbeHelperTimeout)
	defer cancel()
	reply, err := r.runtime.DiscoverTaskCatalogCandidates(probeCtx, sessionID, evidence.Owner)
	return taskProbeProjection(projection, reply, err)
}

func (r *catalogRefresh) commitSession(ctx context.Context, observed *models.TaskSession, evidence *sessioncatalog.ExecutionEvidence, projection sessioncatalog.Observation) error {
	s := r.service
	lock := s.sessionSettingsSnapshotLock(observed.ID)
	lock.Lock()
	defer lock.Unlock()
	session, err := s.repo.GetTaskSession(ctx, observed.ID)
	if err != nil {
		return err
	}
	if s.authorizeTask(ctx, session.TaskID) != nil || session.AgentExecutionID != evidence.Owner.ExecutionID {
		return nil
	}
	prior := session.Metadata[sessioncatalog.MetadataKey]
	if previous, ok := loadCatalogObservation(prior); ok && catalogProjectionMatches(&previous, &projection) {
		return nil
	}
	var stored *sessioncatalog.Observation
	err = r.commit(ctx, func() error {
		if err := s.authorizeTask(ctx, session.TaskID); err != nil {
			return err
		}
		var writeErr error
		stored, writeErr = r.store.PublishSessionCatalog(ctx, session.ID, evidence.Owner.ExecutionID, evidence.ProfileID, prior, projection)
		return writeErr
	})
	if err != nil || stored == nil {
		return err
	}
	return s.publishCatalogObservation(ctx, session.TaskID, session.ID, *stored)
}

type catalogLocality int

const (
	catalogLocalityUnknown catalogLocality = iota
	catalogLocalityHost
	catalogLocalityRemote
)

const catalogExecutorUnknown = "unknown"

// catalogExecutorLocality uses the actual executor. Worktree isolation is not
// container isolation; the legacy local_pc alias is host. Empty is unknown.
func catalogExecutorLocality(executorType string) catalogLocality {
	switch models.ExecutorType(executorType) {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree, models.ExecutorType("local_pc"):
		return catalogLocalityHost
	case "", catalogExecutorUnknown:
		return catalogLocalityUnknown
	default:
		return catalogLocalityRemote
	}
}

// taskProbeProjection turns one target's own result into its record. Only a
// complete result supplies candidates; unsupported and failed probes keep an
// honest notice and are never provider startup evidence.
func taskProbeProjection(projection sessioncatalog.Observation, reply sessioncatalog.TaskProbeReply, err error) sessioncatalog.Observation {
	projection.Reason = sessioncatalog.ReasonTaskDiscoveryFailed
	if err != nil {
		return projection
	}
	result := reply.Result
	switch result.Outcome {
	case sessioncatalog.OutcomeUnsupported:
		projection.Reason = sessioncatalog.ReasonTaskDiscoveryUnsupported
		return projection
	case sessioncatalog.OutcomeComplete:
	default:
		return projection
	}
	if sessioncatalog.ValidateDiscovery(&result) != nil || validateCatalogChoices(result.Models.Choices) != nil || validateCatalogChoices(result.Modes.Choices) != nil {
		return projection
	}
	digest, digestErr := catalogProbeDigest(&result)
	if digestErr != nil {
		return projection
	}
	projection.Disposition, projection.Reason = sessioncatalog.DispositionHostCandidates, sessioncatalog.ReasonNewCandidates
	projection.Source, projection.Probe, projection.Digest = sessioncatalog.SourceTaskProbeV1, &result, digest
	if result.Models.Present && result.Models.Supported {
		projection.Models, projection.Catalog = result.Models.Choices, result.Models.Catalog
	}
	if result.Modes.Present && result.Modes.Supported {
		projection.Modes = result.Modes.Choices
	}
	return projection
}

func catalogProbeDigest(result *sessioncatalog.Discovery) (string, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func catalogProjectionMatches(previous, next *sessioncatalog.Observation) bool {
	return previous.Result == nil && previous.Digest == next.Digest && previous.Owner == next.Owner &&
		previous.ProfileID == next.ProfileID && previous.Disposition == next.Disposition &&
		previous.Reason == next.Reason && previous.Source == next.Source
}

func loadCatalogObservation(raw any) (sessioncatalog.Observation, bool) {
	var observation sessioncatalog.Observation
	payload, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(payload, &observation) != nil || observation.Version != 1 || observation.Revision == 0 {
		return observation, false
	}
	return observation, true
}

func validateCatalogChoices(choices []sessioncatalog.Choice) error {
	if len(choices) > sessioncatalog.MaxTaskProbeChoices {
		return errors.New("catalog exceeds choice limit")
	}
	for _, choice := range choices {
		if choice.ID == "" || len(choice.ID) > modelcatalog.MaxIDLength || len(choice.Name) > modelcatalog.MaxStringLength || len(choice.Description) > modelcatalog.MaxStringLength {
			return errors.New("invalid catalog choice")
		}
	}
	return nil
}
