package github

import (
	"context"
	"fmt"
	"sync"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// PersonalStarDeployment carries the deployment-owned pieces the GitHub
// provider needs to build the dedicated personal-star runtime. A nil or
// unavailable deployment leaves the feature off: connection status reports
// available=false and no OAuth or provisioning path is reachable.
type PersonalStarDeployment struct {
	// Available reports whether this deployment can serve dedicated star
	// connections (supported platform plus complete, resolvable settings).
	Available bool
	// Settings holds the dedicated OAuth App client id and callback URL. The
	// client secret is never held here.
	Settings PersonalStarOAuthSettings
	// ReadClientSecret resolves the deployment-owned client secret. It is only
	// called when Available is true.
	ReadClientSecret func(context.Context) (string, error)
	// Close releases the deployment-owned resolver handles.
	Close func() error
}

// resolve applies the deployment-owned client secret. Any failure downgrades
// the feature to unavailable rather than failing provider startup.
func (d PersonalStarDeployment) resolve(ctx context.Context) (PersonalStarDeployment, error) {
	if !d.Available || d.ReadClientSecret == nil {
		return PersonalStarDeployment{}, nil
	}
	secret, err := d.ReadClientSecret(ctx)
	if err != nil {
		return PersonalStarDeployment{}, fmt.Errorf("resolve personal star client secret: %w", err)
	}
	settings := d.Settings
	settings.ClientSecret = secret
	if !settings.Configured() {
		return PersonalStarDeployment{}, nil
	}
	return PersonalStarDeployment{Available: true, Settings: settings, Close: d.Close}, nil
}

// deploymentOwner guarantees the deployment-owned resolver handles are released
// exactly once: either on a construction failure or, after a successful
// handoff, by the service cleanup registered as Services.GitHubCleanup.
type deploymentOwner struct {
	mu         sync.Mutex
	deployment PersonalStarDeployment
	released   bool
}

func newDeploymentOwner(deployment PersonalStarDeployment) *deploymentOwner {
	return &deploymentOwner{deployment: deployment}
}

func (o *deploymentOwner) release() {
	if o == nil {
		return
	}
	o.mu.Lock()
	deployment, released := o.deployment, o.released
	o.released = true
	o.mu.Unlock()
	if released {
		return
	}
	closePersonalStarDeployment(deployment)
}

func (o *deploymentOwner) transfer() func() error {
	if o == nil {
		return nil
	}
	return func() error {
		o.release()
		return nil
	}
}

// personalStarRuntime groups the dedicated star components attached to a
// service so shutdown can drain them in a fixed order.
type personalStarRuntime struct {
	runner *StarSecretCleanupRunner
	close  func() error
}

// applyPersonalStarRuntime builds and attaches the dedicated star runtime.
// Construction runs before any of it is observable, and the caller-provided
// deployment owner is released on every failure path so a rejected build never
// leaks the resolver handles.
func applyPersonalStarRuntime(
	svc *Service,
	store *Store,
	secrets PersonalStarSecrets,
	deployment PersonalStarDeployment,
	log *logger.Logger,
) (*personalStarRuntime, error) {
	resolved, err := deployment.resolve(context.Background())
	if err != nil {
		// A reference that cannot be resolved (not provisioned, revoked,
		// outside the protected namespace, or bound to another installation)
		// disables the feature without failing provider startup: the backend
		// stays healthy and the connection reports available=false.
		log.Warn("GitHub personal star client secret is unavailable", zap.Error(err))
		return nil, nil
	}
	if !resolved.Available {
		return nil, nil
	}
	oauthService := NewPersonalStarOAuthService(store, secrets, resolved.Settings, true)
	resolver := NewPersonalStarResolverService(store, secrets, PersonalStarDeploymentStatus{
		Available: true,
		Settings:  resolved.Settings,
	})
	runner := NewStarSecretCleanupRunner(store, secrets, log)
	svc.SetPersonalStarOAuthCoordinator(oauthService)
	svc.SetPersonalStarResolver(resolver)
	return &personalStarRuntime{runner: runner, close: resolved.Close}, nil
}

// start makes the cleanup runner observable. It runs after construction so a
// failed build never leaves a worker behind.
func (r *personalStarRuntime) start() {
	if r == nil || r.runner == nil {
		return
	}
	r.runner.Start()
}

// stop drains the cleanup runner before releasing the resolver handles.
func (r *personalStarRuntime) stop() {
	if r == nil {
		return
	}
	if r.runner != nil {
		r.runner.Stop()
	}
	if r.close != nil {
		_ = r.close()
	}
}

func closePersonalStarDeployment(deployment PersonalStarDeployment) {
	if deployment.Close == nil {
		return
	}
	_ = deployment.Close()
}

// Provide creates the full GitHub integration stack: store, client, and service.
// Returns the service, a cleanup function, and any error.
func Provide(
	writer, reader *sqlx.DB,
	secrets SecretProvider,
	connectionSecrets ConnectionSecretStore,
	eventBus bus.EventBus,
	log *logger.Logger,
	deployment PersonalStarDeployment,
) (*Service, func() error, error) {
	owner := newDeploymentOwner(deployment)
	store, err := NewStore(writer, reader)
	if err != nil {
		owner.release()
		return nil, nil, err
	}

	ctx := context.Background()
	client, authMethod, err := NewClient(ctx, secrets, log)
	if err != nil {
		// Not fatal — service works with nil client (unauthenticated).
		log.Warn("GitHub client not available: " + err.Error())
	}

	svc := NewService(client, authMethod, secrets, store, eventBus, log)
	if connectionSecrets != nil {
		svc.SetConnectionSecretStore(connectionSecrets)
	}
	attachRateTracker(client, svc.RateTracker(), log)
	svc.subscribeTaskEvents()

	var starRuntime *personalStarRuntime
	if connectionSecrets != nil {
		starRuntime, err = applyPersonalStarRuntime(svc, store, connectionSecrets, deployment, log)
	}
	if err != nil {
		svc.Stop()
		svc.unsubscribeTaskEvents()
		owner.release()
		return nil, nil, err
	}
	if starRuntime != nil {
		starRuntime.close = owner.transfer()
	} else {
		owner.release()
	}

	cleanup := func() error {
		// Stop background goroutines (currently the per-workspace
		// stale-PR refreshes) before tearing down event subs so an
		// in-flight refresh's DB writes don't race with shutdown.
		svc.Stop()
		svc.unsubscribeTaskEvents()
		starRuntime.stop()
		return nil
	}
	if starRuntime != nil {
		starRuntime.start()
	}
	return svc, cleanup, nil
}
