-- Timestamp ties do not establish a historical server order. UUID order is only
-- a deterministic backfill; freeze all pre-upgrade runs for such a source.
UPDATE data_sources ds SET last_published_ingestion_sequence=next_ingestion_sequence
WHERE EXISTS (
 SELECT 1 FROM connector_clients c JOIN connector_ingestion_runs r ON r.connector_id=c.id
 WHERE c.data_source_id=ds.id
 AND r.received_at <= (SELECT applied_at FROM schema_migrations WHERE name='046_ingestion_order.up.sql')
 GROUP BY r.connector_id,r.received_at HAVING COUNT(*)>1
);

