package keyboards

import (
	"fmt"
	"time"

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

func ScheduleCalendar(month time.Time) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	month = time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())
	rows := []tgbotapi.Row{
		menu.Row(
			menu.Data("Пн", "calendar_noop"),
			menu.Data("Вт", "calendar_noop"),
			menu.Data("Ср", "calendar_noop"),
			menu.Data("Чт", "calendar_noop"),
			menu.Data("Пт", "calendar_noop"),
			menu.Data("Сб", "calendar_noop"),
			menu.Data("Вс", "calendar_noop"),
		),
	}

	weekday := int(month.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	days := time.Date(
		month.Year(),
		month.Month()+1,
		0,
		0,
		0,
		0,
		0,
		month.Location(),
	).Day()
	cells := make([]tgbotapi.Btn, 0, 42)
	for i := 1; i < weekday; i++ {
		cells = append(cells, menu.Data("·", "calendar_noop"))
	}
	today := time.Now().In(month.Location())
	for day := 1; day <= days; day++ {
		date := time.Date(
			month.Year(),
			month.Month(),
			day,
			0,
			0,
			0,
			0,
			month.Location(),
		)
		label := fmt.Sprint(day)
		if sameCalendarDate(date, today) {
			label = "•" + label
		}
		cells = append(
			cells,
			menu.Data(label, "schedule_date", date.Format("2006-01-02")),
		)
	}
	for len(cells)%7 != 0 {
		cells = append(cells, menu.Data("·", "calendar_noop"))
	}
	for index := 0; index < len(cells); index += 7 {
		rows = append(rows, menu.Row(cells[index:index+7]...))
	}

	previous := month.AddDate(0, -1, 0).Format("2006-01")
	next := month.AddDate(0, 1, 0).Format("2006-01")
	rows = append(rows, menu.Row(
		menu.Data("‹", "calendar_month", previous),
		menu.Data("Сегодня", "schedule_date", today.Format("2006-01-02")),
		menu.Data("›", "calendar_month", next),
	))
	menu.Inline(rows...)
	return menu
}

func sameCalendarDate(first, second time.Time) bool {
	firstYear, firstMonth, firstDay := first.Date()
	secondYear, secondMonth, secondDay := second.Date()
	return firstYear == secondYear &&
		firstMonth == secondMonth &&
		firstDay == secondDay
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
	btnChange := menu.Text("Добавить группу")
	btnHotline := menu.Text("Горячая линия")

	menu.Reply(
		menu.Row(btnToday, btnTomorrow),
		menu.Row(btnWeek, btnWeekDay),
		menu.Row(btnSearch),
		menu.Row(btnChange, btnSettings),
		menu.Row(btnHotline),
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

func SubscriptionSettings(
	items []domain.GroupSubscription,
	notificationsEnabled bool,
	reminderEnabled bool,
	reminderMinutes int,
) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	rows := make([]tgbotapi.Row, 0, len(items)+2)
	for _, item := range items {
		label := item.UniversityName + " · " + item.GroupName
		if item.IsDefault {
			label = "● " + label
		}
		selectButton := menu.Data(label, "set_default_subscription", item.GroupID)
		deleteButton := menu.Data("Удалить", "delete_subscription", item.GroupID)
		rows = append(rows, menu.Row(selectButton, deleteButton))
	}

	toggleLabel := "Выключить уведомления"
	if !notificationsEnabled {
		toggleLabel = "Включить уведомления"
	}
	rows = append(rows, menu.Row(menu.Data(toggleLabel, "toggle_notifications")))
	reminderLabel := "Напоминания: выключены"
	if reminderEnabled {
		reminderLabel = fmt.Sprintf("Напоминания: за %d мин.", reminderMinutes)
	}
	rows = append(rows, menu.Row(menu.Data(reminderLabel, "show_reminder_settings")))
	menu.Inline(rows...)
	return menu
}

func ReminderSettings(enabled bool, minutes int) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	label := func(value int) string {
		if enabled && minutes == value {
			return fmt.Sprintf("● %d мин.", value)
		}
		return fmt.Sprintf("%d мин.", value)
	}
	menu.Inline(
		menu.Row(
			menu.Data(label(5), "set_reminder", "5"),
			menu.Data(label(10), "set_reminder", "10"),
			menu.Data(label(15), "set_reminder", "15"),
		),
		menu.Row(
			menu.Data(label(30), "set_reminder", "30"),
			menu.Data(label(60), "set_reminder", "60"),
			menu.Data(label(120), "set_reminder", "120"),
		),
		menu.Row(menu.Data("Выключить", "set_reminder", "off")),
		menu.Row(menu.Data("Назад к настройкам", "back_subscription_settings")),
	)
	return menu
}

func HotlineTypeSelector() *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("Обновить подключённое расписание", "select_hotline_type", domain.SupportRequestUpdateExisting)),
		menu.Row(menu.Data("Добавить учебное заведение", "select_hotline_type", domain.SupportRequestNewInstitution)),
		menu.Row(menu.Data("Отмена", "cancel_hotline")),
	)
	return menu
}

func DeleteProfileConfirmation() *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	menu.Inline(menu.Row(
		menu.Data("Удалить мои данные", "confirm_delete_profile"),
		menu.Data("Отмена", "cancel_delete_profile"),
	))
	return menu
}
