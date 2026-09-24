DROP INDEX IF EXISTS idx_ask_sessions_resolved_pet;

ALTER TABLE ask_sessions DROP COLUMN resolved_pet_id;
