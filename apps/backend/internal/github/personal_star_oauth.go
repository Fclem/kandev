package github

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	// PersonalStarFlowLifetime bounds how long a started consent can complete.
	PersonalStarFlowLifetime = 10 * time.Minute
	// PersonalStarScope is the only scope the dedicated flow requests.
	PersonalStarScope = "public_repo"
	// PersonalStarCallbackPath is the dedicated callback route.
	PersonalStarCallbackPath = "/api/v1/github/user/starred/oauth/callback"

	githubWebBase = "https://github.com"
	// PKCEChallengeMethodS256 is the only accepted S256 challenge method.
	PKCEChallengeMethodS256 = "S256"

	personalStarParamState    = "state"
	personalStarParamCode     = "code"
	personalStarParamClientID = "client_id"
	personalStarParamError    = "error"
	personalStarStateBytes    = 32
	personalStarVerifierBytes = 32
	personalStarReturnPrefix  = "/settings/plugins"

	personalStarOutcomeConnected         = "connected"
	personalStarOutcomeDeclined          = "declined"
	personalStarOutcomeInsufficientScope = "insufficient_scope"
	personalStarOutcomeFailed            = "failed"
)

var (
	// ErrPersonalStarInvalidCallback reports a malformed callback query.
	ErrPersonalStarInvalidCallback = errors.New("github personal star callback is invalid")
	// ErrPersonalStarReturnPath reports a return path outside Settings > Plugins.
	ErrPersonalStarReturnPath = errors.New("github personal star return path is invalid")
)

// PersonalStarOAuthSettings is the deployment-owned dedicated OAuth App
// configuration. The client secret is resolved at startup and never leaves the
// backend process.
type PersonalStarOAuthSettings struct {
	ClientID     string
	ClientSecret string
	CallbackURL  string
}

// Configured reports whether every required setting is present.
func (s PersonalStarOAuthSettings) Configured() bool {
	return strings.TrimSpace(s.ClientID) != "" &&
		strings.TrimSpace(s.ClientSecret) != "" &&
		strings.TrimSpace(s.CallbackURL) != ""
}

// GitHubStarCallbackQuery is a validated callback query.
type GitHubStarCallbackQuery struct {
	State            string
	Code             string
	Error            string
	ErrorDescription string
	ErrorURI         string
}

// Denied reports whether the callback carried an OAuth error.
func (q GitHubStarCallbackQuery) Denied() bool {
	return q.Error != ""
}

// PersonalStarStartRequest is one authenticated consent start.
type PersonalStarStartRequest struct {
	WorkspaceID    string
	UserID         string
	SessionSubject string
	SessionID      string
	ReturnPath     string
}

// PersonalStarStartResult carries the authorization URL and its state expiry.
type PersonalStarStartResult struct {
	AuthorizationURL string
	StateExpiresAt   time.Time
}

// PersonalStarCallbackOutcome is the navigation-only result of a callback.
type PersonalStarCallbackOutcome struct {
	WorkspaceID string
	UserID      string
	ReturnPath  string
	Outcome     string
	Active      bool
	Capable     bool
}

// PersonalStarOAuthClient is the narrow token exchange surface the dedicated
// flow uses.
type PersonalStarOAuthClient interface {
	ExchangeUserCode(ctx context.Context, code, pkceVerifier, redirectURI string) (GitHubOAuthTokens, error)
	RefreshUserToken(ctx context.Context, refreshToken string) (GitHubOAuthTokens, error)
	GetOAuthUser(ctx context.Context, accessToken string) (GitHubOAuthUser, error)
}

// PersonalStarSecrets stores generation-specific encrypted token material.
type PersonalStarSecrets interface {
	Reveal(ctx context.Context, id string) (string, error)
	Exists(ctx context.Context, id string) (bool, error)
	Set(ctx context.Context, id, name, value string) error
	Delete(ctx context.Context, id string) error
}

// PersonalStarOAuthService owns the dedicated OAuth App consent lifecycle.
type PersonalStarOAuthService struct {
	store     *Store
	secrets   PersonalStarSecrets
	client    PersonalStarOAuthClient
	settings  PersonalStarOAuthSettings
	available bool
	now       func() time.Time
	random    io.Reader
}

