WITH ranked_sources AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY university_id
            ORDER BY
                is_enabled DESC,
                (current_snapshot_id IS NOT NULL) DESC,
                last_success_at DESC NULLS LAST,
                updated_at DESC,
                created_at DESC,
                id
        ) AS position
    FROM data_sources
    WHERE lifecycle_status = 'active'
), suspended_sources AS (
    UPDATE data_sources ds
    SET lifecycle_status = 'suspended',
        is_enabled = FALSE,
        updated_at = NOW()
    FROM ranked_sources ranked
    WHERE ds.id = ranked.id
      AND ranked.position > 1
    RETURNING ds.id
)
UPDATE connector_clients connector
SET status = 'suspended',
    updated_at = NOW()
FROM suspended_sources source
WHERE connector.data_source_id = source.id
  AND connector.status = 'active';

CREATE UNIQUE INDEX IF NOT EXISTS idx_data_sources_one_active_per_university
    ON data_sources(university_id)
    WHERE lifecycle_status = 'active';

UPDATE lessons lesson
SET recurrence = jsonb_build_object(
        'cycle_length', 2,
        'cycle_weeks', CASE lesson.week_type
            WHEN 'odd' THEN jsonb_build_array(1)
            ELSE jsonb_build_array(2)
        END,
        'anchor_date', to_jsonb(to_char(semester.start_date, 'YYYY-MM-DD') || 'T00:00:00Z')
    )
FROM data_sources source, semesters semester
WHERE source.id = lesson.source_id
  AND source.adapter_type = 'external_push'
  AND semester.id = lesson.semester_id
  AND lesson.week_type IN ('odd', 'even')
  AND lesson.recurrence = '{}'::jsonb;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS telegram_menu_fingerprint TEXT NOT NULL DEFAULT '';
