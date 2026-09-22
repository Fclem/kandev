package secrets

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// DeploymentSecretDomain identifies the deployment-secret control protocol.
	DeploymentSecretDomain = "kandev-deployment-secret"
	// DeploymentSecretVersion is the only accepted protocol version.
	DeploymentSecretVersion = 1
	// DeploymentSecretClassGitHubPersonalStar is the closed secret class for
	// the dedicated personal-star OAuth client secret.
	DeploymentSecretClassGitHubPersonalStar = "github-personal-star-client"
	// DeploymentSignaturePrefix is the exact ASCII domain separation prefix
	// signed together with the envelope digest.
	DeploymentSignaturePrefix = "kandev-deployment-secret/v1\x00"
	// DeploymentSignatureLength is the unpadded-Base64 length of an Ed25519 signature.
	DeploymentSignatureLength = 86
	// DeploymentRandomBytes is the required length of nonces and challenges.
	DeploymentRandomBytes = 32
	// DeploymentMaxFrameBytes bounds every frame payload.
	DeploymentMaxFrameBytes = 65536
	// DeploymentMaxSealedEnvelopeBytes bounds the sealed ciphertext payload.
	DeploymentMaxSealedEnvelopeBytes = 8192
	// DeploymentMaxSecretPlaintextBytes bounds an operator-supplied secret.
	DeploymentMaxSecretPlaintextBytes = 1024
	// DeploymentChallengeLifetime is the validity window of one challenge.
	DeploymentChallengeLifetime = 60 * time.Second

	deploymentKeyDomain = "domain"
	deploymentKeyError  = "error"
)

// DeploymentSecretAction names the action-specific envelope schema.
type DeploymentSecretAction string

const (
	DeploymentActionChallenge DeploymentSecretAction = "challenge"
	DeploymentActionProvision DeploymentSecretAction = "provision"
	DeploymentActionRevoke    DeploymentSecretAction = "revoke"
)

// DeploymentResultStatus is a closed result-frame status.
type DeploymentResultStatus string

const (
	DeploymentStatusOK    DeploymentResultStatus = "ok"
	DeploymentStatusError DeploymentResultStatus = "error"
)

// DeploymentErrorCode is a closed, non-sensitive failure code.
type DeploymentErrorCode string

const (
	DeploymentErrorInvalidRequest     DeploymentErrorCode = "invalid_request"
	DeploymentErrorInvalidSignature   DeploymentErrorCode = "invalid_signature"
	DeploymentErrorExpiredChallenge   DeploymentErrorCode = "expired_challenge"
	DeploymentErrorReplay             DeploymentErrorCode = "replay"
	DeploymentErrorWrongInstallation  DeploymentErrorCode = "wrong_installation"
	DeploymentErrorWrongClass         DeploymentErrorCode = "wrong_class"
	DeploymentErrorAlreadyProvisioned DeploymentErrorCode = "already_provisioned"
	DeploymentErrorUnknownReference   DeploymentErrorCode = "unknown_reference"
	DeploymentErrorStorage            DeploymentErrorCode = "storage_failure"
)

// DeploymentChallengeRequest is the first frame the operator CLI sends.
type DeploymentChallengeRequest struct {
	InstallationID string
	ClientNonceB64 string
}

// DeploymentChallengeResponse is the supervisor's challenge answer.
type DeploymentChallengeResponse struct {
	ChallengeIDB64 string
	ChallengeB64   string
	ExpiresAt      string
}

// DeploymentSignedRequest is the signed provision or revoke envelope.
type DeploymentSignedRequest struct {
	Action           DeploymentSecretAction
	InstallationID   string
	SecretClass      string
	ReferenceVersion int64
	CiphertextB64    string
	CiphertextDigest string
	NonceB64         string
	ChallengeIDB64   string
	ChallengeB64     string
}

// DeploymentResult is the single bounded result frame.
type DeploymentResult struct {
	Status    DeploymentResultStatus
	ErrorCode DeploymentErrorCode
}

var errDeploymentSchema = errors.New("deployment secret envelope schema violation")

