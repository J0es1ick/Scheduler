package keyboards

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	tgbotapi "gopkg.in/telebot.v3"
)

func UniversitySelector(unis []domain.University, nonce ...string) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	var rows []tgbotapi.Row
	flowNonce := ""
	if len(nonce) > 0 {
		flowNonce = nonce[0]
	}

	for _, u := range unis {
		btn := menu.Data(u.Name, "select_university", UniversityToken(u.ID), flowNonce)
		rows = append(rows, menu.Row(btn))
	}
	rows = append(rows, menu.Row(menu.Data("Назад", "cancel_university_selection", flowNonce)))

	menu.Inline(rows...)
	return menu
}

func ScheduleCalendar(month time.Time, groupToken ...string) *tgbotapi.ReplyMarkup {
	return scheduleCalendar(month, "", time.Time{}, "", firstArgument(groupToken))
}

func ScheduleCalendarWithBack(
	month time.Time,
	backAction string,
	backDate time.Time,
	backArgument ...string,
) *tgbotapi.ReplyMarkup {
	if backAction != "schedule_date" && backAction != "schedule_week" {
		return ScheduleCalendar(month)
	}
	extra := ""
	if len(backArgument) > 0 {
		extra = backArgument[0]
	}
	groupToken := ""
	if len(backArgument) > 1 {
		groupToken = backArgument[1]
	}
	return scheduleCalendar(month, backAction, backDate, extra, groupToken)
}

func scheduleCalendar(
	month time.Time,
	backAction string,
	backDate time.Time,
	backArgument string,
	groupToken string,
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
		if backAction == "schedule_week" {
			daysCount := backArgument
			if daysCount == "" {
				daysCount = "7"
			}
			cells = append(cells, menu.Data(
				label,
				"schedule_period_date",
				date.Format("2006-01-02"),
				backDate.Format("2006-01-02"),
				daysCount,
				groupToken,
			))
		} else {
			cells = append(cells, menu.Data(label, "schedule_date", date.Format("2006-01-02"), groupToken))
		}
	}
	for len(cells)%7 != 0 {
		cells = append(cells, menu.Data("·", "calendar_noop"))
	}
	for index := 0; index < len(cells); index += 7 {
		rows = append(rows, menu.Row(cells[index:index+7]...))
	}

	previous := month.AddDate(0, -1, 0).Format("2006-01")
	next := month.AddDate(0, 1, 0).Format("2006-01")
	action := ""
	if backAction == "schedule_date" {
		action = "d"
	} else if backAction == "schedule_week" {
		action = "w"
	}
	backValue := ""
	if action != "" {
		backValue = backDate.Format("2006-01-02")
	}
	previousButton := menu.Data("‹", "calendar_month", previous, action, backValue, backArgument, groupToken)
	nextButton := menu.Data("›", "calendar_month", next, action, backValue, backArgument, groupToken)
	todayButton := menu.Data("Сегодня", "schedule_date", today.Format("2006-01-02"), groupToken)
	if backAction == "schedule_week" {
		daysCount := backArgument
		if daysCount == "" {
			daysCount = "7"
		}
		todayButton = menu.Data(
			"Сегодня",
			"schedule_period_date",
			today.Format("2006-01-02"),
			backDate.Format("2006-01-02"),
			daysCount,
			groupToken,
		)
	}
	rows = append(rows, menu.Row(previousButton, todayButton, nextButton))
	if backAction == "" {
		rows = append(rows, menu.Row(menu.Data("Закрыть", "close_inline")))
	} else {
		if backAction == "schedule_date" {
			rows = append(rows, menu.Row(menu.Data("Назад", backAction, backDate.Format("2006-01-02"), groupToken)))
		} else {
			rows = append(rows, menu.Row(menu.Data("Назад", backAction, backDate.Format("2006-01-02"), backArgument, groupToken)))
		}
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

func SearchTypeSelector(nonce ...string) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	flowNonce := ""
	if len(nonce) > 0 {
		flowNonce = nonce[0]
	}

	btnGroup := menu.Data("По группе", "select_search_type", "group", flowNonce)
	btnTeacher := menu.Data("По преподавателю", "select_search_type", "teacher", flowNonce)
	btnRoom := menu.Data("По аудитории", "select_search_type", "room", flowNonce)
	btnDiscipline := menu.Data("По дисциплине", "select_search_type", "discipline", flowNonce)

	menu.Inline(
		menu.Row(btnGroup),
		menu.Row(btnTeacher),
		menu.Row(btnRoom),
		menu.Row(btnDiscipline),
		menu.Row(menu.Data("Назад", "cancel_search_type", flowNonce)),
	)

	return menu
}

