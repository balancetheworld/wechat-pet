package ask

import "testing"

// 本文件校验 record_array_v1 机读 JSON Schema（文档 8.4）的结构正确性：
// 对象根 + records 数组、anyOf 八种记录、strict 要求（全部属性必填、additionalProperties=false）。

func anyOfRecordSchemas(t *testing.T) []map[string]any {
	t.Helper()
	schema := RecordArraySchema()
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("top-level schema = %v, want strict object", schema)
	}
	if required := requiredKeys(t, schema); len(required) != 1 || required[0] != "records" {
		t.Fatalf("top-level required = %v, want [records]", required)
	}
	properties := schema["properties"].(map[string]any)
	recordsField, ok := properties["records"].(map[string]any)
	if !ok {
		t.Fatalf("records missing or not an object: %v", properties["records"])
	}
	if recordsField["type"] != "array" {
		t.Fatalf("records type = %v, want array", recordsField["type"])
	}
	items, ok := recordsField["items"].(map[string]any)
	if !ok {
		t.Fatalf("records.items missing or not an object: %v", recordsField["items"])
	}
	anyOf, ok := items["anyOf"].([]any)
	if !ok {
		t.Fatalf("records.items.anyOf missing or not an array: %v", items["anyOf"])
	}
	if len(anyOf) != 8 {
		t.Fatalf("anyOf length = %d, want 8", len(anyOf))
	}
	records := make([]map[string]any, 0, len(anyOf))
	for _, raw := range anyOf {
		m, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("anyOf element is not an object: %v", raw)
		}
		records = append(records, m)
	}
	return records
}

