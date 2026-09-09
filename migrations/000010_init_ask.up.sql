CREATE TABLE IF NOT EXISTS ask_sessions (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL,
    pet_id TEXT NOT NULL,
    created_by TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    risk_level TEXT NOT NULL DEFAULT 'unknown',
    turn_count INTEGER NOT NULL DEFAULT 0,
    prompt_version TEXT NOT NULL,
    rule_version TEXT NOT NULL,
    knowledge_version TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    CONSTRAINT ask_sessions_family_fk FOREIGN KEY (family_id) REFERENCES families (id),
    CONSTRAINT ask_sessions_pet_fk FOREIGN KEY (pet_id) REFERENCES pets (id),
    CONSTRAINT ask_sessions_created_by_fk FOREIGN KEY (created_by) REFERENCES users (id),
    CONSTRAINT ask_sessions_status_check CHECK (status IN ('active', 'completed', 'escalated', 'canceled')),
    CONSTRAINT ask_sessions_risk_level_check CHECK (risk_level IN ('unknown', 'green', 'yellow', 'red'))
);

CREATE TABLE IF NOT EXISTS ask_turns (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_index INTEGER NOT NULL,
    status TEXT NOT NULL,
    input TEXT NOT NULL,
    selected_run_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ask_turns_session_fk FOREIGN KEY (session_id) REFERENCES ask_sessions (id),
    CONSTRAINT ask_turns_status_check CHECK (status IN ('queued', 'running', 'waiting_input', 'completed', 'escalated', 'failed', 'canceled', 'interrupted')),
    CONSTRAINT ask_turns_index_check CHECK (turn_index >= 0),
    CONSTRAINT ask_turns_session_index_unique UNIQUE (session_id, turn_index)
);

CREATE TABLE IF NOT EXISTS ask_runs (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    run_index INTEGER NOT NULL,
    status TEXT NOT NULL,
    risk_level TEXT NOT NULL DEFAULT 'unknown',
    rule_version TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_code TEXT NOT NULL DEFAULT '',
    CONSTRAINT ask_runs_session_fk FOREIGN KEY (session_id) REFERENCES ask_sessions (id),
    CONSTRAINT ask_runs_turn_fk FOREIGN KEY (turn_id) REFERENCES ask_turns (id),
    CONSTRAINT ask_runs_status_check CHECK (status IN ('queued', 'running', 'waiting_input', 'completed', 'escalated', 'failed', 'canceled', 'interrupted')),
    CONSTRAINT ask_runs_risk_level_check CHECK (risk_level IN ('unknown', 'green', 'yellow', 'red')),
    CONSTRAINT ask_runs_index_check CHECK (run_index >= 0),
    CONSTRAINT ask_runs_turn_index_unique UNIQUE (turn_id, run_index)
);

CREATE TABLE IF NOT EXISTS ask_events (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    type TEXT NOT NULL,
    data TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ask_events_session_fk FOREIGN KEY (session_id) REFERENCES ask_sessions (id),
    CONSTRAINT ask_events_turn_fk FOREIGN KEY (turn_id) REFERENCES ask_turns (id),
    CONSTRAINT ask_events_run_fk FOREIGN KEY (run_id) REFERENCES ask_runs (id),
    CONSTRAINT ask_events_sequence_check CHECK (sequence > 0),
    CONSTRAINT ask_events_run_sequence_unique UNIQUE (run_id, sequence)
);

CREATE TABLE IF NOT EXISTS ask_messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    run_id TEXT,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ask_messages_session_fk FOREIGN KEY (session_id) REFERENCES ask_sessions (id),
    CONSTRAINT ask_messages_turn_fk FOREIGN KEY (turn_id) REFERENCES ask_turns (id),
    CONSTRAINT ask_messages_run_fk FOREIGN KEY (run_id) REFERENCES ask_runs (id),
    CONSTRAINT ask_messages_role_check CHECK (role IN ('user', 'assistant', 'question'))
);

CREATE INDEX IF NOT EXISTS idx_ask_sessions_family_pet ON ask_sessions (family_id, pet_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ask_turns_session ON ask_turns (session_id, turn_index);
CREATE INDEX IF NOT EXISTS idx_ask_runs_session ON ask_runs (session_id, run_index);
CREATE INDEX IF NOT EXISTS idx_ask_events_run ON ask_events (run_id, sequence);
CREATE INDEX IF NOT EXISTS idx_ask_messages_session ON ask_messages (session_id, created_at);
