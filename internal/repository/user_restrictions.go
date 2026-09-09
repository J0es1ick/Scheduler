package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func (r *UserRepository) Restrictions(ctx context.Context, userID string) (domain.UserRestrictions, error) {
	var result domain.UserRestrictions
	err := r.db.GetContext(ctx, &result, `SELECT bot_blocked, support_blocked FROM users WHERE id=$1`, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("read user restrictions: %w", err)
	}
	return result, nil
}