// ParseDeploymentChallengeRequest validates the challenge request schema.
func ParseDeploymentChallengeRequest(raw []byte) (DeploymentChallengeRequest, error) {
	object, err := ParseJCSObject(raw)
	if err != nil {
		return DeploymentChallengeRequest{}, fmt.Errorf("deployment challenge request: %w", err)
	}
	if err := requireDeploymentKeys(object, "version", deploymentKeyDomain, "action", "installation_id", "client_nonce"); err != nil {
		return DeploymentChallengeRequest{}, err
	}
	if _, err := requireDeploymentDomainVersion(object); err != nil {
		return DeploymentChallengeRequest{}, err
	}
	action, err := object.String("action")
	if err != nil || action != string(DeploymentActionChallenge) {
		return DeploymentChallengeRequest{}, fmt.Errorf("%w: unexpected action", errDeploymentSchema)
	}
	installationID, err := requireDeploymentInstallationID(object)
	if err != nil {
		return DeploymentChallengeRequest{}, err
	}
	nonce, err := object.String("client_nonce")
	if err != nil {
		return DeploymentChallengeRequest{}, err
	}
	if _, err := DecodeDeploymentBase64("client_nonce", nonce, base64.RawStdEncoding, DeploymentRandomBytes); err != nil {
		return DeploymentChallengeRequest{}, err
	}
	return DeploymentChallengeRequest{InstallationID: installationID, ClientNonceB64: nonce}, nil
}

// EncodeDeploymentChallengeRequest renders a canonical challenge request.
func EncodeDeploymentChallengeRequest(request DeploymentChallengeRequest) []byte {
	return newJCSObject(
		intJCSField("version", DeploymentSecretVersion),
		jcsField{key: "action", text: string(DeploymentActionChallenge)},
		jcsField{key: "client_nonce", text: request.ClientNonceB64},
		jcsField{key: deploymentKeyDomain, text: DeploymentSecretDomain},
		jcsField{key: "installation_id", text: request.InstallationID},
	).Canonical()
}

// ParseDeploymentChallengeResponse validates the challenge response schema.
func ParseDeploymentChallengeResponse(raw []byte) (DeploymentChallengeResponse, error) {
	object, err := ParseJCSObject(raw)
	if err != nil {
		return DeploymentChallengeResponse{}, fmt.Errorf("deployment challenge response: %w", err)
	}
	if err := requireDeploymentKeys(object, "version", deploymentKeyDomain, "challenge_id", "challenge", "expires_at"); err != nil {
		return DeploymentChallengeResponse{}, err
	}
	if _, err := requireDeploymentDomainVersion(object); err != nil {
		return DeploymentChallengeResponse{}, err
	}
	challengeID, err := object.String("challenge_id")
	if err != nil {
		return DeploymentChallengeResponse{}, err
	}
	if _, err := DecodeDeploymentBase64("challenge_id", challengeID, base64.RawStdEncoding, DeploymentRandomBytes); err != nil {
		return DeploymentChallengeResponse{}, err
	}
	challenge, err := object.String("challenge")
	if err != nil {
		return DeploymentChallengeResponse{}, err
	}
	if _, err := DecodeDeploymentBase64("challenge", challenge, base64.RawStdEncoding, DeploymentRandomBytes); err != nil {
		return DeploymentChallengeResponse{}, err
	}
	expiresAt, err := object.String("expires_at")
	if err != nil {
		return DeploymentChallengeResponse{}, err
	}
	if err := ValidateDeploymentTimestamp(expiresAt); err != nil {
		return DeploymentChallengeResponse{}, err
	}
	return DeploymentChallengeResponse{ChallengeIDB64: challengeID, ChallengeB64: challenge, ExpiresAt: expiresAt}, nil
}

// EncodeDeploymentChallengeResponse renders a canonical challenge response.
func EncodeDeploymentChallengeResponse(response DeploymentChallengeResponse) []byte {
	return newJCSObject(
		intJCSField("version", DeploymentSecretVersion),
		jcsField{key: "challenge", text: response.ChallengeB64},
		jcsField{key: "challenge_id", text: response.ChallengeIDB64},
		jcsField{key: deploymentKeyDomain, text: DeploymentSecretDomain},
		jcsField{key: "expires_at", text: response.ExpiresAt},
	).Canonical()
}

