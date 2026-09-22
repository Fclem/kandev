package deploymentsecret

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/secrets"
)

const (
	unixSocketNetwork        = "unix"
	defaultConnectionTimeout = 30 * time.Second
	maxPendingChallenges     = 128
)

// Provisioner is the only writer reachable from the supervisor.
type Provisioner interface {
	Provision(context.Context, secrets.DeploymentProvisionRequest) error
	Revoke(context.Context, secrets.DeploymentRevokeRequest) error
}

// Config wires the supervisor to its operator-facing socket.
type Config struct {
	SocketPath  string
	Target      Target
	Identity    secrets.DeploymentIdentity
	Provisioner Provisioner
	// PeerUID reads the kernel-authenticated peer identity. It defaults to
	// UnixPeerUID; callers with a different credential mechanism substitute it.
	PeerUID func(*net.UnixConn) (uint32, error)
	Now     func() time.Time
	Random  io.Reader
}

type issuedChallenge struct {
	peerUID        uint32
	installationID string
	clientNonceB64 string
	challengeIDB64 string
	challengeB64   string
	expiresAt      time.Time
}

// Server is the root-owned deployment-secret supervisor. It accepts one
// request per connection from a root peer and holds every challenge it issues.
type Server struct {
	config     Config
	listener   *net.UnixListener
	mu         sync.Mutex
	challenges map[string]issuedChallenge
	closed     chan struct{}
}

// NewServer validates the deployment target and supervisor wiring.
func NewServer(config Config) (*Server, error) {
	if err := ValidateTarget(config.Target); err != nil {
		return nil, err
	}
	if config.SocketPath == "" {
		return nil, errors.New("deployment secret socket path is required")
	}
	if config.Provisioner == nil {
		return nil, errors.New("deployment secret provisioner is required")
	}
	if err := secrets.ValidateDeploymentInstallationID(config.Identity.InstallationID); err != nil {
		return nil, fmt.Errorf("deployment secret supervisor identity: %w", err)
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.PeerUID == nil {
		config.PeerUID = UnixPeerUID
	}
	if config.Random == nil {
		config.Random = rand.Reader
	}
	return &Server{config: config, challenges: map[string]issuedChallenge{}, closed: make(chan struct{})}, nil
}

// Listen creates the root-owned socket. The parent directory is forced to 0700
// and the socket to 0600 so only root can connect.
func (s *Server) Listen() error {
	if err := ValidateTarget(s.config.Target); err != nil {
		return err
	}
	parent := filepath.Dir(s.config.SocketPath)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create deployment secret socket dir: %w", err)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return fmt.Errorf("secure deployment secret socket dir: %w", err)
	}
	if err := removeStaleSocket(s.config.SocketPath); err != nil {
		return err
	}
	listener, err := net.ListenUnix(unixSocketNetwork, &net.UnixAddr{Name: s.config.SocketPath, Net: unixSocketNetwork})
	if err != nil {
		return fmt.Errorf("listen on deployment secret socket: %w", err)
	}
	if err := os.Chmod(s.config.SocketPath, 0o600); err != nil {
		_ = listener.Close()
		return fmt.Errorf("secure deployment secret socket: %w", err)
	}
	s.listener = listener
	return nil
}

