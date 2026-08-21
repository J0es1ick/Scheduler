package keyboards

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

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
			if button.Unique == "download_schedule_png" {
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

func TestWeekDaySelectorReturnsToDisplayedWeek(t *testing.T) {
	from := time.Date(2026, time.September, 9, 0, 0, 0, 0, time.Local)
	menu := WeekDaySelector(from)
	lastRow := menu.InlineKeyboard[len(menu.InlineKeyboard)-1]
	if len(lastRow) != 1 || lastRow[0].Text != "Назад" {
		t.Fatalf("last weekday row must contain the back button: %#v", lastRow)
	}
	if lastRow[0].Unique != "schedule_week" || lastRow[0].Data != "2026-09-09" {
		t.Fatalf("back callback = %q %q", lastRow[0].Unique, lastRow[0].Data)
	}
	for _, row := range menu.InlineKeyboard[:len(menu.InlineKeyboard)-1] {
		for _, button := range row {
			if !strings.HasSuffix(button.Data, "|2026-09-09") {
				t.Fatalf("weekday callback %q does not preserve displayed week", button.Data)
			}
		}
	}
}

func TestNestedCalendarReturnsToItsScheduleContext(t *testing.T) {
	month := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.Local)
	backDate := time.Date(2026, time.September, 9, 0, 0, 0, 0, time.Local)
	menu := ScheduleCalendarWithBack(month, "schedule_week", backDate)

	lastRow := menu.InlineKeyboard[len(menu.InlineKeyboard)-1]
	if len(lastRow) != 1 || lastRow[0].Text != "Назад" {
		t.Fatalf("nested calendar must end with back: %#v", lastRow)
	}
	if lastRow[0].Unique != "schedule_week" || lastRow[0].Data != "2026-09-09" {
		t.Fatalf("calendar back callback = %q %q", lastRow[0].Unique, lastRow[0].Data)
	}

	monthNavigation := menu.InlineKeyboard[len(menu.InlineKeyboard)-2]
	if monthNavigation[0].Data != "2026-08|schedule_week|2026-09-09" ||
		monthNavigation[2].Data != "2026-10|schedule_week|2026-09-09" {
		t.Fatalf("month navigation lost return context: %#v", monthNavigation)
	}
}

func TestStandaloneCalendarCanBeClosed(t *testing.T) {
	menu := ScheduleCalendar(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.Local))
	lastRow := menu.InlineKeyboard[len(menu.InlineKeyboard)-1]
	if len(lastRow) != 1 || lastRow[0].Text != "Закрыть" || lastRow[0].Unique != "close_inline" {
		t.Fatalf("standalone calendar must keep close action: %#v", lastRow)
	}
}
