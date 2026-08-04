package repository

import (
	"context"
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
			u.reminder_minutes
		FROM users u
		JOIN groups g ON g.id=u.default_group_id AND g.is_active
		JOIN universities un ON un.id=g.university_id AND un.is_active
		WHERE u.reminder_enabled
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
) error {
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO bot_outbox (id, user_id, group_id, kind, body)
		VALUES ($1, $2, $3, 'lesson_reminder', $4)
		ON CONFLICT (id) DO UPDATE SET
			body=EXCLUDED.body,
			group_id=EXCLUDED.group_id,
			updated_at=NOW()
		WHERE bot_outbox.status='pending'`,
		id, userID, groupID, body,
	); err != nil {
		return fmt.Errorf("enqueue lesson reminder %s: %w", id, err)
	}
	return nil
}
