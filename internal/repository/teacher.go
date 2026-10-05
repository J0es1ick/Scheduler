package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/searchtext"
	"github.com/google/uuid"
)

func (r *UserRepository) GetTeacher(ctx context.Context, id string) (*domain.Teacher, error) {
	var teacher domain.Teacher
	err := r.db.GetContext(ctx, &teacher, `SELECT id,university_id,name,name_key FROM teachers WHERE id=$1`, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &teacher, err
}

func (r *UserRepository) SetTeacher(ctx context.Context, userID, universityID, name string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active bool
	if err = tx.GetContext(ctx, &active, `SELECT is_active FROM universities WHERE id=$1`, universityID); err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("university unavailable")
	}
	var id string
	err = tx.GetContext(ctx, &id, `INSERT INTO teachers(id,university_id,name,name_key) VALUES($1,$2,$3,$4)
		ON CONFLICT(university_id,name_key) DO UPDATE SET name=EXCLUDED.name RETURNING id`, uuid.NewString(), universityID, name, searchtext.TokenKey(name))
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE users SET role='teacher',teacher_id=$2,updated_at=NOW() WHERE id=$1`, userID, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (r *UserRepository) SetRole(ctx context.Context, userID string, role domain.UserRole) error {
	result, err := r.db.ExecContext(ctx, `UPDATE users SET role=$2,updated_at=NOW() WHERE id=$1
		AND (($2='student' AND default_group_id IS NOT NULL) OR ($2='teacher' AND teacher_id IS NOT NULL))`, userID, role)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepository) SetTeacherView(ctx context.Context, userID string, format domain.ScheduleViewFormat) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET teacher_schedule_view_format=$2,updated_at=NOW() WHERE id=$1 AND role='teacher'`, userID, format)
	return err
}

func (r *UserRepository) SetDailySchedule(ctx context.Context, userID string, enabled bool, clock string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE users SET daily_enabled=$2,daily_time=$3::time,daily_setup='done',updated_at=NOW()
		WHERE id=$1 AND ((role='student' AND default_group_id IS NOT NULL) OR (role='teacher' AND teacher_id IS NOT NULL))`, userID, enabled, clock)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepository) SetDailySetup(ctx context.Context, userID, step string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET daily_setup=$2,updated_at=NOW() WHERE id=$1 AND daily_setup<>'done'`, userID, step)
	return err
}
