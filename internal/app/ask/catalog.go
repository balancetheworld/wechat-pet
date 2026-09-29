package ask

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Catalog：由开发者审核后版本化发布的 Skill + 工具目录。
// Run 开始时固定目录版本，正常变更只影响新 Run，紧急禁用立即阻断新使用。
type Catalog struct {
	version   string
	digest    string
	tools     []Tool
	skills    []Skill
	toolIndex *BM25FIndex
	tokenizer Tokenizer
}

// NewCatalog 发布目录：校验所有工具与 Skill 合法，拒绝同名冲突，计算版本摘要。
// tokenizer 为 nil 时使用 DefaultTokenizer。
func NewCatalog(tools []Tool, skills []Skill, version string, tokenizer Tokenizer) (*Catalog, error) {
	if tokenizer == nil {
		tokenizer = DefaultTokenizer()
	}
	if err := validateToolNames(tools); err != nil {
		return nil, err
	}
	if err := validateSkillIDs(skills); err != nil {
		return nil, err
	}
	for _, tool := range tools {
		if err := tool.Validate(); err != nil {
			return nil, err
		}
	}
	for _, skill := range skills {
		if err := skill.Validate(); err != nil {
			return nil, err
		}
	}
	return &Catalog{
		version:   version,
		digest:    computeCatalogDigest(tools, skills),
		tools:     tools,
		skills:    skills,
		toolIndex: NewBM25FIndex(tools, tokenizer),
		tokenizer: tokenizer,
	}, nil
}

// Version 返回目录版本号。
func (c *Catalog) Version() string { return c.version }

// Digest 返回目录内容摘要（工具与 Skill 的稳定哈希）。
func (c *Catalog) Digest() string { return c.digest }

// Filter：候选召回的权限、能力与紧急禁用过滤条件。所有集合为空表示不过滤。
type Filter struct {
	AllowedResources map[ResourceType]struct{} // 允许的资源类型
	AllowedTools     map[string]struct{}       // 允许的工具（operation_id 或 name）
	AllowedActions   map[ActionType]struct{}   // 允许的动作类型（能力过滤）
	AllowedScopes    map[SkillScope]struct{}   // 允许的 Skill scope
	DisabledTools    map[string]struct{}       // 紧急禁用的工具（立即阻断，跨版本）
	DisabledSkills   map[string]struct{}       // 紧急禁用的 Skill（立即阻断，跨版本）
}

func (f Filter) allowTool(t Tool) bool {
	if _, ok := f.DisabledTools[t.OperationID]; ok {
		return false
	}
	if _, ok := f.DisabledTools[t.Name]; ok {
		return false
	}
	if len(f.AllowedResources) > 0 {
		if _, ok := f.AllowedResources[t.ResourceType]; !ok {
			return false
		}
	}
	if len(f.AllowedTools) > 0 {
		if _, ok := f.AllowedTools[t.OperationID]; !ok {
			if _, ok2 := f.AllowedTools[t.Name]; !ok2 {
				return false
			}
		}
	}
	if len(f.AllowedActions) > 0 {
		if _, ok := f.AllowedActions[t.ActionType]; !ok {
			return false
		}
	}
	return true
}

func (f Filter) allowSkill(s Skill) bool {
	if _, ok := f.DisabledSkills[s.ID]; ok {
		return false
	}
	if len(f.AllowedScopes) > 0 {
		_, ok := f.AllowedScopes[s.Scope]
		return ok
	}
	return true
}

// ToolRecallResult：工具候选召回结果。
type ToolRecallResult struct {
	Matches      []ToolMatch
	UsedFallback bool // 是否经过一次兜底检索（BM25F 无命中后用别名再检索）
}

// RecallTools 对查询做工具候选召回：先 BM25F 词法检索，再做权限与能力过滤；
// 无命中时基于已声明别名做一次兜底检索，仍无候选则返回空（明确能力受限，
// 不让模型编造工具）。limit <= 0 不截断。
func (c *Catalog) RecallTools(query string, filter Filter, limit int) ToolRecallResult {
	matches := c.toolIndex.Search(query, 0)
	filtered := filterTools(matches, filter)
	if len(filtered) > 0 {
		if limit > 0 && len(filtered) > limit {
			filtered = filtered[:limit]
		}
		return ToolRecallResult{Matches: filtered}
	}
	fallback := c.fallbackByAlias(query, filter)
	if len(fallback) > 0 {
		if limit > 0 && len(fallback) > limit {
			fallback = fallback[:limit]
		}
		return ToolRecallResult{Matches: fallback, UsedFallback: true}
	}
	return ToolRecallResult{}
}

// ToolByName 返回通过过滤的指定工具，用于把关键工具固定在候选集合内（文档 7.3）。
// 未注册或未通过权限、能力、紧急禁用过滤时返回 false。
func (c *Catalog) ToolByName(name string, filter Filter) (Tool, bool) {
	for _, tool := range c.tools {
		if tool.Name == name && filter.allowTool(tool) {
			return tool, true
		}
	}
	return Tool{}, false
}

