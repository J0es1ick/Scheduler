package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/J0es1ick/Scheduler/internal/connectorapi"
)

type ConnectorWorker struct {
	service  *connectorapi.Service
	interval time.Duration
}

func NewConnectorWorker(service *connectorapi.Service, interval time.Duration) *ConnectorWorker {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &ConnectorWorker{service: service, interval: interval}
}

func (w *ConnectorWorker) Start(ctx context.Context, monitors ...*Monitor) <-chan struct{} {
	done := make(chan struct{})
	var monitor *Monitor
	if len(monitors) > 0 {
		monitor = monitors[0]
	}
	go func() {
		defer close(done)
		monitor.Started(ConnectorWorkerName)
		defer monitor.Stopped(ConnectorWorkerName)
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			processed, err := w.service.ProcessNext(ctx)
			monitor.Heartbeat(ConnectorWorkerName)
			if err != nil {
				slog.Error("connector ingestion failed", "err", err)
			}
			if processed {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}
