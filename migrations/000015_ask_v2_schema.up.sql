-- v2 重构：ask 状态机与领域模型整体替换。
-- 说明：v2 中 Session 仅保留 active/closed，Run 取消 escalated、新增 canceling，
--       Turn 状态从“跟随 Run”改为独立的输入处理状态（received/attached/superseded）。
--       由于状态语义大改且 SQLite 不支持修改 CHECK 约束，此处重建 ask 相关表；
--       重构阶段不保留 v1 历史数据。

DROP TABLE IF EXISTS ask_idempotency_keys;
DROP TABLE IF EXISTS ask_messages;
DROP TABLE IF EXISTS ask_events;
DROP TABLE IF EXISTS ask_runs;
DROP TABLE IF EXISTS ask_turns;
DROP TABLE IF EXISTS ask_session_pets;
DROP TABLE IF EXISTS ask_sessions;

-- ===== 会话 =====
CREATE TABLE ask_sessions (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL,
    pet_id TEXT NOT NULL,
    created_by TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    risk_level TEXT NOT NULL DEFAULT 'unknown',
    turn_count INTEGER NOT NULL DEFAULT 0,
    input_sequence INTEGER NOT NULL DEFAULT 0,
    event_sequence INTEGER NOT NULL DEFAULT 0,
    launch_instance TEXT NOT NULL DEFAULT '',
    generation INTEGER NOT NULL DEFAULT 1,
    prompt_version TEXT NOT NULL,
    rule_version TEXT NOT NULL,
    knowledge_version TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    CONSTRAINT ask_sessions_family_fk FOREIGN KEY (family_id) REFERENCES families (id),
    CONSTRAINT ask_sessions_pet_fk FOREIGN KEY (pet_id) REFERENCES pets (id),
    CONSTRAINT ask_sessions_created_by_fk FOREIGN KEY (created_by) REFERENCES users (id),
    CONSTRAINT ask_sessions_status_check CHECK (status IN ('active', 'closed')),
    CONSTRAINT ask_sessions_risk_level_check CHECK (risk_level IN ('unknown', 'green', 'yellow', 'red')),
    CONSTRAINT ask_sessions_generation_check CHECK (generation > 0)
);

