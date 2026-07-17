package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type LessonRepository struct {
	db *sqlx.DB
}

func NewLessonRepository(db *sqlx.DB) *LessonRepository {
	return &LessonRepository{db: db}
}

func (r *LessonRepository) UpsertLesson(ctx context.Context, lesson domain.Lesson) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO lessons
			(id, university_id, semester_id, day_of_week, special_date, time_start, time_end,
			 week_type, subject, type, teacher, room, group_id, subgroup, valid_from, valid_to, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (id) DO UPDATE SET
			semester_id   = EXCLUDED.semester_id,
			day_of_week   = EXCLUDED.day_of_week,
			special_date  = EXCLUDED.special_date,
			time_start    = EXCLUDED.time_start,
			time_end      = EXCLUDED.time_end,
			week_type     = EXCLUDED.week_type,
			subject       = EXCLUDED.subject,
			type          = EXCLUDED.type,
			teacher       = EXCLUDED.teacher,
			room          = EXCLUDED.room,
			subgroup      = EXCLUDED.subgroup,
			valid_from    = EXCLUDED.valid_from,
			valid_to      = EXCLUDED.valid_to,
			updated_at    = EXCLUDED.updated_at`,
		lesson.ID, lesson.UniversityID, lesson.SemesterID,
		lessonDayOfWeekDB(lesson), lesson.SpecialDate,
		lesson.TimeStart, lesson.TimeEnd,
		lesson.WeekType, lesson.Subject, lesson.Type,
		lesson.Teacher, lesson.Room,
		lesson.GroupID, lesson.Subgroup, lesson.ValidFrom, lesson.ValidTo, lesson.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert lesson %s: %w", lesson.ID, err)
	}
	return nil
}

func (r *LessonRepository) CreateLesson(ctx context.Context, lesson domain.Lesson) (string, error) {
	if err := r.UpsertLesson(ctx, lesson); err != nil {
		return "", err
	}
	return lesson.ID, nil
}

func (r *LessonRepository) GetLessonByID(ctx context.Context, id string) (*domain.Lesson, error) {
	var lesson domain.Lesson
	err := r.db.GetContext(ctx, &lesson, lessonSelect+` WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get lesson %s: %w", id, err)
	}
	return &lesson, nil
}

func (r *LessonRepository) GetLessonsByGroupAndSemester(ctx context.Context, groupID, semesterID string) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	err := r.db.SelectContext(ctx, &lessons,
		lessonSelect+` WHERE group_id = $1 AND semester_id = $2 ORDER BY day_of_week, time_start`,
		groupID, semesterID)
	if err != nil {
		return nil, fmt.Errorf("get lessons group=%s semester=%s: %w", groupID, semesterID, err)
	}
	return lessons, nil
}

func (r *LessonRepository) GetLessonsByGroupAndSemesters(ctx context.Context, groupID string, semesterIDs []string) ([]domain.Lesson, error) {
	if len(semesterIDs) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(
		`SELECT `+lessonCols+` FROM lessons WHERE group_id = ? AND semester_id IN (?) ORDER BY day_of_week, time_start`,
		groupID, semesterIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("build IN query: %w", err)
	}
	query = r.db.Rebind(query)
	var lessons []domain.Lesson
	if err := r.db.SelectContext(ctx, &lessons, query, args...); err != nil {
		return nil, fmt.Errorf("get lessons group=%s semesters=%v: %w", groupID, semesterIDs, err)
	}
	return lessons, nil
}

func (r *LessonRepository) GetLessonsBySemesterID(ctx context.Context, semesterID string) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	err := r.db.SelectContext(ctx, &lessons,
		lessonSelect+` WHERE semester_id = $1 ORDER BY day_of_week, time_start`, semesterID)
	if err != nil {
		return nil, fmt.Errorf("get lessons by semester %s: %w", semesterID, err)
	}
	return lessons, nil
}

func (r *LessonRepository) GetLessonsByGroupID(ctx context.Context, groupID string) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	err := r.db.SelectContext(ctx, &lessons,
		lessonSelect+` WHERE group_id = $1 ORDER BY day_of_week, time_start`, groupID)
	if err != nil {
		return nil, fmt.Errorf("get lessons by group %s: %w", groupID, err)
	}
	return lessons, nil
}

