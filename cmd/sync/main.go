package main

import (
	"context"
	"log/slog"
	"os"
	"time"

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
		logger.Error("config init failed", "err", err)
		os.Exit(1)
	}
	db, err := database.NewDatabase(cfg)
	if err != nil {
		logger.Error("database connect failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if err = database.ApplyMigrations(ctx, db.DB); err != nil {
		logger.Error("database migrations failed", "err", err)
		os.Exit(1)
	}

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
	)
	parserService.RegisterAdapter(isuct.UniversityID, isuct.New(""))
	parserService.RegisterAdapter(ispu.UniversityID, ispu.New(""))

	sources := os.Args[1:]
	if len(sources) == 0 {
		sources = []string{"isuct-main", "ispu-main"}
	}
	totalLessons := 0
	for _, sourceID := range sources {
		lessons, runErr := parserService.RunDataSource(ctx, sourceID)
		if runErr != nil {
			logger.Error("schedule synchronization failed", "source", sourceID, "err", runErr)
			os.Exit(1)
		}
		totalLessons += lessons
		logger.Info("data source synchronization completed", "source", sourceID, "lessons", lessons)
	}
	logger.Info("schedule synchronization completed", "sources", len(sources), "lessons", totalLessons)
}
