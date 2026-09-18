-- T2：调度、预算与恢复基础 —— 四类预算账本与原子预留记录。
-- 依据 v2 架构文档 9.3：四类预算（run / operation_exec / operation_verify / memory_derivation）
-- 互相独立、不互相重置，都受服务总量及并发限制；执行资格预算与真实用量分别记录。
-- 原子预留通过账本行在事务内条件更新实现，防止并发超售。

CREATE TABLE ask_budgets (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    max_model_calls INTEGER NOT NULL DEFAULT 0,
    max_tool_calls INTEGER NOT NULL DEFAULT 0,
    max_tokens INTEGER NOT NULL DEFAULT 0,
    max_cost_micros BIGINT NOT NULL DEFAULT 0,
    max_concurrency INTEGER NOT NULL DEFAULT 0,
    model_calls_used INTEGER NOT NULL DEFAULT 0,
    tool_calls_used INTEGER NOT NULL DEFAULT 0,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    cost_used_micros BIGINT NOT NULL DEFAULT 0,
    model_calls_reserved INTEGER NOT NULL DEFAULT 0,
    tool_calls_reserved INTEGER NOT NULL DEFAULT 0,
    tokens_reserved INTEGER NOT NULL DEFAULT 0,
    cost_reserved_micros BIGINT NOT NULL DEFAULT 0,
    concurrency_reserved INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ask_budgets_scope_unique UNIQUE (scope, scope_id),
    CONSTRAINT ask_budgets_scope_check CHECK (scope IN ('run', 'operation_exec', 'operation_verify', 'memory_derivation'))
);

CREATE INDEX idx_ask_budgets_scope ON ask_budgets (scope, scope_id);

CREATE TABLE ask_reservations (
    id TEXT PRIMARY KEY,
    budget_id TEXT NOT NULL,
    model_calls INTEGER NOT NULL DEFAULT 0,
    tool_calls INTEGER NOT NULL DEFAULT 0,
    tokens INTEGER NOT NULL DEFAULT 0,
    cost_micros BIGINT NOT NULL DEFAULT 0,
    concurrency INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'reserved',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    settled_at TIMESTAMP,
    released_at TIMESTAMP,
    CONSTRAINT ask_reservations_budget_fk FOREIGN KEY (budget_id) REFERENCES ask_budgets (id),
    CONSTRAINT ask_reservations_status_check CHECK (status IN ('reserved', 'settled', 'released'))
);

CREATE INDEX idx_ask_reservations_budget ON ask_reservations (budget_id, status);
