package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Star actor sources reported by the connection status. Workspace sources reuse
// the ConnectionSource spelling; the dedicated OAuth App has its own value.
const (
	StarActorSourceDedicated = "personal_star_oauth"

	StarConnectDedicated = "personal_star_oauth"
	StarConnectAppUser   = "github_app_user"

	StarRemediationTokenScope    = "token_scope"
	StarRemediationAppPermission = "app_permission"

	starClassicRepoScope = "repo"
	starScopeProbeTTL    = 10 * time.Minute
)

// PersonalStarActor names the GitHub identity star calls act as. It carries no
// credential material.
type PersonalStarActor struct {
	Source string `json:"source"`
	Login  string `json:"login"`
	Shared bool   `json:"shared"`
}

// starActor is one resolved, capability-checked identity plus its token.
type starActor struct {
	PersonalStarActor
	generation int64
	capable    bool
	remedy     string
	token      string
	key        string
}

type starScopeProbe struct {
	capable bool
	remedy  string
	at      time.Time
}

// StarCapabilityCache records per-actor star capability. Keys include the
// credential generation, so any credential change starts from a clean slate.
type StarCapabilityCache struct {
	mu     sync.Mutex
	probes map[string]starScopeProbe
	denied map[string]string
	now    func() time.Time
}

func NewStarCapabilityCache() *StarCapabilityCache {
	return &StarCapabilityCache{
		probes: make(map[string]starScopeProbe),
		denied: make(map[string]string),
		now:    time.Now,
	}
}

func (c *StarCapabilityCache) deniedRemedy(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	remedy, ok := c.denied[key]
	return remedy, ok
}
func (c *StarCapabilityCache) deny(key, remedy string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.denied[key] = remedy
}
func (c *StarCapabilityCache) probe(key string) (starScopeProbe, bool) {
	if c == nil {
		return starScopeProbe{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.probes[key]
	if !ok || c.now().Sub(entry.at) > starScopeProbeTTL {
		return starScopeProbe{}, false
	}
	return entry, true
}
func (c *StarCapabilityCache) storeProbe(key string, entry starScopeProbe) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry.at = c.now()
	c.probes[key] = entry
}

func starActorKey(workspaceID, source, login string, generation int64) string {
	return strings.Join([]string{workspaceID, source, strings.ToLower(login), strconv.FormatInt(generation, 10)}, "\x00")
}

// starRemedyForSource names the operator remediation a denial implies.
func starRemedyForSource(source string) string {
	if source == string(ConnectionSourceGitHubAppUser) {
		return StarRemediationAppPermission
	}
	return StarRemediationTokenScope
}

// classifyOAuthScopes maps an X-OAuth-Scopes header to star capability. An
// absent header means a token without classic scopes (fine-grained PAT): it is
// treated as capable until GitHub denies a star call.
func classifyOAuthScopes(header string, present bool) starScopeProbe {
	if !present {
		return starScopeProbe{capable: true}
	}
	for _, scope := range strings.Split(header, ",") {
		switch strings.TrimSpace(scope) {
		case starClassicRepoScope, PersonalStarScope:
			return starScopeProbe{capable: true}
		}
	}
	return starScopeProbe{remedy: StarRemediationTokenScope}
}

// probeStarScopes reads the token's classic OAuth scopes from GET /user.
func probeStarScopes(ctx context.Context, token string) (starScopeProbe, error) {
	client := NewTokenClient(token, TokenPrincipal{Kind: TokenCredentialPersonalStar})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIBase+"/user", nil)
	if err != nil {
		return starScopeProbe{}, err
	}
	client.setGitHubHeaders(request)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return starScopeProbe{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return starScopeProbe{}, fmt.Errorf("GitHub scope probe returned %d", response.StatusCode)
	}
	values, present := response.Header[http.CanonicalHeaderKey("X-OAuth-Scopes")]
	header := ""
	if present && len(values) > 0 {
		header = values[0]
	}
	return classifyOAuthScopes(header, present), nil
}

