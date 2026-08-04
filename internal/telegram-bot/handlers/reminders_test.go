package handlers

import "testing"

func TestParseReminderSetting(t *testing.T) {
	tests := []struct {
		input          string
		currentMinutes int
		wantEnabled    bool
		wantMinutes    int
		wantError      bool
	}{
		{input: "45", currentMinutes: 15, wantEnabled: true, wantMinutes: 45},
		{input: "off", currentMinutes: 30, wantEnabled: false, wantMinutes: 30},
		{input: "вкл", currentMinutes: 60, wantEnabled: true, wantMinutes: 60},
		{input: "on", currentMinutes: 0, wantEnabled: true, wantMinutes: 15},
		{input: "4", currentMinutes: 15, wantError: true},
		{input: "181", currentMinutes: 15, wantError: true},
		{input: "час", currentMinutes: 15, wantError: true},
	}

	for _, test := range tests {
		enabled, minutes, err := parseReminderSetting(
			test.input,
			test.currentMinutes,
		)
		if (err != nil) != test.wantError {
			t.Errorf("parse %q error = %v, wantError %v", test.input, err, test.wantError)
		}
		if test.wantError {
			continue
		}
		if enabled != test.wantEnabled || minutes != test.wantMinutes {
			t.Errorf(
				"parse %q = (%v, %d), want (%v, %d)",
				test.input,
				enabled,
				minutes,
				test.wantEnabled,
				test.wantMinutes,
			)
		}
	}
}
