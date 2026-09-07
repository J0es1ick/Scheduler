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

const maxNotificationAttempts = 5
const notificationClaimLease = 2 * time.Minute

var ErrNotificationClaimLost = errors.New("notification delivery claim was lost")

type NotificationQueueDecision string

const (
	NotificationQueueReady  NotificationQueueDecision = "ready"
	NotificationQueueDefer  NotificationQueueDecision = "defer"
	NotificationQueueCancel NotificationQueueDecision = "cancel"
	NotificationQueueGone   NotificationQueueDecision = "gone"
)

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

func (r *NotificationRepository) EnqueueAdminAlert(
	ctx context.Context,
	alertID, body string,
) error {
	if _, err := r.db.ExecContext(ctx, `SELECT enqueue_admin_alert($1, $2)`, alertID, body); err != nil {
		return fmt.Errorf("enqueue admin alert %s: %w", alertID, err)
	}
	return nil
}

func (r *NotificationRepository) ClaimPending(ctx context.Context, limit int) ([]domain.NotificationDelivery, error) {
	if limit <= 0 {
		return []domain.NotificationDelivery{}, nil
	}
	if err := r.reconcileQueue(ctx); err != nil {
		return nil, err
	}
	claimToken := uuid.NewString()
	var items []domain.NotificationDelivery
	err := r.db.SelectContext(ctx, &items, `
		WITH candidates AS (
			SELECT d.id
			FROM notification_deliveries d
			JOIN notification_queue_eligibility q
			  ON q.queue_type='schedule' AND q.id=d.id
			WHERE d.status='pending'
			  AND d.next_attempt_at<=clock_timestamp()
			  AND (d.claim_token='' OR d.lease_expires_at<=clock_timestamp())
			  AND q.policy_decision='ready'
			ORDER BY d.created_at, d.id
			FOR UPDATE OF d SKIP LOCKED
			LIMIT $1
		), claimed AS (
			UPDATE notification_deliveries d
			SET attempts = d.attempts + 1,
				claim_token=$2,
				lease_expires_at=clock_timestamp()+($3 * INTERVAL '1 second'),
				updated_at = NOW()
			FROM candidates c
			WHERE d.id = c.id
			RETURNING d.id, d.event_id, d.user_id, d.attempts,
				d.claim_token, d.lease_expires_at, d.next_attempt_at,
				d.created_at, d.delivered_at
		)
		SELECT c.id, c.event_id, c.user_id, e.group_id,
			g.name AS group_name, u.name AS university_name,
			COALESCE(usr.default_group_id=e.group_id, FALSE) AS is_default,
			e.source, e.summary, c.attempts, c.claim_token,
			c.lease_expires_at, c.next_attempt_at,
			c.created_at, c.delivered_at
		FROM claimed c
		JOIN schedule_change_events e ON e.id = c.event_id
		JOIN groups g ON g.id = e.group_id
		JOIN universities u ON u.id = g.university_id
		JOIN users usr ON usr.id = c.user_id
		ORDER BY c.created_at, c.id`, limit, claimToken, int(notificationClaimLease/time.Second))
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
	if err := r.reconcileQueue(ctx); err != nil {
		return nil, err
	}
	claimToken := uuid.NewString()
	var items []domain.BotOutboxDelivery
	err := r.db.SelectContext(ctx, &items, `
		WITH ordinary AS (
 SELECT o.id FROM bot_outbox o JOIN notification_queue_eligibility q ON q.queue_type='outbox' AND q.id=o.id
 WHERE o.status='pending' AND o.kind<>'lesson_reminder' AND o.next_attempt_at<=clock_timestamp()
 AND (o.claim_token='' OR o.lease_expires_at<=clock_timestamp()) AND q.policy_decision='ready'
 ORDER BY o.created_at,o.id FOR UPDATE OF o SKIP LOCKED LIMIT GREATEST(1,$1/5)
 ), reminders AS (
			SELECT o.id
			FROM bot_outbox o
			JOIN notification_queue_eligibility q
			  ON q.queue_type='outbox' AND q.id=o.id
			WHERE o.status='pending' AND o.id NOT IN (SELECT id FROM ordinary)
			  AND o.next_attempt_at<=clock_timestamp()
			  AND (o.claim_token='' OR o.lease_expires_at<=clock_timestamp())
			  AND q.policy_decision='ready'
			ORDER BY o.expires_at NULLS LAST, o.created_at, o.id
			FOR UPDATE OF o SKIP LOCKED
			LIMIT ($1-(SELECT COUNT(*) FROM ordinary))
 ), candidates AS (SELECT id FROM ordinary UNION ALL SELECT id FROM reminders), claimed AS (
			UPDATE bot_outbox o
			SET attempts=o.attempts+1,
				claim_token=$2,
				lease_expires_at=clock_timestamp()+($3 * INTERVAL '1 second'),
				updated_at=NOW()
			FROM candidates c
			WHERE o.id=c.id
			RETURNING o.id, o.user_id, o.request_id, o.kind, o.body, o.attempts, o.group_id, o.expires_at, o.reminder_context,
				o.claim_token, o.lease_expires_at
		)
		SELECT id, user_id, COALESCE(request_id, '') AS request_id, kind, body, attempts, COALESCE(group_id, '') AS group_id, expires_at, reminder_context,
			claim_token, lease_expires_at
		FROM claimed
		ORDER BY expires_at NULLS LAST, id`, limit, claimToken, int(notificationClaimLease/time.Second))
	if err != nil {
		return nil, fmt.Errorf("claim bot outbox: %w", err)
	}
	if items == nil {
		items = []domain.BotOutboxDelivery{}
	}
	return items, nil
}

