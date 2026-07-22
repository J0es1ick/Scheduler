package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

const maxNotificationAttempts = 5

// NotificationRepository stores schedule change events and their per-user
// deliveries. Deliveries are claimed with SKIP LOCKED so several bot replicas
// can safely process the same PostgreSQL queue.
type NotificationRepository struct {
	db *sqlx.DB
}

func NewNotificationRepository(db *sqlx.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) EnqueueScheduleChange(
	ctx context.Context,
	eventID, groupID, source, summary string,
) error {
	if _, err := r.db.ExecContext(ctx,
		`SELECT enqueue_schedule_change($1, $2, $3, $4)`,
		eventID, groupID, source, summary,
	); err != nil {
		return fmt.Errorf("enqueue schedule change for group %s: %w", groupID, err)
	}
	return nil
}

// ClaimPending atomically leases due deliveries for two minutes. A crashed
// worker therefore cannot leave a notification permanently stuck.
func (r *NotificationRepository) ClaimPending(ctx context.Context, limit int) ([]domain.NotificationDelivery, error) {
	if limit <= 0 {
		return []domain.NotificationDelivery{}, nil
	}
	var items []domain.NotificationDelivery
	err := r.db.SelectContext(ctx, &items, `
		WITH candidates AS (
			SELECT id
			FROM notification_deliveries
			WHERE status = 'pending' AND next_attempt_at <= NOW()
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		), claimed AS (
			UPDATE notification_deliveries d
			SET attempts = d.attempts + 1,
				next_attempt_at = NOW() + INTERVAL '2 minutes',
				updated_at = NOW()
			FROM candidates c
			WHERE d.id = c.id
			RETURNING d.id, d.event_id, d.user_id, d.attempts,
				d.next_attempt_at, d.created_at, d.delivered_at
		)
		SELECT c.id, c.event_id, c.user_id, e.group_id,
			g.name AS group_name, u.name AS university_name,
			e.source, e.summary, c.attempts, c.next_attempt_at,
			c.created_at, c.delivered_at
		FROM claimed c
		JOIN schedule_change_events e ON e.id = c.event_id
		JOIN groups g ON g.id = e.group_id
		JOIN universities u ON u.id = g.university_id
		ORDER BY c.created_at, c.id`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim pending notifications: %w", err)
	}
	if items == nil {
		items = []domain.NotificationDelivery{}
	}
	return items, nil
}

func (r *NotificationRepository) ClaimBotOutbox(ctx context.Context, limit int) ([]domain.BotOutboxDelivery, error) {
	if limit <= 0 {
		return []domain.BotOutboxDelivery{}, nil
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE bot_outbox o
		SET status='cancelled', updated_at=NOW()
		FROM users u
		WHERE o.user_id=u.id AND o.kind='support_request'
			AND o.status='pending' AND NOT u.is_admin`); err != nil {
		return nil, fmt.Errorf("cancel support messages for former admins: %w", err)
	}
	var items []domain.BotOutboxDelivery
	err := r.db.SelectContext(ctx, &items, `
		WITH candidates AS (
			SELECT id
			FROM bot_outbox
			WHERE status='pending' AND next_attempt_at <= NOW()
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		), claimed AS (
			UPDATE bot_outbox o
			SET attempts=o.attempts+1,
				next_attempt_at=NOW()+INTERVAL '2 minutes',
				updated_at=NOW()
			FROM candidates c
			WHERE o.id=c.id
			RETURNING o.id, o.user_id, o.request_id, o.kind, o.body, o.attempts
		)
		SELECT id, user_id, COALESCE(request_id, '') AS request_id, kind, body, attempts
		FROM claimed
		ORDER BY id`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim bot outbox: %w", err)
	}
	if items == nil {
		items = []domain.BotOutboxDelivery{}
	}
	return items, nil
}

func (r *NotificationRepository) MarkBotOutboxDelivered(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE bot_outbox
		SET status='delivered', delivered_at=NOW(), last_error='', updated_at=NOW()
		WHERE id=$1`, id); err != nil {
		return fmt.Errorf("mark bot outbox %s delivered: %w", id, err)
	}
	return nil
}

func (r *NotificationRepository) IsBotOutboxActive(ctx context.Context, id string) (bool, error) {
	var active bool
	err := r.db.GetContext(ctx, &active, `
		SELECT EXISTS (
			SELECT 1
			FROM bot_outbox o
			JOIN users u ON u.id=o.user_id
			WHERE o.id=$1 AND o.status='pending'
				AND (o.kind <> 'support_request' OR u.is_admin)
		)`, id)
	if err != nil {
		return false, fmt.Errorf("check bot outbox %s eligibility: %w", id, err)
	}
	return active, nil
}

