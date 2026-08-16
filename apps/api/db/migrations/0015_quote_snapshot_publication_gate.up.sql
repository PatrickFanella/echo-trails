-- Quoted Reply snapshots are part of the published Stitch contract. A Stitch
-- stays private until its immutable snapshot is ready or deliberately
-- suppressed by privacy/safety handling.

UPDATE echoes echo
SET status = 'processing', published_at = NULL, updated_at = now()
FROM echo_links link
WHERE link.target_echo_id = echo.id
  AND link.quoted_reply_id IS NOT NULL
  AND link.snapshot_status NOT IN ('ready', 'suppressed')
  AND echo.status = 'published';

CREATE FUNCTION echotrails_quote_snapshot_publication_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status = 'published' AND EXISTS (
        SELECT 1 FROM echo_links link
        WHERE link.target_echo_id = NEW.id
          AND link.quoted_reply_id IS NOT NULL
          AND link.snapshot_status NOT IN ('ready', 'suppressed')
    ) THEN
        RAISE EXCEPTION 'quoted Reply snapshot is not publication-ready' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER quote_snapshot_publication_guard
BEFORE UPDATE OF status ON echoes
FOR EACH ROW EXECUTE FUNCTION echotrails_quote_snapshot_publication_guard();
