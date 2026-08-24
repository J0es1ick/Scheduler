package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

var (
	ErrGroupActive         = errors.New("group is active")
	ErrGroupNotPublished   = errors.New("group is absent from the current source snapshot")
	ErrGroupStillPublished = errors.New("group is present in the current source snapshot")
)

type groupLifecycleState struct {
	ID               string `db:"id"`
	Name             string `db:"name"`
	UniversityID     string `db:"university_id"`
	IsActive         bool   `db:"is_active"`
	SourceActive     bool   `db:"source_active"`
	ManuallyDisabled bool   `db:"manually_disabled"`
}

func (s *Store) SetGroupActive(ctx context.Context, groupID string, active bool) (*GroupView, error) {
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("admin update group lifecycle: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	state, err := lockGroupLifecycle(ctx, tx, groupID)
	if err != nil {
		return nil, err
	}
	if active && !state.SourceActive {
		return nil, ErrGroupNotPublished
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE groups
		SET manually_disabled=$2,
			is_active=CASE WHEN $2 THEN FALSE ELSE source_active END,
			updated_at=NOW()
		WHERE id=$1`, groupID, !active); err != nil {
		return nil, fmt.Errorf("admin update group lifecycle: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("admin update group lifecycle: commit: %w", err)
	}
	return s.Group(ctx, groupID)
}

func (s *Store) DeleteGroup(ctx context.Context, groupID string) (*GroupDeletionResult, error) {
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("admin delete group: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	state, err := lockGroupLifecycle(ctx, tx, groupID)
	if err != nil {
		return nil, err
	}
	if state.IsActive {
		return nil, ErrGroupActive
	}
	if state.SourceActive {
		return nil, ErrGroupStillPublished
	}

	result := &GroupDeletionResult{
		ID:           state.ID,
		Name:         state.Name,
		UniversityID: state.UniversityID,
	}
	if err = tx.GetContext(ctx, result, `
		SELECT group_row.id, group_row.name, group_row.university_id,
			(SELECT COUNT(*)::int FROM subscriptions subscription
				WHERE subscription.object_type='group' AND subscription.object_id=group_row.id) AS subscription_count,
			(SELECT COUNT(*)::int FROM users app_user
				WHERE app_user.default_group_id=group_row.id) AS default_group_count,
			(SELECT COUNT(*)::int FROM chat_schedule_profiles chat
				WHERE chat.default_group_id=group_row.id) AS chat_count,
			(SELECT COUNT(*)::int FROM effective_lessons lesson
				WHERE lesson.group_id=group_row.id) AS lesson_count,
			(SELECT COUNT(*)::int FROM lesson_overrides override_row
				WHERE override_row.group_id=group_row.id) AS override_count
		FROM groups group_row WHERE group_row.id=$1`, groupID); err != nil {
		return nil, fmt.Errorf("admin inspect group deletion: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		DELETE FROM subscriptions WHERE object_type='group' AND object_id=$1`, groupID); err != nil {
		return nil, fmt.Errorf("admin delete group subscriptions: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM groups WHERE id=$1`, groupID); err != nil {
		return nil, fmt.Errorf("admin delete group: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("admin delete group: commit: %w", err)
	}
	return result, nil
}

func (s *Store) Group(ctx context.Context, groupID string) (*GroupView, error) {
	var group GroupView
	if err := s.db.GetContext(ctx, &group, `
		SELECT g.id, g.name, g.university_id, university.name AS university_name,
			g.is_active, g.source_active, g.manually_disabled,
			(SELECT COUNT(*)::int FROM effective_lessons lesson WHERE lesson.group_id=g.id) AS lesson_count,
			(SELECT COUNT(*)::int FROM subscriptions subscription
				WHERE subscription.object_type='group' AND subscription.object_id=g.id) AS subscription_count,
			(SELECT COUNT(*)::int FROM users app_user
				WHERE app_user.default_group_id=g.id) AS default_group_count,
			(SELECT COUNT(*)::int FROM chat_schedule_profiles chat
				WHERE chat.default_group_id=g.id) AS chat_count,
			(SELECT COUNT(*)::int FROM lesson_overrides override_row
				WHERE override_row.group_id=g.id) AS override_count,
			g.created_at, g.updated_at
		FROM groups g
		JOIN universities university ON university.id=g.university_id
		WHERE g.id=$1`, groupID); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("admin load group: %w", err)
	}
	return &group, nil
}

func lockGroupLifecycle(
	ctx context.Context,
	tx *sqlx.Tx,
	groupID string,
) (*groupLifecycleState, error) {
	var universityID string
	if err := tx.GetContext(ctx, &universityID,
		`SELECT university_id FROM groups WHERE id=$1`, groupID,
	); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("admin load group lifecycle: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		SELECT pg_advisory_xact_lock(
			hashtext('scheduler-snapshot-publication'), hashtext($1)
		)`, universityID); err != nil {
		return nil, fmt.Errorf("admin lock group lifecycle: %w", err)
	}
	var state groupLifecycleState
	if err := tx.GetContext(ctx, &state, `
		SELECT id, name, university_id, is_active, source_active, manually_disabled
		FROM groups WHERE id=$1 FOR UPDATE`, groupID); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("admin lock group: %w", err)
	}
	return &state, nil
}