func (r *NotificationRepository) MarkBotOutboxCancelled(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE bot_outbox SET status='cancelled', updated_at=NOW()
		WHERE id=$1 AND status='pending'`, id); err != nil {
		return fmt.Errorf("cancel bot outbox %s: %w", id, err)
	}
	return nil
}

func (r *NotificationRepository) MarkBotOutboxFailed(
	ctx context.Context,
	id string,
	attempts int,
	retryAfter time.Duration,
	deliveryErr error,
) error {
	status := "pending"
	if attempts >= maxNotificationAttempts {
		status = "failed"
	}
	errorText := ""
	if deliveryErr != nil {
		errorText = deliveryErr.Error()
		if len(errorText) > 1000 {
			errorText = errorText[:1000]
		}
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE bot_outbox
		SET status=$2, next_attempt_at=NOW()+($3 * INTERVAL '1 second'),
			last_error=$4, updated_at=NOW()
		WHERE id=$1`, id, status, retryAfter.Seconds(), errorText); err != nil {
		return fmt.Errorf("mark bot outbox %s failed: %w", id, err)
	}
	return nil
}

func (r *NotificationRepository) MarkDelivered(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET status='delivered', delivered_at=NOW(), last_error='', updated_at=NOW()
		WHERE id=$1`, id); err != nil {
		return fmt.Errorf("mark notification %s delivered: %w", id, err)
	}
	return nil
}

func (r *NotificationRepository) MarkCancelled(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET status='cancelled', updated_at=NOW()
		WHERE id=$1 AND status='pending'`, id); err != nil {
		return fmt.Errorf("mark notification %s cancelled: %w", id, err)
	}
	return nil
}

func (r *NotificationRepository) IsDeliveryActive(ctx context.Context, id string) (bool, error) {
	var active bool
	err := r.db.GetContext(ctx, &active, `
		SELECT EXISTS (
			SELECT 1
			FROM notification_deliveries d
			JOIN schedule_change_events e ON e.id=d.event_id
			JOIN users u ON u.id=d.user_id AND u.notifications_enabled
			JOIN subscriptions s
				ON s.user_id=d.user_id
				AND s.object_type='group'
				AND s.object_id=e.group_id
			WHERE d.id=$1 AND d.status='pending'
		)`, id)
	if err != nil {
		return false, fmt.Errorf("check notification %s eligibility: %w", id, err)
	}
	return active, nil
}

func (r *NotificationRepository) MarkFailed(
	ctx context.Context,
	id string,
	attempts int,
	retryAfter time.Duration,
	deliveryErr error,
) error {
	status := "pending"
	if attempts >= maxNotificationAttempts {
		status = "failed"
	}
	errorText := ""
	if deliveryErr != nil {
		errorText = deliveryErr.Error()
		if len(errorText) > 1000 {
			errorText = errorText[:1000]
		}
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET status=$2, next_attempt_at=NOW() + ($3 * INTERVAL '1 second'),
			last_error=$4, updated_at=NOW()
		WHERE id=$1`, id, status, retryAfter.Seconds(), errorText); err != nil {
		return fmt.Errorf("mark notification %s failed: %w", id, err)
	}
	return nil
}

func (r *NotificationRepository) PruneCompleted(ctx context.Context, retention time.Duration) (int64, error) {
	eventResult, err := r.db.ExecContext(ctx, `
		DELETE FROM schedule_change_events e
		WHERE e.created_at < NOW() - ($1 * INTERVAL '1 second')
			AND NOT EXISTS (
				SELECT 1 FROM notification_deliveries d
				WHERE d.event_id=e.id AND d.status='pending'
			)`, retention.Seconds())
	if err != nil {
		return 0, fmt.Errorf("prune completed notifications: %w", err)
	}
	eventCount, err := eventResult.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned notifications: %w", err)
	}
	outboxResult, err := r.db.ExecContext(ctx, `
		DELETE FROM bot_outbox
		WHERE created_at < NOW() - ($1 * INTERVAL '1 second')
			AND status <> 'pending'`, retention.Seconds())
	if err != nil {
		return 0, fmt.Errorf("prune bot outbox: %w", err)
	}
	outboxCount, err := outboxResult.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned bot outbox: %w", err)
	}
	return eventCount + outboxCount, nil
}
