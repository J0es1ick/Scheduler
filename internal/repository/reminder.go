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
		SELECT u.id AS user_id, u.default_group_id AS group_id,
			g.name AS group_name, un.name AS university_name,
			un.timezone, COALESCE(s.subgroup, 0) AS subgroup,
			u.reminder_minutes
		FROM users u
		JOIN groups g ON g.id=u.default_group_id AND g.is_active
		JOIN universities un ON un.id=g.university_id AND un.is_active
		LEFT JOIN subscriptions s ON s.user_id=u.id
			AND s.object_id=u.default_group_id AND s.object_type='group'
		WHERE u.reminder_enabled AND NOT u.bot_blocked
			AND u.default_group_id IS NOT NULL
			AND ($1 = '' OR u.id > $1)
		ORDER BY u.id
		LIMIT $2`, afterUserID, limit); err != nil {
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
		INSERT INTO bot_outbox (id, user_id, group_id, kind, body, expires_at, reminder_context)
		VALUES ($1, $2, $3, 'lesson_reminder', $4, $5, $6::jsonb)
		ON CONFLICT (id) DO UPDATE SET
			body=EXCLUDED.body, expires_at=EXCLUDED.expires_at, reminder_context=EXCLUDED.reminder_context,
			status='pending', last_error='',
			attempts=CASE WHEN bot_outbox.status='cancelled' THEN 0 ELSE bot_outbox.attempts END,
			next_attempt_at=CASE WHEN bot_outbox.status='cancelled' THEN NOW() ELSE bot_outbox.next_attempt_at END,
			cancel_requested_at=NULL, cancel_reason='',
			group_id=EXCLUDED.group_id,
			updated_at=NOW()
		WHERE bot_outbox.kind='lesson_reminder' AND ((bot_outbox.status='pending' AND bot_outbox.claim_token='') OR bot_outbox.status='cancelled')`,
		id, userID, groupID, body, slot.StartsAt, raw,
	); err != nil {
		return fmt.Errorf("enqueue lesson reminder %s: %w", id, err)
	}
	return nil
}
