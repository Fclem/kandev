package backendapp

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/gitcredentials"
	githubpkg "github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	gitCredentialGitHubProviderID = "github"
	gitCredentialGitHubHost       = "github.com"
)

type gitCredentialTaskRepository interface {
	GetTask(context.Context, string) (*taskmodels.Task, error)
	GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error)
	GetRepository(context.Context, string) (*taskmodels.Repository, error)
	ListTaskRepositories(context.Context, string) ([]*taskmodels.TaskRepository, error)
}

func newGitCredentialBroker(
	githubSvc *githubpkg.Service,
	pluginSvc *plugins.Service,
	repo gitCredentialTaskRepository,
) *gitcredentials.Broker {
	resolvers := make([]gitcredentials.Resolver, 0, 2)
	if githubSvc != nil {
		resolvers = append(resolvers, githubSvc.GitCredentialResolver())
	}
	if pluginSvc != nil {
		resolvers = append(resolvers, pluginGitCredentialResolver{service: pluginCredentialServiceAdapter{service: pluginSvc}})
	}
	return gitcredentials.NewBroker(gitcredentials.NewCompositeResolver(resolvers...), &githubBrokerScopeAuthorizer{repo: repo})
}

// pluginGitCredentialResolver resolves only live, active manifest-declared
// repository providers. It never caches a returned secret, so each helper
// redemption observes plugin-side OAuth rotation.
type pluginGitCredentialResolver struct {
	service pluginCredentialService
}

type pluginCredentialRemote interface {
	ResolveGitCredential(context.Context, *pluginsdk.ResolveGitCredentialRequest) (*pluginsdk.ResolveGitCredentialResponse, error)
}

type pluginCredentialBindingRemote interface {
	GetGitCredentialBinding(context.Context, *pluginsdk.GitCredentialBindingRequest) (*pluginsdk.GitCredentialBindingResponse, error)
}

type pluginCredentialService interface {
	Provider(string) (id, version string, remote pluginCredentialRemote, found bool)
}

type pluginCredentialServiceAdapter struct{ service *plugins.Service }

func (a pluginCredentialServiceAdapter) Provider(providerID string) (string, string, pluginCredentialRemote, bool) {
	if a.service == nil || a.service.Registry() == nil || a.service.Runtime() == nil {
		return "", "", nil, false
	}
	for _, record := range a.service.Registry().List() {
		if record.Status != plugins.StatusActive || !declaresProvider(record.RepositoryProviders, providerID) {
			continue
		}
		remote, running := a.service.Runtime().Get(record.ID)
		if !running || remote == nil {
			return "", "", nil, false
		}
		return record.ID, record.Version, remote, true
	}
	return "", "", nil, false
}

func (r pluginGitCredentialResolver) Supports(providerID string) bool {
	_, _, _, found := r.provider(providerID)
	return found
}

func (r pluginGitCredentialResolver) Binding(ctx context.Context, scope gitcredentials.Scope) (string, error) {
	_, _, remote, found := r.provider(scope.ProviderID)
	if !found {
		return "", gitcredentials.ErrUnsupported
	}
	binder, ok := remote.(pluginCredentialBindingRemote)
	if !ok {
		// A legacy plugin can still be active and resolve credentials, but its
		// generation cannot prove a lease remained valid after issuance.
		return "", gitcredentials.ErrUnsupported
	}
	response, err := binder.GetGitCredentialBinding(ctx, &pluginsdk.GitCredentialBindingRequest{
		ProviderID: scope.ProviderID, WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID,
		SessionID: scope.SessionID, RepositoryID: scope.RepositoryID, Host: scope.Host, Path: scope.Path,
	})
	if err != nil {
		return "", fmt.Errorf("resolve plugin Git credential binding: %w", err)
	}
	if response == nil || strings.TrimSpace(response.Binding) == "" {
		return "", gitcredentials.ErrLeaseRevoked
	}
	return response.Binding, nil
}

