package handlers

import (
	"testing"
	"time"
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
