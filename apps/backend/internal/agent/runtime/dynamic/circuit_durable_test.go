package dynamic

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
func TestDurableCircuitMissingAdapterPublishesNothing(t *testing.T) {
	registry := NewCircuitRegistry()
	ctx := context.Background()
	key := "limit|profile:one|model|opus"
	if err := registry.OpenDurable(ctx, key, time.Now().Add(time.Hour), routingerr.CodeQuotaLimited, true); err == nil {
		t.Fatal("a missing persistence adapter accepted a mark")
	}
	if registry.IsOpen(key, time.Now()) {
		t.Fatal("an unpersisted mark became available to recovery")
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.3
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
func TestDurableCircuitWriteFailureKeepsPriorMarkAndRetryRestores(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	persist := newStubCircuitPersistence()
	registry := NewCircuitRegistry(WithCircuitPersistence(persist), WithCircuitClock(func() time.Time { return now }))
	key := "limit|binding|account"
	original := now.Add(30 * time.Minute)
	if err := registry.OpenDurable(ctx, key, original, routingerr.CodeQuotaLimited, false); err != nil {
		t.Fatal(err)
	}
	persist.setFailing(true)
	extended := now.Add(8 * 24 * time.Hour)
	if err := registry.OpenDurable(ctx, key, extended, routingerr.CodeQuotaLimited, true); err == nil {
		t.Fatal("failed persistence accepted a reset update")
	}
	prior, ok := registry.Get(key)
	if !ok || !prior.Until.Equal(original) || prior.ResetKnown {
		t.Fatalf("unpersisted update replaced the prior mark: %+v", prior)
	}
	restored := NewCircuitRegistry(WithCircuitPersistence(persist))
	if err := restored.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	prior, ok = restored.Get(key)
	if !ok || !prior.Until.Equal(original) || prior.ResetKnown {
		t.Fatalf("restart restored an unpersisted update: %+v", prior)
	}
	persist.setFailing(false)
	if err := registry.OpenDurable(ctx, key, extended, routingerr.CodeQuotaLimited, true); err != nil {
		t.Fatal(err)
	}
	if err := restored.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	accepted, ok := restored.Get(key)
	if !ok || !accepted.Until.Equal(extended) || !accepted.ResetKnown {
		t.Fatalf("accepted eight-day reset did not restore: %+v", accepted)
	}
	if err := registry.OpenDurable(ctx, key, now.Add(time.Hour), routingerr.CodeRateLimited, false); err != nil {
		t.Fatal(err)
	}
	accepted, _ = registry.Get(key)
	if !accepted.Until.Equal(extended) || !accepted.ResetKnown {
		t.Fatalf("an earlier expiry shortened or forgot the accepted reset: %+v", accepted)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.5
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
func TestDurableCircuitAtomicClearFailurePreservesBothMarks(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	persist := newStubCircuitPersistence()
	registry := NewCircuitRegistry(WithCircuitPersistence(persist))
	keys := []string{"limit|binding|account", "limit|binding|model|opus"}
	for _, key := range keys {
		if err := registry.OpenDurable(ctx, key, now.Add(time.Hour), routingerr.CodeQuotaLimited, true); err != nil {
			t.Fatal(err)
		}
	}
	persist.setFailing(true)
	if err := registry.CloseManyDurable(ctx, keys); err == nil {
		t.Fatal("failed batch clear accepted")
	}
	restored := NewCircuitRegistry(WithCircuitPersistence(persist))
	if err := restored.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if !registry.IsOpen(key, now) || !restored.IsOpen(key, now) {
			t.Fatalf("failed clear released sibling recovery on %s", key)
		}
	}
	persist.setFailing(false)
	if err := registry.CloseManyDurable(ctx, keys); err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if registry.IsOpen(key, now) {
			t.Fatalf("accepted clear kept %s limited", key)
		}
	}
	restarted := NewCircuitRegistry(WithCircuitPersistence(persist))
	if err := restarted.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if restarted.IsOpen(key, now) {
			t.Fatalf("accepted clear did not survive restart: %s", key)
		}
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.6
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
func TestDurableCircuitProbeFailuresRestoreExactOwnership(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	persist := newStubCircuitPersistence()
	registry := NewCircuitRegistry(WithCircuitPersistence(persist), WithCircuitClock(func() time.Time { return now }))
	key := "limit|binding|account"
	if err := registry.OpenDurable(ctx, key, now.Add(-time.Second), routingerr.CodeQuotaLimited, true); err != nil {
		t.Fatal(err)
	}
	persist.setFailing(true)
	if _, acquired, err := registry.AcquireProbeDurable(ctx, key, 10*time.Minute); err == nil || acquired {
		t.Fatal("failed lease write authorized a probe")
	}
	state, _ := registry.Get(key)
	if state.State != CircuitOpen || !state.ProbeUntil.IsZero() {
		t.Fatalf("failed acquisition published ownership: %+v", state)
	}
	persist.setFailing(false)
	lease, acquired, err := registry.AcquireProbeDurable(ctx, key, 10*time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquire=%t error=%v", acquired, err)
	}
	persist.setFailing(true)
	if released, err := registry.ReleaseProbeDurable(ctx, lease, false, 0); err == nil || released {
		t.Fatal("failed lease release dropped ownership")
	}
	restarted := NewCircuitRegistry(WithCircuitPersistence(persist), WithCircuitClock(func() time.Time { return now }))
	if err := restarted.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	if _, acquired, err := restarted.AcquireProbeDurable(ctx, key, 10*time.Minute); err != nil || acquired {
		t.Fatalf("restart ignored the active lease: acquired=%t error=%v", acquired, err)
	}
	state, _ = restarted.Get(key)
	if !state.ProbeUntil.Equal(lease.ExpiresAt) {
		t.Fatalf("restart changed exact lease identity: %+v", state)
	}
	persist.setFailing(false)
	now = lease.ExpiresAt
	replacement, acquired, err := restarted.AcquireProbeDurable(ctx, key, 10*time.Minute)
	if err != nil || !acquired {
		t.Fatalf("expired lease blocked replacement: acquired=%t error=%v", acquired, err)
	}
	if released, err := restarted.ReleaseProbeDurable(ctx, lease, true, 0); err != nil || released {
		t.Fatalf("stale owner released replacement: released=%t error=%v", released, err)
	}
	state, _ = restarted.Get(key)
	if !state.ProbeUntil.Equal(replacement.ExpiresAt) || state.State != CircuitHalfOpen {
		t.Fatalf("replacement ownership changed: %+v", state)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.5
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.6
func TestDurableCircuitClosedMarkRetainsLeaseUntilDurableRelease(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	persist := newStubCircuitPersistence()
	registry := NewCircuitRegistry(WithCircuitPersistence(persist), WithCircuitClock(func() time.Time { return now }))
	key := "limit|binding|account"
	if err := registry.OpenDurable(ctx, key, now.Add(-time.Second), routingerr.CodeQuotaLimited, true); err != nil {
		t.Fatal(err)
	}
	lease, acquired, err := registry.AcquireProbeDurable(ctx, key, 10*time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquire=%t error=%v", acquired, err)
	}
	if err := registry.CloseManyDurable(ctx, []string{key}); err != nil {
		t.Fatal(err)
	}
	persist.setFailing(true)
	if released, err := registry.ReleaseProbeDurable(ctx, lease, true, 0); err == nil || released {
		t.Fatal("failed release lost the retained closed-mark lease")
	}
	restarted := NewCircuitRegistry(WithCircuitPersistence(persist))
	if err := restarted.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	closed, ok := restarted.Get(key)
	if !ok || closed.State != CircuitClosed || !closed.ProbeUntil.Equal(lease.ExpiresAt) {
		t.Fatalf("restart lost closed-mark ownership: %+v", closed)
	}
	persist.setFailing(false)
	if released, err := restarted.ReleaseProbeDurable(ctx, lease, true, 0); err != nil || !released {
		t.Fatalf("retained exact lease did not release: released=%t error=%v", released, err)
	}
	closed, _ = restarted.Get(key)
	if !closed.ProbeUntil.IsZero() || closed.State != CircuitClosed {
		t.Fatalf("released ownership still retained: %+v", closed)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.6
func TestDurableCircuitFailedProbeRetainsOriginalResetEvidence(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	persist := newStubCircuitPersistence()
	registry := NewCircuitRegistry(WithCircuitPersistence(persist), WithCircuitClock(func() time.Time { return now }))
	key := "limit|binding|account"
	reset := now.Add(-time.Second)
	if err := registry.OpenDurable(ctx, key, reset, routingerr.CodeQuotaLimited, true); err != nil {
		t.Fatal(err)
	}
	lease, acquired, err := registry.AcquireProbeDurable(ctx, key, 10*time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquire=%t error=%v", acquired, err)
	}
	if released, err := registry.ReleaseProbeDurable(ctx, lease, false, 0); err != nil || !released {
		t.Fatalf("release=%t error=%v", released, err)
	}
	retained, _ := registry.Get(key)
	if retained.State != CircuitOpen || !retained.ResetKnown || !retained.Until.Equal(reset) || !retained.ProbeUntil.IsZero() {
		t.Fatalf("a failed probe without a new limit discarded the original reset: %+v", retained)
	}
	if _, acquired, err := registry.AcquireProbeDurable(ctx, key, 10*time.Minute); err != nil || !acquired {
		t.Fatalf("released probe blocked a later waiter: acquired=%t error=%v", acquired, err)
	}
}
