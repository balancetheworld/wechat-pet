package knowledge

import (
	"strings"
	"testing"
)

// sampleEntry 是开发者交来的第一条知识文本原文，解析结果必须与人工阅读一致。
const sampleEntry = `主题：猫咪呕吐
适用宠物：猫
原始来源：某宠物医院科普文章
来源日期：2026-09-20

正文：
猫咪偶尔吐出未消化的猫粮，可能和进食过快、一次进食过多有关。如果只是发生一次，猫咪之后精神正常，愿意喝水和进食，可以先观察。

如果猫咪连续呕吐，无法喝水，精神很差，呕吐物带血，或者可能误食了线、塑料、药物和有毒物质，应尽快联系宠物医生。

不要自行给猫咪使用人用止吐药，也不能仅凭呕吐判断具体疾病。
`

func TestParseEntryDerivesFields(t *testing.T) {
	chunk, err := ParseEntry(sampleEntry)
	if err != nil {
		t.Fatalf("ParseEntry error: %v", err)
	}
	if chunk.Title != "猫咪呕吐" {
		t.Fatalf("title = %q, want 猫咪呕吐", chunk.Title)
	}
	if chunk.Species != SpeciesCat {
		t.Fatalf("species = %q, want %q", chunk.Species, SpeciesCat)
	}
	if chunk.SourceName != "某宠物医院科普文章" {
		t.Fatalf("source_name = %q", chunk.SourceName)
	}
	if chunk.SourceDate.Format(sourceDateLayout) != "2026-09-20" {
		t.Fatalf("source_date = %v", chunk.SourceDate)
	}
	if chunk.RiskLevel != RiskYellow {
		t.Fatalf("risk_level = %q, want %q", chunk.RiskLevel, RiskYellow)
	}
	if chunk.Status != StatusPublished {
		t.Fatalf("status = %q, want %q", chunk.Status, StatusPublished)
	}
	if !strings.Contains(chunk.Content, "不要自行给猫咪使用人用止吐药") {
		t.Fatalf("content missing 用药限制段落: %q", chunk.Content)
	}
}

func TestParseEntryExtractsEscalationConditions(t *testing.T) {
	chunk, err := ParseEntry(sampleEntry)
	if err != nil {
		t.Fatalf("ParseEntry error: %v", err)
	}
	if len(chunk.EscalationConditions) != 1 {
		t.Fatalf("escalation_conditions = %v, want 1 paragraph", chunk.EscalationConditions)
	}
	condition := chunk.EscalationConditions[0]
	if !strings.Contains(condition, "应尽快联系宠物医生") {
		t.Fatalf("escalation condition missing 就医建议: %q", condition)
	}
	if strings.Contains(condition, "可以先观察") {
		t.Fatalf("普通观察建议不应进入升级条件: %q", condition)
	}
	if !chunk.HasEscalationConditions() {
		t.Fatal("chunk with red flag conditions should report HasEscalationConditions")
	}
}

func TestParseEntryRejectsIncompleteText(t *testing.T) {
	if _, err := ParseEntry("主题：猫咪呕吐\n适用宠物：猫\n"); err == nil {
		t.Fatal("entry without 正文 should fail")
	}
	if _, err := ParseEntry("主题：猫咪呕吐\n适用宠物：仓鼠\n\n正文：\n内容\n"); err == nil {
		t.Fatal("entry with unknown species should fail")
	}
	if _, err := ParseEntry("主题：猫咪呕吐\n\n正文：\n内容\n"); err == nil {
		t.Fatal("entry without 适用宠物 should fail")
	}
	if _, err := ParseEntry("主题：猫咪呕吐\n适用宠物：猫\n来源日期：2026/09/20\n\n正文：\n内容\n"); err == nil {
		t.Fatal("entry with invalid date should fail")
	}
}

func TestParseEntryKeepsExplicitEscalationConditions(t *testing.T) {
	chunk, err := ParseEntry("主题：猫咪腹泻\n适用宠物：猫\n升级条件：便中带血、持续腹泻超过一天\n\n正文：\n轻微软便可以先观察。\n")
	if err != nil {
		t.Fatalf("ParseEntry error: %v", err)
	}
	if len(chunk.EscalationConditions) != 2 {
		t.Fatalf("escalation_conditions = %v, want 2 declared items", chunk.EscalationConditions)
	}
	if chunk.EscalationConditions[0] != "便中带血" || chunk.EscalationConditions[1] != "持续腹泻超过一天" {
		t.Fatalf("explicit escalation conditions not preserved: %v", chunk.EscalationConditions)
	}
}

func TestRedFlagTermsAndTopicCodes(t *testing.T) {
	terms := RedFlagTerms("猫咪呕吐物带血，而且无法喝水")
	if len(terms) == 0 {
		t.Fatal("expected red flag terms for 带血/无法喝水")
	}
	codes := TopicCodes("猫咪呕吐怎么办")
	if len(codes) != 1 || codes[0] != "vomiting" {
		t.Fatalf("topic codes = %v, want [vomiting]", codes)
	}
}

