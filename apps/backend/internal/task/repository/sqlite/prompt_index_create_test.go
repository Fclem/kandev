package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestLiveCreateAdvancesCollidingTimestamps covers the atomic per-session
// create boundary for live user messages: with a fixed clock, every create
// starts with the same timestamp, and the boundary advances each new message
// by one microsecond tick so reread order stays distinct and consistent. The
// one-tick advance stays within the bounded lead window.
func TestLiveCreateAdvancesCollidingTimestamps(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-LIVE", "sess-LIVE", "turn-LIVE")

	fixed := time.Date(2026, 8, 19, 10, 0, 0, 123456789, time.UTC)
	repo.clockNow = func() time.Time { return fixed }

	// Reverse-ordered IDs: the later-created message has the smaller id.
	first := &models.Message{ID: "live-zzz", TaskSessionID: "sess-LIVE", TurnID: "turn-LIVE", AuthorType: models.MessageAuthorUser, Content: "first"}
	if err := repo.CreateMessage(ctx, first); err != nil {
		t.Fatalf("first create: %v", err)
	}
	second := &models.Message{ID: "live-aaa", TaskSessionID: "sess-LIVE", TurnID: "turn-LIVE", AuthorType: models.MessageAuthorUser, Content: "second"}
	if err := repo.CreateMessage(ctx, second); err != nil {
		t.Fatalf("second create: %v", err)
	}

	if first.PromptIndex != 1 || second.PromptIndex != 2 {
		t.Fatalf("ordinals = (%d, %d), want (1, 2)", first.PromptIndex, second.PromptIndex)
	}
	// The boundary advanced the second message by one ordering-key tick (the
	// first row retains its full nanosecond precision, so the raw gap may be
	// less than a microsecond; the normalized keys differ by exactly one
	// microsecond) and the timestamps stay strictly ordered.
	if !second.CreatedAt.After(first.CreatedAt) {
		t.Errorf("second.CreatedAt = %v, want strictly after first %v", second.CreatedAt, first.CreatedAt)
	}
	if got := formatPromptKey(second.CreatedAt); got != formatPromptKey(first.CreatedAt.Add(time.Microsecond)) {
		t.Errorf("key advance = %q, want %q (one microsecond tick)", got, formatPromptKey(first.CreatedAt.Add(time.Microsecond)))
	}
	if second.CreatedAt.After(fixed.Add(promptIndexLeadWindow)) {
		t.Errorf("advanced timestamp %v is beyond the lead window", second.CreatedAt)
	}

	// Reread order and indexed reads agree with the assigned ordinals.
	got1, err := repo.GetMessageWithPromptIndex(ctx, "live-zzz")
	if err != nil {
		t.Fatalf("reread first: %v", err)
	}
	got2, err := repo.GetMessageWithPromptIndex(ctx, "live-aaa")
	if err != nil {
		t.Fatalf("reread second: %v", err)
	}
	if got1.PromptIndex != 1 || got2.PromptIndex != 2 {
		t.Errorf("reread ordinals = (%d, %d), want (1, 2)", got1.PromptIndex, got2.PromptIndex)
	}
}

