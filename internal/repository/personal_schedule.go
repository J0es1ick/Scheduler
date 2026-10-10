package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

var ErrPersonalConflict = errors.New("personal schedule changed")

type PersonalScheduleRepository struct{ db *sqlx.DB }

func NewPersonalScheduleRepository(db *sqlx.DB) *PersonalScheduleRepository {
	return &PersonalScheduleRepository{db: db}
}

func (r *PersonalScheduleRepository) Targets(ctx context.Context, userID string) ([]domain.PersonalTarget, error) {
	result := []domain.PersonalTarget{}
	err := r.db.SelectContext(ctx, &result, `SELECT g.id,u.role,g.name,g.university_id,un.name AS university_name,un.timezone,s.subgroup,COALESCE(g.id=u.default_group_id,FALSE) AS is_primary
 FROM users u JOIN subscriptions s ON s.user_id=u.id AND s.object_type='group'
 JOIN groups g ON g.id=s.object_id JOIN universities un ON un.id=g.university_id
 WHERE u.id=$1 AND u.role='student' AND (g.is_active OR EXISTS(SELECT 1 FROM personal_schedule_overrides o WHERE o.user_id=u.id AND o.role='student' AND o.target_id=g.id AND o.needs_review)) AND un.is_active
 UNION ALL SELECT t.id,u.role,t.name,t.university_id,un.name,un.timezone,0,TRUE
 FROM users u JOIN teachers t ON t.id=u.teacher_id JOIN universities un ON un.id=t.university_id
 WHERE u.id=$1 AND u.role='teacher' AND un.is_active ORDER BY is_primary DESC,name,id`, userID)
	return result, err
}

func (r *PersonalScheduleRepository) List(ctx context.Context, userID string) ([]domain.PersonalOverride, error) {
	result := []domain.PersonalOverride{}
	err := r.db.SelectContext(ctx, &result, `SELECT * FROM personal_schedule_overrides WHERE user_id=$1 ORDER BY CASE scope WHEN 'day' THEN 1 ELSE 0 END,valid_from,updated_at,id`, userID)
	return result, err
}

func (r *PersonalScheduleRepository) Save(ctx context.Context, item domain.PersonalOverride, expected int64, publication ...string) (*domain.PersonalOverride, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if len(publication) > 0 {
		if err = checkPersonalPublication(ctx, tx, item.UniversityID, publication[0]); err != nil {
			return nil, err
		}
	}
	if err = lockPersonalOwner(ctx, tx, item.UserID, item.Role, item.TargetID); err != nil {
		return nil, err
	}
	var pending bool
	if err = tx.GetContext(ctx, &pending, `SELECT EXISTS(SELECT 1 FROM personal_schedule_overrides WHERE user_id=$1 AND role=$2 AND target_id=$3 AND needs_review)`, item.UserID, item.Role, item.TargetID); err != nil {
		return nil, err
	}
	if pending {
		return nil, ErrPersonalConflict
	}
	if len(item.Basis) == 0 {
		item.Basis = []byte(`{}`)
	}
	if len(item.Occurrences) == 0 {
		item.Occurrences = []byte(`[]`)
	}
	if item.ID == "" {
		item.ID = uuid.NewString()
		err = tx.GetContext(ctx, &item, `INSERT INTO personal_schedule_overrides(id,user_id,role,target_id,lesson_id,university_id,semester_id,scope,valid_from,valid_to,patch,cancelled,basis,occurrences)
  VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13::jsonb,$14::jsonb) RETURNING *`, item.ID, item.UserID, item.Role, item.TargetID, item.LessonID, item.UniversityID, item.SemesterID, item.Scope, item.ValidFrom, item.ValidTo, item.Patch, item.Cancelled, item.Basis, item.Occurrences)
	} else {
		err = tx.GetContext(ctx, &item, `UPDATE personal_schedule_overrides SET patch=$4::jsonb,cancelled=$5,lesson_id=$8,valid_to=$11,basis=$12::jsonb,occurrences=$13::jsonb,version=version+1,updated_at=NOW()
  WHERE id=$1 AND user_id=$2 AND version=$3 AND role=$6 AND target_id=$7 AND scope=$9 AND valid_from=$10 RETURNING *`, item.ID, item.UserID, expected, item.Patch, item.Cancelled, item.Role, item.TargetID, item.LessonID, item.Scope, item.ValidFrom, item.ValidTo, item.Basis, item.Occurrences)
	}
	var pgErr *pgconn.PgError
	if errors.Is(err, sql.ErrNoRows) || (errors.As(err, &pgErr) && pgErr.Code == "23505") {
		return nil, ErrPersonalConflict
	}
	if err != nil {
		return nil, err
	}
	return &item, tx.Commit()
}

