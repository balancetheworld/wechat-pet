package ask

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// 本文件实现旧聊天只读引用的数据层（对应架构设计 v2 文档 5.6）。
// 检索范围限最近三次 Session（含当前），候选仅来自本人与当前家庭；
// 读取故障与无命中分开，客户端传入的消息 ID 不构成读取授权。
// 当前尚无生产调用方，属规划中能力：接线见 docs/pet-ask-agent-refactor-plan.md P5（T5）。

const (
	defaultReferenceLimit = 20
	maxReferenceLimit     = 100
)

// ReferenceMessage：带回答版本与被替代标记的消息（数据层原始检索结果）。
type ReferenceMessage struct {
	MessageID     string
	SessionID     string
	RunID         string
	Role          string
	Content       string
	CreatedAt     time.Time
	AnswerVersion int
	Superseded    bool
}

// chatReferenceRepository 实现 ChatReferenceRepository，复用 ask 消息数据层。
type chatReferenceRepository struct {
	source *SQLRepository
}

// NewChatReferenceRepository 构造旧聊天引用读取实现。
func NewChatReferenceRepository(source *SQLRepository) ChatReferenceRepository {
	return &chatReferenceRepository{source: source}
}

var _ ChatReferenceRepository = (*chatReferenceRepository)(nil)

// SearchReferences 在最近三次 Session 范围内检索旧聊天片段。
func (r *chatReferenceRepository) SearchReferences(ctx context.Context, query ChatReferenceQuery) (ChatReferenceOutcome, error) {
	sessions, err := r.source.ListRecentSessions(ctx, query.FamilyID, query.UserID, maxReferenceSessions)
	if err != nil {
		return ChatReferenceOutcome{Status: ChatReferenceError}, err
	}
	scope := ReferenceScope(query.CurrentSessionID, sessions)
	sessionByID := make(map[string]Session, len(sessions))
	for _, session := range sessions {
		sessionByID[session.ID] = session
	}

	// 宠物定位只作为线索：过滤出匹配主宠物的 Session，不扩大范围。
	allowed := make([]string, 0, len(scope))
	for _, sessionID := range scope {
		session, ok := sessionByID[sessionID]
		if !ok {
			continue
		}
		if query.PetID != "" && session.PetID != "" && session.PetID != query.PetID {
			continue
		}
		allowed = append(allowed, sessionID)
	}

	messages, err := r.source.SearchReferenceMessages(ctx, allowed, referenceFilter{StartAt: query.StartAt, EndAt: query.EndAt})
	if err != nil {
		return ChatReferenceOutcome{Status: ChatReferenceError}, err
	}

	limit := query.Limit
	if limit <= 0 {
		limit = defaultReferenceLimit
	}
	if limit > maxReferenceLimit {
		limit = maxReferenceLimit
	}
	matches := make([]ChatReferenceMatch, 0, len(messages))
	for _, message := range messages {
		if !referenceMatches(message, query) {
			continue
		}
		matches = append(matches, ChatReferenceMatch{
			SessionID:     message.SessionID,
			MessageID:     message.MessageID,
			RunID:         message.RunID,
			Role:          message.Role,
			AnswerVersion: message.AnswerVersion,
			Content:       message.Content,
			OccurredAt:    message.CreatedAt,
			Complete:      true,
			Superseded:    message.Superseded,
		})
	}

	outcome := ChatReferenceOutcome{
		Scope:  allowed,
		Source: ReadSource{SourceType: "chat_reference", SourceID: query.CurrentSessionID, ReadAt: time.Now().UTC()},
	}
	if len(matches) == 0 {
		outcome.Status = ChatReferenceNotFound
		return outcome, nil
	}
	if len(matches) > limit {
		outcome.Truncated = true
		matches = matches[:limit]
	}
	outcome.Status = ChatReferenceOK
	outcome.Matches = matches
	outcome.Source.Version = referenceCollectionVersion(matches)
	r.recordReferenceVersion(ctx, outcome.Source)
	return outcome, nil
}

