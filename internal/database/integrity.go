package database

import (
	"context"
	"database/sql"
	"fmt"
)

type IntegrityQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func CheckSubscriptionIntegrity(ctx context.Context, db IntegrityQuerier) error {
	var missing, orphan, teachers int64
	if err := db.QueryRowContext(ctx, `SELECT missing_default_subscriptions, orphan_group_subscriptions, teacher_subscriptions FROM subscription_integrity`).Scan(&missing, &orphan, &teachers); err != nil {
		return fmt.Errorf("check subscription integrity: %w", err)
	}
	if missing != 0 || orphan != 0 || teachers != 0 {
		return fmt.Errorf("subscription integrity: missing defaults=%d orphan groups=%d teacher subscriptions=%d", missing, orphan, teachers)
	}
	return nil
}
