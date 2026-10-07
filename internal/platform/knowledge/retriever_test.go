package knowledge

import (
	"context"
	"testing"
	"time"
)

// testChunk 构造测试条目，未列出的字段保持零值。
func testChunk(referenceID, title string, species Species, topics []string, content string, escalation []string) Chunk {
	return Chunk{
		ReferenceID:          referenceID,
		DocumentID:           referenceID,
		ChunkID:              referenceID,
		Title:                title,
		Content:              content,
		Species:              species,
		Topics:               topics,
		RiskLevel:            RiskYellow,
		Status:               StatusPublished,
		Version:              "2026-10-05.v1",
		EscalationConditions: escalation,
	}
}

func testCatalog(t *testing.T, chunks ...Chunk) *Catalog {
	t.Helper()
	catalog, err := NewCatalog(chunks, "2026-10-05.v1", whitespaceTokenizer)
	if err != nil {
		t.Fatalf("NewCatalog error: %v", err)
	}
	return catalog
}

func TestSearchFiltersBySpecies(t *testing.T) {
	catalog := testCatalog(t,
		testChunk("pet_health.vomiting.cat.001", "猫咪呕吐", SpeciesCat, []string{"vomiting"}, "猫咪呕吐后先观察精神和饮水。", nil),
		testChunk("pet_health.vomiting.dog.001", "狗狗呕吐", SpeciesDog, []string{"vomiting"}, "狗狗呕吐后先观察精神和饮水。", nil),
	)
	result, err := catalog.Search(context.Background(), Query{Text: "狗狗呕吐怎么办", Species: "dog"})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if result.Status != RetrievalMatched {
		t.Fatalf("status = %q, want matched", result.Status)
	}
	for _, hit := range result.Hits {
		if hit.Chunk.Species == SpeciesCat {
			t.Fatalf("species hard filter leaked cat chunk: %s", hit.Chunk.ReferenceID)
		}
	}
}

