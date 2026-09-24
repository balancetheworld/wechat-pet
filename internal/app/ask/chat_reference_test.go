package ask

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func insertReferenceSession(t *testing.T, db *sql.DB, id, family, user, pet, status string, createdAt time.Time) {
	t.Helper()
	if _, err := db.Exec("INSERT INTO ask_sessions (id, family_id, pet_id, resolved_pet_id, created_by, status, risk_level, turn_count, prompt_version, rule_version, knowledge_version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, 'unknown', 0, 'prompt-v1', 'rule-v1', 'knowledge-v1', ?, ?)", id, family, pet, pet, user, status, createdAt, createdAt); err != nil {
		t.Fatal(err)
	}
}

func insertReferenceMessage(t *testing.T, db *sql.DB, sessionID, turnID, runID, msgID, role, content string, turnIndex, rowVersion int, createdAt time.Time, supersededBy string) {
	t.Helper()
	if _, err := db.Exec("INSERT INTO ask_turns (id, session_id, turn_index, status, input, selected_run_id, superseded_by, created_at) VALUES (?, ?, ?, 'attached', ?, ?, ?, ?)", turnID, sessionID, turnIndex, content, runID, supersededBy, createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO ask_runs (id, session_id, turn_id, run_index, row_version, status, risk_level, rule_version, prompt_version, created_at) VALUES (?, ?, ?, 0, ?, 'completed', 'green', 'rule-v1', 'prompt-v1', ?)", runID, sessionID, turnID, rowVersion, createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO ask_messages (id, session_id, turn_id, run_id, role, content, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)", msgID, sessionID, turnID, runID, role, content, createdAt); err != nil {
		t.Fatal(err)
	}
}

func TestReferenceScopeKeepsMostRecentThree(t *testing.T) {
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	sessions := []Session{
		{ID: "s5", CreatedAt: base},
		{ID: "s4", CreatedAt: base.Add(-time.Hour)},
		{ID: "s3", CreatedAt: base.Add(-2 * time.Hour)},
		{ID: "s2", CreatedAt: base.Add(-3 * time.Hour)},
		{ID: "s1", CreatedAt: base.Add(-4 * time.Hour)},
	}
	scope := ReferenceScope("s5", sessions)
	if len(scope) != 3 || scope[0] != "s5" || scope[1] != "s4" || scope[2] != "s3" {
		t.Fatalf("scope = %v", scope)
	}
}

func TestReferenceScopeAlwaysIncludesCurrent(t *testing.T) {
	// 当前 Session 不在已列出的最近列表里（异常数据），仍须包含在范围内。
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	sessions := []Session{
		{ID: "s4", CreatedAt: base},
		{ID: "s3", CreatedAt: base.Add(-time.Hour)},
		{ID: "s2", CreatedAt: base.Add(-2 * time.Hour)},
	}
	scope := ReferenceScope("s1", sessions)
	found := false
	for _, id := range scope {
		if id == "s1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("scope = %v, current session missing", scope)
	}
}

