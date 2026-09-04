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
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/worker"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("starting scheduler privacy worker", "build", buildinfo.Values())
	cfg, err := config.InitWorkerConfig()
	if err != nil {
		slog.Error("privacy worker config init failed", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logging.NewJSONLogger(os.Stdout, slog.LevelInfo, cfg.Database.Password))
	db, err := database.NewDatabase(cfg)
	if err != nil {
		slog.Error("privacy worker database connect failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = database.VerifyMigrations(verifyCtx, db.DB)
	verifyCancel()
	if err != nil {
		slog.Error("privacy worker database verification failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	monitor := worker.NewMonitor()
	monitor.Register(worker.PrivacyDeletionWorkerName, 30*time.Second)
	privacyDone := worker.NewPrivacyDeletionWorker(
		repository.NewPrivacyDeletionRepository(db.DB),
		time.Second,
		25,
	).Start(ctx, monitor)
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
			slog.Error("privacy worker health server failed", "err", err)
		}
		cancel()
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err = server.Shutdown(shutdownCtx); err != nil {
		slog.Error("privacy worker health shutdown failed", "err", err)
	}
	select {
	case <-privacyDone:
	case <-shutdownCtx.Done():
		slog.Warn("privacy worker did not stop before deadline", "err", shutdownCtx.Err())
	}
}
