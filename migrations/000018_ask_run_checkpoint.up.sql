-- T2：调度、预算与恢复基础 —— 检查点列。
-- 依据 v2 架构文档 3.4.9：Run 从 running 回到 queued 仅用于有明确原因的调度、
-- 退避或恢复，必须保存检查点和原因，不能重新从头执行已成功步骤。
-- 本迁移给 ask_runs 补 checkpoint 列，供退避/恢复时写入进度快照；
-- 原因写入已存在的 termination_reason 列（见 recovery.go 的原因码常量）。

ALTER TABLE ask_runs ADD COLUMN checkpoint TEXT NOT NULL DEFAULT '';
