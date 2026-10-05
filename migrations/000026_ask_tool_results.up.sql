-- 工具结果持久化与复用（v2 架构文档 7.7）：只读结果允许在同一 Session 的不同 Run
-- 之间复用，复用前必须核验权限、查询范围、来源与集合版本；写入准备结果不参与复用。
-- 正文与可核验证据在执行时固化，Run 挂起恢复时直接重放，不依赖具体业务类型重新渲染。

CREATE TABLE ask_tool_results (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    tool_call_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    tool_version TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    task_keys TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL,
    completeness TEXT NOT NULL DEFAULT '',
    has_more INTEGER NOT NULL DEFAULT 0,
    next_cursor TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    evidence TEXT NOT NULL DEFAULT '[]',
    source_type TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    source_version TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    CONSTRAINT ask_tool_results_run_fk FOREIGN KEY (run_id) REFERENCES ask_runs (id)
);

CREATE INDEX idx_ask_tool_results_run ON ask_tool_results (run_id, created_at);
CREATE INDEX idx_ask_tool_results_reuse ON ask_tool_results (session_id, fingerprint, status);
