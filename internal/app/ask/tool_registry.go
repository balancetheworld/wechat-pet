package ask

import "encoding/json"

// 本文件发布一期只读工具目录（对应架构设计 v2 文档第 6 节、7.2 节）。
// 工具是结构化能力记录（不是裸函数），由开发者审核后版本化发布，不开放运营后台编辑。
// 五个工具与 BusinessReadRepository 五方法一一对应；参数名与 ToolExecutorAdapter
// 的 executeCall 分发保持一致，由 tool_registry_test.go 的契约测试锁定，二者不得单方面改名。

// DefaultToolVersion 是一期工具目录的统一 Schema 版本。
const DefaultToolVersion = "v1"

// petRosterToolName 是家庭宠物清单工具的稳定名称，决策循环据此把该工具固定在
// 候选集合内（没有对象名的提问不会通过召回命中它）。
const petRosterToolName = "list_family_pets"

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
		listFamilyPetsTool(),
		resolvePetTool(),
		readPetProfileTool(),
		searchHealthRecordsTool(),
		aggregateHealthRecordsTool(),
		readHealthRecordTool(),
		createCalendarRecordTool(),
		updateCalendarRecordTool(),
		updatePetProfileTool(),
		completeCalendarReminderTool(),
		listCalendarRecordsTool(),
		listRemindersTool(),
		updatePetHealthTool(),
		createPetTool(),
	}
}

// createPetTool 新建宠物（prepare_create + pet）。
// 只接受模型可核验的字段（名字/品种/性别/绝育/生日）；头像等资产字段不开放。
func createPetTool() Tool {
	return Tool{
		Name:         "create_pet",
		OperationID:  "prepare.pet.create",
		AliasesZH:    []string{"添加宠物", "新增宠物", "新建宠物", "又养了一只", "加一只", "给它建个档案"},
		AliasesEN:    []string{"create pet", "add pet"},
		ResourceType: ResourcePet,
		ActionType:   ActionPrepareCreate,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"name": {"type": "string", "description": "宠物名字，必填"},
				"breed": {"type": "string", "description": "品种，可选"},
				"gender": {"type": "string", "description": "性别，如 公/母，可选"},
				"sterilized": {"type": "boolean", "description": "是否已绝育，可选"},
				"birthday": {"type": "string", "description": "生日，YYYY-MM-DD，可选；相对描述用上下文当前时间换算"},
				"home_date": {"type": "string", "description": "到家/领养日期，YYYY-MM-DD，可选；属于档案字段，不要写成日历记录"}
			},
			"required": ["name"],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"operation_id": {"type": "string"},
				"status": {"type": "string", "enum": ["pending"]},
				"preview": {"type": "string"}
			},
			"required": ["operation_id", "status", "preview"]
		}`),
		UseCases: []string{
			"用户明确表示家里新增了一只宠物并希望建档时，先准备新建预览，由用户确认后创建",
			"用户同时给出到家/领养日期时一并写入 home_date 档案字段，不要额外新建日历日常记录",
		},
		NegativeCases:        []string{"用户只是提到宠物或询问时不得创建；名字缺失时先追问；不得凭推测补全品种、生日等字段"},
		Preconditions:        []string{"宠物名字由用户明确给出"},
		SideEffects:          []string{"创建一条待确认的新建宠物预览（尚未写入业务数据）"},
		RiskLevel:            ToolRiskMedium,
		RequiresConfirmation: true,
		IdempotencyPolicy:    IdempotencyNewIdentity,
		VerificationPolicy:   VerificationRecordVersion,
		Version:              DefaultToolVersion,
	}
}

// updatePetHealthTool 修改宠物健康档案（prepare_update + pet_profile）。
// 覆盖过敏、长期用药与健康状态描述；这些字段影响健康建议，必须由用户确认后写入。
func updatePetHealthTool() Tool {
	return Tool{
		Name:         "update_pet_health",
		OperationID:  "prepare.pet_health.update",
		AliasesZH:    []string{"过敏", "记录过敏", "长期用药", "在吃什么药", "健康状况", "既往病史", "慢性病"},
		AliasesEN:    []string{"update pet health", "pet allergies", "long term medication"},
		ResourceType: ResourcePetProfile,
		ActionType:   ActionPrepareUpdate,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pet_id": {"type": "string", "description": "已明确的宠物 ID，不可猜测"},
				"status": {"type": "string", "description": "当前健康状态描述，可选"},
				"allergies": {"type": "string", "description": "过敏信息，可选"},
				"long_term_medication": {"type": "string", "description": "长期用药，可选"}
			},
			"required": ["pet_id"],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"operation_id": {"type": "string"},
				"status": {"type": "string", "enum": ["pending"]},
				"preview": {"type": "string"}
			},
			"required": ["operation_id", "status", "preview"]
		}`),
		UseCases:             []string{"用户说明宠物的过敏、长期用药或健康状态需要登记/更正时，先准备修改预览，由用户确认后写入"},
		NegativeCases:        []string{"不得把推测或模型判断写成健康事实；用户只是询问时不得修改；未确认前不得声称已写入"},
		Preconditions:        []string{"pet_id 已由解析宠物明确且属于当前授权家庭范围"},
		SideEffects:          []string{"创建一条待确认的健康档案修改预览（尚未写入业务数据）"},
		RiskLevel:            ToolRiskHigh,
		RequiresConfirmation: true,
		IdempotencyPolicy:    IdempotencyNewIdentity,
		VerificationPolicy:   VerificationRecordVersion,
		Version:              DefaultToolVersion,
	}
}

