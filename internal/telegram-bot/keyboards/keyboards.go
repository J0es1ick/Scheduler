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
	rows = append(rows, menu.Row(menu.Data("Закрыть", "close_inline")))

	menu.Inline(rows...)
	return menu
}

func ScheduleCalendar(month time.Time) *tgbotapi.ReplyMarkup {
	return scheduleCalendar(month, "", time.Time{})
}

func ScheduleCalendarWithBack(
	month time.Time,
	backAction string,
	backDate time.Time,
) *tgbotapi.ReplyMarkup {
	if backAction != "schedule_date" && backAction != "schedule_week" {
		return ScheduleCalendar(month)
	}
	return scheduleCalendar(month, backAction, backDate)
}

func scheduleCalendar(
	month time.Time,
	backAction string,
	backDate time.Time,
) *tgbotapi.ReplyMarkup {
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
	previousButton := menu.Data("‹", "calendar_month", previous)
	nextButton := menu.Data("›", "calendar_month", next)
	if backAction != "" {
		backValue := backDate.Format("2006-01-02")
		previousButton = menu.Data("‹", "calendar_month", previous, backAction, backValue)
		nextButton = menu.Data("›", "calendar_month", next, backAction, backValue)
	}
	rows = append(rows, menu.Row(
		previousButton,
		menu.Data("Сегодня", "schedule_date", today.Format("2006-01-02")),
		nextButton,
	))
	if backAction == "" {
		rows = append(rows, menu.Row(menu.Data("Закрыть", "close_inline")))
	} else {
		rows = append(rows, menu.Row(menu.Data(
			"Назад",
			backAction,
			backDate.Format("2006-01-02"),
		)))
	}
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
		menu.Row(menu.Data("Закрыть", "close_inline")),
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
	menu := &tgbotapi.ReplyMarkup{ResizeKeyboard: true, IsPersistent: true}

	btnToday := menu.Text("Сегодня")
	btnTomorrow := menu.Text("Завтра")
	btnWeek := menu.Text("Неделя")
	btnDate := menu.Text("Выбрать дату")
	btnSearch := menu.Text("Поиск")
	btnGroups := menu.Text("Мои группы")
	btnMore := menu.Text("Ещё")

	menu.Reply(
		menu.Row(btnToday, btnTomorrow),
		menu.Row(btnWeek, btnDate),
		menu.Row(btnSearch, btnGroups),
		menu.Row(btnMore),
	)

	return menu
}

func WeekDaySelector(from time.Time) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	weekStart := from.Format("2006-01-02")

	btnMon := menu.Data("Понедельник", "select_weekday", "1", weekStart)
	btnTue := menu.Data("Вторник", "select_weekday", "2", weekStart)
	btnWed := menu.Data("Среда", "select_weekday", "3", weekStart)
	btnThu := menu.Data("Четверг", "select_weekday", "4", weekStart)
	btnFri := menu.Data("Пятница", "select_weekday", "5", weekStart)
	btnSat := menu.Data("Суббота", "select_weekday", "6", weekStart)
	btnSun := menu.Data("Воскресенье", "select_weekday", "7", weekStart)

	menu.Inline(
		menu.Row(btnMon, btnTue, btnWed),
		menu.Row(btnThu, btnFri, btnSat),
		menu.Row(btnSun),
		menu.Row(menu.Data("Назад", "schedule_week", weekStart)),
	)

	return menu
}