// NewPersonalStarOAuthService wires the dedicated consent lifecycle.
func NewPersonalStarOAuthService(
	store *Store,
	secrets PersonalStarSecrets,
	settings PersonalStarOAuthSettings,
	available bool,
) *PersonalStarOAuthService {
	service := &PersonalStarOAuthService{
		store:     store,
		secrets:   secrets,
		settings:  settings,
		available: available,
		now:       time.Now,
		random:    rand.Reader,
	}
	if settings.Configured() {
		service.client = NewGitHubOAuthClient(settings.ClientID, settings.ClientSecret)
	}
	return service
}

// Available reports whether the deployment can serve the dedicated flow.
func (s *PersonalStarOAuthService) Available() bool {
	return s != nil && s.available && s.settings.Configured() && s.client != nil
}

// Start creates one-time flow state after a fence bump and returns the
// dedicated authorization URL. The raw state travels only inside that URL.
func (s *PersonalStarOAuthService) Start(
	ctx context.Context,
	request PersonalStarStartRequest,
) (PersonalStarStartResult, error) {
	if !s.Available() {
		return PersonalStarStartResult{}, ErrGitHubNotConfigured
	}
	returnPath, err := validatePersonalStarReturnPath(request.ReturnPath)
	if err != nil {
		return PersonalStarStartResult{}, err
	}
	if strings.TrimSpace(request.WorkspaceID) == "" || strings.TrimSpace(request.UserID) == "" {
		return PersonalStarStartResult{}, ErrGitHubWorkspaceRequired
	}
	revision, generation, err := s.store.BumpPersonalStarLifecycle(ctx, request.WorkspaceID, request.UserID)
	if err != nil {
		return PersonalStarStartResult{}, err
	}
	state, err := randomPersonalStarValue(s.random, personalStarStateBytes, base64.RawURLEncoding)
	if err != nil {
		return PersonalStarStartResult{}, err
	}
	verifier, err := randomPersonalStarValue(s.random, personalStarVerifierBytes, base64.RawURLEncoding)
	if err != nil {
		return PersonalStarStartResult{}, err
	}
	challenge := personalStarPKCEChallenge(verifier)
	now := s.now().UTC()
	expiresAt := now.Add(PersonalStarFlowLifetime)
	if err := s.store.CreatePersonalStarFlow(ctx, &PersonalStarFlow{
		StateDigest:        personalStarStateDigest(state),
		FlowKind:           PersonalStarFlowKind,
		WorkspaceID:        request.WorkspaceID,
		UserID:             request.UserID,
		SessionSubject:     request.SessionSubject,
		SessionID:          request.SessionID,
		ExpectedRevision:   revision,
		ExpectedGeneration: generation,
		CodeVerifier:       verifier,
		CodeChallenge:      challenge,
		RedirectURI:        s.settings.CallbackURL,
		ReturnPath:         returnPath,
		CreatedAt:          now,
		ExpiresAt:          expiresAt,
	}); err != nil {
		return PersonalStarStartResult{}, err
	}
	return PersonalStarStartResult{
		AuthorizationURL: s.authorizationURL(state, challenge),
		StateExpiresAt:   expiresAt,
	}, nil
}

func (s *PersonalStarOAuthService) authorizationURL(state, challenge string) string {
	query := url.Values{
		personalStarParamClientID: {s.settings.ClientID},
		"redirect_uri":            {s.settings.CallbackURL},
		"scope":                   {PersonalStarScope},
		personalStarParamState:    {state},
		"code_challenge":          {challenge},
		"code_challenge_method":   {PKCEChallengeMethodS256},
	}
	return githubWebBase + "/login/oauth/authorize?" + query.Encode()
}

