package ask

import "testing"

// 本文件校验 record_array_v1 机读 JSON Schema（文档 8.4）的结构正确性：
// 顶层数组、oneOf 八种记录、required 与可选字段对齐 response_protocol.go 的 JSON tag。

func oneOfRecordSchemas(t *testing.T) []map[string]any {
	t.Helper()
	schema := RecordArraySchema()
	if schema["type"] != "array" {
		t.Fatalf("top-level type = %v, want array", schema["type"])
	}
	if schema["title"] != RecordArrayV1 {
		t.Fatalf("title = %v, want %s", schema["title"], RecordArrayV1)
	}
	items, ok := schema["items"].(map[string]any)
	if !ok {
		t.Fatalf("items missing or not an object: %v", schema["items"])
	}
	oneOf, ok := items["oneOf"].([]any)
	if !ok {
		t.Fatalf("items.oneOf missing or not an array: %v", items["oneOf"])
	}
	if len(oneOf) != 8 {
		t.Fatalf("oneOf length = %d, want 8", len(oneOf))
	}
	records := make([]map[string]any, 0, len(oneOf))
	for _, raw := range oneOf {
		m, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("oneOf element is not an object: %v", raw)
		}
		records = append(records, m)
	}
	return records
}

func TestRecordArraySchemaOneOfCoversAllRecordTypes(t *testing.T) {
	records := oneOfRecordSchemas(t)
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
			t.Fatalf("missing record type %q in oneOf", rt)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("unexpected record types: %v", seen)
	}
}

func TestRecordArraySchemaHeaderRequiredFields(t *testing.T) {
	records := oneOfRecordSchemas(t)
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

func TestRecordArraySchemaOptionalFieldsNotRequired(t *testing.T) {
	records := oneOfRecordSchemas(t)
	segment := findRecordSchema(records, "segment")
	if segment == nil {
		t.Fatal("segment record missing")
	}
	required := requiredKeys(t, segment)
	if contains(required, "evidence_refs") {
		t.Fatal("evidence_refs is optional (omitempty), should not be required")
	}
	if !contains(required, "field") || !contains(required, "text") {
		t.Fatalf("segment required missing field/text, got %v", required)
	}

	group := findRecordSchema(records, "group")
	if group == nil {
		t.Fatal("group record missing")
	}
	// group 的 subjects 数组内 AnswerSubject 的 description/pet_id 为 omitempty，不得 required。
	subjects := group["properties"].(map[string]any)["subjects"].(map[string]any)
	items := subjects["items"].(map[string]any)
	subjectRequired := requiredKeys(t, items)
	if contains(subjectRequired, "pet_id") || contains(subjectRequired, "description") {
		t.Fatalf("subject pet_id/description optional, got required %v", subjectRequired)
	}
}

func TestIsRecordArraySchema(t *testing.T) {
	if !IsRecordArraySchema(RecordArraySchema()) {
		t.Fatal("record_array_v1 should be detected as array schema")
	}
	if IsRecordArraySchema(map[string]any{"type": "object"}) {
		t.Fatal("object schema should not be detected as array")
	}
	if IsRecordArraySchema(map[string]any{}) {
		t.Fatal("empty schema should not be detected as array")
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
