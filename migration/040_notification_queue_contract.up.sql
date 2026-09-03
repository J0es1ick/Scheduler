ALTER TABLE notification_deliveries
    ADD COLUMN IF NOT EXISTS cancel_requested_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cancel_reason TEXT NOT NULL DEFAULT '';

ALTER TABLE bot_outbox
    ADD COLUMN IF NOT EXISTS cancel_requested_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cancel_reason TEXT NOT NULL DEFAULT '';

UPDATE notification_deliveries
SET claim_token='', lease_expires_at=NULL
WHERE status<>'pending';

UPDATE bot_outbox
SET claim_token='', lease_expires_at=NULL
WHERE status<>'pending';

ALTER TABLE notification_deliveries
    DROP CONSTRAINT IF EXISTS notification_deliveries_claim_shape_check,
    DROP CONSTRAINT IF EXISTS notification_deliveries_terminal_ownership_check,
    DROP CONSTRAINT IF EXISTS notification_deliveries_cancel_request_check;

ALTER TABLE notification_deliveries
    ADD CONSTRAINT notification_deliveries_claim_shape_check
        CHECK ((claim_token='' AND lease_expires_at IS NULL)
            OR (claim_token<>'' AND lease_expires_at IS NOT NULL)),
    ADD CONSTRAINT notification_deliveries_terminal_ownership_check
        CHECK (status='pending' OR (claim_token='' AND lease_expires_at IS NULL)),
    ADD CONSTRAINT notification_deliveries_cancel_request_check
        CHECK (cancel_requested_at IS NULL
            OR (status='pending' AND claim_token<>'' AND lease_expires_at IS NOT NULL));

ALTER TABLE bot_outbox
    DROP CONSTRAINT IF EXISTS bot_outbox_claim_shape_check,
    DROP CONSTRAINT IF EXISTS bot_outbox_terminal_ownership_check,
    DROP CONSTRAINT IF EXISTS bot_outbox_cancel_request_check;

ALTER TABLE bot_outbox
    ADD CONSTRAINT bot_outbox_claim_shape_check
        CHECK ((claim_token='' AND lease_expires_at IS NULL)
            OR (claim_token<>'' AND lease_expires_at IS NOT NULL)),
    ADD CONSTRAINT bot_outbox_terminal_ownership_check
        CHECK (status='pending' OR (claim_token='' AND lease_expires_at IS NULL)),
    ADD CONSTRAINT bot_outbox_cancel_request_check
        CHECK (cancel_requested_at IS NULL
            OR (status='pending' AND claim_token<>'' AND lease_expires_at IS NOT NULL));

CREATE OR REPLACE FUNCTION scheduler_in_quiet_hours(
    moment TIMESTAMPTZ,
    timezone_name TEXT,
    enabled BOOLEAN,
    quiet_start TIME,
    quiet_end TIME
) RETURNS BOOLEAN
LANGUAGE SQL
STABLE
PARALLEL SAFE
AS $$
    SELECT enabled AND CASE
        WHEN quiet_start < quiet_end THEN
            (moment AT TIME ZONE timezone_name)::time >= quiet_start
            AND (moment AT TIME ZONE timezone_name)::time < quiet_end
        ELSE
            (moment AT TIME ZONE timezone_name)::time >= quiet_start
            OR (moment AT TIME ZONE timezone_name)::time < quiet_end
    END
$$;

