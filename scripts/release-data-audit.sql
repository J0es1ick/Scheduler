


BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET LOCAL statement_timeout = '60s';

SELECT 'recurrence_missing' AS finding, o.id AS override_id, o.group_id,
       o.recurrence AS current_rule, l.recurrence AS proposed_rule,
       (ROW(o.semester_id,o.day_of_week,o.special_date,o.week_type,o.valid_from,o.valid_to)
          IS NOT DISTINCT FROM ROW(l.semester_id,l.day_of_week,l.special_date,l.week_type,l.valid_from,l.valid_to)) AS unambiguous,
       o.valid_from AS override_first_date, l.valid_from AS source_first_date
FROM lesson_overrides o JOIN lessons l ON l.id=o.base_lesson_id
WHERE o.recurrence='{}'::jsonb AND l.recurrence<>'{}'::jsonb;

SELECT 'institution_metadata_mismatch' AS finding, u.id AS university_id, s.id AS live_source,
       u.name AS current_name, p.payload->'metadata'->'institution' AS published_settings
FROM universities u JOIN data_sources s ON s.university_id=u.id AND s.lifecycle_status='active'
JOIN parser_snapshots p ON p.id=s.current_snapshot_id
WHERE p.payload->'metadata'->'institution' IS NOT NULL
AND (u.name IS DISTINCT FROM p.payload->'metadata'->'institution'->>'name'
 OR u.timezone IS DISTINCT FROM p.payload->'metadata'->'institution'->>'timezone'
 OR u.schedule_url IS DISTINCT FROM p.payload->'metadata'->'institution'->>'schedule_url'
 OR u.locale IS DISTINCT FROM p.payload->'metadata'->'institution'->>'locale');

SELECT 'ingestion_needs_review' AS finding, c.data_source_id, r.id AS run_id,
       r.status, r.received_at, r.completed_at, r.attempts,
       s.current_snapshot_id
FROM connector_ingestion_runs r JOIN connector_clients c ON c.id=r.connector_id
JOIN data_sources s ON s.id=c.data_source_id
WHERE r.status IN ('received','processing')
AND (r.received_at<NOW()-INTERVAL '1 day' OR EXISTS (
 SELECT 1 FROM connector_ingestion_runs newer WHERE newer.connector_id=r.connector_id
 AND newer.received_at>r.received_at AND newer.status='published'));

SELECT 'ambiguous_ingestion_timestamps' AS finding, c.data_source_id, r.received_at, COUNT(*) AS tied_runs
FROM connector_ingestion_runs r JOIN connector_clients c ON c.id=r.connector_id
GROUP BY c.data_source_id,r.received_at HAVING COUNT(*)>1;

SELECT 'unrecoverable_publication_order'  AS finding, s.id AS source_id
FROM data_sources s WHERE s.adapter_type='external_push' AND s.current_snapshot_id IS NOT NULL
AND NOT EXISTS (SELECT 1 FROM connector_clients c JOIN connector_ingestion_runs r ON r.connector_id=c.id
 WHERE c.data_source_id=s.id AND r.parser_snapshot_id=s.current_snapshot_id);

SELECT 'audit_orphaned_user_targets' AS finding, COUNT(*) AS affected_records
FROM admin_audit_logs a WHERE a.object_type='user' AND a.object_id ~ '^[0-9]+$'
AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id=a.object_id);
SELECT 'audit_historical_identifier_paths' AS finding, COUNT(*) AS affected_records
FROM admin_audit_logs WHERE details->>'path' ~ '^/api/users/[0-9]+(/|$)';
COMMIT;
