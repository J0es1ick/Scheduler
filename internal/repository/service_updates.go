package repository

import (
	"context"
	"fmt"
)

func (r *UserRepository) SetServiceUpdates(ctx context.Context, userID string, enabled bool, promptKey string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE users SET service_updates_consent=$2,service_updates_answered_at=NOW(),updated_at=NOW()
		WHERE id=$1 AND ($3='' OR (service_updates_consent IS NULL AND service_updates_prompt_key=$3))`, userID, enabled, promptKey)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func (r *UserRepository) MarkServiceUpdatesPrompt(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET service_updates_prompt_delivered_at=COALESCE(service_updates_prompt_delivered_at,NOW()) WHERE id=$1`, userID)
	return err
}

func (r *NotificationRepository) EnqueueServiceUpdatesPrompts(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO bot_outbox(id,user_id,kind,body)
		SELECT 'updates-prompt:'||id,id,'service_updates_prompt',service_updates_prompt_key FROM users
		WHERE service_updates_backfill AND service_updates_consent IS NULL AND service_updates_prompt_delivered_at IS NULL AND NOT bot_blocked
		ON CONFLICT DO NOTHING`)
	if err != nil {
		return fmt.Errorf("enqueue service updates invitations: %w", err)
	}
	return nil
}
