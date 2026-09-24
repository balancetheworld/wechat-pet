DROP INDEX IF EXISTS idx_ask_sessions_current_unique;

ALTER TABLE ask_sessions DROP COLUMN deleted_at;
ALTER TABLE ask_turns DROP COLUMN deleted_at;
ALTER TABLE ask_runs DROP COLUMN deleted_at;
ALTER TABLE ask_events DROP COLUMN deleted_at;
ALTER TABLE ask_messages DROP COLUMN deleted_at;
ALTER TABLE ask_operations DROP COLUMN deleted_at;
ALTER TABLE ask_attempts DROP COLUMN deleted_at;
ALTER TABLE ask_memories DROP COLUMN deleted_at;
ALTER TABLE ask_idempotency_keys DROP COLUMN deleted_at;
