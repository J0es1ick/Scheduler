package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

func RunRetention(ctx context.Context, db *sqlx.DB, task string, statements []string) (bool, error) {
	conn, err := db.Connx(ctx)
	if err != nil {
		return false, fmt.Errorf("%s retention: acquire connection: %w", task, err)
	}
	defer conn.Close()
	lockName := "scheduler-retention-" + task
	var locked bool
	if err = conn.GetContext(ctx, &locked, `SELECT pg_try_advisory_lock(hashtext($1))`, lockName); err != nil {
		return false, fmt.Errorf("%s retention: acquire lock: %w", task, err)
	}
	if !locked {
		return false, nil
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock(hashtext($1))`, lockName)
	}()
	if _, err = conn.ExecContext(ctx, `SET statement_timeout='5min'`); err != nil {
		return false, fmt.Errorf("%s retention: configure timeout: %w", task, err)
	}
	defer func() {
		resetCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(resetCtx, `RESET statement_timeout`)
	}()
	var due bool
	if err = conn.GetContext(ctx, &due, `
		SELECT last_run_at<NOW()-INTERVAL '24 hours'
		FROM operational_maintenance WHERE task_name=$1`, task); err != nil {
		return false, fmt.Errorf("%s retention: inspect schedule: %w", task, err)
	}
	if !due {
		return false, nil
	}
	for _, statement := range statements {
		for batch := 0; batch < 1000; batch++ {
			result, cleanupErr := conn.ExecContext(ctx, statement)
			if cleanupErr != nil {
				return false, fmt.Errorf("%s retention: cleanup: %w", task, cleanupErr)
			}
			rows, rowsErr := result.RowsAffected()
			if rowsErr != nil {
				return false, fmt.Errorf("%s retention: count cleanup: %w", task, rowsErr)
			}
			if rows < 1000 {
				break
			}
			if batch == 999 {
				return false, errors.New(task + " retention: batch safety limit reached")
			}
		}
	}
	if _, err = conn.ExecContext(ctx, `
		UPDATE operational_maintenance SET last_run_at=NOW() WHERE task_name=$1`, task); err != nil {
		return false, fmt.Errorf("%s retention: record completion: %w", task, err)
	}
	return true, nil
}
