DROP INDEX IF EXISTS media_assets_original_cleanup_idx;
ALTER TABLE media_assets DROP COLUMN IF EXISTS original_deleted_at;
