package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type GroupRepository struct {
	db *sqlx.DB
}

func NewGroupRepository(db *sqlx.DB) *GroupRepository {
	return &GroupRepository{db: db}
}

func (r *GroupRepository) CreateGroup(ctx context.Context, id string, universityID string, name string, isActive bool) (string, error) {
	createdAt := time.Now()
	updatedAt := time.Now()

	query := `INSERT INTO groups (id, university_id, name, is_active, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.db.ExecContext(ctx, query, id, universityID, name, isActive, createdAt, updatedAt)
	if err != nil {
		return "", fmt.Errorf("failed to create group: %w", err)
	}
	return id, nil
}

func (r *GroupRepository) GetGroupByID(ctx context.Context, id string) (*domain.Group, error) {
	var group domain.Group
	query := `SELECT id, university_id, name, is_active, created_at, updated_at FROM groups WHERE id = $1`
	err := r.db.GetContext(ctx, &group, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get group by id: %w", err)
	}
	return &group, nil
}

func (r *GroupRepository) GetGroupsByUniversityID(ctx context.Context, universityID string) ([]domain.Group, error) {
	var groups []domain.Group
	query := `SELECT id, university_id, name, is_active, created_at, updated_at
		FROM groups WHERE university_id = $1 AND is_active = TRUE ORDER BY name`
	err := r.db.SelectContext(ctx, &groups, query, universityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get groups by university id: %w", err)
	}
	return groups, nil
}

func (r *GroupRepository) GetGroupByName(ctx context.Context, universityID string, name string) (*domain.Group, error) {
	var group domain.Group
	query := `SELECT id, university_id, name, is_active, created_at, updated_at
		FROM groups WHERE university_id = $1 AND name = $2 AND is_active = TRUE`
	err := r.db.GetContext(ctx, &group, query, universityID, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get group by name: %w", err)
	}
	return &group, nil
}

func (r *GroupRepository) GetAllGroups(ctx context.Context) ([]domain.Group, error) {
	var groups []domain.Group
	query := `SELECT id, university_id, name, is_active, created_at, updated_at FROM groups`
	err := r.db.SelectContext(ctx, &groups, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all groups: %w", err)
	}
	return groups, nil
}

func (r *GroupRepository) UpdateGroup(ctx context.Context, id string, name string, isActive bool) error {
	updatedAt := time.Now()
	query := `UPDATE groups SET name = $1, is_active = $2, updated_at = $3 WHERE id = $4`
	_, err := r.db.ExecContext(ctx, query, name, isActive, updatedAt, id)
	if err != nil {
		return fmt.Errorf("failed to update group: %w", err)
	}
	return nil
}

func (r *GroupRepository) DeleteGroup(ctx context.Context, id string) error {
	query := `DELETE FROM groups WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete group: %w", err)
	}
	return nil
}

func (r *GroupRepository) DeactivateGroupsExcept(ctx context.Context, universityID string, activeIDs []string) error {
	if len(activeIDs) == 0 {
		_, err := r.db.ExecContext(ctx,
			`UPDATE groups SET is_active = FALSE, updated_at = NOW() WHERE university_id = $1`,
			universityID,
		)
		return err
	}
	query, args, err := sqlx.In(
		`UPDATE groups SET is_active = FALSE, updated_at = NOW()
		 WHERE university_id = ? AND id NOT IN (?) AND is_active = TRUE`,
		universityID, activeIDs,
	)
	if err != nil {
		return fmt.Errorf("build deactivate groups query: %w", err)
	}
	query = r.db.Rebind(query)
	if _, err = r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("deactivate stale groups for %s: %w", universityID, err)
	}
	return nil
}
