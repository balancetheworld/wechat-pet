package ask

import "fmt"

// 本文件固定追问、回复与有限答复的领域契约（文档 2.5）。
// 追问是独立 Message，不能因为用户正常回答就被当成“错误回答版本”；
// 问题是否仍待回复，由问题标识、回复关联和 Run 状态共同表达。
// 缺失信息状态与必要性类别只记简短类别，不保存推理过程。

// QuestionStatus 是追问的持久化状态（文档 2.5 问题身份 / 恢复处理）。
type QuestionStatus string

const (
	QuestionOpen       QuestionStatus = "open"       // 待回复
	QuestionAnswered   QuestionStatus = "answered"   // 已收到有效回复
	QuestionSuperseded QuestionStatus = "superseded" // 被补问的新问题替代（原文不覆盖）
	QuestionClosed     QuestionStatus = "closed"     // Run 已结束/终止，不再等待回复
)

func (s QuestionStatus) Valid() bool {
	switch s {
	case QuestionOpen, QuestionAnswered, QuestionSuperseded, QuestionClosed:
		return true
	default:
		return false
	}
}

// MissingInfoStatus 是待补信息的回答状态（文档 2.5 待补信息行）。
// 未知或省略不能转换为“没有症状”；拒答不等于没有症状，也不是写入授权。
type MissingInfoStatus string

const (
	MissingUnanswered  MissingInfoStatus = "unanswered"  // 未回答
	MissingKnown       MissingInfoStatus = "known"       // 已有有效值
	MissingUnknown     MissingInfoStatus = "unknown"     // 用户明确未知
	MissingRefused     MissingInfoStatus = "refused"     // 拒绝提供
	MissingConflicting MissingInfoStatus = "conflicting" // 矛盾待澄清
)

func (s MissingInfoStatus) Valid() bool {
	switch s {
	case MissingUnanswered, MissingKnown, MissingUnknown, MissingRefused, MissingConflicting:
		return true
	default:
		return false
	}
}

// Necessity 是缺失信息的必要性类别（文档 2.5：只记简短类别）。
type Necessity string

const (
	NecessityBlocking Necessity = "blocking" // 阻止当前目标安全推进
	NecessitySupport  Necessity = "support"  // 辅助但不阻塞
)

func (n Necessity) Valid() bool {
	switch n {
	case NecessityBlocking, NecessitySupport:
		return true
	default:
		return false
	}
}

// MissingItem 是一项已持久化的待补信息（文档 2.5 待补信息行）。
// 关联任务项及对象、缺失字段或信息类型、必要性类别、已知值与来源。
type MissingItem struct {
	TaskItemID string
	SubjectKey string
	Field      string
	InfoType   string
	Necessity  Necessity
	KnownValue string
	Source     string
	Status     MissingInfoStatus
}

// Question 是一次正式发布的追问（文档 2.5 问题身份行）。
// 每次正式发布的追问消息有独立标识；补问创建新问题并关联旧问题，原文不覆盖。
type Question struct {
	QuestionID    string
	RunID         string
	OriginTurnID  string // 发布追问时的 Turn
	MessageID     string // 发布的消息
	InputRevision int    // 发布时的输入版本
	Text          string
	MissingItems  []MissingItem
	Status        QuestionStatus
}

// Reply 是已接收的追问回复（文档 2.5 回复输入/接收行）。
// 接收成功创建新 Turn、关联原 Run 并递增输入版本；第一条回复使 waiting_input -> queued。
type Reply struct {
	QuestionID     string
	RunID          string
	TurnID         string // 接收后由服务端创建的新 Turn
	InputRevision  int    // 接收后递增的输入版本
	Text           string
	ImageIDs       []string
	OptionKeys     []string
	IdempotencyKey string
}

// ValidateQuestion 校验追问契约（文档 2.5 问题身份 / 发布）。
func ValidateQuestion(q Question) error {
	if q.QuestionID == "" {
		return fmt.Errorf("question: empty question_id")
	}
	if q.RunID == "" {
		return fmt.Errorf("question %q: empty run_id", q.QuestionID)
	}
	if q.OriginTurnID == "" {
		return fmt.Errorf("question %q: empty origin_turn_id", q.QuestionID)
	}
	if q.InputRevision <= 0 {
		return fmt.Errorf("question %q: input_revision must be positive", q.QuestionID)
	}
	if !q.Status.Valid() {
		return fmt.Errorf("question %q: invalid status %q", q.QuestionID, q.Status)
	}
	if q.Text == "" && len(q.MissingItems) == 0 {
		return fmt.Errorf("question %q: empty text and missing items", q.QuestionID)
	}
	for _, it := range q.MissingItems {
		if err := validateMissingItem(it); err != nil {
			return fmt.Errorf("question %q: %w", q.QuestionID, err)
		}
	}
	return nil
}

