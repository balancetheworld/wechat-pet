-- 回滚：恢复 000015 的旧版 ask_task_items 表结构（kind/object_id/status/reason）。
-- 重构阶段无历史数据保留要求。

DROP TABLE IF EXISTS ask_task_items;

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

CREATE INDEX idx_ask_task_items_run ON ask_task_items (run_id, status);
