package keyboards

import (
	"github.com/J0es1ick/Scheduler/internal/domain"
	tele "gopkg.in/telebot.v3"
)

func UniversitySelector() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row
	for _, u := range domain.SupportedUniversities {
		btn := menu.Data(u.Name, "select_university", u.ID)
		rows = append(rows, menu.Row(btn))
	}
	menu.Inline(rows...)
	return menu
}

func MainMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true}
	btnSchedule := menu.Text("Расписание")
	btnToday := menu.Text("На сегодня")
	btnSettings := menu.Text("Настройки")
	menu.Reply(
		menu.Row(btnSchedule, btnToday),
		menu.Row(btnSettings),
	)
	return menu
}
