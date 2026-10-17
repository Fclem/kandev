package sqlite

import (
	"context"
	"testing"
)

func TestUsageInputPresenceSurvivesStorage(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-input")
	createUsageEventsTestSession(t, repo, "session-input", "task-input")
	ctx := context.Background()
	for _, missing := range []bool{true, false} {
		id := "measured-zero"
		if missing {
			id = "missing"
		}
		event := newTestUsageEvent(id, "task-input", "session-input")
		event.TurnID = "turn-input"
		event.TokensIn, event.TokensInMissing, event.TokensTotal = 0, missing, 55
		event.CostSource = "unpriced"
		if err := repo.CreateTaskUsageEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	turnRows, err := repo.ListSessionUsageEventsByTurn(ctx, "session-input", "turn-input")
	if err != nil || len(turnRows) != 2 {
		t.Fatalf("turn rows = %v, error %v", turnRows, err)
	}
	if !turnRows[0].TokensInMissing || turnRows[1].TokensInMissing {
		t.Fatalf("input presence erased on reload: %+v/%+v", turnRows[0], turnRows[1])
	}
	taskRows, err := repo.ListTaskUsageEvents(ctx, "task-input", 0)
	if err != nil || len(taskRows) != 2 || !taskRows[0].TokensInMissing || taskRows[1].TokensInMissing {
		t.Fatalf("task detail erased input presence: %+v, error %v", taskRows, err)
	}
	input, _, output, _ := readTaskSessionRollup(t, repo, "session-input")
	if input != 0 || output != 60 {
		t.Fatalf("measurement presence changed legacy arithmetic: input %d, output %d", input, output)
	}
}
