package keyboards

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	tele "gopkg.in/telebot.v3"
)

func TestSubgroupSettingsExposeEverySupportedSubgroup(t *testing.T) {
	item := domain.GroupSubscription{GroupID: "group", Subgroup: 100}
	seen := map[string]bool{}
	for page := range 10 {
		menu := SubgroupSettings(item, 0, page)
		for _, row := range menu.InlineKeyboard {
			for _, button := range row {
				if len([]byte(button.Data)) > 64 {
					t.Fatal("subgroup callback exceeds Telegram limit")
				}
				seen[button.Text] = true
			}
		}
	}
	for subgroup := 1; subgroup <= 100; subgroup++ {
		label := fmt.Sprintf("Подгруппа %d", subgroup)
		if subgroup == 100 {
			label = "● " + label
		}
		if !seen[label] {
			t.Errorf("subgroup %d cannot be selected", subgroup)
		}
	}
}

func TestMainMenuKeepsOnlyFrequentActions(t *testing.T) {
	menu := MainMenu()
	want := [][]string{
		{"Сегодня", "Завтра"},
		{"Неделя", "Выбрать дату"},
		{"Поиск", "Мои группы"},
		{"Ещё"},
	}
	if len(menu.ReplyKeyboard) != len(want) {
		t.Fatalf("unexpected main menu rows: %d", len(menu.ReplyKeyboard))
	}
	for rowIndex, row := range want {
		if len(menu.ReplyKeyboard[rowIndex]) != len(row) {
			t.Fatalf("unexpected buttons in row %d", rowIndex)
		}
		for columnIndex, text := range row {
			if menu.ReplyKeyboard[rowIndex][columnIndex].Text != text {
				t.Errorf(
					"button %d:%d = %q, want %q",
					rowIndex,
					columnIndex,
					menu.ReplyKeyboard[rowIndex][columnIndex].Text,
					text,
				)
			}
		}
	}
	if !menu.ResizeKeyboard || menu.OneTimeKeyboard || !menu.IsPersistent {
		t.Fatal("main menu must remain available outside schedule messages")
	}
}

func TestSubscriptionSettingsPaginatesAndDoesNotDeleteImmediately(t *testing.T) {
	items := make([]domain.GroupSubscription, 10)
	for index := range items {
		items[index] = domain.GroupSubscription{
			GroupID:        fmt.Sprintf("g%d", index),
			GroupName:      fmt.Sprintf("1/%d", index),
			UniversityName: "ИГХТУ",
		}
	}
	menu := SubscriptionSettings(items, true, false, 15, 0)
	groupButtons := 0
	for _, row := range menu.InlineKeyboard {
		for _, button := range row {
			if button.Unique == "open_subscription" {
				groupButtons++
			}
			if button.Unique == "confirm_delete_subscription" || button.Unique == "delete_subscription" {
				t.Fatal("subscription list must not contain an immediate delete action")
			}
		}
	}
	if groupButtons != 7 {
		t.Fatalf("first subscription page contains %d groups, want 7", groupButtons)
	}
}

func TestScheduleNavigationContainsDateAndGroupActions(t *testing.T) {
	date := time.Date(2026, time.September, 2, 0, 0, 0, 0, time.Local)
	menu := ScheduleDayNavigation(date, "3/147", false, "isuct:group:3/147")
	seen := map[string]bool{}
	for _, row := range menu.InlineKeyboard {
		for _, button := range row {
			seen[button.Unique] = true
		}
	}
	for _, action := range []string{"schedule_date", "schedule_week", "open_calendar", "open_schedule_group"} {
		if !seen[action] {
			t.Errorf("schedule navigation has no %s action", action)
		}
	}
	if len(menu.InlineKeyboard) != 4 {
		t.Fatalf("daily schedule navigation has %d rows, want 4", len(menu.InlineKeyboard))
	}
	if menu.InlineKeyboard[1][1].Text != "Выбрать дату" {
		t.Fatalf("calendar action label = %q", menu.InlineKeyboard[1][1].Text)
	}
}

func TestScheduleDownloadCallbackUsesBoundedGroupToken(t *testing.T) {
	groupID := strings.Repeat("external-connector-group-", 5)
	menu := ScheduleDayNavigation(time.Now(), "Длинная группа", false, groupID)
	var data string
	for _, row := range menu.InlineKeyboard {
		for _, button := range row {
			if button.Unique == "open_schedule_exports" {
				data = button.Data
			}
		}
	}
	if data == "" || !strings.HasPrefix(data, GroupToken(groupID)) {
		t.Fatalf("download callback does not contain the group token: %q", data)
	}
	if len(GroupToken(groupID)) != 16 {
		t.Fatalf("group token has unexpected length: %q", GroupToken(groupID))
	}
}