func (r *NotificationRepository) reconcileQueue(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `SELECT scheduler_reconcile_notification_queue()`); err != nil {
		return fmt.Errorf("reconcile notification queue: %w", err)
	}
	return nil
}

func (r *NotificationRepository) RenewDeliveryClaims(
	ctx context.Context,
	claimToken string,
	ids []string,
) error {
	return r.renewClaims(ctx, "notification_deliveries", claimToken, ids)
}

func (r *NotificationRepository) RenewBotOutboxClaims(
	ctx context.Context,
	claimToken string,
	ids []string,
) error {
	return r.renewClaims(ctx, "bot_outbox", claimToken, ids)
}

func (r *NotificationRepository) renewClaims(
	ctx context.Context,
	table string,
	claimToken string,
	ids []string,
) error {
	if claimToken == "" || len(ids) == 0 {
		return fmt.Errorf("%w: empty claim", ErrNotificationClaimLost)
	}
	query, args, err := sqlx.In(`
		WITH locked AS MATERIALIZED (
			SELECT id, status='pending' AND claim_token=?
				AND lease_expires_at>clock_timestamp() AS owned
			FROM `+table+` WHERE id IN (?) FOR UPDATE
		), renewed AS (
			UPDATE `+table+` queue
			SET lease_expires_at=clock_timestamp()+(? * INTERVAL '1 second'), updated_at=NOW()
			FROM locked WHERE queue.id=locked.id AND locked.owned
			RETURNING queue.id
		)
		SELECT (SELECT COUNT(*) FROM locked) AS existing,
			(SELECT COUNT(*) FROM renewed) AS renewed`,
		claimToken, ids, int(notificationClaimLease/time.Second))
	if err != nil {
		return fmt.Errorf("renew %s claim: %w", table, err)
	}
	var counts struct {
		Existing int `db:"existing"`
		Renewed  int `db:"renewed"`
	}
	if err = r.db.GetContext(ctx, &counts, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("renew %s claim: %w", table, err)
	}
	if counts.Existing != counts.Renewed {
		return fmt.Errorf("%w: %s", ErrNotificationClaimLost, table)
	}
	return nil
}