func TestRecordArraySchemaAnyOfCoversAllRecordTypes(t *testing.T) {
	records := anyOfRecordSchemas(t)
	want := []RecordType{
		RecordHeader, RecordGroup, RecordSegment, RecordRisk,
		RecordQuestion, RecordCall, RecordCoverage, RecordEnd,
	}
	seen := make(map[string]bool)
	for _, rec := range records {
		props, ok := rec["properties"].(map[string]any)
		if !ok {
			t.Fatalf("record missing properties: %v", rec)
		}
		typeSchema, ok := props["type"].(map[string]any)
		if !ok {
			t.Fatalf("record missing type schema: %v", rec)
		}
		enum, ok := typeSchema["enum"].([]any)
		if !ok || len(enum) != 1 {
			t.Fatalf("record type enum malformed: %v", typeSchema)
		}
		seen[enum[0].(string)] = true
	}
	for _, rt := range want {
		if !seen[string(rt)] {
			t.Fatalf("missing record type %q in anyOf", rt)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("unexpected record types: %v", seen)
	}
}

func TestRecordArraySchemaHeaderRequiredFields(t *testing.T) {
	records := anyOfRecordSchemas(t)
	header := findRecordSchema(records, "header")
	if header == nil {
		t.Fatal("header record missing")
	}
	required := requiredKeys(t, header)
	for _, key := range []string{"type", "schema_version", "action", "task_updates"} {
		if !contains(required, key) {
			t.Fatalf("header required missing %q, got %v", key, required)
		}
	}
	props := header["properties"].(map[string]any)
	if props["action"].(map[string]any)["enum"] == nil {
		t.Fatal("header.action should be an enum of three actions")
	}
}

func TestRecordArraySchemaKeepsEveryPropertyRequired(t *testing.T) {
	records := anyOfRecordSchemas(t)
	segment := findRecordSchema(records, "segment")
	if segment == nil {
		t.Fatal("segment record missing")
	}
	required := requiredKeys(t, segment)
	if !contains(required, "evidence_refs") {
		t.Fatal("strict schema requires every property, evidence_refs should be required")
	}
	if !contains(required, "field") || !contains(required, "text") {
		t.Fatalf("segment required missing field/text, got %v", required)
	}

	group := findRecordSchema(records, "group")
	if group == nil {
		t.Fatal("group record missing")
	}
	// 可省略标量改为必填 nullable，可省略数组改为必填空数组。
	subjects := group["properties"].(map[string]any)["subjects"].(map[string]any)
	items := subjects["items"].(map[string]any)
	subjectRequired := requiredKeys(t, items)
	if !contains(subjectRequired, "pet_id") || !contains(subjectRequired, "description") {
		t.Fatalf("subject pet_id/description must be required nullable, got %v", subjectRequired)
	}
	subjectProperties := items["properties"].(map[string]any)
	for _, name := range []string{"pet_id", "description"} {
		types, ok := subjectProperties[name].(map[string]any)["type"].([]string)
		if !ok || len(types) != 2 || types[1] != "null" {
			t.Fatalf("subject %s type = %v, want nullable string", name, subjectProperties[name])
		}
	}
}

func TestRecordArraySchemaGroupTaskKeysIsRequiredArray(t *testing.T) {
	group := findRecordSchema(anyOfRecordSchemas(t), "group")
	if !contains(requiredKeys(t, group), "task_keys") {
		t.Fatal("group.task_keys should be required")
	}
	taskKeys := group["properties"].(map[string]any)["task_keys"].(map[string]any)
	if taskKeys["type"] != "array" || taskKeys["minItems"] != nil {
		t.Fatalf("group.task_keys = %v, want strict-compatible string array", taskKeys)
	}
}

func TestRecordArraySchemaCallArgumentsIsJSONString(t *testing.T) {
	call := findRecordSchema(anyOfRecordSchemas(t), "call")
	if call == nil {
		t.Fatal("call record missing")
	}
	arguments := call["properties"].(map[string]any)["arguments"].(map[string]any)
	if arguments["type"] != "string" {
		t.Fatalf("call.arguments type = %v, want string (strict 要求 additionalProperties=false)", arguments["type"])
	}
	if arguments["additionalProperties"] != nil {
		t.Fatalf("call.arguments should not declare additionalProperties: %v", arguments)
	}
}

func TestRecordArraySchemaSegmentFieldsMatchAnswerValidation(t *testing.T) {
	segment := findRecordSchema(anyOfRecordSchemas(t), "segment")
	field := segment["properties"].(map[string]any)["field"].(map[string]any)
	values := field["enum"].([]any)
	for _, value := range values {
		name := value.(string)
		if !allowedFieldForKind(AnswerCasual, name) && !allowedFieldForKind(AnswerFact, name) && !allowedFieldForKind(AnswerHealth, name) {
			t.Fatalf("schema field %q is not allowed by any answer kind", name)
		}
	}
	for _, name := range []string{"reply", "result", "observation", "possible_direction", "watch_item", "care_condition", "uncertainty", "risk", "next_action", "limitation"} {
		found := false
		for _, value := range values {
			if value == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("schema is missing field %q", name)
		}
	}
}

func TestIsRecordArraySchema(t *testing.T) {
	if !IsRecordArraySchema(RecordArraySchema()) {
		t.Fatal("record_array_v1 should be detected as record schema")
	}
	if IsRecordArraySchema(map[string]any{"type": "object"}) {
		t.Fatal("plain object schema should not be detected as record schema")
	}
	if IsRecordArraySchema(map[string]any{"type": "array", "items": map[string]any{}}) {
		t.Fatal("top-level array schema should not be detected as record schema")
	}
	if IsRecordArraySchema(map[string]any{}) {
		t.Fatal("empty schema should not be detected as record schema")
	}
}

func findRecordSchema(records []map[string]any, typeName string) map[string]any {
	for _, rec := range records {
		props, ok := rec["properties"].(map[string]any)
		if !ok {
			continue
		}
		typeSchema, ok := props["type"].(map[string]any)
		if !ok {
			continue
		}
		enum, ok := typeSchema["enum"].([]any)
		if !ok || len(enum) == 0 {
			continue
		}
		if enum[0] == typeName {
			return rec
		}
	}
	return nil
}

func requiredKeys(t *testing.T, schema map[string]any) []string {
	t.Helper()
	raw, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required missing or wrong type: %v", schema["required"])
	}
	return raw
}

func contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}
