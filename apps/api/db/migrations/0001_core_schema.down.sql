-- Roll back EchoTrails core schema tables created by 0001_core_schema.up.sql.
-- PostGIS/citext extensions are intentionally retained because extensions can be shared.

DROP TABLE IF EXISTS invite_codes;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS analytics_events;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS blocks;
DROP TABLE IF EXISTS appeals;
DROP TABLE IF EXISTS moderation_actions;
DROP TABLE IF EXISTS reports;
DROP TABLE IF EXISTS bookmarks;
DROP TABLE IF EXISTS reactions;
DROP TABLE IF EXISTS media_derivatives;
DROP TABLE IF EXISTS echo_links;
DROP TABLE IF EXISTS user_profiles;
DROP TABLE IF EXISTS media_assets;
DROP TABLE IF EXISTS replies;
DROP TABLE IF EXISTS echoes;
DROP TABLE IF EXISTS place_subscriptions;
DROP TABLE IF EXISTS place_aliases;
DROP TABLE IF EXISTS place_provider_mappings;
DROP TABLE IF EXISTS places;
DROP TABLE IF EXISTS consents;
DROP TABLE IF EXISTS devices;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS auth_identities;
DROP TABLE IF EXISTS users;
