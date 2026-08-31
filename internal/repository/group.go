package repository

import (
	"context"
	"database/sql"
	"encoding/hex"
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

	query := `INSERT INTO groups (
		id, university_id, name, is_active, source_active, created_at, updated_at
	) VALUES ($1, $2, $3, $4, $4, $5, $6)`
	_, err := r.db.ExecContext(ctx, query, id, universityID, name, isActive, createdAt, updatedAt)
	if err != nil {
		return "", fmt.Errorf("failed to create group: %w", err)
	}
	return id, nil
}

func (r *GroupRepository) GetGroupByID(ctx context.Context, id string) (*domain.Group, error) {
	var group domain.Group
	query := `SELECT id, university_id, name, is_active, source_active,
		manually_disabled, created_at, updated_at FROM groups WHERE id = $1`
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
	query := `SELECT id, university_id, name, is_active, source_active,
		manually_disabled, created_at, updated_at
		FROM groups WHERE university_id = $1 AND is_active = TRUE ORDER BY name`
	err := r.db.SelectContext(ctx, &groups, query, universityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get groups by university id: %w", err)
	}
	return groups, nil
}

func (r *GroupRepository) GetAllGroupsByUniversityID(ctx context.Context, universityID string) ([]domain.Group, error) {
	var groups []domain.Group
	query := `SELECT id, university_id, name, is_active, source_active,
		manually_disabled, created_at, updated_at
		FROM groups WHERE university_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &groups, query, universityID); err != nil {
		return nil, fmt.Errorf("failed to get all groups by university id: %w", err)
	}
	return groups, nil
}

func (r *GroupRepository) GetGroupByName(ctx context.Context, universityID string, name string) (*domain.Group, error) {
	var group domain.Group
	query := `SELECT id, university_id, name, is_active, source_active,
		manually_disabled, created_at, updated_at
		FROM groups
		WHERE university_id = $1
		  AND LOWER(REGEXP_REPLACE(BTRIM(name), '[[:space:]]+', ' ', 'g')) = $2
		  AND is_active = TRUE`
	err := r.db.GetContext(ctx, &group, query, universityID, normalizedSearch(name))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get group by name: %w", err)
	}
	return &group, nil
}

func (r *GroupRepository) GetActiveGroupByToken(ctx context.Context, token string) (*domain.Group, error) {
	if decoded, err := hex.DecodeString(token); err != nil || len(decoded) != 8 {
		return nil, nil
	}
	var groups []domain.Group
	if err := r.db.SelectContext(ctx, &groups, `
		SELECT g.id, g.university_id, g.name, g.is_active, g.source_active,
			g.manually_disabled, g.created_at, g.updated_at
		FROM groups g
		JOIN universities u ON u.id=g.university_id
		WHERE LEFT(ENCODE(SHA256(CONVERT_TO(g.id, 'UTF8')), 'hex'), 16)=$1
		  AND g.is_active=TRUE AND u.is_active=TRUE
		LIMIT 2`, token); err != nil {
		return nil, fmt.Errorf("get active group by token: %w", err)
	}
	if len(groups) == 0 {
		return nil, nil
	}
	if len(groups) > 1 {
		return nil, fmt.Errorf("ambiguous group token")
	}
	return &groups[0], nil
}

func (r *GroupRepository) FindActiveByName(
	ctx context.Context,
	universityID string,
	name string,
) ([]domain.Group, error) {
	var groups []domain.Group
	name = normalizedSearch(name)
	if name == "" {
		return []domain.Group{}, nil
	}
	where := `STRPOS(LOWER(REGEXP_REPLACE(BTRIM(name), '[[:space:]]+', ' ', 'g')), $1)>0 AND is_active=TRUE
		AND EXISTS (SELECT 1 FROM universities u WHERE u.id=groups.university_id AND u.is_active)`
	args := []any{name}
	if universityID != "" {
		where += ` AND university_id=$2`
		args = append(args, universityID)
	}
	if err := r.db.SelectContext(ctx, &groups, `
		SELECT id, university_id, name, is_active, source_active,
			manually_disabled, created_at, updated_at
		FROM groups
		WHERE `+where+`
		ORDER BY (LOWER(BTRIM(name))=LOWER(BTRIM($1))) DESC,
			LENGTH(name), university_id, name
		LIMIT 10`,
		args...,
	); err != nil {
		return nil, fmt.Errorf("find active group by name %q: %w", name, err)
	}
	if groups == nil {
		groups = []domain.Group{}
	}
	if len(groups) == 0 && universityID != "" {
		var candidates []domain.Group
		if err := r.db.SelectContext(ctx, &candidates, `
			SELECT g.id, g.university_id, g.name, g.is_active, g.source_active,
				g.manually_disabled, g.created_at, g.updated_at
			FROM groups g JOIN universities u ON u.id=g.university_id
			WHERE g.university_id=$1 AND g.is_active AND u.is_active
				AND ABS(CHAR_LENGTH(REGEXP_REPLACE(BTRIM(g.name), '[[:space:]]+', ' ', 'g'))-CHAR_LENGTH($2))<=1
			ORDER BY g.name, g.id LIMIT 500`, universityID, name); err != nil {
			return nil, fmt.Errorf("suggest active groups: %w", err)
		}
		for _, candidate := range candidates {
			if closeSearchMatch(candidate.Name, name) {
				candidate.Suggested = true
				groups = append(groups, candidate)
				if len(groups) == 10 {
					break
				}
			}
		}
	}
	return groups, nil
}

func (r *GroupRepository) GetAllGroups(ctx context.Context) ([]domain.Group, error) {
	var groups []domain.Group
	query := `SELECT id, university_id, name, is_active, source_active,
		manually_disabled, created_at, updated_at FROM groups`
	err := r.db.SelectContext(ctx, &groups, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all groups: %w", err)
	}
	return groups, nil
}

func (r *GroupRepository) UpdateGroup(ctx context.Context, id string, name string, isActive bool) error {
	updatedAt := time.Now()
	query := `UPDATE groups SET name=$1, source_active=$2,
		is_active=$2 AND NOT manually_disabled, updated_at=$3 WHERE id=$4`
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
			`UPDATE groups SET source_active=FALSE, is_active=FALSE, updated_at=NOW()
			 WHERE university_id=$1`,
			universityID,
		)
		return err
	}
	query, args, err := sqlx.In(
		`UPDATE groups SET source_active=FALSE, is_active=FALSE, updated_at=NOW()
		 WHERE university_id=? AND id NOT IN (?) AND (source_active OR is_active)`,
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
