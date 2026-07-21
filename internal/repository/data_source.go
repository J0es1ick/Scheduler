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
		`INSERT INTO data_sources (id, university_id, adapter_type, config, update_interval, last_run_at, last_error, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NULL, '', $6, $7)`,
		id, universityID, adapterType, config, updateInterval, now, now)
	if err != nil {
		return "", fmt.Errorf("create data source: %w", err)
	}
	return id, nil
}

func (r *DataSourceRepository) GetDataSourceByID(ctx context.Context, id string) (*domain.DataSource, error) {
	var ds domain.DataSource
	err := r.db.GetContext(ctx, &ds,
		`SELECT id, university_id, adapter_type, config, update_interval,
		        COALESCE(last_run_at, '1970-01-01'::timestamp) AS last_run_at,
		        COALESCE(last_error, '') AS last_error,
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
		     update_interval = $4, last_run_at = $5, last_error = $6, updated_at = $7
		 WHERE id = $8`,
		ds.UniversityID, ds.AdapterType, ds.Config,
		ds.UpdateInterval, ds.LastRunAt, ds.LastError, time.Now(),
		ds.ID)
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
		`SELECT id, university_id, adapter_type, config, update_interval,
		        COALESCE(last_run_at, '1970-01-01'::timestamp) AS last_run_at,
		        COALESCE(last_error, '') AS last_error,
		        created_at, updated_at
		 FROM data_sources
		 WHERE COALESCE(last_error, '') <> ''
		    OR last_run_at IS NULL
		    OR last_run_at + make_interval(secs => update_interval) < NOW()`)
	if err != nil {
		return nil, fmt.Errorf("list active data sources: %w", err)
	}
	return sources, nil
}

func (r *DataSourceRepository) ListDataSourcesByUniversityID(ctx context.Context, universityID string) ([]*domain.DataSource, error) {
	var sources []*domain.DataSource
	err := r.db.SelectContext(ctx, &sources,
		`SELECT id, university_id, adapter_type, config, update_interval,
		        COALESCE(last_run_at, '1970-01-01'::timestamp) AS last_run_at,
		        COALESCE(last_error, '') AS last_error,
		        created_at, updated_at
		 FROM data_sources WHERE university_id = $1`, universityID)
	if err != nil {
		return nil, fmt.Errorf("list data sources for university %s: %w", universityID, err)
	}
	return sources, nil
}
