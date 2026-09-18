package ask

// RunCatalogSnapshot：Run 开始时固定的目录快照。后续只能在同一目录快照内
// 追加少量候选，追加检索计入预算，不加载全量目录。正常目录变更只影响新 Run。
type RunCatalogSnapshot struct {
	ToolCatalogVersion string   `json:"tool_catalog_version"`
	ToolCatalogDigest  string   `json:"tool_catalog_digest"`
	SelectedToolIDs    []string `json:"selected_tool_ids"`
	SelectedSkillIDs   []string `json:"selected_skill_ids"`
}

// NewRunCatalogSnapshot 从目录固定 Run 级快照。
func NewRunCatalogSnapshot(catalog *Catalog) RunCatalogSnapshot {
	return RunCatalogSnapshot{
		ToolCatalogVersion: catalog.Version(),
		ToolCatalogDigest:  catalog.Digest(),
	}
}

// AppendTool 在同一目录快照内追加工具候选（operation_id），去重。
// 返回是否实际追加（false 表示已存在）。
func (s *RunCatalogSnapshot) AppendTool(id string) bool {
	if s.ContainsTool(id) {
		return false
	}
	s.SelectedToolIDs = append(s.SelectedToolIDs, id)
	return true
}

// AppendSkill 在同一目录快照内追加 Skill 候选（skill_id），去重。
// 返回是否实际追加（false 表示已存在）。
func (s *RunCatalogSnapshot) AppendSkill(id string) bool {
	if s.ContainsSkill(id) {
		return false
	}
	s.SelectedSkillIDs = append(s.SelectedSkillIDs, id)
	return true
}

// ContainsTool 判断工具 id 是否已在快照内。
func (s *RunCatalogSnapshot) ContainsTool(id string) bool {
	for _, v := range s.SelectedToolIDs {
		if v == id {
			return true
		}
	}
	return false
}

// ContainsSkill 判断 Skill id 是否已在快照内。
func (s *RunCatalogSnapshot) ContainsSkill(id string) bool {
	for _, v := range s.SelectedSkillIDs {
		if v == id {
			return true
		}
	}
	return false
}
