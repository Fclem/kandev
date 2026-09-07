package plugins

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestConversationMessagesAtUsesImmutableCutoff(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_message_versions (
			session_id TEXT NOT NULL, message_id TEXT NOT NULL, row_sequence INTEGER NOT NULL,
			task_id TEXT, author_type TEXT, created_at TIMESTAMP NOT NULL,
			tombstone BOOLEAN NOT NULL, payload TEXT NOT NULL
		);`)
	require.NoError(t, err)

	createdAt := "2026-09-07T12:00:00Z"
	insertJournalMessageVersion(t, database, 1, false, map[string]any{
		"message_id": "message-1", "turn_id": "turn-1", "task_id": "task-1",
		"author_type": "agent", "author_id": "agent-1", "content": "before",
		"requests_input": 0, "message_type": "message", "created_at": createdAt,
		"updated_at": createdAt, "prompt_index": 0, "metadata_json": `{}`,
	})
	insertJournalMessageVersion(t, database, 2, false, map[string]any{
		"message_id": "message-1", "turn_id": "turn-1", "task_id": "task-1",
		"author_type": "agent", "author_id": "agent-1", "content": "after",
		"requests_input": 0, "message_type": "message", "created_at": createdAt,
		"updated_at": "2026-09-07T12:01:00Z", "prompt_index": 0, "metadata_json": `{}`,
	})
	insertJournalMessageVersion(t, database, 3, true, map[string]any{"message_id": "message-1"})

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)

	atInsert, more, err := service.conversationMessagesAt(context.Background(), "session-1", 1, nil, nil, "asc", "", 20)
	require.NoError(t, err)
	require.False(t, more)
	require.Len(t, atInsert, 1)
	require.Equal(t, "before", atInsert[0].Content)
	require.Equal(t, "session-1", atInsert[0].TaskSessionID)

	atUpdate, _, err := service.conversationMessagesAt(context.Background(), "session-1", 2, nil, nil, "asc", "", 20)
	require.NoError(t, err)
	require.Len(t, atUpdate, 1)
	require.Equal(t, "after", atUpdate[0].Content)

	afterDelete, _, err := service.conversationMessagesAt(context.Background(), "session-1", 3, nil, nil, "asc", "", 20)
	require.NoError(t, err)
	require.Empty(t, afterDelete)
}

func TestSessionEventMaintenanceDispatchesUnsignaledCommittedRowsOnce(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_session_streams (
			session_id TEXT PRIMARY KEY, watermark INTEGER NOT NULL
		);
		CREATE TABLE conversation_session_events (
			session_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_id TEXT NOT NULL,
			protocol_version INTEGER NOT NULL, event_type TEXT NOT NULL, task_id TEXT,
			payload TEXT NOT NULL, created_at TIMESTAMP NOT NULL
		);
		INSERT INTO conversation_session_streams(session_id, watermark) VALUES ('session-1', 1);
		INSERT INTO conversation_session_events(
			session_id, sequence, event_id, protocol_version, event_type, task_id, payload, created_at
		) VALUES (
			'session-1', 1, 'session-1:1', 1, 'message.deleted', 'task-1',
			'{"type":"message.deleted","session_id":"session-1","task_id":"task-1","message_id":"message-1"}',
			'2026-09-07T12:00:00Z'
		);`)
	require.NoError(t, err)

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)
	var dispatched []SessionEvent
	service.SetSessionEventSink(func(event SessionEvent) {
		dispatched = append(dispatched, event)
	})

	require.NoError(t, service.maintainSessionEvents(time.Now().UTC()))
	require.Len(t, dispatched, 1)
	require.Equal(t, uint64(1), dispatched[0].Sequence)
	require.NoError(t, service.maintainSessionEvents(time.Now().UTC()))
	require.Len(t, dispatched, 1)
}

