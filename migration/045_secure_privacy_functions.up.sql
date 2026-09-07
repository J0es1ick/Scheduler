-- Only new migrations may change installed function definitions.
CREATE FUNCTION scheduler_anonymize_audit_json(value JSONB, user_id TEXT, replacement TEXT)
RETURNS JSONB LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $$
DECLARE result JSONB; item RECORD; text_value TEXT;
BEGIN
    CASE jsonb_typeof(value)
    WHEN 'object' THEN
        result := '{}'::JSONB;
        FOR item IN SELECT * FROM jsonb_each(value) LOOP
            result := result || jsonb_build_object(item.key, scheduler_anonymize_audit_json(item.value, user_id, replacement));
        END LOOP;
        RETURN result;
    WHEN 'array' THEN
        SELECT COALESCE(jsonb_agg(scheduler_anonymize_audit_json(element, user_id, replacement)), '[]'::JSONB)
        INTO result FROM jsonb_array_elements(value) AS elements(element);
        RETURN result;
    WHEN 'number' THEN
        IF value::text=user_id THEN RETURN to_jsonb(replacement); END IF;
        RETURN value;
    WHEN 'string' THEN
        text_value := value #>> '{}';
        IF text_value=user_id THEN RETURN to_jsonb(replacement); END IF;
        IF text_value LIKE '/api/users/%' THEN
            RETURN to_jsonb(array_to_string(array_replace(string_to_array(text_value, '/'), user_id, replacement), '/'));
        END IF;
        RETURN value;
    ELSE RETURN value;
    END CASE;
END;
$$;
REVOKE ALL ON FUNCTION scheduler_anonymize_audit_json(JSONB, TEXT, TEXT) FROM PUBLIC;

CREATE OR REPLACE FUNCTION execute_privacy_deletion(deletion_user_id TEXT, anonymized_user_id TEXT)
RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $$
DECLARE
    profile_is_admin BOOLEAN;
    profile_admin_role TEXT;
BEGIN
    PERFORM pg_advisory_xact_lock_shared(hashtext('scheduler-group-references'));

    SELECT is_admin, admin_role
    INTO profile_is_admin, profile_admin_role
    FROM users
    WHERE id=deletion_user_id
    FOR UPDATE;

    IF NOT FOUND THEN
        RETURN FALSE;
    END IF;
    IF profile_is_admin OR profile_admin_role<>'none' THEN
        RAISE EXCEPTION 'remove administrator role before deleting the profile';
    END IF;

    DELETE FROM admin_sessions WHERE admin_id=deletion_user_id;
    UPDATE admin_audit_logs
    SET actor_id=CASE WHEN actor_id=deletion_user_id THEN anonymized_user_id ELSE actor_id END,
        actor_name=CASE WHEN actor_id=deletion_user_id THEN 'Удалённый пользователь' ELSE actor_name END,
        ip_address=CASE WHEN actor_id=deletion_user_id THEN '' ELSE ip_address END,
        object_id=CASE WHEN object_type='user' AND object_id=deletion_user_id THEN anonymized_user_id ELSE object_id END,
        details=scheduler_anonymize_audit_json(details, deletion_user_id, anonymized_user_id)
    WHERE actor_id=deletion_user_id
       OR (object_type='user' AND object_id=deletion_user_id)
       OR details::text LIKE '%' || deletion_user_id || '%';
    UPDATE chat_schedule_profiles SET configured_by=anonymized_user_id WHERE configured_by=deletion_user_id;
    UPDATE lesson_overrides SET created_by=anonymized_user_id WHERE created_by=deletion_user_id;
    UPDATE support_requests SET reviewed_by=anonymized_user_id WHERE reviewed_by=deletion_user_id;
    UPDATE parser_snapshots SET reviewed_by=anonymized_user_id WHERE reviewed_by=deletion_user_id;
    UPDATE connector_clients SET created_by=anonymized_user_id WHERE created_by=deletion_user_id;
    DELETE FROM users WHERE id=deletion_user_id;
    RETURN TRUE;
END;
$$;

REVOKE ALL ON FUNCTION execute_privacy_deletion(TEXT, TEXT) FROM PUBLIC;

-- Resolve the installation schema before changing any function configuration.
DO $$
DECLARE installation_schema TEXT := current_schema(); function_record RECORD;
BEGIN
    FOR function_record IN
        SELECT p.oid FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
        WHERE n.nspname=installation_schema AND
            (p.prosecdef OR p.proname='scheduler_anonymize_audit_json')
    LOOP
        EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',
            function_record.oid::regprocedure, installation_schema);
    END LOOP;
END;
$$;
