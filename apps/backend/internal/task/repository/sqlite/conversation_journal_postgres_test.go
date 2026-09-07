package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresConversationJournalRoundTrip exercises the PostgreSQL journal
// triggers end to end: message insert/update sanitization through
// conversation_visible_content, the meta schema-version marker, and the
// version-guarded one-time migration. Skips unless KANDEV_TEST_POSTGRES_DSN is
// set (CI runs it in postgres-boot).
func TestPostgresConversationJournalRoundTrip(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, '', 'test task', ?, ?)
	`), "task-pg-journal", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, started_at, updated_at)
		VALUES (?, ?, ?, ?)
	`), "session-pg-journal", "task-pg-journal", now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), "turn-pg-journal", "session-pg-journal", "task-pg-journal", now, now, now); err != nil {
		t.Fatalf("seed turn: %v", err)
	}

	content := "pre <kandev-system>hidden one\nline two</kandev-system> mid <kandev-system>two</kandev-system> post"
	message := &models.Message{
		ID: "message-pg-journal", TaskSessionID: "session-pg-journal", TaskID: "task-pg-journal",
		TurnID: "turn-pg-journal", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: content,
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	want := sysprompt.StripSystemContent(content)

	var eventPayload string
	if err := repo.db.Get(&eventPayload, `SELECT payload FROM conversation_session_events WHERE session_id = 'session-pg-journal' AND event_type = 'message.added'`); err != nil {
		t.Fatalf("read event payload: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(eventPayload), &payload); err != nil {
		t.Fatalf("decode event payload: %v", err)
	}
	got, ok := payload["content"].(string)
	if !ok || got != want || strings.Contains(got, "hidden") {
		t.Fatalf("postgres journal content = %q, want %q", got, want)
	}

	// The schema marker must be recorded so the one-time migration is not
	// re-run (and the full-corpus rewrite never repeats on later boots).
	version, err := repo.readConversationJournalMetaInt("schema.version")
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version < journalSchemaVersion {
		t.Fatalf("schema version = %d, want >= %d", version, journalSchemaVersion)
	}
	backfilled, err := repo.readConversationJournalMetaFlag(journalBackfillKey)
	if err != nil {
		t.Fatalf("read backfill flag: %v", err)
	}
	if !backfilled {
		t.Fatal("backfill flag not recorded after init")
	}
}
