package database

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

func LockGroupReferences(ctx context.Context, tx *sqlx.Tx, exclusive bool) error {
	query := `SELECT pg_advisory_xact_lock_shared(hashtext('scheduler-group-references'))`
	if exclusive {
		query = `SELECT pg_advisory_xact_lock(hashtext('scheduler-group-references'))`
	}
	if _, err := tx.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("lock group references: %w", err)
	}
	return nil
}
