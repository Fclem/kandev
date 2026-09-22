package secrets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

const (
	deploymentSecretReferencePrefix = "kandev-runtime:github-personal-star:client-secret:"

	// DeploymentSecretStoreFileName is the deployment-owned store file name.
	DeploymentSecretStoreFileName = "deployment-secrets.sqlite"

	deploymentSecretRotationActive  = "active"
	deploymentSecretRotationRevoked = "revoked"
)

var (
	// ErrDeploymentSecretNotFound reports canary/github-personal-star:secret:1 absence.
	ErrDeploymentSecretNotFound = errors.New("deployment secret not found")
	// ErrDeploymentSecretRevoked reports a revoked reference.
	ErrDeploymentSecretRevoked = errors.New("deployment secret is revoked")
	// ErrDeploymentSecretExists reports a duplicate reference version.
	ErrDeploymentSecretExists = errors.New("deployment secret version already exists")
	// ErrDeploymentSecretBinding reports an installation/class mismatch.
	ErrDeploymentSecretBinding = errors.New("deployment secret binding mismatch")
)

// ValidateDeploymentSecretReference requires the exact canonical reference
// spelling reserved for deployment-owned secrets.
func ValidateDeploymentSecretReference(reference string) error {
	_, err := DeploymentSecretReferenceVersion(reference)
	return err
}

// DeploymentSecretReference renders the canonical reference for a version.
func DeploymentSecretReference(version int64) string {
	return deploymentSecretReferencePrefix + strconv.FormatInt(version, 10)
}

// DeploymentSecretReferenceVersion parses a canonical reference version.
func DeploymentSecretReferenceVersion(reference string) (int64, error) {
	version, ok := strings.CutPrefix(reference, deploymentSecretReferencePrefix)
	if !ok || version == "" {
		return 0, errors.New("deployment secret reference is outside the protected namespace")
	}
	parsed, err := strconv.ParseInt(version, 10, 64)
	if err != nil || parsed < 1 || strconv.FormatInt(parsed, 10) != version {
		return 0, errors.New("deployment secret reference has an invalid version")
	}
	return parsed, nil
}

// DeploymentSecretRecord is one immutable deployment secret row.
type DeploymentSecretRecord struct {
	Reference      string
	SecretClass    string
	InstallationID string
	Version        int64
	SealedEnvelope string
	RotationState  string
	CreatedAt      time.Time
	RevokedAt      *time.Time
}

// DeploymentSecretStore owns deployment_secret_refs. The writer path is used
// only by the provisioner; the reader path serves backend resolution.
type DeploymentSecretStore struct {
	writer   *sqlx.DB
	reader   *sqlx.DB
	keyring  *DeploymentMasterKeyring
	identity DeploymentIdentity
	now      func() time.Time
}

// OpenDeploymentSecretStoreWriter opens the deployment-owned store for writes.
func OpenDeploymentSecretStoreWriter(
	dataDir string,
	identity DeploymentIdentity,
	keyring *DeploymentMasterKeyring,
) (*DeploymentSecretStore, error) {
	path := DeploymentSecretStorePath(dataDir)
	writer, err := db.OpenSQLite(path)
	if err != nil {
		return nil, fmt.Errorf("open deployment secret store: %w", err)
	}
	store := newDeploymentSecretStore(sqlx.NewDb(writer, "sqlite3"), identity, keyring)
	if err := store.initSchema(); err != nil {
		_ = writer.Close()
		return nil, err
	}
	reader, err := db.OpenSQLiteReader(path)
	if err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("open deployment secret store reader: %w", err)
	}
	store.reader = sqlx.NewDb(reader, "sqlite3")
	return store, nil
}

// OpenDeploymentSecretStoreReader opens the deployment-owned store read-only.
// A missing store is an error: the backend must fail closed, never create one.
func OpenDeploymentSecretStoreReader(
	dataDir string,
	identity DeploymentIdentity,
	keyring *DeploymentMasterKeyring,
) (*DeploymentSecretStore, error) {
	path := DeploymentSecretStorePath(dataDir)
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("open deployment secret store reader: %w", err)
	}
	reader, err := db.OpenSQLiteReader(path)
	if err != nil {
		return nil, fmt.Errorf("open deployment secret store reader: %w", err)
	}
	if err := reader.Ping(); err != nil {
		_ = reader.Close()
		return nil, fmt.Errorf("open deployment secret store reader: %w", err)
	}
	store := newDeploymentSecretStore(nil, identity, keyring)
	store.reader = sqlx.NewDb(reader, "sqlite3")
	return store, nil
}

