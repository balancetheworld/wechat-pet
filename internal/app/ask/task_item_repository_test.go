package ask

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestTaskItemJSONRoundTrip(t *testing.T) {
	item := TaskItem{
		TaskItemID:    "task-1",
		OriginTurnID:  "turn-1",
		RunID:         "run-1",
		ItemRevision:  1,
		Goal:          "查看旺仔的记录",
		SourceTurnIDs: []string{"turn-1", "turn-2"},
		Subjects: []AnswerSubject{
			{SubjectKey: "pet-1", Kind: SubjectPet, PetID: "pet-1"},
			{SubjectKey: "u-1", Kind: SubjectUnresolved, Description: "照片里的宠物"},
		},
		Outcome: OutcomeNeedsInput,
		MissingFields: []MissingField{
			{TaskKey: "task-1", SubjectKey: "u-1", Field: "pet_identity", Necessity: string(NecessityBlocking)},
		},
		ResultRef:  &TaskResultRef{Kind: ResultRefToolResult, RefID: "tool-1", Version: "v1"},
		Supersedes: "task-0",
	}

	sourceTurnIDs, err := marshalTaskItemJSON(item.SourceTurnIDs)
	if err != nil {
		t.Fatal(err)
	}
	subjects, err := marshalTaskItemJSON(item.Subjects)
	if err != nil {
		t.Fatal(err)
	}
	missingFields, err := marshalTaskItemJSON(item.MissingFields)
	if err != nil {
		t.Fatal(err)
	}
	resultRef, err := marshalTaskItemJSON(item.ResultRef)
	if err != nil {
		t.Fatal(err)
	}

	if got := unmarshalTaskItemStringSlice(sourceTurnIDs); len(got) != 2 || got[0] != "turn-1" {
		t.Fatalf("source_turn_ids round trip failed: %v", got)
	}
	subjs := unmarshalTaskItemSubjects(subjects)
	if len(subjs) != 2 || subjs[0].PetID != "pet-1" || subjs[1].Kind != SubjectUnresolved {
		t.Fatalf("subjects round trip failed: %+v", subjs)
	}
	fields := unmarshalTaskItemMissingFields(missingFields)
	if len(fields) != 1 || fields[0].Field != "pet_identity" {
		t.Fatalf("missing_fields round trip failed: %+v", fields)
	}
	ref := unmarshalTaskItemResultRef(resultRef)
	if ref == nil || ref.RefID != "tool-1" || ref.Kind != ResultRefToolResult {
		t.Fatalf("result_ref round trip failed: %+v", ref)
	}
}

func TestTaskItemJSONNilHandling(t *testing.T) {
	// nil 字段组序列化为空串，反序列化不报错。
	if got, err := marshalTaskItemJSON(nil); err != nil || got != "" {
		t.Fatalf("nil should marshal to empty, got %q", got)
	}
	if got := unmarshalTaskItemStringSlice(""); got != nil {
		t.Fatalf("empty should unmarshal to nil, got %v", got)
	}
	if got := unmarshalTaskItemResultRef(""); got != nil {
		t.Fatalf("empty result_ref should unmarshal to nil, got %v", got)
	}
	// 非法 JSON 不静默保留部分结果，返回 nil。
	if got := unmarshalTaskItemSubjects("not-json"); got != nil {
		t.Fatalf("invalid JSON should unmarshal to nil, got %v", got)
	}
	// 合法空数组保留为空数组（区别于 nil）。
	if got := unmarshalTaskItemStringSlice("[]"); got == nil || len(got) != 0 {
		t.Fatalf("empty array should unmarshal to empty slice, got %v", got)
	}
}

func TestMarshalTaskItemJSONReturnsError(t *testing.T) {
	if _, err := marshalTaskItemJSON(make(chan int)); err == nil {
		t.Fatal("unserializable value should return an error")
	}
}

func TestCreateTaskItemsRollsBackWhenJSONSerializationFails(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	original := taskItemJSONMarshal
	calls := 0
	taskItemJSONMarshal = func(v any) ([]byte, error) {
		calls++
		if calls == 5 {
			return nil, errors.New("serialize failed")
		}
		return original(v)
	}
	t.Cleanup(func() { taskItemJSONMarshal = original })
	items := []TaskItem{
		{TaskItemID: "task-1", RunID: "run-1", OriginTurnID: "turn-1", Goal: "目标一", SourceTurnIDs: []string{"turn-1"}, Subjects: []AnswerSubject{}, MissingFields: []MissingField{}, ResultRef: &TaskResultRef{}},
		{TaskItemID: "task-2", RunID: "run-1", OriginTurnID: "turn-1", Goal: "目标二", SourceTurnIDs: []string{"turn-1"}, Subjects: []AnswerSubject{}, MissingFields: []MissingField{}, ResultRef: &TaskResultRef{}},
	}
	if err := repository.CreateTaskItems(context.Background(), items); err == nil {
		t.Fatal("CreateTaskItems should return serialization error")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM ask_task_items").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("serialization failure left partial rows: %d", count)
	}
}

func TestUpdateTaskItemDoesNotWriteWhenJSONSerializationFails(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	item := TaskItem{TaskItemID: "task-1", RunID: "run-1", OriginTurnID: "turn-1", Goal: "原目标", SourceTurnIDs: []string{"turn-1"}, Subjects: []AnswerSubject{}, MissingFields: []MissingField{}, ResultRef: &TaskResultRef{}}
	if err := repository.CreateTaskItems(context.Background(), []TaskItem{item}); err != nil {
		t.Fatal(err)
	}
	original := taskItemJSONMarshal
	taskItemJSONMarshal = func(any) ([]byte, error) { return nil, errors.New("serialize failed") }
	t.Cleanup(func() { taskItemJSONMarshal = original })
	item.Goal = "新目标"
	if err := repository.UpdateTaskItem(context.Background(), item); err == nil {
		t.Fatal("UpdateTaskItem should return serialization error")
	}
	items, err := repository.ListTaskItems(context.Background(), item.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Goal != "原目标" {
		t.Fatalf("serialization failure wrote update: %+v", items)
	}
}

func TestTaskResultRefJSONTags(t *testing.T) {
	ref := TaskResultRef{Kind: ResultRefOperation, RefID: "op-1", Version: "v2"}
	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if m["kind"] != "operation" || m["ref_id"] != "op-1" || m["version"] != "v2" {
		t.Fatalf("json tags wrong: %v", m)
	}
}
