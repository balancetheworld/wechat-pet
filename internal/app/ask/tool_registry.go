package ask

import "encoding/json"

// 本文件发布一期只读工具目录（对应架构设计 v2 文档第 6 节、7.2 节）。
// 工具是结构化能力记录（不是裸函数），由开发者审核后版本化发布，不开放运营后台编辑。
// 五个工具与 BusinessReadRepository 五方法一一对应；参数名与 ToolExecutorAdapter
// 的 executeCall 分发保持一致，由 tool_registry_test.go 的契约测试锁定，二者不得单方面改名。

// DefaultToolVersion 是一期工具目录的统一 Schema 版本。
const DefaultToolVersion = "v1"

// DefaultTools 返回一期只读工具集合（文档 7.2 表格）。
// 只注册有真实后端（BusinessReadRepository）支撑的只读工具：
//   - resolve_pet            解析家庭宠物
//   - read_pet_profile       读取宠物档案
//   - search_health_records  检索健康记录
//   - aggregate_health_records 聚合症状次数及最近时间
//   - read_health_record     读取单条记录与媒体引用
//
// 未注册的能力与理由：
//   - health_summary（长期摘要及来源）：BusinessReadRepository 尚未提供对应读取方法，不注册空工具。
//   - prepare_create / prepare_update / verify：属写入准备与核实，在 P6 落地（文档 7.2 末段、9/10 节）。
func DefaultTools() []Tool {
	return []Tool{
		resolvePetTool(),
		readPetProfileTool(),
		searchHealthRecordsTool(),
		aggregateHealthRecordsTool(),
		readHealthRecordTool(),
	}
}

// DefaultCatalog 发布一期工具目录。Skill（急症安全规则等）由后续安全规则落地时补齐，
// 当前 skills 为空：RecallSkills / EmergencySkills 均返回空集合，不伪造规则。
func DefaultCatalog(version string) (*Catalog, error) {
	return NewCatalog(DefaultTools(), nil, version, nil)
}

// resolvePetTool 解析家庭宠物（resolve + pet）。
// 参数 query 与 ToolExecutorAdapter.resolvePet 读取的 args["query"] 一致。
func resolvePetTool() Tool {
	return Tool{
		Name:          "resolve_pet",
		OperationID:   "resolve.pet",
		AliasesZH:     []string{"解析宠物", "识别宠物", "找宠物", "哪只宠物"},
		AliasesEN:     []string{"resolve pet", "identify pet", "find pet", "which pet"},
		ResourceType:  ResourcePet,
		ActionType:    ActionResolve,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "宠物名称或指代线索，例如「旺仔」「最大的那只」。歧义时由 Runtime 返回候选，不猜测 ID。"
				}
			},
			"required": ["query"],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"status": {"type": "string", "enum": ["none", "resolved", "ambiguous"]},
				"resolved": {"type": "array", "items": {"type": "object"}, "description": "唯一匹配的宠物"},
				"ambiguous": {"type": "array", "items": {"type": "object"}, "description": "同名候选组"},
				"source": {"type": "object", "description": "来源类型、对象、版本与读取时间"}
			},
			"required": ["status"]
		}`),
		UseCases:           []string{"用户用名称或指代提到某只宠物时，先解析出唯一 pet_id 再查询其记录"},
		NegativeCases:      []string{"名字有歧义时不得猜测 ID 直接查询，应返回候选让用户澄清"},
		Preconditions:      []string{"候选仅来自当前授权家庭范围"},
		SideEffects:        []string{},
		RiskLevel:          ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:  IdempotencyReadOnly,
		VerificationPolicy: VerificationNone,
		Version:            DefaultToolVersion,
	}
}

// readPetProfileTool 读取宠物档案（read + pet_profile）。
// 参数 pet_id 与 ToolExecutorAdapter.readByResource（ResourcePetProfile 分支）的 args["pet_id"] 一致。
func readPetProfileTool() Tool {
	return Tool{
		Name:          "read_pet_profile",
		OperationID:   "read.pet_profile",
		AliasesZH:     []string{"读取档案", "宠物档案", "看档案"},
		AliasesEN:     []string{"read pet profile", "pet profile"},
		ResourceType:  ResourcePetProfile,
		ActionType:    ActionRead,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pet_id": {
					"type": "string",
					"description": "已明确的宠物 ID，来自解析宠物结果，不可猜测。"
				}
			},
			"required": ["pet_id"],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"profile": {"type": "object", "description": "宠物档案字段及各字段缺失状态"},
				"health": {"type": "object", "description": "宠物健康信息"},
				"source": {"type": "object", "description": "来源及更新时间"}
			}
		}`),
		UseCases:           []string{"用户询问宠物档案、基本信息时读取已保存字段"},
		NegativeCases:      []string{"缺资料不得填默认病史；无权或对象不可读与服务故障分开表达，不泄露存在性"},
		Preconditions:      []string{"pet_id 已由解析宠物明确，且属于当前授权家庭范围"},
		SideEffects:        []string{},
		RiskLevel:          ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:  IdempotencyReadOnly,
		VerificationPolicy: VerificationSourceVersion,
		Version:            DefaultToolVersion,
	}
}

