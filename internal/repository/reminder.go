package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type ReminderRepository struct {
	db *sqlx.DB
}

func NewReminderRepository(db *sqlx.DB) *ReminderRepository {
	return &ReminderRepository{db: db}
}

func (r *ReminderRepository) ActiveRecipientsPage(
	ctx context.Context,
	afterUserID string,
	limit int,
) ([]domain.ReminderRecipient, error) {
	if limit <= 0 {
		return []domain.ReminderRecipient{}, nil
	}
	var recipients []domain.ReminderRecipient
	if err := r.db.SelectContext(ctx, &recipients, `
		SELECT user_id,CASE WHEN role='student' THEN COALESCE(default_group_id,'') ELSE '' END AS group_id,
		CASE WHEN role='teacher' THEN COALESCE(teacher_id,'') ELSE '' END AS teacher_id,
		university_id,name AS group_name,university_name,timezone,subgroup,reminder_minutes
		FROM schedule_profile_recipients WHERE reminder_enabled AND NOT bot_blocked AND is_active
		AND ($1='' OR user_id>$1) ORDER BY user_id LIMIT $2`, afterUserID, limit); err != nil {
		return nil, fmt.Errorf("list active reminder recipients: %w", err)
	}
	if recipients == nil {
		recipients = []domain.ReminderRecipient{}
	}
	return recipients, nil
}

func (r *ReminderRepository) Enqueue(
	ctx context.Context,
	id string,
	userID string,
	groupID string,
	body string,
	slot domain.ReminderContext,
) error {
	raw, err := json.Marshal(slot)
	if err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO bot_outbox (id, user_id, group_id, teacher_id, kind, body, expires_at, reminder_context)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($7,''), 'lesson_reminder', $4, $5, $6::jsonb)
		ON CONFLICT (id) DO UPDATE SET
			body=EXCLUDED.body, expires_at=EXCLUDED.expires_at, reminder_context=EXCLUDED.reminder_context,
			status='pending', last_error='',
			attempts=CASE WHEN bot_outbox.status='cancelled' THEN 0 ELSE bot_outbox.attempts END,
			next_attempt_at=CASE WHEN bot_outbox.status='cancelled' THEN NOW() ELSE bot_outbox.next_attempt_at END,
			cancel_requested_at=NULL, cancel_reason='',
			group_id=EXCLUDED.group_id, teacher_id=EXCLUDED.teacher_id,
			updated_at=NOW()
		WHERE bot_outbox.kind='lesson_reminder' AND ((bot_outbox.status='pending' AND bot_outbox.claim_token='') OR bot_outbox.status='cancelled')`,
		id, userID, groupID, body, slot.StartsAt, raw, slot.TeacherID,
	); err != nil {
		return fmt.Errorf("enqueue lesson reminder %s: %w", id, err)
	}
	return nil
}
