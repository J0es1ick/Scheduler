DELETE FROM subscriptions s
WHERE s.object_type='group'
  AND NOT EXISTS (SELECT 1 FROM groups g WHERE g.id=s.object_id);

UPDATE users u SET default_group_id=NULL, updated_at=NOW()
WHERE u.default_group_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM groups g WHERE g.id=u.default_group_id);

INSERT INTO subscriptions (id, user_id, object_id, object_type)
SELECT 'reconciled:' || md5(u.id || chr(31) || u.default_group_id),
       u.id, u.default_group_id, 'group'
FROM users u
WHERE u.default_group_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM subscriptions s
      WHERE s.user_id=u.id AND s.object_type='group' AND s.object_id=u.default_group_id
  )
ON CONFLICT (user_id, object_id, object_type) DO NOTHING;

ALTER TABLE subscriptions ADD COLUMN group_id TEXT
    GENERATED ALWAYS AS (CASE WHEN object_type='group' THEN object_id END) STORED;
ALTER TABLE subscriptions ADD CONSTRAINT subscriptions_group_fk
    FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE;
ALTER TABLE subscriptions ADD CONSTRAINT subscriptions_user_group_unique UNIQUE (user_id, group_id);
CREATE INDEX subscriptions_group_reference_idx ON subscriptions(group_id) WHERE group_id IS NOT NULL;

ALTER TABLE users ADD CONSTRAINT users_default_subscription_fk
    FOREIGN KEY (id, default_group_id) REFERENCES subscriptions(user_id, group_id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE VIEW subscription_integrity AS
SELECT
    (SELECT COUNT(*) FROM users u WHERE u.default_group_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM subscriptions s WHERE s.user_id=u.id AND s.group_id=u.default_group_id
    )) AS missing_default_subscriptions,
    (SELECT COUNT(*) FROM subscriptions s WHERE s.object_type='group' AND NOT EXISTS (
        SELECT 1 FROM groups g WHERE g.id=s.object_id
    )) AS orphan_group_subscriptions;
