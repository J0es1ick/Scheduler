CREATE TABLE teachers (
    id TEXT PRIMARY KEY,
    university_id TEXT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    name_key TEXT NOT NULL CHECK (name_key<>''),
    UNIQUE(university_id, name_key)
);

ALTER TABLE users
    ADD COLUMN role TEXT NOT NULL DEFAULT 'student' CHECK (role IN ('student', 'teacher')),
    ADD COLUMN teacher_id TEXT REFERENCES teachers(id) ON DELETE SET NULL,
    ADD COLUMN teacher_schedule_view_format TEXT NOT NULL DEFAULT 'visual' CHECK (teacher_schedule_view_format IN ('compact','visual')),
    ADD COLUMN daily_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN daily_time TIME NOT NULL DEFAULT '06:00',
    ADD COLUMN daily_setup TEXT NOT NULL DEFAULT 'choice' CHECK (daily_setup IN ('choice','time','done')),
    ADD COLUMN daily_next_at TIMESTAMPTZ;

UPDATE users SET role='student', daily_setup=CASE WHEN default_group_id IS NULL THEN 'choice' ELSE 'done' END;
CREATE INDEX users_teacher_idx ON users(teacher_id) WHERE teacher_id IS NOT NULL;
CREATE INDEX users_daily_due_idx ON users(daily_next_at) WHERE daily_enabled;

ALTER TABLE bot_outbox
    ADD COLUMN teacher_id TEXT REFERENCES teachers(id) ON DELETE CASCADE,
    ADD COLUMN schedule_context JSONB NOT NULL DEFAULT '{}'::JSONB,
    ADD COLUMN delivered_parts INTEGER NOT NULL DEFAULT 0 CHECK (delivered_parts>=0),
    ADD COLUMN schedule_messages JSONB NOT NULL DEFAULT '[]'::JSONB;

ALTER TABLE bot_outbox DROP CONSTRAINT bot_outbox_kind_check;
ALTER TABLE bot_outbox ADD CONSTRAINT bot_outbox_kind_check
    CHECK (kind IN ('support_request','support_resolution','admin_alert','lesson_reminder','teacher_change','daily_schedule'));

CREATE FUNCTION scheduler_profile_timezone(app_user users) RETURNS TEXT
LANGUAGE SQL STABLE SET search_path FROM CURRENT AS $$
    SELECT COALESCE((SELECT un.timezone FROM universities un WHERE un.id=CASE
        WHEN app_user.role='teacher' THEN (SELECT university_id FROM teachers WHERE id=app_user.teacher_id)
        ELSE (SELECT university_id FROM groups WHERE id=app_user.default_group_id) END),'Europe/Moscow')
$$;

CREATE FUNCTION scheduler_next_daily(local_time TIME, zone TEXT, moment TIMESTAMPTZ) RETURNS TIMESTAMPTZ
LANGUAGE SQL STABLE AS $$
    SELECT CASE WHEN ((moment AT TIME ZONE zone)::date+local_time) AT TIME ZONE zone > moment
        THEN ((moment AT TIME ZONE zone)::date+local_time) AT TIME ZONE zone
        ELSE (((moment AT TIME ZONE zone)::date+1)+local_time) AT TIME ZONE zone END
$$;

CREATE FUNCTION scheduler_profile_schedule_changed() RETURNS TRIGGER
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
BEGIN
    IF NEW.role IS DISTINCT FROM OLD.role OR
       (NEW.role='student' AND NEW.default_group_id IS DISTINCT FROM OLD.default_group_id) OR
       (NEW.role='teacher' AND NEW.teacher_id IS DISTINCT FROM OLD.teacher_id) OR
       NEW.daily_enabled IS DISTINCT FROM OLD.daily_enabled OR NEW.daily_time IS DISTINCT FROM OLD.daily_time THEN
        NEW.daily_next_at := CASE WHEN NEW.daily_enabled THEN scheduler_next_daily(NEW.daily_time,scheduler_profile_timezone(NEW),clock_timestamp()) ELSE NULL END;
        PERFORM scheduler_request_outbox_cancellation(NEW.id,'daily_schedule',NULL,'profile_changed');
    END IF;
    IF NEW.role IS DISTINCT FROM OLD.role OR
       (NEW.role='student' AND NEW.default_group_id IS DISTINCT FROM OLD.default_group_id) OR
       (NEW.role='teacher' AND NEW.teacher_id IS DISTINCT FROM OLD.teacher_id) THEN
        IF OLD.default_group_id IS NOT NULL OR OLD.teacher_id IS NOT NULL THEN
            PERFORM scheduler_request_outbox_cancellation(NEW.id,'lesson_reminder',NULL,'profile_changed');
            PERFORM scheduler_request_outbox_cancellation(NEW.id,'teacher_change',NULL,'profile_changed');
        END IF;
        IF NEW.role<>'student' THEN
            PERFORM scheduler_request_notification_cancellation(NEW.id,NULL,'role_changed');
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER users_schedule_profile_changed BEFORE UPDATE OF role,default_group_id,teacher_id,daily_enabled,daily_time
ON users FOR EACH ROW EXECUTE FUNCTION scheduler_profile_schedule_changed();

