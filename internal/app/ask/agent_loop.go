package ask

import (
	"context"
	"fmt"
)

// 本文件固定 v2 单 Agent 决策循环的主循环编排（文档 2、7、8、9）。
// 循环：组装上下文 -> 业务决策(agent_step) -> 解析 record_array_v1 -> 按动作分发
// -> 工具执行与结果回灌 -> 再次决策，直到 final_answer / request_input 或步骤预算耗尽。
// 模型、工具执行通过端口接口注入，编排本身不依赖具体 Provider 或数据层实现。

// AgentModel 是决策循环依赖的模型端口（文档 4.1 agent_step 用途）。
// 调用方是 Runtime；输入是已组装并筛选的上下文与候选工具，输出是一次
// record_array_v1 响应的完整记录。
type AgentModel interface {
	Step(ctx context.Context, input StepInput) ([]ProtocolRecord, error)
}

// ToolExecutor 是决策循环依赖的工具执行端口（文档 7 节）。
// 整批校验、只读并行、局部失败与结果封装由实现负责；返回逐项结果。
type ToolExecutor interface {
	ExecuteBatch(ctx context.Context, batch ToolBatch) ([]ToolResult, error)
}

// StepInput 是一次 agent_step 决策的输入（文档 4.1、5.8）。
type StepInput struct {
	Blocks []ContextBlock // 已组装、去重、裁剪的上下文
	Tools  []Tool         // 候选工具（已过滤）；无工具时传空集合
}

// LoopOutcome 是决策循环的最终结果（文档 2.2）。
type LoopOutcome struct {
	Action      ResponseAction // final_answer 或 request_input
	Groups      []AnswerGroup  // final_answer 的回答组
	Questions   []Question     // request_input 的追问
	TaskItems   []TaskItem     // 最终任务项快照
	ToolResults []ToolResult   // 循环中累积的真实工具结果
	Steps       int            // 已消耗的决策步数
}

// maxDecisionSteps 是单 Run 决策循环的保守默认步数上限（文档 9.3「有限模型调用总数」）。
// 实际值由实施 Profile 固定；此处给出保守默认，防止无限循环。
const maxDecisionSteps = 16

// RunDecisionLoop 执行 v2 单 Agent 决策循环（文档 2、7、8、9）。
// maxSteps <= 0 时使用保守默认值。循环每步：
//   1. 调用模型得到 record_array_v1 记录；
//   2. DecideStep 冻结动作并分发；
//   3. call_tools -> 构造批次、执行工具、结果回灌后继续；
//      request_input / final_answer -> 返回最终处置。
func RunDecisionLoop(ctx context.Context, model AgentModel, tools ToolExecutor, initial StepInput, maxSteps int) (LoopOutcome, error) {
	if maxSteps <= 0 {
		maxSteps = maxDecisionSteps
	}
	blocks := append([]ContextBlock(nil), initial.Blocks...)
	taskItems := []TaskItem{}
	toolResults := []ToolResult{}
	outcome := LoopOutcome{}

	for step := 0; step < maxSteps; step++ {
		records, err := model.Step(ctx, StepInput{Blocks: blocks, Tools: initial.Tools})
		if err != nil {
			return outcome, err
		}
		decision, err := DecideStep(records)
		if err != nil {
			return outcome, err
		}
		// 合并本轮任务项更新（header.task_updates 映射为待分配稳定 ID 的任务项）。
		if len(decision.TaskUpdates) > 0 {
			mapped, mapErr := MapTaskUpdates(decision.TaskUpdates, "", "")
			if mapErr != nil {
				return outcome, mapErr
			}
			taskItems = mergeTaskItems(taskItems, mapped)
		}

		outcome.Steps = step + 1
		switch decision.Action {
		case ActionCallTools:
			batch := BuildToolBatch(decision.Calls, "", "", "")
			results, execErr := tools.ExecuteBatch(ctx, batch)
			if execErr != nil {
				return outcome, execErr
			}
			toolResults = append(toolResults, results...)
			// 结果回灌：真实工具结果作为低信任参考数据进入下一轮上下文。
			for _, r := range results {
				blocks = append(blocks, toolResultBlock(r))
			}
			// 按 coverage 记录未完成项，继续下一轮决策。
			taskItems = ResolveCoverage(taskItems, decision.Coverage)
			continue

		case ActionRequestInput:
			// 把被追问的任务项标记为「需补信息」并携带缺失字段（文档 2.5、2.3.2）。
			taskItems = ResolveQuestionCoverage(taskItems, decision.Questions)
			outcome.Action = ActionRequestInput
			outcome.Questions = BuildQuestions(decision.Questions)
			outcome.TaskItems = taskItems
			outcome.ToolResults = toolResults
			return outcome, nil

		case ActionFinalAnswer:
			// 先按 coverage + 真实回答组裁决任务项终态，再做回答组校验与覆盖闭合检查
			// （文档 2.3.2、8.5：final_answer 必须逐项交代结果或受阻原因，不能遗留无说明的待处理目标）。
			taskItems = ResolveFinalCoverage(taskItems, decision.Coverage, decision.Groups)
			if err := ValidateFinalAnswer(decision, taskItems); err != nil {
				return outcome, err
			}
			outcome.Action = ActionFinalAnswer
			outcome.Groups = decision.Groups
			outcome.TaskItems = taskItems
			outcome.ToolResults = toolResults
			return outcome, nil

		default:
			return outcome, fmt.Errorf("agent_loop: unhandled action %q", decision.Action)
		}
	}
	return outcome, fmt.Errorf("agent_loop: exceeded %d decision steps without terminal action", maxSteps)
}

// toolResultBlock 把一项真实工具结果转换为回灌上下文块（文档 5.8 第 5 层）。
// 工具结果是有来源的低信任参考数据，不获得指令权限。
func toolResultBlock(r ToolResult) ContextBlock {
	return ContextBlock{
		Layer:    LayerToolInteractions,
		Kind:     "tool_result",
		ObjectID: r.ToolCallID,
		Version:  "",
		Position: "",
		Text:     toolResultText(r),
	}
}

// toolResultText 生成工具结果的受控文本（文档 7.3）。
// 只携带可公开的状态与数据摘要，不暴露内部 ID 或敏感字段。
func toolResultText(r ToolResult) string {
	if r.Error != nil {
		return fmt.Sprintf("[tool %s] error: %s", r.ToolCallID, r.Error.Reason)
	}
	return fmt.Sprintf("[tool %s] status=%s completeness=%s", r.ToolCallID, r.Status, r.Completeness)
}

// mergeTaskItems 合并任务项：同 TaskItemID 保留最新（本轮映射覆盖），新增项追加。
// 稳定 ID 的分配由持久化层负责；此处仅做内存合并。
func mergeTaskItems(existing, mapped []TaskItem) []TaskItem {
	result := append([]TaskItem(nil), existing...)
	for _, item := range mapped {
		replaced := false
		for i := range result {
			if result[i].TaskItemID == item.TaskItemID {
				result[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, item)
		}
	}
	return result
}
