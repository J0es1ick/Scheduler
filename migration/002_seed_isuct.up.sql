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

-- Семестр заранее не задаётся: парсер создаёт и обновляет техническую запись
-- isuct-current по диапазонам дат, полученным непосредственно с сайта.

-- Источник данных для парсера.
-- update_interval = 3600 сек = 1 раз в час.
-- last_run_at = NULL → воркер запустит парсинг при первом же тике.
INSERT INTO data_sources (id, university_id, adapter_type, config, update_interval)
VALUES (
    'isuct-main',
    'isuct',
    'isuct',
    '',
    3600
)
ON CONFLICT (id) DO NOTHING;
