ALTER TABLE echo_rank_snapshots ADD COLUMN evergreen_eligible boolean NOT NULL DEFAULT false;
CREATE INDEX echo_rank_snapshots_evergreen_idx ON echo_rank_snapshots(notability_score DESC, computed_at DESC)
    WHERE evergreen_eligible;
