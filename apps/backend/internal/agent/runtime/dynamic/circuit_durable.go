package dynamic

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

var ErrCircuitPersistenceUnavailable = errors.New("circuit persistence is unavailable")
var ErrCircuitKeyEmpty = errors.New("circuit key is empty")

func (r *CircuitRegistry) OpenDurable(ctx context.Context, key string, until time.Time, code routingerr.Code, resetKnown bool) error {
	if key == "" {
		return ErrCircuitKeyEmpty
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	previous := r.circuits[key]
	proposed := circuit{state: CircuitOpen, until: until, code: code, resetKnown: resetKnown, probeUntil: previous.probeUntil}
	if previous.state != CircuitClosed && previous.until.After(until) {
		proposed.until, proposed.code, proposed.resetKnown = previous.until, previous.code, previous.resetKnown
	} else if previous.until.Equal(until) {
		proposed.resetKnown = resetKnown || previous.resetKnown
	}
	return r.persistDurableLocked(ctx, key, proposed)
}

// CloseManyDurable retains exact probe ownership until a separate durable release.
func (r *CircuitRegistry) CloseManyDurable(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.persist == nil {
		return ErrCircuitPersistenceUnavailable
	}
	snapshots := make([]CircuitSnapshot, len(keys))
	for index, key := range keys {
		if key == "" {
			return ErrCircuitKeyEmpty
		}
		snapshots[index] = circuitSnapshot(key, circuit{state: CircuitClosed, probeUntil: r.circuits[key].probeUntil})
	}
	if err := r.persist.SaveCircuits(ctx, snapshots); err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		r.circuits[snapshot.Key] = circuit{state: CircuitClosed, probeUntil: snapshot.ProbeUntil}
		delete(r.pending, snapshot.Key)
	}
	return nil
}

func (r *CircuitRegistry) AcquireProbeDurable(ctx context.Context, key string, duration time.Duration) (ProbeLease, bool, error) {
	if key == "" || duration <= 0 {
		return ProbeLease{}, false, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, exists := r.circuits[key]
	now := r.now()
	if !exists || entry.state == CircuitClosed || now.Before(entry.until) || now.Before(entry.probeUntil) {
		return ProbeLease{}, false, nil
	}
	lease := ProbeLease{Key: key, ExpiresAt: now.Add(duration)}
	entry.state, entry.probeUntil = CircuitHalfOpen, lease.ExpiresAt
	if err := r.persistDurableLocked(ctx, key, entry); err != nil {
		return ProbeLease{}, false, err
	}
	return lease, true, nil
}

func (r *CircuitRegistry) ReleaseProbeDurable(ctx context.Context, lease ProbeLease, success bool, backoff time.Duration) (bool, error) {
	if lease.Key == "" || lease.ExpiresAt.IsZero() {
		return false, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, exists := r.circuits[lease.Key]
	if !exists || !entry.probeUntil.Equal(lease.ExpiresAt) {
		return false, nil
	}
	entry.probeUntil = time.Time{}
	if success {
		entry.state, entry.until, entry.code, entry.resetKnown = CircuitClosed, time.Time{}, "", false
	} else if entry.state != CircuitClosed {
		entry.state = CircuitOpen
		if backoff > 0 {
			if until := r.now().Add(backoff); until.After(entry.until) {
				entry.until, entry.resetKnown = until, false
			}
		}
	}
	if err := r.persistDurableLocked(ctx, lease.Key, entry); err != nil {
		return false, err
	}
	return true, nil
}

func (r *CircuitRegistry) persistDurableLocked(ctx context.Context, key string, proposed circuit) error {
	if r.persist == nil {
		return ErrCircuitPersistenceUnavailable
	}
	if err := r.persist.SaveCircuit(ctx, circuitSnapshot(key, proposed)); err != nil {
		return err
	}
	r.circuits[key] = proposed
	delete(r.pending, key)
	return nil
}
