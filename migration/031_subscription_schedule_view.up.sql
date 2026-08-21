ALTER TABLE subscriptions
    ADD COLUMN schedule_view_format TEXT NOT NULL DEFAULT 'compact';

ALTER TABLE subscriptions
    ADD CONSTRAINT subscriptions_schedule_view_format_check
    CHECK (schedule_view_format IN ('compact', 'visual'));
