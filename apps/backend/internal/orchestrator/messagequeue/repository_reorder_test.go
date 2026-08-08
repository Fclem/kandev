package messagequeue

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func reorderRepos(t *testing.T) []struct {
	name string
	new  func(*testing.T) Repository
} {
	t.Helper()
	return []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
		{name: "postgres", new: newTestPostgresRepo},
	}
}

func assertOrderedIDs(t *testing.T, entries []QueuedMessage, want ...string) {
	t.Helper()
	if len(entries) != len(want) {
		t.Fatalf("entry count = %d, want %d (entries %#v)", len(entries), len(want), entries)
	}
	for i, id := range want {
		if entries[i].ID != id {
			t.Fatalf("order[%d] = %q, want %q (full order %v)", i, entries[i].ID, id, entryIDs(entries))
		}
	}
}

func entryIDs(entries []QueuedMessage) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}

func snapshotEntries(t *testing.T, repo Repository, sessionID string) []QueuedMessage {
	t.Helper()
	entries, err := repo.ListBySession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("list %q: %v", sessionID, err)
	}
	return entries
}

func TestReorderEntriesRewritesVisibleOrder(t *testing.T) {
	for _, tt := range reorderRepos(t) {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			ctx := context.Background()
			first := insertTestEntry(t, repo, "session-1", "task-1", "first", QueuedByUser, nil, nil)
			second := insertTestEntry(t, repo, "session-1", "task-1", "second", QueuedByUser, nil, nil)
			third := insertTestEntry(t, repo, "session-1", "task-1", "third", QueuedByUser, nil, nil)

			if err := repo.ReorderEntries(ctx, "session-1", []string{third.ID, first.ID, second.ID}); err != nil {
				t.Fatalf("reorder: %v", err)
			}
			entries := snapshotEntries(t, repo, "session-1")
			assertOrderedIDs(t, entries, third.ID, first.ID, second.ID)
			for i, e := range entries {
				if e.Position != int64(i+1) {
					t.Fatalf("position[%d] = %d, want %d", i, e.Position, i+1)
				}
				if e.Content == "" || e.QueuedBy != QueuedByUser || e.SessionID != "session-1" {
					t.Fatalf("reorder lost entry data: %#v", e)
				}
			}
		})
	}
}

func TestReorderEntriesNoOpKeepsPositions(t *testing.T) {
	for _, tt := range reorderRepos(t) {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			ctx := context.Background()
			first := insertTestEntry(t, repo, "session-1", "task-1", "first", QueuedByUser, nil, nil)
			second := insertTestEntry(t, repo, "session-1", "task-1", "second", QueuedByUser, nil, nil)
			before := snapshotEntries(t, repo, "session-1")

			if err := repo.ReorderEntries(ctx, "session-1", []string{first.ID, second.ID}); err != nil {
				t.Fatalf("no-op reorder: %v", err)
			}
			after := snapshotEntries(t, repo, "session-1")
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("no-op reorder changed the queue:\nbefore %#v\nafter  %#v", before, after)
			}
		})
	}
}

func TestReorderEntriesRejectsDriftAtomically(t *testing.T) {
	tests := []struct {
		name string
		ids  func(first, second *QueuedMessage) []string
	}{
		{name: "missing id", ids: func(first, _ *QueuedMessage) []string { return []string{first.ID} }},
		{name: "unknown id", ids: func(first, second *QueuedMessage) []string { return []string{first.ID, second.ID, "ghost"} }},
		{name: "duplicate id", ids: func(first, second *QueuedMessage) []string { return []string{first.ID, first.ID} }},
		{name: "empty list", ids: func(_, _ *QueuedMessage) []string { return nil }},
	}
	for _, tt := range reorderRepos(t) {
		t.Run(tt.name, func(t *testing.T) {
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					repo := tt.new(t)
					ctx := context.Background()
					first := insertTestEntry(t, repo, "session-1", "task-1", "first", QueuedByUser, nil, nil)
					second := insertTestEntry(t, repo, "session-1", "task-1", "second", QueuedByUser, nil, nil)
					before := snapshotEntries(t, repo, "session-1")

					err := repo.ReorderEntries(ctx, "session-1", tc.ids(first, second))
					if !errors.Is(err, ErrQueueChanged) {
						t.Fatalf("reorder error = %v, want ErrQueueChanged", err)
					}
					after := snapshotEntries(t, repo, "session-1")
					if !reflect.DeepEqual(before, after) {
						t.Fatalf("failed reorder mutated the queue:\nbefore %#v\nafter  %#v", before, after)
					}
				})
			}
		})
	}
}

func TestReorderEntriesKeepsReservedRowInPlace(t *testing.T) {
	for _, tt := range reorderRepos(t) {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			ctx := context.Background()
			// A durable lifecycle row already reserved in flight is hidden from
			// clients and must keep its place in the sequence.
			reserved := insertTestEntry(t, repo, "session-1", "task-1", "durable", QueuedByWorkflow, nil,
				map[string]interface{}{MetadataLifecycleDurable: true, MetadataLifecycleReserved: true})
			first := insertTestEntry(t, repo, "session-1", "task-1", "first", QueuedByUser, nil, nil)
			second := insertTestEntry(t, repo, "session-1", "task-1", "second", QueuedByUser, nil, nil)

			if err := repo.ReorderEntries(ctx, "session-1", []string{second.ID, first.ID}); err != nil {
				t.Fatalf("reorder visible rows: %v", err)
			}
			entries := snapshotEntries(t, repo, "session-1")
			assertOrderedIDs(t, entries, reserved.ID, second.ID, first.ID)
			if entries[0].Position != 1 || entries[1].Position != 2 || entries[2].Position != 3 {
				t.Fatalf("positions = %d/%d/%d, want 1/2/3", entries[0].Position, entries[1].Position, entries[2].Position)
			}
			if !entries[0].IsReservedInFlight() {
				t.Fatalf("reserved row lost its in-flight marker: %#v", entries[0])
			}

			// Submitting a reserved id is drift: it is not part of the visible set.
			if err := repo.ReorderEntries(ctx, "session-1", []string{reserved.ID, first.ID, second.ID}); !errors.Is(err, ErrQueueChanged) {
				t.Fatalf("reorder with reserved id error = %v, want ErrQueueChanged", err)
			}
			after := snapshotEntries(t, repo, "session-1")
			if !reflect.DeepEqual(entries, after) {
				t.Fatalf("failed reorder mutated the queue:\nbefore %#v\nafter  %#v", entries, after)
			}
		})
	}
}

func TestReorderEntriesDrainRaceRejected(t *testing.T) {
	for _, tt := range reorderRepos(t) {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			ctx := context.Background()
			first := insertTestEntry(t, repo, "session-1", "task-1", "first", QueuedByUser, nil, nil)
			second := insertTestEntry(t, repo, "session-1", "task-1", "second", QueuedByUser, nil, nil)

			// A drain wins between the client's snapshot and the reorder: the
			// submitted set no longer matches the visible set.
			if _, err := repo.TakeHead(ctx, "session-1"); err != nil {
				t.Fatalf("take head: %v", err)
			}
			if err := repo.ReorderEntries(ctx, "session-1", []string{first.ID, second.ID}); !errors.Is(err, ErrQueueChanged) {
				t.Fatalf("reorder after drain error = %v, want ErrQueueChanged", err)
			}
			entries := snapshotEntries(t, repo, "session-1")
			assertOrderedIDs(t, entries, second.ID)
		})
	}
}
