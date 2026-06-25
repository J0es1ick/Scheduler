package handlers

import (
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func HandleStart(c tele.Context) error {
	name := c.Sender().FirstName
	text := fmt.Sprintf(
		"Добрый день, %s.\n\nДанный бот предоставляет актуальное расписание занятий.\n\n"+
			"Доступные команды:\n"+
			"/change_university — сменить университет\n\n"+
			"Выберите ваш университет:",
		name,
	)
	return c.Send(text, keyboards.UniversitySelector())
}
