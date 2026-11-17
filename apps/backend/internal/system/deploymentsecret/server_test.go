package deploymentsecret

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/secrets"
	"syscall"
)

type recordedProvision struct {
	request secrets.DeploymentProvisionRequest
}

type recordedRevoke struct {
	request secrets.DeploymentRevokeRequest
}

type recordingProvisioner struct {
	provisions   []recordedProvision
	revokes      []recordedRevoke
	provisionErr error
	revokeErr    error
}

func (p *recordingProvisioner) Provision(_ context.Context, request secrets.DeploymentProvisionRequest) error {
	p.provisions = append(p.provisions, recordedProvision{request: request})
	return p.provisionErr
}

func (p *recordingProvisioner) Revoke(_ context.Context, request secrets.DeploymentRevokeRequest) error {
	p.revokes = append(p.revokes, recordedRevoke{request: request})
	return p.revokeErr
}

type supervisorHarness struct {
	socketPath string
	identity   secrets.DeploymentIdentity
	privateKey ed25519.PrivateKey
	dataDir    string
}

func newSupervisorHarness(t *testing.T, provisioner Provisioner) supervisorHarness {
	t.Helper()
	return startSupervisor(t, t.TempDir(), provisioner)
}

func startSupervisor(t *testing.T, dataDir string, provisioner Provisioner) supervisorHarness {
	t.Helper()
	identity, err := secrets.BootstrapDeploymentIdentity(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := secrets.LoadDeploymentRootPrivateKey(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(dataDir, "run", "s.sock")
	server, err := NewServer(Config{
		SocketPath:  socketPath,
		Target:      Target{GOOS: "linux", BackendUID: 1000},
		Identity:    identity,
		Provisioner: provisioner,
		PeerUID:     func(*net.UnixConn) (uint32, error) { return 0, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Listen(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(context.Background()) }()
	t.Cleanup(func() { _ = server.Close() })
	return supervisorHarness{socketPath: socketPath, identity: identity, privateKey: privateKey, dataDir: dataDir}
}

func testClientNonce() string {
	return base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{21}, secrets.DeploymentRandomBytes))
}

func foreignInstallationID() string {
	return strings.Repeat("ab", secrets.DeploymentInstallationIDBytes)
}

func (h supervisorHarness) dial(t *testing.T) *net.UnixConn {
	t.Helper()
	connection, err := net.DialUnix(unixSocketNetwork, nil, &net.UnixAddr{Name: h.socketPath, Net: unixSocketNetwork})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

func (h supervisorHarness) requestChallenge(t *testing.T, connection *net.UnixConn, installationID string) secrets.DeploymentChallengeResponse {
	t.Helper()
	request := secrets.EncodeDeploymentChallengeRequest(secrets.DeploymentChallengeRequest{
		InstallationID: installationID,
		ClientNonceB64: testClientNonce(),
	})
	if err := secrets.WriteDeploymentFrame(connection, request); err != nil {
		t.Fatal(err)
	}
	raw, err := secrets.ReadDeploymentFrame(connection)
	if err != nil {
		t.Fatal(err)
	}
	response, err := secrets.ParseDeploymentChallengeResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

type exchangeResult struct {
	result    secrets.DeploymentResult
	envelope  []byte
	signature string
}

// exchange performs one challenge plus signed request round trip, signing with
// the supplied key so tests can also exercise forgery.
func (h supervisorHarness) exchange(
	t *testing.T,
	build func(secrets.DeploymentChallengeResponse) secrets.DeploymentSignedRequest,
	signWith ed25519.PrivateKey,
) exchangeResult {
	t.Helper()
	connection := h.dial(t)
	response := h.requestChallenge(t, connection, h.identity.InstallationID)
	envelope := secrets.EncodeDeploymentSignedRequest(build(response))
	signature := secrets.SignDeploymentEnvelope(signWith, envelope)
	if err := secrets.WriteDeploymentSignedRequest(connection, envelope, signature); err != nil {
		t.Fatal(err)
	}
	if err := connection.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	return exchangeResult{result: h.readResult(t, connection), envelope: envelope, signature: signature}
}

func (h supervisorHarness) readResult(t *testing.T, connection *net.UnixConn) secrets.DeploymentResult {
	t.Helper()
	raw, err := secrets.ReadDeploymentFrame(connection)
	if err != nil {
		t.Fatal(err)
	}
	result, err := secrets.ParseDeploymentResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (h supervisorHarness) provisionEnvelope(
	response secrets.DeploymentChallengeResponse,
	version int64,
	sealed []byte,
) secrets.DeploymentSignedRequest {
	return secrets.DeploymentSignedRequest{
		Action:           secrets.DeploymentActionProvision,
		InstallationID:   h.identity.InstallationID,
		SecretClass:      secrets.DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: version,
		CiphertextB64:    base64.RawStdEncoding.EncodeToString(sealed),
		CiphertextDigest: secrets.DeploymentCiphertextDigest(sealed),
		NonceB64:         testClientNonce(),
		ChallengeIDB64:   response.ChallengeIDB64,
		ChallengeB64:     response.ChallengeB64,
	}
}

func (h supervisorHarness) revokeEnvelope(
	response secrets.DeploymentChallengeResponse,
	version int64,
) secrets.DeploymentSignedRequest {
	return secrets.DeploymentSignedRequest{
		Action:           secrets.DeploymentActionRevoke,
		InstallationID:   h.identity.InstallationID,
		SecretClass:      secrets.DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: version,
		NonceB64:         testClientNonce(),
		ChallengeIDB64:   response.ChallengeIDB64,
		ChallengeB64:     response.ChallengeB64,
	}
}

func TestSupervisorProvisionsAndRevokesOverRootSocket(t *testing.T) {
	provisioner := &recordingProvisioner{}
	harness := newSupervisorHarness(t, provisioner)

	sealed := []byte(`{"version":1}`)
	outcome := harness.exchange(t, func(response secrets.DeploymentChallengeResponse) secrets.DeploymentSignedRequest {
		return harness.provisionEnvelope(response, 3, sealed)
	}, harness.privateKey)
	if outcome.result.Status != secrets.DeploymentStatusOK {
		t.Fatalf("provision result = %+v", outcome.result)
	}
	if len(provisioner.provisions) != 1 {
		t.Fatalf("provision calls = %d", len(provisioner.provisions))
	}
	captured := provisioner.provisions[0].request
	if captured.ReferenceVersion != 3 || !bytes.Equal(captured.SealedEnvelope, sealed) {
		t.Fatalf("captured provision = %+v", captured)
	}

	outcome = harness.exchange(t, func(response secrets.DeploymentChallengeResponse) secrets.DeploymentSignedRequest {
		return harness.revokeEnvelope(response, 3)
	}, harness.privateKey)
	if outcome.result.Status != secrets.DeploymentStatusOK || len(provisioner.revokes) != 1 {
		t.Fatalf("revoke result = %+v, calls = %d", outcome.result, len(provisioner.revokes))
	}

	info, err := os.Stat(harness.socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %v, want 0600", info.Mode().Perm())
	}
	if dir, err := os.Stat(filepath.Dir(harness.socketPath)); err != nil || dir.Mode().Perm() != 0o700 {
		t.Fatalf("socket dir = %v, %v; want 0700", dir, err)
	}
}

func TestSupervisorProvisionIsResolvableByReader(t *testing.T) {
	dataDir := t.TempDir()
	key := bytes.Repeat([]byte{13}, secrets.MasterKeySize)
	if err := os.WriteFile(filepath.Join(dataDir, secrets.MasterKeyFile), key, 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := secrets.BootstrapDeploymentIdentity(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := secrets.DeploymentMasterKeyringOpenExisting(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := secrets.OpenDeploymentSecretStoreWriter(dataDir, identity, keyring)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	harness := startSupervisor(t, dataDir, secrets.NewDeploymentSecretProvisioner(writer))

	sealed, err := secrets.SealDeploymentSecret(keyring.ID(), key, []byte("operator-client-secret"))
	if err != nil {
		t.Fatal(err)
	}
	outcome := harness.exchange(t, func(response secrets.DeploymentChallengeResponse) secrets.DeploymentSignedRequest {
		return harness.provisionEnvelope(response, 1, sealed.Bytes())
	}, harness.privateKey)
	if outcome.result.Status != secrets.DeploymentStatusOK {
		t.Fatalf("provision result = %+v", outcome.result)
	}

	reader, err := secrets.OpenDeploymentSecretStoreReader(dataDir, identity, keyring)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	value, err := reader.RevealDeploymentSecret(context.Background(), secrets.DeploymentSecretReference(1))
	if err != nil {
		t.Fatal(err)
	}
	if value != "operator-client-secret" {
		t.Fatalf("RevealDeploymentSecret() = %q", value)
	}
}

func TestSupervisorRejectsTamperedAndReplayedRequests(t *testing.T) {
	provisioner := &recordingProvisioner{}
	harness := newSupervisorHarness(t, provisioner)
	sealed := []byte(`{"version":1}`)

	_, foreignPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome := harness.exchange(t, func(response secrets.DeploymentChallengeResponse) secrets.DeploymentSignedRequest {
		return harness.provisionEnvelope(response, 1, sealed)
	}, foreignPrivateKey)
	if outcome.result.ErrorCode != secrets.DeploymentErrorInvalidSignature {
		t.Fatalf("forged signature result = %+v", outcome.result)
	}
	if len(provisioner.provisions) != 0 {
		t.Fatal("forged request reached the provisioner")
	}

	accepted := harness.exchange(t, func(response secrets.DeploymentChallengeResponse) secrets.DeploymentSignedRequest {
		return harness.provisionEnvelope(response, 2, sealed)
	}, harness.privateKey)
	if accepted.result.Status != secrets.DeploymentStatusOK {
		t.Fatalf("first provision result = %+v", accepted.result)
	}

	replay := harness.dial(t)
	_ = harness.requestChallenge(t, replay, harness.identity.InstallationID)
	if err := secrets.WriteDeploymentSignedRequest(replay, accepted.envelope, accepted.signature); err != nil {
		t.Fatal(err)
	}
	if err := replay.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if result := harness.readResult(t, replay); result.ErrorCode != secrets.DeploymentErrorReplay {
		t.Fatalf("replay result = %+v", result)
	}
	if len(provisioner.provisions) != 1 {
		t.Fatalf("replay reached the provisioner: %d calls", len(provisioner.provisions))
	}
}

func TestSupervisorFailsClosedForUnavailableTargetsAndPeers(t *testing.T) {
	dataDir := t.TempDir()
	identity, err := secrets.BootstrapDeploymentIdentity(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(dataDir, "run", "s.sock")
	targets := map[string]Target{
		"darwin":       {GOOS: "darwin", BackendUID: 1000},
		"root-backend": {GOOS: "linux", BackendUID: 0},
		"container":    {GOOS: "linux", BackendUID: 1000, Container: true},
		"user-systemd": {GOOS: "linux", BackendUID: 1000, UserSystemd: true},
	}
	for name, target := range targets {
		if _, err := NewServer(Config{
			SocketPath:  socketPath,
			Target:      target,
			Identity:    identity,
			Provisioner: &recordingProvisioner{},
		}); err == nil {
			t.Fatalf("NewServer(%s) succeeded on an unsupported target", name)
		}
	}
	if _, err := os.Stat(filepath.Dir(socketPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported target left filesystem state: %v", err)
	}

	provisioner := &recordingProvisioner{}
	server, err := NewServer(Config{
		SocketPath:  socketPath,
		Target:      Target{GOOS: "linux", BackendUID: 1000},
		Identity:    identity,
		Provisioner: provisioner,
		PeerUID:     func(*net.UnixConn) (uint32, error) { return 1000, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Listen(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(context.Background()) }()
	t.Cleanup(func() { _ = server.Close() })

	harness := supervisorHarness{socketPath: socketPath, identity: identity, dataDir: dataDir}
	connection := harness.dial(t)
	request := secrets.EncodeDeploymentChallengeRequest(secrets.DeploymentChallengeRequest{
		InstallationID: identity.InstallationID,
		ClientNonceB64: testClientNonce(),
	})
	// A non-root peer is rejected before the server reads the frame, so the
	// server can answer and close while this write is still in flight.
	if err := secrets.WriteDeploymentFrame(connection, request); err != nil && !errors.Is(err, syscall.EPIPE) {
		t.Fatal(err)
	}
	if result := harness.readResult(t, connection); result.ErrorCode != secrets.DeploymentErrorInvalidRequest {
		t.Fatalf("non-root peer result = %+v", result)
	}
}

func TestSupervisorMapsProvisionerFailures(t *testing.T) {
	provisioner := &recordingProvisioner{provisionErr: secrets.ErrDeploymentSecretExists}
	harness := newSupervisorHarness(t, provisioner)
	outcome := harness.exchange(t, func(response secrets.DeploymentChallengeResponse) secrets.DeploymentSignedRequest {
		return harness.provisionEnvelope(response, 1, []byte(`{"version":1}`))
	}, harness.privateKey)
	if outcome.result.ErrorCode != secrets.DeploymentErrorAlreadyProvisioned {
		t.Fatalf("duplicate provision result = %+v", outcome.result)
	}

	provisioner.provisionErr = nil
	provisioner.revokeErr = secrets.ErrDeploymentSecretNotFound
	outcome = harness.exchange(t, func(response secrets.DeploymentChallengeResponse) secrets.DeploymentSignedRequest {
		return harness.revokeEnvelope(response, 9)
	}, harness.privateKey)
	if outcome.result.ErrorCode != secrets.DeploymentErrorUnknownReference {
		t.Fatalf("unknown revoke result = %+v", outcome.result)
	}
}

func TestSupervisorRejectsUnknownInstallation(t *testing.T) {
	harness := newSupervisorHarness(t, &recordingProvisioner{})
	connection := harness.dial(t)
	request := secrets.EncodeDeploymentChallengeRequest(secrets.DeploymentChallengeRequest{
		InstallationID: foreignInstallationID(),
		ClientNonceB64: testClientNonce(),
	})
	if err := secrets.WriteDeploymentFrame(connection, request); err != nil {
		t.Fatal(err)
	}
	if result := harness.readResult(t, connection); result.ErrorCode != secrets.DeploymentErrorWrongInstallation {
		t.Fatalf("foreign installation result = %+v", result)
	}
}
