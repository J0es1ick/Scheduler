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

func NewDataSourceRepository(db *sqlx.DB) *DataSourceRepository {
	return &DataSourceRepository{db: db}
}

func (r *DataSourceRepository) CreateDataSource(ctx context.Context, id string, universityID string, adapterType string, config string, updateInterval int) (string, error) {
	query := `INSERT INTO data_sources (id, university_id, adapter_type, config, update_interval, last_run_at, last_error, created_at, updated_at)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	now := time.Now()
	_, err := r.db.ExecContext(ctx, query, id, universityID, adapterType, config, updateInterval, now, "", now, now)
	if err != nil {
		return "", fmt.Errorf("failed to create data source: %w", err)
	}
	return id, nil
}

func (r *DataSourceRepository) GetDataSourceByID(ctx context.Context, id string) (*domain.DataSource, error) {
	query := `SELECT id, university_id, adapter_type, config, update_interval, last_run_at, last_error, created_at, updated_at
			  FROM data_sources WHERE id = $1`
	var ds domain.DataSource
	err := r.db.GetContext(ctx, &ds, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get data source: %w", err)
	}
	return &ds, nil
}

func (r *DataSourceRepository) UpdateDataSource(ctx context.Context, ds *domain.DataSource) error {
	query := `UPDATE data_sources SET university_id = $1, adapter_type = $2, config = $3, update_interval = $4, last_run_at = $5, last_error = $6, updated_at = $7
			  WHERE id = $8`
	_, err := r.db.ExecContext(ctx, query, ds.UniversityID, ds.AdapterType, ds.Config, ds.UpdateInterval, ds.LastRunAt, ds.LastError, time.Now(), ds.ID)
	if err != nil {
		return fmt.Errorf("failed to update data source: %w", err)
	}
	return nil
}

func (r *DataSourceRepository) DeleteDataSource(ctx context.Context, id string) error {
	query := `DELETE FROM data_sources WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete data source: %w", err)
	}
	return nil
}

// func (r *DataSourceRepository) ListDataSourcesByUniversityID(ctx context.Context, universityID string) ([]*domain.DataSource, error) {
// 	query := `SELECT id, university_id, adapter_type, config, update_interval, last_run_at, last_error, created_at, updated_at
// 			  FROM data_sources WHERE university_id = $1`
// 	var dataSources []*domain.DataSource
// 	err := r.db.SelectContext(ctx, &dataSources, query, universityID)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to list data sources: %w", err)
// 	}
// 	return dataSources, nil
// }