package keyboards

import (
	"testing"
	"time"

	tele "gopkg.in/telebot.v3"
)

func TestWeekAndExportShareRowForGroupsAndTeachers(t *testing.T) {
	for _, days := range []int{7, 14} {
		for _, menu := range []*tele.ReplyMarkup{
			ScheduleWeekNavigation(time.Now(), "4/147", false, "group", days),
			ScheduleWeekNavigation(time.Now(), "4/147", true, "group", days),
			TeacherScheduleWeekNavigation(time.Now(), "Иванов И.И.", days, "teacher-token"),
		} {
			if len(menu.InlineKeyboard) != 4 {
				t.Fatalf("expected four rows, got %d", len(menu.InlineKeyboard))
			}
			row := menu.InlineKeyboard[2]
			if len(row) != 2 || row[0].Unique != "schedule_week" || row[1].Unique != "open_schedule_exports" {
				t.Fatalf("period/export row: %+v", row)
			}
			for _, row := range menu.InlineKeyboard {
				for _, button := range row {
					if button.Unique == "schedule_feedback" {
						t.Fatal("feedback button remains")
					}
				}
			}
		}
	}
}

func TestHotlineOffersGeneralFeedback(t *testing.T) {
	menu := HotlineTypeSelector("test")
	for _, row := range menu.InlineKeyboard {
		for _, button := range row {
			if button.Data == "feedback|test" {
				return
			}
		}
	}
	t.Fatal("general feedback option missing")
}
