package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type GroupIdentityConflictError struct {
	ExternalGroupID string
	ExistingGroupID string
	ExistingName    string
	IncomingName    string
}

func (e *GroupIdentityConflictError) Error() string {
	return fmt.Sprintf(
		"source group id %s changed name from %q to %q",
		e.ExternalGroupID,
		e.ExistingName,
		e.IncomingName,
	)
}

func CanonicalizeSnapshotGroupIDs(
	payload domain.ScheduleSnapshot,
	existing []domain.Group,
) (domain.ScheduleSnapshot, int, error) {
	return CanonicalizeSnapshotGroupIDsWithMappings(payload, existing, nil)
}

func CanonicalizeSnapshotGroupIDsWithMappings(
	payload domain.ScheduleSnapshot,
	existing []domain.Group,
	mappings map[string]domain.GroupSourceIdentityMapping,
) (domain.ScheduleSnapshot, int, error) {
	payload, remapped, _, err := canonicalizeSnapshotGroupIDs(payload, existing, mappings)
	return payload, remapped, err
}

func canonicalizeSnapshotGroupIDs(
	payload domain.ScheduleSnapshot,
	existing []domain.Group,
	mappings map[string]domain.GroupSourceIdentityMapping,
) (domain.ScheduleSnapshot, int, map[string]domain.GroupSourceIdentityMapping, error) {
	byName := make(map[string]domain.Group, len(existing))
	byID := make(map[string]domain.Group, len(existing))
	for _, group := range existing {
		byName[strings.TrimSpace(group.Name)] = group
		byID[group.ID] = group
	}

	remapped := 0
	discovered := make(map[string]domain.GroupSourceIdentityMapping, len(payload.Groups))
	for index := range payload.Groups {
		group := &payload.Groups[index]
		group.Name = strings.TrimSpace(group.Name)
		originalID := strings.TrimSpace(group.ID)
		externalID := strings.TrimSpace(group.ExternalID)
		if externalID == "" {
			externalID = originalID
		}
		group.ExternalID = externalID
		canonicalID := originalID
		if mapping, ok := mappings[externalID]; ok {
			current, exists := byID[mapping.GroupID]
			if !exists || current.Name != group.Name || mapping.ExpectedName != group.Name {
				existingName := mapping.ExpectedName
				if exists {
					existingName = current.Name
				}
				return payload, remapped, discovered, &GroupIdentityConflictError{
					ExternalGroupID: externalID,
					ExistingGroupID: mapping.GroupID,
					ExistingName:    existingName,
					IncomingName:    group.Name,
				}
			}
			canonicalID = mapping.GroupID
		} else if current, ok := byName[group.Name]; ok {
			canonicalID = current.ID
		} else if current, ok := byID[externalID]; ok {
			if current.Name != group.Name {
				return payload, remapped, discovered, &GroupIdentityConflictError{
					ExternalGroupID: externalID,
					ExistingGroupID: current.ID,
					ExistingName:    current.Name,
					IncomingName:    group.Name,
				}
			}
			canonicalID = current.ID
		} else if current, ok := byID[originalID]; ok && current.Name != group.Name {
			return payload, remapped, discovered, &GroupIdentityConflictError{
				ExternalGroupID: externalID,
				ExistingGroupID: current.ID,
				ExistingName:    current.Name,
				IncomingName:    group.Name,
			}
		}
		if canonicalID != originalID {
			remapped++
			group.ID = canonicalID
		}
		discovered[externalID] = domain.GroupSourceIdentityMapping{
			ExternalGroupID: externalID,
			GroupID:         canonicalID,
			ExpectedName:    group.Name,
		}
		for lessonIndex := range group.Lessons {
			group.Lessons[lessonIndex].GroupID = canonicalID
		}
	}
	return payload, remapped, discovered, nil
}

