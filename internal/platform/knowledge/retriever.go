package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 本文件实现只读知识检索（设计文档第六、七节）：候选召回 -> 硬过滤 ->
// 可解释重排 -> 红旗条目强制保留。检索不调用模型，也不访问用户数据。

const (
	// recallLimit 是 BM25F 候选召回条数。
	recallLimit = 20
	// filteredLimit 是硬过滤后保留、进入可解释重排的条数。
	filteredLimit = 8
	// defaultInjectLimit 是最终注入模型的默认条数。
	defaultInjectLimit = 4
	// maxInjectLimit 是单次检索注入模型的最大条数。
	maxInjectLimit = 5
)

// RetrievalStatus 区分「知识库没有相关内容」与「检索本身完成」。
// 检索故障不在此表达：故障由端口返回 error，不得伪装成 no_match。
type RetrievalStatus string

const (
	RetrievalMatched RetrievalStatus = "matched"
	RetrievalNoMatch RetrievalStatus = "no_match"
)

// Query 是一次知识检索的输入。Text 是用户问题原文；Species 与 LifeStage 是由
// Runtime 依据已确认档案注入的过滤提示，模型只能提出搜索请求，不能指定文档身份。
type Query struct {
	Text      string
	Species   string
	LifeStage string
	Topics    []string
	Symptoms  []string
	RiskTerms []string
	Limit     int
	Now       time.Time
}

// Hit 是一条命中的知识条目及其可解释得分。
type Hit struct {
	Chunk   Chunk
	Score   float64
	Reasons []string
	RedFlag bool
}

// Result 是一次检索的结果。RedFlagRequired 表示问题含红旗词；
// RedFlagSatisfied 为 false 时，Runtime 必须改走急症规则，不得让模型自由补充。
type Result struct {
	Status             RetrievalStatus
	Version            string
	Candidates         int
	SpeciesUnconfirmed bool
	RedFlagRequired    bool
	RedFlagSatisfied   bool
	Hits               []Hit
}

// symptomTerms 是症状词表，用于症状匹配打分；与主题码表分开维护，
// 因为「症状」表达用户观察，「主题」表达知识条目的适用范围。
var symptomTerms = []string{
	"呕吐", "吐了", "干呕", "腹泻", "拉稀", "软便", "便秘",
	"咳嗽", "打喷嚏", "流鼻涕", "呼吸",
	"没精神", "精神不振", "食欲不振", "不吃", "拒食",
	"皮肤", "抓挠", "瘙痒", "掉毛", "红肿",
	"发烧", "发热", "疼痛", "跛行",
}

// SymptomTerms 返回文本命中的症状词，顺序固定，便于测试与审计。
func SymptomTerms(text string) []string {
	matched := make([]string, 0)
	for _, term := range symptomTerms {
		if strings.Contains(text, term) {
			matched = append(matched, term)
		}
	}
	return matched
}

// Search 执行一次只读知识检索。查询为空或目录为空时返回 no_match，不编造内容。
func (c *Catalog) Search(ctx context.Context, query Query) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	text := strings.TrimSpace(query.Text)
	if text == "" {
		return Result{}, fmt.Errorf("knowledge: query text is required")
	}
	now := query.Now
	if now.IsZero() {
		now = time.Now()
	}
	riskTerms := sortUnique(query.RiskTerms)
	if len(riskTerms) == 0 {
		riskTerms = RedFlagTerms(text)
	}
	topics := sortUnique(query.Topics)
	if len(topics) == 0 {
		topics = TopicCodes(text)
	}
	symptoms := sortUnique(query.Symptoms)
	if len(symptoms) == 0 {
		symptoms = SymptomTerms(text)
	}
	normalized := Query{Text: text, Species: strings.TrimSpace(query.Species), LifeStage: strings.TrimSpace(query.LifeStage), Topics: topics, Symptoms: symptoms, RiskTerms: riskTerms, Limit: query.Limit, Now: now}
	result := Result{Version: c.version, RedFlagRequired: len(riskTerms) > 0}
	candidates := c.index.search(expandQuery(normalized, speciesConfirmed(normalized.Species)), recallLimit)
	filtered := c.hardFilter(candidates, normalized, now)
	result.Candidates = len(filtered)
	if len(filtered) == 0 {
		result.Status = RetrievalNoMatch
		return result, nil
	}
	scored := c.rerank(filtered, normalized, now)
	if len(scored) > filteredLimit {
		scored = scored[:filteredLimit]
	}
	limit := normalized.Limit
	if limit <= 0 {
		limit = defaultInjectLimit
	}
	if limit > maxInjectLimit {
		limit = maxInjectLimit
	}
	hits := scored
	if len(hits) > limit {
		hits = hits[:limit]
	}
	if result.RedFlagRequired {
		hits, result.RedFlagSatisfied = retainRedFlagHit(scored, hits, limit)
	}
	result.Status = RetrievalMatched
	result.Hits = hits
	return result, nil
}