CREATE FUNCTION enqueue_teacher_change(event_id TEXT, university TEXT, teacher_key TEXT, summary TEXT) RETURNS VOID
LANGUAGE SQL SECURITY DEFINER SET search_path FROM CURRENT AS $$
    INSERT INTO bot_outbox(id,user_id,teacher_id,kind,body)
    SELECT event_id||':'||u.id,u.id,t.id,'teacher_change',t.name||E'\n'||summary
    FROM teachers t JOIN users u ON u.teacher_id=t.id
    WHERE t.university_id=university AND t.name_key=teacher_key AND u.role='teacher' AND u.notifications_enabled AND NOT u.bot_blocked
    ON CONFLICT(id) DO NOTHING
$$;

REVOKE ALL ON FUNCTION enqueue_teacher_change(TEXT,TEXT,TEXT,TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION scheduler_profile_schedule_changed() FROM PUBLIC;

CREATE VIEW schedule_profile_recipients AS
SELECT u.id AS user_id,u.role,u.default_group_id,u.teacher_id,
    CASE WHEN u.role='teacher' THEN t.university_id ELSE g.university_id END AS university_id,
    CASE WHEN u.role='teacher' THEN t.name ELSE g.name END AS name,
    un.name AS university_name,un.timezone,
    CASE WHEN u.role='teacher' THEN un.is_active ELSE COALESCE(g.is_active,FALSE) AND un.is_active END AS is_active,
    CASE WHEN u.role='teacher' THEN u.teacher_schedule_view_format ELSE COALESCE(s.schedule_view_format,'visual') END AS view_format,
    CASE WHEN u.role='teacher' THEN 0 ELSE COALESCE(s.subgroup,0) END AS subgroup,
    u.daily_enabled,u.daily_time,u.daily_next_at,u.reminder_enabled,u.reminder_minutes,u.bot_blocked
FROM users u LEFT JOIN teachers t ON t.id=u.teacher_id
LEFT JOIN groups g ON g.id=u.default_group_id
LEFT JOIN subscriptions s ON s.user_id=u.id AND s.object_type='group' AND s.object_id=u.default_group_id
JOIN universities un ON un.id=CASE WHEN u.role='teacher' THEN t.university_id ELSE g.university_id END
WHERE u.role IN ('student','teacher');

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

DO $$
BEGIN
    EXECUTE format('ALTER FUNCTION scheduler_profile_schedule_changed() SET search_path TO pg_catalog, %I, pg_temp', current_schema());
    EXECUTE format('ALTER FUNCTION enqueue_teacher_change(TEXT,TEXT,TEXT,TEXT) SET search_path TO pg_catalog, %I, pg_temp', current_schema());
END;
$$;

ALTER TABLE bot_outbox ADD CONSTRAINT bot_outbox_schedule_target_check
    CHECK (kind NOT IN ('daily_schedule','teacher_change') OR
           (group_id IS NOT NULL AND teacher_id IS NULL) OR
           (group_id IS NULL AND teacher_id IS NOT NULL));
ALTER TABLE bot_outbox ADD CONSTRAINT bot_outbox_schedule_progress_check
    CHECK (jsonb_typeof(schedule_messages)='array' AND delivered_parts<=jsonb_array_length(schedule_messages));

CREATE OR REPLACE VIEW subscription_integrity AS
SELECT
    (SELECT COUNT(*) FROM users u WHERE u.default_group_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM subscriptions s WHERE s.user_id=u.id AND s.group_id=u.default_group_id
    )) AS missing_default_subscriptions,
    (SELECT COUNT(*) FROM subscriptions s WHERE s.object_type='group' AND NOT EXISTS (
        SELECT 1 FROM groups g WHERE g.id=s.object_id
    )) AS orphan_group_subscriptions,
    (SELECT COUNT(*) FROM subscriptions WHERE object_type='teacher') AS teacher_subscriptions;