// listRemindersTool 列出家庭的待办提醒（read + health_record）。
// 提醒来自已有记录（疫苗/驱虫等）的后续安排，供「下次什么时候」这类问题使用。
func listRemindersTool() Tool {
	return Tool{
		Name:         "list_reminders",
		OperationID:  "read.reminder.list",
		AliasesZH:    []string{"待办", "提醒", "还有什么要做的", "下次疫苗", "疫苗该打了吗", "下次驱虫", "待办提醒"},
		AliasesEN:    []string{"list reminders", "pending reminders", "upcoming"},
		ResourceType: ResourceHealthRecord,
		ActionType:   ActionRead,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"due_before": {"type": "string", "description": "只返回该日期（YYYY-MM-DD）之前的待办，可选；缺省为今天，相对时间用上下文当前时间换算"},
				"limit": {"type": "integer", "minimum": 1, "maximum": 50, "description": "返回条数上限，可选"}
			},
			"required": [],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"reminders": {"type": "array", "items": {"type": "object"}},
				"source": {"type": "object"}
			},
			"required": ["reminders"]
		}`),
		UseCases:             []string{"用户询问还有哪些待办、下次疫苗/驱虫时间时先取提醒列表"},
		NegativeCases:        []string{"只返回当前授权家庭的待办；不得把已完成提醒当作待办；不得用列表之外的信息推断时间"},
		Preconditions:        []string{"调用者属于当前授权家庭"},
		SideEffects:          []string{},
		RiskLevel:            ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:    IdempotencyReadOnly,
		VerificationPolicy:   VerificationSourceVersion,
		Version:              DefaultToolVersion,
	}
}

// listCalendarRecordsTool 列出家庭在时间范围内的日程记录（read + health_record）。
// 覆盖「最近有什么安排/这周记了什么」这类跨宠物问题；相对时间用上下文当前时间换算。
func listCalendarRecordsTool() Tool {
	return Tool{
		Name:         "list_calendar_records",
		OperationID:  "read.calendar_record.list",
		AliasesZH:    []string{"查日程", "最近日程", "这周有什么安排", "最近记录", "日历里有什么", "最近做了什么", "有哪些记录"},
		AliasesEN:    []string{"list calendar records", "recent records", "schedule"},
		ResourceType: ResourceHealthRecord,
		ActionType:   ActionRead,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"start_at": {"type": "string", "description": "起点包含，RFC3339，可选；缺省为最近 30 天，相对时间用上下文当前时间换算"},
				"end_at": {"type": "string", "description": "终点不包含，RFC3339，可选；缺省为当前时间"},
				"limit": {"type": "integer", "minimum": 1, "maximum": 50, "description": "返回条数上限，可选"}
			},
			"required": [],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"records": {"type": "array", "items": {"type": "object"}},
				"source": {"type": "object"}
			},
			"required": ["records"]
		}`),
		UseCases:             []string{"用户询问家庭最近的日程安排或某段时间的记录（可能跨多只宠物）时先取列表"},
		NegativeCases:        []string{"只返回当前授权家庭的记录；不得用列表之外的信息推断未记录的事项"},
		Preconditions:        []string{"调用者属于当前授权家庭"},
		SideEffects:          []string{},
		RiskLevel:            ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:    IdempotencyReadOnly,
		VerificationPolicy:   VerificationSourceVersion,
		Version:              DefaultToolVersion,
	}
}

