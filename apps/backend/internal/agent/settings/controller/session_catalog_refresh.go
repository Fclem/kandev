package controller

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"time"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

const catalogPublicationStatusFailed = "failed"

// ProfileCatalogRefreshResponseBudget is the write deadline of a synchronous
// saved refresh: the forced saved-profile probe, then the bounded session
// fan-out, plus a margin for the surrounding reads, fences and response.
const ProfileCatalogRefreshResponseBudget = hostutility.ProbeTimeout + sessioncatalog.TaskProbeFanoutTimeout + 30*time.Second

// catalogRefreshFlight is the one saved refresh per profile allowed to
// publish. cancel stops its session fan-out once a newer refresh or a saved
// mutation fences it, so a refresh that can no longer commit frees each
// task's single helper slot instead of probing on.
type catalogRefreshFlight struct {
	generation uint64
	cancel     context.CancelFunc
}

// beginCatalogRefresh supersedes the profile's previous flight and returns the
// context its publication must run under.
func (c *Controller) beginCatalogRefresh(ctx context.Context, profileID string) (uint64, context.Context, func()) {
	flightCtx, cancel := context.WithCancel(ctx)
	c.catalogRefreshMu.Lock()
	defer c.catalogRefreshMu.Unlock()
	if c.catalogRefreshFlights == nil {
		c.catalogRefreshFlights = make(map[string]catalogRefreshFlight)
	}
	if previous, ok := c.catalogRefreshFlights[profileID]; ok {
		previous.cancel()
	}
	c.catalogRefreshGeneration++
	generation := c.catalogRefreshGeneration
	c.catalogRefreshFlights[profileID] = catalogRefreshFlight{generation: generation, cancel: cancel}
	return generation, flightCtx, func() {
		cancel()
		c.catalogRefreshMu.Lock()
		defer c.catalogRefreshMu.Unlock()
		if c.catalogRefreshFlights[profileID].generation == generation {
			delete(c.catalogRefreshFlights, profileID)
		}
	}
}

// Saved writes fence publication before persistence begins without retaining a
// controller lock across repository or runtime callbacks.
func (c *Controller) beginCatalogProfileMutation(profileID string) func() {
	c.catalogRefreshMu.Lock()
	if c.catalogProfileMutations == nil {
		c.catalogProfileMutations = make(map[string]uint32)
	}
	c.catalogProfileMutations[profileID]++
	if flight, ok := c.catalogRefreshFlights[profileID]; ok {
		flight.cancel()
		delete(c.catalogRefreshFlights, profileID)
	}
	c.catalogRefreshMu.Unlock()
	return func() {
		c.catalogRefreshMu.Lock()
		defer c.catalogRefreshMu.Unlock()
		if c.catalogProfileMutations[profileID] <= 1 {
			delete(c.catalogProfileMutations, profileID)
		} else {
			c.catalogProfileMutations[profileID]--
		}
	}
}

// CatalogRefreshPublisher fans a successful saved-profile probe out to the
// profile's authorized live sessions. It receives only the saved-source
// identity and the commit fence, never the generic probe's choices.
type CatalogRefreshPublisher func(ctx context.Context, profileID, agentName string, commit func(context.Context, func() error) error) error

// SetCatalogRefreshPublisher connects saved discovery to authorized session publication.
func (c *Controller) SetCatalogRefreshPublisher(publish CatalogRefreshPublisher) {
	c.catalogRefreshPublisher = publish
}

