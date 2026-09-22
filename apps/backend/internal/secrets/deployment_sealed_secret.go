package secrets

import (
	"encoding/base64"
	"errors"
	"fmt"
)

const (
	deploymentSealedAlgorithm    = "AES-256-GCM"
	deploymentSealedNonceBytes   = 12
	deploymentSealedTagBytes     = 16
	deploymentSealedKeyIDMaxSize = 64
)

var errDeploymentSealedEnvelope = errors.New("deployment sealed envelope violation")

// SealedDeploymentSecret is the AES-256-GCM JCS envelope that carries an
// operator-supplied secret to the deployment store. Only the envelope bytes
// and its canonical spelling ever leave the operator CLI.
type SealedDeploymentSecret struct {
	KeyID      string
	Nonce      []byte
	Ciphertext []byte
	canonical  []byte
}

// SealDeploymentSecret encrypts plaintext and renders the canonical envelope.
func SealDeploymentSecret(keyID string, key, plaintext []byte) (*SealedDeploymentSecret, error) {
	if err := validateDeploymentSealedKeyID(keyID); err != nil {
		return nil, err
	}
	if len(plaintext) == 0 || len(plaintext) > DeploymentMaxSecretPlaintextBytes {
		return nil, fmt.Errorf("%w: plaintext length is out of bounds", errDeploymentSealedEnvelope)
	}
	if len(key) != MasterKeySize {
		return nil, fmt.Errorf("%w: encryption key is unavailable", errDeploymentSealedEnvelope)
	}
	ciphertext, nonce, err := Encrypt(plaintext, key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != deploymentSealedNonceBytes {
		return nil, fmt.Errorf("%w: sealed nonce length is unsupported", errDeploymentSealedEnvelope)
	}
	sealed := &SealedDeploymentSecret{KeyID: keyID, Nonce: nonce, Ciphertext: ciphertext}
	sealed.canonical = renderDeploymentSealedEnvelope(sealed)
	return sealed, nil
}

// ParseSealedDeploymentSecret validates a canonical sealed envelope and binds
// it to the expected master-key identity.
func ParseSealedDeploymentSecret(raw []byte, expectedKeyID string) (*SealedDeploymentSecret, error) {
	object, err := ParseJCSObject(raw)
	if err != nil {
		return nil, fmt.Errorf("parse sealed envelope: %w", err)
	}
	if err := validateDeploymentSealedHeader(object, expectedKeyID); err != nil {
		return nil, err
	}
	keyID, _ := object.String("key_id")
	nonce, err := decodeDeploymentSealedNonce(object)
	if err != nil {
		return nil, err
	}
	ciphertext, err := decodeDeploymentSealedCiphertext(object)
	if err != nil {
		return nil, err
	}
	return &SealedDeploymentSecret{
		KeyID:      keyID,
		Nonce:      nonce,
		Ciphertext: ciphertext,
		canonical:  append([]byte(nil), raw...),
	}, nil
}

func validateDeploymentSealedHeader(object *jcsObject, expectedKeyID string) error {
	if err := requireDeploymentKeys(object, "version", "algorithm", "key_id", "nonce_b64", "ciphertext_b64"); err != nil {
		return fmt.Errorf("sealed envelope: %w", err)
	}
	version, err := object.Integer("version")
	if err != nil || version != DeploymentSecretVersion {
		return fmt.Errorf("%w: unsupported version", errDeploymentSealedEnvelope)
	}
	algorithm, err := object.String("algorithm")
	if err != nil || algorithm != deploymentSealedAlgorithm {
		return fmt.Errorf("%w: unsupported algorithm", errDeploymentSealedEnvelope)
	}
	keyID, err := object.String("key_id")
	if err != nil {
		return err
	}
	if err := validateDeploymentSealedKeyID(keyID); err != nil {
		return err
	}
	if expectedKeyID != "" && keyID != expectedKeyID {
		return fmt.Errorf("%w: key identity does not match the deployment master key", errDeploymentSealedEnvelope)
	}
	return nil
}

func decodeDeploymentSealedNonce(object *jcsObject) ([]byte, error) {
	nonceText, err := object.String("nonce_b64")
	if err != nil {
		return nil, err
	}
	return DecodeDeploymentBase64("nonce_b64", nonceText, base64.RawStdEncoding, deploymentSealedNonceBytes)
}

func decodeDeploymentSealedCiphertext(object *jcsObject) ([]byte, error) {
	ciphertextText, err := object.String("ciphertext_b64")
	if err != nil {
		return nil, err
	}
	ciphertext, err := DecodeDeploymentBase64("ciphertext_b64", ciphertextText, base64.RawStdEncoding, 0)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < deploymentSealedTagBytes || len(ciphertext) > DeploymentMaxSealedEnvelopeBytes {
		return nil, fmt.Errorf("%w: ciphertext length is out of bounds", errDeploymentSealedEnvelope)
	}
	return ciphertext, nil
}

// Bytes returns the canonical sealed envelope.
func (s *SealedDeploymentSecret) Bytes() []byte {
	if s.canonical != nil {
		return append([]byte(nil), s.canonical...)
	}
	return renderDeploymentSealedEnvelope(s)
}

// Open decrypts the sealed payload with the deployment master key.
func (s *SealedDeploymentSecret) Open(key []byte) (string, error) {
	if len(key) != MasterKeySize {
		return "", fmt.Errorf("%w: decryption key is unavailable", errDeploymentSealedEnvelope)
	}
	plaintext, err := Decrypt(s.Ciphertext, s.Nonce, key)
	if err != nil {
		return "", fmt.Errorf("%w: sealed payload could not be decrypted", errDeploymentSealedEnvelope)
	}
	if len(plaintext) == 0 {
		return "", fmt.Errorf("%w: sealed payload is empty", errDeploymentSealedEnvelope)
	}
	return string(plaintext), nil
}

func renderDeploymentSealedEnvelope(sealed *SealedDeploymentSecret) []byte {
	return newJCSObject(
		intJCSField("version", DeploymentSecretVersion),
		jcsField{key: "algorithm", text: deploymentSealedAlgorithm},
		jcsField{key: "ciphertext_b64", text: base64.RawStdEncoding.EncodeToString(sealed.Ciphertext)},
		jcsField{key: "key_id", text: sealed.KeyID},
		jcsField{key: "nonce_b64", text: base64.RawStdEncoding.EncodeToString(sealed.Nonce)},
	).Canonical()
}

func validateDeploymentSealedKeyID(keyID string) error {
	if keyID == "" || len(keyID) > deploymentSealedKeyIDMaxSize {
		return fmt.Errorf("%w: key identity is invalid", errDeploymentSealedEnvelope)
	}
	for _, char := range keyID {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
		case char == '-', char == '_', char == '.', char == ':':
		default:
			return fmt.Errorf("%w: key identity is invalid", errDeploymentSealedEnvelope)
		}
	}
	return nil
}