func validateMissingItem(it MissingItem) error {
	if it.TaskItemID == "" {
		return fmt.Errorf("missing_item has empty task_item_id")
	}
	if it.Field == "" {
		return fmt.Errorf("missing_item has empty field")
	}
	if !it.Necessity.Valid() {
		return fmt.Errorf("missing_item has invalid necessity %q", it.Necessity)
	}
	if !it.Status.Valid() {
		return fmt.Errorf("missing_item has invalid status %q", it.Status)
	}
	return nil
}

// ValidateReply 校验回复输入（文档 2.5 回复输入行）。
// 只校验归属、目标、消息结构及资源权限，不在接收事务中要求模型先判定“回答正确”。
func ValidateReply(r Reply) error {
	if r.QuestionID == "" {
		return fmt.Errorf("reply: empty question_id")
	}
	if r.RunID == "" {
		return fmt.Errorf("reply: empty run_id")
	}
	if r.IdempotencyKey == "" {
		return fmt.Errorf("reply: empty idempotency_key")
	}
	if r.Text == "" && len(r.ImageIDs) == 0 && len(r.OptionKeys) == 0 {
		return fmt.Errorf("reply: empty content")
	}
	return nil
}

// IsStaleReply 报告回复是否为迟到/旧目标回复（文档 2.5 并发与迟到回复行）。
// 新问题已发布或 Run 已终止时拒绝旧目标回复，不把迟到消息改投新问题。
func IsStaleReply(r Reply, current Question) bool {
	if r.QuestionID != current.QuestionID {
		return true
	}
	return current.Status != QuestionOpen
}

// RemainingToAsk 返回仍需追问的必要缺失项（文档 2.5 部分回答合并行）。
// 只追问尚未回答（unanswered）或矛盾待澄清（conflicting）且确有必要的 blocking 项；
// 已 known/unknown/refused 的项不再重复索要。
func RemainingToAsk(items []MissingItem) []MissingItem {
	out := make([]MissingItem, 0)
	for _, it := range items {
		if it.Necessity != NecessityBlocking {
			continue
		}
		if it.Status == MissingUnanswered || it.Status == MissingConflicting {
			out = append(out, it)
		}
	}
	return out
}

// DefinitiveMissing 返回关键资料明确缺失的项（文档 2.5 有限答复收尾）。
// 用户明确未知（unknown）或拒绝提供（refused）关键资料时，不再重复索要，
// 按现有信息给出有限答复；未知信息不能当作症状不存在或风险较低的依据。
func DefinitiveMissing(items []MissingItem) []MissingItem {
	out := make([]MissingItem, 0)
	for _, it := range items {
		if it.Necessity != NecessityBlocking {
			continue
		}
		if it.Status == MissingUnknown || it.Status == MissingRefused {
			out = append(out, it)
		}
	}
	return out
}

// ShouldConcludeLimited 报告本次回复后是否应给出有限答复收尾（文档 2.5）。
// 存在关键资料明确缺失（unknown/refused）即应收尾，不猜测身份、不执行缺参数写入。
func ShouldConcludeLimited(items []MissingItem) bool {
	return len(DefinitiveMissing(items)) > 0
}

// HasRepeatedAsk 报告 next 是否换措辞重复询问已有关键答案的资料（文档 2.5 重复追问处理）。
// 依据任务项、对象、缺失信息类型及来源判断是否重复，不能只比较问题文字；
// 已有有效值（known）或用户明确未知（unknown）/拒答（refused）的同一资料应被阻断。
func HasRepeatedAsk(prev, next Question) bool {
	settled := make(map[string]bool)
	for _, it := range prev.MissingItems {
		if it.Status == MissingKnown || it.Status == MissingUnknown || it.Status == MissingRefused {
			settled[missingItemKey(it)] = true
		}
	}
	for _, it := range next.MissingItems {
		if settled[missingItemKey(it)] {
			return true
		}
	}
	return false
}

// missingItemKey 用任务项 + 对象 + 字段 + 信息类型唯一定位一项待补信息。
func missingItemKey(it MissingItem) string {
	return it.TaskItemID + "|" + it.SubjectKey + "|" + it.Field + "|" + it.InfoType
}