// RefreshProfileModelCatalog discovers the saved profile, never an editor draft.
func (c *Controller) RefreshProfileModelCatalog(ctx context.Context, profileID, authorizationScope string) (*dto.DynamicModelsResponse, error) {
	if profileID == "" || authorizationScope == "" || c.repo == nil {
		return nil, ErrInvalidProfileDiscovery
	}
	generation, flightCtx, finish := c.beginCatalogRefresh(ctx, profileID)
	defer finish()
	profile, err := c.repo.GetAgentProfile(ctx, profileID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && profile == nil) {
		return nil, ErrProfileDiscoveryMissing
	}
	if err != nil {
		return nil, err
	}
	agentName, err := c.agentNameForProfile(ctx, profile)
	if err != nil {
		return nil, err
	}
	return c.fetchProfileDynamicModels(ctx, agentName, dto.ProfileCapabilityRequest{
		ProfileID: profileID, AuthorizationScope: authorizationScope, Refresh: true,
	}, func(launch hostutility.ProfileProbeContext, result hostutility.ProfileCapabilityResult) *dto.CatalogPublicationDTO {
		return c.publishSavedProfileCatalog(flightCtx, profile, agentName, authorizationScope, generation, launch, result)
	})
}

func (c *Controller) publishSavedProfileCatalog(
	ctx context.Context,
	profile *models.AgentProfile,
	agentName, scope string,
	generation uint64,
	launch hostutility.ProfileProbeContext,
	result hostutility.ProfileCapabilityResult,
) *dto.CatalogPublicationDTO {
	if result.Capabilities.Status != hostutility.StatusOK {
		return &dto.CatalogPublicationDTO{Status: "not_published"}
	}
	provider, ok := c.hostUtility.(interface {
		CommitProfileCapability(context.Context, hostutility.ProfileCapabilityResult, func() error) error
	})
	if !ok || c.catalogRefreshPublisher == nil {
		return c.catalogPublicationFailed(profile.ID, agentName, catalogPublicationReasonUnavailable, nil)
	}
	commit := func(ctx context.Context, write func() error) error {
		return provider.CommitProfileCapability(ctx, result, func() error {
			c.catalogRefreshMu.Lock()
			defer c.catalogRefreshMu.Unlock()
			if c.catalogRefreshFlights[profile.ID].generation != generation || c.catalogProfileMutations[profile.ID] != 0 {
				return ErrInvalidProfileDiscovery
			}
			if err := c.checkSavedCatalogContext(ctx, profile, agentName, scope, launch); err != nil {
				return err
			}
			return write()
		})
	}
	if err := commit(ctx, func() error { return nil }); err != nil {
		return c.catalogPublicationFailed(profile.ID, agentName, catalogPublicationReasonPrecheck, err)
	}
	if err := c.catalogRefreshPublisher(ctx, profile.ID, agentName, commit); err != nil {
		return c.catalogPublicationFailed(profile.ID, agentName, catalogPublicationReasonFanout, err)
	}
	return &dto.CatalogPublicationDTO{Status: "published"}
}

func (c *Controller) checkSavedCatalogContext(
	ctx context.Context, profile *models.AgentProfile, agentName, scope string, launch hostutility.ProfileProbeContext,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	saved, err := c.repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(profile, saved) {
		return ErrInvalidProfileDiscovery
	}
	latest, err := c.resolveProfileProbeContext(ctx, agentName, scope, profile.ID, nil, nil)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(launch, latest) {
		return ErrInvalidProfileDiscovery
	}
	return nil
}

// Stable publication failure categories for logs. Error text from storage,
// runtime or provider paths can carry private detail, so logs record only a
// category and a sanitized cause, never the error itself.
const (
	catalogPublicationReasonUnavailable = "publisher_unavailable"
	catalogPublicationReasonPrecheck    = "precheck_fence"
	catalogPublicationReasonFanout      = "fanout_failed"
)

func (c *Controller) catalogPublicationFailed(profileID, agentName, reason string, err error) *dto.CatalogPublicationDTO {
	c.logger.Warn("saved catalog publication failed",
		zap.String("profile_id", profileID), zap.String("agent", agentName),
		zap.String("reason", reason), zap.String("cause", sessioncatalog.FailureCause(err)))
	return &dto.CatalogPublicationDTO{Status: catalogPublicationStatusFailed, Error: "session catalog publication could not be completed"}
}
