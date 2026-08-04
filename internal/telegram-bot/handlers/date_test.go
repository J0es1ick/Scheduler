package handlers

import (
	"testing"
	"time"
)

func TestParseScheduleDateFormats(t *testing.T) {
	location := time.FixedZone("test", 3*60*60)
	for input, expected := range map[string]string{
		"01.09.2026": "2026-09-01",
		"2026-09-01": "2026-09-01",
		"01.09.26":   "2026-09-01",
	} {
		date, err := parseScheduleDate(input, location)
		if err != nil {
			t.Fatalf("parse %q: %v", input, err)
		}
		if actual := date.Format("2006-01-02"); actual != expected {
			t.Errorf("parse %q = %s, want %s", input, actual, expected)
		}
	}
}

func TestParseScheduleDateRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", "31.02.2026", "01.09.1999", "завтро"} {
		if _, err := parseScheduleDate(input, time.UTC); err == nil {
			t.Errorf("parse %q succeeded, want error", input)
		}
	}
}
