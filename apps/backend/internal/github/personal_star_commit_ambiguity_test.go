package github

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
)

var starCommitAckDriverID atomic.Uint64

type starCommitAckFault struct {
	target            string
	armed             bool
	fired             bool
	rollbackBeforeAck bool
	afterCommit       func()
}

func (f *starCommitAckFault) arm(target string, afterCommit func()) {
	f.target = strings.ToLower(target)
	f.armed = true
	f.fired = false
	f.rollbackBeforeAck = false
	f.afterCommit = afterCommit
}

func (f *starCommitAckFault) armRolledBack(target string) {
	f.arm(target, nil)
	f.rollbackBeforeAck = true
}

type starCommitAckDriver struct {
	fault *starCommitAckFault
}

func (d *starCommitAckDriver) Open(name string) (driver.Conn, error) {
	conn, err := (&sqlite3.SQLiteDriver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return &starCommitAckConn{Conn: conn, fault: d.fault}, nil
}

type starCommitAckConn struct {
	driver.Conn
	fault   *starCommitAckFault
	current *starCommitAckTx
}

func (c *starCommitAckConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.Conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &starCommitAckStmt{Stmt: stmt, conn: c, query: strings.ToLower(query)}, nil
}

func (c *starCommitAckConn) ExecContext(
	ctx context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Result, error) {
	execer, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	result, err := execer.ExecContext(ctx, query, args)
	if err == nil {
		c.recordQuery(query)
	}
	return result, err
}

func (c *starCommitAckConn) QueryContext(
	ctx context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Rows, error) {
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return queryer.QueryContext(ctx, query, args)
}

func (c *starCommitAckConn) recordQuery(query string) {
	if c.current != nil && c.fault.armed && strings.Contains(strings.ToLower(query), c.fault.target) {
		c.current.targetExecuted = true
	}
}

func (c *starCommitAckConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *starCommitAckConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	beginner, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, errors.New("SQLite connection does not support BeginTx")
	}
	tx, err := beginner.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return c.wrapTx(tx), nil
}

func (c *starCommitAckConn) wrapTx(tx driver.Tx) driver.Tx {
	wrapped := &starCommitAckTx{Tx: tx, conn: c}
	c.current = wrapped
	return wrapped
}

type starCommitAckStmt struct {
	driver.Stmt
	conn  *starCommitAckConn
	query string
}

