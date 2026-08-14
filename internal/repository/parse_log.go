package repository

import (
	"context"
	"database/sql"
	"errors"
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
	var finishedAt *time.Time
	if status != "running" {
		finishedAt = &now
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO parse_logs (id, data_source_id, started_at, finished_at, status, records_fetched, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))`, id, dataSourceID, now, finishedAt, status, recordsFetched, errorMessage)
	if err != nil {
		return "", fmt.Errorf("failed to create parse log: %w", err)
	}
	return id, nil
}

func (r *ParseLogRepository) UpdateParseLog(ctx context.Context, id string, status string, recordsFetched int, errorMessage string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `UPDATE parse_logs SET finished_at = $1, status = $2, records_fetched = $3, error_message = NULLIF($4, '') WHERE id = $5`,
		now, status, recordsFetched, errorMessage, id)
	if err != nil {
		return fmt.Errorf("failed to update parse log: %w", err)
	}
	return nil
}

func (r *ParseLogRepository) FinalizeFailure(
	ctx context.Context,
	logID, dataSourceID string,
	recordsFetched int,
	errorMessage string,
) (int, time.Time, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("finalize parser failure: begin: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE parse_logs
		SET finished_at=NOW(), status='failed', records_fetched=$2,
			error_message=NULLIF($3, '')
		WHERE id=$1 AND status='running'`, logID, recordsFetched, errorMessage)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("finalize parser failure: update log: %w", err)
	}
	if count, countErr := result.RowsAffected(); countErr != nil || count != 1 {
		if countErr != nil {
			return 0, time.Time{}, fmt.Errorf("finalize parser failure: count log update: %w", countErr)
		}
		return 0, time.Time{}, fmt.Errorf("finalize parser failure: parse log %s is not running", logID)
	}
	var state struct {
		Failures  int       `db:"consecutive_failures"`
		NextRetry time.Time `db:"next_retry_at"`
	}
	if err = tx.GetContext(ctx, &state, `
		UPDATE data_sources
		SET last_run_at=NOW(), last_error=$2,
			consecutive_failures=consecutive_failures+1,
			next_retry_at=NOW() + (
				LEAST(21600.0, 300.0 * POWER(2.0, LEAST(consecutive_failures, 7))) *
				INTERVAL '1 second'
			),
			updated_at=NOW()
		WHERE id=$1
		RETURNING consecutive_failures, next_retry_at`, dataSourceID, errorMessage); err != nil {
		return 0, time.Time{}, fmt.Errorf("finalize parser failure: update source: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return 0, time.Time{}, fmt.Errorf("finalize parser failure: commit: %w", err)
	}
	return state.Failures, state.NextRetry, nil
}

func (r *ParseLogRepository) FinalizeQuarantine(
	ctx context.Context,
	logID, dataSourceID string,
	recordsFetched int,
	summary, sourceMessage string,
) error {
	return r.finalizeCandidate(ctx, logID, dataSourceID, "quarantined", recordsFetched, summary, sourceMessage)
}

func (r *ParseLogRepository) FinalizeAcceptedCandidate(
	ctx context.Context,
	logID, dataSourceID string,
	recordsFetched int,
) error {
	return r.finalizeCandidate(ctx, logID, dataSourceID, "success", recordsFetched, "", "")
}

