package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
)

func TestInlineQueryDates(t *testing.T) {
	now := time.Date(2026, time.August, 6, 15, 0, 0, 0, time.UTC)
	dates, ok := inlineQueryDates("", now)
	if !ok || len(dates) != 2 || dates[0].Day() != 6 || dates[1].Day() != 7 {
		t.Fatalf("unexpected default dates: %#v, %t", dates, ok)
	}
	dates, ok = inlineQueryDates("08.08.2026", now)
	if !ok || len(dates) != 1 || dates[0].Day() != 8 {
		t.Fatalf("unexpected explicit date: %#v, %t", dates, ok)
	}
	if _, ok = inlineQueryDates("не дата", now); ok {
		t.Fatal("invalid inline query accepted")
	}
}

func TestInlineScheduleTextPreservesEmptyDaysAndWholeLessons(t *testing.T) {
	target := &scheduleTarget{University: "Вуз <1>", GroupName: "4/147", Subgroup: 1}
	day := dto.DaySchedule{Date: time.Date(2026, time.September, 8, 0, 0, 0, 0, time.UTC)}
	text := inlineScheduleText(target, day, "")
	if !strings.Contains(text, "Занятий нет.") || !strings.Contains(text, "Вуз &lt;1&gt;") || !strings.Contains(text, "Подгруппа: 1") {
		t.Fatalf("empty day or escaped context missing: %s", text)
	}
	day.Lessons = []domain.Lesson{{Subject: "<Первый>", TimeStart: "08:00", TimeEnd: "09:35"}}
	text = inlineScheduleText(target, day, "")
	if !strings.Contains(text, "<b>&lt;Первый&gt;</b>") || strings.Contains(text, "Полное расписание") {
		t.Fatalf("short schedule altered: %s", text)
	}
	day.Lessons = append(day.Lessons, domain.Lesson{Subject: strings.Repeat("Длинное название ", tgMaxLen)})
	text = inlineScheduleText(target, day, "")
	if len([]rune(text)) > tgMaxLen || !strings.Contains(text, "<b>&lt;Первый&gt;</b>") || !strings.Contains(text, "Показано занятий: 1 из 2.") || strings.Contains(text, "Длинное название") {
		t.Fatalf("preview split a lesson or lost the remainder: %s", text)
	}
	day.Lessons = day.Lessons[1:]
	text = inlineScheduleText(target, day, "")
	if len([]rune(text)) > tgMaxLen || strings.Contains(text, "Занятий нет") || !strings.Contains(text, "Полное расписание") {
		t.Fatalf("oversized first lesson represented as an empty day: %s", text)
	}
}