// CompleteCallback consumes the flow exactly once and commits the replacement.
func (s *PersonalStarOAuthService) CompleteCallback(
	ctx context.Context,
	query GitHubStarCallbackQuery,
	sessionSubject, sessionID string,
) (PersonalStarCallbackOutcome, error) {
	if !s.Available() {
		return PersonalStarCallbackOutcome{}, ErrGitHubNotConfigured
	}
	flow, err := s.store.ClaimPersonalStarFlowForExchange(
		ctx, personalStarStateDigest(query.State), s.now().UTC(),
		func(ctx context.Context, exec sqlx.Ext, flow *PersonalStarFlow) error {
			return s.validateClaim(ctx, exec, flow, sessionSubject, sessionID)
		},
	)
	if err != nil {
		return PersonalStarCallbackOutcome{}, err
	}
	base := PersonalStarCallbackOutcome{
		WorkspaceID: flow.WorkspaceID,
		UserID:      flow.UserID,
		ReturnPath:  flow.ReturnPath,
	}
	claimed := PersonalStarFence{Revision: flow.ExpectedRevision, Generation: flow.ExpectedGeneration}
	if query.Denied() {
		return s.finishCallback(ctx, base, claimed, personalStarOutcomeDeclined)
	}
	tokens, err := s.client.ExchangeUserCode(ctx, query.Code, flow.CodeVerifier, flow.RedirectURI)
	if err != nil {
		return s.finishCallback(ctx, base, claimed, personalStarOutcomeFailed)
	}
	user, err := s.client.GetOAuthUser(ctx, tokens.AccessToken)
	if err != nil {
		return s.finishCallback(ctx, base, claimed, personalStarOutcomeFailed)
	}
	return s.commitGrant(ctx, flow, base, tokens, user)
}

func (s *PersonalStarOAuthService) validateClaim(
	ctx context.Context,
	exec sqlx.Ext,
	flow *PersonalStarFlow,
	sessionSubject, sessionID string,
) error {
	if flow.FlowKind != PersonalStarFlowKind {
		return ErrPersonalStarFlowUnavailable
	}
	if flow.SessionSubject != sessionSubject || flow.SessionID != sessionID {
		return ErrPersonalStarFlowUnavailable
	}
	if !ValidPersonalStarPKCEPair(flow.CodeVerifier, flow.CodeChallenge) {
		return ErrPersonalStarFlowUnavailable
	}
	// The fence read runs on the claim transaction's own connection: reading
	// through a second pool would deadlock on SQLite's single writer.
	lifecycle, err := getPersonalStarLifecycleOn(ctx, exec, flow.WorkspaceID, flow.UserID)
	if err != nil {
		return err
	}
	if lifecycle.Revision != flow.ExpectedRevision || lifecycle.CredentialGeneration != flow.ExpectedGeneration {
		return ErrPersonalStarFence
	}
	return nil
}

// commitGrant stages token secrets and commits the connection before queuing
// superseded generations. Ambiguous commit errors are reconciled before cleanup.
func (s *PersonalStarOAuthService) commitGrant(
	ctx context.Context,
	flow *PersonalStarFlow,
	base PersonalStarCallbackOutcome,
	tokens GitHubOAuthTokens,
	user GitHubOAuthUser,
) (PersonalStarCallbackOutcome, error) {
	capable := HasPublicRepoScope(tokens.GrantedScopes)
	if tokens.AccessToken == "" || !capable {
		return s.finishCallback(ctx, base,
			PersonalStarFence{Revision: flow.ExpectedRevision, Generation: flow.ExpectedGeneration},
			personalStarOutcomeInsufficientScope)
	}
	connection := &PersonalStarConnection{
		WorkspaceID:             flow.WorkspaceID,
		UserID:                  flow.UserID,
		Login:                   user.Login,
		GitHubUserID:            user.ID,
		GrantedScopes:           strings.Join(tokens.GrantedScopes, " "),
		PublicRepo:              true,
		CredentialGeneration:    flow.ExpectedGeneration,
		LifecycleRevision:       flow.ExpectedRevision,
		AccessSecretID:          personalStarAccessSecretID(flow.WorkspaceID, flow.UserID, flow.ExpectedGeneration),
		RefreshSecretID:         personalStarRefreshSecretID(flow.WorkspaceID, flow.UserID, flow.ExpectedGeneration),
		AccessSecretGeneration:  flow.ExpectedGeneration,
		RefreshSecretGeneration: flow.ExpectedGeneration,
		LastReconnectOutcome:    personalStarOutcomeConnected,
	}
	staged, err := s.stagePersonalStarSecrets(ctx, connection, tokens)
	if err != nil {
		cleanupErr := s.discardPersonalStarSecrets(ctx, connection.WorkspaceID, connection.UserID, staged)
		outcome, finishErr := s.finishCallback(ctx, base,
			PersonalStarFence{Revision: flow.ExpectedRevision, Generation: flow.ExpectedGeneration},
			personalStarOutcomeFailed)
		if cleanupErr != nil {
			return outcome, errors.Join(finishErr, cleanupErr)
		}
		return outcome, finishErr
	}
	if err := s.commitPersonalStarConnection(ctx, connection, flow); err != nil {
		return s.handlePersonalStarCommitError(ctx, connection, flow.ReturnPath, staged, err)
	}
	return connectedPersonalStarOutcome(connection, flow.ReturnPath), nil
}

