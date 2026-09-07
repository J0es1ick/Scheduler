-- Applies on upgrades even when no fresh bootstrap is needed. The migrator owns
-- this database; restore TEMP is granted explicitly by postgres-bootstrap.
DO $$ BEGIN
 EXECUTE format('REVOKE TEMPORARY ON DATABASE %I FROM PUBLIC',current_database());
 EXECUTE format('GRANT TEMPORARY ON DATABASE %I TO %I',current_database(),current_user);
END $$;
-- Pure invoker function with only scalar inputs; safe for public source readers.
GRANT EXECUTE ON FUNCTION scheduler_source_freshness_state(TIMESTAMPTZ,BOOLEAN,TEXT,TIMESTAMPTZ,INTEGER) TO PUBLIC;