func (r *NotificationRepository) MarkBotOutboxDelivered(ctx context.Context, id, claimToken string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE bot_outbox
		SET status='delivered', delivered_at=NOW(), last_error='', updated_at=NOW(),
			claim_token='', lease_expires_at=NULL,
			cancel_requested_at=NULL, cancel_reason=''
		WHERE id=$1 AND claim_token=$2 AND status='pending'
		  AND lease_expires_at>clock_timestamp()`, id, claimToken)
	if err != nil {
		return fmt.Errorf("mark bot outbox %s delivered: %w", id, err)
	}
	return r.requireNotificationClaim(ctx, result, "bot_outbox", id)
}

func (r *NotificationRepository) IsBotOutboxActive(ctx context.Context, id, claimToken string) (bool, error) {
	decision, err := r.BotOutboxDecision(ctx, id, claimToken)
	if errors.Is(err, ErrNotificationClaimLost) {
		return false, nil
	}
	return decision == NotificationQueueReady, err
}

func (r *NotificationRepository) BotOutboxDecision(
	ctx context.Context,
	id, claimToken string,
) (NotificationQueueDecision, error) {
	return r.queueDecision(ctx, "outbox", id, claimToken)
}

func (r *NotificationRepository) MarkBotOutboxCancelled(ctx context.Context, id, claimToken string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE bot_outbox SET status='cancelled', updated_at=NOW(),
			claim_token='', lease_expires_at=NULL,
			cancel_requested_at=NULL, cancel_reason=''
		WHERE id=$1 AND claim_token=$2 AND status='pending'
		  AND lease_expires_at>clock_timestamp()`, id, claimToken)
	if err != nil {
		return fmt.Errorf("cancel bot outbox %s: %w", id, err)
	}
	return r.requireNotificationClaim(ctx, result, "bot_outbox", id)
}

func (r *NotificationRepository) MarkBotOutboxFailed(
	ctx context.Context,
	id, claimToken string,
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
	result, err := r.db.ExecContext(ctx, `
		UPDATE bot_outbox
		SET status=CASE WHEN cancel_requested_at IS NULL THEN $2 ELSE 'cancelled' END,
			next_attempt_at=NOW()+($3 * INTERVAL '1 second'),
			last_error=$4, updated_at=NOW(), claim_token='', lease_expires_at=NULL
			,cancel_requested_at=NULL, cancel_reason=''
		WHERE id=$1 AND claim_token=$5 AND status='pending'
		  AND lease_expires_at>clock_timestamp()`,
		id, status, retryAfter.Seconds(), errorText, claimToken)
	if err != nil {
		return fmt.Errorf("mark bot outbox %s failed: %w", id, err)
	}
	return r.requireNotificationClaim(ctx, result, "bot_outbox", id)
}

func (r *NotificationRepository) MarkBotOutboxRateLimited(
	ctx context.Context,
	id, claimToken string,
	retryAfter time.Duration,
	deliveryErr error,
) error {
	return r.markRateLimited(ctx, "bot_outbox", id, claimToken, retryAfter, deliveryErr)
}

func (r *NotificationRepository) MarkBotOutboxPermanentFailure(
	ctx context.Context,
	id, claimToken string,
	deliveryErr error,
) error {
	return r.markPermanentFailure(ctx, "bot_outbox", id, claimToken, deliveryErr)
}

func (r *NotificationRepository) MarkDelivered(ctx context.Context, id, claimToken string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET status='delivered', delivered_at=NOW(), last_error='', updated_at=NOW(),
			claim_token='', lease_expires_at=NULL,
			cancel_requested_at=NULL, cancel_reason=''
		WHERE id=$1 AND claim_token=$2 AND status='pending'
		  AND lease_expires_at>clock_timestamp()`, id, claimToken)
	if err != nil {
		return fmt.Errorf("mark notification %s delivered: %w", id, err)
	}
	return r.requireNotificationClaim(ctx, result, "notification_deliveries", id)
}

func (r *NotificationRepository) MarkCancelled(ctx context.Context, id, claimToken string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET status='cancelled', updated_at=NOW(), claim_token='', lease_expires_at=NULL
			,cancel_requested_at=NULL, cancel_reason=''
		WHERE id=$1 AND claim_token=$2 AND status='pending'
		  AND lease_expires_at>clock_timestamp()`, id, claimToken)
	if err != nil {
		return fmt.Errorf("mark notification %s cancelled: %w", id, err)
	}
	return r.requireNotificationClaim(ctx, result, "notification_deliveries", id)
}

