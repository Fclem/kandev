package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

func TestConversationJournalVersionsMutationsAndTerminalDeletion(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal", "session-journal", "turn-journal")
	message := &models.Message{
		ID: "message-journal", TaskSessionID: "session-journal", TaskID: "task-journal",
		TurnID: "turn-journal", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeMessage, Content: "<kandev-system>hidden</kandev-system>first",
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	message.Content = "<kandev-system>hidden-update</kandev-system>updated"
	if err := repo.UpdateMessage(ctx, message); err != nil {
		t.Fatalf("update message: %v", err)
	}
	if err := repo.DeleteMessage(ctx, message.ID); err != nil {
		t.Fatalf("delete message: %v", err)
	}
	if err := repo.DeleteTaskSession(ctx, "session-journal"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	var terminal bool
	var watermark int64
	if err := repo.db.QueryRow(`SELECT watermark, terminal FROM conversation_session_streams WHERE session_id = 'session-journal'`).Scan(&watermark, &terminal); err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if !terminal || watermark != 5 {
		t.Fatalf("stream = watermark %d terminal %v, want 5 true", watermark, terminal)
	}
	rows, err := repo.db.Query(`SELECT event_type FROM conversation_session_events WHERE session_id = 'session-journal' ORDER BY sequence`)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var eventTypes []string
	for rows.Next() {
		var eventType string
		if err := rows.Scan(&eventType); err != nil {
			t.Fatal(err)
		}
		eventTypes = append(eventTypes, eventType)
	}
	want := []string{"session.turn.started", "message.added", "message.updated", "message.deleted", "session.removed"}
	if len(eventTypes) != len(want) {
		t.Fatalf("event types = %v, want %v", eventTypes, want)
	}
	for index := range want {
		if eventTypes[index] != want[index] {
			t.Fatalf("event types = %v, want %v", eventTypes, want)
		}
	}
	var payloadBytes []byte
	if err := repo.db.Get(&payloadBytes, `SELECT payload FROM conversation_session_events WHERE session_id = 'session-journal' AND event_type = 'message.added'`); err != nil {
		t.Fatalf("read message event payload: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("decode message event payload: %v", err)
	}
	if payload["type"] != "message.added" || payload["session_id"] != "session-journal" {
		t.Fatalf("message event identity = %#v", payload)
	}
	if content, ok := payload["content"].(string); !ok || content != "first" || strings.Contains(content, "hidden") {
		t.Fatalf("journal leaked system content: %#v", payload["content"])
	}
	createdAt, ok := payload["created_at"].(string)
	if !ok {
		t.Fatalf("message created_at = %#v", payload["created_at"])
	}
	if _, err := time.Parse(time.RFC3339Nano, createdAt); err != nil {
		t.Fatalf("message created_at is not RFC3339: %q: %v", createdAt, err)
	}
	var tombstone bool
	if err := repo.db.QueryRow(`SELECT tombstone FROM conversation_message_versions WHERE session_id = 'session-journal' AND message_id = 'message-journal' ORDER BY row_sequence DESC LIMIT 1`).Scan(&tombstone); err != nil {
		t.Fatalf("read message tombstone: %v", err)
	}
	if !tombstone {
		t.Fatal("latest message version is not a tombstone")
	}
}

func TestConversationJournalRollsBackWithSourceMutation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal-rollback", "session-journal-rollback", "turn-journal-rollback")
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, content, type, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'agent', 'rolled back', 'message', '{}', ?, ?)
	`), "message-rollback", "session-journal-rollback", "task-journal-rollback", "turn-journal-rollback", now, now)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("insert source row: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM conversation_session_events WHERE session_id = 'session-journal-rollback' AND event_type = 'message.added'`); err != nil {
		t.Fatalf("count rolled-back event: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back mutation left %d event rows", count)
	}
}

