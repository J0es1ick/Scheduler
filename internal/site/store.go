package site

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type Store struct {
	db *sqlx.DB
}

func NewStore(db *sqlx.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) PublicInfo(
	ctx context.Context,
	projectURL string,
	botURL string,
) (*PublicInfo, error) {
	result := &PublicInfo{
		ProjectURL: projectURL,
		BotURL:     botURL,
		UpdatedAt:  time.Now(),
	}
	if err := s.db.GetContext(ctx, result, `
		SELECT
			(SELECT COUNT(*) FROM universities WHERE is_active) AS universities,
			(SELECT COUNT(*) FROM groups WHERE is_active) AS groups,
			(SELECT COUNT(*) FROM effective_lessons l
				JOIN groups g ON g.id=l.group_id WHERE g.is_active) AS lessons,
			(SELECT COUNT(*) FROM users) AS users,
			(SELECT COUNT(*) FROM subscriptions) AS subscriptions`); err != nil {
		return nil, fmt.Errorf("load public project info: %w", err)
	}
	if err := s.db.SelectContext(ctx, &result.UniversityNames, `
		SELECT name
		FROM universities
		WHERE is_active
		ORDER BY name`); err != nil {
		return nil, fmt.Errorf("load public university names: %w", err)
	}
	if result.UniversityNames == nil {
		result.UniversityNames = []string{}
	}
	if err := s.db.SelectContext(ctx, &result.Sources, `
		SELECT u.name AS university_name, COALESCE(u.schedule_url, '') AS schedule_url,
			(COALESCE(u.schedule_url, '') LIKE 'https://%') AS secure,
			ds.last_success_at,
			CASE
				WHEN NOT ds.is_enabled THEN 'disabled'
				WHEN COALESCE(ds.last_error, '')<>'' THEN 'error'
				WHEN ds.last_success_at IS NULL OR
					NOW()-ds.last_success_at > (ds.update_interval*2+300)*INTERVAL '1 second' THEN 'stale'
				ELSE 'current'
			END AS state
		FROM data_sources ds
		JOIN universities u ON u.id=ds.university_id
		WHERE u.is_active AND ds.lifecycle_status='active'
		ORDER BY u.name`); err != nil {
		return nil, fmt.Errorf("load public source status: %w", err)
	}
	if result.Sources == nil {
		result.Sources = []PublicSourceStatus{}
	}
	return result, nil
}
