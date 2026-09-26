package secrets

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func protocolTestIdentity(t *testing.T) DeploymentIdentity {
	t.Helper()
	identity, err := BootstrapDeploymentIdentity(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestBootstrapDeploymentIdentityIsIdempotentAndReadOnlyLoad(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadDeploymentIdentity(dir); err == nil {
		t.Fatal("LoadDeploymentIdentity() succeeded without a bootstrapped identity")
	}
	if _, err := os.Stat(DeploymentIdentityDir(dir)); !os.IsNotExist(err) {
		t.Fatalf("deployment identity dir stat = %v, want absent", err)
	}

	first, err := BootstrapDeploymentIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BootstrapDeploymentIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.InstallationID != second.InstallationID || !bytes.Equal(first.RootPublicKey, second.RootPublicKey) {
		t.Fatal("BootstrapDeploymentIdentity() regenerated the identity")
	}
	loaded, err := LoadDeploymentIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.InstallationID != first.InstallationID || !bytes.Equal(loaded.RootPublicKey, first.RootPublicKey) {
		t.Fatal("LoadDeploymentIdentity() did not reproduce the bootstrapped identity")
	}
	if err := ValidateDeploymentInstallationID(first.InstallationID); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapDeploymentIdentityPermissionsSeparatePublicAndPrivateMaterial(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := BootstrapDeploymentIdentity(dataDir); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]os.FileMode{
		DeploymentIdentityDir(dataDir):                                              0o755,
		filepath.Join(DeploymentIdentityDir(dataDir), deploymentInstallationIDFile): 0o644,
		filepath.Join(DeploymentIdentityDir(dataDir), deploymentPublicKeyFile):      0o644,
		filepath.Join(DeploymentIdentityDir(dataDir), deploymentPrivateKeyFile):     0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %q: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("permissions for %q = %04o, want %04o", path, got, want)
		}
	}
}

func TestBootstrapDeploymentIdentityRepairsPublicPermissionsOnExistingIdentity(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := BootstrapDeploymentIdentity(dataDir); err != nil {
		t.Fatal(err)
	}
	dir := DeploymentIdentityDir(dataDir)
	publicFiles := []string{
		filepath.Join(dir, deploymentInstallationIDFile),
		filepath.Join(dir, deploymentPublicKeyFile),
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range publicFiles {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := BootstrapDeploymentIdentity(dataDir); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{
		dir:            0o755,
		publicFiles[0]: 0o644,
		publicFiles[1]: 0o644,
		filepath.Join(dir, deploymentPrivateKeyFile): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %q: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("permissions for %q = %04o, want %04o", path, got, want)
		}
	}
}

func TestBootstrapDeploymentIdentityRejectsPartialIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(DeploymentIdentityDir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(DeploymentIdentityDir(dir), deploymentInstallationIDFile)
	if err := os.WriteFile(path, []byte("00ff\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BootstrapDeploymentIdentity(dir); err == nil {
		t.Fatal("BootstrapDeploymentIdentity() repaired a partial identity")
	}
}

func TestParseJCSObjectRejectsNonCanonicalAndDuplicateKeys(t *testing.T) {
	canonical := []byte(`{"a":"1","b":2}`)
	if _, err := ParseJCSObject(canonical); err != nil {
		t.Fatalf("ParseJCSObject() error = %v", err)
	}
	for name, raw := range map[string]string{
		"unsorted":       `{"b":2,"a":"1"}`,
		"whitespace":     `{"a": "1","b":2}`,
		"duplicate":      `{"a":"1","a":"1","b":2}`,
		"trailing":       `{"a":"1","b":2} `,
		"escaping":       `{"a":"\u0031","b":2}`,
		"float":          `{"a":"1","b":2.0}`,
		"nested":         `{"a":"1","b":{"c":2}}`,
		"trailing-value": `{"a":"1","b":2}garbage`,
	} {
		if _, err := ParseJCSObject([]byte(raw)); err == nil {
			t.Fatalf("ParseJCSObject(%s) succeeded, want rejection", name)
		}
	}
	object, err := ParseJCSObject([]byte(`{"a":"x\\ny","b":-3}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(object.Canonical(), []byte(`{"a":"x\\ny","b":-3}`)) {
		t.Fatalf("Canonical() = %s", object.Canonical())
	}
}

func TestDeploymentEnvelopeSchemasAreActionSpecific(t *testing.T) {
	identity := protocolTestIdentity(t)
	nonce := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{1}, DeploymentRandomBytes))
	challengeID := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{2}, DeploymentRandomBytes))
	challenge := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{3}, DeploymentRandomBytes))

	challengeRequest := EncodeDeploymentChallengeRequest(DeploymentChallengeRequest{
		InstallationID: identity.InstallationID,
		ClientNonceB64: nonce,
	})
	parsedRequest, err := ParseDeploymentChallengeRequest(challengeRequest)
	if err != nil {
		t.Fatal(err)
	}
	if parsedRequest.InstallationID != identity.InstallationID || parsedRequest.ClientNonceB64 != nonce {
		t.Fatalf("parsed challenge request = %+v", parsedRequest)
	}

	provision := EncodeDeploymentSignedRequest(DeploymentSignedRequest{
		Action:           DeploymentActionProvision,
		InstallationID:   identity.InstallationID,
		SecretClass:      DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: 2,
		CiphertextB64:    "AAAA",
		CiphertextDigest: DeploymentCiphertextDigest([]byte{0, 0, 0}),
		NonceB64:         nonce,
		ChallengeIDB64:   challengeID,
		ChallengeB64:     challenge,
	})
	if !strings.Contains(string(provision), `"reference_version":2`) {
		t.Fatalf("provision envelope = %s", provision)
	}
	parsed, err := ParseDeploymentSignedRequest(provision)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Action != DeploymentActionProvision || parsed.ReferenceVersion != 2 || parsed.CiphertextB64 != "AAAA" {
		t.Fatalf("parsed provision = %+v", parsed)
	}

	revoke := EncodeDeploymentSignedRequest(DeploymentSignedRequest{
		Action:           DeploymentActionRevoke,
		InstallationID:   identity.InstallationID,
		SecretClass:      DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: 2,
		NonceB64:         nonce,
		ChallengeIDB64:   challengeID,
		ChallengeB64:     challenge,
	})
	if _, err := ParseDeploymentSignedRequest(revoke); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseDeploymentSignedRequest(provision); err != nil {
		t.Fatal(err)
	}

	// Cross-action fields and mutated digests must fail closed.
	withCiphertext := []byte(strings.Replace(string(revoke), `"action":"revoke"`,
		`"action":"revoke","ciphertext_b64":"AAAA"`, 1))
	if _, err := ParseDeploymentSignedRequest(withCiphertext); err == nil {
		t.Fatal("ParseDeploymentSignedRequest() accepted a cross-action field")
	}
	mutatedDigest := bytes.Replace(provision, []byte(parsed.CiphertextDigest), []byte(strings.Repeat("0", 64)), 1)
	if _, err := ParseDeploymentSignedRequest(mutatedDigest); err == nil {
		t.Fatal("ParseDeploymentSignedRequest() accepted a mismatched digest")
	}
	paddedNonce := bytes.Replace(provision, []byte(nonce), []byte(nonce+"="), 1)
	if _, err := ParseDeploymentSignedRequest(paddedNonce); err == nil {
		t.Fatal("ParseDeploymentSignedRequest() accepted padded base64")
	}
}

func TestDeploymentEnvelopeSignatureBindsEnvelopeBytes(t *testing.T) {
	identity := protocolTestIdentity(t)
	publicKey, generatedPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := EncodeDeploymentChallengeRequest(DeploymentChallengeRequest{
		InstallationID: identity.InstallationID,
		ClientNonceB64: base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{9}, DeploymentRandomBytes)),
	})
	signature := SignDeploymentEnvelope(generatedPrivateKey, envelope)
	if len(signature) != DeploymentSignatureLength {
		t.Fatalf("signature length = %d, want %d", len(signature), DeploymentSignatureLength)
	}
	if err := VerifyDeploymentEnvelopeSignature(publicKey, envelope, signature); err != nil {
		t.Fatal(err)
	}
	if err := VerifyDeploymentEnvelopeSignature(publicKey, append(envelope, ' '), signature); err == nil {
		t.Fatal("VerifyDeploymentEnvelopeSignature() accepted a mutated envelope")
	}
	if err := VerifyDeploymentEnvelopeSignature(publicKey, envelope, signature[:len(signature)-1]); err == nil {
		t.Fatal("VerifyDeploymentEnvelopeSignature() accepted a truncated signature")
	}
}

func TestDeploymentFramesEnforceFixedShape(t *testing.T) {
	var buffer bytes.Buffer
	envelope := []byte(`{"version":1}`)
	signature := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{7}, ed25519.SignatureSize))
	if err := WriteDeploymentSignedRequest(&buffer, envelope, signature); err != nil {
		t.Fatal(err)
	}
	frame := append([]byte(nil), buffer.Bytes()...)
	readEnvelope, readSignature, err := ReadDeploymentSignedRequest(bytes.NewReader(frame))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readEnvelope, envelope) || readSignature != signature {
		t.Fatalf("round trip = %s, %s", readEnvelope, readSignature)
	}

	withTrailing := append(append([]byte(nil), frame...), 0)
	if _, _, err := ReadDeploymentSignedRequest(bytes.NewReader(withTrailing)); err == nil {
		t.Fatal("ReadDeploymentSignedRequest() accepted trailing bytes")
	}
	truncated := frame[:len(frame)-4]
	if _, _, err := ReadDeploymentSignedRequest(bytes.NewReader(truncated)); err == nil {
		t.Fatal("ReadDeploymentSignedRequest() accepted a truncated frame")
	}
	concatenated := append(append([]byte(nil), frame...), frame...)
	if _, _, err := ReadDeploymentSignedRequest(bytes.NewReader(concatenated)); err == nil {
		t.Fatal("ReadDeploymentSignedRequest() accepted a concatenated frame")
	}
	if err := WriteDeploymentFrame(&bytes.Buffer{}, nil); err == nil {
		t.Fatal("WriteDeploymentFrame() accepted an empty payload")
	}
	oversized := make([]byte, DeploymentMaxFrameBytes+1)
	if err := WriteDeploymentFrame(&bytes.Buffer{}, oversized); err == nil {
		t.Fatal("WriteDeploymentFrame() accepted an oversized payload")
	}
}

func TestDeploymentResultsAreClosed(t *testing.T) {
	ok := EncodeDeploymentResult(DeploymentResult{Status: DeploymentStatusOK})
	parsed, err := ParseDeploymentResult(ok)
	if err != nil || parsed.Status != DeploymentStatusOK {
		t.Fatalf("ParseDeploymentResult() = %+v, %v", parsed, err)
	}
	failure := EncodeDeploymentResult(DeploymentResult{Status: DeploymentStatusError, ErrorCode: DeploymentErrorReplay})
	parsed, err = ParseDeploymentResult(failure)
	if err != nil || parsed.ErrorCode != DeploymentErrorReplay {
		t.Fatalf("ParseDeploymentResult() = %+v, %v", parsed, err)
	}
	unknown := []byte(`{"domain":"kandev-deployment-secret","error":"surprise","status":"error","version":1}`)
	if _, err := ParseDeploymentResult(unknown); err == nil {
		t.Fatal("ParseDeploymentResult() accepted an unknown error code")
	}
}

func TestSealedDeploymentSecretRoundTripAndKeyIdentity(t *testing.T) {
	key := bytes.Repeat([]byte{5}, MasterKeySize)
	sealed, err := SealDeploymentSecret(LegacyDeploymentMasterKeyID, key, []byte("client-secret-value"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseSealedDeploymentSecret(sealed.Bytes(), LegacyDeploymentMasterKeyID)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := parsed.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "client-secret-value" {
		t.Fatalf("Open() = %q", plaintext)
	}
	if _, err := ParseSealedDeploymentSecret(sealed.Bytes(), "other-key"); err == nil {
		t.Fatal("ParseSealedDeploymentSecret() accepted a foreign key identity")
	}
	if _, err := parsed.Open(bytes.Repeat([]byte{6}, MasterKeySize)); err == nil {
		t.Fatal("Open() succeeded with the wrong key")
	}
	if _, err := SealDeploymentSecret(LegacyDeploymentMasterKeyID, key, nil); err == nil {
		t.Fatal("SealDeploymentSecret() accepted empty plaintext")
	}
	if _, err := SealDeploymentSecret(LegacyDeploymentMasterKeyID, key, make([]byte, DeploymentMaxSecretPlaintextBytes+1)); err == nil {
		t.Fatal("SealDeploymentSecret() accepted oversized plaintext")
	}

	var buffer bytes.Buffer
	if err := WriteDeploymentFrame(&buffer, sealed.Bytes()); err != nil {
		t.Fatal(err)
	}
	frame, err := ReadDeploymentFrame(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frame, sealed.Bytes()) {
		t.Fatal("sealed envelope did not survive the frame round trip")
	}
}
