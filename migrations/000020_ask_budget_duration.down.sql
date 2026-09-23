ALTER TABLE ask_reservations DROP COLUMN duration_millis;
ALTER TABLE ask_budgets DROP COLUMN duration_reserved_millis;
ALTER TABLE ask_budgets DROP COLUMN duration_used_millis;
ALTER TABLE ask_budgets DROP COLUMN max_duration_millis;