func connectedPersonalStarOutcome(connection *PersonalStarConnection, returnPath string) PersonalStarCallbackOutcome {
	return PersonalStarCallbackOutcome{
		WorkspaceID: connection.WorkspaceID,
		UserID:      connection.UserID,
		ReturnPath:  returnPath,
		Outcome:     personalStarOutcomeConnected,
		Active:      true,
		Capable:     true,
	}
}

// reconcilePersonalStarCommit preserves row-referenced tokens and queues unreferenced staged IDs.
func (s *PersonalStarOAuthService) reconcilePersonalStarCommit(
	ctx context.Context,
	candidate *PersonalStarConnection,
	staged []personalStarSecretRef,
) (bool, error) {
	current, lifecycle, err := s.store.personalStarConnectionStateForCommitReconciliation(
		ctx, candidate.WorkspaceID, candidate.UserID,
	)
	if err != nil {
		return false, fmt.Errorf("reconcile personal star connection commit: %w", err)
	}
	if personalStarCommitMatches(current, lifecycle, candidate) {
		return true, nil
	}
	var cleanupErr error
	for _, ref := range staged {
		if current != nil && (current.AccessSecretID == ref.SecretID || current.RefreshSecretID == ref.SecretID) {
			continue
		}
		cleanupErr = errors.Join(cleanupErr,
			s.queuePersonalStarSecretCleanup(ctx, candidate.WorkspaceID, candidate.UserID, ref))
	}
	return false, cleanupErr
}

func personalStarCommitMatches(
	current *PersonalStarConnection,
	lifecycle PersonalStarLifecycle,
	candidate *PersonalStarConnection,
) bool {
	if current == nil || lifecycle.ConnectedGeneration != candidate.CredentialGeneration {
		return false
	}
	expected := *candidate
	expected.Status = PersonalStarStatusActive
	expected.CreatedAt = current.CreatedAt
	expected.UpdatedAt = current.UpdatedAt
	return *current == expected
}

func (s *PersonalStarOAuthService) handlePersonalStarCommitError(
	ctx context.Context,
	candidate *PersonalStarConnection,
	returnPath string,
	staged []personalStarSecretRef,
	commitErr error,
) (PersonalStarCallbackOutcome, error) {
	if !errors.Is(commitErr, errPersonalStarCommitOutcomeUnknown) {
		return s.compensatePersonalStarCommitError(ctx, candidate, staged, commitErr)
	}
	committed, reconcileErr := s.reconcilePersonalStarCommit(ctx, candidate, staged)
	if reconcileErr != nil {
		return PersonalStarCallbackOutcome{}, errors.Join(commitErr, reconcileErr)
	}
	if !committed {
		return PersonalStarCallbackOutcome{}, commitErr
	}
	return connectedPersonalStarOutcome(candidate, returnPath), nil
}

func (s *PersonalStarOAuthService) compensatePersonalStarCommitError(
	ctx context.Context,
	candidate *PersonalStarConnection,
	staged []personalStarSecretRef,
	commitErr error,
) (PersonalStarCallbackOutcome, error) {
	cleanupErr := s.discardPersonalStarSecrets(ctx, candidate.WorkspaceID, candidate.UserID, staged)
	if cleanupErr != nil {
		return PersonalStarCallbackOutcome{}, errors.Join(commitErr, cleanupErr)
	}
	return PersonalStarCallbackOutcome{}, commitErr
}

