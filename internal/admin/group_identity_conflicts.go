package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrIdentityConflictChanged = errors.New("group identity conflict changed")
	ErrIdentityNameExists      = errors.New("group identity name already exists")
)

func (s *Store) ResolveGroupIdentityConflict(
	ctx context.Context,
	sourceID string,
	conflictID string,
	resolution string,
	actorID string,
) (*domain.GroupIdentityConflict, error) {
	if resolution != "rename" && resolution != "new_group" {
		return nil, fmt.Errorf("invalid group identity resolution %q", resolution)
	}
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin group identity resolution: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var conflict domain.GroupIdentityConflict
	if err = tx.GetContext(ctx, &conflict, `
		SELECT id, data_source_id, university_id, external_group_id,
			existing_group_id, existing_name, incoming_name, status,
			resolution, resolved_group_id, resolved_by,
			first_seen_at, last_seen_at, resolved_at, occurrences
		FROM group_identity_conflicts
		WHERE id=$1 AND data_source_id=$2
		FOR UPDATE`, conflictID, sourceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load group identity conflict: %w", err)
	}
	if conflict.Status != "pending" {
		return nil, ErrIdentityConflictChanged
	}
	conflict.ExistingName = strings.TrimSpace(conflict.ExistingName)
	conflict.IncomingName = strings.TrimSpace(conflict.IncomingName)
	if conflict.IncomingName == "" {
		return nil, fmt.Errorf("incoming group name is empty")
	}

	var current domain.Group
	if err = tx.GetContext(ctx, &current, `
		SELECT id, university_id, name, is_active, source_active,
			manually_disabled, created_at, updated_at
		FROM groups WHERE id=$1 FOR UPDATE`, conflict.ExistingGroupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrIdentityConflictChanged
		}
		return nil, fmt.Errorf("load existing conflict group: %w", err)
	}
	if current.UniversityID != conflict.UniversityID || current.Name != conflict.ExistingName {
		return nil, ErrIdentityConflictChanged
	}

	resolvedGroupID := current.ID
	switch resolution {
	case "rename":
		var duplicateID string
		err = tx.GetContext(ctx, &duplicateID, `
			SELECT id FROM groups
			WHERE university_id=$1 AND name=$2 AND id<>$3
			LIMIT 1`, conflict.UniversityID, conflict.IncomingName, current.ID)
		if err == nil {
			return nil, ErrIdentityNameExists
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("check renamed group uniqueness: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `
			UPDATE groups SET name=$2, updated_at=NOW() WHERE id=$1`,
			current.ID, conflict.IncomingName); err != nil {
			return nil, fmt.Errorf("rename group identity: %w", err)
		}
	case "new_group":
		err = tx.GetContext(ctx, &resolvedGroupID, `
			SELECT id FROM groups WHERE university_id=$1 AND name=$2 LIMIT 1`,
			conflict.UniversityID, conflict.IncomingName)
		if errors.Is(err, sql.ErrNoRows) {
			resolvedGroupID = conflict.UniversityID + ":group:resolved:" + uuid.NewString()
			if _, err = tx.ExecContext(ctx, `
				INSERT INTO groups (
					id, university_id, name, is_active, source_active,
					manually_disabled, created_at, updated_at
				)
				VALUES ($1,$2,$3,FALSE,FALSE,FALSE,NOW(),NOW())`,
				resolvedGroupID, conflict.UniversityID, conflict.IncomingName); err != nil {
				return nil, fmt.Errorf("create resolved group identity: %w", err)
			}
		} else if err != nil {
			return nil, fmt.Errorf("find resolved group identity: %w", err)
		}
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO group_source_identity_mappings (
			data_source_id, external_group_id, group_id, expected_name
		) VALUES ($1,$2,$3,$4)
		ON CONFLICT (data_source_id, external_group_id) DO UPDATE SET
			group_id=EXCLUDED.group_id,
			expected_name=EXCLUDED.expected_name,
			updated_at=NOW()`,
		conflict.DataSourceID, conflict.ExternalGroupID, resolvedGroupID, conflict.IncomingName); err != nil {
		return nil, fmt.Errorf("save group source identity mapping: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE group_identity_conflicts SET
			status='resolved', resolution=$2, resolved_group_id=$3,
			resolved_by=$4, resolved_at=NOW()
		WHERE id=$1`, conflict.ID, resolution, resolvedGroupID, actorID); err != nil {
		return nil, fmt.Errorf("complete group identity conflict: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE data_sources SET
			last_error='', consecutive_failures=0, next_retry_at=NULL, updated_at=NOW()
		WHERE id=$1`, sourceID); err != nil {
		return nil, fmt.Errorf("reset source after identity resolution: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit group identity resolution: %w", err)
	}

	conflict.Status = "resolved"
	conflict.Resolution = resolution
	conflict.ResolvedGroupID = &resolvedGroupID
	conflict.ResolvedBy = actorID
	return &conflict, nil
}
