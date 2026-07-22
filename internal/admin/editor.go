package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const editorLessonColumns = `
	id, university_id, semester_id, COALESCE(day_of_week, 0) AS day_of_week,
	special_date, time_start, time_end, week_type::text AS week_type,
	subject, type::text AS type, teacher, room, group_id, subgroup,
	valid_from, valid_to, updated_at, origin, base_lesson_id, version`

func (s *Store) EditorSchedule(ctx context.Context, groupID string) (*EditorSchedule, error) {
	var result EditorSchedule
	if err := s.db.GetContext(ctx, &result.Group, `
		SELECT g.id, g.name, g.university_id, u.name AS university_name, g.updated_at
		FROM groups g
		JOIN universities u ON u.id = g.university_id
		WHERE g.id = $1`, groupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("editor get group: %w", err)
	}

	if err := s.db.SelectContext(ctx, &result.Semesters, `
		SELECT id, name, start_date, end_date
		FROM semesters
		WHERE university_id = $1
		ORDER BY start_date DESC`, result.Group.UniversityID); err != nil {
		return nil, fmt.Errorf("editor list semesters: %w", err)
	}

	if err := s.db.SelectContext(ctx, &result.Lessons, `
		SELECT `+editorLessonColumns+`, FALSE AS is_deleted
		FROM effective_lessons
		WHERE group_id = $1
		ORDER BY COALESCE(special_date, valid_from), day_of_week, time_start, subject`, groupID); err != nil {
		return nil, fmt.Errorf("editor list lessons: %w", err)
	}

	if err := s.db.SelectContext(ctx, &result.DeletedLessons, `
		SELECT
			id, university_id, semester_id, COALESCE(day_of_week, 0) AS day_of_week,
			special_date, time_start, time_end, week_type::text AS week_type,
			subject, type::text AS type, teacher, room, group_id, subgroup,
			valid_from, valid_to, updated_at, 'manual'::TEXT AS origin,
			base_lesson_id, version, is_deleted
		FROM lesson_overrides
		WHERE group_id = $1 AND is_deleted
		ORDER BY updated_at DESC`, groupID); err != nil {
		return nil, fmt.Errorf("editor list deleted lessons: %w", err)
	}
	if result.Semesters == nil {
		result.Semesters = []SemesterOption{}
	}
	if result.Lessons == nil {
		result.Lessons = []EditorLesson{}
	}
	if result.DeletedLessons == nil {
		result.DeletedLessons = []EditorLesson{}
	}

	return &result, nil
}

