package keyboards

import (
	"github.com/J0es1ick/Scheduler/internal/domain"
	tgbotapi "gopkg.in/telebot.v3"
)

func UniversitySelector(unis []domain.University) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	var rows []tgbotapi.Row

	for _, u := range unis {
		btn := menu.Data(u.Name, "select_university", u.ID)
		rows = append(rows, menu.Row(btn))
	}

	menu.Inline(rows...)
	return menu
}

func SearchTypeSelector() *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}

	btnGroup := menu.Data("По группе", "select_search_type", "group")
	btnTeacher := menu.Data("По преподавателю", "select_search_type", "teacher")
	btnRoom := menu.Data("По аудитории", "select_search_type", "room")
	btnDiscipline := menu.Data("По дисциплине", "select_search_type", "discipline")

	menu.Inline(
		menu.Row(btnGroup),
		menu.Row(btnTeacher),
		menu.Row(btnRoom),
		menu.Row(btnDiscipline),
	)

	return menu
}

func CancelButton() *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	btnCancel := menu.Data("Назад", "cancel_search")
	menu.Inline(menu.Row(btnCancel))
	return menu
}

func MainMenu() *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{ResizeKeyboard: true}

	btnToday := menu.Text("На сегодня")
	btnTomorrow := menu.Text("На завтра")
	btnWeek := menu.Text("На неделю")
	btnWeekDay := menu.Text("По дню недели")
	btnSearch := menu.Text("Поиск")
	btnSettings := menu.Text("Настройки")
	btnChange := menu.Text("Сменить группу")

	menu.Reply(
		menu.Row(btnToday, btnTomorrow),
		menu.Row(btnWeek, btnWeekDay),
		menu.Row(btnSearch),
		menu.Row(btnChange, btnSettings),
	)

	return menu
}

func WeekDaySelector() *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}

	btnMon := menu.Data("Понедельник", "select_weekday", "1")
	btnTue := menu.Data("Вторник", "select_weekday", "2")
	btnWed := menu.Data("Среда", "select_weekday", "3")
	btnThu := menu.Data("Четверг", "select_weekday", "4")
	btnFri := menu.Data("Пятница", "select_weekday", "5")
	btnSat := menu.Data("Суббота", "select_weekday", "6")
	btnSun := menu.Data("Воскресенье", "select_weekday", "7")

	menu.Inline(
		menu.Row(btnMon, btnTue, btnWed),
		menu.Row(btnThu, btnFri, btnSat),
		menu.Row(btnSun),
	)

	return menu
}
