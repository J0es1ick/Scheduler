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
			created_at, updated_at FROM users`)
	if err != nil {
		return nil, fmt.Errorf("get all users: %w", err)
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

func (r *UserRepository) DeleteUser(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user %s: %w", id, err)
	}
	return nil
}
