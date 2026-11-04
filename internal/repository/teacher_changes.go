package repository

import (
	"context"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/searchtext"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func EffectiveLessonsForUniversity(ctx context.Context, tx *sqlx.Tx, universityID string) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	err := tx.SelectContext(ctx, &lessons, lessonSelect+` WHERE university_id=$1`, universityID)
	return lessons, err
}

func EnqueueTeacherChanges(ctx context.Context, tx *sqlx.Tx, universityID string, changes map[string]string, lessons ...[]domain.Lesson) error {
	return enqueueTeacherTransitions(ctx, tx, universityID, changes, nil, lessons...)
}

func enqueueTeacherTransitions(ctx context.Context, tx *sqlx.Tx, universityID string, changes map[string]string, prior []domain.Semester, lessons ...[]domain.Lesson) error {
	registered := map[string]bool{}
	indexed := []map[string][]domain.Lesson{}
	if len(lessons) == 2 {
		var keys []string
		if err := tx.SelectContext(ctx, &keys, `SELECT name_key FROM teachers WHERE university_id=$1`, universityID); err != nil {
			return err
		}
		for _, key := range keys {
			if _, exists := changes[key]; exists {
				registered[key] = true
			}
		}
		if len(registered) == 0 {
			return nil
		}
		for _, snapshot := range lessons {
			byTeacher := map[string][]domain.Lesson{}
			for _, lesson := range snapshot {
				for _, name := range searchtext.TeacherNames([]string{lesson.Teacher}) {
					key := searchtext.TokenKey(name)
					if registered[key] {
						byTeacher[key] = append(byTeacher[key], lesson)
					}
				}
			}
			indexed = append(indexed, byTeacher)
		}
	}
	for key, summary := range changes {
		if len(lessons) == 2 {
			if !registered[key] {
				continue
			}
			details, err := scheduleTransition(ctx, tx, universityID, indexed[0][key], indexed[1][key], prior)
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `SELECT enqueue_teacher_change($1,$2,$3,$4,$5::jsonb)`, uuid.NewString(), universityID, key, summary, details); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `SELECT enqueue_teacher_change($1,$2,$3,$4)`, uuid.NewString(), universityID, key, summary); err != nil {
			return err
		}
	}
	return nil
}

func (p *SnapshotPublication) EnqueueTeacherChanges(ctx context.Context, universityID string, changes map[string]string, lessons ...[]domain.Lesson) error {
	return enqueueTeacherTransitions(ctx, p.tx, universityID, changes, p.beforeSemesters, lessons...)
}
