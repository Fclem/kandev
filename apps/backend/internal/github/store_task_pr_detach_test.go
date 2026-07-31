package github

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestTaskPRDetachFiltersActiveRowsAndPersistsTombstone(t *testing.T) {
	store := newTestStore(t)
	detacher, ok := any(store).(interface {
		DetachTaskPR(context.Context, string) (*TaskPR, error)
	})
	if !ok {
		t.Fatal("Store does not implement DetachTaskPR")
	}
	ctx := context.Background()
	now := time.Now().UTC()
	first := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1",
		Owner: "acme", Repo: "demo", PRNumber: 1, PRURL: "https://github.com/acme/demo/pull/1",
		PRTitle: "old", HeadBranch: "old", BaseBranch: "main", State: "merged", CreatedAt: now,
	}
	second := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1",
		Owner: "acme", Repo: "demo", PRNumber: 2, PRURL: "https://github.com/acme/demo/pull/2",
		PRTitle: "new", HeadBranch: "new", BaseBranch: "main", State: "open", CreatedAt: now.Add(time.Second),
	}
	if err := store.CreateTaskPR(ctx, first); err != nil {
		t.Fatalf("create first PR: %v", err)
	}
	if err := store.CreateTaskPR(ctx, second); err != nil {
		t.Fatalf("create second PR: %v", err)
	}

	detached, err := detacher.DetachTaskPR(ctx, first.ID)
	if err != nil {
		t.Fatalf("detach first PR: %v", err)
	}
	detachedAt := reflect.Value{}
	if detached != nil {
		detachedAt = reflect.ValueOf(detached).Elem().FieldByName("DetachedAt")
	}
	if detached == nil || detached.ID != first.ID || !detachedAt.IsValid() || detachedAt.IsNil() {
		t.Fatalf("detached row = %+v, want stamped row", detached)
	}

	active, err := store.ListTaskPRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list active PRs: %v", err)
	}
	if len(active) != 1 || active[0].ID != second.ID {
		t.Fatalf("active PRs = %+v, want only second PR", active)
	}

	reopened, err := NewStore(store.db, store.ro)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	active, err = reopened.ListTaskPRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list active PRs after reopen: %v", err)
	}
	if len(active) != 1 || active[0].ID != second.ID {
		t.Fatalf("active PRs after reopen = %+v, want only second PR", active)
	}
}
