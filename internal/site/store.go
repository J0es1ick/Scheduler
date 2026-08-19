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

func (s *Store) Ready(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	var marker int
	return s.db.GetContext(ctx, &marker, `
		SELECT COALESCE(MAX(marker), 1)
		FROM (
			SELECT 1 AS marker FROM public_site_statistics WHERE FALSE
			UNION ALL
			SELECT 1 FROM public_site_universities WHERE FALSE
			UNION ALL
			SELECT 1 FROM public_site_sources WHERE FALSE
		) required_views`)
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
		SELECT universities, groups, lessons, users, subscriptions
		FROM public_site_statistics`); err != nil {
		return nil, fmt.Errorf("load public project info: %w", err)
	}
	if err := s.db.SelectContext(ctx, &result.UniversityNames, `
		SELECT name
		FROM public_site_universities
		ORDER BY name`); err != nil {
		return nil, fmt.Errorf("load public university names: %w", err)
	}
	if result.UniversityNames == nil {
		result.UniversityNames = []string{}
	}
	if err := s.db.SelectContext(ctx, &result.Sources, `
		SELECT university_name, schedule_url, secure, last_success_at, state
		FROM public_site_sources
		ORDER BY university_name`); err != nil {
		return nil, fmt.Errorf("load public source status: %w", err)
	}
	if result.Sources == nil {
		result.Sources = []PublicSourceStatus{}
	}
	return result, nil
}