func TestPruneConversationJournalKeepsLatestRowsAndRecentEvents(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_message_versions (
			session_id TEXT NOT NULL, message_id TEXT NOT NULL, row_sequence INTEGER NOT NULL,
			task_id TEXT, author_type TEXT, created_at TIMESTAMP NOT NULL,
			tombstone BOOLEAN NOT NULL, payload TEXT NOT NULL
		);
		CREATE TABLE conversation_turn_versions (
			session_id TEXT NOT NULL, turn_id TEXT NOT NULL, row_sequence INTEGER NOT NULL,
			task_id TEXT, started_at TIMESTAMP NOT NULL, tombstone BOOLEAN NOT NULL, payload TEXT NOT NULL
		);
		CREATE TABLE conversation_session_events (
			session_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_id TEXT NOT NULL,
			protocol_version INTEGER NOT NULL, event_type TEXT NOT NULL, task_id TEXT,
			payload TEXT NOT NULL, created_at TIMESTAMP NOT NULL
		);`)
	require.NoError(t, err)
	old := time.Now().UTC().Add(-2 * SessionEventRetention)
	recent := time.Now().UTC()
	_, err = database.Exec(`
		INSERT INTO conversation_message_versions VALUES
			('session-1', 'message-1', 1, 'task-1', 'agent', ?, FALSE, '{}'),
			('session-1', 'message-1', 2, 'task-1', 'agent', ?, FALSE, '{}');
		INSERT INTO conversation_turn_versions VALUES
			('session-1', 'turn-1', 1, 'task-1', ?, FALSE, '{}'),
			('session-1', 'turn-1', 2, 'task-1', ?, FALSE, '{}');
		INSERT INTO conversation_session_events VALUES
			('session-1', 1, 'session-1:1', 1, 'message.added', 'task-1', '{}', ?),
			('session-1', 2, 'session-1:2', 1, 'message.updated', 'task-1', '{}', ?);`,
		old, recent, old, recent, old, recent)
	require.NoError(t, err)

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)
	require.NoError(t, service.pruneConversationJournal(context.Background(), time.Now().UTC().Add(-SessionEventRetention), nil))

	var count int
	require.NoError(t, database.Get(&count, "SELECT COUNT(*) FROM conversation_message_versions"))
	require.Equal(t, 1, count)
	require.NoError(t, database.Get(&count, "SELECT COUNT(*) FROM conversation_turn_versions"))
	require.Equal(t, 1, count)
	require.NoError(t, database.Get(&count, "SELECT COUNT(*) FROM conversation_session_events"))
	require.Equal(t, 1, count)
}
func insertJournalMessageVersion(t *testing.T, database *sqlx.DB, sequence int, tombstone bool, payload map[string]any) {

	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	_, err = database.Exec(`
		INSERT INTO conversation_message_versions(
			session_id, message_id, row_sequence, task_id, author_type, created_at, tombstone, payload
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"session-1", "message-1", sequence, "task-1", "agent", time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC), tombstone, string(raw),
	)
	require.NoError(t, err)
}
func TestSyncCommittedSessionEventsStripsSystemContent(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_session_streams (
			session_id TEXT PRIMARY KEY, watermark INTEGER NOT NULL
		);
		CREATE TABLE conversation_session_events (
			session_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_id TEXT NOT NULL,
			protocol_version INTEGER NOT NULL, event_type TEXT NOT NULL, task_id TEXT,
			payload TEXT NOT NULL, created_at TIMESTAMP NOT NULL
		);
		INSERT INTO conversation_session_streams(session_id, watermark) VALUES ('session-1', 1);
		INSERT INTO conversation_session_events VALUES (
			'session-1', 1, 'session-1:1', 1, 'message.added', 'task-1',
			'{"type":"message.added","session_id":"session-1","task_id":"task-1","message_id":"message-1","author_type":"user","author_id":"agent-1","metadata":{"sender_task_id":"sender-1","secret":"no"},"content":"visible <kandev-system>secret</kandev-system>"}',
			'2026-09-07T12:00:00Z'
		);`)
	require.NoError(t, err)

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)
	events, err := service.SyncCommittedSessionEvents(context.Background(), "session-1")
	require.NoError(t, err)
	require.Len(t, events, 1)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(events[0].Payload, &payload))
	require.Equal(t, "visible", payload["content"])
	require.Equal(t, "sender-1", payload["sender_task_id"])
	require.NotContains(t, payload, "author_id")
	require.NotContains(t, payload, "metadata")
}

func TestSyncCommittedSessionEventsPoisonsMalformedPayloadWithoutRetainingRawBytes(t *testing.T) {
	const sessionID = "malformed-journal-session"
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_session_streams (
			session_id TEXT PRIMARY KEY, watermark INTEGER NOT NULL
		);
		CREATE TABLE conversation_session_events (
			session_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_id TEXT NOT NULL,
			protocol_version INTEGER NOT NULL, event_type TEXT NOT NULL, task_id TEXT,
			payload TEXT NOT NULL, created_at TIMESTAMP NOT NULL
		);
		INSERT INTO conversation_session_streams(session_id, watermark) VALUES ('` + sessionID + `', 1);
		INSERT INTO conversation_session_events VALUES (
			'` + sessionID + `', 1, '` + sessionID + `:1', 1, 'message.added', 'task-1',
			'{"content":"private <kandev-system>secret</kandev-system>"',
			'2026-09-07T12:00:00Z'
		);`)
	require.NoError(t, err)

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)
	events, err := service.SyncCommittedSessionEvents(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Empty(t, events[0].Payload)
	_, projectionErr := ProjectSessionEvent(events[0])
	require.Error(t, projectionErr)
}
