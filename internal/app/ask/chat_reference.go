package ask

import (
	"context"
	"time"
)

// 本文件固定旧聊天只读引用的执行契约（对应架构设计 v2 文档 5.6）。
// 旧聊天引用属于当前 Run 的只读工具步骤：不新增 Run 状态、不恢复来源 Session，
// 检索范围限最近三次 Session（含当前），读取故障与无命中始终分开。
// 当前 Service 尚未注入该端口，属规划中能力：接线见
// docs/pet-ask-agent-refactor-plan.md P5（T5），契约依据 docs/pet-ask-agent-architecture-v2.md 5.6。

// maxReferenceSessions：允许引用的最近 Session 数（含当前）。
const maxReferenceSessions = 3

// ChatReferenceStatus：引用检索结果状态。与 Run、Attempt 状态完全分开，
// 不以空数组同时表达「没有命中」和「读取故障」。
type ChatReferenceStatus string

const (
	ChatReferenceOK           ChatReferenceStatus = "ok"           // 找到命中片段
	ChatReferenceNotFound     ChatReferenceStatus = "not_found"    // 允许范围内无命中（区别于故障）
	ChatReferenceAmbiguous    ChatReferenceStatus = "ambiguous"    // 多个候选且无法确定用户所指
	ChatReferenceUnauthorized ChatReferenceStatus = "unauthorized" // 范围外、已删除或无权读取
	ChatReferenceError        ChatReferenceStatus = "error"        // 读取超时或服务故障
)

// ChatReferenceQuery：旧聊天引用定位条件。用户与家庭由鉴权注入，
// 客户端传入的消息 ID 不代表已有读取权限；模型提出检索条件也不能扩大范围。
type ChatReferenceQuery struct {
	FamilyID         string   // 鉴权注入：当前家庭
	UserID           string   // 鉴权注入：本人
	CurrentSessionID string   // 当前 Session，用于确定「最近三次」范围
	PetID            string   // 定位宠物（空表示不限，仅作定位线索）
	Keywords         []string // 定位话题关键词（空表示不限）
	StartAt          time.Time
	EndAt            time.Time
	MessageIDs       []string // 客户端声称的来源消息 ID（仅作定位线索，不构成授权）
	Limit            int      // 返回片段上限，<=0 取默认
}

// ChatReferenceMatch：单条命中，保留来源与状态，不整段当作某一只宠物的病史。
type ChatReferenceMatch struct {
	SessionID     string    // 来源 Session
	MessageID     string    // 来源 Message
	RunID         string    // 来源 Run（可为空）
	Role          string    // 发言者（user / assistant / question）
	AnswerVersion int       // 回答版本（assistant 消息对应 run.row_version）
	Content       string    // 原文片段
	OccurredAt    time.Time // 原始时间
	Complete      bool      // 片段是否完整
	Superseded    bool      // 是否属于被替代版本
}

// ChatReferenceOutcome：引用检索结果。
// Scope 记录本次允许读取的 Session 集合；Truncated 标记因上限被截断。
type ChatReferenceOutcome struct {
	Status    ChatReferenceStatus
	Scope     []string
	Matches   []ChatReferenceMatch
	Truncated bool
	Reason    string // 允许公开的原因
	Source    ReadSource
}

// ChatReferenceRepository：旧聊天只读引用端口。T5 决策循环通过它读取
// 明确授权范围内的旧聊天片段，不直接触碰 ask 消息数据层。
type ChatReferenceRepository interface {
	SearchReferences(context.Context, ChatReferenceQuery) (ChatReferenceOutcome, error)
}

// ReferenceScope 计算允许引用的 Session 集合：按创建时间倒序取最近三次，
// 当前 Session 始终包含在范围内。输入 sessions 需已按 created_at 倒序。
func ReferenceScope(currentSessionID string, sessions []Session) []string {
	result := make([]string, 0, maxReferenceSessions)
	hasCurrent := false
	for _, session := range sessions {
		if len(result) >= maxReferenceSessions {
			break
		}
		result = append(result, session.ID)
		if session.ID == currentSessionID {
			hasCurrent = true
		}
	}
	if !hasCurrent && currentSessionID != "" {
		result = append(result, currentSessionID)
	}
	return result
}

// referenceFilter：数据层消息检索的原始过滤条件（时间边界）。
type referenceFilter struct {
	StartAt time.Time
	EndAt   time.Time
}