func (s *PersonalStarOAuthService) handleRefreshPersonalStarCommitError(
	ctx context.Context,
	workspaceID, userID string,
	candidate *PersonalStarConnection,
	staged []personalStarSecretRef,
	commitErr error,
) error {
	if !errors.Is(commitErr, errPersonalStarCommitOutcomeUnknown) {
		return s.compensateRefreshPersonalStarCommitError(ctx, workspaceID, userID, staged, commitErr)
	}
	committed, reconcileErr := s.reconcilePersonalStarCommit(ctx, candidate, staged)
	if reconcileErr != nil {
		return errors.Join(commitErr, reconcileErr)
	}
	if !committed {
		return commitErr
	}
	return nil
}

func (s *PersonalStarOAuthService) compensateRefreshPersonalStarCommitError(
	ctx context.Context,
	workspaceID, userID string,
	staged []personalStarSecretRef,
	commitErr error,
) error {
	cleanupErr := s.discardPersonalStarSecrets(ctx, workspaceID, userID, staged)
	if cleanupErr != nil {
		return errors.Join(commitErr, cleanupErr)
	}
	return commitErr
}

func (s *PersonalStarOAuthService) commitPersonalStarConnection(
	ctx context.Context,
	connection *PersonalStarConnection,
	flow *PersonalStarFlow,
) error {
	existing, err := s.store.GetPersonalStarConnection(ctx, connection.WorkspaceID, connection.UserID)
	if err != nil {
		return err
	}
	if existing == nil {
		return s.store.InsertPersonalStarConnection(ctx, connection, flow.ExpectedRevision, flow.ExpectedGeneration)
	}
	// Two guards, because the claim-time check cannot see a disconnect that
	// lands while the token exchange is in flight: the committed row must still
	// be the one this consent started from, and the lifecycle row must still
	// carry the fence this flow claimed.
	_, err = s.store.ReplacePersonalStarConnection(
		ctx, connection,
		PersonalStarFence{Revision: existing.LifecycleRevision, Generation: existing.CredentialGeneration},
		PersonalStarFence{Revision: flow.ExpectedRevision, Generation: flow.ExpectedGeneration})
	return err
}

// finishCallback records an attempt that did not commit a connection. The
// write is fenced on the fence this callback claimed: if a newer consent or a
// disconnect has moved the lifecycle on, this attempt lost the race and must
// leave the newer row alone, reporting the state that row actually carries.
func (s *PersonalStarOAuthService) finishCallback(
	ctx context.Context,
	base PersonalStarCallbackOutcome,
	claimed PersonalStarFence,
	outcome string,
) (PersonalStarCallbackOutcome, error) {
	existing, err := s.store.GetPersonalStarConnection(ctx, base.WorkspaceID, base.UserID)
	if err != nil {
		return PersonalStarCallbackOutcome{}, err
	}
	applied := false
	if existing != nil {
		applied, err = s.store.SetPersonalStarReconnectOutcome(ctx, base.WorkspaceID, base.UserID, outcome, claimed)
		if err != nil {
			return PersonalStarCallbackOutcome{}, err
		}
	}
	base.Outcome = outcome
	if existing == nil {
		return base, nil
	}
	if !applied {
		// Another operation owns the row now; the attempt's outcome is not the
		// connection's state.
		if existing.LastReconnectOutcome != "" {
			base.Outcome = existing.LastReconnectOutcome
		}
		base.Active = existing.Status == PersonalStarStatusActive
		base.Capable = base.Active && existing.PublicRepo
		return base, nil
	}
	base.Active = existing.Status == PersonalStarStatusActive
	base.Capable = base.Active && existing.PublicRepo
	return base, nil
}

