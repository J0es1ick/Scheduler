package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/J0es1ick/Scheduler/internal/buildinfo"
	"github.com/J0es1ick/Scheduler/internal/config"
	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/repository"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	logger.Info("starting scheduler migrator", "build", buildinfo.Values())

	cfg, err := config.InitSiteConfig()
	if err != nil {
		logger.Error("migrator config init failed", "err", err)
		os.Exit(1)
	}
	db, err := database.NewDatabase(cfg)
	if err != nil {
		logger.Error("migrator database connect failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(rootCtx, 45*time.Minute)
	defer cancel()
	if err = database.ApplyMigrations(ctx, db.DB); err != nil {
		logger.Error("database migrations failed", "err", err)
		os.Exit(1)
	}
	reconciled, err := repository.NewParserSnapshotRepository(db.DB).ReconcilePendingPublications(ctx)
	if err != nil {
		logger.Error("publication reconciliation failed", "err", err)
		os.Exit(1)
	}
	logger.Info("scheduler migrations and reconciliation completed", "restored_universities", reconciled)
}
