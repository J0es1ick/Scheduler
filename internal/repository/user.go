package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateUser(ctx context.Context, id, username string, isAdmin bool) (string, error) {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, username, is_admin, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, username, isAdmin, now, now)
	if err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

func (r *UserRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user,
		`SELECT id, COALESCE(username, '') AS username, is_admin,
			COALESCE(default_group_id, '') AS default_group_id, notifications_enabled,
			reminder_enabled, reminder_minutes, quiet_hours_enabled,
			to_char(quiet_hours_start, 'HH24:MI') AS quiet_hours_start,
			to_char(quiet_hours_end, 'HH24:MI') AS quiet_hours_end,
			created_at, updated_at
		 FROM users WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user %s: %w", id, err)
	}
	return &user, nil
}

func (r *UserRepository) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user,
		`SELECT id, COALESCE(username, '') AS username, is_admin,
			COALESCE(default_group_id, '') AS default_group_id, notifications_enabled,
			reminder_enabled, reminder_minutes, quiet_hours_enabled,
			to_char(quiet_hours_start, 'HH24:MI') AS quiet_hours_start,
			to_char(quiet_hours_end, 'HH24:MI') AS quiet_hours_end,
			created_at, updated_at
		 FROM users WHERE username = $1`, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by username %q: %w", username, err)
	}
	return &user, nil
}

func (r *UserRepository) GetAllUsers(ctx context.Context) ([]domain.User, error) {
	var users []domain.User
	err := r.db.SelectContext(ctx, &users,
		`SELECT id, COALESCE(username, '') AS username, is_admin,
			COALESCE(default_group_id, '') AS default_group_id, notifications_enabled,
			reminder_enabled, reminder_minutes, quiet_hours_enabled,
			to_char(quiet_hours_start, 'HH24:MI') AS quiet_hours_start,
			to_char(quiet_hours_end, 'HH24:MI') AS quiet_hours_end,
			created_at, updated_at FROM users`)
	if err != nil {
		return nil, fmt.Errorf("get all users: %w", err)
	}
	return users, nil
}

func (r *UserRepository) GetUsersPage(ctx context.Context, afterID string, limit int) ([]domain.User, error) {
	if limit <= 0 {
		return []domain.User{}, nil
	}
	var users []domain.User
	if err := r.db.SelectContext(ctx, &users, `
		SELECT id, COALESCE(username, '') AS username, is_admin
		FROM users
		WHERE ($1 = '' OR id > $1)
		ORDER BY id
		LIMIT $2`, afterID, limit); err != nil {
		return nil, fmt.Errorf("get users page after %q: %w", afterID, err)
	}
	if users == nil {
		users = []domain.User{}
	}
	return users, nil
}

func (r *UserRepository) UpdateUser(ctx context.Context, id, username string, isAdmin bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET username = $1, is_admin = $2, updated_at = $3 WHERE id = $4`,
		username, isAdmin, time.Now(), id)
	if err != nil {
		return fmt.Errorf("update user %s: %w", id, err)
	}
	return nil
}

func (r *UserRepository) SetDefaultGroup(ctx context.Context, userID, groupID string) error {
	var value any
	if groupID != "" {
		value = groupID
	}
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET default_group_id = $1, updated_at = NOW() WHERE id = $2`,
		value, userID,
	)
	if err != nil {
		return fmt.Errorf("set default group for user %s: %w", userID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepository) SetNotificationsEnabled(ctx context.Context, userID string, enabled bool) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set notifications for user %s: begin: %w", userID, err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx,
		`UPDATE users SET notifications_enabled = $1, updated_at = NOW() WHERE id = $2`,
		enabled, userID,
	)
	if err != nil {
		return fmt.Errorf("set notifications for user %s: %w", userID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if !enabled {
		if _, err = tx.ExecContext(ctx, `
			UPDATE notification_deliveries
			SET status='cancelled', updated_at=NOW()
			WHERE user_id=$1 AND status='pending'`, userID); err != nil {
			return fmt.Errorf("cancel pending notifications for user %s: %w", userID, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("set notifications for user %s: commit: %w", userID, err)
	}
	return nil
}

func (r *UserRepository) SetLessonReminder(
	ctx context.Context,
	userID string,
	enabled bool,
	minutes int,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set lesson reminder for user %s: begin: %w", userID, err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET reminder_enabled=$1, reminder_minutes=$2, updated_at=NOW()
		WHERE id=$3`,
		enabled, minutes, userID,
	)
	if err != nil {
		return fmt.Errorf("set lesson reminder for user %s: %w", userID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if !enabled {
		if _, err = tx.ExecContext(ctx, `
			UPDATE bot_outbox
			SET status='cancelled', updated_at=NOW()
			WHERE user_id=$1 AND kind='lesson_reminder' AND status='pending'`,
			userID,
		); err != nil {
			return fmt.Errorf("cancel lesson reminders for user %s: %w", userID, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("set lesson reminder for user %s: commit: %w", userID, err)
	}
	return nil
}

func (r *UserRepository) SetQuietHours(
	ctx context.Context,
	userID string,
	enabled bool,
	start string,
	end string,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET quiet_hours_enabled=$1, quiet_hours_start=$2::time,
			quiet_hours_end=$3::time, updated_at=NOW()
		WHERE id=$4`, enabled, start, end, userID)
	if err != nil {
		return fmt.Errorf("set quiet hours for user %s: %w", userID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepository) DeleteUser(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user %s: %w", id, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepository) ExportUserData(ctx context.Context, id string) (*domain.UserDataExport, error) {
	user, err := r.GetUserByID(ctx, id)
	if err != nil || user == nil {
		return nil, err
	}
	result := &domain.UserDataExport{
		ExportedAt:      time.Now().UTC(),
		User:            *user,
		Subscriptions:   []domain.Subscription{},
		SupportRequests: []domain.SupportRequest{},
	}
	if err = r.db.SelectContext(ctx, &result.Subscriptions, `
		SELECT id, user_id, object_id, object_type, created_at, updated_at
		FROM subscriptions
		WHERE user_id=$1
		ORDER BY created_at`, id); err != nil {
		return nil, fmt.Errorf("export subscriptions for user %s: %w", id, err)
	}
	if err = r.db.SelectContext(ctx, &result.SupportRequests, `
		SELECT id, user_id, request_type, details, status, review_note,
			reviewed_by, reviewed_at, created_at, updated_at
		FROM support_requests
		WHERE user_id=$1
		ORDER BY created_at`, id); err != nil {
		return nil, fmt.Errorf("export support requests for user %s: %w", id, err)
	}
	return result, nil
}