// ParseDeploymentSignedRequest validates the action-specific signed schema.
func ParseDeploymentSignedRequest(raw []byte) (DeploymentSignedRequest, error) {
	object, err := ParseJCSObject(raw)
	if err != nil {
		return DeploymentSignedRequest{}, fmt.Errorf("deployment signed request: %w", err)
	}
	actionText, err := object.String("action")
	if err != nil {
		return DeploymentSignedRequest{}, err
	}
	switch DeploymentSecretAction(actionText) {
	case DeploymentActionProvision:
		if err := requireDeploymentKeys(object, "version", deploymentKeyDomain, "action", "installation_id", "secret_class",
			"reference_version", "ciphertext_b64", "ciphertext_digest", "nonce", "challenge_id", "challenge"); err != nil {
			return DeploymentSignedRequest{}, err
		}
	case DeploymentActionRevoke:
		if err := requireDeploymentKeys(object, "version", deploymentKeyDomain, "action", "installation_id", "secret_class",
			"reference_version", "nonce", "challenge_id", "challenge"); err != nil {
			return DeploymentSignedRequest{}, err
		}
	default:
		return DeploymentSignedRequest{}, fmt.Errorf("%w: unexpected action", errDeploymentSchema)
	}
	return decodeDeploymentSignedRequest(object)
}

func decodeDeploymentSignedRequest(object *jcsObject) (DeploymentSignedRequest, error) {
	if _, err := requireDeploymentDomainVersion(object); err != nil {
		return DeploymentSignedRequest{}, err
	}
	installationID, err := requireDeploymentInstallationID(object)
	if err != nil {
		return DeploymentSignedRequest{}, err
	}
	action, err := object.String("action")
	if err != nil {
		return DeploymentSignedRequest{}, err
	}
	secretClass, err := object.String("secret_class")
	if err != nil {
		return DeploymentSignedRequest{}, err
	}
	if secretClass != DeploymentSecretClassGitHubPersonalStar {
		return DeploymentSignedRequest{}, fmt.Errorf("%w: secret class is not supported", errDeploymentSchema)
	}
	referenceVersion, err := object.Integer("reference_version")
	if err != nil || referenceVersion < 1 {
		return DeploymentSignedRequest{}, fmt.Errorf("%w: reference_version must be a positive integer", errDeploymentSchema)
	}
	request := DeploymentSignedRequest{
		Action:           DeploymentSecretAction(action),
		InstallationID:   installationID,
		SecretClass:      secretClass,
		ReferenceVersion: referenceVersion,
	}
	if err := decodeDeploymentChallengeBinding(object, &request); err != nil {
		return DeploymentSignedRequest{}, err
	}
	if request.Action == DeploymentActionProvision {
		if err := decodeDeploymentCiphertext(object, &request); err != nil {
			return DeploymentSignedRequest{}, err
		}
	}
	return request, nil
}

func decodeDeploymentChallengeBinding(object *jcsObject, request *DeploymentSignedRequest) error {
	nonce, err := object.String("nonce")
	if err != nil {
		return err
	}
	if _, err := DecodeDeploymentBase64("nonce", nonce, base64.RawStdEncoding, DeploymentRandomBytes); err != nil {
		return err
	}
	challengeID, err := object.String("challenge_id")
	if err != nil {
		return err
	}
	if _, err := DecodeDeploymentBase64("challenge_id", challengeID, base64.RawStdEncoding, DeploymentRandomBytes); err != nil {
		return err
	}
	challenge, err := object.String("challenge")
	if err != nil {
		return err
	}
	if _, err := DecodeDeploymentBase64("challenge", challenge, base64.RawStdEncoding, DeploymentRandomBytes); err != nil {
		return err
	}
	request.NonceB64 = nonce
	request.ChallengeIDB64 = challengeID
	request.ChallengeB64 = challenge
	return nil
}

func decodeDeploymentCiphertext(object *jcsObject, request *DeploymentSignedRequest) error {
	ciphertext, err := object.String("ciphertext_b64")
	if err != nil {
		return err
	}
	decoded, err := DecodeDeploymentBase64("ciphertext_b64", ciphertext, base64.RawStdEncoding, 0)
	if err != nil {
		return err
	}
	if len(decoded) == 0 || len(decoded) > DeploymentMaxSealedEnvelopeBytes {
		return fmt.Errorf("%w: ciphertext_b64 length is out of bounds", errDeploymentSchema)
	}
	digest, err := object.String("ciphertext_digest")
	if err != nil {
		return err
	}
	if digest != DeploymentCiphertextDigest(decoded) {
		return fmt.Errorf("%w: ciphertext_digest does not match ciphertext_b64", errDeploymentSchema)
	}
	request.CiphertextB64 = ciphertext
	request.CiphertextDigest = digest
	return nil
}

