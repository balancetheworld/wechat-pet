package knowledge

import (
	"fmt"
	"strings"
	"time"

	"github.com/balancetheworld/wechat-pet/internal/platform/safety"
)

// 本文件解析开发者交来的知识文本（设计文档第五节「知识条目拆分原则」）。
// 一期使用纯文本条目，不引入在线编辑；发布与撤回仍在版本化流程中完成。
//
// 支持的书写格式：
//
//	主题：猫咪呕吐
//	适用宠物：猫
//	原始来源：某宠物医院科普文章
//	来源日期：2026-09-20
//
//	正文：
//	...
//
// 可选字段：生命周期、主题词、关键词、症状、风险等级、就诊等级、审核日期、
// 有效期至、知识版本、状态、支持、不支持、观察项、升级条件（别名：就医条件）、
// 禁忌、相关主题。未书写的字段保持为空，不由解析器编造。

// sourceDateLayout 是条目里「来源日期」的书写格式。
const sourceDateLayout = "2006-01-02"

// entryKeys 把中文书写字段映射为条目字段标识，未列出的键不参与解析。
var entryKeys = map[string]string{
	"主题":   "title",
	"适用宠物": "species",
	"原始来源": "source_name",
	"来源日期": "source_date",
	"生命周期": "life_stage",
	"主题词":  "topics",
	"关键词":  "keywords",
	"症状":   "symptoms",
	"风险等级": "risk_level",
	"就诊等级": "triage_level",
	"审核日期": "reviewed_at",
	"有效期至": "expires_at",
	"知识版本": "version",
	"状态":   "status",
	"支持":   "supports",
	"不支持":  "does_not_support",
	"观察项":  "observation_items",
	"升级条件": "escalation_conditions",
	"就医条件": "escalation_conditions",
	"禁忌":   "contraindications",
	"相关主题": "related_topics",
}

// topicCodes 把主题里的关键词映射为稳定的英文主题码，用于生成 document_id
// （设计示例：pet_health.vomiting.cat）。未命中的主题回退为原文小写化，
// 保证标识稳定，但不伪造成已知主题。
var topicCodes = []struct {
	keyword string
	code    string
}{
	{"呕吐", "vomiting"},
	{"吐了", "vomiting"},
	{"腹泻", "diarrhea"},
	{"拉稀", "diarrhea"},
	{"便秘", "constipation"},
	{"疫苗", "vaccine"},
	{"接种", "vaccine"},
	{"驱虫", "deworming"},
	{"咳嗽", "cough"},
	{"打喷嚏", "sneezing"},
	{"喷嚏", "sneezing"},
	{"呼吸", "breathing"},
	{"皮肤", "skin"},
	{"耳", "ear"},
	{"眼", "eye"},
	{"口腔", "dental"},
	{"牙齿", "dental"},
	{"饮食", "diet"},
	{"喂养", "diet"},
	{"喂食", "diet"},
	{"食欲", "appetite"},
	{"饮水", "water_intake"},
	{"精神", "lethargy"},
	{"误食", "poisoning"},
	{"中毒", "poisoning"},
	{"泌尿", "urinary"},
	{"排尿", "urinary"},
	{"关节", "joint"},
	{"体重", "weight"},
	{"年龄", "age"},
	{"掉毛", "shedding"},
	{"洗澡", "grooming"},
	{"清洁", "grooming"},
	{"绝育", "neuter"},
	{"发热", "fever"},
	{"发烧", "fever"},
	{"行为", "behavior"},
}

