package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/common/modelcatalog"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/task/models"
)

const catalogExecutorUnknown = "unknown"

type catalogRefreshStore interface {
	ListCatalogRefreshSessions(context.Context, string, string, int) ([]*models.TaskSession, error)
	PublishSessionCatalog(context.Context, string, string, string, any, sessioncatalog.Observation) (*sessioncatalog.Observation, error)
}

type catalogRefreshExecutionReader interface {
	CatalogRefreshExecution(context.Context, string, sessioncatalog.LaunchContext) (sessioncatalog.ExecutionEvidence, error)
}

// PublishProfileCatalog publishes only the raw saved-profile probe. Per-task
// authorization is retained throughout the bounded, request-owned traversal.
func (s *Service) PublishProfileCatalog(ctx context.Context, profileID, agentName string, launch hostutility.ProfileProbeContext, caps hostutility.AgentCapabilities, commit func(context.Context, func() error) error) error {
	if caps.Status != hostutility.StatusOK || commit == nil {
		return errors.New("invalid catalog publication")
	}
	store, ok := s.repo.(catalogRefreshStore)
	if !ok {
		return errors.New("catalog repository is unavailable")
	}
	projection, err := catalogProjection(caps)
	if err != nil {
		return err
	}
	projection.ProfileID = profileID
	expected := sessioncatalog.LaunchContext{Env: launch.Env, CLIFlags: launch.CLIFlags, CommandPrefix: launch.CommandPrefix, CopilotSelectedPlugins: launch.CopilotSelectedPlugins}
	var publicationError error
	after := ""
	for {
		if err := ctx.Err(); err != nil {
			return errors.Join(publicationError, err)
		}
		sessions, err := store.ListCatalogRefreshSessions(ctx, profileID, after, 128)
		if err != nil {
			return errors.Join(publicationError, err)
		}
		for _, session := range sessions {
			after = session.ID
			if err := ctx.Err(); err != nil {
				return errors.Join(publicationError, err)
			}
			if err := s.authorizeTask(ctx, session.TaskID); err != nil {
				continue
			}
			if err := s.publishSessionCatalog(ctx, store, session, agentName, expected, projection, commit); err != nil {
				publicationError = errors.New("session catalog publication could not be completed")
			}
		}
		if len(sessions) < 128 {
			return publicationError
		}
	}
}

func (s *Service) publishSessionCatalog(ctx context.Context, store catalogRefreshStore, observed *models.TaskSession, agentName string, expected sessioncatalog.LaunchContext, projection sessioncatalog.Observation, commit func(context.Context, func() error) error) error {
	lock := s.sessionSettingsSnapshotLock(observed.ID)
	lock.Lock()
	defer lock.Unlock()
	session, err := s.repo.GetTaskSession(ctx, observed.ID)
	if err != nil {
		return err
	}
	if err := s.authorizeTask(ctx, session.TaskID); err != nil {
		return nil
	}
	reader, ok := s.agentManager.(catalogRefreshExecutionReader)
	if !ok {
		return errors.New("catalog execution evidence is unavailable")
	}
	evidence, err := reader.CatalogRefreshExecution(ctx, session.ID, expected)
	if err != nil {
		return err
	}
	if !evidence.Initialized || evidence.ProfileID != projection.ProfileID || evidence.Owner.ExecutionID != session.AgentExecutionID {
		return nil
	}
	prepareCatalogProjection(&projection, &evidence, agentName)
	prior := session.Metadata[sessioncatalog.MetadataKey]
	if previous, ok := loadCatalogObservation(prior); ok && catalogProjectionMatches(&previous, &projection) {
		return nil
	}
	var stored *sessioncatalog.Observation
	err = commit(ctx, func() error {
		if err := s.authorizeTask(ctx, session.TaskID); err != nil {
			return err
		}
		var writeErr error
		stored, writeErr = store.PublishSessionCatalog(ctx, session.ID, evidence.Owner.ExecutionID, evidence.ProfileID, prior, projection)
		return writeErr
	})
	if err != nil || stored == nil {
		return err
	}
	return s.publishCatalogObservation(ctx, session.TaskID, session.ID, *stored)
}