// EncodeDeploymentSignedRequest renders a canonical signed request.
func EncodeDeploymentSignedRequest(request DeploymentSignedRequest) []byte {
	fields := []jcsField{
		intJCSField("version", DeploymentSecretVersion),
		jcsField{key: "action", text: string(request.Action)},
		jcsField{key: "challenge", text: request.ChallengeB64},
		jcsField{key: "challenge_id", text: request.ChallengeIDB64},
		jcsField{key: deploymentKeyDomain, text: DeploymentSecretDomain},
		jcsField{key: "installation_id", text: request.InstallationID},
		jcsField{key: "nonce", text: request.NonceB64},
		intJCSField("reference_version", request.ReferenceVersion),
		jcsField{key: "secret_class", text: request.SecretClass},
	}
	if request.Action == DeploymentActionProvision {
		fields = append(fields,
			jcsField{key: "ciphertext_b64", text: request.CiphertextB64},
			jcsField{key: "ciphertext_digest", text: request.CiphertextDigest},
		)
	}
	return newJCSObject(fields...).Canonical()
}

// ParseDeploymentResult validates the bounded result frame.
func ParseDeploymentResult(raw []byte) (DeploymentResult, error) {
	object, err := ParseJCSObject(raw)
	if err != nil {
		return DeploymentResult{}, fmt.Errorf("deployment result: %w", err)
	}
	status, err := object.String("status")
	if err != nil {
		return DeploymentResult{}, err
	}
	switch DeploymentResultStatus(status) {
	case DeploymentStatusOK:
		if err := requireDeploymentKeys(object, "version", deploymentKeyDomain, "status"); err != nil {
			return DeploymentResult{}, err
		}
		if _, err := requireDeploymentDomainVersion(object); err != nil {
			return DeploymentResult{}, err
		}
		return DeploymentResult{Status: DeploymentStatusOK}, nil
	case DeploymentStatusError:
		if err := requireDeploymentKeys(object, "version", deploymentKeyDomain, "status", deploymentKeyError); err != nil {
			return DeploymentResult{}, err
		}
		if _, err := requireDeploymentDomainVersion(object); err != nil {
			return DeploymentResult{}, err
		}
		code, err := object.String("error")
		if err != nil {
			return DeploymentResult{}, err
		}
		if !IsDeploymentErrorCode(code) {
			return DeploymentResult{}, fmt.Errorf("%w: unknown error code", errDeploymentSchema)
		}
		return DeploymentResult{Status: DeploymentStatusError, ErrorCode: DeploymentErrorCode(code)}, nil
	default:
		return DeploymentResult{}, fmt.Errorf("%w: unexpected status", errDeploymentSchema)
	}
}

// EncodeDeploymentResult renders a canonical result frame.
func EncodeDeploymentResult(result DeploymentResult) []byte {
	fields := []jcsField{
		intJCSField("version", DeploymentSecretVersion),
		jcsField{key: deploymentKeyDomain, text: DeploymentSecretDomain},
		jcsField{key: "status", text: string(result.Status)},
	}
	if result.Status != DeploymentStatusOK {
		fields = append(fields, jcsField{key: deploymentKeyError, text: string(result.ErrorCode)})
	}
	return newJCSObject(fields...).Canonical()
}

// DeploymentErrorCodes lists every accepted result error code.
func DeploymentErrorCodes() []DeploymentErrorCode {
	return []DeploymentErrorCode{
		DeploymentErrorInvalidRequest,
		DeploymentErrorInvalidSignature,
		DeploymentErrorExpiredChallenge,
		DeploymentErrorReplay,
		DeploymentErrorWrongInstallation,
		DeploymentErrorWrongClass,
		DeploymentErrorAlreadyProvisioned,
		DeploymentErrorUnknownReference,
		DeploymentErrorStorage,
	}
}

// IsDeploymentErrorCode reports whether code belongs to the closed set.
func IsDeploymentErrorCode(code string) bool {
	for _, candidate := range DeploymentErrorCodes() {
		if string(candidate) == code {
			return true
		}
	}
	return false
}

