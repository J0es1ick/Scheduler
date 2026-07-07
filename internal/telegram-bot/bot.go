package bot

import (
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/handlers"
	tgbotapi "gopkg.in/telebot.v3"
)

func Register(b *tgbotapi.Bot, h *handlers.Handler) {
	b.SetCommands([]tgbotapi.Command{
		{Text: "start", Description: "Запустить бота"},
		{Text: "help", Description: "Список команд"},
		{Text: "today", Description: "Расписание на сегодня"},
		{Text: "tomorrow", Description: "Расписание на завтра"},
		{Text: "week", Description: "Расписание на текущую неделю"},
		{Text: "twoweeks", Description: "Расписание на 2 недели"},
		{Text: "change_university", Description: "Сменить университет"},
		{Text: "search", Description: "Поиск занятий"},
		{Text: "change_group", Description: "Сменить группу"},
	})

	// команды
	b.Handle("/start", h.HandleStart)
	b.Handle("/help", h.HandleHelp)
	b.Handle("/today", h.HandleToday)
	b.Handle("/tomorrow", h.HandleTomorrow)
	b.Handle("/week", h.HandleWeek)
	b.Handle("/twoweeks", h.HandleTwoWeeks)
	b.Handle("/change_university", h.HandleChangeUniversity)
	b.Handle("/search", h.HandleSearch)
	b.Handle("/change_group", h.HandleChangeGroup)

	// callback кнопки
	b.Handle(&tgbotapi.Btn{Unique: "select_university"}, h.HandleUniversitySelect)
	b.Handle(&tgbotapi.Btn{Unique: "select_search_type"}, h.HandleSearchTypeSelect)
	b.Handle(&tgbotapi.Btn{Unique: "select_weekday"}, h.HandleWeekDaySelect)
	b.Handle(&tgbotapi.Btn{Unique: "cancel_search"}, h.HandleCancelSearch)

	// reply кнопки
	b.Handle("На сегодня", h.HandleToday)
	b.Handle("На завтра", h.HandleTomorrow)
	b.Handle("На неделю", h.HandleWeek)
	b.Handle("По дню недели", h.HandleWeekDay)
	b.Handle("Поиск", h.HandleSearch)
	b.Handle("Сменить группу", h.HandleChange)
	b.Handle("Настройки", h.HandleSettings)

	// текст
	b.Handle(tgbotapi.OnText, h.HandleTextInput)
}
