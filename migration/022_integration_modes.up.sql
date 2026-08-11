ALTER TABLE connector_clients
    ADD COLUMN IF NOT EXISTS integration_mode TEXT NOT NULL DEFAULT 'external_push',
    ADD COLUMN IF NOT EXISTS parser_id TEXT NOT NULL DEFAULT '';

ALTER TABLE connector_clients
    ADD CONSTRAINT connector_clients_integration_mode_check
        CHECK (integration_mode IN ('managed_parser', 'declarative_pull', 'external_push'));

CREATE INDEX IF NOT EXISTS idx_connector_clients_mode
    ON connector_clients(integration_mode, status);

CREATE UNIQUE INDEX IF NOT EXISTS idx_connector_clients_parser_unique
    ON connector_clients(parser_id)
    WHERE parser_id<>'';

INSERT INTO universities (
    id, name, full_name, schedule_url, timezone, locale, is_active, created_at, updated_at
) VALUES (
    'ivgpu',
    'ИВГПУ',
    'Ивановский государственный политехнический университет',
    'https://ivgpu.ru/raspisanie',
    'Europe/Moscow',
    'ru-RU',
    TRUE,
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    name=EXCLUDED.name,
    full_name=EXCLUDED.full_name,
    schedule_url=EXCLUDED.schedule_url,
    timezone=EXCLUDED.timezone,
    locale=EXCLUDED.locale,
    is_active=TRUE,
    updated_at=NOW();

DO $$
DECLARE
    selected_source_id TEXT;
BEGIN
    SELECT id INTO selected_source_id
    FROM data_sources
    WHERE university_id='ivgpu'
    ORDER BY CASE WHEN lifecycle_status='active' THEN 0 ELSE 1 END, created_at
    LIMIT 1;

    IF selected_source_id IS NULL THEN
        selected_source_id := 'ivgpu-main';
        INSERT INTO data_sources (
            id, university_id, adapter_type, config, is_enabled, update_interval,
            last_error, lifecycle_status, quality_policy, created_at, updated_at
        ) VALUES (
            selected_source_id, 'ivgpu', 'managed:ivgpu', '{}', TRUE, 3600,
            '', 'active', '{
                "allow_empty": false,
                "minimum_groups": 1,
                "minimum_lessons": 0,
                "maximum_group_drop_ratio": 0.30,
                "maximum_group_growth_ratio": 0.80,
                "maximum_lesson_drop_ratio": 0.40,
                "maximum_lesson_growth_ratio": 1.00
            }'::jsonb, NOW(), NOW()
        );
    ELSE
        UPDATE data_sources
        SET adapter_type='managed:ivgpu', config='{}', update_interval=3600,
            is_enabled=(lifecycle_status='active'), updated_at=NOW()
        WHERE id=selected_source_id;
    END IF;

	UPDATE connector_clients
	SET status='archived', updated_at=NOW()
	WHERE data_source_id IN (
		SELECT id FROM data_sources WHERE university_id='ivgpu' AND id<>selected_source_id
	);
	UPDATE data_sources
	SET lifecycle_status='archived', is_enabled=FALSE, archived_at=NOW(), updated_at=NOW()
	WHERE university_id='ivgpu' AND id<>selected_source_id;

    IF EXISTS (SELECT 1 FROM connector_clients WHERE data_source_id=selected_source_id) THEN
        UPDATE connector_clients
        SET integration_mode='managed_parser', parser_id='ivgpu',
            display_name='ИВГПУ · управляемый парсер',
            description='Официальный JSON API. Парсер запускается и наблюдается самим Scheduler.',
            maintainer_name='Scheduler contributors',
            maintainer_url='https://github.com/J0es1ick/Scheduler/tree/master/integrations/ivgpu',
            updated_at=NOW()
        WHERE data_source_id=selected_source_id;
    ELSE
        INSERT INTO connector_clients (
            id, data_source_id, display_name, description, maintainer_name,
            maintainer_url, key_id, public_key, status, created_by,
            integration_mode, parser_id, created_at, updated_at
        ) VALUES (
            'managed-ivgpu', selected_source_id, 'ИВГПУ · управляемый парсер',
            'Официальный JSON API. Парсер запускается и наблюдается самим Scheduler.',
            'Scheduler contributors',
            'https://github.com/J0es1ick/Scheduler/tree/master/integrations/ivgpu',
            'managed-key-ivgpu', decode('', 'hex'), 'active', 'system',
            'managed_parser', 'ivgpu', NOW(), NOW()
        );
    END IF;
END $$;
