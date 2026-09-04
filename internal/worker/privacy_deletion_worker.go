package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

const PrivacyDeletionWorkerName = "privacy_deletion"

type privacyDeletionQueue interface {
	ClaimPending(context.Context, int) ([]domain.PrivacyDeletionRequest, error)
	Renew(context.Context, string, string) error
	Complete(context.Context, string, string) error
	Retry(context.Context, string, string, error) error
}

type PrivacyDeletionWorker struct {
	queue        privacyDeletionQueue
	pollInterval time.Duration
	batchSize    int
	failed       map[string]error
}

func NewPrivacyDeletionWorker(
	queue privacyDeletionQueue,
	pollInterval time.Duration,
	batchSize int,
) *PrivacyDeletionWorker {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	if batchSize <= 0 {
		batchSize = 25
	}
	return &PrivacyDeletionWorker{
		queue:        queue,
		pollInterval: pollInterval,
		batchSize:    batchSize,
		failed:       make(map[string]error),
	}
}

func (w *PrivacyDeletionWorker) Start(ctx context.Context, monitors ...*Monitor) <-chan struct{} {
	done := make(chan struct{})
	var monitor *Monitor
	if len(monitors) > 0 {
		monitor = monitors[0]
	}
	go func() {
		defer close(done)
		monitor.Started(PrivacyDeletionWorkerName)
		defer monitor.Stopped(PrivacyDeletionWorkerName)
		w.tick(ctx, monitor)
		ticker := time.NewTicker(w.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.tick(ctx, monitor)
			}
		}
	}()
	return done
}

func (w *PrivacyDeletionWorker) tick(ctx context.Context, monitor *Monitor) {
	monitor.Heartbeat(PrivacyDeletionWorkerName)
	requests, err := w.queue.ClaimPending(ctx, w.batchSize)
	if err != nil {
		monitor.Record(PrivacyDeletionWorkerName, err)
		slog.Error("privacy deletion claim failed", "err", err)
		return
	}
	cycleErrors := make([]error, 0)
	for _, request := range requests {
		if ctx.Err() != nil {
			w.failed[request.ID] = ctx.Err()
			cycleErrors = append(cycleErrors, ctx.Err())
			monitor.Record(PrivacyDeletionWorkerName, errors.Join(cycleErrors...))
			return
		}
		if err = w.queue.Renew(ctx, request.ID, request.ClaimToken); err != nil {
			w.failed[request.ID] = err
			cycleErrors = append(cycleErrors, err)
			continue
		}
		err = w.queue.Complete(ctx, request.ID, request.ClaimToken)
		if err != nil {
			if retryErr := w.queue.Retry(ctx, request.ID, request.ClaimToken, err); retryErr != nil {
				slog.Error("privacy deletion retry failed", "request_id", request.ID, "err", retryErr)
				err = errors.Join(err, retryErr)
			}
			w.failed[request.ID] = err
			cycleErrors = append(cycleErrors, err)
			continue
		}
		delete(w.failed, request.ID)
	}
	if len(cycleErrors) != 0 {
		monitor.Record(PrivacyDeletionWorkerName, errors.Join(cycleErrors...))
		return
	}
	if len(w.failed) != 0 {
		unresolved := make([]error, 0, len(w.failed))
		for _, failure := range w.failed {
			unresolved = append(unresolved, failure)
		}
		monitor.Record(PrivacyDeletionWorkerName, errors.Join(unresolved...))
		return
	}
	monitor.Succeeded(PrivacyDeletionWorkerName)
}