func TestScheduleWeekNavigationUsesSelectedPeriodStep(t *testing.T) {
	from := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.Local)
	tests := []struct {
		name          string
		days          int
		previousDate  string
		nextDate      string
		previousLabel string
		nextLabel     string
	}{
		{name: "week", days: 7, previousDate: "2026-08-31", nextDate: "2026-09-14", previousLabel: "← Неделя", nextLabel: "Неделя →"},
		{name: "two weeks", days: 14, previousDate: "2026-08-24", nextDate: "2026-09-21", previousLabel: "← 2 недели", nextLabel: "2 недели →"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			menu := ScheduleWeekNavigation(from, "3/147", false, "group-id", test.days)
			row := menu.InlineKeyboard[0]
			suffix := "|" + fmt.Sprint(test.days) + "|" + GroupToken("group-id")
			if row[0].Data != test.previousDate+suffix || row[2].Data != test.nextDate+suffix {
				t.Fatalf("period callbacks = %q and %q", row[0].Data, row[2].Data)
			}
			if row[0].Text != test.previousLabel || row[2].Text != test.nextLabel {
				t.Fatalf("period labels = %q and %q", row[0].Text, row[2].Text)
			}
		})
	}
}

func TestScheduleExportMenuListsFilesAndReturnsToSchedule(t *testing.T) {
	menu := ScheduleExportFormats("group-token", "2026-09-07", 7)
	want := []string{"download_schedule", "download_schedule", "download_schedule", "download_schedule", "back_to_schedule"}
	if len(menu.InlineKeyboard) != len(want) {
		t.Fatalf("export rows = %d, want %d", len(menu.InlineKeyboard), len(want))
	}
	for index, unique := range want {
		if len(menu.InlineKeyboard[index]) != 1 || menu.InlineKeyboard[index][0].Unique != unique {
			t.Fatalf("export row %d = %#v, want %s", index, menu.InlineKeyboard[index], unique)
		}
	}
	if menu.InlineKeyboard[3][0].Data != "ics|group-token|2026-09-07|7" {
		t.Fatalf("calendar export callback = %q", menu.InlineKeyboard[3][0].Data)
	}
	if menu.InlineKeyboard[4][0].Data != "group-token|2026-09-07|7" {
		t.Fatalf("back callback data = %q", menu.InlineKeyboard[4][0].Data)
	}
}

func TestNestedMenusExposeBackNavigation(t *testing.T) {
	tests := []struct {
		name   string
		menu   *tele.ReplyMarkup
		unique string
	}{
		{name: "search type", menu: SearchTypeSelector("search-flow"), unique: "cancel_search_type"},
		{name: "hotline type", menu: HotlineTypeSelector("hotline-flow"), unique: "cancel_hotline_type"},
		{name: "group input", menu: BackButton("cancel_group_change", "main"), unique: "cancel_group_change"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lastRow := test.menu.InlineKeyboard[len(test.menu.InlineKeyboard)-1]
			if len(lastRow) != 1 || lastRow[0].Text != "Назад" || lastRow[0].Unique != test.unique {
				t.Fatalf("last row = %#v, want back action %s", lastRow, test.unique)
			}
		})
	}
}

func TestWeekDaySelectorReturnsToDisplayedWeek(t *testing.T) {
	from := time.Date(2026, time.September, 9, 0, 0, 0, 0, time.Local)
	menu := WeekDaySelector(from, 14, "group-token")
	lastRow := menu.InlineKeyboard[len(menu.InlineKeyboard)-1]
	if len(lastRow) != 1 || lastRow[0].Text != "Назад" {
		t.Fatalf("last weekday row must contain the back button: %#v", lastRow)
	}
	if lastRow[0].Unique != "schedule_week" || lastRow[0].Data != "2026-09-09|14|group-token" {
		t.Fatalf("back callback = %q %q", lastRow[0].Unique, lastRow[0].Data)
	}
	buttons := 0
	for _, row := range menu.InlineKeyboard[:len(menu.InlineKeyboard)-1] {
		for _, button := range row {
			buttons++
			if button.Unique != "schedule_period_date" {
				t.Fatalf("weekday callback %q does not select an exact date", button.Unique)
			}
		}
	}
	if buttons != 14 {
		t.Fatalf("two-week selector has %d dates, want 14", buttons)
	}
}

func TestNestedCalendarReturnsToItsScheduleContext(t *testing.T) {
	month := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.Local)
	backDate := time.Date(2026, time.September, 9, 0, 0, 0, 0, time.Local)
	menu := ScheduleCalendarWithBack(month, "schedule_week", backDate, "14", "group-token")

	lastRow := menu.InlineKeyboard[len(menu.InlineKeyboard)-1]
	if len(lastRow) != 1 || lastRow[0].Text != "Назад" {
		t.Fatalf("nested calendar must end with back: %#v", lastRow)
	}
	if lastRow[0].Unique != "schedule_week" || lastRow[0].Data != "2026-09-09|14|group-token" {
		t.Fatalf("calendar back callback = %q %q", lastRow[0].Unique, lastRow[0].Data)
	}

	monthNavigation := menu.InlineKeyboard[len(menu.InlineKeyboard)-2]
	if monthNavigation[0].Data != "2026-08|w|2026-09-09|14|group-token" ||
		monthNavigation[2].Data != "2026-10|w|2026-09-09|14|group-token" {
		t.Fatalf("month navigation lost return context: %#v", monthNavigation)
	}
	for _, row := range menu.InlineKeyboard[1 : len(menu.InlineKeyboard)-2] {
		for _, button := range row {
			if button.Text == "·" {
				continue
			}
			if button.Unique != "schedule_period_date" || !strings.HasSuffix(button.Data, "|2026-09-09|14|group-token") {
				t.Fatalf("calendar day lost two-week context: %#v", button)
			}
		}
	}
	if monthNavigation[1].Unique != "schedule_period_date" ||
		!strings.HasSuffix(monthNavigation[1].Data, "|2026-09-09|14|group-token") {
		t.Fatalf("today button lost two-week context: %#v", monthNavigation[1])
	}
}

