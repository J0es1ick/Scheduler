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

	"github.com/J0es1ick/Scheduler/internal/admin"
	"github.com/J0es1ick/Scheduler/internal/config"
	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/scrapper/ispu"
	"github.com/J0es1ick/Scheduler/internal/scrapper/isuct"
	"github.com/J0es1ick/Scheduler/internal/service"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.InitConfig()
	if err != nil {
		logger.Error("admin config init failed", "err", err)
		os.Exit(1)
	}
	db, err := database.NewDatabase(cfg)
	if err != nil {
		logger.Error("admin database connect failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err = database.ApplyMigrations(ctx, db.DB); err != nil {
		cancel()
		logger.Error("admin database migrations failed", "err", err)
		os.Exit(1)
	}
	cancel()

	groupRepo := repository.NewGroupRepository(db.DB)
	lessonRepo := repository.NewLessonRepository(db.DB)
	semesterRepo := repository.NewSemesterRepository(db.DB)
	scheduleService := service.NewScheduleService(lessonRepo, semesterRepo, groupRepo)
	parserService := service.NewParserService(
		repository.NewDataSourceRepository(db.DB),
		repository.NewParseLogRepository(db.DB),
		groupRepo,
		scheduleService,
		service.NewSemesterService(semesterRepo),
		repository.NewNotificationRepository(db.DB),
	)
	parserService.RegisterAdapter(isuct.UniversityID, isuct.New(""))
	parserService.RegisterAdapter(ispu.UniversityID, ispu.New(""))

	store := admin.NewStore(db.DB)
	auth := admin.NewAuthManager(cfg.BotToken, cfg.Admin.AccessToken, cfg.Admin.PublicURL)
	adminServer, err := admin.NewServer(store, auth, parserService)
	if err != nil {
		logger.Error("admin server init failed", "err", err)
		os.Exit(1)
	}
	if !auth.AccessKeyEnabled() {
		logger.Warn("ADMIN_ACCESS_TOKEN is empty; standalone access-key login is disabled")
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.Admin.Port,
		Handler:           adminServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("admin server started", "address", httpServer.Addr, "public_url", cfg.Admin.PublicURL)
		serverErrors <- httpServer.ListenAndServe()
	}()

	select {
	case <-rootCtx.Done():
		logger.Info("admin shutdown signal received")
	case err = <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("admin server failed", "err", err)
			os.Exit(1)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err = httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("admin graceful shutdown failed", "err", err)
	}
}