func CancelButton(arguments ...string) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	btnCancel := menu.Data("Назад", "cancel_search", arguments...)
	menu.Inline(menu.Row(btnCancel))
	return menu
}

func TeacherMatches(names []string, nonce string) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	rows := make([]tgbotapi.Row, 0, len(names)+1)
	for index, name := range names {
		rows = append(rows, menu.Row(menu.Data(name, "select_teacher", fmt.Sprint(index), nonce)))
	}
	rows = append(rows, menu.Row(menu.Data("Назад", "cancel_teacher_selection", nonce)))
	menu.Inline(rows...)
	return menu
}

func BackButton(action string, arguments ...string) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("Назад", action, arguments...)))
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

func WeekDaySelector(from time.Time, daysCount int, groupToken ...string) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	periodStart := from.Format("2006-01-02")
	if daysCount != 14 {
		daysCount = 7
	}
	labels := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
	rows := make([]tgbotapi.Row, 0, daysCount/2+1)
	for offset := 0; offset < daysCount; offset += 2 {
		row := make(tgbotapi.Row, 0, 2)
		for index := offset; index < min(offset+2, daysCount); index++ {
			date := from.AddDate(0, 0, index)
			weekday := int(date.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			row = append(row, menu.Data(
				fmt.Sprintf("%s %s", labels[weekday-1], date.Format("02.01")),
				"schedule_period_date",
				date.Format("2006-01-02"),
				periodStart,
				fmt.Sprint(daysCount),
				firstArgument(groupToken),
			))
		}
		rows = append(rows, row)
	}
	rows = append(rows, menu.Row(menu.Data("Назад", "schedule_week", periodStart, fmt.Sprint(daysCount), firstArgument(groupToken))))
	menu.Inline(rows...)

	return menu
}

func SchedulePeriodDateBack(from time.Time, daysCount int, groupToken ...string) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	label := "Назад к неделе"
	if daysCount == 14 {
		label = "Назад к двум неделям"
	}
	menu.Inline(menu.Row(menu.Data(label, "schedule_week", from.Format("2006-01-02"), fmt.Sprint(daysCount), firstArgument(groupToken))))
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
		if !item.IsActive {
			label += " · неактивна"
		}
		rows = append(rows, menu.Row(menu.Data(label, "open_subscription", GroupToken(item.GroupID), fmt.Sprint(page))))
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
	rows = append(rows, menu.Row(menu.Data("Добавить группу", "add_subscription", fmt.Sprint(page))))
	rows = append(rows, menu.Row(menu.Data("Закрыть", "close_inline")))
	menu.Inline(rows...)
	return menu
}

func SubscriptionActions(item domain.GroupSubscription, page int) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	groupToken := GroupToken(item.GroupID)
	rows := make([]tgbotapi.Row, 0, 8)
	if item.IsActive {
		rows = append(rows,
			menu.Row(
				menu.Data("Сегодня", "subscription_schedule", groupToken, "today", fmt.Sprint(page)),
				menu.Data("Завтра", "subscription_schedule", groupToken, "tomorrow", fmt.Sprint(page)),
			),
			menu.Row(menu.Data("Неделя", "subscription_schedule", groupToken, "week", fmt.Sprint(page))),
		)
	}
	if !item.IsDefault && item.IsActive {
		rows = append(rows, menu.Row(menu.Data("Сделать основной", "set_default_subscription", groupToken, fmt.Sprint(page))))
	}
	if item.IsDefault {
		rows = append(rows, menu.Row(menu.Data("Настроить напоминания", "show_reminder_settings", fmt.Sprint(page))))
	}
	formatLabel := "Формат: компактный"
	if item.ScheduleViewFormat == domain.ScheduleViewVisual {
		formatLabel = "Формат: таблица"
	}
	rows = append(rows, menu.Row(menu.Data(formatLabel, "schedule_view_settings", groupToken, fmt.Sprint(page))))
	subgroupLabel := "Подгруппа: все"
	if item.Subgroup > 0 {
		subgroupLabel = fmt.Sprintf("Подгруппа: %d", item.Subgroup)
	}
	rows = append(rows, menu.Row(menu.Data(subgroupLabel, "subgroup_settings", groupToken, fmt.Sprint(page))))
	rows = append(rows,
		menu.Row(menu.Data("Удалить подписку", "request_delete_subscription", groupToken, fmt.Sprint(page))),
		menu.Row(menu.Data("Назад к группам", "subscription_page", fmt.Sprint(page))),
	)
	menu.Inline(rows...)
	return menu
}

