package worker

import (
	"context"
	"database/sql"
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
	completed    int
	retried      int
	renewed      int
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

type privacyUserDeleterStub struct {
	errs  []error
	calls int
}

func (s *privacyUserDeleterStub) DeleteUser(context.Context, string) error {
	s.calls++
	if len(s.errs) == 0 {
		return nil
	}
	err := s.errs[0]
	s.errs = s.errs[1:]
	return err
}

func TestPrivacyDeletionWorkerCompletesAfterCrashBoundaryRetry(t *testing.T) {
	request := domain.PrivacyDeletionRequest{ID: "request", UserID: "user", ClaimToken: "claim"}
	queue := &privacyQueueStub{
		batches:      [][]domain.PrivacyDeletionRequest{{request}, {request}},
		completeErrs: []error{errors.New("connection lost"), nil},
	}
	users := &privacyUserDeleterStub{errs: []error{nil, sql.ErrNoRows}}
	worker := NewPrivacyDeletionWorker(queue, users, 0, 1)
	monitor := NewMonitor()
	monitor.Register(PrivacyDeletionWorkerName, 0)

	worker.tick(context.Background(), monitor)
	worker.tick(context.Background(), monitor)

	if users.calls != 2 {
		t.Fatalf("delete calls = %d, want 2", users.calls)
	}
	if queue.completed != 2 || queue.retried != 0 {
		t.Fatalf("completed=%d retried=%d", queue.completed, queue.retried)
	}
}

func TestPrivacyDeletionWorkerRetriesDeletionFailure(t *testing.T) {
	request := domain.PrivacyDeletionRequest{ID: "request", UserID: "user", ClaimToken: "claim"}
	queue := &privacyQueueStub{batches: [][]domain.PrivacyDeletionRequest{{request}}}
	users := &privacyUserDeleterStub{errs: []error{errors.New("database unavailable")}}
	worker := NewPrivacyDeletionWorker(queue, users, 0, 1)

	worker.tick(context.Background(), nil)

	if queue.retried != 1 || queue.completed != 0 {
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
	users := &privacyUserDeleterStub{errs: []error{
		errors.New("database unavailable"), nil, nil, nil,
	}}
	worker := NewPrivacyDeletionWorker(queue, users, 0, 2)
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

func TestPrivacyDeletionWorkerRecoversFromClaimFailureAfterCleanClaim(t *testing.T) {
	queue := &privacyQueueStub{
		batches:   [][]domain.PrivacyDeletionRequest{{}},
		claimErrs: []error{errors.New("database unavailable"), nil},
	}
	worker := NewPrivacyDeletionWorker(queue, &privacyUserDeleterStub{}, 0, 1)
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