func (s *Store) CreateManualLesson(ctx context.Context, actorID string, lesson LessonMutation) (string, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("editor create lesson: begin: %w", err)
	}
	defer tx.Rollback()

	universityID, err := editorUniversity(tx, ctx, lesson.GroupID, lesson.SemesterID)
	if err != nil {
		return "", err
	}
	id := "manual:" + uuid.NewString()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO lesson_overrides (
			id, university_id, semester_id, day_of_week, special_date,
			time_start, time_end, week_type, subject, type, teacher, room,
			group_id, subgroup, valid_from, valid_to, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17
		)`,
		id, universityID, lesson.SemesterID, editorDayOfWeek(lesson), lesson.SpecialDate,
		lesson.TimeStart, lesson.TimeEnd, lesson.WeekType, lesson.Subject, lesson.Type,
		lesson.Teacher, lesson.Room, lesson.GroupID, lesson.Subgroup,
		lesson.ValidFrom, lesson.ValidTo, actorID,
	); err != nil {
		return "", fmt.Errorf("editor create lesson: insert: %w", err)
	}
	if err = enqueueManualScheduleChange(
		tx,
		ctx,
		lesson.GroupID,
		fmt.Sprintf("Добавлено занятие «%s».", lesson.Subject),
	); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("editor create lesson: commit: %w", err)
	}
	return id, nil
}

func (s *Store) UpdateEditorLesson(
	ctx context.Context,
	actorID string,
	lessonID string,
	expectedUpdatedAt time.Time,
	lesson LessonMutation,
) (string, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("editor update lesson: begin: %w", err)
	}
	defer tx.Rollback()

	current, err := effectiveLessonForUpdate(tx, ctx, lessonID)
	if err != nil {
		return "", err
	}
	if !sameInstant(current.UpdatedAt, expectedUpdatedAt) {
		return "", ErrConflict
	}
	lesson.GroupID = current.GroupID
	universityID, err := editorUniversity(tx, ctx, lesson.GroupID, lesson.SemesterID)
	if err != nil {
		return "", err
	}

	resultID := lessonID
	if current.Origin == "manual" {
		result, updateErr := tx.ExecContext(ctx, `
			UPDATE lesson_overrides SET
				university_id=$1, semester_id=$2, day_of_week=$3, special_date=$4,
				time_start=$5, time_end=$6, week_type=$7, subject=$8, type=$9,
				teacher=$10, room=$11, subgroup=$12, valid_from=$13, valid_to=$14,
				updated_at=NOW(), version=version+1, is_deleted=FALSE
			WHERE id=$15 AND updated_at=$16`,
			universityID, lesson.SemesterID, editorDayOfWeek(lesson), lesson.SpecialDate,
			lesson.TimeStart, lesson.TimeEnd, lesson.WeekType, lesson.Subject, lesson.Type,
			lesson.Teacher, lesson.Room, lesson.Subgroup, lesson.ValidFrom, lesson.ValidTo,
			lessonID, expectedUpdatedAt,
		)
		if updateErr != nil {
			return "", fmt.Errorf("editor update override: %w", updateErr)
		}
		if rows, _ := result.RowsAffected(); rows == 0 {
			return "", ErrConflict
		}
	} else {
		resultID = "manual:" + uuid.NewString()
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO lesson_overrides (
				id, base_lesson_id, university_id, semester_id, day_of_week, special_date,
				time_start, time_end, week_type, subject, type, teacher, room,
				group_id, subgroup, valid_from, valid_to, created_by
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
				$14, $15, $16, $17, $18
			)`,
			resultID, current.ID, universityID, lesson.SemesterID,
			editorDayOfWeek(lesson), lesson.SpecialDate, lesson.TimeStart, lesson.TimeEnd,
			lesson.WeekType, lesson.Subject, lesson.Type, lesson.Teacher, lesson.Room,
			lesson.GroupID, lesson.Subgroup, lesson.ValidFrom, lesson.ValidTo, actorID,
		); err != nil {
			return "", editorWriteError("editor create override", err)
		}
	}
	if err = enqueueManualScheduleChange(
		tx,
		ctx,
		lesson.GroupID,
		fmt.Sprintf("Изменено занятие «%s».", lesson.Subject),
	); err != nil {
		return "", err
	}

	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("editor update lesson: commit: %w", err)
	}
	return resultID, nil
}