func SubscriptionSettings(
	items []domain.GroupSubscription,
	notificationsEnabled bool,
	reminderEnabled bool,
	reminderMinutes int,
	page int,
) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	const pageSize = 7
	pageCount := max(1, (len(items)+pageSize-1)/pageSize)
	if page < 0 {
		page = 0
	}
	if page >= pageCount {
		page = pageCount - 1
	}
	start := page * pageSize
	end := min(start+pageSize, len(items))
	rows := make([]tgbotapi.Row, 0, pageSize+6)
	for _, item := range items[start:end] {
		label := item.UniversityName + " · " + item.GroupName
		if item.IsDefault {
			label = "● " + label
		}
		rows = append(rows, menu.Row(menu.Data(label, "open_subscription", item.GroupID, fmt.Sprint(page))))
	}
	if pageCount > 1 {
		previousPage := max(0, page-1)
		nextPage := min(pageCount-1, page+1)
		rows = append(rows, menu.Row(
			menu.Data("‹", "subscription_page", fmt.Sprint(previousPage)),
			menu.Data(fmt.Sprintf("%d/%d", page+1, pageCount), "subscription_page", fmt.Sprint(page)),
			menu.Data("›", "subscription_page", fmt.Sprint(nextPage)),
		))
	}

	toggleLabel := "Выключить уведомления"
	if !notificationsEnabled {
		toggleLabel = "Включить уведомления"
	}
	rows = append(rows, menu.Row(menu.Data(toggleLabel, "toggle_notifications", fmt.Sprint(page))))
	reminderLabel := "Напоминания: выключены"
	if reminderEnabled {
		reminderLabel = fmt.Sprintf("Напоминания: за %d мин.", reminderMinutes)
	}
	rows = append(rows, menu.Row(menu.Data(reminderLabel, "show_reminder_settings", fmt.Sprint(page))))
	rows = append(rows, menu.Row(menu.Data("Добавить группу", "add_subscription")))
	rows = append(rows, menu.Row(menu.Data("Закрыть", "close_inline")))
	menu.Inline(rows...)
	return menu
}

func SubscriptionActions(item domain.GroupSubscription, page int) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	rows := make([]tgbotapi.Row, 0, 4)
	if !item.IsDefault {
		rows = append(rows, menu.Row(menu.Data("Сделать основной", "set_default_subscription", item.GroupID, fmt.Sprint(page))))
	}
	if item.IsDefault {
		rows = append(rows, menu.Row(menu.Data("Настроить напоминания", "show_reminder_settings", fmt.Sprint(page))))
	}
	rows = append(rows,
		menu.Row(menu.Data("Удалить подписку", "request_delete_subscription", item.GroupID, fmt.Sprint(page))),
		menu.Row(menu.Data("Назад к группам", "subscription_page", fmt.Sprint(page))),
	)
	menu.Inline(rows...)
	return menu
}

func DeleteSubscriptionConfirmation(groupID string, page int) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("Да, удалить", "confirm_delete_subscription", groupID, fmt.Sprint(page))),
		menu.Row(menu.Data("Отмена", "open_subscription", groupID, fmt.Sprint(page))),
	)
	return menu
}

func ReminderSettings(enabled bool, minutes int, page int) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	label := func(value int) string {
		if enabled && minutes == value {
			return fmt.Sprintf("● %d мин.", value)
		}
		return fmt.Sprintf("%d мин.", value)
	}
	menu.Inline(
		menu.Row(
			menu.Data(label(5), "set_reminder", "5", fmt.Sprint(page)),
			menu.Data(label(10), "set_reminder", "10", fmt.Sprint(page)),
			menu.Data(label(15), "set_reminder", "15", fmt.Sprint(page)),
		),
		menu.Row(
			menu.Data(label(30), "set_reminder", "30", fmt.Sprint(page)),
			menu.Data(label(60), "set_reminder", "60", fmt.Sprint(page)),
			menu.Data(label(120), "set_reminder", "120", fmt.Sprint(page)),
		),
		menu.Row(menu.Data("Выключить", "set_reminder", "off", fmt.Sprint(page))),
		menu.Row(menu.Data("Назад к группам", "back_subscription_settings", fmt.Sprint(page))),
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

func MoreMenu() *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("Источники расписания", "show_sources")),
		menu.Row(menu.Data("Подключить своё расписание", "show_connector")),
		menu.Row(menu.Data("Сообщить о расписании", "open_hotline")),
		menu.Row(menu.Data("Конфиденциальность и данные", "show_privacy")),
		menu.Row(menu.Data("Помощь", "show_help")),
		menu.Row(menu.Data("Закрыть", "close_inline")),
	)
	return menu
}

