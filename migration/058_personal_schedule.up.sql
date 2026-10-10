CREATE TABLE personal_schedule_overrides (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('student','teacher')),
    target_id TEXT NOT NULL,
    lesson_id TEXT NOT NULL,
    university_id TEXT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    semester_id TEXT NOT NULL REFERENCES semesters(id) ON DELETE CASCADE,
    scope TEXT NOT NULL CHECK (scope IN ('day','semester')),
    valid_from DATE NOT NULL,
    valid_to DATE NOT NULL CHECK (valid_to>=valid_from),
    patch JSONB NOT NULL CHECK (jsonb_typeof(patch)='object'),
    cancelled BOOLEAN NOT NULL DEFAULT FALSE,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (scope<>'day' OR valid_to=valid_from),
    UNIQUE(user_id,role,target_id,lesson_id,scope,valid_from)
);
CREATE INDEX personal_schedule_owner ON personal_schedule_overrides(user_id,role,target_id,valid_to);
CREATE TABLE personal_sessions (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    csrf_token TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX personal_sessions_expiry ON personal_sessions(expires_at);
CREATE INDEX personal_sessions_owner ON personal_sessions(user_id,created_at);

CREATE FUNCTION scheduler_personal_schedule_changed() RETURNS TRIGGER
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
DECLARE owner_id TEXT;
BEGIN
    owner_id := CASE WHEN TG_OP='DELETE' THEN OLD.user_id ELSE NEW.user_id END;
    UPDATE users SET daily_next_at=CASE WHEN daily_enabled AND EXISTS (
        SELECT 1 FROM bot_outbox WHERE user_id=owner_id AND kind='daily_schedule'
        AND status='pending' AND delivered_parts=0
    ) THEN clock_timestamp() ELSE daily_next_at END WHERE id=owner_id;
    PERFORM scheduler_request_outbox_cancellation(owner_id,'daily_schedule',NULL,'personal_schedule_changed');
    PERFORM scheduler_request_outbox_cancellation(owner_id,'lesson_reminder',NULL,'personal_schedule_changed');
    PERFORM pg_notify('scheduler_outbox_ready','');
    RETURN NULL;
END;
$$;
DO $$ BEGIN
    EXECUTE format('ALTER FUNCTION scheduler_personal_schedule_changed() SET search_path TO pg_catalog, %I, pg_temp',current_schema());
END; $$;
REVOKE ALL ON FUNCTION scheduler_personal_schedule_changed() FROM PUBLIC;
CREATE TRIGGER personal_schedule_changed AFTER INSERT OR UPDATE OR DELETE ON personal_schedule_overrides
FOR EACH ROW EXECUTE FUNCTION scheduler_personal_schedule_changed();
