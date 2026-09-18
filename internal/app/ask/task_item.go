package ask

import "fmt"

// 本文件固定任务项领域模型与结果分类（文档 2.3、2.3.1、2.3.2）。
// 任务项表达用户要得到的结果，不把每次模型调用、检索或工具执行都拆成一个任务项。
// 它保存覆盖信息，不创建一套可调度的 Run，也不是主动随访任务。
// 任务项不能扩展为主动随访任务；不保存推理过程，也不为完整计划增加独立模型调用。

// TaskOutcome 是任务项结果分类（文档 2.3.2）。
// 这些分类用于记录覆盖情况，不新增独立 Worker、队列、预算或可自行恢复的任务项状态机。
type TaskOutcome string

const (
	OutcomePending         TaskOutcome = "pending"          // 待处理：当前有效输入已识别目标，尚无足够结果
	OutcomeNeedsInput      TaskOutcome = "needs_input"      // 需补信息：缺失信息实质影响对象、操作或判断
	OutcomeEvidenceReady   TaskOutcome = "evidence_ready"   // 依据已就绪：已取得仍有效的工具结果，尚未发布对应答复
	OutcomeAnswered        TaskOutcome = "answered"         // 已回答：对应回答块已通过最终校验并持久化
	OutcomePreviewProvided TaskOutcome = "preview_provided" // 已提供预览：冻结预览真实存在，答复准确指向该 Operation
	OutcomeIncomplete      TaskOutcome = "incomplete"       // 未完成：资料无法取得或能力/权限/预算/故障/停止导致不再处理
	OutcomeSuperseded      TaskOutcome = "superseded"       // 已撤回或替代：有用户有效输入支持目标变更或有可追溯拆分纠正
)

// Valid 报告结果分类是否为契约内合法值。
func (o TaskOutcome) Valid() bool {
	switch o {
	case OutcomePending, OutcomeNeedsInput, OutcomeEvidenceReady,
		OutcomeAnswered, OutcomePreviewProvided, OutcomeIncomplete, OutcomeSuperseded:
		return true
	default:
		return false
	}
}

// Terminal 报告该分类是否表示「本 Run 已对此任务项给出交付或明确收尾」。
// answered / preview_provided / incomplete / superseded 都属于不再需要本 Run 继续推进的终态；
// pending / needs_input / evidence_ready 仍需要后续动作。
func (o TaskOutcome) Terminal() bool {
	switch o {
	case OutcomeAnswered, OutcomePreviewProvided, OutcomeIncomplete, OutcomeSuperseded:
		return true
	default:
		return false
	}
}

// ResultRefKind 区分任务项结果关联的对象类型（文档 2.3.1）。
type ResultRefKind string

const (
	ResultRefAnswerGroup ResultRefKind = "answer_group" // 回答内容块
	ResultRefToolResult  ResultRefKind = "tool_result"  // 实际采用的工具结果
	ResultRefOperation   ResultRefKind = "operation"    // 独立授权的 Operation
)

func (k ResultRefKind) Valid() bool {
	switch k {
	case ResultRefAnswerGroup, ResultRefToolResult, ResultRefOperation:
		return true
	default:
		return false
	}
}

// TaskResultRef 是任务项与真实对象状态的关联（文档 2.3.1）。
// 引用这些对象的真实状态与版本，不复制一个可被模型任意改写的成功标志。
type TaskResultRef struct {
	Kind    ResultRefKind `json:"kind"`
	RefID   string        `json:"ref_id"`
	Version string        `json:"version"`
}

// TaskItem 是一个已持久化的任务项（文档 2.3.1）。
// 标识由服务端分配，执行结果按 Run 及任务项版本保存，手动重试不覆盖旧 Run 的记录。
type TaskItem struct {
	TaskItemID       string          // 服务端分配的稳定标识
	OriginTurnID     string          // 首次识别该目标的 Turn
	RunID            string          // 所属 Run
	ItemRevision     int             // 任务项版本，服务端分配，随目标更新递增
	Goal             string          // 简短目标
	SourceTurnIDs    []string        // 输入依据，只取当前 Run 已消费输入
	Subjects         []AnswerSubject // 已解析对象或尚待明确的对象描述
	Outcome          TaskOutcome     // 当前结果分类
	MissingFields    []MissingField  // 需补信息及对应问题（OutcomeNeedsInput 时非空）
	IncompleteReason string          // 未完成原因（OutcomeIncomplete 时非空）
	ResultRef        *TaskResultRef  // 结果关联（answered/preview_provided 时非空）
	Supersedes       string          // 替代关系：被本项替代的旧 task_item_id
	SupersededBy     string          // 替代关系：替代本项的新 task_item_id
	WithdrawnReason  string          // 撤回依据
}

