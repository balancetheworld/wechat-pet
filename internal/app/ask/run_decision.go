package ask

import (
	"context"
	"strings"
)

// 本文件是决策循环结果到 RunDecision 的降级映射（文档 8.4、8.5），
// 以及 processRun 的 v2 决策循环入口。v2 分组回答降级为现有 assistant.completed
// / assistant.question Event，前端暂不大改，后续再渐进升级结构化展示。

// maxDecisionStepsPerRun 是单 Run 决策循环步数上限（一期保守值，文档 9.3）。
const maxDecisionStepsPerRun = 16

// runV2DecisionLoop 执行 v2 单 Agent 决策循环并降级映射为 RunDecision。
// 组装上下文 -> 召回候选工具 -> 构造工具执行器 -> RunDecisionLoop -> 降级映射。
// ruleLevel 是 processRun 已评估的确定性规则风险，与模型风险合并后作为最终风险。
func (s *Service) runV2DecisionLoop(ctx context.Context, session Session, contextMessages []ContextMessage, currentInput string, contextSnapshot ContextSnapshot, ruleLevel RiskLevel) (RunDecision, error) {
	assembly := BuildRunContext(currentInput, contextMessages, contextSnapshot)
	filter := readOnlyFilter()
	recall := s.catalog.RecallTools(currentInput, filter, 0)
	tools := make([]Tool, 0, len(recall.Matches))
	for _, m := range recall.Matches {
		tools = append(tools, m.Tool)
	}
	executor := NewToolExecutorAdapter(s.catalog, s.businessRead, filter, session.FamilyID)
	outcome, err := RunDecisionLoop(ctx, s.agentModel, executor, StepInput{Blocks: assembly.Blocks, Tools: tools}, maxDecisionStepsPerRun)
	if err != nil {
		return RunDecision{}, err
	}
	return mapLoopOutcomeToDecision(outcome, ruleLevel), nil
}

// readOnlyFilter 构造一期只读工具的权限过滤（文档 7.1）。
func readOnlyFilter() Filter {
	return Filter{AllowedActions: map[ActionType]struct{}{
		ActionResolve:   {},
		ActionRead:      {},
		ActionSearch:    {},
		ActionAggregate: {},
	}}
}

// mapLoopOutcomeToDecision 把决策循环最终结果降级映射为 RunDecision（文档 8.4）。
// final_answer -> assistant.completed；request_input -> assistant.question；
// 其他动作（不应出现）映射为 run.failed。
func mapLoopOutcomeToDecision(outcome LoopOutcome, ruleLevel RiskLevel) RunDecision {
	switch outcome.Action {
	case ActionFinalAnswer:
		answer := joinGroupTexts(outcome.Groups)
		intent := intentForGroups(outcome.Groups)
		risk := MergeRisk(ruleLevel, maxModelRisk(outcome.Groups))
		return RunDecision{
			Status:    RunCompleted,
			RiskLevel: risk,
			EventType: "assistant.completed",
			Data:       map[string]any{"answer": answer, "intent": string(intent), "risk_level": string(risk)},
		}
	case ActionRequestInput:
		question := joinQuestionTexts(outcome.Questions)
		return RunDecision{
			Status:    RunWaitingInput,
			RiskLevel: MergeRisk(ruleLevel, RiskUnknown),
			EventType: "assistant.question",
			Data:       map[string]any{"question": question},
		}
	default:
		return RunDecision{Status: RunFailed, RiskLevel: RiskUnknown, EventType: "run.failed", Data: map[string]any{"error_code": "invalid_executor_status"}}
	}
}

// joinGroupTexts 拼接所有回答组的正文段（按组顺序、组内按段顺序），用于降级展示。
func joinGroupTexts(groups []AnswerGroup) string {
	parts := make([]string, 0)
	for _, g := range groups {
		for _, seg := range g.Segments {
			if text := strings.TrimSpace(seg.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// joinQuestionTexts 拼接所有追问文本，用于降级展示。
func joinQuestionTexts(questions []Question) string {
	parts := make([]string, 0, len(questions))
	for _, q := range questions {
		if text := strings.TrimSpace(q.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// intentForGroups 依据首个回答组类型映射 v1 intent（前端兼容字段）。
func intentForGroups(groups []AnswerGroup) Intent {
	if len(groups) == 0 {
		return IntentCasualChat
	}
	switch groups[0].AnswerKind {
	case AnswerHealth:
		return IntentPetHealth
	case AnswerFact:
		return IntentPetFact
	default:
		return IntentCasualChat
	}
}

// maxModelRisk 取所有回答组中模型给出的最高风险等级（文档 11.1 合并前）。
func maxModelRisk(groups []AnswerGroup) RiskLevel {
	level := RiskUnknown
	for _, g := range groups {
		for _, r := range g.Risks {
			if riskRank(r.Level) > riskRank(level) {
				level = r.Level
			}
		}
	}
	return level
}
