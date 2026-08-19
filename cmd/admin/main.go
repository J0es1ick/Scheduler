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

	"github.com/J0es1ick/Scheduler/integrations/ivgpu"
	"github.com/J0es1ick/Scheduler/internal/admin"
	"github.com/J0es1ick/Scheduler/internal/buildinfo"
	"github.com/J0es1ick/Scheduler/internal/config"
	"github.com/J0es1ick/Scheduler/internal/connectorapi"
	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/declarative"
	"github.com/J0es1ick/Scheduler/internal/logging"
	"github.com/J0es1ick/Scheduler/internal/managedparser"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/scraper/ispu"
	"github.com/J0es1ick/Scheduler/internal/scraper/isuct"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/worker"
	managed "github.com/J0es1ick/Scheduler/parser/v1"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	logger.Info("starting scheduler admin", "build", buildinfo.Values())

	cfg, err := config.InitAdminConfig()
	if err != nil {
		logger.Error("admin config init failed", "err", err)
		os.Exit(1)
	}
	logger = logging.NewJSONLogger(
		os.Stdout,
		slog.LevelInfo,
		cfg.BotToken,
		cfg.Database.Password,
		cfg.Admin.AccessToken,
		cfg.Admin.MetricsToken,
	)
	slog.SetDefault(logger)
	db, err := database.NewDatabase(cfg)
	if err != nil {
		logger.Error("admin database connect failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err = database.VerifyMigrations(ctx, db.DB); err != nil {
		cancel()
		logger.Error("admin database schema verification failed", "err", err)
		os.Exit(1)
	}
	cancel()

	groupRepo := repository.NewGroupRepository(db.DB)
	lessonRepo := repository.NewLessonRepository(db.DB)
	semesterRepo := repository.NewSemesterRepository(db.DB)
	snapshotRepo := repository.NewParserSnapshotRepository(db.DB)
	reconciliationCtx, reconciliationCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err = snapshotRepo.EnsureNoPendingPublicationReconciliations(reconciliationCtx); err != nil {
		reconciliationCancel()
		logger.Error("publication reconciliation verification failed", "err", err)
		os.Exit(1)
	}
	reconciliationCancel()
	scheduleService := service.NewScheduleService(lessonRepo, semesterRepo, groupRepo)
	parserService := service.NewParserService(
		repository.NewDataSourceRepository(db.DB),
		repository.NewParseLogRepository(db.DB),
		groupRepo,
		scheduleService,
		snapshotRepo,
		repository.NewNotificationRepository(db.DB),
		repository.NewParserDiagnosticRepository(db.DB),
	)
	parserService.RegisterAdapter(isuct.UniversityID, isuct.New(""))
	parserService.RegisterAdapter(ispu.UniversityID, ispu.New(""))
	parserService.RegisterAdapterFactory("managed:"+ivgpu.ParserID, managedparser.Factory(ivgpu.New))
	parserService.RegisterAdapterFactory(declarative.AdapterType, declarative.AdapterFactory)
	connectorRepository := repository.NewConnectorRepository(db.DB)
	connectorService := connectorapi.NewService(connectorRepository, parserService)
	connectorServer := connectorapi.NewServer(connectorRepository)

	store := admin.NewStore(db.DB)
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = store.PurgeAccessKeySessions(cleanupCtx)
	cleanupCancel()
	if err != nil {
		logger.Error("admin access-key session cleanup failed", "err", err)
		os.Exit(1)
	}
	auth := admin.NewAuthManager(
		cfg.BotToken,
		cfg.Admin.AccessToken,
		cfg.Admin.AccessKeyLoginEnabled,
		cfg.Admin.CookieSecure,
	)
	auth.UseSessionStore(store)
	workerMonitor := worker.NewMonitor()
	workerMonitor.Register(worker.ConnectorWorkerName, 35*time.Minute)
	adminServer, err := admin.NewServer(store, auth, parserService, admin.ServerOptions{
		MetricsToken:      cfg.Admin.MetricsToken,
		TrustedProxyCIDRs: cfg.Admin.TrustedProxyCIDRs,
		ConnectorHandler:  connectorServer.Handler(),
		ManagedParsers:    []managed.Manifest{ivgpu.Manifest()},
		WorkerReadiness:   workerMonitor,
	})
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
	adminServer.UseBackgroundContext(rootCtx)
	connectorDone := worker.NewConnectorWorker(connectorService, 2*time.Second).Start(rootCtx, workerMonitor)
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
	stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err = httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("admin graceful shutdown failed", "err", err)
	}
	if err = adminServer.WaitBackground(shutdownCtx); err != nil {
		logger.Warn("admin background work did not stop before deadline", "err", err)
	}
	select {
	case <-connectorDone:
	case <-shutdownCtx.Done():
		logger.Warn("connector worker did not stop before deadline", "err", shutdownCtx.Err())
	}
}