// listFamilyPetsTool 列出当前家庭的宠物（read + pet）。
// 用户问「有几只宠物/都有谁」时先拿这份列表；模型不得凭名字猜 pet_id。
func listFamilyPetsTool() Tool {
	return Tool{
		Name:         petRosterToolName,
		OperationID:  "read.pet.list",
		AliasesZH:    []string{"几只宠物", "有多少宠物", "宠物列表", "都有哪些宠物", "家里有哪些宠物", "现有宠物", "谁在家"},
		AliasesEN:    []string{"list family pets", "how many pets", "my pets"},
		ResourceType: ResourcePet,
		ActionType:   ActionRead,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {},
			"required": [],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pets": {"type": "array", "items": {"type": "object", "properties": {"pet_id": {"type": "string"}, "name": {"type": "string"}}}},
				"source": {"type": "object", "description": "来源类型、对象与版本"}
			},
			"required": ["pets"]
		}`),
		UseCases:             []string{"用户询问家庭里有几只宠物、都有哪些宠物，或需要按名字/指代确认宠物身份时先取列表"},
		NegativeCases:        []string{"列表只包含当前授权家庭的宠物；不得据此推断其他家庭或已删除的宠物"},
		Preconditions:        []string{"调用者属于当前授权家庭"},
		SideEffects:          []string{},
		RiskLevel:            ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:    IdempotencyReadOnly,
		VerificationPolicy:   VerificationSourceVersion,
		Version:              DefaultToolVersion,
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
		Name:         "resolve_pet",
		OperationID:  "resolve.pet",
		AliasesZH:    []string{"解析宠物", "识别宠物", "找宠物", "哪只宠物", "分别是谁", "分别是哪只"},
		AliasesEN:    []string{"resolve pet", "identify pet", "find pet", "which pet"},
		ResourceType: ResourcePet,
		ActionType:   ActionResolve,
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
		UseCases:             []string{"用户用名称或指代提到某只宠物时，先解析出唯一 pet_id 再查询其记录"},
		NegativeCases:        []string{"名字有歧义时不得猜测 ID 直接查询，应返回候选让用户澄清"},
		Preconditions:        []string{"候选仅来自当前授权家庭范围"},
		SideEffects:          []string{},
		RiskLevel:            ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:    IdempotencyReadOnly,
		VerificationPolicy:   VerificationNone,
		Version:              DefaultToolVersion,
	}
}

// readPetProfileTool 读取宠物档案（read + pet_profile）。
// 参数 pet_id 与 ToolExecutorAdapter.readByResource（ResourcePetProfile 分支）的 args["pet_id"] 一致。
func readPetProfileTool() Tool {
	return Tool{
		Name:         "read_pet_profile",
		OperationID:  "read.pet_profile",
		AliasesZH:    []string{"读取档案", "宠物档案", "看档案"},
		AliasesEN:    []string{"read pet profile", "pet profile"},
		ResourceType: ResourcePetProfile,
		ActionType:   ActionRead,
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
		UseCases:             []string{"用户询问宠物档案、基本信息时读取已保存字段"},
		NegativeCases:        []string{"缺资料不得填默认病史；无权或对象不可读与服务故障分开表达，不泄露存在性"},
		Preconditions:        []string{"pet_id 已由解析宠物明确，且属于当前授权家庭范围"},
		SideEffects:          []string{},
		RiskLevel:            ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:    IdempotencyReadOnly,
		VerificationPolicy:   VerificationSourceVersion,
		Version:              DefaultToolVersion,
	}
}

// searchHealthRecordsTool 检索健康记录（search + health_record）。
// 参数 pet_id/category/medical_type/start_at/end_at/limit/cursor 与
// ToolExecutorAdapter.searchRecords（healthRecordSearchQuery）一致。
func searchHealthRecordsTool() Tool {
	return Tool{
		Name:         "search_health_records",
		OperationID:  "search.health_record",
		AliasesZH:    []string{"查健康记录", "检索记录", "健康记录", "查记录", "疫苗", "上次疫苗"},
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
				"limit": {"type": "integer", "minimum": 1, "maximum": 100, "description": "页大小，可选"},
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
		UseCases: []string{
			"用户询问某只宠物在某个时间范围内的健康记录时检索",
			"「上周/最近三天/上个月」等相对时间必须用上下文中的当前时间换算成 start_at/end_at 的 RFC3339 绝对值后再调用",
		},
		NegativeCases:        []string{"服务失败不得返回成功空列表；截断或只有一页不得声称已读取全部记录"},
		Preconditions:        []string{"pet_id 已由解析宠物明确，且属于当前授权家庭范围"},
		SideEffects:          []string{},
		RiskLevel:            ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:    IdempotencyReadOnly,
		VerificationPolicy:   VerificationSourceVersion,
		Version:              DefaultToolVersion,
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
		UseCases: []string{
			"用户询问某只宠物某症状发生过几次、最近一次是什么时候",
			"「最近一周/上个月」等相对区间必须用上下文中的当前时间换算成 start_at/end_at 的 RFC3339 绝对值后再聚合",
		},
		NegativeCases:        []string{"无可靠口径或覆盖不全时不得输出伪精确统计；未知次数不得按记录条数代替"},
		Preconditions:        []string{"pet_id 已由解析宠物明确，且属于当前授权家庭范围"},
		SideEffects:          []string{},
		RiskLevel:            ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:    IdempotencyReadOnly,
		VerificationPolicy:   VerificationSourceVersion,
		Version:              DefaultToolVersion,
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
		UseCases:             []string{"用户询问某条健康记录的详情或媒体引用时读取单条记录"},
		NegativeCases:        []string{"已删、无权、资源失效与读取故障分别处理，不用过期 URL 或缓存替代授权"},
		Preconditions:        []string{"record_id 已明确，且属于当前授权家庭范围"},
		SideEffects:          []string{},
		RiskLevel:            ToolRiskLow,
		RequiresConfirmation: false,
		IdempotencyPolicy:    IdempotencyReadOnly,
		VerificationPolicy:   VerificationRecordVersion,
		Version:              DefaultToolVersion,
	}
}

// createCalendarRecordTool 准备一条日历记录（prepare_create + health_record）。
// 不直接写入：只形成待用户确认的 Operation 预览，确认后由服务端执行并核实（文档 7.1、8.7）。
func createCalendarRecordTool() Tool {
	return Tool{
		Name:         "create_calendar_record",
		OperationID:  "prepare.calendar_record.create",
		AliasesZH:    []string{"记一笔", "添加记录", "新增记录", "记录一下", "记到日历", "日历事件", "记下洗澡", "记下喂药", "登记疫苗"},
		AliasesEN:    []string{"create calendar record", "add record", "log event"},
		ResourceType: ResourceHealthRecord,
		ActionType:   ActionPrepareCreate,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pet_id": {"type": "string", "description": "已明确的宠物 ID，不可猜测"},
				"category": {"type": "string", "enum": ["daily", "medical"], "description": "记录大类：daily 日常 / medical 医疗"},
				"medical_type": {"type": "string", "enum": ["vaccine", "deworming", "checkup", "visit", "medication", "other"], "description": "医疗类型，仅 category=medical 时使用；自定义类型填 other 并在 custom_medical_type 写明名称，category=daily 时不得填写"},
				"custom_medical_type": {"type": "string", "maxLength": 50, "description": "自定义医疗类型名称，仅 medical_type=other 时填写，最多 50 个字符；其他取值下不得填写"},
				"content": {"type": "string", "description": "记录正文，例如「洗澡」「服用驱虫药」"},
				"occurred_at": {"type": "string", "description": "发生时间，RFC3339；相对时间用上下文当前时间换算"},
				"sync_targets": {"type": "array", "items": {"type": "string", "enum": ["growth"]}, "description": "可选：让这条记录同时写入成长足迹页（growth）；仅当用户明确要求同步到成长足迹时填写"}
			},
			"required": ["pet_id", "category", "content", "occurred_at"],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"operation_id": {"type": "string", "description": "待确认写入预览的标识"},
				"status": {"type": "string", "enum": ["pending"]},
				"preview": {"type": "string", "description": "确认界面展示的摘要"}
			},
			"required": ["operation_id", "status", "preview"]
		}`),
		UseCases: []string{
			"用户明确要求把某件事记到宠物日历或记录里时，先准备写入预览，由用户在页面确认后写入",
			"医疗记录类型不在枚举内时用 medical_type=other 并在 custom_medical_type 写明用户原话类型，例如 medical_type=other、custom_medical_type=过敏复查",
			"用户明确要求记录同时同步到档案时，用 sync_targets 指定 growth（成长足迹页）",
		},
		NegativeCases:        []string{"用户只是询问或尚未确认时不得写入；发生时间不确定时先追问，不猜测时间；不得声称已写入；日常记录不得填 medical_type 或 custom_medical_type；不得把自定义名称直接塞进 medical_type；用户没有要求同步档案时不得自行填写 sync_targets；生日纪念页只记录生日内容，不得作为日常或医疗记录的同步目标"},
		Preconditions:        []string{"pet_id 已由解析宠物明确且属于当前授权家庭范围；occurred_at 已换算为绝对时间"},
		SideEffects:          []string{"创建一条待确认的日历记录预览（尚未写入业务数据）"},
		RiskLevel:            ToolRiskMedium,
		RequiresConfirmation: true,
		IdempotencyPolicy:    IdempotencyNewIdentity,
		VerificationPolicy:   VerificationRecordVersion,
		Version:              DefaultToolVersion,
	}
}

