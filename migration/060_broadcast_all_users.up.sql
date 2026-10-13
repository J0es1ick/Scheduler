ALTER TABLE broadcasts DROP CONSTRAINT broadcasts_audience_mode_check;
ALTER TABLE broadcasts ADD CONSTRAINT broadcasts_audience_mode_check
    CHECK (audience_mode IN ('all','selected','all_users'));

CREATE OR REPLACE FUNCTION scheduler_service_updates_changed() RETURNS TRIGGER
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
BEGIN
    IF NEW.service_updates_consent IS DISTINCT FROM OLD.service_updates_consent THEN
        IF NEW.service_updates_consent IS NOT NULL THEN
            PERFORM scheduler_request_outbox_cancellation(NEW.id,'service_updates_prompt',NULL,'consent_answered');
        END IF;
        IF NEW.service_updates_consent IS NOT TRUE THEN
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
                        THEN 'consent_revoked'
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
            WHERE o.user_id=NEW.id
              AND o.kind='service_update'
              AND EXISTS (SELECT 1 FROM broadcasts b WHERE b.id=o.broadcast_id AND b.audience_mode<>'all_users')
              AND o.status='pending';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
DO $$
BEGIN
    EXECUTE format('ALTER FUNCTION scheduler_service_updates_changed() SET search_path TO pg_catalog, %I, pg_temp',current_schema());
END;
$$;
REVOKE ALL ON FUNCTION scheduler_service_updates_changed() FROM PUBLIC;

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
            WHEN usr.role<>'student' THEN 'cancel'
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
            WHEN usr.role<>'student' THEN 'role_changed'
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
            WHEN o.kind='service_update' AND ((usr.service_updates_consent IS NOT TRUE AND NOT EXISTS (SELECT 1 FROM broadcasts b WHERE b.id=o.broadcast_id AND b.audience_mode='all_users')) OR NOT EXISTS (SELECT 1 FROM broadcasts b WHERE b.id=o.broadcast_id AND b.status='sending')) THEN 'cancel'
            WHEN o.kind='service_updates_prompt' AND (usr.service_updates_consent IS NOT NULL OR usr.service_updates_prompt_delivered_at IS NOT NULL) THEN 'cancel'
            WHEN o.kind IN ('daily_schedule','teacher_change','lesson_reminder') AND
                ((o.teacher_id IS NOT NULL AND (usr.role<>'teacher' OR usr.teacher_id IS DISTINCT FROM o.teacher_id)) OR
                 (o.teacher_id IS NULL AND usr.role<>'student')) THEN 'cancel'
            WHEN o.kind='teacher_change' AND NOT usr.notifications_enabled THEN 'cancel'
            WHEN o.kind='daily_schedule' AND (NOT usr.daily_enabled OR o.expires_at<=clock_timestamp() OR o.expires_at IS NULL OR
                 (o.teacher_id IS NULL AND usr.default_group_id IS DISTINCT FROM o.group_id)) THEN 'cancel'
            WHEN o.kind='teacher_change' AND scheduler_in_quiet_hours(clock_timestamp(),un.timezone,usr.quiet_hours_enabled,usr.quiet_hours_start,usr.quiet_hours_end) THEN 'defer'
            WHEN o.kind IN ('support_request', 'admin_alert') AND NOT usr.is_admin THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND (o.expires_at IS NULL OR o.expires_at<=clock_timestamp() OR o.reminder_context='{}'::jsonb) THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND NOT usr.reminder_enabled THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND o.teacher_id IS NULL AND usr.default_group_id IS DISTINCT FROM o.group_id THEN 'cancel'
            WHEN o.kind='lesson_reminder' AND o.teacher_id IS NULL AND COALESCE(g.is_active, FALSE)=FALSE THEN 'cancel'
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
            WHEN o.kind='service_update' AND (usr.service_updates_consent IS NOT TRUE AND NOT EXISTS (SELECT 1 FROM broadcasts b WHERE b.id=o.broadcast_id AND b.audience_mode='all_users')) THEN 'consent_revoked'
            WHEN o.kind='service_update' AND NOT EXISTS (SELECT 1 FROM broadcasts b WHERE b.id=o.broadcast_id AND b.status='sending') THEN 'broadcast_stopped'
            WHEN o.kind='service_updates_prompt' AND (usr.service_updates_consent IS NOT NULL OR usr.service_updates_prompt_delivered_at IS NOT NULL) THEN 'prompt_completed'
            WHEN o.kind IN ('daily_schedule','teacher_change','lesson_reminder') AND
                ((o.teacher_id IS NOT NULL AND (usr.role<>'teacher' OR usr.teacher_id IS DISTINCT FROM o.teacher_id)) OR
                 (o.teacher_id IS NULL AND usr.role<>'student')) THEN 'profile_changed'
            WHEN o.kind='teacher_change' AND NOT usr.notifications_enabled THEN 'notifications_disabled'
            WHEN o.kind='daily_schedule' AND NOT usr.daily_enabled THEN 'daily_disabled'
            WHEN o.kind='daily_schedule' AND (o.expires_at IS NULL OR o.expires_at<=clock_timestamp()) THEN 'daily_expired'
            WHEN o.kind='daily_schedule' AND o.teacher_id IS NULL AND usr.default_group_id IS DISTINCT FROM o.group_id THEN 'profile_changed'
            WHEN o.kind='teacher_change' AND scheduler_in_quiet_hours(clock_timestamp(),un.timezone,usr.quiet_hours_enabled,usr.quiet_hours_start,usr.quiet_hours_end) THEN 'quiet_hours'
            WHEN o.kind IN ('support_request', 'admin_alert') AND NOT usr.is_admin THEN 'admin_role_removed'
            WHEN o.kind='lesson_reminder' AND (o.expires_at IS NULL OR o.expires_at<=clock_timestamp() OR o.reminder_context='{}'::jsonb) THEN 'reminder_expired'
            WHEN o.kind='lesson_reminder' AND NOT usr.reminder_enabled THEN 'reminder_disabled'
            WHEN o.kind='lesson_reminder' AND o.teacher_id IS NULL AND usr.default_group_id IS DISTINCT FROM o.group_id THEN 'default_group_changed'
            WHEN o.kind='lesson_reminder' AND o.teacher_id IS NULL AND COALESCE(g.is_active, FALSE)=FALSE THEN 'reminder_group_inactive'
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
    LEFT JOIN teachers t ON t.id=o.teacher_id
    LEFT JOIN universities un ON un.id=COALESCE(t.university_id,g.university_id)
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
