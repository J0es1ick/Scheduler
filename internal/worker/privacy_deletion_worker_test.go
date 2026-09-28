package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

type privacyQueueStub struct {
	batches      [][]domain.PrivacyDeletionRequest
	claimErrs    []error
	renewErrs    []error
	completeErrs []error
	existing     []string
	reconcileErr error
	completed    int
	retried      int
	renewed      int
}

func (s *privacyQueueStub) ExistingRequestIDs(context.Context, []string) ([]string, error) {
	return s.existing, s.reconcileErr
}

func (s *privacyQueueStub) Renew(context.Context, string, string) error {
	s.renewed++
	if len(s.renewErrs) != 0 {
		err := s.renewErrs[0]
		s.renewErrs = s.renewErrs[1:]
		return err
	}
	return nil
}

func (s *privacyQueueStub) ClaimPending(context.Context, int) ([]domain.PrivacyDeletionRequest, error) {
	if len(s.claimErrs) != 0 {
		err := s.claimErrs[0]
		s.claimErrs = s.claimErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	if len(s.batches) == 0 {
		return []domain.PrivacyDeletionRequest{}, nil
	}
	batch := s.batches[0]
	s.batches = s.batches[1:]
	return batch, nil
}

func (s *privacyQueueStub) Complete(context.Context, string, string) error {
	s.completed++
	if len(s.completeErrs) == 0 {
		return nil
	}
	err := s.completeErrs[0]
	s.completeErrs = s.completeErrs[1:]
	return err
}

func (s *privacyQueueStub) Retry(context.Context, string, string, error) error {
	s.retried++
	return nil
}

func TestPrivacyDeletionWorkerCompletesAfterCrashBoundaryRetry(t *testing.T) {
	request := domain.PrivacyDeletionRequest{ID: "request", UserID: "user", ClaimToken: "claim"}
	queue := &privacyQueueStub{
		batches:      [][]domain.PrivacyDeletionRequest{{request}, {request}},
		completeErrs: []error{errors.New("connection lost"), nil},
	}
	worker := NewPrivacyDeletionWorker(queue, 0, 1)
	monitor := NewMonitor()
	monitor.Register(PrivacyDeletionWorkerName, 0)

	worker.tick(context.Background(), monitor)
	worker.tick(context.Background(), monitor)

	if queue.completed != 2 || queue.retried != 1 {
		t.Fatalf("completed=%d retried=%d", queue.completed, queue.retried)
	}
}

func TestPrivacyDeletionWorkerRetriesDeletionFailure(t *testing.T) {
	request := domain.PrivacyDeletionRequest{ID: "request", UserID: "user", ClaimToken: "claim"}
	queue := &privacyQueueStub{batches: [][]domain.PrivacyDeletionRequest{{request}}, completeErrs: []error{errors.New("database unavailable")}}
	worker := NewPrivacyDeletionWorker(queue, 0, 1)

	worker.tick(context.Background(), nil)

	if queue.retried != 1 || queue.completed != 1 {
		t.Fatalf("completed=%d retried=%d", queue.completed, queue.retried)
	}
}

func TestPrivacyDeletionWorkerKeepsFailedHealthUntilSameRequestSucceeds(t *testing.T) {
	failedRequest := domain.PrivacyDeletionRequest{ID: "failed", UserID: "failed-user", ClaimToken: "failed-claim"}
	successfulRequest := domain.PrivacyDeletionRequest{ID: "successful", UserID: "successful-user", ClaimToken: "successful-claim"}
	queue := &privacyQueueStub{batches: [][]domain.PrivacyDeletionRequest{
		{failedRequest, successfulRequest},
		{},
		{successfulRequest},
		{failedRequest},
	}}
	queue.completeErrs = []error{
		errors.New("database unavailable"), nil, nil, nil,
	}
	queue.existing = []string{failedRequest.ID}
	worker := NewPrivacyDeletionWorker(queue, 0, 2)
	monitor := NewMonitor()
	monitor.Register(PrivacyDeletionWorkerName, time.Minute)
	monitor.Started(PrivacyDeletionWorkerName)

	worker.tick(context.Background(), monitor)
	assertPrivacyWorkerHealth(t, monitor, false)
	worker.tick(context.Background(), monitor)
	assertPrivacyWorkerHealth(t, monitor, false)
	worker.tick(context.Background(), monitor)
	assertPrivacyWorkerHealth(t, monitor, false)
	worker.tick(context.Background(), monitor)
	assertPrivacyWorkerHealth(t, monitor, true)
}

func TestPrivacyDeletionWorkerRecoversWhenFailedRequestDisappears(t *testing.T) {
	for _, renewFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "lost commit acknowledgement", true: "completed by another worker"}[renewFailure], func(t *testing.T) {
			request := domain.PrivacyDeletionRequest{ID: "request", UserID: "user", ClaimToken: "claim"}
			queue := &privacyQueueStub{batches: [][]domain.PrivacyDeletionRequest{{request}}}
			if renewFailure {
				queue.renewErrs = []error{errors.New("claim lost")}
			} else {
				queue.completeErrs = []error{errors.New("commit acknowledgement lost")}
			}
			worker := NewPrivacyDeletionWorker(queue, 0, 1)
			monitor := NewMonitor()
			monitor.Register(PrivacyDeletionWorkerName, time.Minute)
			monitor.Started(PrivacyDeletionWorkerName)
			worker.tick(context.Background(), monitor)
			assertPrivacyWorkerHealth(t, monitor, false)
			worker.tick(context.Background(), monitor)
			assertPrivacyWorkerHealth(t, monitor, true)
			if len(worker.failed) != 0 {
				t.Fatal("resolved failures retained")
			}
		})
	}
}

