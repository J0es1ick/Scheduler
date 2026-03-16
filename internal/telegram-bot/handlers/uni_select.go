package handlers

import (
	"fmt"
	"log"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func HandleUniversitySelect(c tele.Context) error {
	universityID := c.Args()[0]

	var selected *domain.University
	for i, u := range domain.SupportedUniversities {
		if u.ID == universityID {
			selected = &domain.SupportedUniversities[i]
			break
		}
	}
	if selected == nil {
		return c.Respond(&tele.CallbackResponse{Text: "Университет не найден"})
	}
	if err := c.Respond(); err != nil {
		log.Printf("Respond error: %v", err)
	}
	if err := c.Edit("Университет выбран."); err != nil {
		log.Printf("Edit error: %v", err)
	}
	text := fmt.Sprintf("Университет: *%s*\n\nВыберите нужный раздел в меню ниже.", selected.Name)
	return c.Send(text, &tele.SendOptions{ParseMode: tele.ModeMarkdown}, keyboards.MainMenu())
}