func requireDeploymentKeys(object *jcsObject, keys ...string) error {
	if object.Len() != len(keys) {
		return fmt.Errorf("%w: unexpected field set", errDeploymentSchema)
	}
	for _, key := range keys {
		if !object.Has(key) {
			return fmt.Errorf("%w: missing field %q", errDeploymentSchema, key)
		}
	}
	return nil
}

func requireDeploymentDomainVersion(object *jcsObject) (int64, error) {
	version, err := object.Integer("version")
	if err != nil || version != DeploymentSecretVersion {
		return 0, fmt.Errorf("%w: unsupported version", errDeploymentSchema)
	}
	domain, err := object.String(deploymentKeyDomain)
	if err != nil || domain != DeploymentSecretDomain {
		return 0, fmt.Errorf("%w: unexpected domain", errDeploymentSchema)
	}
	return version, nil
}

func requireDeploymentInstallationID(object *jcsObject) (string, error) {
	installationID, err := object.String("installation_id")
	if err != nil {
		return "", err
	}
	if err := ValidateDeploymentInstallationID(installationID); err != nil {
		return "", err
	}
	return installationID, nil
}

// ValidateDeploymentInstallationID enforces the bootstrap identity spelling.
func ValidateDeploymentInstallationID(installationID string) error {
	if len(installationID) != 2*DeploymentInstallationIDBytes {
		return fmt.Errorf("%w: installation_id length is invalid", errDeploymentSchema)
	}
	if _, err := hex.DecodeString(installationID); err != nil || strings.ToLower(installationID) != installationID {
		return fmt.Errorf("%w: installation_id is not lowercase hexadecimal", errDeploymentSchema)
	}
	return nil
}

// ValidateDeploymentTimestamp accepts only UTC RFC3339Nano with a trailing Z.
func ValidateDeploymentTimestamp(value string) error {
	if !strings.HasSuffix(value, "Z") {
		return fmt.Errorf("%w: timestamp must be UTC", errDeploymentSchema)
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		return fmt.Errorf("%w: timestamp is malformed", errDeploymentSchema)
	}
	return nil
}

// DecodeDeploymentBase64 decodes a strict RFC 4648 value and requires that
// re-encoding reproduces the received text exactly.
func DecodeDeploymentBase64(field, value string, encoding *base64.Encoding, size int) ([]byte, error) {
	decoded, err := encoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%w: %s is not valid base64", errDeploymentSchema, field)
	}
	if encoding.EncodeToString(decoded) != value {
		return nil, fmt.Errorf("%w: %s is not canonical base64", errDeploymentSchema, field)
	}
	if size > 0 && len(decoded) != size {
		return nil, fmt.Errorf("%w: %s length is invalid", errDeploymentSchema, field)
	}
	return decoded, nil
}

// DeploymentCiphertextDigest is the lowercase hex SHA-256 of decoded ciphertext.
func DeploymentCiphertextDigest(ciphertext []byte) string {
	digest := sha256.Sum256(ciphertext)
	return hex.EncodeToString(digest[:])
}

// DeploymentEnvelopeMessage builds the exact message covered by the detached
// Ed25519 signature: the fixed ASCII prefix plus SHA-256(envelope bytes).
func DeploymentEnvelopeMessage(envelope []byte) []byte {
	digest := sha256.Sum256(envelope)
	message := make([]byte, 0, len(DeploymentSignaturePrefix)+len(digest))
	message = append(message, DeploymentSignaturePrefix...)
	return append(message, digest[:]...)
}

// SignDeploymentEnvelope signs an envelope with the deployment root key.
func SignDeploymentEnvelope(privateKey ed25519.PrivateKey, envelope []byte) string {
	signature := ed25519.Sign(privateKey, DeploymentEnvelopeMessage(envelope))
	return base64.RawStdEncoding.EncodeToString(signature)
}

// VerifyDeploymentEnvelopeSignature verifies the detached signature frame.
func VerifyDeploymentEnvelopeSignature(publicKey ed25519.PublicKey, envelope []byte, encoded string) error {
	signature, err := DecodeDeploymentBase64("signature", encoded, base64.RawStdEncoding, ed25519.SignatureSize)
	if err != nil {
		return fmt.Errorf("deployment signature: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return errors.New("deployment signature: root public key is unavailable")
	}
	if !ed25519.Verify(publicKey, DeploymentEnvelopeMessage(envelope), signature) {
		return errors.New("deployment signature: verification failed")
	}
	return nil
}
