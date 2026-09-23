package cli

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/launcher/deploymentsecretd"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/system/deploymentsecret"
)

const testClientSecret = "personal-star-client-secret"

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

type cliHarness struct {
	dataDir     string
	socketPath  string
	masterKey   []byte
	identity    secrets.DeploymentIdentity
	provisioner *recordingProvisioner
}

// shortSocketPath returns a socket path under the ~107 byte sockaddr_un limit.
// t.TempDir() names embed the test name, which is long enough to overflow it.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "kandev-ds")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "deployment-secret.sock")
}

// newCLIHarness starts the real root-owned supervisor in-process with a root
// peer check, so the CLI is exercised over a genuine Unix socket.
func newCLIHarness(t *testing.T, provisioner *recordingProvisioner) cliHarness {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the deployment secret supervisor is Linux-only")
	}
	dataDir := t.TempDir()
	masterKey := bytes.Repeat([]byte{7}, secrets.MasterKeySize)
	if err := os.WriteFile(filepath.Join(dataDir, secrets.MasterKeyFile), masterKey, 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := secrets.BootstrapDeploymentIdentity(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	socketPath := shortSocketPath(t)
	server, err := deploymentsecret.NewServer(deploymentsecret.Config{
		SocketPath:  socketPath,
		Target:      deploymentsecret.Target{GOOS: "linux", BackendUID: 1000},
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
	return cliHarness{
		dataDir:     dataDir,
		socketPath:  socketPath,
		masterKey:   masterKey,
		identity:    identity,
		provisioner: provisioner,
	}
}

func (h cliHarness) run(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	return runAsRoot(t, DeploymentSecretDefaults{DataDir: h.dataDir, Socket: h.socketPath}, stdin, args...)
}

func runAsRoot(t *testing.T, defaults DeploymentSecretDefaults, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	runner := deploymentSecretRunner{
		out:      &out,
		errOut:   &errOut,
		input:    strings.NewReader(stdin),
		defaults: defaults,
		geteuid:  func() int { return 0 },
	}
	code := runner.run(args)
	return code, out.String(), errOut.String()
}

func writeClientSecretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "client-secret")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (h cliHarness) openSealedEnvelope(t *testing.T, sealedEnvelope []byte) string {
	t.Helper()
	sealed, err := secrets.ParseSealedDeploymentSecret(sealedEnvelope, secrets.LegacyDeploymentMasterKeyID)
	if err != nil {
		t.Fatalf("parse sealed envelope: %v", err)
	}
	plaintext, err := sealed.Open(h.masterKey)
	if err != nil {
		t.Fatalf("open sealed envelope: %v", err)
	}
	return plaintext
}

func TestDeploymentSecretCLIProvisionsSealedSecret(t *testing.T) {
	provisioner := &recordingProvisioner{}
	harness := newCLIHarness(t, provisioner)
	secretFile := writeClientSecretFile(t, testClientSecret+"\n")

	code, out, errOut := harness.run(t, "",
		"github-personal-star", "provision", "--version", "4", "--client-secret-file", secretFile)
	if code != 0 || errOut != "" {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	reference := "kandev-runtime:github-personal-star:client-secret:4"
	if !strings.Contains(out, "reference="+reference) {
		t.Fatalf("stdout does not report the reference:\n%s", out)
	}
	if strings.Contains(out, testClientSecret) || strings.Contains(errOut, testClientSecret) {
		t.Fatal("the CLI echoed the client secret")
	}
	if len(provisioner.provisions) != 1 {
		t.Fatalf("provision calls = %d, want 1", len(provisioner.provisions))
	}
	captured := provisioner.provisions[0].request
	if captured.ReferenceVersion != 4 ||
		captured.SecretClass != secrets.DeploymentSecretClassGitHubPersonalStar ||
		captured.InstallationID != harness.identity.InstallationID {
		t.Fatalf("captured provision = %+v", captured)
	}
	// The trailing newline of the operator's file is dropped, and the envelope
	// is bound to the existing master key rather than a freshly generated one.
	if plaintext := harness.openSealedEnvelope(t, captured.SealedEnvelope); plaintext != testClientSecret {
		t.Fatalf("sealed plaintext = %q, want %q", plaintext, testClientSecret)
	}
}

func TestDeploymentSecretCLIReadsSecretFromStdinAndRevokes(t *testing.T) {
	provisioner := &recordingProvisioner{}
	harness := newCLIHarness(t, provisioner)

	code, _, errOut := harness.run(t, testClientSecret+"\r\n",
		"github-personal-star", "provision", "--version", "1", "--client-secret-file", "-")
	if code != 0 {
		t.Fatalf("provision exit = %d, stderr = %q", code, errOut)
	}
	if len(provisioner.provisions) != 1 {
		t.Fatalf("provision calls = %d, want 1", len(provisioner.provisions))
	}
	if plaintext := harness.openSealedEnvelope(t, provisioner.provisions[0].request.SealedEnvelope); plaintext != testClientSecret {
		t.Fatalf("stdin sealed plaintext = %q, want %q", plaintext, testClientSecret)
	}

	code, out, errOut := harness.run(t, "", "github-personal-star", "revoke", "--version", "1")
	if code != 0 || errOut != "" {
		t.Fatalf("revoke exit = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, "revoked") || !strings.Contains(out, "client-secret:1") {
		t.Fatalf("revoke stdout does not report the revoked reference:\n%s", out)
	}
	if len(provisioner.revokes) != 1 || provisioner.revokes[0].request.ReferenceVersion != 1 {
		t.Fatalf("captured revokes = %+v", provisioner.revokes)
	}
}

func TestDeploymentSecretCLIRejectsOversizedSecret(t *testing.T) {
	provisioner := &recordingProvisioner{}
	harness := newCLIHarness(t, provisioner)
	secretFile := writeClientSecretFile(t, strings.Repeat("x", secrets.DeploymentMaxSecretPlaintextBytes+1))

	code, out, errOut := harness.run(t, "",
		"github-personal-star", "provision", "--version", "1", "--client-secret-file", secretFile)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", code, errOut)
	}
	if !strings.Contains(errOut, "at most 1024 bytes") {
		t.Fatalf("stderr is not actionable: %q", errOut)
	}
	if len(provisioner.provisions) != 0 {
		t.Fatal("an oversized secret reached the daemon")
	}
	if out != "" {
		t.Fatalf("stdout = %q, want empty", out)
	}
}

func TestDeploymentSecretCLIRejectsEmptySecret(t *testing.T) {
	harness := newCLIHarness(t, &recordingProvisioner{})
	secretFile := writeClientSecretFile(t, "\n")

	code, _, errOut := harness.run(t, "",
		"github-personal-star", "provision", "--version", "1", "--client-secret-file", secretFile)
	if code != 1 || !strings.Contains(errOut, "is empty") {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
}

func TestDeploymentSecretCLIMapsClosedResultCodes(t *testing.T) {
	cases := []struct {
		name         string
		provisionErr error
		revokeErr    error
		args         []string
		wantMessage  string
	}{
		{
			name:         "already provisioned",
			provisionErr: secrets.ErrDeploymentSecretExists,
			args:         []string{"github-personal-star", "provision", "--version", "2", "--client-secret-file", "-"},
			wantMessage:  "already provisioned",
		},
		{
			name:        "unknown reference",
			revokeErr:   secrets.ErrDeploymentSecretNotFound,
			args:        []string{"github-personal-star", "revoke", "--version", "9"},
			wantMessage: "is not provisioned",
		},
		{
			name:         "binding mismatch",
			provisionErr: secrets.ErrDeploymentSecretBinding,
			args:         []string{"github-personal-star", "provision", "--version", "2", "--client-secret-file", "-"},
			wantMessage:  "github-personal-star client secret",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			provisioner := &recordingProvisioner{provisionErr: testCase.provisionErr, revokeErr: testCase.revokeErr}
			harness := newCLIHarness(t, provisioner)
			code, out, errOut := harness.run(t, testClientSecret, testCase.args...)
			if code != 1 {
				t.Fatalf("exit = %d, want 1 (stdout %q, stderr %q)", code, out, errOut)
			}
			if !strings.Contains(errOut, testCase.wantMessage) {
				t.Fatalf("stderr = %q, want it to explain %q", errOut, testCase.wantMessage)
			}
			if strings.Contains(errOut, testClientSecret) {
				t.Fatal("the failure message leaked the client secret")
			}
		})
	}
}

func TestDeploymentSecretCLIRefusesNonRootOperator(t *testing.T) {
	for _, args := range [][]string{
		{"bootstrap"},
		{"github-personal-star", "provision", "--version", "1", "--client-secret-file", "-"},
		{"github-personal-star", "revoke", "--version", "1"},
	} {
		dataDir := filepath.Join(t.TempDir(), "deployment-data")
		var out, errOut bytes.Buffer
		runner := deploymentSecretRunner{
			out:      &out,
			errOut:   &errOut,
			input:    strings.NewReader(testClientSecret),
			defaults: DeploymentSecretDefaults{DataDir: dataDir, Socket: filepath.Join(dataDir, "deployment-secret.sock")},
			geteuid:  func() int { return 1000 },
		}
		if code := runner.run(args); code != 1 {
			t.Fatalf("%v exit = %d, want 1", args, code)
		}
		if !strings.Contains(errOut.String(), "requires root") {
			t.Fatalf("%v stderr = %q, want a root refusal", args, errOut.String())
		}
		if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
			t.Fatalf("%v touched the filesystem before refusing (stat error %v)", args, err)
		}
	}
}

func TestDeploymentSecretCLIBootstrapIsIdempotent(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	defaults := DeploymentSecretDefaults{DataDir: dataDir, Socket: filepath.Join(dataDir, "run", "deployment-secret.sock")}

	code, out, errOut := runAsRoot(t, defaults, "", "bootstrap")
	if code != 0 || errOut != "" {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	installationID, ok := strings.CutPrefix(strings.TrimSpace(out), "deployment installation id: ")
	if !ok || !strings.Contains(out, installationID) {
		t.Fatalf("stdout does not report the installation id:\n%s", out)
	}
	identity, err := secrets.LoadDeploymentIdentity(dataDir)
	if err != nil {
		t.Fatalf("bootstrap did not write a loadable identity: %v", err)
	}
	if identity.InstallationID != installationID {
		t.Fatalf("reported installation id %q, stored %q", installationID, identity.InstallationID)
	}

	code, repeated, errOut := runAsRoot(t, defaults, "", "bootstrap")
	if code != 0 || errOut != "" {
		t.Fatalf("second bootstrap exit = %d, stderr = %q", code, errOut)
	}
	if repeated != out {
		t.Fatalf("second bootstrap changed the identity:\n%s\n%s", out, repeated)
	}
}

func TestDeploymentSecretCLIReportsMissingIdentityAndUnreachableDaemon(t *testing.T) {
	emptyDataDir := filepath.Join(t.TempDir(), "data")
	code, _, errOut := runAsRoot(t,
		// A bindable socket path: the subject here is the missing deployment
		// identity, and a path under the temp data dir exceeds the unix socket
		// limit before the daemon ever gets to it.
		DeploymentSecretDefaults{DataDir: emptyDataDir, Socket: shortSocketPath(t)},
		testClientSecret,
		"github-personal-star", "provision", "--version", "1", "--client-secret-file", "-")
	if code != 1 || !strings.Contains(errOut, "bootstrap") {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}

	harness := newCLIHarness(t, &recordingProvisioner{})
	missingSocket := filepath.Join(filepath.Dir(shortSocketPath(t)), "absent.sock")
	code, _, errOut = runAsRoot(t,
		DeploymentSecretDefaults{DataDir: harness.dataDir, Socket: missingSocket},
		testClientSecret,
		"github-personal-star", "provision", "--version", "1", "--client-secret-file", "-")
	if code != 1 || !strings.Contains(errOut, missingSocket) || !strings.Contains(errOut, "kandev-deployment-secretd.service") {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if strings.Contains(errOut, testClientSecret) {
		t.Fatal("the transport error leaked the client secret")
	}
}

func TestDeploymentSecretCLIReportsForeignInstallation(t *testing.T) {
	harness := newCLIHarness(t, &recordingProvisioner{})
	foreignDataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(foreignDataDir, secrets.MasterKeyFile), bytes.Repeat([]byte{9}, secrets.MasterKeySize), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := secrets.BootstrapDeploymentIdentity(foreignDataDir); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runAsRoot(t,
		DeploymentSecretDefaults{DataDir: foreignDataDir, Socket: harness.socketPath},
		testClientSecret,
		"github-personal-star", "provision", "--version", "1", "--client-secret-file", "-")
	if code != 1 || !strings.Contains(errOut, "different deployment installation id") {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
}

func TestDeploymentSecretCLIRejectsUsageErrors(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantMessage string
	}{
		{name: "unknown command", args: []string{"star-provision"}, wantMessage: "unknown deployment secret command"},
		{name: "missing star action", args: []string{"github-personal-star"}, wantMessage: "requires provision or revoke"},
		{name: "unknown star action", args: []string{"github-personal-star", "rotate"}, wantMessage: "unknown deployment secret action"},
		{name: "missing secret file", args: []string{"github-personal-star", "provision", "--version", "1"}, wantMessage: "--client-secret-file"},
		{name: "missing version", args: []string{"github-personal-star", "revoke"}, wantMessage: ""},
		{name: "non numeric version", args: []string{"github-personal-star", "revoke", "--version", "latest"}, wantMessage: "positive integer"},
		{name: "zero version", args: []string{"github-personal-star", "revoke", "--version", "0"}, wantMessage: "positive integer"},
		{name: "missing flag value", args: []string{"github-personal-star", "revoke", "--version"}, wantMessage: "requires a value"},
		{name: "flag name as value", args: []string{"github-personal-star", "revoke", "--version", "--socket"}, wantMessage: "requires a value"},
		{name: "bootstrap version", args: []string{"bootstrap", "--version", "2"}, wantMessage: "--version is valid only"},
		{name: "bootstrap socket", args: []string{"bootstrap", "--socket", "/tmp/sock"}, wantMessage: "--socket is valid only"},
		{name: "unknown flag", args: []string{"bootstrap", "--force"}, wantMessage: "unknown flag"},
		{name: "empty data dir", args: []string{"bootstrap", "--data-dir="}, wantMessage: "non-empty value"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			defaults := DeploymentSecretDefaults{DataDir: t.TempDir(), Socket: "/run/kandev/deployment-secret.sock"}
			code, out, errOut := runAsRoot(t, defaults, testClientSecret, testCase.args...)
			if code != 2 {
				t.Fatalf("exit = %d, want 2 (stdout %q, stderr %q)", code, out, errOut)
			}
			if !strings.HasPrefix(errOut, "[kandev] ") {
				t.Fatalf("stderr = %q, want the launcher error prefix", errOut)
			}
			if testCase.wantMessage != "" && !strings.Contains(errOut, testCase.wantMessage) {
				t.Fatalf("stderr = %q, want it to mention %q", errOut, testCase.wantMessage)
			}
			// The revoke case has no required flags, so the version check must
			// still reject the default version 0 rather than reaching the daemon.
			if testCase.name == "missing version" && !strings.Contains(errOut, "--version") {
				t.Fatalf("stderr = %q, want it to name --version", errOut)
			}
		})
	}
}

func TestDeploymentSecretCLIHelpDescribesEveryCommand(t *testing.T) {
	code, out, errOut := runAsRoot(t, DeploymentSecretDefaults{DataDir: t.TempDir()}, "")
	if code != 0 || errOut != "" {
		t.Fatalf("bare group exit = %d, stderr = %q", code, errOut)
	}
	for _, want := range []string{
		"kandev deployment-secret bootstrap",
		"github-personal-star provision",
		"github-personal-star revoke",
		"--client-secret-file",
		"kandev-runtime:github-personal-star:client-secret:<version>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help does not describe %q:\n%s", want, out)
		}
	}
	code, explicit, errOut := runAsRoot(t, DeploymentSecretDefaults{DataDir: t.TempDir()}, "", "--help")
	if code != 0 || errOut != "" || explicit != out {
		t.Fatalf("--help exit = %d, stderr = %q", code, errOut)
	}
}

// TestRunRefusesUnbindableSocketPath pins the client-side half of the socket
// path preflight: the daemon cannot bind a path past the platform limit, so the
// operator CLI must report that before dialing instead of surfacing a raw
// connect error against a socket that can never exist.
func TestRunRefusesUnbindableSocketPath(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), strings.Repeat("repository-star-", 8), "deployment-secret.sock")
	if len(socketPath) <= deploymentsecretd.UnixSocketPathLimit {
		t.Fatalf("probe socket path is only %d bytes; the test needs one above the limit", len(socketPath))
	}
	code, _, errOut := runAsRoot(t, DeploymentSecretDefaults{
		DataDir: t.TempDir(),
		Socket:  socketPath,
	}, testClientSecret, "github-personal-star", "provision", "--version", "1", "--client-secret-file", "-")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	for _, want := range []string{socketPath, "107", "--socket"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("stderr %q does not name %q", errOut, want)
		}
	}
}
