package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type ParseLogRepository struct {
	db *sqlx.DB
}

func NewParseLogRepository(db *sqlx.DB) *ParseLogRepository {
	return &ParseLogRepository{db: db}
}

func (r *ParseLogRepository) CreateParseLog(ctx context.Context, id string, dataSourceID string, status string, recordsFetched int, errorMessage string) (string, error) {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `INSERT INTO parse_logs (id, data_source_id, started_at, finished_at, status, records_fetched, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, dataSourceID, now, now, status, recordsFetched, errorMessage)
	if err != nil {
		return "", fmt.Errorf("failed to create parse log: %w", err)
	}
	return id, nil
}

func (r *ParseLogRepository) UpdateParseLog(ctx context.Context, id string, status string, recordsFetched int, errorMessage string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `UPDATE parse_logs SET finished_at = $1, status = $2, records_fetched = $3, error_message = $4 WHERE id = $5`,
		now, status, recordsFetched, errorMessage, id)
	if err != nil {
		return fmt.Errorf("failed to update parse log: %w", err)
	}
	return nil
}

func (r *ParseLogRepository) GetParseLogByID(ctx context.Context, id string) (*domain.ParseLog, error) {
	var log domain.ParseLog
	err := r.db.GetContext(ctx, &log, `SELECT id, data_source_id, started_at, finished_at, status, records_fetched, error_message FROM parse_logs WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get parse log by id: %w", err)
	}
	return &log, nil
}

func (r *ParseLogRepository) GetParseLogsByDataSourceID(ctx context.Context, dataSourceID string) ([]domain.ParseLog, error) {
	var logs []domain.ParseLog
	err := r.db.SelectContext(ctx, &logs, `SELECT id, data_source_id, started_at, finished_at, status, records_fetched, error_message FROM parse_logs WHERE data_source_id = $1 ORDER BY started_at DESC`, dataSourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get parse logs by data source id: %w", err)
	}
	return logs, nil
}

func (r *ParseLogRepository) GetAllParseLogs(ctx context.Context) ([]domain.ParseLog, error) {
	var logs []domain.ParseLog
	err := r.db.SelectContext(ctx, &logs, `SELECT id, data_source_id, started_at, finished_at, status, records_fetched, error_message FROM parse_logs ORDER BY started_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to get all parse logs: %w", err)
	}
	return logs, nil
}

func (r *ParseLogRepository) DeleteParseLog(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM parse_logs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete parse log: %w", err)
	}
	return nil
}