// TestLiveCreateOverWindowClockSkewReturnsRetryableError pins the bounded
// lead window: when the session's newest user message is ahead of the clock
// by more than the window, a live create must return a retryable error
// instead of stamping a message far in the future, and must leave the
// caller-visible CreatedAt/PromptIndex fields untouched.
func TestLiveCreateOverWindowClockSkewReturnsRetryableError(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-SKEW", "sess-SKEW", "turn-SKEW")

	fixed := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	repo.clockNow = func() time.Time { return fixed }

	// An explicit seed two minutes in the future (relative to the fixed
	// clock) is valid on its own: it is strictly after the empty session max.
	seed := &models.Message{
		ID: "skew-seed", TaskSessionID: "sess-SKEW", TurnID: "turn-SKEW",
		AuthorType: models.MessageAuthorUser, Content: "future seed",
		CreatedAt: fixed.Add(2 * time.Minute),
	}
	if err := repo.CreateMessage(ctx, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if seed.PromptIndex != 1 {
		t.Fatalf("seed ordinal = %d, want 1", seed.PromptIndex)
	}

	// A live create at the fixed clock would need to advance past the seed,
	// beyond the lead window: reject with the retryable sentinel and keep the
	// model clean for the retry.
	live := &models.Message{
		ID: "skew-live", TaskSessionID: "sess-SKEW", TurnID: "turn-SKEW",
		AuthorType: models.MessageAuthorUser, Content: "live",
	}
	err := repo.CreateMessage(ctx, live)
	if !errors.Is(err, ErrMessageClockSkew) {
		t.Fatalf("live create error = %v, want ErrMessageClockSkew", err)
	}
	if !live.CreatedAt.IsZero() || live.PromptIndex != 0 {
		t.Errorf("failed attempt mutated model: CreatedAt=%v PromptIndex=%d, want zero/0", live.CreatedAt, live.PromptIndex)
	}

	// Once the clock catches up, the same create succeeds with the next
	// ordinal.
	repo.clockNow = func() time.Time { return fixed.Add(3 * time.Minute) }
	if err := repo.CreateMessage(ctx, live); err != nil {
		t.Fatalf("create after clock catch-up: %v", err)
	}
	if live.PromptIndex != 2 {
		t.Errorf("ordinal after catch-up = %d, want 2", live.PromptIndex)
	}
}

// TestExplicitUserTimestampMustBeStrictlyAfterMax covers the zero-vs-explicit
// CreatedAt branch: explicit imports preserve only valid strictly-after-max
// timestamps; an explicit timestamp at or before the session's newest user
// message is rejected and leaves the model untouched.
func TestExplicitUserTimestampMustBeStrictlyAfterMax(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-EXP", "sess-EXP", "turn-EXP")

	base := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	first := &models.Message{
		ID: "exp-1", TaskSessionID: "sess-EXP", TurnID: "turn-EXP",
		AuthorType: models.MessageAuthorUser, Content: "first", CreatedAt: base,
	}
	if err := repo.CreateMessage(ctx, first); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if first.PromptIndex != 1 {
		t.Fatalf("first ordinal = %d, want 1", first.PromptIndex)
	}

	// Equal timestamp: not strictly after max → rejected.
	dup := &models.Message{
		ID: "exp-dup", TaskSessionID: "sess-EXP", TurnID: "turn-EXP",
		AuthorType: models.MessageAuthorUser, Content: "duplicate", CreatedAt: base,
	}
	err := repo.CreateMessage(ctx, dup)
	if !errors.Is(err, ErrMessageTimestampNotAfterNewest) {
		t.Fatalf("equal-timestamp create error = %v, want ErrMessageTimestampNotAfterNewest", err)
	}
	if !dup.CreatedAt.Equal(base) || dup.PromptIndex != 0 {
		t.Errorf("rejected create mutated model: CreatedAt=%v PromptIndex=%d", dup.CreatedAt, dup.PromptIndex)
	}

	// Backward timestamp: also rejected.
	older := &models.Message{
		ID: "exp-older", TaskSessionID: "sess-EXP", TurnID: "turn-EXP",
		AuthorType: models.MessageAuthorUser, Content: "older", CreatedAt: base.Add(-time.Second),
	}
	if err := repo.CreateMessage(ctx, older); !errors.Is(err, ErrMessageTimestampNotAfterNewest) {
		t.Fatalf("older-timestamp create error = %v, want ErrMessageTimestampNotAfterNewest", err)
	}

	// Strictly-after timestamp: preserved verbatim with the next ordinal.
	second := &models.Message{
		ID: "exp-2", TaskSessionID: "sess-EXP", TurnID: "turn-EXP",
		AuthorType: models.MessageAuthorUser, Content: "second", CreatedAt: base.Add(time.Second),
	}
	if err := repo.CreateMessage(ctx, second); err != nil {
		t.Fatalf("second create: %v", err)
	}
	if second.PromptIndex != 2 {
		t.Fatalf("second ordinal = %d, want 2", second.PromptIndex)
	}
	if !second.CreatedAt.Equal(base.Add(time.Second)) {
		t.Errorf("explicit timestamp not preserved: %v", second.CreatedAt)
	}
}

// TestFailedCreateAttemptKeepsModelClean proves a failed repository attempt
// (here: a duplicate primary key insert after the boundary computed the
// ordinal) does not leave CreatedAt/PromptIndex on the retried model.
func TestFailedCreateAttemptKeepsModelClean(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-RETRY", "sess-RETRY", "turn-RETRY")

	winner := &models.Message{
		ID: "retry-id", TaskSessionID: "sess-RETRY", TurnID: "turn-RETRY",
		AuthorType: models.MessageAuthorUser, Content: "winner",
	}
	if err := repo.CreateMessage(ctx, winner); err != nil {
		t.Fatalf("winner create: %v", err)
	}
	if winner.PromptIndex != 1 {
		t.Fatalf("winner ordinal = %d, want 1", winner.PromptIndex)
	}

	// Same ID: the boundary computes an ordinal, then the insert fails on the
	// primary key. The caller-visible fields must be restored so the retry
	// sees a clean model.
	retry := &models.Message{
		ID: "retry-id", TaskSessionID: "sess-RETRY", TurnID: "turn-RETRY",
		AuthorType: models.MessageAuthorUser, Content: "retry",
	}
	if err := repo.CreateMessage(ctx, retry); err == nil {
		t.Fatal("duplicate create succeeded, want failure")
	}
	if !retry.CreatedAt.IsZero() || retry.PromptIndex != 0 || !retry.UpdatedAt.IsZero() {
		t.Errorf("failed attempt mutated model: CreatedAt=%v UpdatedAt=%v PromptIndex=%d",
			retry.CreatedAt, retry.UpdatedAt, retry.PromptIndex)
	}

	// The winning row's ordinal is stable.
	got, err := repo.GetMessageWithPromptIndex(ctx, "retry-id")
	if err != nil {
		t.Fatalf("reread winner: %v", err)
	}
	if got.PromptIndex != 1 {
		t.Errorf("winner ordinal after failed retry = %d, want 1", got.PromptIndex)
	}
}
