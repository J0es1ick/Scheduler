package service

import "testing"

func TestDailyTimeValidation(t *testing.T) {
	for _, value := range []string{"00:00", "06:00", "23:59", " 08:30 "} {
		if _, err := ParseDailyTime(value); err != nil {
			t.Fatalf("%q: %v", value, err)
		}
	}
	for _, value := range []string{"6:00", "24:00", "06:60", "06:00:00", "morning", ""} {
		if _, err := ParseDailyTime(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