func (s *starCommitAckStmt) ExecContext(
	ctx context.Context,
	args []driver.NamedValue,
) (driver.Result, error) {
	execer, ok := s.Stmt.(driver.StmtExecContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	result, err := execer.ExecContext(ctx, args)
	if err == nil {
		s.conn.recordQuery(s.query)
	}
	return result, err
}

type starCommitAckTx struct {
	driver.Tx
	conn           *starCommitAckConn
	targetExecuted bool
}

func (tx *starCommitAckTx) Commit() error {
	fault := tx.conn.fault
	if tx.targetExecuted && fault.armed && fault.rollbackBeforeAck {
		fault.armed = false
		fault.fired = true
		fault.rollbackBeforeAck = false
		err := tx.Tx.Rollback()
		tx.conn.current = nil
		if err != nil {
			return errors.Join(errors.New("simulated rollback before commit acknowledgment"), err)
		}
		return errors.New("simulated commit acknowledgment lost after rollback")
	}
	if err := tx.Tx.Commit(); err != nil {
		tx.conn.current = nil
		return err
	}
	tx.conn.current = nil
	if tx.targetExecuted && fault.armed {
		fault.armed = false
		fault.fired = true
		afterCommit := fault.afterCommit
		fault.afterCommit = nil
		if afterCommit != nil {
			afterCommit()
		}
		return errors.New("simulated lost commit acknowledgment")
	}
	return nil
}

func (tx *starCommitAckTx) Rollback() error {
	err := tx.Tx.Rollback()
	tx.conn.current = nil
	return err
}

func newPersonalStarCommitAckStore(t *testing.T) (*Store, *starCommitAckFault) {
	t.Helper()
	fault := &starCommitAckFault{}
	driverName := fmt.Sprintf("personal-star-commit-ack-%d", starCommitAckDriverID.Add(1))
	sql.Register(driverName, &starCommitAckDriver{fault: fault})
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_mode=rwc&_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL&_cache=shared",
		filepath.ToSlash(filepath.Join(t.TempDir(), "github.db")))
	rawDB, err := sql.Open(driverName, dsn)
	if err != nil {
		t.Fatalf("open fault-injection database: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, archived_at DATETIME)`,
		`CREATE TABLE workspaces (id TEXT PRIMARY KEY)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create GitHub store prerequisite: %v", err)
		}
	}
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("create GitHub store: %v", err)
	}
	return store, fault
}

func TestPersonalStarInsertCommitAckLossReconcilesCommittedGrant(t *testing.T) {
	ctx := context.Background()
	store, fault := newPersonalStarCommitAckStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{
			AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{PersonalStarScope},
		},
		user: GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	state := startPersonalStarConsent(t, service, "/settings/plugins")
	lifecycle, err := store.GetPersonalStarLifecycle(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	accessID := personalStarAccessSecretID("ws-1", "user-1", lifecycle.CredentialGeneration)
	refreshID := personalStarRefreshSecretID("ws-1", "user-1", lifecycle.CredentialGeneration)
	fault.arm("INSERT INTO github_personal_star_connection", nil)

	outcome, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1")
	if err != nil {
		t.Fatalf("callback after committed insert returned error: %v", err)
	}
	if outcome.Outcome != personalStarOutcomeConnected || !outcome.Active || !outcome.Capable {
		t.Fatalf("callback outcome = %+v, want connected", outcome)
	}
	if !fault.fired {
		t.Fatal("commit acknowledgment fault was not injected")
	}
	connection, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || connection == nil || connection.AccessSecretID != accessID || connection.RefreshSecretID != refreshID {
		t.Fatalf("committed connection = %+v, %v", connection, err)
	}
	if secrets.values[accessID] != "access-1" || secrets.values[refreshID] != "refresh-1" {
		t.Fatalf("committed token secrets were removed: %v", secrets.values)
	}
	if len(secrets.deleted) != 0 {
		t.Fatalf("committed token secrets were compensated: %v", secrets.deleted)
	}
}

func TestPersonalStarRefreshCommitAckLossReconcilesReplacement(t *testing.T) {
	ctx := context.Background()
	store, fault := newPersonalStarCommitAckStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{
			AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{PersonalStarScope},
		},
		user: GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	state := startPersonalStarConsent(t, service, "/settings/plugins")
	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1"); err != nil {
		t.Fatal(err)
	}
	before, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || before == nil {
		t.Fatalf("initial connection = %+v, %v", before, err)
	}
	client.refreshed = GitHubOAuthTokens{
		AccessToken: "access-2", RefreshToken: "refresh-2", GrantedScopes: []string{PersonalStarScope},
	}
	fault.arm("UPDATE github_personal_star_connection SET", nil)

	if err := service.RefreshPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatalf("refresh after committed replacement returned error: %v", err)
	}
	if !fault.fired {
		t.Fatal("commit acknowledgment fault was not injected")
	}
	after, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || after == nil || after.CredentialGeneration <= before.CredentialGeneration {
		t.Fatalf("refreshed connection = %+v, %v", after, err)
	}
	if secrets.values[after.AccessSecretID] != "access-2" || secrets.values[after.RefreshSecretID] != "refresh-2" {
		t.Fatalf("committed replacement token secrets were removed: %v", secrets.values)
	}
	if _, ok := secrets.values[before.AccessSecretID]; !ok {
		t.Fatal("superseded access secret was deleted before cleanup processing")
	}
}

func TestPersonalStarAmbiguousInsertRollbackQueuesUnreferencedTokens(t *testing.T) {
	ctx := context.Background()
	store, fault := newPersonalStarCommitAckStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{
			AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{PersonalStarScope},
		},
		user: GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	state := startPersonalStarConsent(t, service, "/settings/plugins")
	lifecycle, err := store.GetPersonalStarLifecycle(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	fault.armRolledBack("INSERT INTO github_personal_star_connection")

	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1"); err == nil {
		t.Fatal("callback succeeded after the connection insert rolled back")
	}
	if !fault.fired {
		t.Fatal("commit failure fault was not injected")
	}
	connection, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || connection != nil {
		t.Fatalf("connection after rolled-back commit = %+v, %v", connection, err)
	}
	accessID := personalStarAccessSecretID("ws-1", "user-1", lifecycle.CredentialGeneration)
	refreshID := personalStarRefreshSecretID("ws-1", "user-1", lifecycle.CredentialGeneration)
	if secrets.values[accessID] != "access-1" || secrets.values[refreshID] != "refresh-1" {
		t.Fatalf("staged secrets were deleted before durable cleanup: %v", secrets.values)
	}
	if len(secrets.deleted) != 0 {
		t.Fatalf("staged secrets were directly deleted: %v", secrets.deleted)
	}
	records, err := store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("staged cleanup records = %+v, want both token kinds", records)
	}
	queued := map[string]*PersonalStarSecretCleanup{}
	for _, record := range records {
		queued[record.SecretID] = record
	}
	for secretID, kind := range map[string]string{
		accessID:  personalStarSecretKindAccess,
		refreshID: personalStarSecretKindRefresh,
	} {
		record, ok := queued[secretID]
		if !ok || record.UserID != "user-1" || record.SecretKind != kind ||
			record.CredentialGeneration != lifecycle.CredentialGeneration ||
			record.Status != personalStarCleanupPending {
			t.Fatalf("queued cleanup for %q = %+v", secretID, record)
		}
	}
}

func TestPersonalStarCommitAckLossRetainsSecretsWhenReconciliationFails(t *testing.T) {
	store, fault := newPersonalStarCommitAckStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{
			AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{PersonalStarScope},
		},
		user: GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	state := startPersonalStarConsent(t, service, "/settings/plugins")
	lifecycle, err := store.GetPersonalStarLifecycle(context.Background(), "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	accessID := personalStarAccessSecretID("ws-1", "user-1", lifecycle.CredentialGeneration)
	refreshID := personalStarRefreshSecretID("ws-1", "user-1", lifecycle.CredentialGeneration)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fault.arm("INSERT INTO github_personal_star_connection", cancel)

	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1"); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("callback error = %v, want commit and reconciliation errors", err)
	}
	if !fault.fired {
		t.Fatal("commit acknowledgment fault was not injected")
	}
	if secrets.values[accessID] != "access-1" || secrets.values[refreshID] != "refresh-1" {
		t.Fatalf("secrets were deleted after reconciliation failed: %v", secrets.values)
	}
	connection, err := store.GetPersonalStarConnection(context.Background(), "ws-1", "user-1")
	if err != nil || connection == nil || connection.AccessSecretID != accessID || connection.RefreshSecretID != refreshID {
		t.Fatalf("committed connection = %+v, %v", connection, err)
	}
}
