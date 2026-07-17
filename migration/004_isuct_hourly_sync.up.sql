UPDATE data_sources
SET update_interval = 3600,
    updated_at = NOW()
WHERE id = 'isuct-main';
