-- One clock/rule for the bot, Mini App and public source status.
CREATE FUNCTION scheduler_source_freshness_state(checked_at TIMESTAMPTZ, enabled BOOLEAN, last_error TEXT, published_at TIMESTAMPTZ, interval_seconds INTEGER)
RETURNS TEXT LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$ SELECT CASE WHEN NOT enabled THEN 'disabled' WHEN COALESCE(last_error,'')<>'' THEN 'error'
 WHEN published_at IS NULL OR checked_at-published_at > (GREATEST(interval_seconds,0)::bigint*2+300)*INTERVAL '1 second' THEN 'stale' ELSE 'current' END $$;

CREATE OR REPLACE VIEW public_site_sources WITH (security_barrier=true,security_invoker=false) AS
SELECT university.name AS university_name,COALESCE(university.schedule_url,'') AS schedule_url,
 (COALESCE(university.schedule_url,'') LIKE 'https://%') AS secure,
 snapshot.published_at AS last_success_at,
 scheduler_source_freshness_state(NOW(),source.is_enabled,source.last_error,snapshot.published_at,source.update_interval) AS state
FROM data_sources source JOIN universities university ON university.id=source.university_id
LEFT JOIN parser_snapshots snapshot ON snapshot.id=source.current_snapshot_id
WHERE university.is_active AND source.lifecycle_status='active';
