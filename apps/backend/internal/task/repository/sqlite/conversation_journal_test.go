package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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