CREATE OR REPLACE VIEW notification_queue_eligibility AS
WITH queue_items AS (
    SELECT
        'schedule'::TEXT AS queue_type,
        d.id,
        d.created_at,
        d.status,
        d.next_attempt_at,
        d.claim_token,
        d.lease_expires_at,
        CASE
            WHEN d.status<>'pending' THEN 'terminal'
            WHEN d.cancel_requested_at IS NOT NULL THEN 'cancel'
            WHEN NOT usr.notifications_enabled THEN 'cancel'
            WHEN NOT EXISTS (
                SELECT 1
                FROM subscriptions sub
                WHERE sub.user_id=d.user_id
                  AND sub.object_type='group'
                  AND sub.object_id=e.group_id
            ) THEN 'cancel'
            WHEN scheduler_in_quiet_hours(
                clock_timestamp(), un.timezone, usr.quiet_hours_enabled,
                usr.quiet_hours_start, usr.quiet_hours_end
            ) THEN 'defer'
            ELSE 'ready'
        END AS policy_decision,
        CASE
            WHEN d.cancel_requested_at IS NOT NULL THEN COALESCE(NULLIF(d.cancel_reason, ''), 'cancel_requested')
            WHEN NOT usr.notifications_enabled THEN 'notifications_disabled'
            WHEN NOT EXISTS (
                SELECT 1
                FROM subscriptions sub
                WHERE sub.user_id=d.user_id
                  AND sub.object_type='group'
                  AND sub.object_id=e.group_id
            ) THEN 'subscription_removed'
            WHEN scheduler_in_quiet_hours(
                clock_timestamp(), un.timezone, usr.quiet_hours_enabled,
                usr.quiet_hours_start, usr.quiet_hours_end
            ) THEN 'quiet_hours'
            ELSE ''
        END AS policy_reason
    FROM notification_deliveries d
    JOIN schedule_change_events e ON e.id=d.event_id
    JOIN groups g ON g.id=e.group_id
    JOIN universities un ON un.id=g.university_id
    JOIN users usr ON usr.id=d.user_id

    UNION ALL

    SELECT
        'outbox'::TEXT AS queue_type,
        o.id,
        o.created_at,
        o.status,
        o.next_attempt_at,
        o.claim_token,
        o.lease_expires_at,
        CASE
            WHEN o.status<>'pending' THEN 'terminal'
            WHEN o.cancel_requested_at IS NOT NULL THEN 'cancel'
            WHEN o.kind IN ('support_request', 'admin_alert') AND NOT usr.is_admin THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND NOT usr.reminder_enabled THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND usr.default_group_id IS DISTINCT FROM o.group_id THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND scheduler_in_quiet_hours(
                clock_timestamp(), COALESCE(un.timezone, 'Europe/Moscow'), usr.quiet_hours_enabled,
                usr.quiet_hours_start, usr.quiet_hours_end
            ) THEN 'cancel'
            ELSE 'ready'
        END AS policy_decision,
        CASE
            WHEN o.cancel_requested_at IS NOT NULL THEN COALESCE(NULLIF(o.cancel_reason, ''), 'cancel_requested')
            WHEN o.kind IN ('support_request', 'admin_alert') AND NOT usr.is_admin THEN 'admin_role_removed'
            WHEN o.kind='lesson_reminder' AND NOT usr.reminder_enabled THEN 'reminder_disabled'
            WHEN o.kind='lesson_reminder' AND usr.default_group_id IS DISTINCT FROM o.group_id THEN 'default_group_changed'
            WHEN o.kind='lesson_reminder' AND scheduler_in_quiet_hours(
                clock_timestamp(), COALESCE(un.timezone, 'Europe/Moscow'), usr.quiet_hours_enabled,
                usr.quiet_hours_start, usr.quiet_hours_end
            ) THEN 'quiet_hours'
            ELSE ''
        END AS policy_reason
    FROM bot_outbox o
    JOIN users usr ON usr.id=o.user_id
    LEFT JOIN groups g ON g.id=o.group_id
    LEFT JOIN universities un ON un.id=g.university_id
)
SELECT
    queue_type,
    id,
    created_at,
    status,
    next_attempt_at,
    claim_token,
    lease_expires_at,
    policy_decision,
    policy_reason,
    status='pending'
        AND next_attempt_at<=clock_timestamp()
        AND (claim_token='' OR lease_expires_at<=clock_timestamp())
        AND policy_decision='ready' AS claimable
FROM queue_items;

CREATE OR REPLACE FUNCTION scheduler_request_notification_cancellation(
    target_user_id TEXT,
    target_group_id TEXT,
    reason TEXT
) RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE
    affected BIGINT;
BEGIN
    WITH changed AS (
        UPDATE notification_deliveries d
        SET status=CASE
                WHEN d.claim_token<>'' AND d.lease_expires_at>clock_timestamp() THEN 'pending'
                ELSE 'cancelled'
            END,
            cancel_requested_at=CASE
                WHEN d.claim_token<>'' AND d.lease_expires_at>clock_timestamp()
                    THEN COALESCE(d.cancel_requested_at, clock_timestamp())
                ELSE NULL
            END,
            cancel_reason=CASE
                WHEN d.claim_token<>'' AND d.lease_expires_at>clock_timestamp()
                    THEN LEFT(COALESCE(NULLIF(reason, ''), 'cancel_requested'), 500)
                ELSE ''
            END,
            claim_token=CASE
                WHEN d.claim_token<>'' AND d.lease_expires_at>clock_timestamp() THEN d.claim_token
                ELSE ''
            END,
            lease_expires_at=CASE
                WHEN d.claim_token<>'' AND d.lease_expires_at>clock_timestamp() THEN d.lease_expires_at
                ELSE NULL
            END,
            updated_at=clock_timestamp()
        FROM schedule_change_events e
        WHERE d.event_id=e.id
          AND d.user_id=target_user_id
          AND (target_group_id IS NULL OR e.group_id=target_group_id)
          AND d.status='pending'
        RETURNING 1
    )
    SELECT COUNT(*) INTO affected FROM changed;
    RETURN affected;
END
$$;

CREATE OR REPLACE FUNCTION scheduler_request_outbox_cancellation(
    target_user_id TEXT,
    target_kind TEXT,
    target_group_id TEXT,
    reason TEXT
) RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE
    affected BIGINT;
