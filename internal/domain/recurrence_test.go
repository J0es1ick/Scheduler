package domain

import (
	"testing"
	"time"
)

func TestRecurrenceMatchesCivilDatesAcrossTimezones(t *testing.T) {
	anchor := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	moscow := time.FixedZone("MSK", 3*60*60)
	rule := RecurrenceRule{CycleLength: 2, CycleWeeks: []int{1}, AnchorDate: &anchor}

	if !rule.Matches(time.Date(2026, 9, 14, 0, 0, 0, 0, moscow), nil) {
		t.Fatal("same civil recurrence week must match across timezones")
	}
	if rule.Matches(time.Date(2026, 9, 7, 0, 0, 0, 0, moscow), nil) {
		t.Fatal("opposite civil recurrence week must not match across timezones")
	}
}
