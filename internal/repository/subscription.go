package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
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
	if objectType == "group" {
		return r.UpsertActiveGroupSubscription(ctx, id, userID, objectID)
	}
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

func (r *SubscriptionRepository) UpsertActiveGroupSubscription(
	ctx context.Context,
	id, userID, groupID string,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("subscribe active group: begin: %w", err)
	}
	defer tx.Rollback()
	if err = database.LockGroupReferences(ctx, tx, false); err != nil {
		return err
	}
	if err = lockActiveGroup(ctx, tx, groupID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO subscriptions (id, user_id, object_id, object_type, created_at, updated_at)
		VALUES ($1,$2,$3,'group',NOW(),NOW())
		ON CONFLICT (user_id, object_id, object_type)
		DO UPDATE SET updated_at=NOW()`, id, userID, groupID); err != nil {
		return fmt.Errorf("subscribe active group user=%s group=%s: %w", userID, groupID, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("subscribe active group: commit: %w", err)
	}
	return nil
}

func (r *SubscriptionRepository) SubscribeAndSetDefault(
	ctx context.Context,
	id, userID, groupID string,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("subscribe and set default: begin: %w", err)
	}
	defer tx.Rollback()
	if err = database.LockGroupReferences(ctx, tx, false); err != nil {
		return err
	}
	if err = lockActiveGroup(ctx, tx, groupID); err != nil {
		return err
	}
	if err = lockUser(ctx, tx, userID); err != nil {
		return err
	}
	var previousDefault sql.NullString
	if err = tx.GetContext(ctx, &previousDefault, `
		SELECT default_group_id FROM users WHERE id=$1`, userID); err != nil {
		return fmt.Errorf("subscribe and set default: load current group: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO subscriptions (id, user_id, object_id, object_type, created_at, updated_at)
		VALUES ($1,$2,$3,'group',NOW(),NOW())
		ON CONFLICT (user_id, object_id, object_type)
		DO UPDATE SET updated_at=NOW()`, id, userID, groupID); err != nil {
		return fmt.Errorf("subscribe and set default: upsert: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE users SET default_group_id=$2, updated_at=NOW() WHERE id=$1`, userID, groupID); err != nil {
		return fmt.Errorf("subscribe and set default: update user: %w", err)
	}
	if previousDefault.Valid && previousDefault.String != groupID {
		if _, err = tx.ExecContext(ctx, `
			SELECT scheduler_request_outbox_cancellation(
				$1, 'lesson_reminder', $2, 'default_group_changed'
			)`, userID, previousDefault.String); err != nil {
			return fmt.Errorf("subscribe and set default: cancel previous reminders: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("subscribe and set default: commit: %w", err)
	}
	return nil
}

func (r *SubscriptionRepository) SetDefaultSubscribedGroup(
	ctx context.Context,
	userID, groupID string,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set subscribed default: begin: %w", err)
	}
	defer tx.Rollback()
	if err = database.LockGroupReferences(ctx, tx, false); err != nil {
		return err
	}
	if err = lockActiveGroup(ctx, tx, groupID); err != nil {
		return err
	}
	if err = lockUser(ctx, tx, userID); err != nil {
		return err
	}
	var previousDefault sql.NullString
	if err = tx.GetContext(ctx, &previousDefault, `
		SELECT default_group_id FROM users WHERE id=$1`, userID); err != nil {
		return fmt.Errorf("set subscribed default: load current group: %w", err)
	}
	var subscribed bool
	if err = tx.GetContext(ctx, &subscribed, `
		SELECT EXISTS (
			SELECT 1 FROM subscriptions
			WHERE user_id=$1 AND object_id=$2 AND object_type='group'
		)`, userID, groupID); err != nil {
		return fmt.Errorf("set subscribed default: inspect subscription: %w", err)
	}
	if !subscribed {
		return sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE users SET default_group_id=$2, updated_at=NOW() WHERE id=$1`, userID, groupID); err != nil {
		return fmt.Errorf("set subscribed default: update user: %w", err)
	}
	if previousDefault.Valid && previousDefault.String != groupID {
		if _, err = tx.ExecContext(ctx, `
			SELECT scheduler_request_outbox_cancellation(
				$1, 'lesson_reminder', $2, 'default_group_changed'
			)`, userID, previousDefault.String); err != nil {
			return fmt.Errorf("set subscribed default: cancel previous reminders: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("set subscribed default: commit: %w", err)
	}
	return nil
}

func (r *SubscriptionRepository) UnsubscribeAndSelectDefault(
	ctx context.Context,
	userID, groupID string,
) (string, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("unsubscribe and select default: begin: %w", err)
	}
	defer tx.Rollback()
	if err = database.LockGroupReferences(ctx, tx, false); err != nil {
		return "", err
	}
	var currentDefault sql.NullString
	if err = tx.GetContext(ctx, &currentDefault, `
		SELECT default_group_id FROM users WHERE id=$1 FOR NO KEY UPDATE`, userID); errors.Is(err, sql.ErrNoRows) {
		return "", sql.ErrNoRows
	} else if err != nil {
		return "", fmt.Errorf("unsubscribe and select default: lock user: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		DELETE FROM subscriptions
		WHERE user_id=$1 AND object_id=$2 AND object_type='group'`, userID, groupID)
	if err != nil {
		return "", fmt.Errorf("unsubscribe and select default: delete: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return "", sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, `
		SELECT scheduler_request_notification_cancellation($1, $2, 'subscription_removed')`,
		userID, groupID); err != nil {
		return "", fmt.Errorf("unsubscribe and select default: cancel notifications: %w", err)
	}
	newDefault := ""
	if currentDefault.Valid && currentDefault.String != groupID {
		newDefault = currentDefault.String
	} else {
		err = tx.GetContext(ctx, &newDefault, `
			SELECT s.object_id
			FROM subscriptions s
			JOIN groups g ON g.id=s.object_id AND g.is_active
			JOIN universities u ON u.id=g.university_id AND u.is_active
			WHERE s.user_id=$1 AND s.object_type='group'
			ORDER BY s.updated_at DESC, s.created_at DESC, s.id
			LIMIT 1 FOR SHARE OF g, u SKIP LOCKED`, userID)
		if errors.Is(err, sql.ErrNoRows) {
			newDefault = ""
		} else if err != nil {
			return "", fmt.Errorf("unsubscribe and select default: choose replacement: %w", err)
		}
		var value any
		if newDefault != "" {
			value = newDefault
		}
		if _, err = tx.ExecContext(ctx, `
			UPDATE users SET default_group_id=$2, updated_at=NOW() WHERE id=$1`, userID, value); err != nil {
			return "", fmt.Errorf("unsubscribe and select default: update user: %w", err)
		}
		if currentDefault.Valid {
			if _, err = tx.ExecContext(ctx, `
				SELECT scheduler_request_outbox_cancellation(
					$1, 'lesson_reminder', $2, 'default_group_changed'
				)`, userID, currentDefault.String); err != nil {
				return "", fmt.Errorf("unsubscribe and select default: cancel previous reminders: %w", err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("unsubscribe and select default: commit: %w", err)
	}
	return newDefault, nil
}

func lockUser(ctx context.Context, tx *sqlx.Tx, userID string) error {
	var id string
	if err := tx.GetContext(ctx, &id, `SELECT id FROM users WHERE id=$1 FOR NO KEY UPDATE`, userID); errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	} else if err != nil {
		return fmt.Errorf("lock user %s: %w", userID, err)
	}
	return nil
}

func lockActiveGroup(ctx context.Context, tx *sqlx.Tx, groupID string) error {
	var universityID string
	if err := tx.GetContext(ctx, &universityID, `SELECT university_id FROM groups WHERE id=$1`, groupID); err != nil {
		return fmt.Errorf("load group university: %w", err)
	}
	var id string
	if err := tx.GetContext(ctx, &id, `SELECT id FROM universities WHERE id=$1 AND is_active FOR SHARE`, universityID); err != nil {
		return fmt.Errorf("lock active university: %w", err)
	}
	if err := tx.GetContext(ctx, &id, `
		SELECT id FROM groups WHERE id=$1 AND university_id=$2 AND is_active FOR SHARE`, groupID, universityID); errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	} else if err != nil {
		return fmt.Errorf("lock active group %s: %w", groupID, err)
	}
	return nil
}

func (r *SubscriptionRepository) GetGroupSubscriptions(ctx context.Context, userID string) ([]domain.GroupSubscription, error) {
	var items []domain.GroupSubscription
	err := r.db.SelectContext(ctx, &items, `
		SELECT s.id, s.user_id, g.id AS group_id, g.name AS group_name,
			g.university_id, u.name AS university_name,
			COALESCE(users.default_group_id = g.id, FALSE) AS is_default,
			(g.is_active AND u.is_active) AS is_active,
			s.schedule_view_format, s.subgroup,
			s.created_at, s.updated_at
		FROM subscriptions s
		JOIN users ON users.id = s.user_id
		JOIN groups g ON g.id = s.object_id
		JOIN universities u ON u.id = g.university_id
		WHERE s.user_id = $1 AND s.object_type = 'group'
		ORDER BY is_default DESC, s.updated_at DESC, u.name, g.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("get group subscriptions for user %s: %w", userID, err)
	}
	return items, nil
}

func (r *SubscriptionRepository) SetGroupSubgroup(
	ctx context.Context,
	userID string,
	groupID string,
	subgroup int,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE subscriptions
		SET subgroup=$3, updated_at=NOW()
		WHERE user_id=$1 AND object_id=$2 AND object_type='group'`,
		userID, groupID, subgroup,
	)
	if err != nil {
		return fmt.Errorf("set subgroup user=%s group=%s: %w", userID, groupID, err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
		return fmt.Errorf("set subgroup user=%s group=%s: rows affected: %w", userID, groupID, rowsErr)
	} else if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *SubscriptionRepository) SetGroupScheduleView(
	ctx context.Context,
	userID string,
	groupID string,
	format domain.ScheduleViewFormat,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE subscriptions
		SET schedule_view_format=$3, updated_at=NOW()
		WHERE user_id=$1 AND object_id=$2 AND object_type='group'`,
		userID, groupID, format,
	)
	if err != nil {
		return fmt.Errorf("set schedule view user=%s group=%s: %w", userID, groupID, err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
		return fmt.Errorf("set schedule view user=%s group=%s: rows affected: %w", userID, groupID, rowsErr)
	} else if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
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
		`SELECT id, user_id, object_id, object_type, schedule_view_format, subgroup, created_at, updated_at FROM subscriptions WHERE id = $1`, id)
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
		`SELECT id, user_id, object_id, object_type, schedule_view_format, subgroup, created_at, updated_at
		 FROM subscriptions WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("get subscriptions for user %s: %w", userID, err)
	}
	return subs, nil
}

func (r *SubscriptionRepository) DeleteSubscriptionByObject(ctx context.Context, userID, objectID, objectType string) error {
	if objectType == "group" {
		_, err := r.UnsubscribeAndSelectDefault(ctx, userID, objectID)
		return err
	}
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
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("delete subscription user=%s obj=%s/%s: commit: %w", userID, objectType, objectID, err)
	}
	return nil
}

func (r *SubscriptionRepository) DeleteSubscription(ctx context.Context, id string) error {
	subscription, err := r.GetSubscriptionByID(ctx, id)
	if err != nil || subscription == nil {
		return err
	}
	if subscription.ObjectType == "group" {
		_, err = r.UnsubscribeAndSelectDefault(ctx, subscription.UserID, subscription.ObjectID)
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = $1`, id)
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
