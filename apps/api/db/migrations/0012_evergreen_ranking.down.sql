DROP INDEX IF EXISTS echo_rank_snapshots_evergreen_idx;
ALTER TABLE echo_rank_snapshots DROP COLUMN IF EXISTS evergreen_eligible;
