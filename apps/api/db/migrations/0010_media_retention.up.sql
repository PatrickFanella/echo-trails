-- Original-object retention bookkeeping for privacy-safe cleanup.

ALTER TABLE media_assets ADD COLUMN original_deleted_at timestamptz;
CREATE INDEX media_assets_original_cleanup_idx ON media_assets(created_at)
    WHERE original_deleted_at IS NULL AND media_type <> 'quote_snapshot';
