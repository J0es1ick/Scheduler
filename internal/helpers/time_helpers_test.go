package helpers

import (
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestWeekday(t *testing.T) {
	// Воскресенье должно быть 7, а не 0
	sunday := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	if Weekday(sunday) != 7 {
		t.Errorf("Sunday: got %d, want 7", Weekday(sunday))
	}

	// Понедельник = 1
	monday := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	if Weekday(monday) != 1 {
		t.Errorf("Monday: got %d, want 1", Weekday(monday))
	}
}

func TestNormalizeDate(t *testing.T) {
	// Дата с временем должна стать полуночью
	dirty := time.Date(2026, 4, 25, 14, 32, 11, 999, time.UTC)
	clean := NormalizeDate(dirty)
	if clean.Hour() != 0 || clean.Minute() != 0 || clean.Second() != 0 {
		t.Errorf("NormalizeDate не обнулил время: %v", clean)
	}
}

func TestNormalizeDateEqual(t *testing.T) {
	// Две даты с разным временем должны совпадать после нормализации
	a := time.Date(2026, 4, 25, 14, 32, 11, 0, time.UTC)
	b := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	if !NormalizeDate(a).Equal(NormalizeDate(b)) {
		t.Errorf("После нормализации даты должны совпадать")
	}
}

func TestDetermineWeekType(t *testing.T) {
	start := time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC) // начало семестра = нечётная неделя

	week1 := time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC)
	if DetermineWeekType(week1, start) != domain.WeekTypeOdd {
		t.Errorf("Первая неделя должна быть нечётной")
	}

	week2 := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	if DetermineWeekType(week2, start) != domain.WeekTypeEven {
		t.Errorf("Вторая неделя должна быть чётной")
	}
}
