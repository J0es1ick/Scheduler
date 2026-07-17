ALTER TYPE lesson_type ADD VALUE IF NOT EXISTS 'exam';
ALTER TYPE lesson_type ADD VALUE IF NOT EXISTS 'credit';
ALTER TYPE lesson_type ADD VALUE IF NOT EXISTS 'consultation';

ALTER TABLE lessons DROP CONSTRAINT IF EXISTS lessons_schedule_shape_check;
ALTER TABLE lessons
    ADD CONSTRAINT lessons_schedule_shape_check CHECK (
        (week_type = 'date' AND special_date IS NOT NULL AND day_of_week IS NULL) OR
        (week_type != 'date' AND special_date IS NULL AND day_of_week BETWEEN 1 AND 7)
    );

INSERT INTO universities (id, name, full_name, schedule_url, is_active)
VALUES (
    'ispu',
    'ИГЭУ',
    'Ивановский государственный энергетический университет имени В.И. Ленина',
    'http://schedule.ispu.ru/',
    TRUE
)
ON CONFLICT (id) DO UPDATE SET
    name         = EXCLUDED.name,
    full_name    = EXCLUDED.full_name,
    schedule_url = EXCLUDED.schedule_url,
    is_active    = EXCLUDED.is_active,
    updated_at   = NOW();

INSERT INTO data_sources (id, university_id, adapter_type, config, update_interval)
VALUES ('ispu-main', 'ispu', 'ispu', '', 3600)
ON CONFLICT (id) DO UPDATE SET
    university_id   = EXCLUDED.university_id,
    adapter_type    = EXCLUDED.adapter_type,
    update_interval = EXCLUDED.update_interval,
    updated_at      = NOW();