func TestPrivacyDeletionWorkerDoesNotHideFailureWhenReconciliationFails(t *testing.T) {
	request := domain.PrivacyDeletionRequest{ID: "request", UserID: "user", ClaimToken: "claim"}
	queue := &privacyQueueStub{
		batches:      [][]domain.PrivacyDeletionRequest{{request}},
		completeErrs: []error{errors.New("commit acknowledgement lost")},
		reconcileErr: errors.New("database unavailable"),
	}
	worker := NewPrivacyDeletionWorker(queue, 0, 1)
	monitor := NewMonitor()
	monitor.Register(PrivacyDeletionWorkerName, time.Minute)
	monitor.Started(PrivacyDeletionWorkerName)
	worker.tick(context.Background(), monitor)
	worker.tick(context.Background(), monitor)
	assertPrivacyWorkerHealth(t, monitor, false)
	if len(worker.failed) != 1 {
		t.Fatal("unverified failure was discarded")
	}
	queue.reconcileErr = nil
	worker.tick(context.Background(), monitor)
	assertPrivacyWorkerHealth(t, monitor, true)
}

func TestPrivacyDeletionWorkerRecoversFromClaimFailureAfterCleanClaim(t *testing.T) {
	queue := &privacyQueueStub{
		batches:   [][]domain.PrivacyDeletionRequest{{}},
		claimErrs: []error{errors.New("database unavailable"), nil},
	}
	worker := NewPrivacyDeletionWorker(queue, 0, 1)
	monitor := NewMonitor()
	monitor.Register(PrivacyDeletionWorkerName, time.Minute)
	monitor.Started(PrivacyDeletionWorkerName)

	worker.tick(context.Background(), monitor)
	assertPrivacyWorkerHealth(t, monitor, false)
	worker.tick(context.Background(), monitor)
	assertPrivacyWorkerHealth(t, monitor, true)
}

func assertPrivacyWorkerHealth(t *testing.T, monitor *Monitor, want bool) {
	t.Helper()
	if got := monitor.Checks()[PrivacyDeletionWorkerName]; got != want {
		t.Fatalf("privacy worker health = %t, want %t", got, want)
	}
}