func SubgroupSettings(item domain.GroupSubscription, page int, subgroupPages ...int) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	groupToken := GroupToken(item.GroupID)
	label := func(value int, text string) string {
		if item.Subgroup == value {
			return "● " + text
		}
		return text
	}
	subgroupPage := max(0, item.Subgroup-1) / 10
	if len(subgroupPages) > 0 {
		subgroupPage = min(9, max(0, subgroupPages[0]))
	}
	rows := []tgbotapi.Row{menu.Row(menu.Data(label(0, "Все подгруппы"), "set_subscription_subgroup", groupToken, "0", fmt.Sprint(page)))}
	for value := subgroupPage*10 + 1; value <= subgroupPage*10+10; value += 2 {
		rows = append(rows, menu.Row(
			menu.Data(label(value, fmt.Sprintf("Подгруппа %d", value)), "set_subscription_subgroup", groupToken, fmt.Sprint(value), fmt.Sprint(page)),
			menu.Data(label(value+1, fmt.Sprintf("Подгруппа %d", value+1)), "set_subscription_subgroup", groupToken, fmt.Sprint(value+1), fmt.Sprint(page)),
		))
	}
	var navigation []tgbotapi.Btn
	if subgroupPage > 0 {
		navigation = append(navigation, menu.Data("← Подгруппы", "subgroup_settings", groupToken, fmt.Sprint(page), fmt.Sprint(subgroupPage-1)))
	}
	if subgroupPage < 9 {
		navigation = append(navigation, menu.Data("Подгруппы →", "subgroup_settings", groupToken, fmt.Sprint(page), fmt.Sprint(subgroupPage+1)))
	}
	rows = append(rows, menu.Row(navigation...), menu.Row(menu.Data("Назад", "open_subscription", groupToken, fmt.Sprint(page))))
	menu.Inline(rows...)
	return menu
}

func ScheduleViewSettings(item domain.GroupSubscription, page int) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	groupToken := GroupToken(item.GroupID)
	compactLabel := "Компактный текст"
	visualLabel := "Визуальная таблица"
	if item.ScheduleViewFormat == domain.ScheduleViewCompact {
		compactLabel = "● " + compactLabel
	}
	if item.ScheduleViewFormat == domain.ScheduleViewVisual {
		visualLabel = "● " + visualLabel
	}
	menu.Inline(
		menu.Row(menu.Data(compactLabel, "set_schedule_view", groupToken, string(domain.ScheduleViewCompact), fmt.Sprint(page))),
		menu.Row(menu.Data(visualLabel, "set_schedule_view", groupToken, string(domain.ScheduleViewVisual), fmt.Sprint(page))),
		menu.Row(menu.Data("Назад", "open_subscription", groupToken, fmt.Sprint(page))),
	)
	return menu
}

func DeleteSubscriptionConfirmation(groupToken string, page int, intentToken string) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("Да, удалить", "confirm_sub_delete", groupToken, fmt.Sprint(page), intentToken)),
		menu.Row(menu.Data("Отмена", "cancel_sub_delete", groupToken, fmt.Sprint(page), intentToken)),
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

func HotlineTypeSelector(nonce ...string) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	flowNonce := ""
	if len(nonce) > 0 {
		flowNonce = nonce[0]
	}
	menu.Inline(
		menu.Row(menu.Data("Обновить подключённое расписание", "select_hotline_type", domain.SupportRequestUpdateExisting, flowNonce)),
		menu.Row(menu.Data("Добавить учебное заведение", "select_hotline_type", domain.SupportRequestNewInstitution, flowNonce)),
		menu.Row(menu.Data("Назад", "cancel_hotline_type", flowNonce)),
	)
	return menu
}

func MoreMenu() *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("Формат расписания из поиска", "search_view_settings")),
		menu.Row(menu.Data("Источники расписания", "show_sources")),
		menu.Row(menu.Data("Подключить своё расписание", "show_connector")),
		menu.Row(menu.Data("Сообщить о расписании", "open_hotline")),
		menu.Row(menu.Data("Конфиденциальность и данные", "show_privacy")),
		menu.Row(menu.Data("Помощь", "show_help")),
		menu.Row(menu.Data("Закрыть", "close_inline")),
	)
	return menu
}