func (r pluginGitCredentialResolver) Resolve(ctx context.Context, scope gitcredentials.Scope) (gitcredentials.Credential, error) {
	_, _, remote, found := r.provider(scope.ProviderID)
	if !found {
		return gitcredentials.Credential{}, gitcredentials.ErrUnsupported
	}
	response, err := remote.ResolveGitCredential(ctx, &pluginsdk.ResolveGitCredentialRequest{
		ProviderID: scope.ProviderID, WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID,
		SessionID: scope.SessionID, RepositoryID: scope.RepositoryID, Host: scope.Host, Path: scope.Path,
	})
	if err != nil {
		return gitcredentials.Credential{}, fmt.Errorf("resolve plugin Git credential: %w", err)
	}
	if response == nil || strings.TrimSpace(response.Username) == "" || strings.TrimSpace(response.Secret) == "" {
		return gitcredentials.Credential{}, gitcredentials.ErrLeaseRevoked
	}
	expiresAt, err := pluginCredentialExpiry(response.ExpiresAt)
	if err != nil {
		return gitcredentials.Credential{}, err
	}
	return gitcredentials.Credential{Username: response.Username, Password: response.Secret, ExpiresAt: expiresAt}, nil
}

func (r pluginGitCredentialResolver) provider(providerID string) (string, string, pluginCredentialRemote, bool) {
	if r.service == nil {
		return "", "", nil, false
	}
	return r.service.Provider(providerID)
}

func declaresProvider(providers []string, want string) bool {
	for _, provider := range providers {
		if strings.EqualFold(strings.TrimSpace(provider), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func pluginCredentialExpiry(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	expiresAt, err := time.Parse(time.RFC3339, raw)
	if err != nil || !expiresAt.After(time.Now()) {
		return time.Time{}, fmt.Errorf("plugin returned invalid Git credential expiry")
	}
	return expiresAt, nil
}

func (a *githubBrokerScopeAuthorizer) AuthorizeGitCredential(ctx context.Context, scope gitcredentials.Scope) error {
	if a == nil || a.repo == nil {
		return fmt.Errorf("task repository is unavailable")
	}
	if err := a.authorizeTaskSession(ctx, scope.WorkspaceID, scope.TaskID, scope.SessionID); err != nil {
		return err
	}
	if err := a.authorizeTaskRepository(ctx, scope.TaskID, scope.RepositoryID); err != nil {
		return err
	}
	return a.authorizeRepositoryIdentity(ctx, scope)
}

func (a *githubBrokerScopeAuthorizer) authorizeRepositoryIdentity(ctx context.Context, scope gitcredentials.Scope) error {
	repository, err := a.repo.GetRepository(ctx, scope.RepositoryID)
	if err != nil {
		return err
	}
	if repository == nil || repository.WorkspaceID != scope.WorkspaceID ||
		!strings.EqualFold(repository.Provider, scope.ProviderID) {
		return fmt.Errorf("repository identity does not match lease scope")
	}
	host, path, err := repositoryHTTPSIdentity(repository)
	if err != nil || !strings.EqualFold(host, scope.Host) || path != scope.Path {
		return fmt.Errorf("repository identity does not match lease scope")
	}
	return nil
}

func repositoryHTTPSIdentity(repository *taskmodels.Repository) (string, string, error) {
	if repository == nil {
		return "", "", fmt.Errorf("repository is required")
	}
	remoteURL := strings.TrimSpace(repository.RemoteURL)
	if remoteURL == "" && strings.EqualFold(repository.Provider, gitCredentialGitHubProviderID) &&
		repository.ProviderOwner != "" && repository.ProviderName != "" {
		remoteURL = "https://" + gitCredentialGitHubHost + "/" + repository.ProviderOwner + "/" + repository.ProviderName + ".git"
	}
	parsed, err := url.Parse(remoteURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path == "" {
		return "", "", fmt.Errorf("repository HTTPS clone URL is unavailable")
	}
	return strings.ToLower(parsed.Host), parsed.Path, nil
}