func TestNormalizeSpeciesAcceptsProfileWording(t *testing.T) {
	for _, value := range []string{"猫", "猫咪", "中华田园猫", "cat", "Feline"} {
		if got := NormalizeSpecies(value); got != "cat" {
			t.Fatalf("NormalizeSpecies(%q) = %q, want cat", value, got)
		}
	}
	for _, value := range []string{"狗", "狗狗", "犬", "dog", "Canine"} {
		if got := NormalizeSpecies(value); got != "dog" {
			t.Fatalf("NormalizeSpecies(%q) = %q, want dog", value, got)
		}
	}
	// 未确认或不限一律返回空串，调用方不得据此猜测或替代。
	for _, value := range []string{"", "  ", "不限", "通用", "仓鼠"} {
		if got := NormalizeSpecies(value); got != "" {
			t.Fatalf("NormalizeSpecies(%q) = %q, want empty", value, got)
		}
	}
}

func TestParseLifeStagesAcceptsAllFourStages(t *testing.T) {
	chunk, err := ParseEntry("主题：猫咪呕吐\n适用宠物：猫\n生命周期：幼年、青年、熟龄、老年\n\n正文：\n内容\n")
	if err != nil {
		t.Fatalf("ParseEntry error: %v", err)
	}
	want := []LifeStage{LifeStageYoung, LifeStageAdult, LifeStageMature, LifeStageSenior}
	if len(chunk.LifeStage) != len(want) {
		t.Fatalf("life stages = %v, want %v", chunk.LifeStage, want)
	}
	for index, stage := range want {
		if chunk.LifeStage[index] != stage {
			t.Fatalf("life stages = %v, want %v", chunk.LifeStage, want)
		}
	}
}

func TestNewCatalogAssignsStableReferenceIDs(t *testing.T) {
	first, err := ParseEntry("主题：猫咪呕吐\n适用宠物：猫\n\n正文：\n第一次。\n")
	if err != nil {
		t.Fatalf("ParseEntry error: %v", err)
	}
	second, err := ParseEntry("主题：猫咪呕吐\n适用宠物：猫\n风险等级：red\n\n正文：\n第二次。\n")
	if err != nil {
		t.Fatalf("ParseEntry error: %v", err)
	}
	catalog, err := NewCatalog([]Chunk{first, second}, "2026-10-05.v1", whitespaceTokenizer)
	if err != nil {
		t.Fatalf("NewCatalog error: %v", err)
	}
	chunks := catalog.Chunks()
	if len(chunks) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(chunks))
	}
	if chunks[0].ReferenceID != "pet_health.vomiting.cat.001" {
		t.Fatalf("first reference_id = %q", chunks[0].ReferenceID)
	}
	if chunks[1].ReferenceID != "pet_health.vomiting.cat.002" {
		t.Fatalf("second reference_id = %q", chunks[1].ReferenceID)
	}
	if chunks[0].Version != "2026-10-05.v1" {
		t.Fatalf("chunk version = %q, want catalog version", chunks[0].Version)
	}
	if catalog.Digest() == "" {
		t.Fatal("catalog digest should not be empty")
	}
}

func TestNewCatalogRequiresVersion(t *testing.T) {
	if _, err := NewCatalog(nil, "  ", whitespaceTokenizer); err == nil {
		t.Fatal("catalog without version should fail")
	}
}

func TestEmbeddedCatalogLoadsSeedEntry(t *testing.T) {
	catalog, err := EmbeddedCatalog(whitespaceTokenizer)
	if err != nil {
		t.Fatalf("EmbeddedCatalog error: %v", err)
	}
	if catalog.Version() != "2026-10-06.v2" {
		t.Fatalf("catalog version = %q", catalog.Version())
	}
	chunks := catalog.Chunks()
	var seed *Chunk
	for index := range chunks {
		if chunks[index].SourceName == "某宠物医院科普文章" {
			seed = &chunks[index]
			break
		}
	}
	if seed == nil {
		t.Fatalf("embedded catalog missing the reviewed seed entry: %+v", chunks)
	}
	if seed.ReferenceID != "pet_health.vomiting.cat.observation" {
		t.Fatalf("embedded reference_id = %q", seed.ReferenceID)
	}
}

// whitespaceTokenizer 是测试用确定性分词器：按空白切分后逐字符展开，
// 让中文查询与条目在没有 gse 词典时也能产生稳定词法重合。
func whitespaceTokenizer(text string) []string {
	fields := strings.Fields(text)
	result := make([]string, 0, len(text))
	for _, field := range fields {
		for _, r := range field {
			result = append(result, string(r))
		}
	}
	return result
}
