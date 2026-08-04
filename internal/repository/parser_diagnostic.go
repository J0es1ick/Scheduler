package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type ParserDiagnosticRepository struct {
	db *sqlx.DB
}

func NewParserDiagnosticRepository(db *sqlx.DB) *ParserDiagnosticRepository {
	return &ParserDiagnosticRepository{db: db}
}

func (r *ParserDiagnosticRepository) Create(
	ctx context.Context,
	diagnostic *domain.ParserDiagnostic,
) error {
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO parser_diagnostics (
			id, parse_log_id, data_source_id, stage, category, summary,
			group_id, http_status, content_type, response_size,
			response_sha256, response_preview, occurrences, metadata
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13, $14
		)`,
		diagnostic.ID,
		diagnostic.ParseLogID,
		diagnostic.DataSourceID,
		diagnostic.Stage,
		diagnostic.Category,
		diagnostic.Summary,
		diagnostic.GroupID,
		diagnostic.HTTPStatus,
		diagnostic.ContentType,
		diagnostic.ResponseSize,
		diagnostic.ResponseSHA256,
		diagnostic.ResponsePreview,
		diagnostic.Occurrences,
		string(diagnostic.Metadata),
	); err != nil {
		return fmt.Errorf("create parser diagnostic: %w", err)
	}
	return nil
}

func (r *ParserDiagnosticRepository) Prune(
	ctx context.Context,
	retention time.Duration,
) error {
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM parser_diagnostics
		WHERE created_at < NOW() - ($1 * INTERVAL '1 second')`,
		retention.Seconds(),
	); err != nil {
		return fmt.Errorf("prune parser diagnostics: %w", err)
	}
	return nil
}