func lockPersonalOwner(ctx context.Context, tx *sqlx.Tx, userID string, role domain.UserRole, targetID string) error {
	var valid bool
	err := tx.GetContext(ctx, &valid, `SELECT COALESCE(role=$2 AND NOT bot_blocked AND CASE WHEN role='teacher' THEN teacher_id=$3 ELSE EXISTS(SELECT 1 FROM subscriptions WHERE user_id=users.id AND object_type='group' AND object_id=$3) END,FALSE) FROM users WHERE id=$1 FOR UPDATE`, userID, role, targetID)
	if err != nil {
		return err
	}
	if !valid {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PersonalScheduleRepository) Delete(ctx context.Context, userID, id string, version int64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner string
	if err = tx.GetContext(ctx, &owner, `SELECT id FROM users WHERE id=$1 AND NOT bot_blocked FOR UPDATE`, userID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM personal_schedule_overrides WHERE user_id=$1 AND id=$2 AND version=$3`, userID, id, version)
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
	return tx.Commit()
}

func (r *PersonalScheduleRepository) Semester(ctx context.Context, id string) (*domain.Semester, error) {
	var result domain.Semester
	err := r.db.GetContext(ctx, &result, `SELECT * FROM semesters WHERE id=$1`, id)
	return &result, err
}

func (r *PersonalScheduleRepository) ResolveReview(ctx context.Context, userID string, target domain.PersonalTarget, revision string, items []domain.PersonalOverride) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = checkPersonalPublication(ctx, tx, target.UniversityID, revision); err != nil {
		return err
	}
	if err = lockPersonalOwner(ctx, tx, userID, target.Role, target.ID); err != nil {
		return err
	}
	var count int
	if err = tx.GetContext(ctx, &count, `SELECT count(*) FROM personal_schedule_overrides WHERE user_id=$1 AND role=$2 AND target_id=$3 AND needs_review`, userID, target.Role, target.ID); err != nil {
		return err
	}
	if count != len(items) {
		return ErrPersonalConflict
	}
	for _, item := range items {
		var result sql.Result
		if len(item.Occurrences) <= 2 {
			result, err = tx.ExecContext(ctx, `DELETE FROM personal_schedule_overrides WHERE id=$1 AND user_id=$2 AND version=$3 AND needs_review`, item.ID, userID, item.Version)
		} else {
			result, err = tx.ExecContext(ctx, `UPDATE personal_schedule_overrides SET occurrences=$4::jsonb,needs_review=FALSE,version=version+1,updated_at=NOW() WHERE id=$1 AND user_id=$2 AND version=$3 AND needs_review`, item.ID, userID, item.Version, item.Occurrences)
		}
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
	}
	return tx.Commit()
}

func (r *NotificationRepository) PersonalNotification(ctx context.Context, userID, groupID, body string) (string, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM personal_schedule_overrides o JOIN users u ON u.id=o.user_id WHERE u.id=$1 AND o.role=u.role AND o.target_id=CASE WHEN u.role='teacher' THEN u.teacher_id ELSE $2 END AND (o.valid_to>=$3::date OR o.needs_review))`, userID, groupID, time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly))
	if err != nil {
		return "", err
	}
	if !exists {
		return body, nil
	}
	var pending bool
	err = r.db.GetContext(ctx, &pending, `SELECT EXISTS(SELECT 1 FROM personal_schedule_overrides o JOIN users u ON u.id=o.user_id WHERE u.id=$1 AND o.role=u.role AND o.target_id=CASE WHEN u.role='teacher' THEN u.teacher_id ELSE $2 END AND o.needs_review)`, userID, groupID)
	if err != nil {
		return "", err
	}
	if pending {
		return body + "\n\nВуз обновил расписание. Зайдите в «Личное расписание» через кнопку «Расписание», чтобы согласовать ваши правки: перенести их на занятия в тех же слотах или сбросить. До подтверждения эти правки не применяются.", nil
	}
	return "Вуз обновил расписание. Ваши личные правки сохранены. Актуальное расписание: /today или /week.", nil
}
