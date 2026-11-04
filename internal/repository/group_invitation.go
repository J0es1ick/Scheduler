package repository

import (
	"context"
	"database/sql"
	"github.com/J0es1ick/Scheduler/internal/database"
)

func (r *SubscriptionRepository) AcceptGroupInvitation(ctx context.Context, id, userID, groupID string, subgroup int, primary bool) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = database.LockGroupReferences(ctx, tx, false); err != nil {
		return err
	}
	if err = lockActiveGroup(ctx, tx, groupID); err != nil {
		return err
	}
	if err = lockUser(ctx, tx, userID); err != nil {
		return err
	}
	var valid bool
	if err = tx.GetContext(ctx, &valid, `SELECT role='student' AND NOT bot_blocked FROM users WHERE id=$1`, userID); err != nil {
		return err
	}
	if !valid {
		return sql.ErrNoRows
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO subscriptions(id,user_id,object_id,object_type,subgroup) VALUES($1,$2,$3,'group',$4)
 ON CONFLICT(user_id,object_id,object_type) DO UPDATE SET subgroup=EXCLUDED.subgroup,updated_at=NOW()`, id, userID, groupID, subgroup)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE users SET default_group_id=CASE WHEN $3 OR default_group_id IS NULL THEN $2 ELSE default_group_id END,
 daily_setup=CASE WHEN default_group_id IS NULL THEN 'done' ELSE daily_setup END,updated_at=clock_timestamp() WHERE id=$1`, userID, groupID, primary)
	if err != nil {
		return err
	}
	return tx.Commit()
}
