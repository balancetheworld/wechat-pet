package ask

// 本文件固定 v2 领域契约（对应架构设计 v2 文档 3.5、9.1、2.5）。
// 只定义命令/结果/错误 Schema 的类型与稳定常量，不承载业务执行逻辑；
// HTTP 层接入与具体路由在 P8 完成，业务执行在 P2–P7 完成。

// CommandType：领域命令类型。输入命令区分 new_task / supplement / reply，
// Run 控制为 stop / retry，Operation 控制为 confirm / withdraw / abandon。
type CommandType string

const (
	CommandNewTask    CommandType = "new_task"   // 新任务：新建后续 Run
	CommandSupplement CommandType = "supplement" // 补充当前任务：指定原 Run
	CommandReply      CommandType = "reply"      // 追问回复：指定 Run + 问题标识
	CommandStop       CommandType = "stop"       // 主动停止目标 Run
	CommandRetry      CommandType = "retry"      // 手动重试：关联原任务，新建 Run 与回答版本
	CommandConfirm    CommandType = "confirm"    // 确认 Operation 写入
	CommandWithdraw   CommandType = "withdraw"   // 撤回已确认、尚未领取执行的 Operation
	CommandAbandon    CommandType = "abandon"    // 放弃待确认的冻结预览
)

// IsInputCommand 判断是否属于聊天输入类命令（会创建 Turn）。
func (c CommandType) IsInputCommand() bool {
	return c == CommandNewTask || c == CommandSupplement || c == CommandReply
}

// IsRunControl 判断是否属于 Run 控制命令（不创建 Turn）。
func (c CommandType) IsRunControl() bool {
	return c == CommandStop || c == CommandRetry
}

// IsOperationControl 判断是否属于 Operation 控制命令（不创建 Turn）。
func (c CommandType) IsOperationControl() bool {
	return c == CommandConfirm || c == CommandWithdraw || c == CommandAbandon
}

// Valid 判断命令类型是否为契约内合法值。
func (c CommandType) Valid() bool {
	switch c {
	case CommandNewTask, CommandSupplement, CommandReply,
		CommandStop, CommandRetry,
		CommandConfirm, CommandWithdraw, CommandAbandon:
		return true
	default:
		return false
	}
}

// InputCommand：聊天输入命令（new_task / supplement / reply）的必需输入。
// 对应 9.1「启动与输入」边界：用户及家庭由鉴权注入，不在此处作为可填字段。
type InputCommand struct {
	Type                      CommandType
	SessionID                 string   // 目标 Session；new_task 为空表示新建当前 Session
	RunID                     string   // supplement 指定原 Run；reply 指定问题所属 Run
	QuestionID                string   // reply 的问题标识（2.5、3.5）
	LaunchInstance            string   // 启动实例（3.5）
	ExpectedSessionGeneration int      // 预期会话代次（3.5）
	Message                   string   // 实际消息文本
	AssetRefs                 []string // 受控资产引用（图片等）
	IdempotencyKey            string
}

// RunControlCommand：Run 控制命令（stop / retry）的必需输入。
type RunControlCommand struct {
	Type            CommandType
	SessionID       string
	RunID           string
	ExpectedVersion int // 目标 Run 的状态版本
	IdempotencyKey  string
}

// OperationCommand：Operation 控制命令（confirm / withdraw / abandon）的必需输入。
type OperationCommand struct {
	Type                      CommandType
	OperationID               string
	ExpectedVersion           int    // Operation 状态版本
	PreviewVersion            int    // confirm 携带的预览版本
	FrozenSummary             string // confirm 携带的冻结内容摘要（9.1）
	ExpectedSessionGeneration int    // confirm 携带的当前 Session 代次（9.1）
	IdempotencyKey            string
}

// CommandResult：统一命令结果（9.1 共同结果）。
// 包含稳定对象身份、状态与版本、命令是否受理、允许公开的失败原因。
// 可重试性由 Runtime 根据当前条件判定，客户端与模型均不能以标志绕过授权，
// 因此不作为对外字段。
type CommandResult struct {
	Accepted bool   // 命令是否受理
	ObjectID string // 稳定对象身份（turn_id / run_id / operation_id）
	Status   string // 对象当前状态
	Version  int    // 对象状态版本
	Reason   string // 允许公开的失败原因（稳定 code）
}

// CommandReason：领域命令的稳定失败原因，用于跨版本客户端识别，
// 对应 3.5「状态冲突提供稳定原因和允许读取的最新状态」。
type CommandReason string

const (
	ReasonSessionClosed       CommandReason = "ask.session.closed"       // 会话已关闭，拒绝回复/继续/确认
	ReasonRunConflict         CommandReason = "ask.run.conflict"         // Run 状态冲突
	ReasonObjectMismatch      CommandReason = "ask.object.mismatch"      // 对象归属不符（错绑家庭/用户）
	ReasonIdempotencyConflict CommandReason = "ask.idempotency.conflict" // 同键不同请求
	ReasonQueueFull           CommandReason = "ask.queue.full"           // 队列满
	ReasonTargetConflict      CommandReason = "ask.target.conflict"      // 命令目标冲突（如 supplement 指向已完成 Run）
	ReasonAssetNotReady       CommandReason = "ask.asset.not_ready"      // 受控资产未就绪
	ReasonPreviewInvalid      CommandReason = "ask.preview.invalid"      // 预览失效或版本不符
	ReasonVersionConflict     CommandReason = "ask.version.conflict"     // 预期版本冲突
)
