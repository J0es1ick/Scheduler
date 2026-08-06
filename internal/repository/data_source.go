package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type DataSourceRepository struct {
	db *sqlx.DB
}

func (r *DataSourceRepository) TryAcquireRunLock(ctx context.Context, dataSourceID string) (release func() error, acquired bool, err error) {
	conn, err := r.db.Connx(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("acquire data source lock connection: %w", err)
	}
	if err = conn.GetContext(ctx, &acquired,
		`SELECT pg_try_advisory_lock(hashtext('scheduler-parser'), hashtext($1))`, dataSourceID,
	); err != nil {
		_ = conn.Close()
		return nil, false, fmt.Errorf("acquire data source lock %s: %w", dataSourceID, err)
	}
	if !acquired {
		_ = conn.Close()
		return nil, false, nil
	}
	release = func() error {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		unlockErr := conn.GetContext(releaseCtx, &unlocked,
			`SELECT pg_advisory_unlock(hashtext('scheduler-parser'), hashtext($1))`, dataSourceID,
		)
		closeErr := conn.Close()
		if unlockErr != nil {
			return fmt.Errorf("release data source lock %s: %w", dataSourceID, unlockErr)
		}
		if !unlocked {
			return fmt.Errorf("release data source lock %s: lock was not held", dataSourceID)
		}
		return closeErr
	}
	return release, true, nil
}

func NewDataSourceRepository(db *sqlx.DB) *DataSourceRepository {
	return &DataSourceRepository{db: db}
}

func (r *DataSourceRepository) CreateDataSource(ctx context.Context, id, universityID, adapterType, config string, updateInterval int) (string, error) {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO data_sources (id, university_id, adapter_type, config, is_enabled, update_interval, last_run_at, last_error, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, TRUE, $5, NULL, '', $6, $7)`,
		id, universityID, adapterType, config, updateInterval, now, now)
	if err != nil {
		return "", fmt.Errorf("create data source: %w", err)
	}
	return id, nil
}

func (r *DataSourceRepository) GetDataSourceByID(ctx context.Context, id string) (*domain.DataSource, error) {
	var ds domain.DataSource
	err := r.db.GetContext(ctx, &ds,
		`SELECT id, university_id, adapter_type, config, is_enabled, update_interval,
		        COALESCE(last_run_at, '1970-01-01'::timestamp) AS last_run_at,
		        last_success_at,
		        COALESCE(last_error, '') AS last_error,
		        consecutive_failures,
		        next_retry_at,
		        COALESCE(current_snapshot_id, '') AS current_snapshot_id,
		        created_at, updated_at
		 FROM data_sources WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get data source %s: %w", id, err)
	}
	return &ds, nil
}

func (r *DataSourceRepository) UpdateDataSource(ctx context.Context, ds *domain.DataSource) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE data_sources
		 SET university_id = $1, adapter_type = $2, config = $3,
		     is_enabled = $4, update_interval = $5, last_run_at = $6, last_success_at = $7,
		     last_error = $8, consecutive_failures = $9,
		     next_retry_at = $10,
		     current_snapshot_id = NULLIF($11, ''), updated_at = $12
		 WHERE id = $13`,
		ds.UniversityID, ds.AdapterType, ds.Config,
		ds.IsEnabled, ds.UpdateInterval, ds.LastRunAt, ds.LastSuccessAt,
		ds.LastError, ds.ConsecutiveFailures, ds.NextRetryAt,
		ds.CurrentSnapshotID, time.Now(), ds.ID)
	if err != nil {
		return fmt.Errorf("update data source %s: %w", ds.ID, err)
	}
	return nil
}

func (r *DataSourceRepository) DeleteDataSource(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM data_sources WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete data source %s: %w", id, err)
	}
	return nil
}

func (r *DataSourceRepository) ListActiveDataSources(ctx context.Context) ([]*domain.DataSource, error) {
	var sources []*domain.DataSource
	err := r.db.SelectContext(ctx, &sources,
		`SELECT id, university_id, adapter_type, config, is_enabled, update_interval,
		        COALESCE(last_run_at, '1970-01-01'::timestamp) AS last_run_at,
		        last_success_at,
		        COALESCE(last_error, '') AS last_error,
		        consecutive_failures,
		        next_retry_at,
		        COALESCE(current_snapshot_id, '') AS current_snapshot_id,
		        created_at, updated_at
		 FROM data_sources
		 WHERE is_enabled
		 AND NOT EXISTS (
			SELECT 1 FROM parser_snapshots ps
			WHERE ps.data_source_id=data_sources.id AND ps.status='quarantined'
		 )
		 AND (
			(
				COALESCE(last_error, '') <> ''
				AND COALESCE(next_retry_at, NOW()) <= NOW()
			)
			OR (
				COALESCE(last_error, '') = ''
				AND (
					last_run_at IS NULL
					OR last_run_at + make_interval(secs => update_interval) < NOW()
				)
			)
		 )`)
	if err != nil {
		return nil, fmt.Errorf("list active data sources: %w", err)
	}
	return sources, nil
}

func (r *DataSourceRepository) ListDataSourcesByUniversityID(ctx context.Context, universityID string) ([]*domain.DataSource, error) {
	var sources []*domain.DataSource
	err := r.db.SelectContext(ctx, &sources,
		`SELECT id, university_id, adapter_type, config, is_enabled, update_interval,
		        COALESCE(last_run_at, '1970-01-01'::timestamp) AS last_run_at,
		        last_success_at,
		        COALESCE(last_error, '') AS last_error,
		        consecutive_failures,
		        next_retry_at,
		        COALESCE(current_snapshot_id, '') AS current_snapshot_id,
		        created_at, updated_at
		 FROM data_sources WHERE university_id = $1`, universityID)
	if err != nil {
		return nil, fmt.Errorf("list data sources for university %s: %w", universityID, err)
	}
	return sources, nil
}

func (r *DataSourceRepository) RecordFailure(
	ctx context.Context,
	id, message string,
) (int, time.Time, error) {
	var state struct {
		Failures  int       `db:"consecutive_failures"`
		NextRetry time.Time `db:"next_retry_at"`
	}
	err := r.db.GetContext(ctx, &state, `
		UPDATE data_sources
		SET last_run_at=NOW(), last_error=$2,
			consecutive_failures=consecutive_failures+1,
			next_retry_at=NOW() + (
				LEAST(
					21600.0,
					300.0 * POWER(2.0, LEAST(consecutive_failures, 7))
				) * INTERVAL '1 second'
			),
			updated_at=NOW()
		WHERE id=$1
		RETURNING consecutive_failures, next_retry_at`, id, message)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("record data source failure %s: %w", id, err)
	}
	return state.Failures, state.NextRetry, nil
}

func (r *DataSourceRepository) RecordQuarantine(
	ctx context.Context,
	id, message string,
) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE data_sources
		SET last_run_at=NOW(), last_error=$2, next_retry_at=NULL,
			updated_at=NOW()
		WHERE id=$1`, id, message)
	if err != nil {
		return fmt.Errorf("record data source quarantine %s: %w", id, err)
	}
	return nil
}