// searchHealthRecordsTool 检索健康记录（search + health_record）。
// 参数 pet_id/category/medical_type/start_at/end_at/limit/cursor 与
// ToolExecutorAdapter.searchRecords（healthRecordSearchQuery）一致。
func searchHealthRecordsTool() Tool {
	return Tool{
		Name:         "search_health_records",
		OperationID:  "search.health_record",
		AliasesZH:    []string{"查健康记录", "检索记录", "健康记录", "查记录"},
		AliasesEN:    []string{"search health records", "health records"},
		ResourceType: ResourceHealthRecord,
		ActionType:   ActionSearch,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pet_id": {"type": "string", "description": "已明确的宠物 ID，不可猜测"},
				"category": {"type": "string", "enum": ["medical", "daily"], "description": "记录大类，可选"},
				"medical_type": {"type": "string", "description": "医疗类型，如 vaccine/deworming，可选"},
				"start_at": {"type": "string", "description": "起点包含，RFC3339，可选"},
				"end_at": {"type": "string", "description": "终点不包含，RFC3339，可选"},
				"limit": {"type": "integer", "description": "页大小，可选"},
				"cursor": {"type": "string", "description": "分页游标，可选"}
			},
			"required": ["pet_id"],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"records": {"type": "array", "items": {"type": "object"}},
				"has_more": {"type": "boolean", "description": "是否还有更多页"},
				"next_cursor": {"type": "string", "description": "下一页游标"},
				"source": {"type": "object", "description": "来源版本与读取时间"}
			}
		}`),
		UseCases:           []string{"用户询问某只宠物在某个时间范围内的健康记录时检索"},
		NegativeCases:      []string{"服务失败不得返回成功空列表；截断或只有一页不得声称已读取全部记录"},
		Preconditions:      []string{"pet_id 已由解析宠物明确，且属于当前授权家庭范围"},
		SideEffects:        []string{},
		RiskLevel:          ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:  IdempotencyReadOnly,
		VerificationPolicy: VerificationSourceVersion,
		Version:            DefaultToolVersion,
	}
}

// aggregateHealthRecordsTool 聚合症状次数及最近时间（aggregate + health_record）。
// 参数 pet_id/category/medical_type/custom_medical_type/start_at/end_at 与
// ToolExecutorAdapter.aggregateRecords（healthRecordAggregateQuery）一致。
func aggregateHealthRecordsTool() Tool {
	return Tool{
		Name:         "aggregate_health_records",
		OperationID:  "aggregate.health_record",
		AliasesZH:    []string{"症状次数", "统计次数", "最近一次", "发生次数"},
		AliasesEN:    []string{"aggregate health records", "symptom count"},
		ResourceType: ResourceHealthRecord,
		ActionType:   ActionAggregate,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pet_id": {"type": "string", "description": "已明确的宠物 ID，不可猜测"},
				"category": {"type": "string", "enum": ["medical", "daily"], "description": "记录大类，可选"},
				"medical_type": {"type": "string", "description": "医疗类型，可选"},
				"custom_medical_type": {"type": "string", "description": "自定义医疗类型，可选"},
				"start_at": {"type": "string", "description": "起点包含，RFC3339，可选"},
				"end_at": {"type": "string", "description": "终点不包含，RFC3339，可选"}
			},
			"required": ["pet_id"],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"record_count": {"type": "integer", "description": "记录条数"},
				"occurrence_count": {"type": "integer", "description": "明确报告的发生次数合计"},
				"occurrence_known": {"type": "boolean", "description": "发生次数是否全部可知"},
				"unknown_records": {"type": "integer", "description": "次数未知的记录条数"},
				"latest_at": {"type": "string", "description": "最近发生时间"},
				"covered_start_at": {"type": "string", "description": "实际覆盖起点"},
				"covered_end_at": {"type": "string", "description": "实际覆盖终点"},
				"source": {"type": "object", "description": "数据版本"}
			}
		}`),
		UseCases:           []string{"用户询问某只宠物某症状发生过几次、最近一次是什么时候"},
		NegativeCases:      []string{"无可靠口径或覆盖不全时不得输出伪精确统计；未知次数不得按记录条数代替"},
		Preconditions:      []string{"pet_id 已由解析宠物明确，且属于当前授权家庭范围"},
		SideEffects:        []string{},
		RiskLevel:          ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:  IdempotencyReadOnly,
		VerificationPolicy: VerificationSourceVersion,
		Version:            DefaultToolVersion,
	}
}

// readHealthRecordTool 读取单条记录与媒体引用（read + health_record）。
// 参数 record_id 与 ToolExecutorAdapter.readByResource（ResourceHealthRecord 分支）的 args["record_id"] 一致。
func readHealthRecordTool() Tool {
	return Tool{
		Name:         "read_health_record",
		OperationID:  "read.health_record",
		AliasesZH:    []string{"查看记录详情", "单条记录", "记录详情"},
		AliasesEN:    []string{"read health record", "record detail"},
		ResourceType: ResourceHealthRecord,
		ActionType:   ActionRead,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"record_id": {
					"type": "string",
					"description": "已明确的健康记录 ID，不可猜测。"
				}
			},
			"required": ["record_id"],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"record": {"type": "object", "description": "记录版本、字段、资产身份与可用状态"},
				"source": {"type": "object", "description": "来源版本与读取时间"}
			}
		}`),
		UseCases:           []string{"用户询问某条健康记录的详情或媒体引用时读取单条记录"},
		NegativeCases:      []string{"已删、无权、资源失效与读取故障分别处理，不用过期 URL 或缓存替代授权"},
		Preconditions:      []string{"record_id 已明确，且属于当前授权家庭范围"},
		SideEffects:        []string{},
		RiskLevel:          ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:  IdempotencyReadOnly,
		VerificationPolicy: VerificationRecordVersion,
		Version:            DefaultToolVersion,
	}
}