func SearchScheduleViewSettings(format domain.ScheduleViewFormat) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	compactLabel := "Компактный текст"
	visualLabel := "Визуальная таблица"
	if format == domain.ScheduleViewCompact {
		compactLabel = "● " + compactLabel
	} else {
		visualLabel = "● " + visualLabel
	}
	menu.Inline(
		menu.Row(menu.Data(compactLabel, "set_search_view", string(domain.ScheduleViewCompact))),
		menu.Row(menu.Data(visualLabel, "set_search_view", string(domain.ScheduleViewVisual))),
		menu.Row(menu.Data("Назад", "back_more")),
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

func ScheduleDayNavigation(date time.Time, groupName string, groupChat bool, groupID string, reference ...string) *tgbotapi.ReplyMarkup {
	token := firstArgument(reference)
	if token == "" {
		token = scheduleGroupToken(groupID)
	}
	return scheduleDayNavigation(date, "Группа: "+groupName, "open_schedule_group", groupChat, groupID != "", token)
}

func TeacherScheduleDayNavigation(date time.Time, teacherName, reference string) *tgbotapi.ReplyMarkup {
	return scheduleDayNavigation(date, "Преподаватель: "+teacherName, "search_teacher_again", false, true, reference)
}

func scheduleDayNavigation(
	date time.Time,
	targetLabel string,
	targetAction string,
	groupChat bool,
	canExport bool,
	token string,
) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	today := time.Now().In(date.Location())
	if groupChat {
		targetLabel = "Настройки чата"
	}
	rows := []tgbotapi.Row{
		menu.Row(
			menu.Data("←", "schedule_date", date.AddDate(0, 0, -1).Format("2006-01-02"), token),
			menu.Data("Сегодня", "schedule_date", today.Format("2006-01-02"), token),
			menu.Data("→", "schedule_date", date.AddDate(0, 0, 1).Format("2006-01-02"), token),
		),
		menu.Row(
			menu.Data("Неделя", "schedule_week", date.Format("2006-01-02"), "7", token),
			menu.Data(
				"Выбрать дату",
				"open_calendar",
				date.Format("2006-01"),
				"d",
				date.Format("2006-01-02"),
				"1",
				token,
			),
		),
	}
	if canExport {
		rows = append(rows, menu.Row(menu.Data(
			"Скачать расписание",
			"open_schedule_exports",
			token,
			date.Format("2006-01-02"),
			"1",
		)))
	}
	if groupChat {
		rows = append(rows, menu.Row(menu.Data(targetLabel, "open_schedule_group")))
	} else {
		rows = append(rows, menu.Row(
			menu.Data(targetLabel, targetAction, token),
			menu.Data("Главное меню", "open_main_menu"),
		))
	}
	menu.Inline(rows...)
	return menu
}

func ScheduleWeekNavigation(from time.Time, groupName string, groupChat bool, groupID string, daysCount int, reference ...string) *tgbotapi.ReplyMarkup {
	token := firstArgument(reference)
	if token == "" {
		token = scheduleGroupToken(groupID)
	}
	return scheduleWeekNavigation(from, "Группа: "+groupName, "open_schedule_group", groupChat, groupID != "", daysCount, token)
}

func TeacherScheduleWeekNavigation(from time.Time, teacherName string, daysCount int, reference string) *tgbotapi.ReplyMarkup {
	return scheduleWeekNavigation(from, "Преподаватель: "+teacherName, "search_teacher_again", false, true, daysCount, reference)
}

