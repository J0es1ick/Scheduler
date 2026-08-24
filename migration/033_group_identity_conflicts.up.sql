CREATE TABLE group_source_identity_mappings (
    data_source_id    TEXT NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    external_group_id TEXT NOT NULL,
    group_id          TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    expected_name     TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (data_source_id, external_group_id)
);

CREATE INDEX idx_group_source_identity_mappings_group
    ON group_source_identity_mappings(group_id);

CREATE TABLE group_identity_conflicts (
    id                TEXT PRIMARY KEY,
    data_source_id    TEXT NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    university_id     TEXT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    external_group_id TEXT NOT NULL,
    existing_group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    existing_name     TEXT NOT NULL,
    incoming_name     TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'resolved')),
    resolution        TEXT NOT NULL DEFAULT ''
                      CHECK (resolution IN ('', 'rename', 'new_group')),
    resolved_group_id TEXT REFERENCES groups(id) ON DELETE SET NULL,
    resolved_by       TEXT NOT NULL DEFAULT '',
    first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at       TIMESTAMPTZ,
    occurrences       INT NOT NULL DEFAULT 1 CHECK (occurrences > 0),
    UNIQUE (data_source_id, external_group_id)
);

CREATE INDEX idx_group_identity_conflicts_pending
    ON group_identity_conflicts(data_source_id, last_seen_at DESC)
    WHERE status = 'pending';
