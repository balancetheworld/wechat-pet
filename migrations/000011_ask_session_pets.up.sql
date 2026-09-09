CREATE TABLE IF NOT EXISTS ask_session_pets (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    pet_id TEXT NOT NULL,
    mention TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    CONSTRAINT ask_session_pets_session_fk FOREIGN KEY (session_id) REFERENCES ask_sessions (id),
    CONSTRAINT ask_session_pets_pet_fk FOREIGN KEY (pet_id) REFERENCES pets (id),
    CONSTRAINT ask_session_pets_sort_check CHECK (sort_order >= 0),
    CONSTRAINT ask_session_pets_unique UNIQUE (session_id, pet_id),
    CONSTRAINT ask_session_pets_order_unique UNIQUE (session_id, sort_order)
);

CREATE INDEX IF NOT EXISTS idx_ask_session_pets_pet ON ask_session_pets (pet_id, session_id);
