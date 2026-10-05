package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type DailyRepository struct{ db *sqlx.DB }

func NewDailyRepository(db *sqlx.DB) *DailyRepository { return &DailyRepository{db: db} }

func (r *DailyRepository) EnqueueDue(ctx context.Context, now time.Time, limit int) (int, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var recipients []struct {
		UserID    string          `db:"user_id"`
		Role      domain.UserRole `db:"role"`
		GroupID   string          `db:"group_id"`
		TeacherID string          `db:"teacher_id"`
		Timezone  string          `db:"timezone"`
		Clock     string          `db:"clock"`
	}
	err = tx.SelectContext(ctx, &recipients, `SELECT u.id AS user_id,u.role,
		CASE WHEN u.role='student' THEN COALESCE(u.default_group_id,'') ELSE '' END AS group_id,
		CASE WHEN u.role='teacher' THEN COALESCE(u.teacher_id,'') ELSE '' END AS teacher_id,
		scheduler_profile_timezone(u) AS timezone,to_char(u.daily_time,'HH24:MI') AS clock
		FROM users u WHERE u.daily_enabled AND NOT u.bot_blocked AND u.daily_next_at<=$1
		ORDER BY u.daily_next_at,u.id FOR UPDATE OF u SKIP LOCKED LIMIT $2`, now, limit)
	if err != nil {
		return 0, err
	}
	for _, recipient := range recipients {
		location, loadErr := time.LoadLocation(recipient.Timezone)
		if loadErr != nil {
			return 0, loadErr
		}
		local := now.In(location)
		date := local.Format(time.DateOnly)
		due, parseErr := time.ParseInLocation("2006-01-02 15:04", date+" "+recipient.Clock, location)
		if parseErr != nil {
			return 0, parseErr
		}
		if !now.Before(due) && (recipient.GroupID != "" || recipient.TeacherID != "") {
			expires := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, location)
			payload, _ := json.Marshal(domain.DailyContext{Date: date, Timezone: recipient.Timezone})
			_, err = tx.ExecContext(ctx, `INSERT INTO bot_outbox(id,user_id,group_id,teacher_id,kind,body,expires_at,schedule_context)
				VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),'daily_schedule','Daily schedule',$5,$6::jsonb)
				ON CONFLICT(id) DO UPDATE SET group_id=EXCLUDED.group_id,teacher_id=EXCLUDED.teacher_id,
				expires_at=EXCLUDED.expires_at,schedule_context=EXCLUDED.schedule_context,schedule_messages='[]'::jsonb,
				status='pending',attempts=0,claim_token='',lease_expires_at=NULL,next_attempt_at=NOW(),cancel_requested_at=NULL,cancel_reason='',updated_at=NOW()
				WHERE (bot_outbox.status='cancelled' OR (bot_outbox.status='pending' AND bot_outbox.cancel_requested_at IS NOT NULL AND (bot_outbox.claim_token='' OR bot_outbox.lease_expires_at<=clock_timestamp()))) AND bot_outbox.delivered_parts=0`,
				"daily:"+recipient.UserID+":"+date, recipient.UserID, recipient.GroupID, recipient.TeacherID, expires, payload)
			if err != nil {
				return 0, err
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE users SET daily_next_at=CASE WHEN EXISTS(SELECT 1 FROM bot_outbox WHERE id='daily:'||users.id||':'||to_char($3::timestamptz AT TIME ZONE $2,'YYYY-MM-DD') AND status='pending' AND cancel_requested_at IS NOT NULL AND delivered_parts=0) THEN $3::timestamptz+INTERVAL '1 minute' ELSE scheduler_next_daily(daily_time,$2,$3) END WHERE id=$1`, recipient.UserID, recipient.Timezone, now); err != nil {
			return 0, err
		}
	}
	return len(recipients), tx.Commit()
}

func (r *NotificationRepository) SaveScheduleMessages(ctx context.Context, id, token string, messages []domain.ScheduleMessage) error {
	raw, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE bot_outbox SET schedule_messages=$3::jsonb WHERE id=$1 AND claim_token=$2
		AND status='pending' AND lease_expires_at>clock_timestamp() AND delivered_parts=0`, id, token, raw)
	if err != nil {
		return err
	}
	return r.requireNotificationClaim(ctx, result, "bot_outbox", id)
}

func (r *NotificationRepository) SaveDeliveredParts(ctx context.Context, id, token string, count int) error {
	result, err := r.db.ExecContext(ctx, `UPDATE bot_outbox SET delivered_parts=$3 WHERE id=$1 AND claim_token=$2
		AND status='pending' AND lease_expires_at>clock_timestamp()`, id, token, count)
	if err != nil {
		return err
	}
	return r.requireNotificationClaim(ctx, result, "bot_outbox", id)
}