BEGIN
    WITH changed AS (
        UPDATE bot_outbox o
        SET status=CASE
                WHEN o.claim_token<>'' AND o.lease_expires_at>clock_timestamp() THEN 'pending'
                ELSE 'cancelled'
            END,
            cancel_requested_at=CASE
                WHEN o.claim_token<>'' AND o.lease_expires_at>clock_timestamp()
                    THEN COALESCE(o.cancel_requested_at, clock_timestamp())
                ELSE NULL
            END,
            cancel_reason=CASE
                WHEN o.claim_token<>'' AND o.lease_expires_at>clock_timestamp()
                    THEN LEFT(COALESCE(NULLIF(reason, ''), 'cancel_requested'), 500)
                ELSE ''
            END,
            claim_token=CASE
                WHEN o.claim_token<>'' AND o.lease_expires_at>clock_timestamp() THEN o.claim_token
                ELSE ''
            END,
            lease_expires_at=CASE
                WHEN o.claim_token<>'' AND o.lease_expires_at>clock_timestamp() THEN o.lease_expires_at
                ELSE NULL
            END,
            updated_at=clock_timestamp()
        WHERE o.user_id=target_user_id
          AND (target_kind IS NULL OR o.kind=target_kind)
          AND (target_group_id IS NULL OR o.group_id=target_group_id)
          AND o.status='pending'
        RETURNING 1
    )
    SELECT COUNT(*) INTO affected FROM changed;
    RETURN affected;
END
$$;

CREATE OR REPLACE FUNCTION scheduler_reconcile_notification_queue()
RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE
    affected BIGINT := 0;
    changed BIGINT;
BEGIN
    WITH reconciled AS (
        UPDATE notification_deliveries d
        SET status=CASE
                WHEN d.claim_token<>'' AND d.lease_expires_at>clock_timestamp() THEN 'pending'
                ELSE 'cancelled'
            END,
            cancel_requested_at=CASE
                WHEN d.claim_token<>'' AND d.lease_expires_at>clock_timestamp()
                    THEN COALESCE(d.cancel_requested_at, clock_timestamp())
                ELSE NULL
            END,
            cancel_reason=CASE
                WHEN d.claim_token<>'' AND d.lease_expires_at>clock_timestamp()
                    THEN LEFT(COALESCE(NULLIF(q.policy_reason, ''), 'policy_changed'), 500)
                ELSE ''
            END,
            claim_token=CASE
                WHEN d.claim_token<>'' AND d.lease_expires_at>clock_timestamp() THEN d.claim_token
                ELSE ''
            END,
            lease_expires_at=CASE
                WHEN d.claim_token<>'' AND d.lease_expires_at>clock_timestamp() THEN d.lease_expires_at
                ELSE NULL
            END,
            updated_at=clock_timestamp()
        FROM notification_queue_eligibility q
        WHERE q.queue_type='schedule'
          AND q.id=d.id
          AND q.policy_decision='cancel'
          AND d.status='pending'
          AND (d.cancel_requested_at IS NULL
            OR d.claim_token='' OR d.lease_expires_at<=clock_timestamp())
        RETURNING 1
    )
    SELECT COUNT(*) INTO changed FROM reconciled;
    affected := affected + changed;

    WITH reconciled AS (
        UPDATE bot_outbox o
        SET status=CASE
                WHEN o.claim_token<>'' AND o.lease_expires_at>clock_timestamp() THEN 'pending'
                ELSE 'cancelled'
            END,
            cancel_requested_at=CASE
                WHEN o.claim_token<>'' AND o.lease_expires_at>clock_timestamp()
                    THEN COALESCE(o.cancel_requested_at, clock_timestamp())
                ELSE NULL
            END,
            cancel_reason=CASE
                WHEN o.claim_token<>'' AND o.lease_expires_at>clock_timestamp()
                    THEN LEFT(COALESCE(NULLIF(q.policy_reason, ''), 'policy_changed'), 500)
                ELSE ''
            END,
            claim_token=CASE
                WHEN o.claim_token<>'' AND o.lease_expires_at>clock_timestamp() THEN o.claim_token
                ELSE ''
            END,
            lease_expires_at=CASE
                WHEN o.claim_token<>'' AND o.lease_expires_at>clock_timestamp() THEN o.lease_expires_at
                ELSE NULL
            END,
            updated_at=clock_timestamp()
        FROM notification_queue_eligibility q
        WHERE q.queue_type='outbox'
          AND q.id=o.id
          AND q.policy_decision='cancel'
          AND o.status='pending'
          AND (o.cancel_requested_at IS NULL
            OR o.claim_token='' OR o.lease_expires_at<=clock_timestamp())
        RETURNING 1
    )
    SELECT COUNT(*) INTO changed FROM reconciled;
    RETURN affected + changed;
END
$$;

REVOKE ALL ON FUNCTION scheduler_request_notification_cancellation(TEXT, TEXT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION scheduler_request_outbox_cancellation(TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION scheduler_reconcile_notification_queue() FROM PUBLIC;
