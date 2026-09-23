ALTER TABLE ask_budgets ADD COLUMN max_duration_millis BIGINT NOT NULL DEFAULT 0;
ALTER TABLE ask_budgets ADD COLUMN duration_used_millis BIGINT NOT NULL DEFAULT 0;
ALTER TABLE ask_budgets ADD COLUMN duration_reserved_millis BIGINT NOT NULL DEFAULT 0;
ALTER TABLE ask_reservations ADD COLUMN duration_millis BIGINT NOT NULL DEFAULT 0;
