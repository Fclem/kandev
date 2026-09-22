package cli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/kandev/kandev/internal/launcher/deploymentsecretd"
	"github.com/kandev/kandev/internal/secrets"
)

const (
	deploymentSecretSocketNetwork = "unix"
	// deploymentSecretExchangeTimeout bounds one whole challenge/sign/result
	// exchange. The supervisor applies the same 30s deadline to a connection.
	deploymentSecretExchangeTimeout = 30 * time.Second
)

// bootstrap creates the deployment identity exactly once. It is the only
// command that writes deployment-owned trust material.
func (r *deploymentSecretRunner) bootstrap(options deploymentSecretOptions) int {
	identity, err := secrets.BootstrapDeploymentIdentity(options.dataDir)
	if err != nil {
		r.fail(fmt.Errorf("bootstrap deployment identity in %s: %w", options.dataDir, err))
		return deploymentSecretExitFailure
	}
	_, _ = fmt.Fprintf(r.out, "deployment installation id: %s\n", identity.InstallationID)
	return 0
}

func (r *deploymentSecretRunner) provisionOrRevoke(options deploymentSecretOptions) int {
	identity, err := secrets.LoadDeploymentIdentity(options.dataDir)
	if err != nil {
		r.fail(fmt.Errorf("load deployment identity from %s: %w (run `kandev %s %s` first)",
			options.dataDir, err, deploymentSecretGroup, deploymentSecretActionBootstrap))
		return deploymentSecretExitFailure
	}
	privateKey, err := secrets.LoadDeploymentRootPrivateKey(options.dataDir)
	if err != nil {
		r.fail(fmt.Errorf("load deployment root private key from %s: %w", options.dataDir, err))
		return deploymentSecretExitFailure
	}
	request := secrets.DeploymentSignedRequest{
		Action:           deploymentSecretProtocolAction(options.action),
		InstallationID:   identity.InstallationID,
		SecretClass:      secrets.DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: options.version,
	}
	if options.action == deploymentSecretActionProvision {
		sealed, err := r.sealClientSecret(options)
		if err != nil {
			r.fail(err)
			return deploymentSecretExitFailure
		}
		request.CiphertextB64 = base64.RawStdEncoding.EncodeToString(sealed)
		request.CiphertextDigest = secrets.DeploymentCiphertextDigest(sealed)
	}
	result, err := r.exchange(options.socket, request, privateKey)
	if err != nil {
		r.fail(err)
		return deploymentSecretExitFailure
	}
	if result.Status != secrets.DeploymentStatusOK {
		r.fail(deploymentSecretResultError(result))
		return deploymentSecretExitFailure
	}
	_, _ = fmt.Fprintf(r.out, "deployment secret %s: reference=%s\n",
		deploymentSecretPastTense(options.action), secrets.DeploymentSecretReference(options.version))
	return 0
}

// sealClientSecret reads the operator's plaintext and seals it with the
// existing deployment master key. The key is opened read-only: provisioning
// never creates or rotates it.
func (r *deploymentSecretRunner) sealClientSecret(options deploymentSecretOptions) ([]byte, error) {
	plaintext, err := r.readClientSecret(options.clientSecretFile)
	if err != nil {
		return nil, err
	}
	keyring, err := secrets.DeploymentMasterKeyringOpenExisting(options.dataDir)
	if err != nil {
		return nil, fmt.Errorf("open the existing deployment master key in %s: %w (start the backend once so it creates the key)", options.dataDir, err)
	}
	sealed, err := secrets.SealDeploymentSecret(keyring.ID(), keyring.Key(), plaintext)
	if err != nil {
		return nil, fmt.Errorf("seal the github-personal-star client secret: %w", err)
	}
	return sealed.Bytes(), nil
}

func (r *deploymentSecretRunner) readClientSecret(path string) ([]byte, error) {
	// Read one byte past the limit plus the optional trailing newline so an
	// oversized secret is rejected instead of silently truncated.
	reader, closeReader, err := r.clientSecretReader(path)
	if err != nil {
		return nil, err
	}
	defer closeReader()
	data, err := io.ReadAll(io.LimitReader(reader, int64(secrets.DeploymentMaxSecretPlaintextBytes)+2))
	if err != nil {
		return nil, fmt.Errorf("read the client secret: %w", err)
	}
	plaintext := trimDeploymentSecretLineBreak(data)
	if len(plaintext) == 0 {
		return nil, errors.New("the client secret is empty")
	}
	if len(plaintext) > secrets.DeploymentMaxSecretPlaintextBytes {
		return nil, fmt.Errorf("the client secret must be at most %d bytes", secrets.DeploymentMaxSecretPlaintextBytes)
	}
	return plaintext, nil
}

func (r *deploymentSecretRunner) clientSecretReader(path string) (io.Reader, func(), error) {
	if path == deploymentSecretStdinPath {
		return r.input, func() {}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read the client secret file: %w", err)
	}
	return file, func() { _ = file.Close() }, nil
}

// trimDeploymentSecretLineBreak drops one trailing line break so secrets
// written by an editor or `echo` are not sealed with a stray newline. Every
// other byte, including interior whitespace, is sealed verbatim.
func trimDeploymentSecretLineBreak(data []byte) []byte {
	return bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r"))
}

