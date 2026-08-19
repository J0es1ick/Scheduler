CREATE VIEW public_site_statistics
WITH (security_barrier = true, security_invoker = false)
AS
SELECT
    (SELECT COUNT(*) FROM universities WHERE is_active) AS universities,
    (SELECT COUNT(*) FROM groups WHERE is_active) AS groups,
    (SELECT COUNT(*)
     FROM effective_lessons lesson
     JOIN groups study_group ON study_group.id=lesson.group_id
     WHERE study_group.is_active) AS lessons,
    (SELECT COUNT(*) FROM users) AS users,
    (SELECT COUNT(*) FROM subscriptions) AS subscriptions;

CREATE VIEW public_site_universities
WITH (security_barrier = true, security_invoker = false)
AS
SELECT name
FROM universities
WHERE is_active;

CREATE VIEW public_site_sources
WITH (security_barrier = true, security_invoker = false)
AS
SELECT
    university.name AS university_name,
    COALESCE(university.schedule_url, '') AS schedule_url,
    (COALESCE(university.schedule_url, '') LIKE 'https://%') AS secure,
    source.last_success_at,
    CASE
        WHEN NOT source.is_enabled THEN 'disabled'
        WHEN COALESCE(source.last_error, '')<>'' THEN 'error'
        WHEN source.last_success_at IS NULL OR
            NOW()-source.last_success_at > (source.update_interval*2+300)*INTERVAL '1 second' THEN 'stale'
        ELSE 'current'
    END AS state
FROM data_sources source
JOIN universities university ON university.id=source.university_id
WHERE university.is_active AND source.lifecycle_status='active';

REVOKE ALL ON public_site_statistics, public_site_universities, public_site_sources FROM PUBLIC;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='scheduler_public_reader') THEN
        EXECUTE 'GRANT SELECT ON public_site_statistics, public_site_universities, public_site_sources TO scheduler_public_reader';
    END IF;
END
$$;