func TestStandaloneCalendarCanBeClosed(t *testing.T) {
	menu := ScheduleCalendar(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.Local))
	lastRow := menu.InlineKeyboard[len(menu.InlineKeyboard)-1]
	if len(lastRow) != 1 || lastRow[0].Text != "Закрыть" || lastRow[0].Unique != "close_inline" {
		t.Fatalf("standalone calendar must keep close action: %#v", lastRow)
	}
}

func TestAllSubscriptionCallbacksFitTelegramLimit(t *testing.T) {
	item := domain.GroupSubscription{
		GroupID:            strings.Repeat("connector-group-identifier-", 6),
		GroupName:          strings.Repeat("Очень длинная группа ", 4),
		UniversityName:     "Университет",
		IsActive:           true,
		ScheduleViewFormat: domain.ScheduleViewVisual,
	}
	menus := []*tele.ReplyMarkup{
		SubscriptionSettings([]domain.GroupSubscription{item}, true, true, 15, 0),
		SubscriptionActions(item, 0),
		ScheduleViewSettings(item, 0),
		SubgroupSettings(item, 0),
		DeleteSubscriptionConfirmation(GroupToken(item.GroupID), 0, "intent-token"),
		ScheduleExportFormats(GroupToken(item.GroupID), "2026-09-07", 14),
		ScheduleExportResultNavigation(GroupToken(item.GroupID), "2026-09-07", 14),
		ScheduleDayNavigation(time.Now(), item.GroupName, false, item.GroupID),
		ScheduleWeekNavigation(time.Now(), item.GroupName, false, item.GroupID, 14),
		ScheduleCalendarWithBack(time.Now(), "schedule_week", time.Now(), "14", GroupToken(item.GroupID)),
		ScheduleCalendarWithBack(time.Now(), "schedule_date", time.Now(), "1", GroupToken(item.GroupID)),
		WeekDaySelector(time.Now(), 14, GroupToken(item.GroupID)),
		SchedulePeriodDateBack(time.Now(), 14, GroupToken(item.GroupID)),
		ScheduleWeekNavigation(time.Now(), item.GroupName, false, item.GroupID, 14, "p"+GroupToken(item.GroupID)),
		ScheduleCalendarWithBack(time.Now(), "schedule_week", time.Now(), "14", "p"+GroupToken(item.GroupID)),
		WeekDaySelector(time.Now(), 14, "p"+GroupToken(item.GroupID)),
		ScheduleExportFormats("p"+GroupToken(item.GroupID), "2026-09-07", 14),
		TeacherMatches([]string{"Константинов Е.С.", "Константинов А.В."}, "teacher-flow"),
		TeacherScheduleDayNavigation(time.Now(), "Константинов Е.С.", TeacherToken("isuct", "Константинов Е.С.")),
		TeacherScheduleWeekNavigation(time.Now(), "Константинов Е.С.", 14, TeacherToken("isuct", "Константинов Е.С.")),
		SearchScheduleViewSettings(domain.ScheduleViewVisual),
	}
	for _, menu := range menus {
		for _, row := range menu.InlineKeyboard {
			for _, button := range row {
				payload := "\f" + button.Unique
				if button.Data != "" {
					payload += "|" + button.Data
				}
				if len([]byte(payload)) > 64 {
					t.Fatalf("callback %q is %d bytes", payload, len([]byte(payload)))
				}
			}
		}
	}
}

func TestUniversitySelectorCallbackFitsTelegramLimit(t *testing.T) {
	universityID := strings.Repeat("u", 63)
	menu := UniversitySelector([]domain.University{{ID: universityID, Name: "Университет"}}, strings.Repeat("n", 16))
	button := menu.InlineKeyboard[0][0]
	payload := "\f" + button.Unique + "|" + button.Data
	if len([]byte(payload)) > 64 {
		t.Fatalf("university callback is %d bytes: %q", len([]byte(payload)), payload)
	}
	if strings.Contains(button.Data, universityID) || !strings.HasPrefix(button.Data, UniversityToken(universityID)+"|") {
		t.Fatalf("university callback does not use a stable token: %q", button.Data)
	}
}
