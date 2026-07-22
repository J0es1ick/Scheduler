CREATE TABLE lesson_overrides (
    id                 TEXT PRIMARY KEY,
    base_lesson_id     TEXT,
    university_id      TEXT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    semester_id        TEXT NOT NULL REFERENCES semesters(id) ON DELETE CASCADE,
    day_of_week        INT,
    special_date       DATE,
    time_start         TEXT NOT NULL,
    time_end           TEXT NOT NULL,
    week_type          week_type NOT NULL,
    subject            TEXT NOT NULL,
    type               lesson_type NOT NULL,
    teacher            TEXT NOT NULL DEFAULT '',
    room               TEXT NOT NULL DEFAULT '',
    group_id           TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    subgroup           INT NOT NULL DEFAULT 0,
    valid_from         DATE,
    valid_to           DATE,
    is_deleted         BOOLEAN NOT NULL DEFAULT FALSE,
    created_by         TEXT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version            BIGINT NOT NULL DEFAULT 1,
    CONSTRAINT lesson_overrides_schedule_shape_check CHECK (
        (week_type = 'date' AND special_date IS NOT NULL AND day_of_week IS NULL) OR
        (week_type != 'date' AND special_date IS NULL AND day_of_week BETWEEN 1 AND 7)
    ),
    CONSTRAINT lesson_overrides_validity_check CHECK (
        (valid_from IS NULL AND valid_to IS NULL) OR
        (valid_from IS NOT NULL AND valid_to IS NOT NULL AND valid_from <= valid_to)
    ),
    CONSTRAINT lesson_overrides_subgroup_check CHECK (subgroup BETWEEN 0 AND 10)
);

CREATE UNIQUE INDEX idx_lesson_overrides_base
    ON lesson_overrides(base_lesson_id)
    WHERE base_lesson_id IS NOT NULL;

CREATE INDEX idx_lesson_overrides_group
    ON lesson_overrides(group_id);

CREATE OR REPLACE VIEW effective_lessons AS
SELECT
    l.id,
    l.university_id,
    l.semester_id,
    l.day_of_week,
    l.special_date,
    l.time_start,
    l.time_end,
    l.week_type,
    l.subject,
    l.type,
    l.teacher,
    l.room,
    l.group_id,
    l.subgroup,
    l.valid_from,
    l.valid_to,
    l.updated_at,
    'parsed'::TEXT AS origin,
    NULL::TEXT AS base_lesson_id,
    0::BIGINT AS version
FROM lessons l
WHERE NOT EXISTS (
    SELECT 1
    FROM lesson_overrides o
    WHERE o.base_lesson_id = l.id
)
UNION ALL
SELECT
    o.id,
    o.university_id,
    o.semester_id,
    o.day_of_week,
    o.special_date,
    o.time_start,
    o.time_end,
    o.week_type,
    o.subject,
    o.type,
    o.teacher,
    o.room,
    o.group_id,
    o.subgroup,
    o.valid_from,
    o.valid_to,
    o.updated_at,
    'manual'::TEXT AS origin,
    o.base_lesson_id,
    o.version
FROM lesson_overrides o
WHERE NOT o.is_deleted;