func (r *LessonRepository) GetLessonsByTeacher(ctx context.Context, teacher string) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	err := r.db.SelectContext(ctx, &lessons,
		lessonSelect+` WHERE teacher = $1 ORDER BY day_of_week, time_start`, teacher)
	if err != nil {
		return nil, fmt.Errorf("get lessons by teacher %q: %w", teacher, err)
	}
	return lessons, nil
}

func (r *LessonRepository) GetLessonsByRoom(ctx context.Context, room string) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	err := r.db.SelectContext(ctx, &lessons,
		lessonSelect+` WHERE room = $1 ORDER BY day_of_week, time_start`, room)
	if err != nil {
		return nil, fmt.Errorf("get lessons by room %q: %w", room, err)
	}
	return lessons, nil
}

func (r *LessonRepository) UpdateLesson(ctx context.Context, lesson domain.Lesson) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE lessons SET
			university_id=$1, semester_id=$2, day_of_week=$3, special_date=$4,
			time_start=$5, time_end=$6, week_type=$7, subject=$8, type=$9,
			teacher=$10, room=$11, group_id=$12, subgroup=$13,
			valid_from=$14, valid_to=$15, updated_at=$16
		WHERE id=$17`,
		lesson.UniversityID, lesson.SemesterID, lessonDayOfWeekDB(lesson), lesson.SpecialDate,
		lesson.TimeStart, lesson.TimeEnd, lesson.WeekType, lesson.Subject, lesson.Type,
		lesson.Teacher, lesson.Room, lesson.GroupID, lesson.Subgroup,
		lesson.ValidFrom, lesson.ValidTo, time.Now(),
		lesson.ID,
	)
	if err != nil {
		return fmt.Errorf("update lesson %s: %w", lesson.ID, err)
	}
	return nil
}

func (r *LessonRepository) DeleteLesson(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM lessons WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete lesson %s: %w", id, err)
	}
	return nil
}

func (r *LessonRepository) DeleteLessonsByGroupAndSemester(ctx context.Context, groupID, semesterID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM lessons WHERE group_id = $1 AND semester_id = $2`,
		groupID, semesterID)
	if err != nil {
		return fmt.Errorf("delete lessons group=%s semester=%s: %w", groupID, semesterID, err)
	}
	return nil
}

func (r *LessonRepository) ReplaceLessonsForGroup(ctx context.Context, groupID string, lessons []domain.Lesson) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace lessons group=%s: begin: %w", groupID, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.ExecContext(ctx, `DELETE FROM lessons WHERE group_id = $1`, groupID); err != nil {
		return fmt.Errorf("replace lessons group=%s: delete old: %w", groupID, err)
	}

	const query = `
		INSERT INTO lessons
			(id, university_id, semester_id, day_of_week, special_date, time_start, time_end,
			 week_type, subject, type, teacher, room, group_id, subgroup, valid_from, valid_to, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`
	for _, lesson := range lessons {
		if _, err = tx.ExecContext(ctx, query,
			lesson.ID, lesson.UniversityID, lesson.SemesterID,
			lessonDayOfWeekDB(lesson), lesson.SpecialDate,
			lesson.TimeStart, lesson.TimeEnd,
			lesson.WeekType, lesson.Subject, lesson.Type,
			lesson.Teacher, lesson.Room, lesson.GroupID, lesson.Subgroup,
			lesson.ValidFrom, lesson.ValidTo, lesson.UpdatedAt,
		); err != nil {
			return fmt.Errorf("replace lessons group=%s: insert %s: %w", groupID, lesson.ID, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("replace lessons group=%s: commit: %w", groupID, err)
	}
	return nil
}

const lessonCols = `id, university_id, semester_id, COALESCE(day_of_week, 0) AS day_of_week, special_date,
	time_start, time_end, week_type, subject, type, teacher, room, group_id, subgroup,
	valid_from, valid_to, updated_at`

const lessonSelect = `SELECT ` + lessonCols + ` FROM lessons`

func lessonDayOfWeekDB(lesson domain.Lesson) any {
	if lesson.WeekType == domain.WeekTypeDate || lesson.SpecialDate != nil {
		return nil
	}
	return lesson.DayOfWeek
}

var _ = strings.Join