func TestConversationJournalStripsMultiLineAndMultiBlockSystemContent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal-strip", "session-journal-strip", "turn-journal-strip")
	content := "pre <kandev-system>hidden one\nline two</kandev-system> mid\n<kandev-system>hidden2</kandev-system> post"
	message := &models.Message{
		ID: "message-strip", TaskSessionID: "session-journal-strip", TaskID: "task-journal-strip",
		TurnID: "turn-journal-strip", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: content,
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	want := sysprompt.StripSystemContent(content)

	var versionContent string
	if err := repo.db.Get(&versionContent, `SELECT json_extract(payload, '$.content') FROM conversation_message_versions WHERE message_id = 'message-strip'`); err != nil {
		t.Fatalf("read message version content: %v", err)
	}
	if versionContent != want {
		t.Fatalf("version content = %q, want %q", versionContent, want)
	}
	var eventContent string
	if err := repo.db.Get(&eventContent, `SELECT json_extract(payload, '$.content') FROM conversation_session_events WHERE session_id = 'session-journal-strip' AND event_type = 'message.added'`); err != nil {
		t.Fatalf("read message event content: %v", err)
	}
	if eventContent != want {
		t.Fatalf("event content = %q, want %q", eventContent, want)
	}

	// The update trigger strips the same way.
	message.Content = "<kandev-system>a</kandev-system>\nupdated <kandev-system>b</kandev-system><kandev-system>c</kandev-system> end"
	if err := repo.UpdateMessage(ctx, message); err != nil {
		t.Fatalf("update message: %v", err)
	}
	wantUpdated := sysprompt.StripSystemContent(message.Content)
	if err := repo.db.Get(&versionContent, `SELECT json_extract(payload, '$.content') FROM conversation_message_versions WHERE message_id = 'message-strip' ORDER BY row_sequence DESC LIMIT 1`); err != nil {
		t.Fatalf("read updated message version content: %v", err)
	}
	if versionContent != wantUpdated {
		t.Fatalf("updated version content = %q, want %q", versionContent, wantUpdated)
	}
	if err := repo.db.Get(&eventContent, `SELECT json_extract(payload, '$.content') FROM conversation_session_events WHERE session_id = 'session-journal-strip' AND event_type = 'message.updated'`); err != nil {
		t.Fatalf("read updated message event content: %v", err)
	}
	if eventContent != wantUpdated {
		t.Fatalf("updated event content = %q, want %q", eventContent, wantUpdated)
	}
}

func TestConversationJournalSanitizeMigrationRewritesLegacyRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal-migrate", "session-journal-migrate", "turn-journal-migrate")
	legacy := "<kandev-system>old\nsecret</kandev-system>kept <kandev-system>second</kandev-system> text"
	message := &models.Message{
		ID: "message-legacy", TaskSessionID: "session-journal-migrate", TaskID: "task-journal-migrate",
		TurnID: "turn-journal-migrate", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: "placeholder",
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	// Simulate rows written before the sanitize migration existed.
	if _, err := repo.db.Exec(`
		UPDATE conversation_message_versions SET payload = json_set(payload, '$.content', ?) WHERE message_id = 'message-legacy';
	`, legacy); err != nil {
		t.Fatalf("seed legacy version payload: %v", err)
	}
	if _, err := repo.db.Exec(`
		UPDATE conversation_session_events SET payload = json_set(payload, '$.content', ?) WHERE session_id = 'session-journal-migrate' AND event_type = 'message.added';
	`, legacy); err != nil {
		t.Fatalf("seed legacy event payload: %v", err)
	}

	if err := repo.initSQLiteConversationJournalTriggers(); err != nil {
		t.Fatalf("re-init triggers (runs sanitize migration): %v", err)
	}
	want := sysprompt.StripSystemContent(legacy)
	var versionContent string
	if err := repo.db.Get(&versionContent, `SELECT json_extract(payload, '$.content') FROM conversation_message_versions WHERE message_id = 'message-legacy'`); err != nil {
		t.Fatalf("read migrated version content: %v", err)
	}
	if versionContent != want {
		t.Fatalf("migrated version content = %q, want %q", versionContent, want)
	}
	var eventContent string
	if err := repo.db.Get(&eventContent, `SELECT json_extract(payload, '$.content') FROM conversation_session_events WHERE session_id = 'session-journal-migrate' AND event_type = 'message.added'`); err != nil {
		t.Fatalf("read migrated event content: %v", err)
	}
	if eventContent != want {
		t.Fatalf("migrated event content = %q, want %q", eventContent, want)
	}
}