// ParseEntry 解析一条知识文本。缺必需字段或字段值非法时返回错误，不静默补猜。
func ParseEntry(text string) (Chunk, error) {
	meta, body, err := splitEntry(text)
	if err != nil {
		return Chunk{}, err
	}
	title := strings.TrimSpace(meta["title"])
	if title == "" {
		return Chunk{}, fmt.Errorf("knowledge: entry has no 主题")
	}
	species, err := parseSpecies(meta["species"])
	if err != nil {
		return Chunk{}, err
	}
	if strings.TrimSpace(body) == "" {
		return Chunk{}, fmt.Errorf("knowledge: entry %q has empty 正文", title)
	}
	risk, err := parseRiskLevel(meta["risk_level"])
	if err != nil {
		return Chunk{}, err
	}
	status, err := parseStatus(meta["status"])
	if err != nil {
		return Chunk{}, err
	}
	sourceDate, err := parseOptionalDate(meta["source_date"], "来源日期")
	if err != nil {
		return Chunk{}, err
	}
	reviewedAt, err := parseOptionalDate(meta["reviewed_at"], "审核日期")
	if err != nil {
		return Chunk{}, err
	}
	expiresAt, err := parseOptionalDate(meta["expires_at"], "有效期至")
	if err != nil {
		return Chunk{}, err
	}
	lifeStages := parseLifeStages(meta["life_stage"])
	if len(lifeStages) == 0 {
		// 「适用宠物」写成幼猫、幼犬、老年猫、老年犬时，同样代表作者声明的生命周期。
		lifeStages = parseLifeStages(meta["species"])
	}
	chunk := Chunk{
		Title:                title,
		Content:              strings.TrimSpace(body),
		Species:              species,
		LifeStage:            lifeStages,
		Topics:               splitList(meta["topics"]),
		Keywords:             splitList(meta["keywords"]),
		Symptoms:             splitList(meta["symptoms"]),
		RiskLevel:            risk,
		TriageLevel:          strings.TrimSpace(meta["triage_level"]),
		SourceName:           strings.TrimSpace(meta["source_name"]),
		SourceDate:           sourceDate,
		ReviewedAt:           reviewedAt,
		ExpiresAt:            expiresAt,
		Version:              strings.TrimSpace(meta["version"]),
		Status:               status,
		Supports:             splitList(meta["supports"]),
		DoesNotSupport:       splitList(meta["does_not_support"]),
		ObservationItems:     splitList(meta["observation_items"]),
		EscalationConditions: splitList(meta["escalation_conditions"]),
		Contraindications:    splitList(meta["contraindications"]),
		RelatedTopics:        splitList(meta["related_topics"]),
	}
	if len(chunk.EscalationConditions) == 0 {
		chunk.EscalationConditions = extractEscalationConditions(chunk.Content)
	}
	return chunk, nil
}

// splitEntry 拆出元数据键值对与正文。正文由「正文：」起始行确定，
// 起始行之后的全部内容都属于正文，避免正文里的「字段：值」被误当元数据。
func splitEntry(text string) (map[string]string, string, error) {
	meta := make(map[string]string)
	lines := strings.Split(normalizeNewlines(text), "\n")
	bodyStart := -1
	for index, line := range lines {
		key, value, ok := splitKeyValue(line)
		if !ok {
			continue
		}
		if key == "正文" {
			bodyStart = index
			break
		}
		field, known := entryKeys[key]
		if !known {
			continue
		}
		meta[field] = strings.TrimSpace(value)
	}
	if bodyStart < 0 {
		return nil, "", fmt.Errorf("knowledge: entry has no 正文")
	}
	body := strings.Join(lines[bodyStart+1:], "\n")
	return meta, body, nil
}

// splitKeyValue 解析形如「主题：猫咪呕吐」的一行。半角与全角冒号均接受。
func splitKeyValue(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	for _, separator := range []string{"：", ":"} {
		if index := strings.Index(trimmed, separator); index > 0 {
			return strings.TrimSpace(trimmed[:index]), strings.TrimSpace(trimmed[index+len(separator):]), true
		}
	}
	return "", "", false
}

func normalizeNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

// splitList 把「、，,；;换行」分隔的多值字段拆为有序去重列表。
func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case '、', '，', ',', '；', ';', '\n', '|':
			return true
		default:
			return false
		}
	})
	return sortUnique(fields)
}

// splitParagraphs 按空行切分正文段落，供红旗条件抽取使用。
func splitParagraphs(body string) []string {
	blocks := strings.Split(normalizeNewlines(body), "\n\n")
	result := make([]string, 0, len(blocks))
	for _, block := range blocks {
		trimmed := strings.TrimSpace(block)
		if trimmed == "" {
			continue
		}
		result = append(result, strings.Join(strings.Fields(trimmed), " "))
	}
	return result
}

// extractEscalationConditions 抽取带红旗关键词的段落。返回段落原文（压缩空白），
// 不做改写、不做诊断，只做定位：让红旗信息能独立于普通科普被保留。
func extractEscalationConditions(body string) []string {
	conditions := make([]string, 0)
	for _, paragraph := range splitParagraphs(body) {
		if containsRedFlagTerm(paragraph) {
			conditions = append(conditions, paragraph)
		}
	}
	return sortUnique(conditions)
}

// containsRedFlagTerm 报告文本是否命中红旗关键词表。
func containsRedFlagTerm(text string) bool {
	return safety.ContainsPhrase(text)
}

// RedFlagTerms 返回文本命中的红旗短语，词表来自项目唯一的红旗表
// （internal/platform/safety）。Runtime 据此判断是否需要强制保留升级条件。
func RedFlagTerms(text string) []string {
	return safety.Terms(text)
}

