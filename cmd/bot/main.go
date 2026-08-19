package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/J0es1ick/Scheduler/integrations/ivgpu"
	"github.com/J0es1ick/Scheduler/internal/buildinfo"
	"github.com/J0es1ick/Scheduler/internal/config"
	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/declarative"
	"github.com/J0es1ick/Scheduler/internal/logging"
	"github.com/J0es1ick/Scheduler/internal/managedparser"
	"github.com/J0es1ick/Scheduler/internal/miniapp"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/scraper/ispu"
	"github.com/J0es1ick/Scheduler/internal/scraper/isuct"
	"github.com/J0es1ick/Scheduler/internal/service"
	botpkg "github.com/J0es1ick/Scheduler/internal/telegram-bot"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/handlers"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	"github.com/J0es1ick/Scheduler/internal/worker"
	_ "github.com/jackc/pgx/v5/stdlib"
	tgbotapi "gopkg.in/telebot.v3"
)

// parserTickInterval — как часто воркер проверяет активные источники данных.
// Реальный интервал обновления каждого источника хранится в data_sources.update_interval.
const parserTickInterval = 5 * time.Minute

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("starting scheduler bot", "build", buildinfo.Values())

	cfg, err := config.InitConfig()
	if err != nil {
		slog.Error("config init failed", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logging.NewJSONLogger(
		os.Stdout,
		slog.LevelInfo,
		cfg.BotToken,
		cfg.Database.Password,
		cfg.Admin.AccessToken,
		cfg.Admin.MetricsToken,
	))
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	db, err := database.NewDatabase(cfg)
	if err != nil {
		slog.Error("database connect failed", "err", err)
		os.Exit(1)
	}
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := database.VerifyMigrations(verifyCtx, db.DB); err != nil {
		verifyCancel()
		slog.Error("database schema verification failed", "err", err)
		os.Exit(1)
	}
	verifyCancel()
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("db close failed", "err", err)
		}
	}()

	bot, err := tgbotapi.NewBot(tgbotapi.Settings{
		URL:     cfg.BotTelegramAPIURL,
		Token:   cfg.BotToken,
		OnError: botpkg.HandleError,
		Client: &http.Client{
			Timeout: 25 * time.Second,
		},
	})
	if err != nil {
		slog.Error("telegram bot init failed", "err", err)
		os.Exit(1)
	}
	botUsername, err := botpkg.ConfigureIdentity(bot, cfg.BotUsername, cfg.BotPublicURL)
	if err != nil {
		slog.Error("telegram bot identity configuration failed", "err", err)
		os.Exit(1)
	}
	slog.Info("telegram bot identity configured", "username", botUsername)

	// --- Репозитории ---
	userRepo := repository.NewUserRepository(db.DB)
	lessonRepo := repository.NewLessonRepository(db.DB)
	semesterRepo := repository.NewSemesterRepository(db.DB)
	groupRepo := repository.NewGroupRepository(db.DB)
	universityRepo := repository.NewUniversityRepository(db.DB)
	subscriptionRepo := repository.NewSubscriptionRepository(db.DB)
	supportRequestRepo := repository.NewSupportRequestRepository(db.DB)
	metricsRepo := repository.NewMetricsRepository(db.DB)
	chatProfileRepo := repository.NewChatProfileRepository(db.DB)
	reminderRepo := repository.NewReminderRepository(db.DB)
	workerStatusRepo := repository.NewWorkerStatusRepository(db.DB)
	dataSourceRepo := repository.NewDataSourceRepository(db.DB)
	parseLogRepo := repository.NewParseLogRepository(db.DB)
	notificationRepo := repository.NewNotificationRepository(db.DB)
	snapshotRepo := repository.NewParserSnapshotRepository(db.DB)
	diagnosticRepo := repository.NewParserDiagnosticRepository(db.DB)
	reconciliationCtx, reconciliationCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err = snapshotRepo.EnsureNoPendingPublicationReconciliations(reconciliationCtx); err != nil {
		reconciliationCancel()
		slog.Error("publication reconciliation verification failed", "err", err)
		os.Exit(1)
	}
	reconciliationCancel()

	// --- Сервисы ---
	scheduleService := service.NewScheduleService(lessonRepo, semesterRepo, groupRepo)
	userService := service.NewUserService(userRepo)
	subscriptionService := service.NewSubscriptionService(subscriptionRepo)
	supportRequestService := service.NewSupportRequestService(supportRequestRepo)
	metricsService := service.NewMetricsService(metricsRepo)
	chatProfileService := service.NewChatProfileService(chatProfileRepo)
	groupService := service.NewGroupService(groupRepo)
	universityService := service.NewUniversityService(universityRepo)
	parserService := service.NewParserService(
		dataSourceRepo, parseLogRepo, groupRepo, scheduleService,
		snapshotRepo, notificationRepo, diagnosticRepo,
	)

	// --- Адаптеры ---
	// semesterID не задаём при старте: ParserService резолвит его динамически
	// перед каждым запуском через SemesterService.GetCurrentSemester.
	isuctAdapter := isuct.New("")
	parserService.RegisterAdapter(isuct.UniversityID, isuctAdapter)
	slog.Info("adapter registered", "type", isuct.UniversityID)
	ispuAdapter := ispu.New("")
	parserService.RegisterAdapter(ispu.UniversityID, ispuAdapter)
	slog.Info("adapter registered", "type", ispu.UniversityID)
	parserService.RegisterAdapterFactory("managed:"+ivgpu.ParserID, managedparser.Factory(ivgpu.New))
	parserService.RegisterAdapterFactory(declarative.AdapterType, declarative.AdapterFactory)
	slog.Info("managed parser registered", "type", ivgpu.ParserID, "contract", ivgpu.Manifest().ContractVersion)

	// --- Telegram ---
	stateManager := state.NewManagerWithTTL(time.Duration(cfg.BotStateTTLMinutes) * time.Minute)
	stateCleanupDone := stateManager.StartCleanup(ctx, time.Minute)
	handler := handlers.NewHandler(
		scheduleService, userService, groupService,
		universityService, stateManager, subscriptionService, supportRequestService,
		metricsService, chatProfileService, cfg.Admin.PublicURL, cfg.ProjectURL,
	)
	handlerTracker := botpkg.NewHandlerTracker()
	bot.Use(
		botpkg.RecoverPanics(),
		handlerTracker.Middleware(),
		botpkg.SerializeBySender(cfg.BotMaxPendingPerSender),
		botpkg.LimitConcurrent(cfg.BotMaxConcurrentHandlers),
	)
	commandsReady := botpkg.Register(ctx, bot, handler)

	workerMonitor := worker.NewMonitor()
	workerMonitor.Register(worker.ParserWorkerName, 35*time.Minute)
	workerMonitor.Register(worker.ReminderWorkerName, 2*time.Minute)
	workerMonitor.Register(worker.NotificationWorkerName, 2*time.Minute)
	health := botpkg.NewHealth(db.DB, workerMonitor)
	healthListener, err := net.Listen("tcp", ":"+cfg.BotHealthPort)
	if err != nil {
		slog.Error("bot health listener failed", "port", cfg.BotHealthPort, "err", err)
		os.Exit(1)
	}
	healthServer := &http.Server{
		Handler:           health.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	healthErrors := make(chan error, 1)
	go func() {
		slog.Info("bot health server started", "port", cfg.BotHealthPort)
		if serveErr := healthServer.Serve(healthListener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			healthErrors <- serveErr
		}
	}()
	go func() {
		select {
		case <-commandsReady:
			health.SetCommandsConfigured(true)
		case <-ctx.Done():
		}
	}()

	// --- Фоновый воркер парсера ---
	// Запускается после регистрации адаптеров, до старта бота.
	// Останавливается вместе с ctx при получении сигнала.
	parserWorker := worker.NewParserWorker(parserService, parserTickInterval)
	parserDone := parserWorker.Start(ctx, workerMonitor)
	reminderWorker := worker.NewReminderWorker(
		reminderRepo,
		workerStatusRepo,
		scheduleService,
		30*time.Second,
	)
	reminderDone := reminderWorker.Start(ctx, workerMonitor)
	notificationWorker := worker.NewNotificationWorker(notificationRepo, bot, 15*time.Second)
	notificationDone := notificationWorker.Start(ctx, workerMonitor)
	go keepAdminMenusConfigured(ctx, bot, userRepo, cfg.Admin.PublicURL)

	// --- Бот ---
	health.SetPolling(true)
	go func() {
		slog.Info("bot started")
		bot.Start()
		health.SetPolling(false)
		if ctx.Err() == nil {
			slog.Error("Telegram polling stopped unexpectedly")
			cancel()
		}
	}()

	select {
	case <-ctx.Done():
	case healthErr := <-healthErrors:
		slog.Error("bot health server stopped unexpectedly", "err", healthErr)
		cancel()
	}
	slog.Info("shutdown signal received, stopping...")
	health.SetPolling(false)
	handlerTracker.StopAccepting()
	bot.Stop()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := handlerTracker.Wait(shutdownCtx); err != nil {
		slog.Warn("Telegram handlers did not stop before deadline", "err", err)
	}
	if err := waitForBackgroundTasks(shutdownCtx, parserDone, reminderDone, notificationDone, stateCleanupDone); err != nil {
		slog.Warn("background tasks did not stop before deadline", "err", err)
	}
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("bot health server shutdown failed", "err", err)
	}
	slog.Info("bot stopped")
}

