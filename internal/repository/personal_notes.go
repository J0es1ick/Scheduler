package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/google/uuid"
)

func (r *PersonalScheduleRepository) Notes(ctx context.Context, userID string) ([]domain.PersonalNote, error) {
	result := []domain.PersonalNote{}
	err := r.db.SelectContext(ctx, &result, `SELECT * FROM personal_notes WHERE user_id=$1 ORDER BY completed,due_date NULLS LAST,lesson_date,id`, userID)
	return result, err
}

func (r *PersonalScheduleRepository) SaveNote(ctx context.Context, item domain.PersonalNote, expected int64, rebind bool) (*domain.PersonalNote, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var owner string
	if err = tx.GetContext(ctx, &owner, `SELECT id FROM users WHERE id=$1 AND NOT bot_blocked FOR UPDATE`, item.UserID); err != nil {
		return nil, err
	}
	if item.ID == "" || rebind {
		if err = lockPersonalOwner(ctx, tx, item.UserID, item.Role, item.TargetID); err != nil {
			return nil, err
		}
	}
	if item.ID == "" {
		var count int
		if err = tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM personal_notes WHERE user_id=$1`, item.UserID); err != nil {
			return nil, err
		}
		if count >= 2000 {
			return nil, ErrPersonalConflict
		}
		item.ID = uuid.NewString()
		err = tx.GetContext(ctx, &item, `INSERT INTO personal_notes(id,user_id,role,target_id,lesson_id,lesson_date,basis,body,due_date,completed)
 VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10) RETURNING *`, item.ID, item.UserID, item.Role, item.TargetID, item.LessonID, item.LessonDate, item.Basis, item.Body, item.DueDate, item.Completed)
	} else {
		err = tx.GetContext(ctx, &item, `UPDATE personal_notes SET body=$4,due_date=$5,completed=$6,version=version+1,updated_at=clock_timestamp(),
 role=CASE WHEN $7 THEN $8 ELSE role END,target_id=CASE WHEN $7 THEN $9 ELSE target_id END,
 lesson_id=CASE WHEN $7 THEN $10 ELSE lesson_id END,lesson_date=CASE WHEN $7 THEN $11 ELSE lesson_date END,basis=CASE WHEN $7 THEN $12::jsonb ELSE basis END
 WHERE id=$1 AND user_id=$2 AND version=$3 RETURNING *`, item.ID, item.UserID, expected, item.Body, item.DueDate, item.Completed, rebind, item.Role, item.TargetID, item.LessonID, item.LessonDate, item.Basis)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPersonalConflict
	}
	if err != nil {
		return nil, err
	}
	return &item, tx.Commit()
}

func (r *PersonalScheduleRepository) DeleteNote(ctx context.Context, userID, id string, version int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM personal_notes WHERE id=$1 AND user_id=$2 AND version=$3`, id, userID, version)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrPersonalConflict
	}
	return nil
}
