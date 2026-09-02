package messagequeue

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type admissionProbeRepository struct {
	Repository
	listCalled        chan struct{}
	restoreCalled     chan struct{}
	acknowledgeCalled chan struct{}
	setPendingCalled  chan struct{}
	getPendingCalled  chan struct{}
	takePendingCalled chan struct{}
}

func (r *admissionProbeRepository) ListBySession(ctx context.Context, sessionID string) ([]QueuedMessage, error) {
	select {
	case r.listCalled <- struct{}{}:
	default:
	}
	return r.Repository.ListBySession(ctx, sessionID)
}

func (r *admissionProbeRepository) RestoreSendNowClaim(ctx context.Context, claim *SendNowClaim) error {
	select {
	case r.restoreCalled <- struct{}{}:
	default:
	}
	return r.Repository.RestoreSendNowClaim(ctx, claim)
}

func (r *admissionProbeRepository) AcknowledgeSendNowClaim(ctx context.Context, claim *SendNowClaim) error {
	select {
	case r.acknowledgeCalled <- struct{}{}:
	default:
	}
	return r.Repository.AcknowledgeSendNowClaim(ctx, claim)
}

func (r *admissionProbeRepository) SetPendingMove(ctx context.Context, sessionID string, move *PendingMove) error {
	select {
	case r.setPendingCalled <- struct{}{}:
	default:
	}
	return r.Repository.SetPendingMove(ctx, sessionID, move)
}

func (r *admissionProbeRepository) GetPendingMove(ctx context.Context, sessionID string) (*PendingMove, error) {
	select {
	case r.getPendingCalled <- struct{}{}:
	default:
	}
	return r.Repository.GetPendingMove(ctx, sessionID)
}

func (r *admissionProbeRepository) TakePendingMove(ctx context.Context, sessionID string) (*PendingMove, error) {
	select {
	case r.takePendingCalled <- struct{}{}:
	default:
	}
	return r.Repository.TakePendingMove(ctx, sessionID)
}

func newAdmissionProbeRepository() *admissionProbeRepository {
	return &admissionProbeRepository{
		Repository:        NewMemoryRepository(),
		listCalled:        make(chan struct{}, 1),
		restoreCalled:     make(chan struct{}, 1),
		acknowledgeCalled: make(chan struct{}, 1),
		setPendingCalled:  make(chan struct{}, 1),
		getPendingCalled:  make(chan struct{}, 1),
		takePendingCalled: make(chan struct{}, 1),
	}
}

func TestServiceQueueOperationsWaitForAdmission(t *testing.T) {
	tests := []struct {
		name    string
		probe   func(*admissionProbeRepository) <-chan struct{}
		prepare func(context.Context, *Service, *admissionProbeRepository) func(context.Context) error
	}{
		{
			name:  "get entry",
			probe: func(repo *admissionProbeRepository) <-chan struct{} { return repo.listCalled },
			prepare: func(ctx context.Context, svc *Service, _ *admissionProbeRepository) func(context.Context) error {
				entry, err := svc.QueueMessage(ctx, "session", "task", "content", "", QueuedByUser, false, nil)
				require.NoError(t, err)
				return func(operationCtx context.Context) error {
					_, err := svc.GetEntry(operationCtx, "session", entry.ID)
					return err
				}
			},
		},
		{
			name:  "restore send-now claim",
			probe: func(repo *admissionProbeRepository) <-chan struct{} { return repo.restoreCalled },
			prepare: func(ctx context.Context, svc *Service, _ *admissionProbeRepository) func(context.Context) error {
				entry, err := svc.QueueMessage(ctx, "session", "task", "content", "", QueuedByUser, false, nil)
				require.NoError(t, err)
				claim, err := svc.ClaimSendNow(ctx, "session", []QueuedMessage{*entry})
				require.NoError(t, err)
				return func(operationCtx context.Context) error { return svc.RestoreSendNowClaim(operationCtx, claim) }
			},
		},
		{
			name:  "acknowledge send-now claim",
			probe: func(repo *admissionProbeRepository) <-chan struct{} { return repo.acknowledgeCalled },
			prepare: func(ctx context.Context, svc *Service, _ *admissionProbeRepository) func(context.Context) error {
				entry, err := svc.QueueMessage(ctx, "session", "task", "content", "", QueuedByUser, false, nil)
				require.NoError(t, err)
				claim, err := svc.ClaimSendNow(ctx, "session", []QueuedMessage{*entry})
				require.NoError(t, err)
				return func(operationCtx context.Context) error { return svc.AcknowledgeSendNowClaim(operationCtx, claim) }
			},
		},
		{
			name:  "set pending move",
			probe: func(repo *admissionProbeRepository) <-chan struct{} { return repo.setPendingCalled },
			prepare: func(_ context.Context, svc *Service, _ *admissionProbeRepository) func(context.Context) error {
				return func(operationCtx context.Context) error {
					svc.SetPendingMove(operationCtx, "session", &PendingMove{TaskID: "task"})
					return nil
				}
			},
		},
		{
			name:  "get pending move",
			probe: func(repo *admissionProbeRepository) <-chan struct{} { return repo.getPendingCalled },
			prepare: func(ctx context.Context, svc *Service, _ *admissionProbeRepository) func(context.Context) error {
				svc.SetPendingMove(ctx, "session", &PendingMove{TaskID: "task"})
				return func(operationCtx context.Context) error {
					_, _ = svc.GetPendingMove(operationCtx, "session")
					return nil
				}
			},
		},
		{
			name:  "take pending move",
			probe: func(repo *admissionProbeRepository) <-chan struct{} { return repo.takePendingCalled },
			prepare: func(ctx context.Context, svc *Service, _ *admissionProbeRepository) func(context.Context) error {
				svc.SetPendingMove(ctx, "session", &PendingMove{TaskID: "task"})
				return func(operationCtx context.Context) error {
					_, _ = svc.TakePendingMove(operationCtx, "session")
					return nil
				}
			},
		},
		{
			name:  "get status",
			probe: func(repo *admissionProbeRepository) <-chan struct{} { return repo.listCalled },
			prepare: func(_ context.Context, svc *Service, _ *admissionProbeRepository) func(context.Context) error {
				return func(operationCtx context.Context) error {
					_ = svc.GetStatus(operationCtx, "session")
					return nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newAdmissionProbeRepository()
			svc := newAutoMergeTestServiceWithRepository(t, repo, 10)
			svc.SetAutoMergeEnabled(false)
			ctx := context.Background()
			op := tt.prepare(ctx, svc, repo)

			holderStarted := make(chan struct{})
			releaseHolder := make(chan struct{})
			holderDone := make(chan error, 1)
			go func() {
				holderDone <- svc.WithSessionAdmission(ctx, "session", func(context.Context) error {
					close(holderStarted)
					<-releaseHolder
					return nil
				})
			}()
			<-holderStarted

			opDone := make(chan error, 1)
			go func() { opDone <- op(ctx) }()
			select {
			case <-tt.probe(repo):
				t.Fatal("queue operation reached repository while admission was held")
			case <-time.After(100 * time.Millisecond):
			}

			close(releaseHolder)
			require.NoError(t, <-holderDone)
			require.NoError(t, <-opDone)
			select {
			case <-tt.probe(repo):
			case <-time.After(time.Second):
				t.Fatal("queue operation did not reach repository after admission was released")
			}
		})
	}
}
