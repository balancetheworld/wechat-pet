package ask

// 本文件固定首次业务决策的纯判定（文档 2.3、2.3.1、2.3.3）。
// 已确认：按步骤选择下一动作，不强制每次先生成完整执行计划；
// 无需工具的简单问题可以直接回答；多项请求也不因存在任务项而强制采用某条固定工具链。
// 首次能完整识别任务项的模型响应可同时携带下一动作，不必先单独提交清单再调用模型。

// FirstStepKind 是首次响应的业务形态（文档 2.3）。
type FirstStepKind string

const (
	StepSimpleAnswer  FirstStepKind = "simple_answer"   // 无需工具，直接回答
	StepDirectTool    FirstStepKind = "direct_tool"     // 直接工具，无需显式任务清单
	StepToolWithTasks FirstStepKind = "tool_with_tasks" // 完整识别任务项并同时携带下一动作
	StepNeedClarify   FirstStepKind = "need_clarify"    // 对象未解析，先追问，不启动依赖工具
	StepTasksOnly     FirstStepKind = "tasks_only"      // 仅任务清单，待后续决策
)

// FirstDecision 是首次业务决策结果。
type FirstDecision struct {
	Kind   FirstStepKind
	Reason string
}

// ClassifyFirstResponse 判定首次响应的业务形态（文档 2.3）。
// 输入：已识别任务项与本次携带的工具调用数。
// 输出：首次决策形态与依据。
func ClassifyFirstResponse(items []TaskItem, toolCalls int) FirstDecision {
	switch {
	case toolCalls == 0 && len(items) == 0:
		return FirstDecision{Kind: StepSimpleAnswer, Reason: "no tasks and no tools, answer directly"}
	case toolCalls > 0 && len(items) == 0:
		return FirstDecision{Kind: StepDirectTool, Reason: "tool call without an explicit task list"}
	case toolCalls > 0 && len(items) > 0:
		// 首次能完整识别任务项时可同时携带下一动作；对象未解析则先追问。
		if err := ValidateToolReadiness(items); err != nil {
			return FirstDecision{Kind: StepNeedClarify, Reason: err.Error()}
		}
		return FirstDecision{Kind: StepToolWithTasks, Reason: "fully identified tasks carried with next action"}
	default: // toolCalls == 0 && len(items) > 0
		if err := ValidateToolReadiness(items); err != nil {
			return FirstDecision{Kind: StepNeedClarify, Reason: err.Error()}
		}
		return FirstDecision{Kind: StepTasksOnly, Reason: "task list only, awaiting later decision"}
	}
}

// ValidateToolReadiness 校验一批任务项是否具备启动工具的条件（文档 2.3.1、2.3.3）。
// 只先执行权限、对象和参数已明确的步骤，不猜测缺失信息；
// 对象未解析的任务项不能启动依赖它的工具，需先追问。
func ValidateToolReadiness(items []TaskItem) error {
	for _, it := range items {
		if !it.CanStartTool() {
			return &TaskNotReadyError{TaskItemID: it.TaskItemID}
		}
	}
	return nil
}

// TaskNotReadyError 表示某任务项因对象未解析而不能启动工具。
type TaskNotReadyError struct {
	TaskItemID string
}

func (e *TaskNotReadyError) Error() string {
	return "first_decision: task " + e.TaskItemID + " has unresolved subject, clarify before tool call"
}