func TestConversationJournalTurnDeleteRemovesJournalHistoryOnly(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-journal-turn-del", "session-journal-turn-del", "turn-journal-turn-del")

	var versionsBefore int
	if err := repo.db.Get(&versionsBefore, `SELECT COUNT(*) FROM conversation_turn_versions WHERE turn_id = 'turn-journal-turn-del'`); err != nil {
		t.Fatalf("count turn versions: %v", err)
	}
	requireTurnVersionCount := func(want int) {
		var count int
		if err := repo.db.Get(&count, `SELECT COUNT(*) FROM conversation_turn_versions WHERE turn_id = 'turn-journal-turn-del'`); err != nil {
			t.Fatalf("count turn versions: %v", err)
		}
		if count != want {
			t.Fatalf("turn version count = %d, want %d", count, want)
		}
	}
	if versionsBefore != 1 {
		t.Fatalf("expected one started version from the seed turn, got %d", versionsBefore)
	}
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_session_turns SET completed_at = ?, updated_at = ? WHERE id = ?`), now, now, "turn-journal-turn-del"); err != nil {
		t.Fatalf("complete turn: %v", err)
	}
	requireTurnVersionCount(2)

	var eventsBefore int
	if err := repo.db.Get(&eventsBefore, `SELECT COUNT(*) FROM conversation_session_events WHERE session_id = 'session-journal-turn-del'`); err != nil {
		t.Fatalf("count events before delete: %v", err)
	}
	var watermarkBefore int64
	if err := repo.db.QueryRow(`SELECT watermark FROM conversation_session_streams WHERE session_id = 'session-journal-turn-del'`).Scan(&watermarkBefore); err != nil {
		t.Fatalf("read stream watermark: %v", err)
	}

	// Deleting an empty turn must drop its journal history...
	if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM task_session_turns WHERE id = ?`), "turn-journal-turn-del"); err != nil {
		t.Fatalf("delete turn: %v", err)
	}
	requireTurnVersionCount(0)

	// ...without consuming a stream sequence or emitting a new event row, so
	// ordered mirroring and replay never observe a gap.
	var eventsAfter int
	if err := repo.db.Get(&eventsAfter, `SELECT COUNT(*) FROM conversation_session_events WHERE session_id = 'session-journal-turn-del'`); err != nil {
		t.Fatalf("count events after delete: %v", err)
	}
	if eventsAfter != eventsBefore {
		t.Fatalf("turn delete changed event count from %d to %d", eventsBefore, eventsAfter)
	}
	var watermarkAfter int64
	if err := repo.db.QueryRow(`SELECT watermark FROM conversation_session_streams WHERE session_id = 'session-journal-turn-del'`).Scan(&watermarkAfter); err != nil {
		t.Fatalf("read stream watermark after delete: %v", err)
	}
	if watermarkAfter != watermarkBefore {
		t.Fatalf("turn delete advanced stream watermark from %d to %d", watermarkBefore, watermarkAfter)
	}
}

func TestConversationJournalBackfillsExistingRowsIdempotently(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-backfill", "session-backfill", "turn-backfill")
	message := &models.Message{
		ID: "message-backfill", TaskSessionID: "session-backfill", TaskID: "task-backfill",
		TurnID: "turn-backfill", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: "<kandev-system>hidden</kandev-system>existing",
	}
	if err := repo.CreateMessage(context.Background(), message); err != nil {
		t.Fatalf("create source message: %v", err)
	}
	if _, err := repo.db.Exec(`
		DELETE FROM conversation_session_events;
		DELETE FROM conversation_message_versions;
		DELETE FROM conversation_turn_versions;
		DELETE FROM conversation_session_streams;
	`); err != nil {
		t.Fatalf("clear journal: %v", err)
	}

	if err := repo.backfillConversationJournal(); err != nil {
		t.Fatalf("backfill conversation journal: %v", err)
	}
	if err := repo.backfillConversationJournal(); err != nil {
		t.Fatalf("repeat conversation journal backfill: %v", err)
	}
	var eventCount int
	if err := repo.db.Get(&eventCount, `SELECT COUNT(*) FROM conversation_session_events WHERE session_id = 'session-backfill'`); err != nil {
		t.Fatalf("count backfill events: %v", err)
	}
	if eventCount != 2 {
		t.Fatalf("backfill event count = %d, want 2", eventCount)
	}
	var content string
	if err := repo.db.Get(&content, `SELECT json_extract(payload, '$.content') FROM conversation_message_versions WHERE message_id = 'message-backfill'`); err != nil {
		t.Fatalf("read backfill message content: %v", err)
	}
	if content != "existing" {
		t.Fatalf("backfill content = %q, want existing", content)
	}
}
