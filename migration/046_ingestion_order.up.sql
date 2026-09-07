ALTER TABLE data_sources
    ADD COLUMN next_ingestion_sequence BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN last_published_ingestion_sequence BIGINT NOT NULL DEFAULT 0;
ALTER TABLE connector_ingestion_runs ADD COLUMN ingestion_sequence BIGINT NOT NULL DEFAULT 0;

WITH ordered AS (
    SELECT id, row_number() OVER (PARTITION BY connector_id ORDER BY received_at, id) AS sequence
    FROM connector_ingestion_runs
)
UPDATE connector_ingestion_runs r SET ingestion_sequence=o.sequence FROM ordered o WHERE r.id=o.id;

UPDATE data_sources ds SET
    next_ingestion_sequence=COALESCE((SELECT MAX(r.ingestion_sequence) FROM connector_ingestion_runs r
        JOIN connector_clients c ON c.id=r.connector_id WHERE c.data_source_id=ds.id),0),
    last_published_ingestion_sequence=COALESCE((SELECT MAX(r.ingestion_sequence) FROM connector_ingestion_runs r
        JOIN connector_clients c ON c.id=r.connector_id
        LEFT JOIN parser_snapshots p ON p.id=r.parser_snapshot_id
        WHERE c.data_source_id=ds.id AND (r.status='published' OR p.published_at IS NOT NULL)),0);

-- Missing historical ingestion cannot establish an order. Only a newly accepted
-- document may automatically replace the current snapshot after this upgrade.
UPDATE data_sources ds SET last_published_ingestion_sequence=next_ingestion_sequence
WHERE ds.adapter_type='external_push' AND ds.current_snapshot_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM connector_ingestion_runs r WHERE r.parser_snapshot_id=ds.current_snapshot_id);

UPDATE parser_snapshots p SET payload=jsonb_set(p.payload, '{ingestion_sequence}', to_jsonb(r.ingestion_sequence))
FROM connector_ingestion_runs r WHERE r.parser_snapshot_id=p.id;

CREATE FUNCTION scheduler_assign_ingestion_sequence() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    UPDATE data_sources ds SET next_ingestion_sequence=next_ingestion_sequence+1
    FROM connector_clients c WHERE c.id=NEW.connector_id AND ds.id=c.data_source_id
    RETURNING ds.next_ingestion_sequence INTO NEW.ingestion_sequence;
    IF NOT FOUND THEN RAISE EXCEPTION 'connector source does not exist'; END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER connector_ingestion_sequence BEFORE INSERT ON connector_ingestion_runs
FOR EACH ROW EXECUTE FUNCTION scheduler_assign_ingestion_sequence();
CREATE UNIQUE INDEX connector_ingestion_order ON connector_ingestion_runs(connector_id, ingestion_sequence);
ALTER TABLE connector_ingestion_runs DROP CONSTRAINT connector_ingestion_runs_status_check;
ALTER TABLE connector_ingestion_runs ADD CONSTRAINT connector_ingestion_runs_status_check
    CHECK(status IN ('received','processing','staged','quarantined','published','rejected','failed','superseded'));
