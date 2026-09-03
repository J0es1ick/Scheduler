INSERT INTO operational_maintenance (task_name)
VALUES ('parser_retention'), ('admin_retention')
ON CONFLICT (task_name) DO NOTHING;

CREATE OR REPLACE FUNCTION enqueue_schedule_change(
    event_id TEXT,
    changed_group_id TEXT,
    change_source TEXT,
    change_summary TEXT
) RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
BEGIN
    INSERT INTO schedule_change_events (id, group_id, source, summary)
    VALUES (event_id, changed_group_id, change_source, change_summary);

    INSERT INTO notification_deliveries (id, event_id, user_id)
    SELECT event_id || ':' || subscription.user_id, event_id, subscription.user_id
    FROM subscriptions subscription
    JOIN users app_user ON app_user.id = subscription.user_id
    WHERE subscription.object_type = 'group'
      AND subscription.object_id = changed_group_id
      AND app_user.notifications_enabled
    ON CONFLICT ON CONSTRAINT notification_deliveries_event_id_user_id_key DO NOTHING;
END;
$$;

CREATE OR REPLACE FUNCTION enqueue_admin_alert(alert_id TEXT, alert_body TEXT)
RETURNS VOID
LANGUAGE sql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
    INSERT INTO bot_outbox (id, user_id, kind, body)
    SELECT alert_id || ':' || app_user.id, app_user.id, 'admin_alert', alert_body
    FROM users app_user
    WHERE app_user.is_admin
    ON CONFLICT (id) DO NOTHING;
$$;

REVOKE ALL ON FUNCTION enqueue_schedule_change(TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION enqueue_admin_alert(TEXT, TEXT) FROM PUBLIC;
