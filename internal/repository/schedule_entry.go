package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type ScheduleEntryRepository struct {
	db *sqlx.DB
}

func NewScheduleEntryRepository(db *sqlx.DB) *ScheduleEntryRepository {
	return &ScheduleEntryRepository{db: db}
}

func (r *ScheduleEntryRepository) CreateScheduleEntry(ctx context.Context, entry domain.ScheduleEntry) (string, error) {
	query := `INSERT INTO schedule_entries (id, lesson_id, date) VALUES ($1, $2, $3)`
	_, err := r.db.ExecContext(ctx, query, entry.ID, entry.LessonID, entry.Date)
	if err != nil {
		return "", fmt.Errorf("failed to create schedule entry: %w", err)
	}
	return entry.ID, nil
}

func (r *ScheduleEntryRepository) GetScheduleEntryByID(ctx context.Context, id string) (*domain.ScheduleEntry, error) {
	var entry domain.ScheduleEntry
	query := `SELECT id, lesson_id, date FROM schedule_entries WHERE id = $1`
	err := r.db.GetContext(ctx, &entry, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get schedule entry by id: %w", err)
	}
	return &entry, nil
}

func (r *ScheduleEntryRepository) GetScheduleEntriesByDateRange(ctx context.Context, from, to time.Time) ([]domain.ScheduleEntry, error) {
	var entries []domain.ScheduleEntry
	query := `SELECT id, lesson_id, date FROM schedule_entries WHERE date >= $1 AND date <= $2 ORDER BY date`  
	err := r.db.SelectContext(ctx, &entries, query, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule entries by date range: %w", err)
	}
	return entries, nil
}

func (r *ScheduleEntryRepository) DeleteScheduleEntry(ctx context.Context, id string) error {
	query := `DELETE FROM schedule_entries WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete schedule entry: %w", err)
	}
	return nil
}