// resolveCredentialStarActor resolves the non-dedicated star actor: the user's
// GitHub App personal connection, otherwise the workspace's human PAT, gh CLI,
// or legacy credential. An App installation never resolves. It returns nil
// when no human credential is available.
func (s *Service) resolveCredentialStarActor(ctx context.Context, workspaceID, userID string) *starActor {
	if s == nil || s.resolver == nil {
		return nil
	}
	resolved, err := s.resolver.Resolve(ctx, ResolveCredentialRequest{
		WorkspaceID: workspaceID, UserID: userID, Purpose: CredentialPurposePersonalRead,
	})
	if err != nil || resolved == nil || resolved.Principal.Kind != AuthPrincipalHuman {
		return nil
	}
	token := resolved.credential
	if strings.TrimSpace(token) == "" && resolved.Principal.Source == ConnectionSourceLegacyShared {
		transport, transportErr := s.resolver.Resolve(ctx, ResolveCredentialRequest{
			WorkspaceID: workspaceID, UserID: userID, Purpose: CredentialPurposeGitTransport,
		})
		if transportErr == nil && transport != nil {
			token = transport.credential
		}
	}
	if strings.TrimSpace(token) == "" {
		return nil
	}
	source := string(resolved.Principal.Source)
	actor := &starActor{
		PersonalStarActor: PersonalStarActor{
			Source: source,
			Login:  resolved.Principal.Login,
			Shared: resolved.Principal.Source != ConnectionSourceGitHubAppUser,
		},
		generation: resolved.CredentialGeneration,
		token:      token,
		key:        starActorKey(workspaceID, source, resolved.Principal.Login, resolved.CredentialGeneration),
	}
	s.evaluateStarCapability(ctx, actor)
	return actor
}

func (s *Service) evaluateStarCapability(ctx context.Context, actor *starActor) {
	cache := s.starCapabilities
	if remedy, denied := cache.deniedRemedy(actor.key); denied {
		actor.remedy = remedy
		return
	}
	// App user tokens and mock credentials expose no classic scopes; they are
	// capable until GitHub denies a star call.
	if actor.Source == string(ConnectionSourceGitHubAppUser) || s.mockAuth != nil {
		actor.capable = true
		return
	}
	if entry, ok := cache.probe(actor.key); ok {
		actor.capable, actor.remedy = entry.capable, entry.remedy
		return
	}
	entry, err := probeStarScopes(ctx, actor.token)
	if err != nil {
		return
	}
	cache.storeProbe(actor.key, entry)
	actor.capable, actor.remedy = entry.capable, entry.remedy
}

func (s *Service) starClientForActor(actor *starActor, workspaceID, userID string) PersonalStarClient {
	if resolver, ok := s.personalStarResolver.(*PersonalStarResolverService); ok && resolver.builder != nil {
		return resolver.builder(actor.token, workspaceID, userID)
	}
	return NewPersonalStarClient(actor.token)
}

// recordStarDenial marks the actor incapable after GitHub refuses a star call.
func (s *Service) recordStarDenial(actor *starActor, err error) {
	var apiError *GitHubAPIError
	if actor == nil || !errors.As(err, &apiError) || apiError.StatusCode != http.StatusForbidden {
		return
	}
	s.starCapabilities.deny(actor.key, starRemedyForSource(actor.Source))
}

// starConnectPath names the consent flow that can produce an actor when none
// resolved.
func (s *Service) starConnectPath(ctx context.Context, workspaceID, userID string, dedicatedAvailable bool) string {
	if dedicatedAvailable {
		return StarConnectDedicated
	}
	if s == nil || s.store == nil {
		return ""
	}
	connection, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil || connection == nil || connection.Source != ConnectionSourceGitHubAppInstallation ||
		connection.Status != ConnectionStatusActive {
		return ""
	}
	personal, err := s.store.GetUserConnection(ctx, workspaceID, userID)
	if err != nil || (personal != nil && personal.Status == ConnectionStatusActive) {
		return ""
	}
	return StarConnectAppUser
}
