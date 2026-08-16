-- Candidate retrieval must stop after the bounded recent/notable windows instead
-- of sorting every matching published Echo before final weighted ranking.
CREATE INDEX echoes_published_search_candidates_idx
    ON echoes(published_at DESC, id DESC)
    WHERE status = 'published' AND deleted_at IS NULL;

CREATE INDEX echo_rank_snapshots_search_candidates_idx
    ON echo_rank_snapshots(notability_score DESC, echo_id DESC);