// Serve accepts connections until the listener closes.
func (s *Server) Serve(ctx context.Context) error {
	if s.listener == nil {
		return errors.New("deployment secret supervisor is not listening")
	}
	for {
		connection, err := s.listener.AcceptUnix()
		if err != nil {
			select {
			case <-s.closed:
				return nil
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept deployment secret connection: %w", err)
		}
		go s.handleConnection(ctx, connection)
	}
}

// Close stops the supervisor and removes the socket.
func (s *Server) Close() error {
	s.mu.Lock()
	select {
	case <-s.closed:
		s.mu.Unlock()
		return nil
	default:
		close(s.closed)
	}
	s.mu.Unlock()
	if s.listener != nil {
		_ = s.listener.Close()
		_ = os.Remove(s.config.SocketPath)
	}
	return nil
}

func (s *Server) handleConnection(ctx context.Context, connection *net.UnixConn) {
	defer func() { _ = connection.Close() }()
	if deadlineErr := connection.SetDeadline(s.config.Now().Add(defaultConnectionTimeout)); deadlineErr != nil {
		return
	}
	peerUID, err := s.config.PeerUID(connection)
	if err != nil {
		s.writeError(connection, secrets.DeploymentErrorInvalidRequest)
		return
	}
	if err := AuthorizeRootPeer(peerUID); err != nil {
		s.writeError(connection, secrets.DeploymentErrorInvalidRequest)
		return
	}
	s.handleChallengePhase(ctx, connection, peerUID)
}

func (s *Server) handleChallengePhase(ctx context.Context, connection *net.UnixConn, peerUID uint32) {
	raw, err := secrets.ReadDeploymentFrame(connection)
	if err != nil {
		s.writeError(connection, secrets.DeploymentErrorInvalidRequest)
		return
	}
	request, err := secrets.ParseDeploymentChallengeRequest(raw)
	if err != nil || request.InstallationID != s.config.Identity.InstallationID {
		s.writeError(connection, secrets.DeploymentErrorWrongInstallation)
		return
	}
	response, err := s.issueChallenge(peerUID, request)
	if err != nil {
		s.writeError(connection, secrets.DeploymentErrorInvalidRequest)
		return
	}
	if err := secrets.WriteDeploymentFrame(connection, secrets.EncodeDeploymentChallengeResponse(response)); err != nil {
		return
	}
	s.handleSignedPhase(ctx, connection, peerUID)
}

func (s *Server) handleSignedPhase(ctx context.Context, connection *net.UnixConn, peerUID uint32) {
	envelope, signature, err := secrets.ReadDeploymentSignedRequest(connection)
	if err != nil {
		s.writeError(connection, secrets.DeploymentErrorInvalidRequest)
		return
	}
	request, err := secrets.ParseDeploymentSignedRequest(envelope)
	if err != nil {
		s.writeError(connection, secrets.DeploymentErrorInvalidRequest)
		return
	}
	if err := secrets.VerifyDeploymentEnvelopeSignature(s.config.Identity.RootPublicKey, envelope, signature); err != nil {
		s.writeError(connection, secrets.DeploymentErrorInvalidSignature)
		return
	}
	if err := s.consumeChallenge(peerUID, request); err != nil {
		s.writeError(connection, deploymentChallengeErrorCode(err))
		return
	}
	s.writeResult(connection, s.dispatch(ctx, request))
}

func (s *Server) dispatch(ctx context.Context, request secrets.DeploymentSignedRequest) secrets.DeploymentErrorCode {
	if request.InstallationID != s.config.Identity.InstallationID {
		return secrets.DeploymentErrorWrongInstallation
	}
	if request.SecretClass != secrets.DeploymentSecretClassGitHubPersonalStar {
		return secrets.DeploymentErrorWrongClass
	}
	return s.invokeProvisioner(ctx, request)
}

func (s *Server) invokeProvisioner(ctx context.Context, request secrets.DeploymentSignedRequest) secrets.DeploymentErrorCode {
	switch request.Action {
	case secrets.DeploymentActionProvision:
		ciphertext, err := base64.RawStdEncoding.DecodeString(request.CiphertextB64)
		if err != nil {
			return secrets.DeploymentErrorInvalidRequest
		}
		return mapProvisionerError(s.config.Provisioner.Provision(ctx, secrets.DeploymentProvisionRequest{
			InstallationID:   request.InstallationID,
			SecretClass:      request.SecretClass,
			ReferenceVersion: request.ReferenceVersion,
			SealedEnvelope:   ciphertext,
		}))
	case secrets.DeploymentActionRevoke:
		return mapProvisionerError(s.config.Provisioner.Revoke(ctx, secrets.DeploymentRevokeRequest{
			InstallationID:   request.InstallationID,
			SecretClass:      request.SecretClass,
			ReferenceVersion: request.ReferenceVersion,
		}))
	default:
		return secrets.DeploymentErrorInvalidRequest
	}
}

func mapProvisionerError(err error) secrets.DeploymentErrorCode {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, secrets.ErrDeploymentSecretExists):
		return secrets.DeploymentErrorAlreadyProvisioned
	case errors.Is(err, secrets.ErrDeploymentSecretNotFound):
		return secrets.DeploymentErrorUnknownReference
	case errors.Is(err, secrets.ErrDeploymentSecretBinding):
		return secrets.DeploymentErrorWrongClass
	default:
		return secrets.DeploymentErrorStorage
	}
}

