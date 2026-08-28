UPDATE users
SET is_admin = (admin_role <> 'none')
WHERE is_admin IS DISTINCT FROM (admin_role <> 'none');

ALTER TABLE users
    ADD CONSTRAINT users_admin_role_consistency_check
    CHECK (is_admin = (admin_role <> 'none'));

ALTER TABLE subscriptions
    ADD COLUMN subgroup SMALLINT NOT NULL DEFAULT 0;

ALTER TABLE subscriptions
    ADD CONSTRAINT subscriptions_subgroup_check
    CHECK (subgroup BETWEEN 0 AND 100);

ALTER TABLE chat_schedule_profiles
    ADD COLUMN schedule_view_format TEXT NOT NULL DEFAULT 'compact';

ALTER TABLE chat_schedule_profiles
    ADD CONSTRAINT chat_schedule_profiles_view_format_check
    CHECK (schedule_view_format IN ('compact', 'visual'));

UPDATE data_sources
SET is_enabled = FALSE,
    updated_at = NOW()
WHERE lifecycle_status <> 'active'
  AND is_enabled;

ALTER TABLE data_sources
    ADD CONSTRAINT data_sources_enabled_lifecycle_check
    CHECK (NOT is_enabled OR lifecycle_status = 'active');
