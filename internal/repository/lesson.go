package repository

import (
	"context"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type LessonRepository struct {
	db *sqlx.DB
}

func NewLessonRepository(db *sqlx.DB) *LessonRepository {
	return &LessonRepository{db: db}
}

func (r *LessonRepository) CreateLesson(ctx context.Context, lesson domain.Lesson) (string, error) {
	query := `INSERT INTO lessons (id, university_id, semester_id, day_of_week, special_date, time_start, time_end, week_type, subject, type, teacher, room, group_id, subgroup, updated_at) 
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`
	_, err := r.db.ExecContext(ctx, query, lesson.ID, lesson.UniversityID, lesson.SemesterID, lesson.DayOfWeek, lesson.SpecialDate, lesson.TimeStart, lesson.TimeEnd, lesson.WeekType, lesson.Subject, lesson.Type, lesson.Teacher, lesson.Room, lesson.GroupID, lesson.Subgroup, lesson.UpdatedAt)
	if err != nil {
		return "", fmt.Errorf("failed to create lesson: %w", err)
	}
	return lesson.ID, nil
}

func (r *LessonRepository) GetLessonByID(ctx context.Context, id string) (*domain.Lesson, error) {
	var lesson domain.Lesson
	query := `SELECT id, university_id, semester_id, day_of_week, special_date, time_start, time_end, week_type, subject, type, teacher, room, group_id, subgroup, updated_at
			  FROM lessons WHERE id = $1`
	err := r.db.GetContext(ctx, &lesson, query, id)	
	if err != nil {
		return nil, fmt.Errorf("failed to get lesson by id: %w", err)
	}
	return &lesson, nil
}

func (r *LessonRepository) GetLessonsBySemesterID(ctx context.Context, semesterID string) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	query := `SELECT id, university_id, semester_id, day_of_week, special_date, time_start, time_end, week_type, subject, type, teacher, room, group_id, subgroup, updated_at
			  FROM lessons WHERE semester_id = $1 ORDER BY day_of_week, time_start`
	err := r.db.SelectContext(ctx, &lessons, query, semesterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get lessons by semester id: %w", err)
	}
	return lessons, nil
}

func (r *LessonRepository) GetLessonsByGroupID(ctx context.Context, groupID string) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	query := `SELECT id, university_id, semester_id, day_of_week, special_date, time_start, time_end, week_type, subject, type, teacher, room, group_id, subgroup, updated_at
			  FROM lessons WHERE group_id = $1 ORDER BY day_of_week, time_start`
	err := r.db.SelectContext(ctx, &lessons, query, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get lessons by group id: %w", err)
	}
	return lessons, nil
}

func (r *LessonRepository) GetLessonsByTeacher(ctx context.Context, teacher string) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	query := `SELECT id, university_id, semester_id, day_of_week, special_date, time_start, time_end, week_type, subject, type, teacher, room, group_id, subgroup, updated_at
			  FROM lessons WHERE teacher = $1 ORDER BY day_of_week, time_start`
	err := r.db.SelectContext(ctx, &lessons, query, teacher)
	if err != nil {
		return nil, fmt.Errorf("failed to get lessons by teacher: %w", err)
	}
	return lessons, nil
}

func (r *LessonRepository) GetLessonsByRoom(ctx context.Context, room string) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	query := `SELECT id, university_id, semester_id, day_of_week, special_date, time_start, time_end, week_type, subject, type, teacher, room, group_id, subgroup, updated_at	
			  FROM lessons WHERE room = $1 ORDER BY day_of_week, time_start`
	err := r.db.SelectContext(ctx, &lessons, query, room)
	if err != nil {
		return nil, fmt.Errorf("failed to get lessons by room: %w", err)
	}
	return lessons, nil
}

func (r *LessonRepository) GetLessonsBySubject(ctx context.Context, subject string) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	query := `SELECT id, university_id, semester_id, day_of_week, special_date, time_start, time_end, week_type, subject, type, teacher, room, group_id, subgroup, updated_at
			  FROM lessons WHERE subject = $1 ORDER BY day_of_week, time_start`
	err := r.db.SelectContext(ctx, &lessons, query, subject)
	if err != nil {
		return nil, fmt.Errorf("failed to get lessons by subject: %w", err)
	}
	return lessons, nil
}

func (r *LessonRepository) GetLessonsByType(ctx context.Context, lessonType domain.LessonType) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	query := `SELECT id, university_id, semester_id, day_of_week, special_date, time_start, time_end, week_type, subject, type, teacher, room, group_id, subgroup, updated_at
			  FROM lessons WHERE type = $1 ORDER BY day_of_week, time_start`
	err := r.db.SelectContext(ctx, &lessons, query, lessonType)
	if err != nil {
		return nil, fmt.Errorf("failed to get lessons by type: %w", err)
	}
	return lessons, nil
}

func (r *LessonRepository) UpdateLesson(ctx context.Context, lesson domain.Lesson) error {
	query := `UPDATE lessons SET university_id = $1, semester_id = $2, day_of_week = $3, special_date = $4, time_start = $5, time_end = $6, week_type = $7, subject = $8, type = $9, teacher = $10, room = $11, group_id = $12, subgroup = $13, updated_at = $14
			  WHERE id = $15`
	_, err := r.db.ExecContext(ctx, query, lesson.UniversityID, lesson.SemesterID, lesson.DayOfWeek, lesson.SpecialDate, lesson.TimeStart, lesson.TimeEnd, lesson.WeekType, lesson.Subject, lesson.Type, lesson.Teacher, lesson.Room, lesson.GroupID, lesson.Subgroup, lesson.UpdatedAt, lesson.ID)
	if err != nil {
		return fmt.Errorf("failed to update lesson: %w", err)
	}
	return nil
}

func (r *LessonRepository) DeleteLesson(ctx context.Context, id string) error {
	query := `DELETE FROM lessons WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete lesson: %w", err)
	}
	return nil
}