// TopicCodes 返回文本命中的主题码，供检索做主题过滤与加权。
func TopicCodes(text string) []string {
	codes := make([]string, 0, len(topicCodes))
	seen := make(map[string]struct{})
	for _, entry := range topicCodes {
		if !strings.Contains(text, entry.keyword) {
			continue
		}
		if _, ok := seen[entry.code]; ok {
			continue
		}
		seen[entry.code] = struct{}{}
		codes = append(codes, entry.code)
	}
	return codes
}

// documentID 依据主题与物种生成稳定标识，例如 pet_health.vomiting.cat。
func documentID(title string, species Species) string {
	return "pet_health." + topicCode(title) + "." + speciesCode(species)
}

// topicCode 取主题中首个命中关键词的主题码；未命中时回退为原文小写形式。
func topicCode(title string) string {
	for _, entry := range topicCodes {
		if strings.Contains(title, entry.keyword) {
			return entry.code
		}
	}
	code := strings.ToLower(strings.Join(strings.Fields(title), "-"))
	code = strings.ReplaceAll(code, "/", "-")
	if code == "" {
		return "unknown"
	}
	return code
}

func speciesCode(species Species) string {
	if species == "" {
		return string(SpeciesAll)
	}
	return string(species)
}

// NormalizeSpecies 把宠物档案里的物种写法归一化为知识库物种码，例如「猫」「cat」→ cat。
// 空值、无法识别的写法与「不限」一律返回空串，表示物种未确认；
// 调用方不得据此猜测物种，也不得用品种替代。
func NormalizeSpecies(value string) string {
	species, err := parseSpecies(value)
	if err != nil || species == SpeciesAll {
		return ""
	}
	return string(species)
}

func parseSpecies(value string) (Species, error) {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return "", fmt.Errorf("knowledge: entry has no 适用宠物")
	case strings.Contains(value, "猫狗"), strings.Contains(value, "通用"), strings.Contains(value, "不限"), strings.Contains(value, "all"):
		return SpeciesAll, nil
	case strings.Contains(value, "猫"), strings.Contains(strings.ToLower(value), "cat"), strings.Contains(strings.ToLower(value), "feline"):
		return SpeciesCat, nil
	case strings.Contains(value, "狗"), strings.Contains(value, "犬"), strings.Contains(strings.ToLower(value), "dog"), strings.Contains(strings.ToLower(value), "canine"):
		return SpeciesDog, nil
	default:
		return "", fmt.Errorf("knowledge: unknown 适用宠物 %q", value)
	}
}

func parseRiskLevel(value string) (RiskLevel, error) {
	switch strings.TrimSpace(value) {
	case "":
		return RiskYellow, nil
	case string(RiskGreen):
		return RiskGreen, nil
	case string(RiskYellow):
		return RiskYellow, nil
	case string(RiskRed):
		return RiskRed, nil
	default:
		return "", fmt.Errorf("knowledge: unknown 风险等级 %q", value)
	}
}

func parseStatus(value string) (Status, error) {
	switch strings.TrimSpace(value) {
	case "":
		return StatusPublished, nil
	case string(StatusDraft):
		return StatusDraft, nil
	case string(StatusPublished):
		return StatusPublished, nil
	case string(StatusWithdrawn):
		return StatusWithdrawn, nil
	default:
		return "", fmt.Errorf("knowledge: unknown 状态 %q", value)
	}
}

func parseLifeStages(value string) []LifeStage {
	values := splitList(value)
	if len(values) == 0 {
		return nil
	}
	result := make([]LifeStage, 0, len(values))
	for _, item := range values {
		stage := lifeStageFromText(item)
		if stage == "" {
			continue
		}
		result = append(result, stage)
	}
	return result
}

func lifeStageFromText(value string) LifeStage {
	switch {
	case strings.Contains(value, "幼"), strings.Contains(value, "young"):
		return LifeStageYoung
	case strings.Contains(value, "熟"), strings.Contains(value, "壮"), strings.Contains(value, "中年"), strings.Contains(value, "mature"):
		return LifeStageMature
	case strings.Contains(value, "老"), strings.Contains(value, "senior"):
		return LifeStageSenior
	case strings.Contains(value, "成"), strings.Contains(value, "青年"), strings.Contains(value, "青"), strings.Contains(value, "adult"):
		return LifeStageAdult
	case strings.Contains(value, "全"), strings.Contains(value, "不限"), strings.Contains(value, "all"):
		return LifeStageAll
	default:
		return ""
	}
}

func parseOptionalDate(value, field string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(sourceDateLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("knowledge: %s 必须是 YYYY-MM-DD，实际 %q", field, value)
	}
	return parsed, nil
}
