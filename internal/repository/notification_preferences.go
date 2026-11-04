package repository

import (
	"context"
	"github.com/J0es1ick/Scheduler/internal/domain"
)

func (r *UserRepository) SaveNotificationPreferences(ctx context.Context, userID string, p domain.NotificationPreferences) error {
	result, err := r.db.ExecContext(ctx, `UPDATE users SET notifications_enabled=$2,daily_enabled=$3,daily_time=$4::time,
 daily_day_offset=$5,daily_weekdays=$6,daily_skip_empty=$7,notifications_paused_until=NULLIF($8,'')::date,
 reminder_enabled=$9,reminder_minutes=$10,quiet_hours_enabled=$11,quiet_hours_start=$12::time,quiet_hours_end=$13::time,
 service_updates_consent=$14,service_updates_answered_at=CASE WHEN $14::boolean IS DISTINCT FROM service_updates_consent THEN NOW() ELSE service_updates_answered_at END,
 daily_setup='done',updated_at=clock_timestamp() WHERE id=$1 AND updated_at=$15 AND NOT bot_blocked`,
		userID, p.Changes, p.DailyEnabled, p.DailyTime, p.DailyDayOffset, p.DailyWeekdays, p.DailySkipEmpty, p.PausedUntil, p.ReminderEnabled, p.ReminderMinutes, p.QuietEnabled, p.QuietStart, p.QuietEnd, p.ServiceUpdates, p.Version)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrPersonalConflict
	}
	return nil
}
