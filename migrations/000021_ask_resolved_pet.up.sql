ALTER TABLE ask_sessions ADD COLUMN resolved_pet_id TEXT;

UPDATE ask_sessions SET resolved_pet_id = pet_id;

CREATE INDEX IF NOT EXISTS idx_ask_sessions_resolved_pet
    ON ask_sessions (family_id, resolved_pet_id, created_at DESC);