func loadGroupSourceIdentityMappings(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	dataSourceID string,
) (map[string]domain.GroupSourceIdentityMapping, error) {
	var rows []domain.GroupSourceIdentityMapping
	if err := sqlx.SelectContext(ctx, queryer, &rows, `
		SELECT data_source_id, external_group_id, group_id, expected_name, created_at, updated_at
		FROM group_source_identity_mappings
		WHERE data_source_id=$1`, dataSourceID); err != nil {
		return nil, fmt.Errorf("load group source identity mappings: %w", err)
	}
	result := make(map[string]domain.GroupSourceIdentityMapping, len(rows))
	for _, row := range rows {
		result[row.ExternalGroupID] = row
	}
	return result, nil
}

func (r *GroupRepository) GroupSourceIdentityMappings(
	ctx context.Context,
	dataSourceID string,
) (map[string]domain.GroupSourceIdentityMapping, error) {
	return loadGroupSourceIdentityMappings(ctx, r.db, dataSourceID)
}

func (r *GroupRepository) RecordIdentityConflict(
	ctx context.Context,
	dataSourceID string,
	universityID string,
	conflict *GroupIdentityConflictError,
) error {
	if conflict == nil {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("record group identity conflict: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var sourceUniversityID string
	if err = tx.GetContext(ctx, &sourceUniversityID,
		`SELECT university_id FROM data_sources WHERE id=$1`, dataSourceID,
	); err != nil {
		return fmt.Errorf("record group identity conflict: load source: %w", err)
	}
	if sourceUniversityID != universityID {
		return fmt.Errorf(
			"record group identity conflict: source %s belongs to university %s, expected %s",
			dataSourceID, sourceUniversityID, universityID,
		)
	}
	if err = lockUniversityPublication(ctx, tx, sourceUniversityID); err != nil {
		return fmt.Errorf("record group identity conflict: %w", err)
	}

	var currentMapping struct {
		ExpectedName string `db:"expected_name"`
		GroupName    string `db:"group_name"`
	}
	err = tx.GetContext(ctx, &currentMapping, `
		SELECT mapping.expected_name, group_row.name AS group_name
		FROM group_source_identity_mappings mapping
		JOIN groups group_row ON group_row.id=mapping.group_id
		WHERE mapping.data_source_id=$1 AND mapping.external_group_id=$2
		FOR UPDATE OF mapping, group_row`, dataSourceID, conflict.ExternalGroupID)
	if err == nil &&
		strings.TrimSpace(currentMapping.ExpectedName) == strings.TrimSpace(conflict.IncomingName) &&
		strings.TrimSpace(currentMapping.GroupName) == strings.TrimSpace(conflict.IncomingName) {
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("record group identity conflict: commit resolved mapping: %w", err)
		}
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("record group identity conflict: inspect resolved mapping: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO group_identity_conflicts (
			id, data_source_id, university_id, external_group_id,
			existing_group_id, existing_name, incoming_name
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (data_source_id, external_group_id) DO UPDATE SET
			university_id=EXCLUDED.university_id,
			existing_group_id=EXCLUDED.existing_group_id,
			existing_name=EXCLUDED.existing_name,
			incoming_name=EXCLUDED.incoming_name,
			status='pending', resolution='', resolved_group_id=NULL,
			resolved_by='', resolved_at=NULL,
			last_seen_at=NOW(), occurrences=group_identity_conflicts.occurrences+1`,
		uuid.NewString(), dataSourceID, universityID, conflict.ExternalGroupID,
		conflict.ExistingGroupID, conflict.ExistingName, conflict.IncomingName,
	)
	if err != nil {
		return fmt.Errorf("record group identity conflict: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("record group identity conflict: commit: %w", err)
	}
	return nil
}

func (r *GroupRepository) ClearPendingIdentityConflicts(ctx context.Context, dataSourceID string) error {
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM group_identity_conflicts
		WHERE data_source_id=$1 AND status='pending'`, dataSourceID); err != nil {
		return fmt.Errorf("clear pending group identity conflicts: %w", err)
	}
	return nil
}
