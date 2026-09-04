package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const privacyDeletionLease = 2 * time.Minute

var ErrPrivacyDeletionClaimLost = errors.New("privacy deletion claim was lost")

type PrivacyDeletionRepository struct {
	db *sqlx.DB
}

func NewPrivacyDeletionRepository(db *sqlx.DB) *PrivacyDeletionRepository {
	return &PrivacyDeletionRepository{db: db}
}

func enqueuePrivacyDeletion(ctx context.Context, db sqlx.ExtContext, userID string) (string, error) {
	var requestID string
	err := sqlx.GetContext(ctx, db, &requestID, `SELECT enqueue_privacy_deletion($1)`, userID)
	if err != nil {
		return "", fmt.Errorf("enqueue privacy deletion for user %s: %w", userID, err)
	}
	return requestID, nil
}

func (r *PrivacyDeletionRepository) Enqueue(ctx context.Context, userID string) (string, error) {
	return enqueuePrivacyDeletion(ctx, r.db, userID)
}

func (r *PrivacyDeletionRepository) ClaimPending(ctx context.Context, limit int) ([]domain.PrivacyDeletionRequest, error) {
	if limit <= 0 {
		return []domain.PrivacyDeletionRequest{}, nil
	}
	claimToken := uuid.NewString()
	var requests []domain.PrivacyDeletionRequest
	err := r.db.SelectContext(ctx, &requests, `
		WITH candidates AS (
			SELECT id
			FROM privacy_deletion_requests
			WHERE (status='pending' AND next_attempt_at<=clock_timestamp())
			   OR (status='processing' AND lease_expires_at<=clock_timestamp())
			ORDER BY requested_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE privacy_deletion_requests request
		SET status='processing', attempts=request.attempts+1, claim_token=$2,
			lease_expires_at=clock_timestamp()+($3 * INTERVAL '1 second'), updated_at=NOW()
		FROM candidates
		WHERE request.id=candidates.id
		RETURNING request.id, request.user_id, request.attempts,
			request.claim_token, request.lease_expires_at`, limit, claimToken, int(privacyDeletionLease/time.Second))
	if err != nil {
		return nil, fmt.Errorf("claim privacy deletions: %w", err)
	}
	if requests == nil {
		requests = []domain.PrivacyDeletionRequest{}
	}
	return requests, nil
}

func (r *PrivacyDeletionRepository) Complete(ctx context.Context, id, claimToken string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("complete privacy deletion %s: begin: %w", id, err)
	}
	defer tx.Rollback()
	var userID string
	err = tx.GetContext(ctx, &userID, `
		SELECT user_id FROM privacy_deletion_requests
		WHERE id=$1 AND status='processing' AND claim_token=$2
		  AND lease_expires_at>clock_timestamp()
		FOR UPDATE`, id, claimToken)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPrivacyDeletionClaimLost
	}
	if err != nil {
		return fmt.Errorf("complete privacy deletion %s: lock request: %w", id, err)
	}
	var deleted bool
	if err = tx.GetContext(ctx, &deleted, `SELECT execute_privacy_deletion($1, $2)`, userID, "deleted:"+uuid.NewString()); err != nil {
		return fmt.Errorf("complete privacy deletion %s: delete profile: %w", id, err)
	}
	result, err := tx.ExecContext(ctx, `
		DELETE FROM privacy_deletion_requests
		WHERE id=$1 AND status='processing' AND claim_token=$2
		  AND lease_expires_at>clock_timestamp()`, id, claimToken)
	if err != nil {
		return fmt.Errorf("complete privacy deletion %s: %w", id, err)
	}
	if err = requirePrivacyDeletionClaim(result); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("complete privacy deletion %s: commit: %w", id, err)
	}
	return nil
}

func (r *PrivacyDeletionRepository) Renew(ctx context.Context, id, claimToken string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE privacy_deletion_requests
		SET lease_expires_at=clock_timestamp()+($3 * INTERVAL '1 second'), updated_at=NOW()
		WHERE id=$1 AND status='processing' AND claim_token=$2
		  AND lease_expires_at>clock_timestamp()`, id, claimToken, int(privacyDeletionLease/time.Second))
	if err != nil {
		return fmt.Errorf("renew privacy deletion %s: %w", id, err)
	}
	return requirePrivacyDeletionClaim(result)
}

func (r *PrivacyDeletionRepository) Retry(ctx context.Context, id, claimToken string, cause error) error {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	runes := []rune(message)
	if len(runes) > 4000 {
		message = string(runes[:4000])
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE privacy_deletion_requests
		SET status='pending', claim_token='', lease_expires_at=NULL,
			next_attempt_at=clock_timestamp()+(LEAST(3600, 5 * POWER(2, LEAST(attempts, 9))) * INTERVAL '1 second'),
			last_error=$3, updated_at=NOW()
		WHERE id=$1 AND status='processing' AND claim_token=$2
		  AND lease_expires_at>clock_timestamp()`, id, claimToken, message)
	if err != nil {
		return fmt.Errorf("retry privacy deletion %s: %w", id, err)
	}
	return requirePrivacyDeletionClaim(result)
}

func requirePrivacyDeletionClaim(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect privacy deletion claim: %w", err)
	}
	if rows != 1 {
		return ErrPrivacyDeletionClaimLost
	}
	return nil
}
