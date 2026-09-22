package github

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

const (
	personalStarCleanupInterval     = time.Minute
	personalStarCleanupLease        = 2 * time.Minute
	personalStarCleanupBaseBackoff  = 30 * time.Second
	personalStarCleanupMaxBackoff   = time.Hour
	personalStarCleanupMaxAttempts  = 8
	personalStarCleanupBatchSize    = 32
	personalStarFlowSweepEveryTicks = 15
)

// StarSecretCleanupRunner retries generation-specific secret deletion. It is
// created by Provide, started before readiness, and drained by the GitHub
// runtime Stop path.
type StarSecretCleanupRunner struct {
	store   *Store
	secrets PersonalStarSecrets
	log     *logger.Logger
	now     func() time.Time

	mu      sync.Mutex
	started bool
	stop    chan struct{}
	done    chan struct{}
}

// NewStarSecretCleanupRunner creates the durable cleanup runner.
func NewStarSecretCleanupRunner(store *Store, secrets PersonalStarSecrets, log *logger.Logger) *StarSecretCleanupRunner {
	return &StarSecretCleanupRunner{
		store:   store,
		secrets: secrets,
		log:     log,
		now:     time.Now,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Start begins the retry loop. Repeated calls are no-ops.
func (r *StarSecretCleanupRunner) Start() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.mu.Unlock()
	go r.loop()
}

// Stop cancels the loop and waits for the in-flight pass to finish, so callers
// can close the secret store after Stop returns.
func (r *StarSecretCleanupRunner) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return
	}
	r.started = false
	close(r.stop)
	r.mu.Unlock()
	<-r.done
}

func (r *StarSecretCleanupRunner) loop() {
	defer close(r.done)
	ticker := time.NewTicker(personalStarCleanupInterval)
	defer ticker.Stop()
	ticks := 0
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			ticks++
			if _, err := r.RunOnce(context.Background()); err != nil && r.log != nil {
				r.log.Warn("personal star secret cleanup pass failed", zap.Error(err))
			}
			if ticks%personalStarFlowSweepEveryTicks == 0 {
				if _, err := r.store.DeleteExpiredPersonalStarFlows(context.Background(), r.now().UTC()); err != nil && r.log != nil {
					r.log.Warn("personal star flow sweep failed", zap.Error(err))
				}
			}
		}
	}
}

// RunOnce performs one bounded pass and returns how many secrets it deleted.
// Each claimed record is retried with bounded exponential backoff and becomes
// terminal after the attempt cap so operators can see the failure.
func (r *StarSecretCleanupRunner) RunOnce(ctx context.Context) (int, error) {
	now := r.now().UTC()
	claimed, err := r.store.ClaimDuePersonalStarSecretCleanups(ctx, now, now.Add(personalStarCleanupLease), personalStarCleanupBatchSize)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, record := range claimed {
		if err := r.secrets.Delete(ctx, record.SecretID); err != nil {
			if rescheduleErr := r.reschedule(ctx, record, err); rescheduleErr != nil {
				return deleted, rescheduleErr
			}
			continue
		}
		if err := r.store.CompletePersonalStarSecretCleanup(ctx, record.ID); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func (r *StarSecretCleanupRunner) reschedule(ctx context.Context, record *PersonalStarSecretCleanup, cause error) error {
	attempts := record.Attempts + 1
	if attempts >= personalStarCleanupMaxAttempts {
		return r.store.MarkPersonalStarSecretCleanupTerminal(ctx, record.ID, cause.Error())
	}
	return r.store.ReschedulePersonalStarSecretCleanup(
		ctx, record.ID, attempts, r.now().UTC().Add(personalStarCleanupBackoff(attempts)), cause.Error(),
	)
}

func personalStarCleanupBackoff(attempts int) time.Duration {
	delay := personalStarCleanupBaseBackoff
	for index := 1; index < attempts; index++ {
		delay *= 2
		if delay >= personalStarCleanupMaxBackoff {
			return personalStarCleanupMaxBackoff
		}
	}
	return delay
}
