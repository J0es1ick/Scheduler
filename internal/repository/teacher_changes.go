package repository

import (
	"context"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func EffectiveLessonsForUniversity(ctx context.Context, tx *sqlx.Tx, universityID string) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	err := tx.SelectContext(ctx, &lessons, lessonSelect+` WHERE university_id=$1`, universityID)
	return lessons, err
}

func EnqueueTeacherChanges(ctx context.Context, tx *sqlx.Tx, universityID string, changes map[string]string) error {
	for key, summary := range changes {
		if _, err := tx.ExecContext(ctx, `SELECT enqueue_teacher_change($1,$2,$3,$4)`, uuid.NewString(), universityID, key, summary); err != nil {
			return err
		}
	}
	return nil
}

func (p *SnapshotPublication) EnqueueTeacherChanges(ctx context.Context, universityID string, changes map[string]string) error {
	return EnqueueTeacherChanges(ctx, p.tx, universityID, changes)
}