func TestSearchReferencesReturnsMatchesWithSource(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	insertReferenceSession(t, db, "s2", "family-1", "user-1", "pet-1", "active", base.Add(-2*time.Hour))
	insertReferenceMessage(t, db, "s2", "turn-2", "run-2", "msg-2a", "user", "旺仔今天吐了", 0, 1, base.Add(-90*time.Minute), "")
	insertReferenceMessage(t, db, "s2", "turn-3", "run-3", "msg-2b", "assistant", "可能是消化不良，先观察", 1, 2, base.Add(-80*time.Minute), "")
	insertReferenceSession(t, db, "s1", "family-1", "user-1", "pet-1", "closed", base.Add(-3*time.Hour))
	insertReferenceMessage(t, db, "s1", "turn-1", "run-1", "msg-1a", "assistant", "上次旺仔呕吐是两天前", 0, 1, base.Add(-170*time.Minute), "")

	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	reference := NewChatReferenceRepository(repository)

	outcome, err := reference.SearchReferences(context.Background(), ChatReferenceQuery{
		FamilyID:         "family-1",
		UserID:           "user-1",
		CurrentSessionID: "s2",
		Keywords:         []string{"呕吐"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != ChatReferenceOK {
		t.Fatalf("status = %s", outcome.Status)
	}
	if len(outcome.Matches) != 1 || outcome.Matches[0].MessageID != "msg-1a" {
		t.Fatalf("matches = %+v", outcome.Matches)
	}
	match := outcome.Matches[0]
	if match.SessionID != "s1" || match.Role != "assistant" || match.AnswerVersion != 1 {
		t.Fatalf("match source = %+v", match)
	}
	if len(outcome.Scope) != 2 || outcome.Scope[0] != "s2" || outcome.Scope[1] != "s1" {
		t.Fatalf("scope = %v", outcome.Scope)
	}
	if outcome.Source.Version == "" {
		t.Fatalf("source version empty")
	}
}

func TestSearchReferencesExcludesFailedRunButKeepsSessionMessages(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	insertReferenceSession(t, db, "s1", "family-1", "user-1", "pet-1", "active", base)
	insertReferenceMessage(t, db, "s1", "turn-1", "run-1", "msg-1", "user", "成功的呕吐记录", 0, 1, base, "")
	insertReferenceMessage(t, db, "s1", "turn-2", "run-2", "msg-2", "user", "失败的呕吐提问", 1, 1, base.Add(time.Minute), "")
	if _, err := db.Exec("UPDATE ask_runs SET status = 'failed' WHERE id = ?", "run-2"); err != nil {
		t.Fatal(err)
	}
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	messages, err := repository.ListSessionMessages(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].ID != "msg-2" {
		t.Fatalf("session messages = %+v", messages)
	}
	turns, err := repository.ListContextTurns(context.Background(), "s1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[1].Input != "失败的呕吐提问" {
		t.Fatalf("context turns = %+v", turns)
	}
	reference := NewChatReferenceRepository(repository)
	outcome, err := reference.SearchReferences(context.Background(), ChatReferenceQuery{
		FamilyID: "family-1", UserID: "user-1", CurrentSessionID: "s1", Keywords: []string{"呕吐"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != ChatReferenceOK || len(outcome.Matches) != 1 || outcome.Matches[0].MessageID != "msg-1" {
		t.Fatalf("references = %+v", outcome)
	}
}

func TestSearchReferencesNotFoundVersusError(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	insertReferenceSession(t, db, "s1", "family-1", "user-1", "pet-1", "active", base)
	insertReferenceMessage(t, db, "s1", "turn-1", "run-1", "msg-1", "assistant", "无关键词内容", 0, 1, base, "")

	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	reference := NewChatReferenceRepository(repository)

	// 无命中：status=not_found，error=nil，不以空数组冒充读取故障。
	outcome, err := reference.SearchReferences(context.Background(), ChatReferenceQuery{
		FamilyID:         "family-1",
		UserID:           "user-1",
		CurrentSessionID: "s1",
		Keywords:         []string{"不存在的关键词"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != ChatReferenceNotFound {
		t.Fatalf("status = %s, want not_found", outcome.Status)
	}

	// 读取故障：关闭 db 后触发 error，与无命中分开。
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	outcome, err = reference.SearchReferences(context.Background(), ChatReferenceQuery{
		FamilyID:         "family-1",
		UserID:           "user-1",
		CurrentSessionID: "s1",
	})
	if err == nil {
		t.Fatalf("expected read failure")
	}
	if outcome.Status != ChatReferenceError {
		t.Fatalf("status = %s, want error", outcome.Status)
	}
}

func TestSearchReferencesScopesToFamilyAndUser(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	// 跨家庭、跨用户的 Session 不得进入范围。
	insertReferenceSession(t, db, "s-other-family", "family-2", "user-1", "pet-1", "closed", base)
	insertReferenceMessage(t, db, "s-other-family", "turn-x", "run-x", "msg-x", "assistant", "跨家庭呕吐记录", 0, 1, base, "")
	insertReferenceSession(t, db, "s-other-user", "family-1", "user-2", "pet-1", "closed", base.Add(-time.Hour))
	insertReferenceMessage(t, db, "s-other-user", "turn-y", "run-y", "msg-y", "assistant", "跨用户呕吐记录", 0, 1, base.Add(-time.Hour), "")
	insertReferenceSession(t, db, "s1", "family-1", "user-1", "pet-1", "active", base.Add(-2*time.Hour))
	insertReferenceMessage(t, db, "s1", "turn-1", "run-1", "msg-1", "assistant", "本人呕吐记录", 0, 1, base.Add(-2*time.Hour), "")

	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	reference := NewChatReferenceRepository(repository)

	outcome, err := reference.SearchReferences(context.Background(), ChatReferenceQuery{
		FamilyID:         "family-1",
		UserID:           "user-1",
		CurrentSessionID: "s1",
		Keywords:         []string{"呕吐"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != ChatReferenceOK || len(outcome.Matches) != 1 || outcome.Matches[0].MessageID != "msg-1" {
		t.Fatalf("outcome = %+v", outcome)
	}

	// 跨家庭伪造消息 ID：不在授权范围，不返回。
	forged, err := reference.SearchReferences(context.Background(), ChatReferenceQuery{
		FamilyID:         "family-1",
		UserID:           "user-1",
		CurrentSessionID: "s1",
		MessageIDs:       []string{"msg-x", "msg-y"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if forged.Status != ChatReferenceNotFound || len(forged.Matches) != 0 {
		t.Fatalf("forged = %+v", forged)
	}
}

func TestSearchReferencesTruncates(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	insertReferenceSession(t, db, "s1", "family-1", "user-1", "pet-1", "active", base)
	for i := 0; i < 5; i++ {
		insertReferenceMessage(t, db, "s1", "turn-"+string(rune('a'+i)), "run-"+string(rune('a'+i)), "msg-"+string(rune('a'+i)), "assistant", "呕吐记录"+string(rune('a'+i)), i, 1, base.Add(time.Duration(i)*time.Minute), "")
	}
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	reference := NewChatReferenceRepository(repository)

	outcome, err := reference.SearchReferences(context.Background(), ChatReferenceQuery{
		FamilyID:         "family-1",
		UserID:           "user-1",
		CurrentSessionID: "s1",
		Keywords:         []string{"呕吐"},
		Limit:            3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != ChatReferenceOK || !outcome.Truncated || len(outcome.Matches) != 3 {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestSearchReferencesFiltersByPet(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	insertReferenceSession(t, db, "s-pet1", "family-1", "user-1", "pet-1", "active", base)
	insertReferenceMessage(t, db, "s-pet1", "turn-1", "run-1", "msg-1", "assistant", "旺仔呕吐", 0, 1, base, "")
	insertReferenceSession(t, db, "s-pet2", "family-1", "user-1", "pet-2", "closed", base.Add(-time.Hour))
	insertReferenceMessage(t, db, "s-pet2", "turn-2", "run-2", "msg-2", "assistant", "球球呕吐", 0, 1, base.Add(-time.Hour), "")

	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	reference := NewChatReferenceRepository(repository)

	outcome, err := reference.SearchReferences(context.Background(), ChatReferenceQuery{
		FamilyID:         "family-1",
		UserID:           "user-1",
		CurrentSessionID: "s-pet1",
		PetID:            "pet-1",
		Keywords:         []string{"呕吐"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != ChatReferenceOK || len(outcome.Matches) != 1 || outcome.Matches[0].MessageID != "msg-1" {
		t.Fatalf("outcome = %+v", outcome)
	}
}