func scheduleWeekNavigation(
	from time.Time,
	targetLabel string,
	targetAction string,
	groupChat bool,
	canExport bool,
	daysCount int,
	token string,
) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	step := 7
	periodLabel := "Неделя"
	currentLabel := "Текущая"
	if daysCount == 14 {
		step = 14
		periodLabel = "2 недели"
		currentLabel = "Текущие"
	}
	if groupChat {
		targetLabel = "Настройки чата"
	}
	rows := []tgbotapi.Row{
		menu.Row(
			menu.Data("← "+periodLabel, "schedule_week", from.AddDate(0, 0, -step).Format("2006-01-02"), fmt.Sprint(daysCount), token),
			menu.Data(currentLabel, "schedule_week", time.Now().In(from.Location()).Format("2006-01-02"), fmt.Sprint(daysCount), token),
			menu.Data(periodLabel+" →", "schedule_week", from.AddDate(0, 0, step).Format("2006-01-02"), fmt.Sprint(daysCount), token),
		),
		menu.Row(
			menu.Data("Выбрать день", "open_weekday", from.Format("2006-01-02"), fmt.Sprint(daysCount), token),
			menu.Data(
				"Выбрать дату",
				"open_calendar",
				from.Format("2006-01"),
				"w",
				from.Format("2006-01-02"),
				fmt.Sprint(daysCount),
				token,
			),
		),
	}
	if daysCount == 14 {
		rows = append(rows, menu.Row(menu.Data("Одна неделя", "schedule_week", from.Format("2006-01-02"), "7", token)))
	} else {
		rows = append(rows, menu.Row(menu.Data("Две недели", "schedule_week", from.Format("2006-01-02"), "14", token)))
	}
	if canExport {
		rows = append(rows, menu.Row(menu.Data(
			"Скачать расписание",
			"open_schedule_exports",
			token,
			from.Format("2006-01-02"),
			fmt.Sprint(daysCount),
		)))
	}
	if groupChat {
		rows = append(rows, menu.Row(menu.Data(targetLabel, "open_schedule_group")))
	} else {
		rows = append(rows, menu.Row(
			menu.Data(targetLabel, targetAction, token),
			menu.Data("Главное меню", "open_main_menu"),
		))
	}
	menu.Inline(rows...)
	return menu
}

func ScheduleExportFormats(groupToken, from string, daysCount int) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	days := fmt.Sprint(daysCount)
	menu.Inline(
		menu.Row(menu.Data("Изображение PNG", "download_schedule", "png", groupToken, from, days)),
		menu.Row(menu.Data("Данные JSON", "download_schedule", "json", groupToken, from, days)),
		menu.Row(menu.Data("Таблица CSV", "download_schedule", "csv", groupToken, from, days)),
		menu.Row(menu.Data("Календарь ICS", "download_schedule", "ics", groupToken, from, days)),
		menu.Row(menu.Data("Назад к расписанию", "back_to_schedule", groupToken, from, days)),
	)
	return menu
}

func ScheduleExportResultNavigation(groupToken, from string, daysCount int) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data(
		"Назад к форматам",
		"open_schedule_exports",
		groupToken,
		from,
		fmt.Sprint(daysCount),
	)))
	return menu
}

func GroupToken(groupID string) string {
	digest := sha256.Sum256([]byte(groupID))
	return fmt.Sprintf("%x", digest[:8])
}

func TeacherToken(universityID, teacherName string) string {
	value := strings.ToLower(strings.Join(strings.Fields(teacherName), " "))
	digest := sha256.Sum256([]byte(universityID + "\x00" + value))
	return "t" + fmt.Sprintf("%x", digest[:8])
}

func scheduleGroupToken(groupID string) string {
	if groupID == "" {
		return ""
	}
	return GroupToken(groupID)
}

func firstArgument(arguments []string) string {
	if len(arguments) == 0 {
		return ""
	}
	return arguments[0]
}

func UniversityToken(universityID string) string {
	digest := sha256.Sum256([]byte(universityID))
	return "~" + fmt.Sprintf("%x", digest[:8])
}

func IsUniversityToken(value string) bool {
	if len(value) != 17 || value[0] != '~' {
		return false
	}
	for _, char := range value[1:] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func ChatSettings(groupName string, isAdmin bool, format domain.ScheduleViewFormat) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	rows := []tgbotapi.Row{
		menu.Row(menu.Data("Расписание на сегодня", "schedule_date", time.Now().Format("2006-01-02"))),
	}
	if isAdmin {
		formatLabel := "Формат: текст"
		nextFormat := string(domain.ScheduleViewVisual)
		if format == domain.ScheduleViewVisual {
			formatLabel = "Формат: таблица"
			nextFormat = string(domain.ScheduleViewCompact)
		}
		rows = append(rows,
			menu.Row(menu.Data(formatLabel, "set_chat_schedule_view", nextFormat)),
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

func UnsetChatConfirmation(intentToken string) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("Да, удалить привязку", "confirm_unset_chat_group", intentToken)),
		menu.Row(menu.Data("Отмена", "cancel_unset_chat_group", intentToken)),
	)
	return menu
}

func DeleteProfileConfirmation(token string) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	menu.Inline(menu.Row(
		menu.Data("Удалить мои данные", "confirm_delete_profile", token),
		menu.Data("Отмена", "cancel_delete_profile", token),
	))
	return menu
}