// updateCalendarRecordTool 修改一条已存在的日历记录（prepare_update + health_record）。
// record_id 必须先由查询工具取得；同样只形成待确认预览。
func updateCalendarRecordTool() Tool {
	return Tool{
		Name:         "update_calendar_record",
		OperationID:  "prepare.calendar_record.update",
		AliasesZH:    []string{"改记录", "修改记录", "更正记录", "改时间", "改内容", "记录写错了"},
		AliasesEN:    []string{"update calendar record", "edit record"},
		ResourceType: ResourceHealthRecord,
		ActionType:   ActionPrepareUpdate,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"record_id": {"type": "string", "description": "已存在记录的 ID，必须先用查询工具取得，不可猜测"},
				"content": {"type": "string", "description": "修改后的记录正文，可选"},
				"occurred_at": {"type": "string", "description": "修改后的发生时间，RFC3339，可选；相对时间用上下文当前时间换算"}
			},
			"required": ["record_id"],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"operation_id": {"type": "string"},
				"status": {"type": "string", "enum": ["pending"]},
				"preview": {"type": "string"}
			},
			"required": ["operation_id", "status", "preview"]
		}`),
		UseCases:             []string{"用户指出已保存记录的内容或时间需要修改时，先准备修改预览，由用户确认后写入"},
		NegativeCases:        []string{"只是询问记录内容时不得修改；未确认前不得声称已修改；不得修改当前授权范围外的记录"},
		Preconditions:        []string{"record_id 来自当前授权范围且真实存在的记录"},
		SideEffects:          []string{"创建一条待确认的记录修改预览（尚未写入业务数据）"},
		RiskLevel:            ToolRiskMedium,
		RequiresConfirmation: true,
		IdempotencyPolicy:    IdempotencyNewIdentity,
		VerificationPolicy:   VerificationRecordVersion,
		Version:              DefaultToolVersion,
	}
}

// updatePetProfileTool 修改宠物档案字段（prepare_update + pet_profile）。
// 只暴露模型可核验的文本/布尔/日期字段；头像与封面等资产字段不开放给模型。
func updatePetProfileTool() Tool {
	return Tool{
		Name:         "update_pet_profile",
		OperationID:  "prepare.pet_profile.update",
		AliasesZH:    []string{"改档案", "修改档案", "更新宠物资料", "改成", "改名字", "改品种", "改生日", "改性别", "是否绝育", "名字是", "品种是", "性别是", "生日是"},
		AliasesEN:    []string{"update pet profile", "edit pet profile"},
		ResourceType: ResourcePetProfile,
		ActionType:   ActionPrepareUpdate,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pet_id": {"type": "string", "description": "已明确的宠物 ID，不可猜测"},
				"name": {"type": "string", "description": "宠物名，可选"},
				"breed": {"type": "string", "description": "品种，可选"},
				"gender": {"type": "string", "description": "性别，如 公/母，可选"},
				"sterilized": {"type": "boolean", "description": "是否已绝育，可选"},
				"birthday": {"type": "string", "description": "生日，YYYY-MM-DD，可选；相对描述用上下文当前时间换算"},
				"home_date": {"type": "string", "description": "到家/领养日期，YYYY-MM-DD，可选；属于档案字段，不要写成日历记录"}
			},
			"required": ["pet_id"],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"operation_id": {"type": "string"},
				"status": {"type": "string", "enum": ["pending"]},
				"preview": {"type": "string"}
			},
			"required": ["operation_id", "status", "preview"]
		}`),
		UseCases: []string{
			"用户要求修改宠物档案字段（名字、品种、性别、绝育、生日、到家日期）时，先准备修改预览，由用户确认后写入",
			"用户提到「到家日期/领养日期」时写入档案 home_date；不要用日历日常记录代替档案字段",
		},
		NegativeCases:        []string{"用户只是询问档案内容时不得修改；未确认前不得声称已修改；头像/封面等资产字段不由模型修改"},
		Preconditions:        []string{"pet_id 已由解析宠物明确且属于当前授权家庭范围；字段值可由用户陈述或已保存数据核验"},
		SideEffects:          []string{"创建一条待确认的档案修改预览（尚未写入业务数据）"},
		RiskLevel:            ToolRiskMedium,
		RequiresConfirmation: true,
		IdempotencyPolicy:    IdempotencyNewIdentity,
		VerificationPolicy:   VerificationRecordVersion,
		Version:              DefaultToolVersion,
	}
}