// exchange performs the fixed one-request-per-connection handshake: challenge
// frame, signed provision/revoke frame, result frame.
func (r *deploymentSecretRunner) exchange(
	socketPath string,
	request secrets.DeploymentSignedRequest,
	privateKey ed25519.PrivateKey,
) (secrets.DeploymentResult, error) {
	connection, err := net.DialUnix(deploymentSecretSocketNetwork, nil, &net.UnixAddr{
		Name: socketPath,
		Net:  deploymentSecretSocketNetwork,
	})
	if err != nil {
		return secrets.DeploymentResult{}, fmt.Errorf("connect to the deployment secret daemon at %s: %w (is %s running as root?)",
			socketPath, err, deploymentsecretd.UnitName)
	}
	defer func() { _ = connection.Close() }()
	if err := connection.SetDeadline(time.Now().Add(deploymentSecretExchangeTimeout)); err != nil {
		return secrets.DeploymentResult{}, fmt.Errorf("set the deployment secret socket deadline: %w", err)
	}
	nonce, err := deploymentSecretNonce()
	if err != nil {
		return secrets.DeploymentResult{}, err
	}
	challenge := secrets.EncodeDeploymentChallengeRequest(secrets.DeploymentChallengeRequest{
		InstallationID: request.InstallationID,
		ClientNonceB64: nonce,
	})
	if err := secrets.WriteDeploymentFrame(connection, challenge); err != nil {
		return secrets.DeploymentResult{}, fmt.Errorf("send the deployment secret challenge: %w", err)
	}
	rawResponse, err := secrets.ReadDeploymentFrame(connection)
	if err != nil {
		return secrets.DeploymentResult{}, fmt.Errorf("read the deployment secret challenge: %w", err)
	}
	response, err := secrets.ParseDeploymentChallengeResponse(rawResponse)
	if err != nil {
		// A refusal in the challenge phase arrives as a result frame, so the
		// operator sees the closed error code instead of a schema complaint.
		if refusal, refusalErr := secrets.ParseDeploymentResult(rawResponse); refusalErr == nil {
			return secrets.DeploymentResult{}, deploymentSecretResultError(refusal)
		}
		return secrets.DeploymentResult{}, fmt.Errorf("parse the deployment secret challenge: %w", err)
	}
	request.NonceB64 = nonce
	request.ChallengeIDB64 = response.ChallengeIDB64
	request.ChallengeB64 = response.ChallengeB64
	envelope := secrets.EncodeDeploymentSignedRequest(request)
	if err := secrets.WriteDeploymentSignedRequest(connection, envelope, secrets.SignDeploymentEnvelope(privateKey, envelope)); err != nil {
		return secrets.DeploymentResult{}, fmt.Errorf("send the signed deployment secret request: %w", err)
	}
	// The supervisor requires EOF at the signed-frame boundary before it
	// answers, so half-close the write side while the read side stays open.
	if err := connection.CloseWrite(); err != nil {
		return secrets.DeploymentResult{}, fmt.Errorf("half-close the deployment secret socket: %w", err)
	}
	rawResult, err := secrets.ReadDeploymentFrame(connection)
	if err != nil {
		return secrets.DeploymentResult{}, fmt.Errorf("read the deployment secret result: %w", err)
	}
	result, err := secrets.ParseDeploymentResult(rawResult)
	if err != nil {
		return secrets.DeploymentResult{}, fmt.Errorf("parse the deployment secret result: %w", err)
	}
	return result, nil
}

func deploymentSecretNonce() (string, error) {
	buffer := make([]byte, secrets.DeploymentRandomBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate the deployment secret client nonce: %w", err)
	}
	return base64.RawStdEncoding.EncodeToString(buffer), nil
}

// deploymentSecretResultError maps the closed supervisor result codes onto
// operator actions. Secret material is never part of a message.
func deploymentSecretResultError(result secrets.DeploymentResult) error {
	switch result.ErrorCode {
	case secrets.DeploymentErrorInvalidRequest:
		return errors.New("the daemon rejected the request as malformed; verify the operator CLI and the daemon are the same version")
	case secrets.DeploymentErrorInvalidSignature:
		return errors.New("the daemon rejected the request signature; --data-dir must hold the identity the daemon serves")
	case secrets.DeploymentErrorExpiredChallenge:
		return errors.New("the provisioning challenge expired before the request was signed; re-run the command")
	case secrets.DeploymentErrorReplay:
		return errors.New("the daemon rejected the challenge as unknown or already consumed; re-run the command")
	case secrets.DeploymentErrorWrongInstallation:
		return errors.New("the daemon serves a different deployment installation id; pass the daemon's --data-dir")
	case secrets.DeploymentErrorWrongClass:
		return errors.New("the reference is not this deployment's github-personal-star client secret")
	case secrets.DeploymentErrorAlreadyProvisioned:
		return errors.New("that --version is already provisioned; reference versions are immutable, so provision a new --version instead")
	case secrets.DeploymentErrorUnknownReference:
		return errors.New("that --version is not provisioned; revoke requires an existing reference version")
	case secrets.DeploymentErrorStorage:
		return errors.New("the daemon could not update the deployment secret store; check the daemon journal and the data directory permissions")
	default:
		return fmt.Errorf("the daemon returned the unrecognized error code %q", result.ErrorCode)
	}
}

func deploymentSecretProtocolAction(action string) secrets.DeploymentSecretAction {
	if action == deploymentSecretActionRevoke {
		return secrets.DeploymentActionRevoke
	}
	return secrets.DeploymentActionProvision
}

func deploymentSecretPastTense(action string) string {
	if action == deploymentSecretActionRevoke {
		return "revoked"
	}
	return "provisioned"
}
