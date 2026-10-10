package repository

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

func personalPublicationFingerprints(lessons []domain.Lesson) map[string]string {
	groups := map[string][]string{}
	for _, lesson := range lessons {
		lesson.ID, lesson.PersonalKey, lesson.SourceID, lesson.ExternalID, lesson.SourceFingerprint = "", "", "", "", ""
		lesson.FetchedAt = nil
		lesson.UpdatedAt = time.Time{}
		raw, _ := json.Marshal(lesson)
		groups[lesson.GroupID] = append(groups[lesson.GroupID], string(raw))
	}
	result := map[string]string{}
	for id, values := range groups {
		sort.Strings(values)
		result[id] = strings.Join(values, "\n")
	}
	return result
}

func markPersonalPublicationChanges(ctx context.Context, tx *sqlx.Tx, universityID string, before []domain.Lesson, beforeSemesters []domain.Semester) error {
	var after []domain.Lesson
	if err := tx.SelectContext(ctx, &after, lessonSelect+` WHERE university_id=$1`, universityID); err != nil {
		return err
	}
	old, current := personalPublicationFingerprints(before), personalPublicationFingerprints(after)
	changed := []string{}
	for groupID, value := range old {
		if value != current[groupID] {
			changed = append(changed, groupID)
		}
	}
	for groupID := range current {
		if _, exists := old[groupID]; !exists {
			changed = append(changed, groupID)
		}
	}
	var afterSemesters []domain.Semester
	if err := tx.SelectContext(ctx, &afterSemesters, `SELECT * FROM semesters WHERE university_id=$1`, universityID); err != nil {
		return err
	}
	changedSemesters := map[string]bool{}
	for _, previous := range beforeSemesters {
		for _, current := range afterSemesters {
			if previous.ID == current.ID && (!previous.StartDate.Equal(current.StartDate) || !previous.EndDate.Equal(current.EndDate)) {
				changedSemesters[previous.ID] = true
			}
		}
	}
	seen := map[string]bool{}
	for _, id := range changed {
		seen[id] = true
	}
	for _, lessons := range [][]domain.Lesson{before, after} {
		for _, lesson := range lessons {
			if changedSemesters[lesson.SemesterID] && !seen[lesson.GroupID] {
				changed = append(changed, lesson.GroupID)
				seen[lesson.GroupID] = true
			}
		}
	}
	if len(changed) == 0 {
		return nil
	}
	sort.Strings(changed)
	_, err := tx.ExecContext(ctx, `SELECT scheduler_personal_publication_changed($1,$2::text[])`, universityID, changed)
	return err
}

const personalPublicationRevision = `SELECT COALESCE(string_agg(id || ':' || COALESCE(current_snapshot_id,''),',' ORDER BY id),'') FROM data_sources WHERE university_id=$1 AND lifecycle_status='active'`

func (r *PersonalScheduleRepository) PublicationRevision(ctx context.Context, universityID string) (string, error) {
	var result string
	err := r.db.GetContext(ctx, &result, personalPublicationRevision, universityID)
	return result, err
}

func checkPersonalPublication(ctx context.Context, tx *sqlx.Tx, universityID, expected string) error {
	if err := lockUniversityPublication(ctx, tx, universityID); err != nil {
		return err
	}
	var current string
	if err := tx.GetContext(ctx, &current, personalPublicationRevision, universityID); err != nil {
		return err
	}
	if current != expected {
		return ErrPersonalConflict
	}
	return nil
}