// RefreshPersonalStarConnection rotates the dedicated access token through the
// stored refresh token. The new grant's scopes are re-normalized: losing
// public_repo disables the capability, and the previous access secret is
// enqueued only after the new one is committed.
func (s *PersonalStarOAuthService) RefreshPersonalStarConnection(
	ctx context.Context,
	workspaceID, userID string,
) error {
	if !s.Available() {
		return ErrGitHubNotConfigured
	}
	// Keep refresh IDs distinct when consent reserves the next generation in flight.
	uniqueSuffix, err := randomPersonalStarValue(s.random, personalStarStateBytes, base64.RawURLEncoding)
	if err != nil {
		return ErrGitHubPersonalRequired
	}
	tokens, connection, err := s.refreshTokens(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	expectedFence := PersonalStarFence{
		Revision: connection.LifecycleRevision, Generation: connection.CredentialGeneration,
	}
	revision, generation := expectedFence.Revision+1, expectedFence.Generation+1
	replacement := *connection
	replacement.CredentialGeneration = generation
	replacement.LifecycleRevision = revision
	replacement.AccessSecretGeneration = generation
	replacement.AccessSecretID = personalStarAccessSecretID(workspaceID, userID, generation) + ":" + uniqueSuffix
	if len(tokens.GrantedScopes) > 0 {
		replacement.GrantedScopes = strings.Join(tokens.GrantedScopes, " ")
		replacement.PublicRepo = HasPublicRepoScope(tokens.GrantedScopes)
	}
	staged := make([]personalStarSecretRef, 0, 2)
	accessRef := personalStarSecretRef{
		SecretID: replacement.AccessSecretID, Kind: personalStarSecretKindAccess,
		Generation: replacement.AccessSecretGeneration,
	}
	if err := s.secrets.Set(ctx, replacement.AccessSecretID,
		"github personal star access token", tokens.AccessToken); err != nil {
		cleanupErr := s.reconcilePersonalStarSecretWriteFailure(ctx, workspaceID, userID, accessRef)
		if cleanupErr != nil {
			cleanupErr = errors.Join(cleanupErr,
				s.discardPersonalStarSecrets(ctx, workspaceID, userID, []personalStarSecretRef{accessRef}))
		}
		return errors.Join(ErrGitHubPersonalRequired, cleanupErr)
	}
	staged = append(staged, accessRef)
	if tokens.RefreshToken != "" {
		replacement.RefreshSecretID = personalStarRefreshSecretID(workspaceID, userID, generation) + ":" + uniqueSuffix
		replacement.RefreshSecretGeneration = generation
		refreshRef := personalStarSecretRef{
			SecretID: replacement.RefreshSecretID, Kind: personalStarSecretKindRefresh,
			Generation: replacement.RefreshSecretGeneration,
		}
		if err := s.secrets.Set(ctx, replacement.RefreshSecretID,
			"github personal star refresh token", tokens.RefreshToken); err != nil {
			cleanupErr := s.reconcilePersonalStarSecretWriteFailure(ctx, workspaceID, userID, refreshRef)
			stagedCleanupErr := s.discardPersonalStarSecrets(ctx, workspaceID, userID, staged)
			return errors.Join(ErrGitHubPersonalRequired, cleanupErr, stagedCleanupErr)
		}
		staged = append(staged, refreshRef)
	}
	// Rotate only if neither consent nor disconnect changed the captured fence.
	if _, err := s.store.RotatePersonalStarConnection(
		ctx, &replacement, expectedFence, expectedFence); err != nil {
		return s.handleRefreshPersonalStarCommitError(ctx, workspaceID, userID, &replacement, staged, err)
	}
	return nil
}

// refreshTokens loads the committed connection and exchanges its refresh token.
func (s *PersonalStarOAuthService) refreshTokens(
	ctx context.Context,
	workspaceID, userID string,
) (GitHubOAuthTokens, *PersonalStarConnection, error) {
	if !s.Available() {
		return GitHubOAuthTokens{}, nil, ErrGitHubNotConfigured
	}
	connection, err := s.store.GetPersonalStarConnection(ctx, workspaceID, userID)
	if err != nil {
		return GitHubOAuthTokens{}, nil, err
	}
	if connection == nil || connection.RefreshSecretID == "" || connection.Status != PersonalStarStatusActive {
		return GitHubOAuthTokens{}, nil, ErrGitHubPersonalRequired
	}
	refreshToken, err := s.secrets.Reveal(ctx, connection.RefreshSecretID)
	if err != nil || strings.TrimSpace(refreshToken) == "" {
		return GitHubOAuthTokens{}, nil, ErrGitHubPersonalRequired
	}
	tokens, err := s.client.RefreshUserToken(ctx, refreshToken)
	if err != nil {
		if isDefinitivePersonalAuthFailure(err) {
			fence := PersonalStarFence{
				Revision: connection.LifecycleRevision, Generation: connection.CredentialGeneration,
			}
			if _, markErr := s.store.SetPersonalStarReconnectOutcome(
				ctx, workspaceID, userID, personalStarOutcomeFailed, fence,
			); markErr != nil {
				return GitHubOAuthTokens{}, nil, fmt.Errorf("record rejected personal star refresh: %w", markErr)
			}
		}
		return GitHubOAuthTokens{}, nil, ErrGitHubPersonalRequired
	}
	if tokens.AccessToken == "" {
		return GitHubOAuthTokens{}, nil, ErrGitHubPersonalRequired
	}
	return tokens, connection, nil
}

// DisconnectPersonalStarConnection bumps the fence and clears the committed
// token pointers. The store records the superseded generations in the durable
// cleanup queue in the same transaction, so a disconnect can never leave token
// ciphertext that no row references and no queue retries.
func (s *PersonalStarOAuthService) DisconnectPersonalStarConnection(
	ctx context.Context,
	workspaceID, userID string,
) error {
	_, err := s.store.DisconnectPersonalStarConnection(ctx, workspaceID, userID)
	return err
}

// ParseGitHubStarCallbackRawQuery parses the untouched callback query bytes.
// It rejects duplicate or unknown keys, empty values, mixed code/error, and any
// non-canonical state spelling before state lookup.
func ParseGitHubStarCallbackRawQuery(rawQuery string) (GitHubStarCallbackQuery, error) {
	values, err := parsePersonalStarQueryPairs(rawQuery)
	if err != nil {
		return GitHubStarCallbackQuery{}, err
	}
	state, err := decodePersonalStarState(values[personalStarParamState])
	if err != nil {
		return GitHubStarCallbackQuery{}, err
	}
	code, hasCode := values[personalStarParamCode]
	oauthError, hasError := values[personalStarParamError]
	switch {
	case hasCode && hasError, !hasCode && !hasError:
		return GitHubStarCallbackQuery{}, fmt.Errorf("%w: exactly one of code or error is required", ErrPersonalStarInvalidCallback)
	case hasCode:
		decoded, err := decodePersonalStarValue("code", code)
		if err != nil {
			return GitHubStarCallbackQuery{}, err
		}
		return GitHubStarCallbackQuery{State: state, Code: decoded}, nil
	default:
		decoded, err := decodePersonalStarValue(personalStarParamError, oauthError)
		if err != nil {
			return GitHubStarCallbackQuery{}, err
		}
		query := GitHubStarCallbackQuery{State: state, Error: decoded}
		if query.ErrorDescription, err = optionalPersonalStarValue(values, "error_description"); err != nil {
			return GitHubStarCallbackQuery{}, err
		}
		if query.ErrorURI, err = optionalPersonalStarValue(values, "error_uri"); err != nil {
			return GitHubStarCallbackQuery{}, err
		}
		return query, nil
	}
}

func parsePersonalStarQueryPairs(rawQuery string) (map[string]string, error) {
	values := map[string]string{}
	if rawQuery == "" {
		return nil, fmt.Errorf("%w: empty query", ErrPersonalStarInvalidCallback)
	}
	for _, pair := range strings.Split(rawQuery, "&") {
		key, value, found := strings.Cut(pair, "=")
		if !found || value == "" {
			return nil, fmt.Errorf("%w: empty query value", ErrPersonalStarInvalidCallback)
		}
		switch key {
		case personalStarParamState, personalStarParamCode, personalStarParamError, "error_description", "error_uri":
		default:
			return nil, fmt.Errorf("%w: unknown query key", ErrPersonalStarInvalidCallback)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("%w: duplicate query key", ErrPersonalStarInvalidCallback)
		}
		values[key] = value
	}
	if _, ok := values[personalStarParamState]; !ok {
		return nil, fmt.Errorf("%w: missing state", ErrPersonalStarInvalidCallback)
	}
	return values, nil
}

// decodePersonalStarState requires the exact RawURLEncoding spelling of a
// 32-byte state: no percent-escapes, no plus signs, no padding.
func decodePersonalStarState(raw string) (string, error) {
	if strings.ContainsAny(raw, "%+") {
		return "", fmt.Errorf("%w: state is not canonical", ErrPersonalStarInvalidCallback)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != personalStarStateBytes {
		return "", fmt.Errorf("%w: state is not canonical", ErrPersonalStarInvalidCallback)
	}
	if base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return "", fmt.Errorf("%w: state is not canonical", ErrPersonalStarInvalidCallback)
	}
	return raw, nil
}

func decodePersonalStarValue(field, raw string) (string, error) {
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %s is not percent-encoded correctly", ErrPersonalStarInvalidCallback, field)
	}
	if decoded == "" {
		return "", fmt.Errorf("%w: %s is empty", ErrPersonalStarInvalidCallback, field)
	}
	return decoded, nil
}