func waitForBackgroundTasks(ctx context.Context, tasks ...<-chan struct{}) error {
	for _, done := range tasks {
		if done == nil {
			continue
		}
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func keepAdminMenusConfigured(
	ctx context.Context,
	bot *tgbotapi.Bot,
	users *repository.UserRepository,
	publicURL string,
) {
	const syncInterval = 30 * time.Second
	if _, err := miniapp.EditorURL(publicURL); err != nil {
		slog.Warn("Mini App menu is disabled until ADMIN_PUBLIC_URL is a public HTTPS URL")
		return
	}
	firstSync := true
	for {
		configured := configureAdminMenus(ctx, bot, users, publicURL)
		if configured {
			if firstSync {
				slog.Info("Mini App menu configured for administrators")
				firstSync = false
			}
		} else {
			slog.Warn("Telegram API is unavailable; Mini App menu configuration will be retried")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(syncInterval):
		}
	}
}

func configureAdminMenus(
	parent context.Context,
	bot *tgbotapi.Bot,
	users *repository.UserRepository,
	publicURL string,
) bool {
	const pageSize = 200
	const requestInterval = 50 * time.Millisecond
	regularFingerprint := miniapp.MenuFingerprint(publicURL, false)
	adminFingerprint := miniapp.MenuFingerprint(publicURL, true)
	afterID := ""
	lastRequest := time.Time{}
	for {
		pageCtx, cancel := context.WithTimeout(parent, 5*time.Second)
		items, err := users.GetUsersPendingMenuSync(
			pageCtx, afterID, pageSize, adminFingerprint, regularFingerprint,
		)
		cancel()
		if err != nil {
			slog.Warn("load users page for Mini App menu failed", "after_user_id", afterID, "err", err)
			return false
		}
		for _, user := range items {
			telegramID, parseErr := strconv.ParseInt(user.ID, 10, 64)
			if parseErr != nil {
				continue
			}
			if err = waitUntil(parent, lastRequest.Add(requestInterval)); err != nil {
				return false
			}
			lastRequest = time.Now()
			configureErr := miniapp.ConfigureMenu(
				bot,
				&tgbotapi.User{ID: telegramID},
				publicURL,
				user.IsAdmin,
			)
			var flood tgbotapi.FloodError
			if errors.As(configureErr, &flood) {
				if err = waitUntil(parent, time.Now().Add(time.Duration(flood.RetryAfter+1)*time.Second)); err != nil {
					return false
				}
				lastRequest = time.Now()
				configureErr = miniapp.ConfigureMenu(
					bot,
					&tgbotapi.User{ID: telegramID},
					publicURL,
					user.IsAdmin,
				)
			}
			if configureErr != nil {
				if !permanentTelegramMenuError(configureErr) {
					slog.Warn("Mini App menu configuration temporarily failed", "user_id", user.ID, "err", configureErr)
					return false
				}
				slog.Debug("Mini App menu cannot be configured for user", "user_id", user.ID, "err", configureErr)
			}
			fingerprint := regularFingerprint
			if user.IsAdmin {
				fingerprint = adminFingerprint
			}
			markCtx, markCancel := context.WithTimeout(parent, 5*time.Second)
			markErr := users.MarkMenuConfigured(markCtx, user.ID, fingerprint)
			markCancel()
			if markErr != nil {
				slog.Warn("save Mini App menu state failed", "user_id", user.ID, "err", markErr)
				return false
			}
		}
		if len(items) < pageSize {
			return true
		}
		afterID = items[len(items)-1].ID
	}
}

func permanentTelegramMenuError(err error) bool {
	if err == nil {
		return false
	}
	var telegramError *tgbotapi.Error
	if errors.As(err, &telegramError) {
		return telegramError.Code == 400 || telegramError.Code == 403 || telegramError.Code == 404
	}
	message := err.Error()
	return strings.HasSuffix(message, "(400)") ||
		strings.HasSuffix(message, "(403)") ||
		strings.HasSuffix(message, "(404)")
}

func waitUntil(ctx context.Context, deadline time.Time) error {
	delay := time.Until(deadline)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