CREATE TABLE ask_session_pets (
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

-- ===== 输入轮次（Turn 独立状态）=====
CREATE TABLE ask_turns (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_index INTEGER NOT NULL,
    input_sequence INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'received',
    input TEXT NOT NULL,
    selected_run_id TEXT NOT NULL,
    superseded_by TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ask_turns_session_fk FOREIGN KEY (session_id) REFERENCES ask_sessions (id),
    CONSTRAINT ask_turns_status_check CHECK (status IN ('received', 'attached', 'superseded')),
    CONSTRAINT ask_turns_index_check CHECK (turn_index >= 0),
    CONSTRAINT ask_turns_session_index_unique UNIQUE (session_id, turn_index)
);

-- ===== 任务执行 =====
CREATE TABLE ask_runs (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    origin_turn_id TEXT NOT NULL DEFAULT '',
    run_index INTEGER NOT NULL,
    row_version INTEGER NOT NULL DEFAULT 1,
    clarification_count INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'queued',
    risk_level TEXT NOT NULL DEFAULT 'unknown',
    input_revision INTEGER NOT NULL DEFAULT 0,
    execution_epoch INTEGER NOT NULL DEFAULT 0,
    termination_reason TEXT NOT NULL DEFAULT '',
    rule_version TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_code TEXT NOT NULL DEFAULT '',
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at TIMESTAMP,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMP,
    CONSTRAINT ask_runs_session_fk FOREIGN KEY (session_id) REFERENCES ask_sessions (id),
    CONSTRAINT ask_runs_turn_fk FOREIGN KEY (turn_id) REFERENCES ask_turns (id),
    CONSTRAINT ask_runs_status_check CHECK (status IN ('queued', 'running', 'waiting_input', 'canceling', 'completed', 'failed', 'canceled', 'interrupted')),
    CONSTRAINT ask_runs_risk_level_check CHECK (risk_level IN ('unknown', 'green', 'yellow', 'red')),
    CONSTRAINT ask_runs_index_check CHECK (run_index >= 0),
    CONSTRAINT ask_runs_turn_index_unique UNIQUE (turn_id, run_index)
);

CREATE TABLE ask_events (
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

CREATE TABLE ask_messages (
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

CREATE TABLE ask_idempotency_keys (
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

-- ===== 至多一次模型请求 =====
CREATE TABLE ask_attempts (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    purpose TEXT NOT NULL DEFAULT '',
    input_snapshot TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    usage TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    CONSTRAINT ask_attempts_session_fk FOREIGN KEY (session_id) REFERENCES ask_sessions (id),
    CONSTRAINT ask_attempts_run_fk FOREIGN KEY (run_id) REFERENCES ask_runs (id),
    CONSTRAINT ask_attempts_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled', 'unknown')),
    CONSTRAINT ask_attempts_sequence_check CHECK (sequence > 0),
    CONSTRAINT ask_attempts_run_sequence_unique UNIQUE (run_id, sequence)
);

-- ===== 任务项（多宠物 / 多意图覆盖）=====
CREATE TABLE ask_task_items (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    object_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ask_task_items_session_fk FOREIGN KEY (session_id) REFERENCES ask_sessions (id),
    CONSTRAINT ask_task_items_run_fk FOREIGN KEY (run_id) REFERENCES ask_runs (id)
);

-- ===== 确认写入操作 =====
CREATE TABLE ask_operations (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    created_by TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    preview TEXT NOT NULL DEFAULT '',
    target TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL DEFAULT '',
    result TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 0,
    confirmed_at TIMESTAMP,
    expires_at TIMESTAMP,
    verify_until TIMESTAMP,
    verify_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ask_operations_session_fk FOREIGN KEY (session_id) REFERENCES ask_sessions (id),
    CONSTRAINT ask_operations_run_fk FOREIGN KEY (run_id) REFERENCES ask_runs (id),
    CONSTRAINT ask_operations_created_by_fk FOREIGN KEY (created_by) REFERENCES users (id),
    CONSTRAINT ask_operations_status_check CHECK (status IN ('pending', 'confirmed', 'executing', 'succeeded', 'failed', 'unknown', 'abandoned', 'withdrawn', 'expired'))
);

-- ===== 长期记忆（带来源版本）=====
CREATE TABLE ask_memories (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL,
    pet_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    content TEXT NOT NULL,
    source_type TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    source_version TEXT NOT NULL DEFAULT '',
    active INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ask_memories_family_fk FOREIGN KEY (family_id) REFERENCES families (id),
    CONSTRAINT ask_memories_kind_check CHECK (kind IN ('profile', 'event', 'fact'))
);

CREATE TABLE ask_source_versions (
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL,
    version TEXT NOT NULL,
    versioned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ask_source_versions_pk PRIMARY KEY (source_type, source_id)
);

CREATE INDEX idx_ask_sessions_family_pet ON ask_sessions (family_id, pet_id, created_at DESC);
CREATE INDEX idx_ask_session_pets_pet ON ask_session_pets (pet_id, session_id);
CREATE INDEX idx_ask_turns_session ON ask_turns (session_id, turn_index);
CREATE INDEX idx_ask_runs_session ON ask_runs (session_id, run_index);
CREATE INDEX idx_ask_runs_runnable ON ask_runs (status, next_attempt_at, lease_expires_at, created_at);
CREATE INDEX idx_ask_events_run ON ask_events (run_id, sequence);
CREATE INDEX idx_ask_messages_session ON ask_messages (session_id, created_at);
CREATE INDEX idx_ask_idempotency_keys_session ON ask_idempotency_keys (session_id, created_at);
CREATE INDEX idx_ask_attempts_run ON ask_attempts (run_id, sequence);
CREATE INDEX idx_ask_task_items_run ON ask_task_items (run_id, status);
CREATE INDEX idx_ask_operations_session ON ask_operations (session_id, status, created_at);
CREATE INDEX idx_ask_memories_family_pet ON ask_memories (family_id, pet_id, kind);
