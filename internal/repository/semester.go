package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type SemesterRepository struct {
	db *sqlx.DB
}

func NewSemesterRepository(db *sqlx.DB) *SemesterRepository {
	return &SemesterRepository{db: db}
}

func (r *SemesterRepository) CreateSemester(ctx context.Context, id, universityID, name string, startDate, endDate time.Time) (string, error) {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO semesters (id, university_id, name, start_date, end_date)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, universityID, name, startDate, endDate)
	if err != nil {
		return "", fmt.Errorf("create semester: %w", err)
	}
	return id, nil
}

func (r *SemesterRepository) UpsertSemester(ctx context.Context, id, universityID, name string, startDate, endDate time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO semesters (id, university_id, name, start_date, end_date)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET
			university_id = EXCLUDED.university_id,
			name = EXCLUDED.name,
			start_date = EXCLUDED.start_date,
			end_date = EXCLUDED.end_date`,
		id, universityID, name, startDate, endDate,
	)
	if err != nil {
		return fmt.Errorf("upsert semester %s: %w", id, err)
	}
	return nil
}

func (r *SemesterRepository) GetSemesterByID(ctx context.Context, id string) (*domain.Semester, error) {
	var sem domain.Semester
	err := r.db.GetContext(ctx, &sem,
		`SELECT id, university_id, name, start_date, end_date FROM semesters WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get semester %s: %w", id, err)
	}
	return &sem, nil
}

func (r *SemesterRepository) GetSemestersByIDs(ctx context.Context, ids []string) ([]domain.Semester, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(
		`SELECT id, university_id, name, start_date, end_date FROM semesters WHERE id IN (?)`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("build IN query for semesters: %w", err)
	}
	query = r.db.Rebind(query)
	var sems []domain.Semester
	if err := r.db.SelectContext(ctx, &sems, query, args...); err != nil {
		return nil, fmt.Errorf("get semesters by ids: %w", err)
	}
	return sems, nil
}

func (r *SemesterRepository) GetSemestersByUniversityID(ctx context.Context, universityID string) ([]domain.Semester, error) {
	var sems []domain.Semester
	err := r.db.SelectContext(ctx, &sems,
		`SELECT id, university_id, name, start_date, end_date
		 FROM semesters WHERE university_id = $1`, universityID)
	if err != nil {
		return nil, fmt.Errorf("get semesters for university %s: %w", universityID, err)
	}
	return sems, nil
}

func (r *SemesterRepository) GetSemesterByDate(ctx context.Context, universityID string, date time.Time) (*domain.Semester, error) {
	var sem domain.Semester
	err := r.db.GetContext(ctx, &sem,
		`SELECT id, university_id, name, start_date, end_date
		 FROM semesters
		 WHERE university_id = $1 AND start_date <= $2 AND end_date >= $2
		 LIMIT 1`,
		universityID, date)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get semester by date %v for university %s: %w", date, universityID, err)
	}
	return &sem, nil
}

func (r *SemesterRepository) GetAllSemesters(ctx context.Context) ([]domain.Semester, error) {
	var sems []domain.Semester
	err := r.db.SelectContext(ctx, &sems,
		`SELECT id, university_id, name, start_date, end_date FROM semesters`)
	if err != nil {
		return nil, fmt.Errorf("get all semesters: %w", err)
	}
	return sems, nil
}

func (r *SemesterRepository) UpdateSemester(ctx context.Context, id, universityID, name string, startDate, endDate time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE semesters SET university_id=$1, name=$2, start_date=$3, end_date=$4 WHERE id=$5`,
		universityID, name, startDate, endDate, id)
	if err != nil {
		return fmt.Errorf("update semester %s: %w", id, err)
	}
	return nil
}

func (r *SemesterRepository) DeleteSemester(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM semesters WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete semester %s: %w", id, err)
	}
	return nil
}
