package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type WorkerStatusRepository struct {
	db *sqlx.DB
}

func NewWorkerStatusRepository(db *sqlx.DB) *WorkerStatusRepository {
	return &WorkerStatusRepository{db: db}
}

func (r *WorkerStatusRepository) Get(
	ctx context.Context,
	name string,
) (*domain.WorkerStatus, error) {
	var status domain.WorkerStatus
	err := r.db.GetContext(ctx, &status, `
		SELECT name, cursor, last_started_at, last_finished_at,
			last_full_cycle_at, last_duration_ms, last_processed,
			last_failures, last_error, updated_at
		FROM worker_status
		WHERE name=$1`, name)
	if err == sql.ErrNoRows {
		return &domain.WorkerStatus{Name: name}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get worker status %s: %w", name, err)
	}
	return &status, nil
}

func (r *WorkerStatusRepository) RecordRun(
	ctx context.Context,
	result domain.WorkerRunResult,
) error {
	duration := result.FinishedAt.Sub(result.StartedAt)
	if duration < 0 {
		duration = 0
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO worker_status (
			name, cursor, last_started_at, last_finished_at,
			last_full_cycle_at, last_duration_ms, last_processed,
			last_failures, last_error, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (name) DO UPDATE SET
			cursor=EXCLUDED.cursor,
			last_started_at=EXCLUDED.last_started_at,
			last_finished_at=EXCLUDED.last_finished_at,
			last_full_cycle_at=COALESCE(
				EXCLUDED.last_full_cycle_at,
				worker_status.last_full_cycle_at
			),
			last_duration_ms=EXCLUDED.last_duration_ms,
			last_processed=EXCLUDED.last_processed,
			last_failures=EXCLUDED.last_failures,
			last_error=EXCLUDED.last_error,
			updated_at=NOW()`,
		result.Name,
		result.Cursor,
		result.StartedAt,
		result.FinishedAt,
		result.LastFullCycleAt,
		duration.Milliseconds(),
		result.Processed,
		result.Failures,
		result.LastError,
	)
	if err != nil {
		return fmt.Errorf("record worker run %s: %w", result.Name, err)
	}
	return nil
}