// RecallSkills 对查询做 Skill 候选召回：trigger_examples 词法匹配，命中
// negative_examples 的 Skill 排除；通过过滤后按固定 scope 顺序排序。允许一次
// 命中多个领域 Skill。
func (c *Catalog) RecallSkills(query string, filter Filter) []Skill {
	queryTokens := uniqueTokens(c.tokenizer(query))
	result := make([]Skill, 0)
	for _, s := range c.skills {
		if !filter.allowSkill(s) {
			continue
		}
		if matchesAnyToken(s.NegativeExamples, queryTokens, c.tokenizer) {
			continue
		}
		if !matchesAnyToken(s.TriggerExamples, queryTokens, c.tokenizer) {
			continue
		}
		result = append(result, s)
	}
	SortSkills(result)
	return result
}

// EmergencySkills 返回高风险规则兜底：所有急症安全 Skill，不论模型是否选择。
func (c *Catalog) EmergencySkills() []Skill {
	result := make([]Skill, 0)
	for _, s := range c.skills {
		if s.Scope == ScopeEmergencySafety {
			result = append(result, s)
		}
	}
	SortSkills(result)
	return result
}

func filterTools(matches []ToolMatch, filter Filter) []ToolMatch {
	result := make([]ToolMatch, 0, len(matches))
	for _, m := range matches {
		if filter.allowTool(m.Tool) {
			result = append(result, m)
		}
	}
	return result
}

// fallbackByAlias：一次兜底检索——检查通过过滤的工具，其已声明别名（中英文别名、
// operation_id、name）是否与查询存在子串包含关系。
func (c *Catalog) fallbackByAlias(query string, filter Filter) []ToolMatch {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	result := make([]ToolMatch, 0)
	for _, tool := range c.tools {
		if !filter.allowTool(tool) {
			continue
		}
		for _, alias := range toolAliases(tool) {
			a := strings.ToLower(strings.TrimSpace(alias))
			if a == "" {
				continue
			}
			if strings.Contains(q, a) || strings.Contains(a, q) {
				result = append(result, ToolMatch{Tool: tool, Score: 1.0})
				break
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Tool.OperationID < result[j].Tool.OperationID
	})
	return result
}

// toolAliases 返回工具用于兜底检索的别名集合。
func toolAliases(t Tool) []string {
	result := make([]string, 0, len(t.AliasesZH)+len(t.AliasesEN)+2)
	result = append(result, t.OperationID, t.Name)
	result = append(result, t.AliasesZH...)
	result = append(result, t.AliasesEN...)
	return result
}

// matchesAnyToken 判断 examples 中是否任一条目与 queryTokens 有共享 token。
func matchesAnyToken(examples []string, queryTokens []string, tokenizer Tokenizer) bool {
	for _, example := range examples {
		exampleTokens := uniqueTokens(tokenizer(example))
		for _, qt := range queryTokens {
			for _, et := range exampleTokens {
				if qt == et {
					return true
				}
			}
		}
	}
	return false
}

// validateToolNames 拒绝相互冲突的同名工具定义（operation_id 或 name 重复）。
func validateToolNames(tools []Tool) error {
	byOperation := make(map[string]string, len(tools))
	byName := make(map[string]string, len(tools))
	for _, tool := range tools {
		if prev, ok := byOperation[tool.OperationID]; ok && prev != tool.Name {
			return fmt.Errorf("catalog: duplicate operation_id %q", tool.OperationID)
		}
		if prev, ok := byName[tool.Name]; ok && prev != tool.OperationID {
			return fmt.Errorf("catalog: duplicate tool_name %q", tool.Name)
		}
		byOperation[tool.OperationID] = tool.Name
		byName[tool.Name] = tool.OperationID
	}
	return nil
}

// validateSkillIDs 拒绝重复的 skill_id 定义。
func validateSkillIDs(skills []Skill) error {
	seen := make(map[string]struct{}, len(skills))
	for _, skill := range skills {
		if _, ok := seen[skill.ID]; ok {
			return fmt.Errorf("catalog: duplicate skill_id %q", skill.ID)
		}
		seen[skill.ID] = struct{}{}
	}
	return nil
}

// computeCatalogDigest 计算目录内容摘要：工具按 operation_id、Skill 按 skill_id
// 稳定排序后拼接 id|version，取 SHA-256。
func computeCatalogDigest(tools []Tool, skills []Skill) string {
	toolIDs := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolIDs = append(toolIDs, tool.OperationID+"|"+tool.Version)
	}
	sort.Strings(toolIDs)
	skillIDs := make([]string, 0, len(skills))
	for _, skill := range skills {
		skillIDs = append(skillIDs, skill.ID+"|"+skill.Version)
	}
	sort.Strings(skillIDs)
	h := sha256.New()
	for _, id := range toolIDs {
		h.Write([]byte(id))
		h.Write([]byte{0})
	}
	h.Write([]byte("::skills::"))
	for _, id := range skillIDs {
		h.Write([]byte(id))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
