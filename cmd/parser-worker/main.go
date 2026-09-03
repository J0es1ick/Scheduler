package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/J0es1ick/Scheduler/internal/buildinfo"
	"github.com/J0es1ick/Scheduler/internal/config"
	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/logging"
	"github.com/J0es1ick/Scheduler/internal/parserruntime"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/worker"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const parserTickInterval = 5 * time.Minute

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("starting scheduler parser worker", "build", buildinfo.Values())
	cfg, err := config.InitWorkerConfig()
	if err != nil {
		slog.Error("parser worker config init failed", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logging.NewJSONLogger(os.Stdout, slog.LevelInfo, cfg.Database.Password))
	db, err := database.NewDatabase(cfg)
	if err != nil {
		slog.Error("parser worker database connect failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err = database.VerifyMigrations(verifyCtx, db.DB); err == nil {
		err = repository.NewParserSnapshotRepository(db.DB).EnsureNoPendingPublicationReconciliations(verifyCtx)
	}
	verifyCancel()
	if err != nil {
		slog.Error("parser worker database verification failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	monitor := worker.NewMonitor()
	monitor.Register(worker.ParserWorkerName, 35*time.Minute)
	parserDone := worker.NewParserWorker(parserruntime.New(db.DB), parserTickInterval).Start(ctx, monitor)
	server := &http.Server{
		Addr:              ":" + cfg.WorkerHealthPort,
		Handler:           worker.HealthHandler(db.DB, monitor),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
	case err = <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("parser worker health server failed", "err", err)
		}
		cancel()
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err = server.Shutdown(shutdownCtx); err != nil {
		slog.Error("parser worker health shutdown failed", "err", err)
	}
	select {
	case <-parserDone:
	case <-shutdownCtx.Done():
		slog.Warn("parser worker did not stop before deadline", "err", shutdownCtx.Err())
	}
}
