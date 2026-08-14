CREATE TABLE operational_maintenance (
    task_name   TEXT PRIMARY KEY,
    last_run_at TIMESTAMPTZ NOT NULL DEFAULT '-infinity'
);

INSERT INTO operational_maintenance (task_name)
VALUES ('retention')
ON CONFLICT (task_name) DO NOTHING;

CREATE INDEX idx_connector_ingestion_retention
    ON connector_ingestion_runs(received_at)
    WHERE status IN ('staged','quarantined','published','rejected','failed');

CREATE INDEX idx_parser_snapshots_retention
    ON parser_snapshots(created_at)
    WHERE status IN ('quarantined','rejected');

CREATE INDEX idx_parse_logs_retention
    ON parse_logs(finished_at)
    WHERE finished_at IS NOT NULL;
