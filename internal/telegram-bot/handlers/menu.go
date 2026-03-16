package handlers

import (
	"log"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func HandleSchedule(c tele.Context) error {
	return c.Send("Расписание в разработке.")
}

func HandleToday(c tele.Context) error {
	return c.Send("На сегодня в разработке.")
}

func HandleSettings(c tele.Context) error {
	return c.Send("Настройки в разработке.")
}

func HandleChangeUniversity(c tele.Context) error {
	remove := &tele.ReplyMarkup{RemoveKeyboard: true}
	if err := c.Send("Выберите университет:", remove); err != nil {
		log.Printf("Send error: %v", err)
	}
	return c.Send("Доступные университеты:", keyboards.UniversitySelector())
}
