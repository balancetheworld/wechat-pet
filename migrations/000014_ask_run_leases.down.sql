DROP INDEX IF EXISTS idx_ask_runs_runnable;

ALTER TABLE ask_runs DROP COLUMN next_attempt_at;
ALTER TABLE ask_runs DROP COLUMN attempt_count;
ALTER TABLE ask_runs DROP COLUMN lease_expires_at;
ALTER TABLE ask_runs DROP COLUMN lease_owner;