func BackToMoreMenu() *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	menu.Inline(menu.Row(
		menu.Data("Назад", "back_more"),
		menu.Data("Закрыть", "close_inline"),
	))
	return menu
}

func ScheduleDayNavigation(date time.Time, groupName string, groupChat bool) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	today := time.Now().In(date.Location())
	groupLabel := "Группа: " + groupName
	if groupChat {
		groupLabel = "Настройки чата"
	}
	rows := []tgbotapi.Row{
		menu.Row(
			menu.Data("←", "schedule_date", date.AddDate(0, 0, -1).Format("2006-01-02")),
			menu.Data("Сегодня", "schedule_date", today.Format("2006-01-02")),
			menu.Data("→", "schedule_date", date.AddDate(0, 0, 1).Format("2006-01-02")),
		),
		menu.Row(
			menu.Data("Неделя", "schedule_week", date.Format("2006-01-02")),
			menu.Data(
				"Выбрать дату",
				"open_calendar",
				date.Format("2006-01"),
				"schedule_date",
				date.Format("2006-01-02"),
			),
		),
	}
	if groupChat {
		rows = append(rows, menu.Row(menu.Data(groupLabel, "open_schedule_group")))
	} else {
		rows = append(rows, menu.Row(
			menu.Data(groupLabel, "open_schedule_group"),
			menu.Data("Главное меню", "open_main_menu"),
		))
	}
	menu.Inline(rows...)
	return menu
}

func ScheduleWeekNavigation(from time.Time, groupName string, groupChat bool) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	groupLabel := "Группа: " + groupName
	if groupChat {
		groupLabel = "Настройки чата"
	}
	rows := []tgbotapi.Row{
		menu.Row(
			menu.Data("← Неделя", "schedule_week", from.AddDate(0, 0, -7).Format("2006-01-02")),
			menu.Data("Текущая", "schedule_week", time.Now().In(from.Location()).Format("2006-01-02")),
			menu.Data("Неделя →", "schedule_week", from.AddDate(0, 0, 7).Format("2006-01-02")),
		),
		menu.Row(
			menu.Data("Выбрать день", "open_weekday", from.Format("2006-01-02")),
			menu.Data(
				"Выбрать дату",
				"open_calendar",
				from.Format("2006-01"),
				"schedule_week",
				from.Format("2006-01-02"),
			),
		),
	}
	if groupChat {
		rows = append(rows, menu.Row(menu.Data(groupLabel, "open_schedule_group")))
	} else {
		rows = append(rows, menu.Row(
			menu.Data(groupLabel, "open_schedule_group"),
			menu.Data("Главное меню", "open_main_menu"),
		))
	}
	menu.Inline(rows...)
	return menu
}

func ChatSettings(groupName string, isAdmin bool) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	rows := []tgbotapi.Row{
		menu.Row(menu.Data("Расписание на сегодня", "schedule_date", time.Now().Format("2006-01-02"))),
	}
	if isAdmin {
		rows = append(rows,
			menu.Row(menu.Data("Сменить группу", "chat_change_group")),
			menu.Row(menu.Data("Удалить привязку", "request_unset_chat_group")),
		)
	}
	rows = append(rows, menu.Row(menu.Data("Закрыть", "close_inline")))
	menu.Inline(rows...)
	return menu
}

func EmptyChatSettings(isAdmin bool) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	rows := make([]tgbotapi.Row, 0, 2)
	if isAdmin {
		rows = append(rows, menu.Row(menu.Data("Настроить группу", "chat_change_group")))
	}
	rows = append(rows, menu.Row(menu.Data("Закрыть", "close_inline")))
	menu.Inline(rows...)
	return menu
}

func UnsetChatConfirmation() *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("Да, удалить привязку", "confirm_unset_chat_group")),
		menu.Row(menu.Data("Отмена", "open_schedule_group")),
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