func (r *NotificationRepository) IsDeliveryActive(ctx context.Context, id, claimToken string) (bool, error) {
	decision, err := r.DeliveryDecision(ctx, id, claimToken)
	if errors.Is(err, ErrNotificationClaimLost) {
		return false, nil
	}
	return decision == NotificationQueueReady, err
}

func (r *NotificationRepository) DeliveryDecision(
	ctx context.Context,
	id, claimToken string,
) (NotificationQueueDecision, error) {
	return r.queueDecision(ctx, "schedule", id, claimToken)
}

func (r *NotificationRepository) queueDecision(
	ctx context.Context,
	queueType, id, claimToken string,
) (NotificationQueueDecision, error) {
	var decision string
	err := r.db.GetContext(ctx, &decision, `
		SELECT policy_decision
		FROM notification_queue_eligibility
		WHERE queue_type=$1 AND id=$2 AND status='pending'
		  AND claim_token=$3 AND lease_expires_at>clock_timestamp()`,
		queueType, id, claimToken)
	if err == nil {
		switch NotificationQueueDecision(decision) {
		case NotificationQueueReady, NotificationQueueDefer, NotificationQueueCancel:
			return NotificationQueueDecision(decision), nil
		default:
			return "", fmt.Errorf("invalid %s queue decision %q for %s", queueType, decision, id)
		}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("check %s queue decision for %s: %w", queueType, id, err)
	}
	var status string
	err = r.db.GetContext(ctx, &status, `
		SELECT status FROM notification_queue_eligibility
		WHERE queue_type=$1 AND id=$2`, queueType, id)
	if errors.Is(err, sql.ErrNoRows) || status == "cancelled" {
		return NotificationQueueGone, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect %s queue claim for %s: %w", queueType, id, err)
	}
	return "", fmt.Errorf("%w: %s", ErrNotificationClaimLost, id)
}

func (r *NotificationRepository) MarkDeferred(ctx context.Context, id, claimToken string) error {
	return r.markDeferred(ctx, "notification_deliveries", id, claimToken)
}

func (r *NotificationRepository) MarkBotOutboxDeferred(ctx context.Context, id, claimToken string) error {
	return r.markDeferred(ctx, "bot_outbox", id, claimToken)
}

func (r *NotificationRepository) markDeferred(
	ctx context.Context,
	table, id, claimToken string,
) error {
	query := `UPDATE ` + table + `
		SET status=CASE WHEN cancel_requested_at IS NULL THEN 'pending' ELSE 'cancelled' END,
			attempts=CASE WHEN cancel_requested_at IS NULL THEN GREATEST(attempts-1, 0) ELSE attempts END,
			next_attempt_at=clock_timestamp()+INTERVAL '1 minute', updated_at=NOW(),
			claim_token='', lease_expires_at=NULL,
			cancel_requested_at=NULL, cancel_reason=''
		WHERE id=$1 AND claim_token=$2 AND status='pending'
		  AND lease_expires_at>clock_timestamp()`
	result, err := r.db.ExecContext(ctx, query, id, claimToken)
	if err != nil {
		return fmt.Errorf("defer %s %s: %w", table, id, err)
	}
	return r.requireNotificationClaim(ctx, result, table, id)
}