func optionalPersonalStarValue(values map[string]string, key string) (string, error) {
	raw, ok := values[key]
	if !ok {
		return "", nil
	}
	return decodePersonalStarValue(key, raw)
}

// NormalizeGitHubScopes parses a space- or comma-separated scope list into a
// deduplicated, sorted, lowercase set.
func NormalizeGitHubScopes(raw string) []string {
	fields := strings.FieldsFunc(raw, func(char rune) bool {
		return char == ' ' || char == ',' || char == '\t'
	})
	seen := make(map[string]struct{}, len(fields))
	scopes := make([]string, 0, len(fields))
	for _, field := range fields {
		scope := strings.ToLower(strings.TrimSpace(field))
		if scope == "" {
			continue
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}
	return scopes
}

// HasPublicRepoScope reports whether a normalized grant carries public_repo.
func HasPublicRepoScope(scopes []string) bool {
	for _, scope := range scopes {
		if strings.EqualFold(strings.TrimSpace(scope), PersonalStarScope) {
			return true
		}
	}
	return false
}

// ValidPersonalStarPKCEPair enforces the RFC 7636 verifier grammar and that the
// stored challenge is the S256 derivation of that verifier.
func ValidPersonalStarPKCEPair(verifier, challenge string) bool {
	if !validPersonalStarVerifier(verifier) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(challenge)
	if err != nil || len(decoded) != sha256.Size {
		return false
	}
	if base64.RawURLEncoding.EncodeToString(decoded) != challenge {
		return false
	}
	return personalStarPKCEChallenge(verifier) == challenge
}

func validPersonalStarVerifier(verifier string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	for _, char := range verifier {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
		case char == '-', char == '.', char == '_', char == '~':
		default:
			return false
		}
	}
	return true
}

func personalStarPKCEChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func personalStarStateDigest(state string) string {
	digest := sha256.Sum256([]byte(state))
	return hex.EncodeToString(digest[:])
}

// personalStarTokenSecretPrefix reserves the dedicated star token namespace.
// It sits under kandev-runtime: so the backend-owned credential policy
// (secrets.IsInternalID) keeps OAuth token material out of every user-visible
// secret surface and out of agent credential selection.
const personalStarTokenSecretPrefix = "kandev-runtime:github-personal-star:"

const (
	// KandevRuntimePersonalStarAccessPrefix and
	// KandevRuntimePersonalStarRefreshPrefix name the reserved token
	// namespaces so workspace teardown and E2E reset delete them by prefix.
	KandevRuntimePersonalStarAccessPrefix  = personalStarTokenSecretPrefix + "access:"
	KandevRuntimePersonalStarRefreshPrefix = personalStarTokenSecretPrefix + "refresh:"
)

func personalStarAccessSecretID(workspaceID, userID string, generation int64) string {
	return fmt.Sprintf(personalStarTokenSecretPrefix+"access:%s:%s:%d", workspaceID, userID, generation)
}

func personalStarRefreshSecretID(workspaceID, userID string, generation int64) string {
	return fmt.Sprintf(personalStarTokenSecretPrefix+"refresh:%s:%s:%d", workspaceID, userID, generation)
}

func randomPersonalStarValue(source io.Reader, size int, encoding *base64.Encoding) (string, error) {
	buffer := make([]byte, size)
	if _, err := io.ReadFull(source, buffer); err != nil {
		return "", fmt.Errorf("generate personal star value: %w", err)
	}
	return encoding.EncodeToString(buffer), nil
}
