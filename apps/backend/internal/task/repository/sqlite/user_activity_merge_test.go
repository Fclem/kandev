package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestRecordUserActivityMergeMatrix(t *testing.T) {
	base := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
	tests := []struct {
		name          string
		beats         []time.Duration
		wantOutcomes  []string
		wantIntervals int64
		wantActiveMS  int64
	}{
		{
			name:          "coalesced beat does not advance the last recorded interval",
			beats:         []time.Duration{0, 4 * time.Second, 47 * time.Second},
			wantOutcomes:  []string{userActivityOutcomeRecorded, userActivityOutcomeCoalesced, userActivityOutcomeRecorded},
			wantIntervals: 2,
		},
		{
			name:          "gap below forty-five seconds extends the interval",
			beats:         []time.Duration{0, 4 * time.Second, 10 * time.Second},
			wantOutcomes:  []string{userActivityOutcomeRecorded, userActivityOutcomeCoalesced, userActivityOutcomeRecorded},
			wantIntervals: 1,
			wantActiveMS:  10_000,
		},
		{
			name:          "gap at forty-five seconds opens a new interval",
			beats:         []time.Duration{0, 45 * time.Second},
			wantOutcomes:  []string{userActivityOutcomeRecorded, userActivityOutcomeRecorded},
			wantIntervals: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			if err := repo.CreateTask(ctx, &models.Task{ID: "merge-task", Title: "merge"}); err != nil {
				t.Fatal(err)
			}
			if err := repo.RecordCaptureTransition(ctx, true, base); err != nil {
				t.Fatal(err)
			}
			repo.SetUserActivityCapture(true)
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "merge-session", TaskID: "merge-task", StartedAt: base}); err != nil {
				t.Fatal(err)
			}

			for i, offset := range test.beats {
				outcome, err := repo.RecordUserActivity(ctx, "actor", "merge-session", base.Add(offset))
				if err != nil {
					t.Fatalf("beat %d: %v", i, err)
				}
				if outcome != test.wantOutcomes[i] {
					t.Errorf("beat %d outcome = %q, want %q", i, outcome, test.wantOutcomes[i])
				}
			}

			summaries, err := repo.SumUserActivity(ctx, []string{"merge-session"})
			if err != nil {
				t.Fatal(err)
			}
			if len(summaries) != 1 {
				t.Fatalf("summary count = %d, want one", len(summaries))
			}
			if summaries[0].IntervalCount != test.wantIntervals {
				t.Errorf("interval count = %d, want %d", summaries[0].IntervalCount, test.wantIntervals)
			}
			if delta := summaries[0].ActiveMS - test.wantActiveMS; delta < -1 || delta > 1 {
				t.Errorf("active_ms = %d, want %d within SQLite timestamp precision", summaries[0].ActiveMS, test.wantActiveMS)
			}
		})
	}
}

func TestRecordUserActivityConcurrentBeatsDoNotOverlap(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
	if err := repo.CreateTask(ctx, &models.Task{ID: "concurrent-task", Title: "concurrent"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCaptureTransition(ctx, true, base); err != nil {
		t.Fatal(err)
	}
	repo.SetUserActivityCapture(true)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "concurrent-session", TaskID: "concurrent-task", StartedAt: base}); err != nil {
		t.Fatal(err)
	}

	const beats = 12
	start := make(chan struct{})
	errs := make(chan error, beats)
	var wg sync.WaitGroup
	for range beats {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := repo.RecordUserActivity(ctx, "actor", "concurrent-session", base)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent beat: %v", err)
		}
	}

	summaries, err := repo.SumUserActivity(ctx, []string{"concurrent-session"})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].IntervalCount != 1 {
		t.Fatalf("summary = %+v, want one non-overlapping interval", summaries)
	}
}
