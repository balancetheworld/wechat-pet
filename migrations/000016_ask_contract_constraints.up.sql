-- v2 重构：P1 契约约束补丁（对应架构设计 v2 文档 11.3、9.2）。
-- 1) 给需要保留管理的 ask 表补 deleted_at 软删除标记。
--    30/90 天的到期清理、删除传播与备份恢复在 T9 实现；
--    本迁移只建立删除标记，避免后补过滤。
-- 2) 落实 9.2「同一用户 + 家庭至多一个当前 Session」的部分唯一约束。

ALTER TABLE ask_sessions ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE ask_turns ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE ask_runs ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE ask_events ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE ask_messages ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE ask_operations ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE ask_attempts ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE ask_memories ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE ask_idempotency_keys ADD COLUMN deleted_at TIMESTAMP NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_ask_sessions_current_unique
    ON ask_sessions (family_id, created_by)
    WHERE status = 'active' AND deleted_at IS NULL;
