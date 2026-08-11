package handlers

import "testing"

func TestParseQuietHours(t *testing.T) {
	enabled, start, end, err := parseQuietHours("22:00-07:00", "", "")
	if err != nil || !enabled || start != "22:00" || end != "07:00" {
		t.Fatalf("unexpected result: %t %s %s %v", enabled, start, end, err)
	}
	if _, _, _, err = parseQuietHours("25:00-07:00", "", ""); err == nil {
		t.Fatal("invalid time accepted")
	}
}
