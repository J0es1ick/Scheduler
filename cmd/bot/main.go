package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/J0es1ick/Scheduler/internal/config"
	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/scrapper/isuct"
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

	db, err := database.NewDatabase(cfg)
	if err != nil {
		slog.Error("database connect failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("db close failed", "err", err)
		}
	}()

	bot, err := tgbotapi.NewBot(tgbotapi.Settings{
		Token: cfg.BotToken,
	})
	if err != nil {
		slog.Error("telegram bot init failed", "err", err)
		os.Exit(1)
	}

	// --- Репозитории ---
	userRepo := repository.NewUserRepository(db.DB)
	lessonRepo := repository.NewLessonRepository(db.DB)
	semesterRepo := repository.NewSemesterRepository(db.DB)
	groupRepo := repository.NewGroupRepository(db.DB)
	universityRepo := repository.NewUniversityRepository(db.DB)
	subscriptionRepo := repository.NewSubscriptionRepository(db.DB)
	dataSourceRepo := repository.NewDataSourceRepository(db.DB)
	parseLogRepo := repository.NewParseLogRepository(db.DB)

	// --- Сервисы ---
	scheduleService := service.NewScheduleService(lessonRepo, semesterRepo, groupRepo)
	userService := service.NewUserService(userRepo)
	subscriptionService := service.NewSubscriptionService(subscriptionRepo)
	semesterService := service.NewSemesterService(semesterRepo)
	groupService := service.NewGroupService(groupRepo)
	universityService := service.NewUniversityService(universityRepo)
	parserService := service.NewParserService(dataSourceRepo, parseLogRepo, groupRepo, scheduleService, semesterService)

	// --- Адаптеры ---
	// semesterID не задаём при старте: ParserService резолвит его динамически
	// перед каждым запуском через SemesterService.GetCurrentSemester.
	isuctAdapter := isuct.New("")
	parserService.RegisterAdapter(isuct.UniversityID, isuctAdapter)
	slog.Info("adapter registered", "type", isuct.UniversityID)

	// --- Telegram ---
	stateManager := state.NewManager()
	handler := handlers.NewHandler(
		scheduleService, userService, groupService,
		universityService, stateManager, subscriptionService,
	)
	botpkg.Register(bot, handler)

	// --- Graceful shutdown ---
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// --- Фоновый воркер парсера ---
	// Запускается после регистрации адаптеров, до старта бота.
	// Останавливается вместе с ctx при получении сигнала.
	parserWorker := worker.NewParserWorker(parserService, parserTickInterval)
	parserWorker.Start(ctx)

	// --- Бот ---
	go func() {
		slog.Info("bot started")
		bot.Start()
	}()

	<-ctx.Done()
	slog.Info("shutdown signal received, stopping...")
	bot.Stop()
	slog.Info("bot stopped")
}
