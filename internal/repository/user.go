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

func (r *UserRepository) CreateUser(ctx context.Context, id string, universityID string, username string, isAdmin bool, group string) (string, error) {
	createdAt := time.Now()
	updatedAt := time.Now()

	query := `INSERT INTO users (id, university_id, username, is_admin, group, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.ExecContext(ctx, query, id, universityID, username, isAdmin, group, createdAt, updatedAt)
	if err != nil {
		return "", fmt.Errorf("failed to create user: %w", err)
	}
	return id, nil
}

func (r *UserRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	var user domain.User
	query := `SELECT id, university_id, username, is_admin, group, created_at, updated_at FROM users WHERE id = $1`
	err := r.db.GetContext(ctx, &user, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) GetUsersByUniversityID(ctx context.Context, universityID string) ([]domain.User, error) {
	var users []domain.User
	query := `SELECT id, university_id, username, is_admin, group, created_at, updated_at FROM users WHERE university_id = $1`
	err := r.db.SelectContext(ctx, &users, query, universityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get users by university id: %w", err)
	}
	return users, nil
}

func (r *UserRepository) GetUserByUsername(ctx context.Context, universityID string, username string) (*domain.User, error) {
	var user domain.User
	query := `SELECT id, university_id, username, is_admin, group, created_at, updated_at FROM users WHERE university_id = $1 AND username = $2`
	err := r.db.GetContext(ctx, &user, query, universityID, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) UpdateUser(ctx context.Context, id string, username string, isAdmin bool, group string) error {
	updatedAt := time.Now()
	query := `UPDATE users SET username = $1, is_admin = $2, group = $3, updated_at = $4 WHERE id = $5`
	_, err := r.db.ExecContext(ctx, query, username, isAdmin, group, updatedAt, id)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

func (r *UserRepository) DeleteUser(ctx context.Context, id string) error {	
	query := `DELETE FROM users WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}