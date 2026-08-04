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
	"syscall"
	"time"

	"github.com/J0es1ick/Scheduler/internal/config"
	"github.com/J0es1ick/Scheduler/internal/database"
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

	slog.Info("starting scheduler bot")

	cfg, err := config.InitConfig()
	if err != nil {
		slog.Error("config init failed", "err", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	db, err := database.NewDatabase(cfg)
	if err != nil {
		slog.Error("database connect failed", "err", err)
		os.Exit(1)
	}
	if err := database.ApplyMigrations(context.Background(), db.DB); err != nil {
		slog.Error("database migrations failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("db close failed", "err", err)
		}
	}()

	bot, err := tgbotapi.NewBot(tgbotapi.Settings{
		Token:   cfg.BotToken,
		Offline: true,
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
	dataSourceRepo := repository.NewDataSourceRepository(db.DB)
	parseLogRepo := repository.NewParseLogRepository(db.DB)
	notificationRepo := repository.NewNotificationRepository(db.DB)
	snapshotRepo := repository.NewParserSnapshotRepository(db.DB)
	diagnosticRepo := repository.NewParserDiagnosticRepository(db.DB)

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

	// --- Telegram ---
	stateManager := state.NewManagerWithTTL(time.Duration(cfg.BotStateTTLMinutes) * time.Minute)
	handler := handlers.NewHandler(
		scheduleService, userService, groupService,
		universityService, stateManager, subscriptionService, supportRequestService,
		metricsService, chatProfileService, cfg.Admin.PublicURL,
	)
	bot.Use(
		botpkg.RecoverPanics(),
		botpkg.SerializeBySender(cfg.BotMaxPendingPerSender),
		botpkg.LimitConcurrent(cfg.BotMaxConcurrentHandlers),
	)
	commandsReady := botpkg.Register(ctx, bot, handler)

	health := botpkg.NewHealth(db.DB)
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
	parserWorker.Start(ctx)
	reminderWorker := worker.NewReminderWorker(reminderRepo, scheduleService, 30*time.Second)
	reminderWorker.Start(ctx)
	notificationWorker := worker.NewNotificationWorker(notificationRepo, bot, 15*time.Second)
	notificationWorker.Start(ctx)
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
	bot.Stop()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("bot health server shutdown failed", "err", err)
	}
	shutdownCancel()
	slog.Info("bot stopped")
}

func keepAdminMenusConfigured(
	ctx context.Context,
	bot *tgbotapi.Bot,
	users *repository.UserRepository,
	publicURL string,
) {
	for {
		if configureAdminMenus(ctx, bot, users, publicURL) {
			slog.Info("Mini App menu configured for administrators")
			return
		}
		slog.Warn("Telegram API is unavailable; Mini App menu configuration will be retried")
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
}

func configureAdminMenus(
	parent context.Context,
	bot *tgbotapi.Bot,
	users *repository.UserRepository,
	publicURL string,
) bool {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	items, err := users.GetAllUsers(ctx)
	if err != nil {
		slog.Warn("load users for Mini App menu failed", "err", err)
		return false
	}
	configured := true
	for _, user := range items {
		telegramID, parseErr := strconv.ParseInt(user.ID, 10, 64)
		if parseErr != nil {
			continue
		}
		if configureErr := miniapp.ConfigureMenu(
			bot,
			&tgbotapi.User{ID: telegramID},
			publicURL,
			user.IsAdmin,
		); configureErr != nil {
			configured = false
			slog.Debug("Mini App menu configuration failed", "user_id", user.ID, "err", configureErr)
		}
	}
	return configured
}