// CanStartTool 报告该任务项能否启动依赖它的工具（文档 2.3.1、2.3.3）。
// 对象未解析（尚待明确）时不能启动，需先追问；不能猜测 pet_id 或生成可执行写入。
func (i TaskItem) CanStartTool() bool {
	for _, s := range i.Subjects {
		if s.Kind == SubjectUnresolved {
			return false
		}
	}
	return true
}

// HasSubject 报告该任务项是否涉及给定对象键（用于校验工具的归属）。
func (i TaskItem) HasSubject(subjectKey string) bool {
	for _, s := range i.Subjects {
		if s.SubjectKey == subjectKey {
			return true
		}
	}
	return false
}

// ValidateTaskItem 校验一个任务项的契约约束（文档 2.3.1）。
// 关键：不得仅凭模型说“完成”更新为完成；纠正目标必须有依据；
// 对象未知时不填猜测的 pet_id。
func ValidateTaskItem(i TaskItem) error {
	if i.TaskItemID == "" {
		return fmt.Errorf("task_item: empty task_item_id")
	}
	if i.ItemRevision <= 0 {
		return fmt.Errorf("task_item %q: item_revision must be positive", i.TaskItemID)
	}
	if i.Goal == "" {
		return fmt.Errorf("task_item %q: empty goal", i.TaskItemID)
	}
	if !i.Outcome.Valid() {
		return fmt.Errorf("task_item %q: invalid outcome %q", i.TaskItemID, i.Outcome)
	}
	// 对象：已解析宠物必须有 pet_id；待明确对象必须有描述（不填猜测的 pet_id）。
	for _, s := range i.Subjects {
		if s.Kind == SubjectPet && s.PetID == "" {
			return fmt.Errorf("task_item %q: subject %q missing pet_id", i.TaskItemID, s.SubjectKey)
		}
		if s.Kind == SubjectUnresolved && s.Description == "" {
			return fmt.Errorf("task_item %q: subject %q missing description", i.TaskItemID, s.SubjectKey)
		}
	}
	// 按结果分类校验「依据」与「关联」。
	switch i.Outcome {
	case OutcomeAnswered:
		if !i.hasRefOfKind(ResultRefAnswerGroup) {
			return fmt.Errorf("task_item %q: answered requires an answer_group result ref", i.TaskItemID)
		}
	case OutcomePreviewProvided:
		if !i.hasRefOfKind(ResultRefOperation) {
			return fmt.Errorf("task_item %q: preview_provided requires an operation result ref", i.TaskItemID)
		}
	case OutcomeNeedsInput:
		if len(i.MissingFields) == 0 {
			return fmt.Errorf("task_item %q: needs_input requires missing_fields", i.TaskItemID)
		}
	case OutcomeIncomplete:
		if i.IncompleteReason == "" {
			return fmt.Errorf("task_item %q: incomplete requires a reason", i.TaskItemID)
		}
	case OutcomeSuperseded:
		// 纠正目标必须有依据：替代或撤回关系必须可追溯（文档 2.3.1 修改依据）。
		if i.Supersedes == "" && i.SupersededBy == "" && i.WithdrawnReason == "" {
			return fmt.Errorf("task_item %q: superseded requires a traceable basis (supersedes/superseded_by/withdrawn_reason)", i.TaskItemID)
		}
	}
	return nil
}

// hasRefOfKind 报告结果关联是否存在且类型匹配、引用非空。
func (i TaskItem) hasRefOfKind(kind ResultRefKind) bool {
	if i.ResultRef == nil {
		return false
	}
	if i.ResultRef.Kind != kind {
		return false
	}
	if i.ResultRef.RefID == "" {
		return false
	}
	return true
}
