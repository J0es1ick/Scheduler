package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type SubscriptionRepository struct {
	db *sqlx.DB
}

func NewSubscriptionRepository(db *sqlx.DB) *SubscriptionRepository {
	return &SubscriptionRepository{db: db}
}

func (r *SubscriptionRepository) UpsertSubscription(ctx context.Context, id, userID, objectID, objectType string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO subscriptions (id, user_id, object_id, object_type, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (user_id, object_id, object_type)
		 DO UPDATE SET updated_at = EXCLUDED.updated_at`,
		id, userID, objectID, objectType, now, now)
	if err != nil {
		return fmt.Errorf("upsert subscription user=%s obj=%s/%s: %w", userID, objectType, objectID, err)
	}
	return nil
}

func (r *SubscriptionRepository) GetGroupSubscriptions(ctx context.Context, userID string) ([]domain.GroupSubscription, error) {
	var items []domain.GroupSubscription
	err := r.db.SelectContext(ctx, &items, `
		SELECT s.id, s.user_id, g.id AS group_id, g.name AS group_name,
			g.university_id, u.name AS university_name,
			(users.default_group_id = g.id) AS is_default,
			s.created_at, s.updated_at
		FROM subscriptions s
		JOIN users ON users.id = s.user_id
		JOIN groups g ON g.id = s.object_id AND g.is_active
		JOIN universities u ON u.id = g.university_id
		WHERE s.user_id = $1 AND s.object_type = 'group'
		ORDER BY is_default DESC, s.updated_at DESC, u.name, g.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("get group subscriptions for user %s: %w", userID, err)
	}
	return items, nil
}

func (r *SubscriptionRepository) HasGroupSubscription(ctx context.Context, userID, groupID string) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, `
		SELECT EXISTS (
			SELECT 1 FROM subscriptions
			WHERE user_id=$1 AND object_id=$2 AND object_type='group'
		)`, userID, groupID)
	if err != nil {
		return false, fmt.Errorf("check group subscription user=%s group=%s: %w", userID, groupID, err)
	}
	return exists, nil
}

func (r *SubscriptionRepository) GetSubscriptionByID(ctx context.Context, id string) (*domain.Subscription, error) {
	var sub domain.Subscription
	err := r.db.GetContext(ctx, &sub,
		`SELECT id, user_id, object_id, object_type, created_at, updated_at FROM subscriptions WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get subscription %s: %w", id, err)
	}
	return &sub, nil
}

func (r *SubscriptionRepository) GetSubscriptionsByUserID(ctx context.Context, userID string) ([]domain.Subscription, error) {
	var subs []domain.Subscription
	err := r.db.SelectContext(ctx, &subs,
		`SELECT id, user_id, object_id, object_type, created_at, updated_at
		 FROM subscriptions WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("get subscriptions for user %s: %w", userID, err)
	}
	return subs, nil
}

// DeleteSubscriptionByObject удаляет подписку пользователя на конкретный объект.
func (r *SubscriptionRepository) DeleteSubscriptionByObject(ctx context.Context, userID, objectID, objectType string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete subscription user=%s obj=%s/%s: begin: %w", userID, objectType, objectID, err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx,
		`DELETE FROM subscriptions WHERE user_id = $1 AND object_id = $2 AND object_type = $3`,
		userID, objectID, objectType)
	if err != nil {
		return fmt.Errorf("delete subscription user=%s obj=%s/%s: %w", userID, objectType, objectID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if objectType == "group" {
		if _, err = tx.ExecContext(ctx, `
			UPDATE notification_deliveries d
			SET status='cancelled', updated_at=NOW()
			FROM schedule_change_events e
			WHERE d.event_id=e.id AND d.user_id=$1 AND e.group_id=$2
				AND d.status='pending'`, userID, objectID); err != nil {
			return fmt.Errorf("cancel pending notifications user=%s group=%s: %w", userID, objectID, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("delete subscription user=%s obj=%s/%s: commit: %w", userID, objectType, objectID, err)
	}
	return nil
}

func (r *SubscriptionRepository) DeleteSubscription(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete subscription %s: %w", id, err)
	}
	return nil
}

func (r *SubscriptionRepository) GetUserIDsByObject(ctx context.Context, objectID, objectType string) ([]string, error) {
	var ids []string
	err := r.db.SelectContext(ctx, &ids,
		`SELECT user_id FROM subscriptions WHERE object_id = $1 AND object_type = $2`,
		objectID, objectType)
	if err != nil {
		return nil, fmt.Errorf("get subscribers for %s/%s: %w", objectType, objectID, err)
	}
	return ids, nil
}
