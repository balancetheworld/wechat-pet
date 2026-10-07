package ask

// 本文件固定 record_array_v1 的机读 JSON Schema（文档 8.4）。
// 顶层是 {"records":[...]} 对象，records 是有序记录数组：首个为 header、末尾为 coverage + end，
// 中间记录与动作匹配。Schema 严格对齐 response_protocol.go 的类型与 JSON tag。
// Provider 的 strict 结构化输出要求根节点为 object、所有 properties 进入 required、
// additionalProperties 为 false，且不接受顶层数组与 oneOf，因此：
// 可省略标量用 nullable 表达，可省略数组用「必填空数组」表达，判别联合用 anyOf。
// 解析端（unwrapRecordArray）同时接受该外壳与顶层数组，便于非 strict 回退。

// RecordArraySchema 返回 record_array_v1 的 JSON Schema（文档 8.4）。
func RecordArraySchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"records"},
		"properties": map[string]any{
			"records": map[string]any{
				"type": "array",
				"items": map[string]any{
					"anyOf": []any{
						headerRecordSchema(),
						groupRecordSchema(),
						segmentRecordSchema(),
						riskRecordSchema(),
						questionRecordSchema(),
						callRecordSchema(),
					},
				},
			},
		},
	}
}

// IsRecordArraySchema 报告一个 Schema 是否为 record_array_v1（对象根 + records 数组）。
func IsRecordArraySchema(schema map[string]any) bool {
	if schema["type"] != "object" {
		return false
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	records, ok := properties["records"].(map[string]any)
	if !ok {
		return false
	}
	return records["type"] == "array"
}

func headerRecordSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"type", "schema_version", "action", "task_updates"},
		"properties": map[string]any{
			"type":           strEnum("header"),
			"schema_version": strEnum(RecordArrayV1),
			"action":         strEnum(string(ActionCallTools), string(ActionRequestInput), string(ActionFinalAnswer)),
			"task_updates": map[string]any{
				"type":  "array",
				"items": taskUpdateSchema(),
			},
		},
	}
}

func taskUpdateSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"task_key", "goal", "source_turn_ids", "subject_keys"},
		"properties": map[string]any{
			"task_key":        map[string]any{"type": "string"},
			"goal":            map[string]any{"type": "string"},
			"source_turn_ids": stringArraySchema(),
			"subject_keys":    stringArraySchema(),
		},
	}
}

func groupRecordSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"type", "group_key", "task_keys", "answer_kind", "subjects", "scope"},
		"properties": map[string]any{
			"type":        strEnum("group"),
			"group_key":   map[string]any{"type": "string"},
			"task_keys":   stringArraySchema(),
			"answer_kind": strEnum(string(AnswerCasual), string(AnswerFact), string(AnswerHealth)),
			"subjects": map[string]any{
				"type":  "array",
				"items": answerSubjectSchema(),
			},
			"scope": strEnum(string(ScopeFull), string(ScopeLimited), string(ScopeDeclined), string(ScopeUnavailable)),
		},
	}
}

func answerSubjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"subject_key", "kind", "pet_id", "description", "source_turn_ids"},
		"properties": map[string]any{
			"subject_key":     map[string]any{"type": "string"},
			"kind":            strEnum(string(SubjectPet), string(SubjectUnresolved)),
			"pet_id":          nullableStringSchema(),
			"description":     nullableStringSchema(),
			"source_turn_ids": stringArraySchema(),
		},
	}
}

func segmentRecordSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"type", "segment_key", "group_key", "subject_keys", "field", "text", "basis_kind", "evidence_refs"},
		"properties": map[string]any{
			"type":         strEnum("segment"),
			"segment_key":  map[string]any{"type": "string"},
			"group_key":    map[string]any{"type": "string"},
			"subject_keys": stringArraySchema(),
			"field":        strEnum("reply", "result", "observation", "possible_direction", "watch_item", "care_condition", "uncertainty", "risk", "next_action", "limitation"),
			"text":         map[string]any{"type": "string"},
			"basis_kind":   strEnum(string(BasisUserStatement), string(BasisImageObservation), string(BasisBusinessFact), string(BasisGeneralKnowledge), string(BasisSpeculation)),
			"evidence_refs": map[string]any{
				"type":  "array",
				"items": evidenceRefSchema(),
			},
		},
	}
}

func evidenceRefSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"source_type", "source_id", "version", "position"},
		"properties": map[string]any{
			"source_type": map[string]any{"type": "string"},
			"source_id":   map[string]any{"type": "string"},
			"version":     nullableStringSchema(),
			"position":    nullableStringSchema(),
		},
	}
}

func riskRecordSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"type", "group_key", "subject_key", "level", "evidence", "uncertainty"},
		"properties": map[string]any{
			"type":        strEnum("risk"),
			"group_key":   map[string]any{"type": "string"},
			"subject_key": map[string]any{"type": "string"},
			"level":       strEnum(string(RiskUnknown), string(RiskGreen), string(RiskYellow), string(RiskRed)),
			"evidence": map[string]any{
				"type":  "array",
				"items": evidenceRefSchema(),
			},
			"uncertainty": nullableStringSchema(),
		},
	}
}

func questionRecordSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"type", "question_key", "task_keys", "subject_keys", "text", "missing_fields", "purpose"},
		"properties": map[string]any{
			"type":         strEnum("question"),
			"question_key": map[string]any{"type": "string"},
			"task_keys":    stringArraySchema(),
			"subject_keys": stringArraySchema(),
			"text":         map[string]any{"type": "string"},
			"missing_fields": map[string]any{
				"type":  "array",
				"items": missingFieldSchema(),
			},
			"purpose": nullableStringSchema(),
		},
	}
}

func missingFieldSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"task_key", "subject_key", "field", "necessity", "known_value"},
		"properties": map[string]any{
			"task_key":    map[string]any{"type": "string"},
			"subject_key": nullableStringSchema(),
			"field":       map[string]any{"type": "string"},
			"necessity":   strEnum(string(NecessityBlocking), string(NecessitySupport)),
			"known_value": nullableStringSchema(),
		},
	}
}

func callRecordSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"type", "call_key", "task_keys", "tool_name", "catalog_version", "depends_on", "arguments"},
		"properties": map[string]any{
			"type":            strEnum("call"),
			"call_key":        map[string]any{"type": "string"},
			"task_keys":       stringArraySchema(),
			"tool_name":       map[string]any{"type": "string"},
			"catalog_version": map[string]any{"type": "string", "description": "该工具的目录版本；必须与【可用工具】列表中同名工具的 Version 字段完全一致"},
			"depends_on":      stringArraySchema(),
			"arguments":       map[string]any{"type": "string", "description": "工具参数的 JSON 字符串（对象内容需转义），例如 {\"pet_id\":\"pet-1\"}"},
		},
	}
}

func strEnum(values ...string) map[string]any {
	items := make([]any, 0, len(values))
	for _, v := range values {
		items = append(items, v)
	}
	return map[string]any{"type": "string", "enum": items}
}

func stringArraySchema() map[string]any {
	return map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "string"},
	}
}

func nullableStringSchema() map[string]any {
	return map[string]any{"type": []string{"string", "null"}}
}