func (r *NotificationRepository) MarkFailed(
	ctx context.Context,
	id, claimToken string,
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
	result, err := r.db.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET status=CASE WHEN cancel_requested_at IS NULL THEN $2 ELSE 'cancelled' END,
			next_attempt_at=NOW() + ($3 * INTERVAL '1 second'),
			last_error=$4, updated_at=NOW(), claim_token='', lease_expires_at=NULL
			,cancel_requested_at=NULL, cancel_reason=''
		WHERE id=$1 AND claim_token=$5 AND status='pending'
		  AND lease_expires_at>clock_timestamp()`,
		id, status, retryAfter.Seconds(), errorText, claimToken)
	if err != nil {
		return fmt.Errorf("mark notification %s failed: %w", id, err)
	}
	return r.requireNotificationClaim(ctx, result, "notification_deliveries", id)
}

func (r *NotificationRepository) MarkRateLimited(
	ctx context.Context,
	id, claimToken string,
	retryAfter time.Duration,
	deliveryErr error,
) error {
	return r.markRateLimited(ctx, "notification_deliveries", id, claimToken, retryAfter, deliveryErr)
}

func (r *NotificationRepository) markRateLimited(
	ctx context.Context,
	table string,
	id, claimToken string,
	retryAfter time.Duration,
	deliveryErr error,
) error {
	errorText := ""
	if deliveryErr != nil {
		errorText = deliveryErr.Error()
		if len(errorText) > 1000 {
			errorText = errorText[:1000]
		}
	}
	query := `UPDATE ` + table + `
		SET status=CASE WHEN cancel_requested_at IS NULL THEN 'pending' ELSE 'cancelled' END,
			attempts=CASE WHEN cancel_requested_at IS NULL THEN GREATEST(attempts-1, 0) ELSE attempts END,
			next_attempt_at=NOW()+($2 * INTERVAL '1 second'),
			last_error=$3, updated_at=NOW(), claim_token='', lease_expires_at=NULL,
			cancel_requested_at=NULL, cancel_reason=''
		WHERE id=$1 AND claim_token=$4 AND status='pending'
		  AND lease_expires_at>clock_timestamp()`
	result, err := r.db.ExecContext(ctx, query,
		id, retryAfter.Seconds(), errorText, claimToken)
	if err != nil {
		return fmt.Errorf("mark %s %s rate limited: %w", table, id, err)
	}
	return r.requireNotificationClaim(ctx, result, table, id)
}

func (r *NotificationRepository) MarkPermanentFailure(
	ctx context.Context,
	id, claimToken string,
	deliveryErr error,
) error {
	return r.markPermanentFailure(ctx, "notification_deliveries", id, claimToken, deliveryErr)
}

func (r *NotificationRepository) markPermanentFailure(
	ctx context.Context,
	table string,
	id, claimToken string,
	deliveryErr error,
) error {
	errorText := ""
	if deliveryErr != nil {
		errorText = deliveryErr.Error()
		if len(errorText) > 1000 {
			errorText = errorText[:1000]
		}
	}
	query := `UPDATE ` + table + `
		SET status=CASE WHEN cancel_requested_at IS NULL THEN 'failed' ELSE 'cancelled' END,
			last_error=$2, updated_at=NOW(), claim_token='', lease_expires_at=NULL,
			cancel_requested_at=NULL, cancel_reason=''
		WHERE id=$1 AND claim_token=$3 AND status='pending'
		  AND lease_expires_at>clock_timestamp()`
	result, err := r.db.ExecContext(ctx, query, id, errorText, claimToken)
	if err != nil {
		return fmt.Errorf("mark %s %s permanently failed: %w", table, id, err)
	}
	return r.requireNotificationClaim(ctx, result, table, id)
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

var ErrNotificationGone = errors.New("notification queue item no longer exists")

func (r *NotificationRepository) requireNotificationClaim(ctx context.Context, result sql.Result, table, id string) error {
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count notification claim updates for %s: %w", id, err)
	}
	if updated == 0 {
		var exists bool
		if err = r.db.GetContext(ctx, &exists, `SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id=$1)`, id); err != nil {
			return fmt.Errorf("inspect notification claim %s: %w", id, err)
		}
		if !exists {
			return fmt.Errorf("%w: %s", ErrNotificationGone, id)
		}
	}
	if updated != 1 {
		return fmt.Errorf("%w: %s", ErrNotificationClaimLost, id)
	}
	return nil
}

func (r *NotificationRepository) ReminderRecipient(ctx context.Context, userID, groupID string) (*domain.ReminderRecipient, error) {
	var recipient domain.ReminderRecipient
	err := r.db.GetContext(ctx, &recipient, `SELECT u.id AS user_id, g.id AS group_id, g.name AS group_name,
 un.name AS university_name, un.timezone, u.reminder_minutes, COALESCE(s.subgroup,0) AS subgroup
 FROM users u JOIN groups g ON g.id=u.default_group_id AND g.is_active
 JOIN universities un ON un.id=g.university_id AND un.is_active
 LEFT JOIN subscriptions s ON s.user_id=u.id AND s.object_id=g.id AND s.object_type='group'
 WHERE u.id=$1 AND g.id=$2 AND u.reminder_enabled`, userID, groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &recipient, nil
}
