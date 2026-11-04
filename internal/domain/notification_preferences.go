package domain

import "time"

type NotificationPreferences struct {
	Changes         bool      `json:"changes"`
	DailyEnabled    bool      `json:"daily_enabled"`
	DailyTime       string    `json:"daily_time"`
	DailyDayOffset  int       `json:"daily_day_offset"`
	DailyWeekdays   int       `json:"daily_weekdays"`
	DailySkipEmpty  bool      `json:"daily_skip_empty"`
	PausedUntil     string    `json:"paused_until"`
	ReminderEnabled bool      `json:"reminder_enabled"`
	ReminderMinutes int       `json:"reminder_minutes"`
	QuietEnabled    bool      `json:"quiet_enabled"`
	QuietStart      string    `json:"quiet_start"`
	QuietEnd        string    `json:"quiet_end"`
	ServiceUpdates  *bool     `json:"service_updates"`
	Version         time.Time `json:"version"`
}

func (u User) NotificationPreferences() NotificationPreferences {
	pause := ""
	if u.NotificationsPausedUntil != nil {
		pause = u.NotificationsPausedUntil.Format(time.DateOnly)
	}
	return NotificationPreferences{Changes: u.NotificationsEnabled, DailyEnabled: u.DailyEnabled, DailyTime: u.DailyTime,
		DailyDayOffset: u.DailyDayOffset, DailyWeekdays: u.DailyWeekdays, DailySkipEmpty: u.DailySkipEmpty, PausedUntil: pause,
		ReminderEnabled: u.ReminderEnabled, ReminderMinutes: u.ReminderMinutes, QuietEnabled: u.QuietHoursEnabled,
		QuietStart: u.QuietHoursStart, QuietEnd: u.QuietHoursEnd, ServiceUpdates: u.ServiceUpdatesConsent, Version: u.UpdatedAt}
}

func (p NotificationPreferences) DailyOn(date time.Time) bool {
	day := (int(date.Weekday()) + 6) % 7
	return p.DailyEnabled && p.DailyWeekdays&(1<<day) != 0 && (p.PausedUntil == "" || date.Format(time.DateOnly) >= p.PausedUntil)
}

func (p NotificationPreferences) InQuietHours(moment time.Time) bool {
	if !p.QuietEnabled {
		return false
	}
	clock := moment.Format("15:04")
	if p.QuietStart < p.QuietEnd {
		return clock >= p.QuietStart && clock < p.QuietEnd
	}
	return clock >= p.QuietStart || clock < p.QuietEnd
}