func TestSearchExcludesUnpublishedAndExpired(t *testing.T) {
	expired := testChunk("pet_health.vomiting.cat.002", "猫咪呕吐过期条目", SpeciesCat, []string{"vomiting"}, "猫咪呕吐后先观察精神和饮水。", nil)
	expired.ExpiresAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	withdrawn := testChunk("pet_health.vomiting.cat.003", "猫咪呕吐撤回条目", SpeciesCat, []string{"vomiting"}, "猫咪呕吐后先观察精神和饮水。", nil)
	withdrawn.Status = StatusWithdrawn
	draft := testChunk("pet_health.vomiting.cat.004", "猫咪呕吐草稿条目", SpeciesCat, []string{"vomiting"}, "猫咪呕吐后先观察精神和饮水。", nil)
	draft.Status = StatusDraft
	catalog := testCatalog(t, expired, withdrawn, draft)

	result, err := catalog.Search(context.Background(), Query{
		Text:    "猫咪呕吐怎么办",
		Species: "cat",
		Now:     time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if result.Status != RetrievalNoMatch || len(result.Hits) != 0 {
		t.Fatalf("expected no_match for unpublished corpus, got status=%q hits=%d", result.Status, len(result.Hits))
	}
}

func TestSearchReportsNoMatchInsteadOfFabricating(t *testing.T) {
	catalog := testCatalog(t,
		testChunk("pet_health.vomiting.cat.001", "猫咪呕吐", SpeciesCat, []string{"vomiting"}, "猫咪呕吐后先观察精神和饮水。", nil),
	)
	result, err := catalog.Search(context.Background(), Query{Text: "蜥蜴蜕皮怎么办"})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if result.Status != RetrievalNoMatch {
		t.Fatalf("status = %q, want no_match", result.Status)
	}
	if len(result.Hits) != 0 {
		t.Fatalf("no_match must not return hits: %v", result.Hits)
	}
}

func TestSearchKeepsRedFlagKnowledgeForRedFlagQuery(t *testing.T) {
	ordinary := testChunk("pet_health.vomiting.cat.001", "猫咪呕吐基础观察", SpeciesCat, []string{"vomiting"},
		"猫咪呕吐后先观察精神。猫咪呕吐后先观察饮水。猫咪呕吐后先观察进食。猫咪呕吐可能和进食过快有关。", nil)
	redFlag := testChunk("pet_health.vomiting.cat.002", "猫咪呕吐就医条件", SpeciesCat, []string{"vomiting"},
		"呕吐。",
		[]string{"连续呕吐，无法喝水，精神很差，呕吐物带血时应尽快联系宠物医生。"})
	catalog := testCatalog(t, ordinary, redFlag)

	result, err := catalog.Search(context.Background(), Query{Text: "猫咪呕吐物带血而且无法喝水", Species: "cat", Limit: 1})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if !result.RedFlagRequired {
		t.Fatal("red flag query should be marked as requiring red flag knowledge")
	}
	if !result.RedFlagSatisfied {
		t.Fatal("red flag knowledge exists and should be retained")
	}
	if len(result.Hits) != 1 || !result.Hits[0].RedFlag {
		t.Fatalf("red flag hit should be retained first, got %v", result.Hits)
	}
	if result.Hits[0].Chunk.ReferenceID != "pet_health.vomiting.cat.002" {
		t.Fatalf("retained hit = %q, want 就医条件条目", result.Hits[0].Chunk.ReferenceID)
	}
}

func TestSearchReportsUnsatisfiedRedFlagKnowledge(t *testing.T) {
	catalog := testCatalog(t,
		testChunk("pet_health.vomiting.cat.001", "猫咪呕吐基础观察", SpeciesCat, []string{"vomiting"}, "猫咪呕吐后先观察精神和饮水，呕吐可能和进食过快有关。", nil),
	)
	result, err := catalog.Search(context.Background(), Query{Text: "猫咪呕吐物带血而且无法喝水", Species: "cat"})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if !result.RedFlagRequired {
		t.Fatal("red flag query should be marked as requiring red flag knowledge")
	}
	if result.RedFlagSatisfied {
		t.Fatal("catalog has no red flag knowledge, satisfaction must stay false")
	}
	if len(result.Hits) == 0 {
		t.Fatal("ordinary hits should still be returned, satisfaction is reported separately")
	}
}

// TestSearchRetainsRedFlagHitWithoutMutatingHits 锁定红旗条目已在结果内时的保留行为：
// 只应把它排到最前，不能重复，也不能改写调用方持有的切片。
func TestSearchRetainsRedFlagHitWithoutMutatingHits(t *testing.T) {
	redFlag := testChunk("pet_health.vomiting.cat.900", "猫咪呕吐就医条件", SpeciesCat, []string{"vomiting"},
		"呕吐。", []string{"连续呕吐、无法喝水时应尽快联系宠物医生。"})
	ordinary := testChunk("pet_health.care.cat.901", "猫咪护理常识", SpeciesCat, nil,
		"呕吐后先观察精神状态和饮水。", nil)
	catalog := testCatalog(t, redFlag, ordinary)

	result, err := catalog.Search(context.Background(), Query{Text: "猫咪连续呕吐", Species: "cat"})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if !result.RedFlagSatisfied {
		t.Fatal("red flag knowledge is present and should be satisfied")
	}
	if len(result.Hits) != 2 {
		t.Fatalf("hits = %+v, want 2 unique entries", result.Hits)
	}
	if result.Hits[0].Chunk.ReferenceID != redFlag.ReferenceID {
		t.Fatalf("first hit = %q, want red flag entry", result.Hits[0].Chunk.ReferenceID)
	}
	if result.Hits[1].Chunk.ReferenceID != ordinary.ReferenceID {
		t.Fatalf("second hit = %q, want ordinary entry", result.Hits[1].Chunk.ReferenceID)
	}
}

func TestSearchRejectsEmptyQuery(t *testing.T) {
	catalog := testCatalog(t)
	if _, err := catalog.Search(context.Background(), Query{Text: "   "}); err == nil {
		t.Fatal("empty query should fail instead of returning no_match")
	}
}

func TestEmbeddedCatalogSearchReturnsSeedReference(t *testing.T) {
	catalog, err := EmbeddedCatalog(whitespaceTokenizer)
	if err != nil {
		t.Fatalf("EmbeddedCatalog error: %v", err)
	}
	result, err := catalog.Search(context.Background(), Query{Text: "猫咪连续呕吐，无法喝水", Species: "cat", Now: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if result.Status != RetrievalMatched {
		t.Fatalf("status = %q, want matched", result.Status)
	}
	if len(result.Hits) == 0 {
		t.Fatalf("unexpected hits: %+v", result.Hits)
	}
	if !result.RedFlagRequired || !result.RedFlagSatisfied {
		t.Fatalf("seed entry should satisfy red flag requirement: required=%t satisfied=%t", result.RedFlagRequired, result.RedFlagSatisfied)
	}
	for _, hit := range result.Hits {
		if hit.Chunk.ReferenceID == "pet_health.vomiting.cat.observation" {
			return
		}
	}
	t.Fatalf("seed reference missing from hits: %+v", result.Hits)
}

// TestEmbeddedCatalogTypicalQuestions 用生产分词器验证典型提问能召回对应主题，
// 作为知识条目的检索回归基线。条目变多后召回被挤掉时，这里会失败。
func TestEmbeddedCatalogTypicalQuestions(t *testing.T) {
	catalog, err := EmbeddedCatalog(nil)
	if err != nil {
		t.Fatalf("EmbeddedCatalog error: %v", err)
	}
	tests := []struct {
		name      string
		query     string
		species   string
		lifeStage string
		wantDoc   string
	}{
		{name: "cat vomiting", query: "我家猫今天吐了两次，第一次是没消化的猫粮", species: "cat", wantDoc: "pet_health.vomiting.cat"},
		{name: "kitten vomiting", query: "两个月大的幼猫吐了，还没打完疫苗", species: "cat", lifeStage: "young", wantDoc: "pet_health.vomiting.cat"},
		{name: "senior dog vomiting", query: "老年犬吃完饭就吐黄水，精神也不好", species: "dog", lifeStage: "senior", wantDoc: "pet_health.vomiting.dog"},
		{name: "dog diarrhea", query: "狗狗拉肚子，便便有点软，精神还行", species: "dog", wantDoc: "pet_health.diarrhea.dog"},
		{name: "cat constipation", query: "猫咪三天没排便了，总蹲猫砂盆", species: "cat", wantDoc: "pet_health.constipation.cat"},
		{name: "water intake", query: "猫咪最近喝水特别多，尿也变多了", species: "cat", wantDoc: "pet_health.water_intake.all"},
		{name: "lethargy", query: "宠物突然没精神，一直趴着不动", species: "cat", wantDoc: "pet_health.lethargy.all"},
		{name: "sneezing", query: "猫咪打喷嚏，鼻涕是黄绿色的", species: "cat", wantDoc: "pet_health.sneezing.all"},
		{name: "itching", query: "狗狗一直抓挠，还不停甩头", species: "dog", wantDoc: "pet_health.skin.all"},
		{name: "eye discharge", query: "宠物眼睛分泌物很多，眼睛睁不开", species: "cat", wantDoc: "pet_health.eye.all"},
		{name: "cough", query: "猫咪咳嗽，像干呕一样", species: "cat", wantDoc: "pet_health.cough.all"},
		{name: "appetite", query: "狗狗突然不吃东西了，精神还可以", species: "dog", wantDoc: "pet_health.appetite.all"},
		{name: "age stages", query: "猫咪几岁算老年猫", species: "cat", wantDoc: "pet_health.age.all"},
	}
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := catalog.Search(context.Background(), Query{Text: test.query, Species: test.species, LifeStage: test.lifeStage, Limit: maxInjectLimit, Now: now})
			if err != nil {
				t.Fatalf("Search error: %v", err)
			}
			documents := make([]string, 0, len(result.Hits))
			for _, hit := range result.Hits {
				if hit.Chunk.DocumentID == test.wantDoc {
					return
				}
				documents = append(documents, hit.Chunk.DocumentID)
			}
			t.Fatalf("query %q did not recall %q, got %v", test.query, test.wantDoc, documents)
		})
	}
}
