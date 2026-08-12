package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/J0es1ick/Scheduler/internal/service"
)

// ParserWorker периодически запускает парсинг всех активных источников данных.
type ParserWorker struct {
	parserSvc    *service.ParserService
	tickInterval time.Duration
}

// NewParserWorker создаёт воркер.
//
// tickInterval — интервал между проверками активных источников.
// Рекомендуется 1–5 минут; каждый источник всё равно не запустится чаще,
// чем его собственный update_interval.
func NewParserWorker(parserSvc *service.ParserService, tickInterval time.Duration) *ParserWorker {
	return &ParserWorker{
		parserSvc:    parserSvc,
		tickInterval: tickInterval,
	}
}

// Start запускает воркер в отдельной горутине и возвращает управление немедленно.
// Горутина завершается при отмене ctx.
func (w *ParserWorker) Start(ctx context.Context, monitors ...*Monitor) <-chan struct{} {
	done := make(chan struct{})
	var monitor *Monitor
	if len(monitors) > 0 {
		monitor = monitors[0]
	}
	go func() {
		defer close(done)
		monitor.Started(ParserWorkerName)
		defer monitor.Stopped(ParserWorkerName)
		w.run(ctx, monitor)
	}()
	return done
}

func (w *ParserWorker) run(ctx context.Context, monitor *Monitor) {
	slog.Info("parser worker started", "tick_interval", w.tickInterval)

	// Первый запуск — сразу при старте, не ждать первого тика.
	w.tick(ctx, monitor)

	ticker := time.NewTicker(w.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("parser worker stopped")
			return
		case <-ticker.C:
			w.tick(ctx, monitor)
		}
	}
}

// tick — один проход: находим источники, которым пришло время обновиться, и запускаем.
func (w *ParserWorker) tick(ctx context.Context, monitor *Monitor) {
	monitor.Heartbeat(ParserWorkerName)
	defer monitor.Heartbeat(ParserWorkerName)
	cleanupCtx, cleanupCancel := context.WithTimeout(ctx, time.Minute)
	cleanupErr := w.parserSvc.CleanupInterruptedRuns(cleanupCtx, 35*time.Minute)
	if cleanupErr != nil {
		slog.Error("parser worker: stale run cleanup failed", "err", cleanupErr)
	}
	cleanupCancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- w.parserSvc.RunAllActiveSources(ctx)
	}()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case err := <-runDone:
			if err != nil {
				slog.Error("parser worker: run active sources failed", "err", err)
			}
			monitor.Record(ParserWorkerName, errors.Join(cleanupErr, err))
			return
		case <-heartbeat.C:
			monitor.Heartbeat(ParserWorkerName)
		case <-ctx.Done():
			monitor.Record(ParserWorkerName, ctx.Err())
			return
		}
	}
}
