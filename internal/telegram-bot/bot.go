package bot

import (
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/handlers"
	tele "gopkg.in/telebot.v3"
)

func Register(b *tele.Bot) {
	b.SetCommands([]tele.Command{
		{Text: "start", Description: "Запустить бота"},
		{Text: "change_university", Description: "Сменить университет"},
	})

	b.Handle("/start", handlers.HandleStart)
	b.Handle("/change_university", handlers.HandleChangeUniversity)

	b.Handle(&tele.Btn{Unique: "select_university"}, handlers.HandleUniversitySelect)

	b.Handle("Расписание", handlers.HandleSchedule)
	b.Handle("На сегодня", handlers.HandleToday)
	b.Handle("Настройки", handlers.HandleSettings)
}
