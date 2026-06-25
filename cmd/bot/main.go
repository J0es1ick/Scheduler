package main

import (
	"log"

	"github.com/J0es1ick/Scheduler/internal/config"
	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
	tgbotapi "gopkg.in/telebot.v3"
)

func main() {
	cfg, err := config.InitConfig()
	if err != nil {
		log.Fatal("Failed to initialize config:", err)
	}
	
	db, err := database.NewDatabase(cfg)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	bot, err := tgbotapi.NewBot(tgbotapi.Settings{
		Token: cfg.BotToken,
	})
	if err != nil {
		log.Fatal("Failed to create Telegram bot:", err)
	}

	subRepo := repository.NewSubscriptionRepository(db.DB)
	userRepo := repository.NewUserRepository(db.DB)
	lessonRepo := repository.NewLessonRepository(db.DB)
	semestrRepo := repository.NewSemesterRepository(db.DB)
	groupRepo := repository.NewGroupRepository(db.DB)

	scheduleService := service.NewScheduleService(lessonRepo, semestrRepo, groupRepo)
	notificationService := service.NewNotificationService(subRepo, userRepo, bot)
	userService := service.NewUserService(userRepo)
	semesterService := service.NewSemesterService(semestrRepo)
	groupService := service.NewGroupService(groupRepo)
}