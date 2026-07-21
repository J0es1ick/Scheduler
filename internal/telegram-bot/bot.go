package bot

import (
	"log/slog"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/handlers"
	tele "gopkg.in/telebot.v3"
)

func Register(bot *tele.Bot, handler *handlers.Handler) {
	commands := []tele.Command{
		{Text: "start", Description: "Запустить бота"},
		{Text: "help", Description: "Список команд"},
		{Text: "today", Description: "Расписание на сегодня"},
		{Text: "tomorrow", Description: "Расписание на завтра"},
		{Text: "week", Description: "Расписание на неделю"},
		{Text: "twoweeks", Description: "Расписание на две недели"},
		{Text: "search", Description: "Поиск занятий"},
		{Text: "change_group", Description: "Добавить или сменить основную группу"},
		{Text: "change_university", Description: "Выбрать другой вуз"},
		{Text: "settings", Description: "Подписки и уведомления"},
		{Text: "subscriptions", Description: "Подписки на группы"},
		{Text: "admin", Description: "Открыть админ-панель"},
	}
	go configureCommands(bot, commands)

	bot.Handle("/start", handler.HandleStart)
	bot.Handle("/help", handler.HandleHelp)
	bot.Handle("/today", handler.HandleToday)
	bot.Handle("/tomorrow", handler.HandleTomorrow)
	bot.Handle("/week", handler.HandleWeek)
	bot.Handle("/twoweeks", handler.HandleTwoWeeks)
	bot.Handle("/search", handler.HandleSearch)
	bot.Handle("/change_group", handler.HandleChangeGroup)
	bot.Handle("/change_university", handler.HandleChangeUniversity)
	bot.Handle("/settings", handler.HandleSettings)
	bot.Handle("/subscriptions", handler.HandleSettings)
	bot.Handle("/admin", handler.HandleAdmin)

	bot.Handle(&tele.Btn{Unique: "select_university"}, handler.HandleUniversitySelect)
	bot.Handle(&tele.Btn{Unique: "select_search_type"}, handler.HandleSearchTypeSelect)
	bot.Handle(&tele.Btn{Unique: "select_weekday"}, handler.HandleWeekDaySelect)
	bot.Handle(&tele.Btn{Unique: "cancel_search"}, handler.HandleCancelSearch)
	bot.Handle(&tele.Btn{Unique: "set_default_subscription"}, handler.HandleSetDefaultSubscription)
	bot.Handle(&tele.Btn{Unique: "delete_subscription"}, handler.HandleDeleteSubscription)
	bot.Handle(&tele.Btn{Unique: "toggle_notifications"}, handler.HandleToggleNotifications)

	bot.Handle("На сегодня", handler.HandleToday)
	bot.Handle("На завтра", handler.HandleTomorrow)
	bot.Handle("На неделю", handler.HandleWeek)
	bot.Handle("По дню недели", handler.HandleWeekDay)
	bot.Handle("Поиск", handler.HandleSearch)
	bot.Handle("Сменить группу", handler.HandleChange)
	bot.Handle("Добавить группу", handler.HandleChange)
	bot.Handle("Настройки", handler.HandleSettings)

	bot.Handle(tele.OnText, handler.HandleTextInput)
}

func configureCommands(bot *tele.Bot, commands []tele.Command) {
	for {
		if err := bot.SetCommands(commands); err == nil {
			slog.Info("Telegram commands configured")
			return
		} else {
			slog.Warn("Telegram commands configuration failed; retrying", "err", err)
		}
		time.Sleep(30 * time.Second)
	}
}