// completeCalendarReminderTool 完成一条待办提醒（prepare_update + health_record）。
// 完成会生成一条记录，因此同样先形成待确认预览。
func completeCalendarReminderTool() Tool {
	return Tool{
		Name:         "complete_calendar_reminder",
		OperationID:  "prepare.calendar_reminder.complete",
		AliasesZH:    []string{"完成提醒", "做完了", "打了疫苗了", "喂过药了", "已完成待办", "标记完成"},
		AliasesEN:    []string{"complete reminder", "mark reminder done"},
		ResourceType: ResourceHealthRecord,
		ActionType:   ActionPrepareUpdate,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"reminder_id": {"type": "string", "description": "待办提醒 ID，必须来自当前家庭的真实提醒，不可猜测"},
				"completed_at": {"type": "string", "description": "完成时间，RFC3339，可选；相对时间用上下文当前时间换算"},
				"content": {"type": "string", "description": "完成时补充的记录正文，可选"}
			},
			"required": ["reminder_id"],
			"additionalProperties": false
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"operation_id": {"type": "string"},
				"status": {"type": "string", "enum": ["pending"]},
				"preview": {"type": "string"}
			},
			"required": ["operation_id", "status", "preview"]
		}`),
		UseCases:             []string{"用户表示某条待办提醒已经完成（打过疫苗、喂过药等）时，先准备完成预览，由用户确认后写入"},
		NegativeCases:        []string{"只是询问提醒时不得标记完成；未确认前不得声称已完成；不得完成其他家庭的提醒"},
		Preconditions:        []string{"reminder_id 来自当前授权家庭的真实待办提醒"},
		SideEffects:          []string{"创建一条待确认的提醒完成预览（尚未写入业务数据）"},
		RiskLevel:            ToolRiskMedium,
		RequiresConfirmation: true,
		IdempotencyPolicy:    IdempotencyNewIdentity,
		VerificationPolicy:   VerificationRecordVersion,
		Version:              DefaultToolVersion,
	}
}
