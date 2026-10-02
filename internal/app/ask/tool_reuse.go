package ask

// 本文件固定只读业务查询结果的复用判定（对应架构设计 v2 文档 7.7）。
// 只读结果允许在同一 Session 的不同 Run 间复用，每次重新核验权限、
// 查询范围、来源及集合版本；无法证明仍有效就重查。
// 重复调用的阻断判定由 loop_guard.go 的 LoopGuard 承担（文档 7.4）。

// ReuseDecision：复用判定处置（7.7.1 输出）。
type ReuseDecision string

const (
	ReuseReusable    ReuseDecision = "reusable"    // 可复用既有结果
	ReuseReread      ReuseDecision = "reread"      // 需重新读取
	ReuseUnavailable ReuseDecision = "unavailable" // 无法继续读取（无权/过期）
)

// ReuseCandidate：待复用的候选结果。复用对象是已通过工具输出校验的业务数据，
// 不是旧模型回答、写入授权或笼统的「上次任务成功」。
type ReuseCandidate struct {
	SessionID   string
	RunID       string
	ToolCallID  string
	ToolName    string
	ToolVersion string
	Fingerprint string
	Result      ToolResult
}

// ReuseInput：复用核验输入（7.7.1 表格）。权限、来源与集合版本由调用方
// 在执行前真实核验后传入，本判定只承载决策规则。
type ReuseInput struct {
	SessionID        string
	ToolName         string
	ToolVersion      string
	Fingerprint      string
	Candidate        ReuseCandidate
	Permitted        bool // 当前对象、字段、结果引用及派生来源均可读取
	SourceStillValid bool // 单条未变，且集合未发生影响结果的变更
}

// DecideReuse 判定候选结果能否复用。跨 Session 候选不使用；契约不匹配、
// 指纹变化、权限失效或来源变化分别处置。
func DecideReuse(input ReuseInput) ReuseDecision {
	if input.Candidate.SessionID != input.SessionID {
		return ReuseUnavailable
	}
	if input.Candidate.ToolName != input.ToolName || input.Candidate.ToolVersion != input.ToolVersion {
		return ReuseReread
	}
	if input.Candidate.Fingerprint != input.Fingerprint {
		return ReuseReread
	}
	if !input.Permitted {
		return ReuseUnavailable
	}
	if !input.SourceStillValid {
		return ReuseReread
	}
	return ReuseReusable
}
