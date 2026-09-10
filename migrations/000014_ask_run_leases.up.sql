ALTER TABLE ask_runs ADD COLUMN lease_owner TEXT NOT NULL DEFAULT '';
ALTER TABLE ask_runs ADD COLUMN lease_expires_at TIMESTAMP;
ALTER TABLE ask_runs ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ask_runs ADD COLUMN next_attempt_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_ask_runs_runnable ON ask_runs (status, next_attempt_at, lease_expires_at, created_at);