func (r *ParseLogRepository) finalizeCandidate(
	ctx context.Context,
	logID, dataSourceID, status string,
	recordsFetched int,
	logMessage, sourceMessage string,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("finalize parser candidate: begin: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE parse_logs
		SET finished_at=NOW(), status=$2, records_fetched=$3,
			error_message=NULLIF($4, '')
		WHERE id=$1 AND status='running'`, logID, status, recordsFetched, logMessage)
	if err != nil {
		return fmt.Errorf("finalize parser candidate: update log: %w", err)
	}
	if count, countErr := result.RowsAffected(); countErr != nil || count != 1 {
		if countErr != nil {
			return fmt.Errorf("finalize parser candidate: count log update: %w", countErr)
		}
		return fmt.Errorf("finalize parser candidate: parse log %s is not running", logID)
	}
	if status == "quarantined" {
		_, err = tx.ExecContext(ctx, `
			UPDATE data_sources
			SET last_run_at=NOW(), last_error=$2, next_retry_at=NULL, updated_at=NOW()
			WHERE id=$1`, dataSourceID, sourceMessage)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE data_sources
			SET last_run_at=NOW(), last_error='', consecutive_failures=0,
				next_retry_at=NULL, updated_at=NOW()
			WHERE id=$1`, dataSourceID)
	}
	if err != nil {
		return fmt.Errorf("finalize parser candidate: update source: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("finalize parser candidate: commit: %w", err)
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

func (r *ParseLogRepository) FailInterrupted(ctx context.Context, olderThan time.Duration) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE parse_logs
		SET status='failed',
			finished_at=NOW(),
			error_message=COALESCE(error_message, 'Проход прерван до завершения процесса')
		WHERE status='running'
		  AND started_at < NOW()-($1 * INTERVAL '1 second')`,
		olderThan.Seconds(),
	)
	if err != nil {
		return 0, fmt.Errorf("mark interrupted parse logs: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count interrupted parse logs: %w", err)
	}
	return count, nil
}

func (r *ParseLogRepository) RunOperationalRetention(ctx context.Context) (bool, error) {
	conn, err := r.db.Connx(ctx)
	if err != nil {
		return false, fmt.Errorf("operational retention: acquire connection: %w", err)
	}
	defer conn.Close()
	var locked bool
	if err = conn.GetContext(ctx, &locked,
		`SELECT pg_try_advisory_lock(hashtext('scheduler-operational-retention'))`); err != nil {
		return false, fmt.Errorf("operational retention: acquire lock: %w", err)
	}
	if !locked {
		return false, nil
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx,
			`SELECT pg_advisory_unlock(hashtext('scheduler-operational-retention'))`)
	}()
	if _, err = conn.ExecContext(ctx, `SET statement_timeout='5min'`); err != nil {
		return false, fmt.Errorf("operational retention: configure timeout: %w", err)
	}
	defer func() {
		resetCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(resetCtx, `RESET statement_timeout`)
	}()
	var due bool
	if err = conn.GetContext(ctx, &due, `
		SELECT last_run_at<NOW()-INTERVAL '24 hours'
		FROM operational_maintenance WHERE task_name='retention'`); err != nil {
		return false, fmt.Errorf("operational retention: inspect schedule: %w", err)
	}
	if !due {
		return false, nil
	}
	statements := []string{
		`UPDATE connector_ingestion_runs
		 SET payload='{}'::jsonb
		 WHERE ctid IN (
			SELECT ctid FROM connector_ingestion_runs
			WHERE received_at<NOW()-INTERVAL '30 days'
			  AND status IN ('staged','quarantined','published','rejected','failed')
			  AND payload<>'{}'::jsonb LIMIT 1000
		 )`,
		`DELETE FROM connector_ingestion_runs WHERE ctid IN (
			SELECT ctid FROM connector_ingestion_runs
			WHERE received_at<NOW()-INTERVAL '180 days'
			  AND status IN ('staged','quarantined','published','rejected','failed') LIMIT 1000
		 )`,
		`DELETE FROM parser_snapshots WHERE ctid IN (
			SELECT snapshot.ctid FROM parser_snapshots snapshot
			WHERE snapshot.created_at<NOW()-INTERVAL '90 days'
			  AND snapshot.status IN ('quarantined','rejected')
			  AND NOT EXISTS (
				SELECT 1 FROM data_sources source WHERE source.current_snapshot_id=snapshot.id
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM publication_reconciliation_queue queue WHERE queue.snapshot_id=snapshot.id
			  ) LIMIT 1000
		 )`,
		`DELETE FROM parse_logs WHERE ctid IN (
			SELECT log.ctid FROM parse_logs log
			WHERE log.finished_at<NOW()-INTERVAL '180 days'
			  AND NOT EXISTS (
				SELECT 1 FROM parser_snapshots snapshot WHERE snapshot.parse_log_id=log.id
			  ) LIMIT 1000
		 )`,
		`DELETE FROM admin_audit_logs WHERE ctid IN (
			SELECT ctid FROM admin_audit_logs WHERE created_at<NOW()-INTERVAL '365 days' LIMIT 1000
		 )`,
		`DELETE FROM connector_request_nonces WHERE ctid IN (
			SELECT ctid FROM connector_request_nonces WHERE expires_at<NOW() LIMIT 1000
		 )`,
		`DELETE FROM admin_sessions WHERE ctid IN (
			SELECT ctid FROM admin_sessions WHERE expires_at<NOW() LIMIT 1000
		 )`,
		`DELETE FROM lesson_source_identities WHERE ctid IN (
			SELECT identity.ctid FROM lesson_source_identities identity
			WHERE identity.last_seen_at<NOW()-INTERVAL '730 days'
			  AND NOT EXISTS (SELECT 1 FROM lessons lesson WHERE lesson.id=identity.lesson_id)
			  AND NOT EXISTS (
				SELECT 1 FROM lesson_overrides override
				WHERE override.base_lesson_id=identity.lesson_id
			  ) LIMIT 1000
		 )`,
	}
	for _, statement := range statements {
		for batch := 0; batch < 1000; batch++ {
			result, cleanupErr := conn.ExecContext(ctx, statement)
			if cleanupErr != nil {
				return false, fmt.Errorf("operational retention: cleanup: %w", cleanupErr)
			}
			rows, rowsErr := result.RowsAffected()
			if rowsErr != nil {
				return false, fmt.Errorf("operational retention: count cleanup: %w", rowsErr)
			}
			if rows < 1000 {
				break
			}
			if batch == 999 {
				return false, errors.New("operational retention: batch safety limit reached")
			}
		}
	}
	if _, err = conn.ExecContext(ctx, `
		UPDATE operational_maintenance SET last_run_at=NOW() WHERE task_name='retention'`); err != nil {
		return false, fmt.Errorf("operational retention: record completion: %w", err)
	}
	return true, nil
}
