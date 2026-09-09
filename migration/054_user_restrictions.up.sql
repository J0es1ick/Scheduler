ALTER TABLE users ADD COLUMN bot_blocked BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN support_blocked BOOLEAN NOT NULL DEFAULT FALSE;

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
            WHEN usr.bot_blocked THEN 'cancel'
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
            WHEN usr.bot_blocked THEN 'user_blocked'
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
            WHEN usr.bot_blocked THEN 'cancel'
            WHEN o.cancel_requested_at IS NOT NULL THEN 'cancel'
            WHEN o.kind IN ('support_request', 'admin_alert') AND NOT usr.is_admin THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND (o.expires_at IS NULL OR o.expires_at<=clock_timestamp() OR o.reminder_context='{}'::jsonb) THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND NOT usr.reminder_enabled THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND usr.default_group_id IS DISTINCT FROM o.group_id THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND COALESCE(g.is_active, FALSE)=FALSE THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND COALESCE(un.is_active, FALSE)=FALSE THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND scheduler_in_quiet_hours(
                clock_timestamp(), COALESCE(un.timezone, 'Europe/Moscow'), usr.quiet_hours_enabled,
                usr.quiet_hours_start, usr.quiet_hours_end
            ) THEN 'cancel'
            ELSE 'ready'
        END AS policy_decision,
        CASE
            WHEN usr.bot_blocked THEN 'user_blocked'
            WHEN o.cancel_requested_at IS NOT NULL THEN COALESCE(NULLIF(o.cancel_reason, ''), 'cancel_requested')
            WHEN o.kind IN ('support_request', 'admin_alert') AND NOT usr.is_admin THEN 'admin_role_removed'
            WHEN o.kind='lesson_reminder' AND (o.expires_at IS NULL OR o.expires_at<=clock_timestamp() OR o.reminder_context='{}'::jsonb) THEN 'reminder_expired'
            WHEN o.kind='lesson_reminder' AND NOT usr.reminder_enabled THEN 'reminder_disabled'
            WHEN o.kind='lesson_reminder' AND usr.default_group_id IS DISTINCT FROM o.group_id THEN 'default_group_changed'
            WHEN o.kind='lesson_reminder' AND COALESCE(g.is_active, FALSE)=FALSE THEN 'reminder_group_inactive'
            WHEN o.kind='lesson_reminder' AND COALESCE(un.is_active, FALSE)=FALSE THEN 'reminder_university_inactive'
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
