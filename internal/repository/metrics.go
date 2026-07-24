package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type MetricsRepository struct {
	db *sqlx.DB
}

type sourceMetricsRow struct {
	UpdateInterval   int        `db:"update_interval"`
	LastSuccessAt    *time.Time `db:"last_success_at"`
	LastError        string     `db:"last_error"`
	QuarantinedCount int        `db:"quarantined_count"`
	LatestStatus     string     `db:"latest_status"`
	LatestStartedAt  *time.Time `db:"latest_started_at"`
}

func NewMetricsRepository(db *sqlx.DB) *MetricsRepository {
	return &MetricsRepository{db: db}
}

func (r *MetricsRepository) Get(ctx context.Context) (*domain.ServiceMetrics, error) {
	result := &domain.ServiceMetrics{CheckedAt: time.Now()}
	if err := r.db.GetContext(ctx, result, `
		SELECT
			(SELECT COUNT(*)::int FROM universities WHERE is_active) AS universities,
			(SELECT COUNT(*)::int FROM groups WHERE is_active) AS groups,
			(SELECT COUNT(*)::int FROM effective_lessons l
				JOIN groups g ON g.id=l.group_id WHERE g.is_active) AS lessons,
			(SELECT COUNT(*)::int FROM users) AS users,
			(SELECT COUNT(*)::int FROM subscriptions) AS subscriptions,
			(SELECT COUNT(*)::int FROM notification_deliveries
				WHERE status='pending') AS pending_notifications,
			(SELECT COUNT(*)::int FROM notification_deliveries
				WHERE status='failed') AS failed_notifications,
			(SELECT COUNT(*)::int FROM bot_outbox
				WHERE status='pending') AS pending_outbox,
			(SELECT COUNT(*)::int FROM bot_outbox
				WHERE status='failed') AS failed_outbox,
			(SELECT MAX(finished_at) FROM parse_logs
				WHERE status='success') AS last_successful_parse_at`); err != nil {
		return nil, fmt.Errorf("load service metrics: %w", err)
	}

	var sources []sourceMetricsRow
	if err := r.db.SelectContext(ctx, &sources, `
		SELECT
			ds.update_interval,
			ds.last_success_at,
			COALESCE(ds.last_error, '') AS last_error,
			(SELECT COUNT(*)::int FROM parser_snapshots ps
				WHERE ps.data_source_id=ds.id AND ps.status='quarantined') AS quarantined_count,
			COALESCE(latest.status, '') AS latest_status,
			latest.started_at AS latest_started_at
		FROM data_sources ds
		JOIN universities u ON u.id=ds.university_id AND u.is_active
		LEFT JOIN LATERAL (
			SELECT status, started_at
			FROM parse_logs
			WHERE data_source_id=ds.id
			ORDER BY started_at DESC
			LIMIT 1
		) latest ON TRUE
		ORDER BY ds.id`); err != nil {
		return nil, fmt.Errorf("load source metrics: %w", err)
	}

	result.SourcesTotal = len(sources)
	for _, source := range sources {
		switch {
		case source.LatestStatus == "running" &&
			source.LatestStartedAt != nil &&
			result.CheckedAt.Sub(*source.LatestStartedAt) < 2*time.Hour:
			result.SourcesRunning++
		case source.QuarantinedCount > 0:
			result.SourcesQuarantined++
		case source.LastError != "":
			result.SourcesError++
		case source.LastSuccessAt == nil ||
			result.CheckedAt.Sub(*source.LastSuccessAt) >
				2*time.Duration(source.UpdateInterval)*time.Second+5*time.Minute:
			result.SourcesStale++
		default:
			result.SourcesHealthy++
		}
	}
	return result, nil
}
