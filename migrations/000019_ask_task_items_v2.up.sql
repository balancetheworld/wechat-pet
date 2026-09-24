-- T5：重建 ask_task_items 表，对齐定稿的任务项领域模型（task_item.go）。
-- 000015 的旧表仅存 kind/object_id/status/reason 四条简化字段，与 v2 任务项
-- （目标、对象、结果分类、缺失信息、结果关联、替代关系）无法对应，故重建。
-- 重构阶段不保留旧数据；deleted_at 软删除对齐 000016。

DROP TABLE IF EXISTS ask_task_items;

CREATE TABLE ask_task_items (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    origin_turn_id TEXT NOT NULL DEFAULT '',
    item_revision INTEGER NOT NULL DEFAULT 1,
    goal TEXT NOT NULL,
    source_turn_ids TEXT NOT NULL DEFAULT '[]',
    subjects TEXT NOT NULL DEFAULT '[]',
    outcome TEXT NOT NULL DEFAULT 'pending',
    missing_fields TEXT NOT NULL DEFAULT '[]',
    incomplete_reason TEXT NOT NULL DEFAULT '',
    result_ref TEXT NOT NULL DEFAULT '',
    supersedes TEXT NOT NULL DEFAULT '',
    superseded_by TEXT NOT NULL DEFAULT '',
    withdrawn_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    CONSTRAINT ask_task_items_run_fk FOREIGN KEY (run_id) REFERENCES ask_runs (id),
    CONSTRAINT ask_task_items_outcome_check CHECK (outcome IN ('pending', 'needs_input', 'evidence_ready', 'answered', 'preview_provided', 'incomplete', 'superseded')),
    CONSTRAINT ask_task_items_revision_check CHECK (item_revision > 0)
);

CREATE INDEX idx_ask_task_items_run ON ask_task_items (run_id, outcome);
CREATE INDEX idx_ask_task_items_origin_turn ON ask_task_items (origin_turn_id, created_at);
