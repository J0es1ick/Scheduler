package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const publicationReconciliationClaimTTL = 30 * time.Minute

type pendingPublicationReconciliation struct {
	UniversityID string `db:"university_id"`
	SnapshotID   string `db:"snapshot_id"`
}

func (r *ParserSnapshotRepository) ReconcilePendingPublications(ctx context.Context) (int, error) {
	claimToken := uuid.NewString()
	completed := 0
	poll := time.NewTicker(200 * time.Millisecond)
	defer poll.Stop()

	for {
		item, err := r.claimPublicationReconciliation(ctx, claimToken)
		if err != nil {
			return completed, err
		}
		if item != nil {
			if _, err = r.RestorePublishedSnapshot(ctx, item.SnapshotID); err != nil {
				if recordErr := r.releasePublicationReconciliation(
					item.UniversityID, claimToken, err,
				); recordErr != nil {
					return completed, fmt.Errorf(
						"reconcile university %s: %v; record failure: %w",
						item.UniversityID, err, recordErr,
					)
				}
				return completed, fmt.Errorf(
					"reconcile university %s from snapshot %s: %w",
					item.UniversityID, item.SnapshotID, err,
				)
			}
			result, deleteErr := r.db.ExecContext(ctx, `
				DELETE FROM publication_reconciliation_queue
				WHERE university_id=$1 AND claim_token=$2`, item.UniversityID, claimToken)
			if deleteErr != nil {
				return completed, fmt.Errorf(
					"complete publication reconciliation for university %s: %w",
					item.UniversityID, deleteErr,
				)
			}
			if count, _ := result.RowsAffected(); count != 1 {
				return completed, fmt.Errorf(
					"complete publication reconciliation for university %s: claim was lost",
					item.UniversityID,
				)
			}
			completed++
			continue
		}

		var remaining int
		if err = r.db.GetContext(ctx, &remaining,
			`SELECT COUNT(*)::int FROM publication_reconciliation_queue`); err != nil {
			return completed, fmt.Errorf("count pending publication reconciliations: %w", err)
		}
		if remaining == 0 {
			return completed, nil
		}
		select {
		case <-ctx.Done():
			return completed, fmt.Errorf("wait for publication reconciliation claim: %w", ctx.Err())
		case <-poll.C:
		}
	}
}

func (r *ParserSnapshotRepository) EnsureNoPendingPublicationReconciliations(ctx context.Context) error {
	var pending int
	if err := r.db.GetContext(ctx, &pending,
		`SELECT COUNT(*)::int FROM publication_reconciliation_queue`); err != nil {
		return fmt.Errorf("count pending publication reconciliations: %w", err)
	}
	if pending > 0 {
		return fmt.Errorf(
			"%d publication reconciliation(s) are pending; run scheduler-migrate before starting services",
			pending,
		)
	}
	return nil
}

func (r *ParserSnapshotRepository) claimPublicationReconciliation(
	ctx context.Context,
	claimToken string,
) (*pendingPublicationReconciliation, error) {
	var item pendingPublicationReconciliation
	err := r.db.GetContext(ctx, &item, `
		WITH candidate AS (
			SELECT university_id
			FROM publication_reconciliation_queue
			WHERE claim_token=''
			   OR claimed_at < NOW() - ($2 * INTERVAL '1 second')
			ORDER BY created_at, university_id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE publication_reconciliation_queue queue
		SET claim_token=$1, claimed_at=NOW(), attempts=attempts+1
		FROM candidate
		WHERE queue.university_id=candidate.university_id
		RETURNING queue.university_id, queue.snapshot_id`,
		claimToken, int(publicationReconciliationClaimTTL/time.Second),
	)
	if err == nil {
		return &item, nil
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return nil, fmt.Errorf("claim pending publication reconciliation: %w", err)
}

func (r *ParserSnapshotRepository) releasePublicationReconciliation(
	universityID, claimToken string,
	reconciliationErr error,
) error {
	message := reconciliationErr.Error()
	if len(message) > 2000 {
		message = message[:2000]
	}
	recordCtx, recordCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer recordCancel()
	_, err := r.db.ExecContext(recordCtx, `
		UPDATE publication_reconciliation_queue
		SET claim_token='', claimed_at=NULL, last_error=$3
		WHERE university_id=$1 AND claim_token=$2`, universityID, claimToken, message)
	return err
}
