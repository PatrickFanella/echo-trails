ALTER TABLE appeals DROP COLUMN IF EXISTS body_revisions;
DROP TABLE IF EXISTS admin_sessions;
DROP TABLE IF EXISTS user_privacy_settings;
ALTER TABLE users DROP COLUMN IF EXISTS beta_access_at;
