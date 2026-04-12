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

func (r *SemesterRepository) CreateSemester(ctx context.Context, id string, universityID string, name string, startDate time.Time, endDate time.Time) (string, error) {
	query := `INSERT INTO semesters (id, university_id, name, start_date, end_date) VALUES ($1, $2, $3, $4, $5)`
	_, err := r.db.ExecContext(ctx, query, id, universityID, name, startDate, endDate)
	if err != nil {
		return "", fmt.Errorf("failed to create semester: %w", err)
	}
	return id, nil
}

func (r *SemesterRepository) GetSemesterByID(ctx context.Context, id string) (*domain.Semester, error) {
	var semester domain.Semester
	query := `SELECT id, university_id, name, start_date, end_date FROM semesters WHERE id = $1`
	err := r.db.GetContext(ctx, &semester, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get semester by id: %w", err)
	}
	return &semester, nil
}

func (r *SemesterRepository) GetSemestersByUniversityID(ctx context.Context, universityID string) ([]domain.Semester, error) {
	var semesters []domain.Semester
	query := `SELECT id, university_id, name, start_date, end_date FROM semesters WHERE university_id = $1`
	err := r.db.SelectContext(ctx, &semesters, query, universityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get semesters by university id: %w", err)
	}
	return semesters, nil
}

func (r *SemesterRepository) GetSemesterByDate(ctx context.Context, universityID string, date time.Time) (*domain.Semester, error) {
	var semester domain.Semester
	query := `SELECT id, university_id, name, start_date, end_date FROM semesters WHERE university_id = $1 AND start_date <= $2 AND end_date >= $2`
	err := r.db.GetContext(ctx, &semester, query, universityID, date)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get semester by date: %w", err)
	}
	return &semester, nil
}

func (r *SemesterRepository) GetAllSemesters(ctx context.Context) ([]domain.Semester, error) {
	var semesters []domain.Semester
	query := `SELECT id, university_id, name, start_date, end_date FROM semesters`
	err := r.db.SelectContext(ctx, &semesters, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all semesters: %w", err)
	}
	return semesters, nil
}

func (r *SemesterRepository) UpdateSemester(ctx context.Context, id string, universityID string, name string, startDate time.Time, endDate time.Time) error {
	query := `UPDATE semesters SET university_id = $1, name = $2, start_date = $3, end_date = $4 WHERE id = $5`
	_, err := r.db.ExecContext(ctx, query, universityID, name, startDate, endDate, id)
	if err != nil {
		return fmt.Errorf("failed to update semester: %w", err)
	}
	return nil
}

func (r *SemesterRepository) DeleteSemester(ctx context.Context, id string) error {
	query := `DELETE FROM semesters WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete semester: %w", err)
	}
	return nil
}