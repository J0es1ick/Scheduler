UPDATE parser_snapshots snapshot
SET status = 'approved',
    published_at = NULL
FROM data_sources source
WHERE source.current_snapshot_id = snapshot.id
  AND source.lifecycle_status <> 'active'
  AND snapshot.status = 'published';

UPDATE connector_ingestion_runs run
SET status = 'staged'
FROM parser_snapshots snapshot, data_sources source
WHERE run.parser_snapshot_id = snapshot.id
  AND snapshot.data_source_id = source.id
  AND snapshot.status = 'approved'
  AND source.lifecycle_status <> 'active'
  AND run.status = 'published';

UPDATE data_sources
SET current_snapshot_id = NULL,
    updated_at = NOW()
WHERE lifecycle_status <> 'active'
  AND current_snapshot_id IS NOT NULL;