func (s *Server) writeResult(connection *net.UnixConn, code secrets.DeploymentErrorCode) {
	if code == "" {
		_ = secrets.WriteDeploymentFrame(connection, secrets.EncodeDeploymentResult(secrets.DeploymentResult{Status: secrets.DeploymentStatusOK}))
		return
	}
	s.writeError(connection, code)
}

func (s *Server) writeError(connection *net.UnixConn, code secrets.DeploymentErrorCode) {
	result := secrets.DeploymentResult{Status: secrets.DeploymentStatusError, ErrorCode: code}
	_ = secrets.WriteDeploymentFrame(connection, secrets.EncodeDeploymentResult(result))
}

func (s *Server) issueChallenge(peerUID uint32, request secrets.DeploymentChallengeRequest) (secrets.DeploymentChallengeResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.config.Now().UTC()
	s.purgeExpiredChallenges(now)
	if len(s.challenges) >= maxPendingChallenges {
		return secrets.DeploymentChallengeResponse{}, errors.New("deployment challenge capacity reached")
	}
	challengeID, err := randomDeploymentField(s.config.Random)
	if err != nil {
		return secrets.DeploymentChallengeResponse{}, err
	}
	challenge, err := randomDeploymentField(s.config.Random)
	if err != nil {
		return secrets.DeploymentChallengeResponse{}, err
	}
	expiresAt := now.Add(secrets.DeploymentChallengeLifetime)
	s.challenges[challengeID] = issuedChallenge{
		peerUID:        peerUID,
		installationID: request.InstallationID,
		clientNonceB64: request.ClientNonceB64,
		challengeIDB64: challengeID,
		challengeB64:   challenge,
		expiresAt:      expiresAt,
	}
	return secrets.DeploymentChallengeResponse{
		ChallengeIDB64: challengeID,
		ChallengeB64:   challenge,
		ExpiresAt:      expiresAt.Format(time.RFC3339Nano),
	}, nil
}

func (s *Server) consumeChallenge(peerUID uint32, request secrets.DeploymentSignedRequest) error {
	s.mu.Lock()
	issued, ok := s.challenges[request.ChallengeIDB64]
	if !ok {
		s.mu.Unlock()
		return errUnknownChallenge
	}
	delete(s.challenges, request.ChallengeIDB64)
	s.mu.Unlock()

	if s.config.Now().UTC().After(issued.expiresAt) {
		return errExpiredChallenge
	}
	if issued.peerUID != peerUID || issued.installationID != request.InstallationID {
		return errUnknownChallenge
	}
	if issued.clientNonceB64 != request.NonceB64 || issued.challengeB64 != request.ChallengeB64 {
		return errUnknownChallenge
	}
	return nil
}

func (s *Server) purgeExpiredChallenges(now time.Time) {
	for id, challenge := range s.challenges {
		if now.After(challenge.expiresAt) {
			delete(s.challenges, id)
		}
	}
}

var (
	errUnknownChallenge = errors.New("deployment challenge is unknown or already consumed")
	errExpiredChallenge = errors.New("deployment challenge expired")
)

func deploymentChallengeErrorCode(err error) secrets.DeploymentErrorCode {
	if errors.Is(err, errExpiredChallenge) {
		return secrets.DeploymentErrorExpiredChallenge
	}
	return secrets.DeploymentErrorReplay
}

func randomDeploymentField(source io.Reader) (string, error) {
	buffer := make([]byte, secrets.DeploymentRandomBytes)
	if _, err := io.ReadFull(source, buffer); err != nil {
		return "", fmt.Errorf("generate deployment challenge field: %w", err)
	}
	return base64.RawStdEncoding.EncodeToString(buffer), nil
}

func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect deployment secret socket: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("deployment secret socket path %s is not a socket", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove stale deployment secret socket: %w", err)
	}
	return nil
}
