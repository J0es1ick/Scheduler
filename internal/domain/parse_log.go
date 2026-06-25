package domain

import "time"

type ParseLog struct {
	ID             string    `db:"id"`
	DataSourceID   string    `db:"data_source_id"`
	StartedAt      time.Time `db:"started_at"`
	FinishedAt     time.Time `db:"finished_at"`
	Status         string    `db:"status"`
	RecordsFetched int       `db:"records_fetched"`
	ErrorMessage   string    `db:"error_message"`
}