func (s *Store) DeleteEditorLesson(
	ctx context.Context,
	actorID string,
	lessonID string,
	expectedUpdatedAt time.Time,
) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("editor delete lesson: begin: %w", err)
	}
	defer tx.Rollback()

	current, err := effectiveLessonForUpdate(tx, ctx, lessonID)
	if err != nil {
		return err
	}
	if !sameInstant(current.UpdatedAt, expectedUpdatedAt) {
		return ErrConflict
	}

	if current.Origin == "manual" {
		var result sql.Result
		if current.BaseLessonID == nil {
			result, err = tx.ExecContext(ctx,
				`DELETE FROM lesson_overrides WHERE id=$1 AND updated_at=$2`,
				lessonID, expectedUpdatedAt,
			)
		} else {
			result, err = tx.ExecContext(ctx, `
				UPDATE lesson_overrides
				SET is_deleted=TRUE, updated_at=NOW(), version=version+1
				WHERE id=$1 AND updated_at=$2`, lessonID, expectedUpdatedAt)
		}
		if err != nil {
			return fmt.Errorf("editor delete manual lesson: %w", err)
		}
		if rows, _ := result.RowsAffected(); rows == 0 {
			return ErrConflict
		}
	} else {
		id := "manual:" + uuid.NewString()
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO lesson_overrides (
				id, base_lesson_id, university_id, semester_id, day_of_week, special_date,
				time_start, time_end, week_type, subject, type, teacher, room,
				group_id, subgroup, valid_from, valid_to, is_deleted, created_by
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
				$14, $15, $16, $17, TRUE, $18
			)`,
			id, current.ID, current.UniversityID, current.SemesterID,
			editorDayOfWeekFromView(current), current.SpecialDate,
			current.TimeStart, current.TimeEnd, current.WeekType, current.Subject,
			current.Type, current.Teacher, current.Room, current.GroupID,
			current.Subgroup, current.ValidFrom, current.ValidTo, actorID,
		); err != nil {
			return editorWriteError("editor create deletion override", err)
		}
	}
	if err = enqueueManualScheduleChange(
		tx,
		ctx,
		current.GroupID,
		fmt.Sprintf("Удалено занятие «%s».", current.Subject),
	); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("editor delete lesson: commit: %w", err)
	}
	return nil
}

func (s *Store) RestoreEditorLesson(ctx context.Context, lessonID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("editor restore lesson: begin: %w", err)
	}
	defer tx.Rollback()

	var lesson struct {
		GroupID string `db:"group_id"`
		Subject string `db:"subject"`
	}
	err = tx.GetContext(ctx, &lesson, `
		SELECT group_id, subject
		FROM lesson_overrides
		WHERE id=$1 AND base_lesson_id IS NOT NULL
		FOR UPDATE`, lessonID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("editor restore lesson: read override: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		DELETE FROM lesson_overrides
		WHERE id=$1 AND base_lesson_id IS NOT NULL`, lessonID)
	if err != nil {
		return fmt.Errorf("editor restore lesson: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	if err = enqueueManualScheduleChange(
		tx,
		ctx,
		lesson.GroupID,
		fmt.Sprintf("Восстановлено занятие «%s» из расписания источника.", lesson.Subject),
	); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("editor restore lesson: commit: %w", err)
	}
	return nil
}

func enqueueManualScheduleChange(
	tx *sqlx.Tx,
	ctx context.Context,
	groupID, summary string,
) error {
	if _, err := tx.ExecContext(
		ctx,
		`SELECT enqueue_schedule_change($1, $2, 'manual', $3)`,
		uuid.NewString(),
		groupID,
		summary,
	); err != nil {
		return fmt.Errorf("editor enqueue schedule notification: %w", err)
	}
	return nil
}

func effectiveLessonForUpdate(
	tx *sqlx.Tx,
	ctx context.Context,
	lessonID string,
) (*EditorLesson, error) {
	var lesson EditorLesson
	err := tx.GetContext(ctx, &lesson, `
		SELECT `+editorLessonColumns+`, FALSE AS is_deleted
		FROM effective_lessons
		WHERE id=$1`, lessonID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("editor get lesson: %w", err)
	}
	return &lesson, nil
}

func editorUniversity(tx *sqlx.Tx, ctx context.Context, groupID, semesterID string) (string, error) {
	var universityID string
	err := tx.GetContext(ctx, &universityID, `
		SELECT g.university_id
		FROM groups g
		JOIN semesters s ON s.university_id = g.university_id
		WHERE g.id=$1 AND s.id=$2`, groupID, semesterID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("editor validate group and semester: %w", err)
	}
	return universityID, nil
}

func editorDayOfWeek(lesson LessonMutation) any {
	if lesson.WeekType == "date" {
		return nil
	}
	return lesson.DayOfWeek
}

func editorDayOfWeekFromView(lesson *EditorLesson) any {
	if lesson.WeekType == "date" {
		return nil
	}
	return lesson.DayOfWeek
}

func sameInstant(left, right time.Time) bool {
	return left.UTC().Truncate(time.Microsecond).Equal(right.UTC().Truncate(time.Microsecond))
}

func editorWriteError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if sqlState := sqlStateOf(err); sqlState == "23505" {
		return ErrConflict
	}
	return fmt.Errorf("%s: %w", operation, err)
}

type sqlStateError interface {
	SQLState() string
}

func sqlStateOf(err error) string {
	var stateErr sqlStateError
	if errors.As(err, &stateErr) {
		return stateErr.SQLState()
	}
	return ""
}
