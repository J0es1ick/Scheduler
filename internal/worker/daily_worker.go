package worker

import (
	"context"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

const DailyWorkerName = "daily_schedule"

type dailyRepository interface {
	EnqueueDue(context.Context, time.Time, int) (int, error)
}

type DailyWorker struct {
	repository dailyRepository
	status     workerStatusRepository
	now        func() time.Time
}

func NewDailyWorker(repository dailyRepository, status workerStatusRepository) *DailyWorker {
	return &DailyWorker{repository: repository, status: status, now: time.Now}
}

func (w *DailyWorker) Start(ctx context.Context, monitor *Monitor) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitor.Started(DailyWorkerName)
		defer monitor.Stopped(DailyWorkerName)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			monitor.Record(DailyWorkerName, w.tick(ctx))
			monitor.Heartbeat(DailyWorkerName)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

func (w *DailyWorker) tick(parent context.Context) error {
	started := w.now()
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	processed := 0
	var runErr error
	for ctx.Err() == nil {
		count, err := w.repository.EnqueueDue(ctx, started, 250)
		processed += count
		if err != nil {
			runErr = err
			break
		}
		if count < 250 {
			break
		}
	}
	if runErr == nil {
		runErr = ctx.Err()
	}
	finished := w.now()
	failures := 0
	if runErr != nil {
		failures = 1
	}
	recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(parent), workerStatusTimeout)
	defer recordCancel()
	err := w.status.RecordRun(recordCtx, domain.WorkerRunResult{Name: DailyWorkerName, StartedAt: started, FinishedAt: finished, LastFullCycleAt: fullCycleTimestamp(runErr == nil, finished), Processed: processed, Failures: failures, LastError: compactWorkerError(runErr)})
	if runErr != nil {
		return runErr
	}
	return err
}
