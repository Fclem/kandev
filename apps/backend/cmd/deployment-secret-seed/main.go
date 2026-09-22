// Command deployment-secret-seed is test-support tooling that prepares a data
// directory for the dedicated personal-star deployment flow without a
// root-owned supervisor round trip.
//
// It exists so E2E runs can exercise the deployment-owned secret boundary —
// bootstrap the deployment identity, create the master key the backend itself
// would create, and provision the `github-personal-star` client-secret
// reference into `deployment-secrets.sqlite` — using exactly the production
// packages in internal/secrets. Production binaries never link it: it is a
// separate `cmd/` program that no runtime target builds or imports, and
// `make -C apps/backend build-runtime` names its output binaries explicitly.
//
// Usage:
//
//	deployment-secret-seed --data-dir <dir> --client-secret <value>
//	deployment-secret-seed --data-dir <dir> --client-secret-file <path> [--version 1]
//
// The command is idempotent: an existing identity, master key, and matching
// reference version are reused, and a re-run with the already-sealed value
// succeeds without writing. A reference that exists with a different value is
// an error rather than a silent overwrite, because deployment secret rows are
// immutable.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kandev/kandev/internal/secrets"
)

const maxClientSecretBytes = secrets.DeploymentMaxSecretPlaintextBytes

type options struct {
	dataDir          string
	clientSecret     string
	clientSecretFile string
	version          int64
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "deployment-secret-seed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	options, err := parseOptions(args)
	if err != nil {
		return err
	}
	plaintext, err := readPlaintext(options)
	if err != nil {
		return err
	}
	return seed(context.Background(), options, plaintext, out)
}

func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("deployment-secret-seed", flag.ContinueOnError)
	var options options
	flags.StringVar(&options.dataDir, "data-dir", "", "deployment data directory (the backend's resolved data dir)")
	flags.StringVar(&options.clientSecret, "client-secret", "", "personal-star OAuth App client secret (prefer --client-secret-file)")
	flags.StringVar(&options.clientSecretFile, "client-secret-file", "", "file holding the client secret; one trailing line break is dropped")
	flags.Int64Var(&options.version, "version", 1, "reference version to provision")
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if strings.TrimSpace(options.dataDir) == "" {
		return options, errors.New("--data-dir is required")
	}
	if (options.clientSecret == "") == (options.clientSecretFile == "") {
		return options, errors.New("provide exactly one of --client-secret or --client-secret-file")
	}
	if options.version < 1 {
		return options, errors.New("--version must be positive")
	}
	return options, nil
}

// readPlaintext mirrors the operator CLI: one trailing line break is dropped so
// a secret written by an editor or `echo` is not sealed with a stray newline,
// and every other byte is sealed verbatim.
func readPlaintext(options options) ([]byte, error) {
	if options.clientSecretFile == "" {
		return []byte(options.clientSecret), nil
	}
	file, err := os.Open(options.clientSecretFile)
	if err != nil {
		return nil, fmt.Errorf("open the client secret file: %w", err)
	}
	defer func() { _ = file.Close() }()
	// Read one byte past the limit plus the optional trailing newline so an
	// oversized secret is rejected instead of silently truncated.
	data, err := io.ReadAll(io.LimitReader(file, int64(maxClientSecretBytes)+2))
	if err != nil {
		return nil, fmt.Errorf("read the client secret: %w", err)
	}
	return bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r")), nil
}

func seed(ctx context.Context, options options, plaintext []byte, out io.Writer) error {
	if len(plaintext) == 0 || len(plaintext) > maxClientSecretBytes {
		return errors.New("the client secret is empty or too long")
	}

	// The backend creates master.key on ordinary startup; create-or-load it the
	// same way so a fresh data directory is provisionable before the first run.
	if _, err := secrets.NewMasterKeyProvider(options.dataDir); err != nil {
		return err
	}
	identity, err := secrets.BootstrapDeploymentIdentity(options.dataDir)
	if err != nil {
		return fmt.Errorf("bootstrap the deployment identity: %w", err)
	}
	keyring, err := secrets.DeploymentMasterKeyringOpenExisting(options.dataDir)
	if err != nil {
		return err
	}
	store, err := secrets.OpenDeploymentSecretStoreWriter(options.dataDir, identity, keyring)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	reference := secrets.DeploymentSecretReference(options.version)
	_, err = store.Load(ctx, reference)
	switch {
	case err == nil:
		revealed, revealErr := store.RevealDeploymentSecret(ctx, reference)
		if revealErr != nil {
			return fmt.Errorf("resolve the existing %s: %w", reference, revealErr)
		}
		if revealed != string(plaintext) {
			return fmt.Errorf("%s already holds a different secret; deployment secret rows are immutable, revoke it first", reference)
		}
		_, _ = fmt.Fprintf(out, "deployment secret already provisioned: reference=%s\n", reference)
		return nil
	case !errors.Is(err, secrets.ErrDeploymentSecretNotFound):
		return fmt.Errorf("load %s: %w", reference, err)
	}

	sealed, err := secrets.SealDeploymentSecret(keyring.ID(), keyring.Key(), plaintext)
	if err != nil {
		return err
	}
	provisioner := secrets.NewDeploymentSecretProvisioner(store)
	if err := provisioner.Provision(ctx, secrets.DeploymentProvisionRequest{
		InstallationID:   identity.InstallationID,
		SecretClass:      secrets.DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: options.version,
		SealedEnvelope:   sealed.Bytes(),
	}); err != nil {
		if errors.Is(err, secrets.ErrDeploymentSecretExists) {
			_, _ = fmt.Fprintf(out, "deployment secret already provisioned: reference=%s\n", reference)
			return nil
		}
		return fmt.Errorf("provision %s: %w", reference, err)
	}
	_, _ = fmt.Fprintf(out, "deployment secret provisioned: reference=%s installation_id=%s\n",
		reference, identity.InstallationID)
	return nil
}
