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
	return result, nil
}