// DeploymentSecretStorePath returns the deployment-owned store location.
func DeploymentSecretStorePath(dataDir string) string {
	return filepath.Join(dataDir, DeploymentSecretStoreFileName)
}

func newDeploymentSecretStore(
	handle *sqlx.DB,
	identity DeploymentIdentity,
	keyring *DeploymentMasterKeyring,
) *DeploymentSecretStore {
	store := &DeploymentSecretStore{writer: handle, reader: handle, keyring: keyring, identity: identity, now: time.Now}
	if store.reader == nil {
		store.reader = handle
	}
	return store
}

func (s *DeploymentSecretStore) initSchema() error {
	_, err := s.writer.Exec(`
	CREATE TABLE IF NOT EXISTS deployment_secret_refs (
		reference TEXT PRIMARY KEY,
		secret_class TEXT NOT NULL CHECK (secret_class = '` + DeploymentSecretClassGitHubPersonalStar + `'),
		installation_id TEXT NOT NULL CHECK (installation_id <> ''),
		version BIGINT NOT NULL CHECK (version >= 1),
		sealed_envelope TEXT NOT NULL CHECK (sealed_envelope <> ''),
		rotation_state TEXT NOT NULL CHECK (rotation_state IN ('` + deploymentSecretRotationActive + `', '` + deploymentSecretRotationRevoked + `')),
		created_at TEXT NOT NULL,
		revoked_at TEXT
	);
	CREATE UNIQUE INDEX IF NOT EXISTS deployment_secret_refs_version
		ON deployment_secret_refs (secret_class, installation_id, version);
	`)
	if err != nil {
		return fmt.Errorf("init deployment secret schema: %w", err)
	}
	return nil
}