func prepareCatalogProjection(projection *sessioncatalog.Observation, evidence *sessioncatalog.ExecutionEvidence, agentName string) {
	projection.Owner = evidence.Owner
	projection.Disposition = sessioncatalog.DispositionRediscoveryRequired
	projection.Reason = "executor_scoped"
	host := false
	switch models.ExecutorType(evidence.ExecutorType) {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree, models.ExecutorType("local_pc"):
		host = true
	}
	if host && evidence.Compatible && evidence.AgentID == agentName {
		projection.Disposition = sessioncatalog.DispositionHostCandidates
		projection.Reason = sessioncatalog.ReasonNewCandidates
		return
	}
	if host || evidence.ExecutorType == "" || evidence.ExecutorType == catalogExecutorUnknown {
		projection.Reason = "context_unverified"
	}
	projection.Models = nil
	projection.Modes = nil
	projection.Catalog = nil
}

func catalogProjectionMatches(previous, next *sessioncatalog.Observation) bool {
	return previous.Result == nil && previous.Digest == next.Digest && previous.Owner == next.Owner &&
		previous.ProfileID == next.ProfileID && previous.Disposition == next.Disposition && previous.Reason == next.Reason
}

func loadCatalogObservation(raw any) (sessioncatalog.Observation, bool) {
	var observation sessioncatalog.Observation
	payload, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(payload, &observation) != nil || observation.Version != 1 || observation.Revision == 0 {
		return observation, false
	}
	return observation, true
}

func catalogProjection(caps hostutility.AgentCapabilities) (sessioncatalog.Observation, error) {
	projection := sessioncatalog.Observation{Version: 1}
	if len(caps.Models) > modelcatalog.MaxNodes || len(caps.Modes) > modelcatalog.MaxNodes {
		return projection, errors.New("catalog exceeds choice limit")
	}
	projection.Models = make([]sessioncatalog.Choice, 0, len(caps.Models))
	for _, model := range caps.Models {
		projection.Models = append(projection.Models, sessioncatalog.Choice{ID: model.ID, Name: model.Name, Description: model.Description})
	}
	projection.Modes = make([]sessioncatalog.Choice, 0, len(caps.Modes))
	for _, mode := range caps.Modes {
		projection.Modes = append(projection.Modes, sessioncatalog.Choice{ID: mode.ID, Name: mode.Name, Description: mode.Description})
	}
	for _, option := range caps.ConfigOptions {
		if option.Category != sessionModelConfigKey && option.ID != sessionModelConfigKey {
			continue
		}
		if option.Catalog != nil && modelcatalog.Validate(option.Catalog) == nil {
			projection.Catalog = option.Catalog
		}
		if len(projection.Models) == 0 {
			for _, choice := range option.Options {
				projection.Models = append(projection.Models, sessioncatalog.Choice{ID: choice.Value, Name: choice.Name, Description: choice.Description})
			}
		}
		break
	}
	for _, choices := range [][]sessioncatalog.Choice{projection.Models, projection.Modes} {
		if err := validateCatalogChoices(choices); err != nil {
			return projection, err
		}
	}
	payload, err := json.Marshal(struct {
		Models, Modes []sessioncatalog.Choice
		Catalog       *modelcatalog.Catalog
	}{projection.Models, projection.Modes, projection.Catalog})
	if err != nil {
		return projection, fmt.Errorf("encode catalog: %w", err)
	}
	digest := sha256.Sum256(payload)
	projection.Digest = hex.EncodeToString(digest[:])
	return projection, nil
}

func validateCatalogChoices(choices []sessioncatalog.Choice) error {
	if len(choices) > modelcatalog.MaxNodes {
		return errors.New("catalog exceeds choice limit")
	}
	for _, choice := range choices {
		if choice.ID == "" || len(choice.ID) > modelcatalog.MaxIDLength || len(choice.Name) > modelcatalog.MaxStringLength || len(choice.Description) > modelcatalog.MaxStringLength {
			return errors.New("invalid catalog choice")
		}
	}
	return nil
}
