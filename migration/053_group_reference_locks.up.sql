CREATE FUNCTION scheduler_lock_active_group(candidate_group_id TEXT)
RETURNS SETOF TEXT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
DECLARE
    candidate_university_id TEXT;
BEGIN
    SELECT university_id INTO candidate_university_id
    FROM groups WHERE id=candidate_group_id;
    IF NOT FOUND THEN
        RETURN;
    END IF;

    PERFORM id FROM universities
    WHERE id=candidate_university_id AND is_active
    FOR SHARE;
    IF NOT FOUND THEN
        RETURN;
    END IF;

    RETURN QUERY
    SELECT id FROM groups
    WHERE id=candidate_group_id AND university_id=candidate_university_id AND is_active
    FOR SHARE;
END;
$$;

CREATE FUNCTION scheduler_select_replacement_group(candidate_user_id TEXT)
RETURNS SETOF TEXT
LANGUAGE sql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
    SELECT s.object_id
    FROM subscriptions s
    JOIN groups g ON g.id=s.object_id AND g.is_active
    JOIN universities u ON u.id=g.university_id AND u.is_active
    WHERE s.user_id=candidate_user_id AND s.object_type='group'
    ORDER BY s.updated_at DESC, s.created_at DESC, s.id
    LIMIT 1 FOR SHARE OF g, u SKIP LOCKED;
$$;

REVOKE ALL ON FUNCTION scheduler_lock_active_group(TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION scheduler_select_replacement_group(TEXT) FROM PUBLIC;

DO $$
DECLARE installation_schema TEXT := current_schema();
BEGIN
    EXECUTE format('ALTER FUNCTION %I.scheduler_lock_active_group(TEXT) SET search_path TO pg_catalog, %I, pg_temp',
        installation_schema, installation_schema);
    EXECUTE format('ALTER FUNCTION %I.scheduler_select_replacement_group(TEXT) SET search_path TO pg_catalog, %I, pg_temp',
        installation_schema, installation_schema);
END;
$$;
