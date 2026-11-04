ALTER TABLE schedule_change_events ADD COLUMN change_context JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(change_context)='object');

CREATE FUNCTION enqueue_schedule_change(event_id TEXT,group_id TEXT,source TEXT,summary TEXT,details JSONB) RETURNS VOID
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
BEGIN
    PERFORM enqueue_schedule_change(event_id,group_id,source,summary);
    UPDATE schedule_change_events e SET change_context=details WHERE e.id=event_id;
END;
$$;

CREATE FUNCTION enqueue_teacher_change(event_id TEXT,university TEXT,teacher_key TEXT,summary TEXT,details JSONB) RETURNS VOID
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
DECLARE recipient RECORD; pending TEXT;
BEGIN
    FOR recipient IN SELECT u.id,t.id AS teacher_id,t.name FROM teachers t JOIN users u ON u.teacher_id=t.id
        WHERE t.university_id=university AND t.name_key=teacher_key AND u.role='teacher' AND u.notifications_enabled AND NOT u.bot_blocked
        ORDER BY u.id FOR UPDATE OF u LOOP
        SELECT o.id INTO pending FROM bot_outbox o WHERE o.user_id=recipient.id AND o.teacher_id=recipient.teacher_id
            AND o.kind='teacher_change' AND o.status='pending' AND o.attempts=0 AND o.claim_token='' AND o.cancel_requested_at IS NULL
            AND o.schedule_context ? 'before' ORDER BY o.created_at,o.id LIMIT 1 FOR UPDATE;
        IF pending IS NOT NULL THEN
            UPDATE bot_outbox SET body=recipient.name||E'\n'||summary,
                schedule_context=details||jsonb_build_object('before',schedule_context->'before','before_semesters',COALESCE(schedule_context->'before_semesters',schedule_context->'semesters')),updated_at=NOW() WHERE id=pending;
        ELSE
            INSERT INTO bot_outbox(id,user_id,teacher_id,kind,body,schedule_context)
            VALUES(event_id||':'||recipient.id,recipient.id,recipient.teacher_id,'teacher_change',recipient.name||E'\n'||summary,details)
            ON CONFLICT(id) DO NOTHING;
        END IF;
    END LOOP;
END;
$$;
DO $$ BEGIN
    EXECUTE format('ALTER FUNCTION enqueue_schedule_change(TEXT,TEXT,TEXT,TEXT,JSONB) SET search_path TO pg_catalog, %I, pg_temp',current_schema());
    EXECUTE format('ALTER FUNCTION enqueue_teacher_change(TEXT,TEXT,TEXT,TEXT,JSONB) SET search_path TO pg_catalog, %I, pg_temp',current_schema());
END; $$;
REVOKE ALL ON FUNCTION enqueue_schedule_change(TEXT,TEXT,TEXT,TEXT,JSONB),enqueue_teacher_change(TEXT,TEXT,TEXT,TEXT,JSONB) FROM PUBLIC;
