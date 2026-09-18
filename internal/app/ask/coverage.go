package ask

import "fmt"

// 本文件固定 Run 完成时的任务覆盖校验（文档 2.3.2 末尾）。
// 最终输出必须覆盖当前有效清单中的各项：给出对应答复、真实预览，或明确未完成事项及原因。
// Run 完成与本次清单的结果快照一并提交，不能先把所有项标为已回答再发现最终校验失败。
// 工具失败的任务项必须表达失败或局限，不能从任务清单中静默消失。

// UnresolvedItems 返回当前清单中仍需继续推进、尚未形成终态的任务项。
// pending / needs_input / evidence_ready 均属未覆盖；这些项不能随 Run 完成而静默消失。
func UnresolvedItems(items []TaskItem) []TaskItem {
	out := make([]TaskItem, 0)
	for _, it := range items {
		if !it.Outcome.Terminal() {
			out = append(out, it)
		}
	}
	return out
}

// CheckRunClosure 校验 Run 完成时的清单结果快照是否完整覆盖（文档 2.3.2）。
// 规则：
//  1. 不能有 pending / needs_input / evidence_ready 残留项（这些仍需后续动作）；
//  2. 终态项必须携带其分类要求的依据（已由 ValidateTaskItem 保证，此处再次显式核验）。
//
// 返回 nil 表示清单可随 Run 完成一并提交；否则返回首个阻塞错误。
func CheckRunClosure(items []TaskItem) error {
	unresolved := UnresolvedItems(items)
	if len(unresolved) > 0 {
		return fmt.Errorf("coverage: run closure has %d unresolved task(s), first %q is %q",
			len(unresolved), unresolved[0].TaskItemID, unresolved[0].Outcome)
	}
	// 终态项逐一核验依据，防止「先把所有项标为已回答再发现最终校验失败」。
	for _, it := range items {
		if err := ValidateTaskItem(it); err != nil {
			return fmt.Errorf("coverage: invalid terminal task %q: %w", it.TaskItemID, err)
		}
	}
	return nil
}

// CheckTerminalConsistency 校验一个任务项从当前分类迁移到终态分类是否合法（文档 2.3.2）。
// 只允许向更明确的终态推进，不允许把已交付项悄悄回退成 pending 来规避校验。
func CheckTerminalConsistency(from, to TaskOutcome) error {
	if !from.Valid() || !to.Valid() {
		return fmt.Errorf("coverage: invalid outcome transition %q -> %q", from, to)
	}
	// 终态不可回退到未覆盖状态（防止模型通过删除失败项让清单看起来全部完成）。
	if from.Terminal() && !to.Terminal() {
		return fmt.Errorf("coverage: terminal outcome %q cannot regress to %q", from, to)
	}
	return nil
}
