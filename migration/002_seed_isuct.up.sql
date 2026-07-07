-- Seed: ИГХТУ — базовые данные для запуска бота.
-- Выполнить один раз после 001_init.up.sql.

-- Университет
INSERT INTO universities (id, name, full_name, schedule_url, is_active)
VALUES (
    'isuct',
    'ИГХТУ',
    'Ивановский государственный химико-технологический университет',
    'https://www.isuct.ru/student/schedule',
    TRUE
)
ON CONFLICT (id) DO UPDATE SET
    name         = EXCLUDED.name,
    full_name    = EXCLUDED.full_name,
    schedule_url = EXCLUDED.schedule_url,
    is_active    = EXCLUDED.is_active,
    updated_at   = NOW();

-- Текущий семестр (весна 2026).
-- Скорректируй start_date/end_date если они изменятся.
-- Нечётная неделя = 1-я от start_date, чётная = 2-я и т.д.
INSERT INTO semesters (id, university_id, name, start_date, end_date)
VALUES (
    'isuct-2026-spring',
    'isuct',
    'Весенний семестр 2025–2026',
    '2026-02-02',   -- первый учебный день (понедельник, I неделя)
    '2026-05-31'
)
ON CONFLICT (id) DO UPDATE SET
    name       = EXCLUDED.name,
    start_date = EXCLUDED.start_date,
    end_date   = EXCLUDED.end_date;

-- Источник данных для парсера.
-- update_interval = 86400 сек = 1 раз в сутки (достаточно, расписание меняется редко).
-- last_run_at = NULL → воркер запустит парсинг при первом же тике.
INSERT INTO data_sources (id, university_id, adapter_type, config, update_interval)
VALUES (
    'isuct-main',
    'isuct',
    'isuct',
    '',
    86400
)
ON CONFLICT (id) DO NOTHING;
