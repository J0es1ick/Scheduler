package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type UniversityRepository struct {
	db *sqlx.DB
}

func NewUniversityRepository(db *sqlx.DB) *UniversityRepository {
	return &UniversityRepository{db: db}
}

func (r *UniversityRepository) CreateUniversity(ctx context.Context, id string, name string, fullName string, scheduleURL string, isActive bool) (string, error) {
	createdAt := time.Now()
	updatedAt := time.Now()
	query := `INSERT INTO universities (id, name, full_name, schedule_url, is_active, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.ExecContext(ctx, query, id, name, fullName, scheduleURL, isActive, createdAt, updatedAt)	
	if err != nil {
		return "", fmt.Errorf("failed to create university: %w", err)
	}
	return id, nil
}

func (r *UniversityRepository) GetUniversityByID(ctx context.Context, id string) (*domain.University, error) {
	var university domain.University
	query := `SELECT id, name, full_name, schedule_url, is_active, created_at, updated_at FROM universities WHERE id = $1`
	err := r.db.GetContext(ctx, &university, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get university by id: %w", err)
	}	
	return &university, nil
}

func (r *UniversityRepository) GetUniversityByName(ctx context.Context, name string) (*domain.University, error) {
	var university domain.University
	query := `SELECT id, name, full_name, schedule_url, is_active, created_at, updated_at FROM universities WHERE name = $1`
	err := r.db.GetContext(ctx, &university, query, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}	
		return nil, fmt.Errorf("failed to get university by name: %w", err)
	}
	return &university, nil
}

func (r *UniversityRepository) GetAllUniversities(ctx context.Context) ([]domain.University, error) {
	var universities []domain.University
	query := `SELECT id, name, full_name, schedule_url, is_active, created_at, updated_at FROM universities`
	err := r.db.SelectContext(ctx, &universities, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all universities: %w", err)
	}	
	return universities, nil
}

func (r *UniversityRepository) UpdateUniversity(ctx context.Context, id string, name string, fullName string, scheduleURL string, isActive bool) error {
	updatedAt := time.Now()
	query := `UPDATE universities SET name = $1, full_name = $2, schedule_url = $3, is_active = $4, updated_at = $5 WHERE id = $6`
	_, err := r.db.ExecContext(ctx, query, name, fullName, scheduleURL, isActive, updatedAt, id)	
	if err != nil {
		return fmt.Errorf("failed to update university: %w", err)
	}
	return nil
}

func (r *UniversityRepository) DeleteUniversity(ctx context.Context, id string) error {
	query := `DELETE FROM universities WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete university: %w", err)
	}
	return nil
}