// Close releases the store handles.
func (s *DeploymentSecretStore) Close() error {
	var firstErr error
	for _, handle := range []*sqlx.DB{s.reader, s.writer} {
		if handle == nil {
			continue
		}
		if err := handle.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.reader, s.writer = nil, nil
	return firstErr
}

// Load returns a validated, non-revoked record bound to this deployment.
func (s *DeploymentSecretStore) Load(ctx context.Context, reference string) (*DeploymentSecretRecord, error) {
	row, err := s.loadRow(ctx, reference)
	if err != nil {
		return nil, err
	}
	return s.validateRecord(row)
}

func (s *DeploymentSecretStore) loadRow(ctx context.Context, reference string) (deploymentSecretRow, error) {
	if _, err := DeploymentSecretReferenceVersion(reference); err != nil {
		return deploymentSecretRow{}, err
	}
	if s == nil || s.reader == nil {
		return deploymentSecretRow{}, errors.New("deployment secret store is unavailable")
	}
	var row deploymentSecretRow
	err := s.reader.GetContext(ctx, &row, s.reader.Rebind(`
		SELECT reference, secret_class, installation_id, version, sealed_envelope, rotation_state, created_at, revoked_at
		FROM deployment_secret_refs WHERE reference = ?`), reference)
	if errors.Is(err, sql.ErrNoRows) {
		return deploymentSecretRow{}, fmt.Errorf("%w: %s", ErrDeploymentSecretNotFound, reference)
	}
	if err != nil {
		return deploymentSecretRow{}, fmt.Errorf("load deployment secret: %w", err)
	}
	return row, nil
}

func (s *DeploymentSecretStore) validateRecord(row deploymentSecretRow) (*DeploymentSecretRecord, error) {
	if row.SecretClass != DeploymentSecretClassGitHubPersonalStar {
		return nil, fmt.Errorf("%w: secret class mismatch", ErrDeploymentSecretBinding)
	}
	if row.InstallationID != s.identity.InstallationID {
		return nil, fmt.Errorf("%w: installation mismatch", ErrDeploymentSecretBinding)
	}
	if row.Reference != DeploymentSecretReference(row.Version) {
		return nil, fmt.Errorf("%w: reference is not canonical", ErrDeploymentSecretBinding)
	}
	if row.RotationState == deploymentSecretRotationRevoked || row.RevokedAt.Valid {
		return nil, fmt.Errorf("%w: %s", ErrDeploymentSecretRevoked, row.Reference)
	}
	if row.RotationState != deploymentSecretRotationActive {
		return nil, fmt.Errorf("%w: unknown rotation state", ErrDeploymentSecretBinding)
	}
	return row.toRecord(), nil
}

// Insert creates a new immutable record. Existing references are never updated.
func (s *DeploymentSecretStore) Insert(ctx context.Context, record *DeploymentSecretRecord) error {
	if s == nil || s.writer == nil {
		return errors.New("deployment secret store is write-disabled")
	}
	if record.Reference != DeploymentSecretReference(record.Version) {
		return fmt.Errorf("%w: reference is not canonical", ErrDeploymentSecretBinding)
	}
	if record.SecretClass != DeploymentSecretClassGitHubPersonalStar {
		return fmt.Errorf("%w: secret class mismatch", ErrDeploymentSecretBinding)
	}
	if record.InstallationID != s.identity.InstallationID {
		return fmt.Errorf("%w: installation mismatch", ErrDeploymentSecretBinding)
	}
	createdAt := s.now().UTC()
	result, err := s.writer.ExecContext(ctx, s.writer.Rebind(`
		INSERT INTO deployment_secret_refs
			(reference, secret_class, installation_id, version, sealed_envelope, rotation_state, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (reference) DO NOTHING`),
		record.Reference, record.SecretClass, record.InstallationID, record.Version,
		record.SealedEnvelope, deploymentSecretRotationActive, createdAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert deployment secret: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("insert deployment secret: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: %s", ErrDeploymentSecretExists, record.Reference)
	}
	record.CreatedAt = createdAt
	record.RotationState = deploymentSecretRotationActive
	return nil
}

// MarkRevoked revokes an existing reference. It reports whether this call was
// the transition that revoked it.
func (s *DeploymentSecretStore) MarkRevoked(ctx context.Context, reference string, at time.Time) (bool, error) {
	if s == nil || s.writer == nil {
		return false, errors.New("deployment secret store is write-disabled")
	}
	result, err := s.writer.ExecContext(ctx, s.writer.Rebind(`
		UPDATE deployment_secret_refs
		SET rotation_state = ?, revoked_at = ?
		WHERE reference = ? AND rotation_state = ?`),
		deploymentSecretRotationRevoked, at.UTC().Format(time.RFC3339Nano),
		reference, deploymentSecretRotationActive)
	if err != nil {
		return false, fmt.Errorf("revoke deployment secret: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("revoke deployment secret: %w", err)
	}
	if affected > 0 {
		return true, nil
	}
	if _, err := s.loadRow(ctx, reference); err != nil {
		return false, err
	}
	return false, nil
}

// RevealDeploymentSecret resolves a protected reference to its plaintext.
func (s *DeploymentSecretStore) RevealDeploymentSecret(ctx context.Context, reference string) (string, error) {
	record, err := s.Load(ctx, reference)
	if err != nil {
		return "", err
	}
	if s.keyring == nil {
		return "", errors.New("deployment master keyring is unavailable")
	}
	sealed, err := ParseSealedDeploymentSecret([]byte(record.SealedEnvelope), s.keyring.ID())
	if err != nil {
		return "", err
	}
	return sealed.Open(s.keyring.Key())
}

type deploymentSecretRow struct {
	Reference      string         `db:"reference"`
	SecretClass    string         `db:"secret_class"`
	InstallationID string         `db:"installation_id"`
	Version        int64          `db:"version"`
	SealedEnvelope string         `db:"sealed_envelope"`
	RotationState  string         `db:"rotation_state"`
	CreatedAt      string         `db:"created_at"`
	RevokedAt      sql.NullString `db:"revoked_at"`
}

func (r *deploymentSecretRow) toRecord() *DeploymentSecretRecord {
	record := &DeploymentSecretRecord{
		Reference:      r.Reference,
		SecretClass:    r.SecretClass,
		InstallationID: r.InstallationID,
		Version:        r.Version,
		SealedEnvelope: r.SealedEnvelope,
		RotationState:  r.RotationState,
	}
	if createdAt, err := time.Parse(time.RFC3339Nano, r.CreatedAt); err == nil {
		record.CreatedAt = createdAt
	}
	if r.RevokedAt.Valid {
		if revokedAt, err := time.Parse(time.RFC3339Nano, r.RevokedAt.String); err == nil {
			record.RevokedAt = &revokedAt
		}
	}
	return record
}
