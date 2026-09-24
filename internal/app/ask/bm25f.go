package ask

import (
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/go-ego/gse"
)

// Tokenizer：把一段文本切分为检索 token。抽象为函数类型，便于测试注入
// 简单分词器，生产使用基于 gse 的 DefaultTokenizer。
type Tokenizer func(text string) []string

var (
	defaultSegmenterOnce sync.Once
	defaultSegmenter     gse.Segmenter
	defaultSegmenterErr  error
)

// DefaultTokenizer 返回基于 gse 内置词典的默认分词器（搜索模式，召回优先）。
// 词典只在首次调用时加载一次；加载失败时降级为按空白切分。
func DefaultTokenizer() Tokenizer {
	defaultSegmenterOnce.Do(func() {
		defaultSegmenter, defaultSegmenterErr = gse.New()
	})
	return func(text string) []string {
		text = strings.ToLower(text)
		if defaultSegmenterErr != nil {
			return strings.Fields(text)
		}
		return defaultSegmenter.CutSearch(text, true)
	}
}

// searchField：工具的 BM25F 检索字段。
type searchField int

const (
	fieldActionType searchField = iota
	fieldResourceType
	fieldOperationID
	fieldParameterNames
	fieldAliases
	fieldUseCases
	searchFieldCount
)

// fieldWeights：各检索字段的 BM25F 权重。action_type 权重最高，其次
// resource_type、operation_id、参数名、中英文别名、使用场景。
var fieldWeights = [searchFieldCount]float64{
	fieldActionType:     5.0,
	fieldResourceType:   4.0,
	fieldOperationID:    3.0,
	fieldParameterNames: 2.0,
	fieldAliases:        1.5,
	fieldUseCases:       1.0,
}

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// ToolMatch：工具候选与其 BM25F 得分。
type ToolMatch struct {
	Tool  Tool
	Score float64
}

// indexedTool：建索引后的单个工具。
type indexedTool struct {
	tool     Tool
	tf       [searchFieldCount]map[string]int
	fieldLen [searchFieldCount]int
}

// BM25FIndex：工具目录的 BM25F 词法检索索引。一期不依赖 Embedding。
type BM25FIndex struct {
	tokenizer   Tokenizer
	docs        []indexedTool
	docFreq     map[string]int
	avgFieldLen [searchFieldCount]float64
}

// NewBM25FIndex 基于工具列表建索引。tokenizer 为 nil 时使用 DefaultTokenizer。
func NewBM25FIndex(tools []Tool, tokenizer Tokenizer) *BM25FIndex {
	if tokenizer == nil {
		tokenizer = DefaultTokenizer()
	}
	idx := &BM25FIndex{
		tokenizer: tokenizer,
		docFreq:   make(map[string]int),
	}
	idx.docs = make([]indexedTool, 0, len(tools))
	var totalFieldLen [searchFieldCount]int
	for _, tool := range tools {
		it := indexedTool{tool: tool}
		texts := toolFieldTexts(tool)
		seen := make(map[string]struct{})
		for f := searchField(0); f < searchFieldCount; f++ {
			it.tf[f] = make(map[string]int)
			for _, tok := range tokenizer(texts[f]) {
				if tok == "" {
					continue
				}
				it.tf[f][tok]++
				it.fieldLen[f]++
				totalFieldLen[f]++
				seen[tok] = struct{}{}
			}
		}
		for tok := range seen {
			idx.docFreq[tok]++
		}
		idx.docs = append(idx.docs, it)
	}
	if n := len(idx.docs); n > 0 {
		f := float64(n)
		for field := searchField(0); field < searchFieldCount; field++ {
			idx.avgFieldLen[field] = float64(totalFieldLen[field]) / f
		}
	}
	return idx
}

// Search 对查询分词后按 BM25F 打分，返回按得分降序的有序候选。
// limit <= 0 表示不截断。无任何命中时返回空切片（不伪造候选）。
func (idx *BM25FIndex) Search(query string, limit int) []ToolMatch {
	tokens := uniqueTokens(idx.tokenizer(query))
	if len(tokens) == 0 || len(idx.docs) == 0 {
		return nil
	}
	n := float64(len(idx.docs))
	idfs := make(map[string]float64, len(tokens))
	for _, tok := range tokens {
		df := float64(idx.docFreq[tok])
		idfs[tok] = math.Log(1 + (n-df+0.5)/(df+0.5))
	}
	matches := make([]ToolMatch, 0, len(idx.docs))
	for _, d := range idx.docs {
		var score float64
		for _, tok := range tokens {
			idf := idfs[tok]
			for f := searchField(0); f < searchFieldCount; f++ {
				tf := float64(d.tf[f][tok])
				if tf == 0 {
					continue
				}
				norm := 1.0
				if avg := idx.avgFieldLen[f]; avg > 0 {
					norm = 1 - bm25B + bm25B*float64(d.fieldLen[f])/avg
				}
				score += idf * fieldWeights[f] * tf * (bm25K1 + 1) / (tf + bm25K1*norm)
			}
		}
		if score > 0 {
			matches = append(matches, ToolMatch{Tool: d.tool, Score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// toolFieldTexts 提取工具各检索字段的原始文本。
func toolFieldTexts(t Tool) [searchFieldCount]string {
	var texts [searchFieldCount]string
	texts[fieldActionType] = splitSnake(string(t.ActionType))
	texts[fieldResourceType] = splitSnake(string(t.ResourceType))
	texts[fieldOperationID] = splitSnake(t.OperationID)
	texts[fieldParameterNames] = splitSnake(strings.Join(t.ParameterNames(), " "))
	aliases := make([]string, 0, len(t.AliasesZH)+len(t.AliasesEN))
	aliases = append(aliases, t.AliasesZH...)
	aliases = append(aliases, t.AliasesEN...)
	texts[fieldAliases] = splitSnake(strings.Join(aliases, " "))
	texts[fieldUseCases] = splitSnake(strings.Join(t.UseCases, " "))
	return texts
}

// splitSnake 把 snake_case 标识符的下划线替换为空格，使分词器能切出英文单词。
func splitSnake(s string) string {
	return strings.ReplaceAll(s, "_", " ")
}

// uniqueTokens 去除空 token 与重复 token，保留首次出现顺序。
func uniqueTokens(tokens []string) []string {
	seen := make(map[string]struct{}, len(tokens))
	result := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		if tok == "" {
			continue
		}
		if _, ok := seen[tok]; ok {
			continue
		}
		seen[tok] = struct{}{}
		result = append(result, tok)
	}
	return result
}