// expandQuery 把结构化提示并入检索文本，让词法检索能覆盖用户原话之外的
// 主题码与症状词；原始问题始终保留，不因结构化信息丢失。
//
// 物种已确认时去掉查询里的物种词：物种已经在硬过滤中生效，继续参与词法打分会让
// 「猫」「狗」这类高频词压过「喝水」「便秘」等真正的主题词，把结果带偏。
func expandQuery(query Query, speciesConfirmed bool) string {
	text := query.Text
	if speciesConfirmed {
		text = stripSpeciesWords(text)
	}
	parts := []string{text}
	parts = append(parts, query.Topics...)
	parts = append(parts, query.Symptoms...)
	parts = append(parts, query.RiskTerms...)
	return strings.Join(parts, " ")
}

// speciesWords 是查询里需要去掉的物种说法；只在物种已确认时使用。
var speciesWords = []string{"猫咪", "小猫", "母猫", "公猫", "猫猫", "猫", "狗狗", "小狗", "犬", "狗"}

// stripSpeciesWords 去掉查询文本里的物种词。索引侧保留物种词，只有查询侧去除。
func stripSpeciesWords(text string) string {
	for _, word := range speciesWords {
		text = strings.ReplaceAll(text, word, " ")
	}
	return strings.TrimSpace(text)
}

// speciesConfirmed 报告物种是否已由 Runtime 确认（排除未确认与不限物种）。
func speciesConfirmed(species string) bool {
	return species != "" && species != "unknown" && species != string(SpeciesAll)
}

// hardFilter 执行必须过滤的硬条件（设计文档第六节第三步）：
// 发布状态、版本有效期、物种匹配、生命周期匹配与主题匹配。
func (c *Catalog) hardFilter(matches []chunkMatch, query Query, now time.Time) []chunkMatch {
	confirmed := speciesConfirmed(query.Species)
	topics := make(map[string]struct{}, len(query.Topics))
	for _, topic := range query.Topics {
		topics[topic] = struct{}{}
	}
	result := make([]chunkMatch, 0, len(matches))
	for _, match := range matches {
		chunk := match.chunk
		if chunk.Status != StatusPublished {
			continue
		}
		if chunk.Expired(now) {
			continue
		}
		if confirmed && chunk.Species != SpeciesAll && string(chunk.Species) != query.Species {
			continue
		}
		if query.LifeStage != "" && query.LifeStage != "unknown" && !lifeStageMatches(chunk, query.LifeStage) {
			continue
		}
		if len(topics) > 0 && len(chunk.Topics) > 0 && !hasTopicMatch(chunk.Topics, topics) {
			continue
		}
		result = append(result, match)
	}
	return result
}

// lifeStageMatches 判断生命周期过滤是否命中；条目声明 all 或不限时视为命中。
func lifeStageMatches(chunk Chunk, want string) bool {
	if len(chunk.LifeStage) == 0 {
		return true
	}
	for _, stage := range chunk.LifeStage {
		if stage == LifeStageAll || string(stage) == want {
			return true
		}
	}
	return false
}

func hasTopicMatch(topics []string, want map[string]struct{}) bool {
	for _, topic := range topics {
		if _, ok := want[topic]; ok {
			return true
		}
	}
	return false
}

