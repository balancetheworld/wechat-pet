CREATE TABLE IF NOT EXISTS ask_idempotency_keys (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    operation TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    response_data TEXT NOT NULL,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ask_idempotency_keys_family_fk FOREIGN KEY (family_id) REFERENCES families (id),
    CONSTRAINT ask_idempotency_keys_user_fk FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT ask_idempotency_keys_session_fk FOREIGN KEY (session_id) REFERENCES ask_sessions (id),
    CONSTRAINT ask_idempotency_keys_turn_fk FOREIGN KEY (turn_id) REFERENCES ask_turns (id),
    CONSTRAINT ask_idempotency_keys_run_fk FOREIGN KEY (run_id) REFERENCES ask_runs (id),
    CONSTRAINT ask_idempotency_keys_unique UNIQUE (user_id, operation, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_ask_idempotency_keys_session ON ask_idempotency_keys (session_id, created_at);
