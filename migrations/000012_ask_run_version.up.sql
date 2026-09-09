ALTER TABLE ask_runs ADD COLUMN row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE ask_runs ADD COLUMN clarification_count INTEGER NOT NULL DEFAULT 0;

UPDATE ask_runs SET completed_at = NULL WHERE status = 'waiting_input';

INSERT INTO ask_session_pets (id, session_id, pet_id, mention, sort_order)
SELECT ask_sessions.id || '-pet-0', ask_sessions.id, ask_sessions.pet_id, pets.name, 0
FROM ask_sessions
INNER JOIN pets ON pets.id = ask_sessions.pet_id
WHERE NOT EXISTS (
    SELECT 1 FROM ask_session_pets WHERE ask_session_pets.session_id = ask_sessions.id
);
