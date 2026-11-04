package repository

import (
	"context"
	"encoding/json"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

func scheduleTransition(ctx context.Context, tx *sqlx.Tx, universityID string, before, after []domain.Lesson, prior ...[]domain.Semester) (json.RawMessage, error) {
	semesters := []domain.Semester{}
	if err := tx.SelectContext(ctx, &semesters, `SELECT * FROM semesters WHERE university_id=$1`, universityID); err != nil {
		return nil, err
	}
	baseline := semesters
	if len(prior) > 0 && prior[0] != nil {
		baseline = prior[0]
	}
	var groups []struct {
		ID   string
		Name string
	}
	if err := tx.SelectContext(ctx, &groups, `SELECT id,name FROM groups WHERE university_id=$1`, universityID); err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, g := range groups {
		names[g.ID] = g.Name
	}
	for i := range before {
		before[i].GroupName = names[before[i].GroupID]
	}
	for i := range after {
		after[i].GroupName = names[after[i].GroupID]
	}
	return json.Marshal(domain.ScheduleTransition{UniversityID: universityID, Before: before, After: after, Semesters: semesters, BeforeSemesters: baseline})
}

func EnqueueScheduleTransition(ctx context.Context, tx *sqlx.Tx, eventID, groupID, source, summary string, before, after []domain.Lesson, prior ...[]domain.Semester) error {
	var universityID string
	if err := tx.GetContext(ctx, &universityID, `SELECT university_id FROM groups WHERE id=$1`, groupID); err != nil {
		return err
	}
	filter := func(items []domain.Lesson) []domain.Lesson {
		result := []domain.Lesson{}
		for _, item := range items {
			if item.GroupID == groupID {
				result = append(result, item)
			}
		}
		return result
	}
	details, err := scheduleTransition(ctx, tx, universityID, filter(before), filter(after), prior...)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `SELECT enqueue_schedule_change($1,$2,$3,$4,$5::jsonb)`, eventID, groupID, source, summary, details)
	return err
}
