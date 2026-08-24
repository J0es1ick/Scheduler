package repository

import (
	"context"
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
	byName := make(map[string]domain.Group, len(existing))
	byID := make(map[string]domain.Group, len(existing))
	for _, group := range existing {
		byName[strings.TrimSpace(group.Name)] = group
		byID[group.ID] = group
	}

	remapped := 0
	for index := range payload.Groups {
		group := &payload.Groups[index]
		group.Name = strings.TrimSpace(group.Name)
		canonicalID := group.ID
		if mapping, ok := mappings[group.ID]; ok {
			current, exists := byID[mapping.GroupID]
			if !exists || current.Name != group.Name || mapping.ExpectedName != group.Name {
				existingName := mapping.ExpectedName
				if exists {
					existingName = current.Name
				}
				return payload, remapped, &GroupIdentityConflictError{
					ExternalGroupID: group.ID,
					ExistingGroupID: mapping.GroupID,
					ExistingName:    existingName,
					IncomingName:    group.Name,
				}
			}
			canonicalID = mapping.GroupID
		} else if current, ok := byName[group.Name]; ok {
			canonicalID = current.ID
		} else if current, ok := byID[group.ID]; ok && current.Name != group.Name {
			return payload, remapped, &GroupIdentityConflictError{
				ExternalGroupID: group.ID,
				ExistingGroupID: current.ID,
				ExistingName:    current.Name,
				IncomingName:    group.Name,
			}
		}
		if canonicalID != group.ID {
			remapped++
			group.ID = canonicalID
		}
		for lessonIndex := range group.Lessons {
			group.Lessons[lessonIndex].GroupID = canonicalID
		}
	}
	return payload, remapped, nil
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
	_, err := r.db.ExecContext(ctx, `
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
