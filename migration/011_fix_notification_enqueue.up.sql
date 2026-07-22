CREATE OR REPLACE FUNCTION enqueue_schedule_change(
    event_id TEXT,
    changed_group_id TEXT,
    change_source TEXT,
    change_summary TEXT
) RETURNS VOID AS $$
BEGIN
    INSERT INTO schedule_change_events (id, group_id, source, summary)
    VALUES (event_id, changed_group_id, change_source, change_summary);

    INSERT INTO notification_deliveries (id, event_id, user_id)
    SELECT event_id || ':' || s.user_id, event_id, s.user_id
    FROM subscriptions s
    JOIN users u ON u.id = s.user_id
    WHERE s.object_type = 'group'
      AND s.object_id = changed_group_id
      AND u.notifications_enabled
    ON CONFLICT ON CONSTRAINT notification_deliveries_event_id_user_id_key DO NOTHING;
END;
$$ LANGUAGE plpgsql;