// referenceMatches 判断消息是否命中定位条件（关键词 + 声称的消息 ID）。
func referenceMatches(message ReferenceMessage, query ChatReferenceQuery) bool {
	if len(query.MessageIDs) > 0 {
		found := false
		for _, id := range query.MessageIDs {
			if id == message.MessageID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(query.Keywords) == 0 {
		return true
	}
	for _, keyword := range query.Keywords {
		if keyword != "" && strings.Contains(message.Content, keyword) {
			return true
		}
	}
	return false
}

// referenceCollectionVersion 计算引用结果集合的稳定版本。
func referenceCollectionVersion(matches []ChatReferenceMatch) string {
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		parts = append(parts, match.SessionID, match.MessageID, match.OccurredAt.UTC().Format(time.RFC3339Nano))
	}
	return hashSourceVersion(parts...)
}

func (r *chatReferenceRepository) recordReferenceVersion(ctx context.Context, source ReadSource) {
	if r.source == nil || source.Version == "" {
		return
	}
	_ = r.source.UpsertSourceVersion(ctx, source.SourceType, source.SourceID, source.Version)
}

// ListRecentSessions 列出本人与当前家庭下最近 limit 个 Session（按创建时间倒序）。
func (r *SQLRepository) ListRecentSessions(ctx context.Context, familyID, userID string, limit int) ([]Session, error) {
	if limit <= 0 {
		limit = maxReferenceSessions
	}
	rows, err := r.db.QueryContext(ctx, r.query("SELECT id, family_id, COALESCE(resolved_pet_id, ''), created_by, status, risk_level, turn_count, prompt_version, rule_version, knowledge_version, created_at, updated_at, completed_at FROM ask_sessions WHERE family_id = ? AND created_by = ? AND deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT ?"), familyID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Session, 0, limit)
	for rows.Next() {
		var value Session
		var completedAt sql.NullTime
		if err := rows.Scan(&value.ID, &value.FamilyID, &value.PetID, &value.CreatedBy, &value.Status, &value.RiskLevel, &value.TurnCount, &value.PromptVersion, &value.RuleVersion, &value.KnowledgeVersion, &value.CreatedAt, &value.UpdatedAt, &completedAt); err != nil {
			return nil, err
		}
		if completedAt.Valid {
			value.CompletedAt = &completedAt.Time
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// SearchReferenceMessages 在允许的 Session 集合内检索消息，附带回答版本与被替代标记。
// 空 sessionIDs 直接返回空结果（不触发查询）。
func (r *SQLRepository) SearchReferenceMessages(ctx context.Context, sessionIDs []string, filter referenceFilter) ([]ReferenceMessage, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(sessionIDs))
	args := make([]any, 0, len(sessionIDs)+2)
	for index, id := range sessionIDs {
		placeholders[index] = "?"
		args = append(args, id)
	}
	condition := "m.session_id IN (" + strings.Join(placeholders, ", ") + ") AND m.deleted_at IS NULL AND r.status = 'completed'"
	if !filter.StartAt.IsZero() {
		condition += " AND m.created_at >= ?"
		args = append(args, filter.StartAt)
	}
	if !filter.EndAt.IsZero() {
		condition += " AND m.created_at < ?"
		args = append(args, filter.EndAt)
	}
	query := r.query("SELECT m.id, m.session_id, m.run_id, m.role, m.content, m.created_at, COALESCE(r.row_version, 0), COALESCE(t.superseded_by, '') FROM ask_messages m LEFT JOIN ask_runs r ON r.id = m.run_id LEFT JOIN ask_turns t ON t.id = m.turn_id WHERE " + condition + " ORDER BY m.created_at, m.id")
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ReferenceMessage, 0)
	for rows.Next() {
		var value ReferenceMessage
		var runID sql.NullString
		var supersededBy string
		if err := rows.Scan(&value.MessageID, &value.SessionID, &runID, &value.Role, &value.Content, &value.CreatedAt, &value.AnswerVersion, &supersededBy); err != nil {
			return nil, err
		}
		if runID.Valid {
			value.RunID = runID.String
		}
		value.Superseded = supersededBy != ""
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