// rerank 按设计文档第六节的可解释权重重排：
// 0.40*bm25 + 0.20*topic + 0.15*species + 0.10*symptom + 0.10*source + 0.05*freshness。
func (c *Catalog) rerank(matches []chunkMatch, query Query, now time.Time) []Hit {
	maxScore := 0.0
	for _, match := range matches {
		if match.score > maxScore {
			maxScore = match.score
		}
	}
	topics := make(map[string]struct{}, len(query.Topics))
	for _, topic := range query.Topics {
		topics[topic] = struct{}{}
	}
	confirmed := speciesConfirmed(query.Species)
	hits := make([]Hit, 0, len(matches))
	for _, match := range matches {
		chunk := match.chunk
		lexical := 0.0
		if maxScore > 0 {
			lexical = match.score / maxScore
		}
		topicScore := 0.0
		if len(topics) > 0 && hasTopicMatch(chunk.Topics, topics) {
			topicScore = 1
		}
		speciesScore := 0.5
		switch {
		case !confirmed:
			speciesScore = 0.5
		case chunk.Species == SpeciesAll:
			speciesScore = 0.5
		case string(chunk.Species) == query.Species:
			speciesScore = 1
		}
		symptomScore := 0.0
		if len(query.Symptoms) > 0 && hasSymptomMatch(chunk, query.Symptoms) {
			symptomScore = 1
		}
		score := 0.40*lexical + 0.20*topicScore + 0.15*speciesScore + 0.10*symptomScore + 0.10*sourceQuality(chunk) + 0.05*freshness(chunk, now)
		hits = append(hits, Hit{
			Chunk:   chunk,
			Score:   score,
			Reasons: hitReasons(chunk, topicScore, speciesScore, symptomScore),
			RedFlag: chunk.HasEscalationConditions(),
		})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Chunk.ReferenceID < hits[j].Chunk.ReferenceID
	})
	return hits
}

func hasSymptomMatch(chunk Chunk, symptoms []string) bool {
	for _, symptom := range symptoms {
		if strings.Contains(chunk.Content, symptom) {
			return true
		}
		for _, declared := range chunk.Symptoms {
			if strings.Contains(declared, symptom) || strings.Contains(symptom, declared) {
				return true
			}
		}
	}
	return false
}

// sourceQuality 由来源名称与审核时间共同决定；未审核或未注明来源的条目得分更低。
func sourceQuality(chunk Chunk) float64 {
	hasSource := strings.TrimSpace(chunk.SourceName) != ""
	hasReview := !chunk.ReviewedAt.IsZero()
	switch {
	case hasSource && hasReview:
		return 1.0
	case hasSource || hasReview:
		return 0.6
	default:
		return 0.3
	}
}

// freshness 由审核日期决定：半年内为 1，两年以上降到 0.3，中间线性衰减。
func freshness(chunk Chunk, now time.Time) float64 {
	if chunk.ReviewedAt.IsZero() {
		return 0.5
	}
	ageDays := now.Sub(chunk.ReviewedAt).Hours() / 24
	switch {
	case ageDays <= 180:
		return 1.0
	case ageDays >= 730:
		return 0.3
	default:
		return 1.0 - 0.7*(ageDays-180)/(730-180)
	}
}

func hitReasons(chunk Chunk, topicScore, speciesScore, symptomScore float64) []string {
	reasons := make([]string, 0, 4)
	if topicScore > 0 {
		reasons = append(reasons, "topic_match")
	}
	if speciesScore == 1 {
		reasons = append(reasons, "species_match")
	}
	if symptomScore > 0 {
		reasons = append(reasons, "symptom_match")
	}
	if chunk.HasEscalationConditions() {
		reasons = append(reasons, "has_escalation_conditions")
	}
	return reasons
}

// retainRedFlagHit 保证含红旗条件的条目不被普通科普挤出最终注入集合：
// 已入选时移到最前；未入选时替换掉末位。找不到红旗条目时如实返回未满足。
func retainRedFlagHit(scored []Hit, hits []Hit, limit int) ([]Hit, bool) {
	if limit < 1 {
		limit = 1
	}
	redFlagIndex := -1
	for index := range scored {
		if scored[index].RedFlag {
			redFlagIndex = index
			break
		}
	}
	if redFlagIndex < 0 {
		return hits, false
	}
	// 重新构造结果，不原地改写 hits 与 scored 共享的底层数组。
	redFlag := scored[redFlagIndex]
	result := make([]Hit, 0, limit)
	result = append(result, redFlag)
	for _, hit := range hits {
		if len(result) >= limit {
			break
		}
		if hit.Chunk.ReferenceID == redFlag.Chunk.ReferenceID {
			continue
		}
		result = append(result, hit)
	}
	return result, true
}
