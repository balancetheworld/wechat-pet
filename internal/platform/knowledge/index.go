package knowledge

import (
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/go-ego/gse"
)

// 本文件实现知识检索专用的 BM25F 索引（设计文档第六节「检索链路」）。
// 索引独立于工具目录索引：两者字段、权限与结果类型不同，不能共用一份索引。

// Tokenizer 把一段文本切分为检索 token；抽象为函数类型，便于测试注入确定性分词器。
type Tokenizer func(text string) []string

var (
	defaultSegmenterOnce sync.Once
	defaultSegmenter     gse.Segmenter
	defaultSegmenterErr  error
)

// DefaultTokenizer 返回基于 gse 内置词典的默认分词器（搜索模式，召回优先）。
// 生产装配可注入 ask 包的同一分词器，避免重复加载词典。
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

// searchField 是知识条目的 BM25F 检索字段。
type searchField int

const (
	fieldTitle searchField = iota
	fieldKeywords
	fieldSymptoms
	fieldTopics
	fieldBody
	searchFieldCount
)

// fieldWeights 是各检索字段权重：title/keywords/symptoms 最高，
// topics 次之，body 为普通权重（设计文档第六节）。
var fieldWeights = [searchFieldCount]float64{
	fieldTitle:    5.0,
	fieldKeywords: 4.0,
	fieldSymptoms: 4.0,
	fieldTopics:   3.0,
	fieldBody:     1.0,
}

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// chunkMatch 是一个知识条目与 BM25F 得分。
type chunkMatch struct {
	chunk Chunk
	score float64
}

// indexedChunk 是建索引后的单个知识条目。
type indexedChunk struct {
	chunk    Chunk
	tf       [searchFieldCount]map[string]int
	fieldLen [searchFieldCount]int
}

// fieldIndex 是知识库的词法检索索引，一期不依赖 Embedding 与向量库。
type fieldIndex struct {
	tokenizer   Tokenizer
	docs        []indexedChunk
	docFreq     map[string]int
	avgFieldLen [searchFieldCount]float64
}

// newFieldIndex 基于知识条目建索引；tokenizer 为 nil 时使用 DefaultTokenizer。
func newFieldIndex(chunks []Chunk, tokenizer Tokenizer) *fieldIndex {
	if tokenizer == nil {
		tokenizer = DefaultTokenizer()
	}
	index := &fieldIndex{tokenizer: tokenizer, docFreq: make(map[string]int)}
	index.docs = make([]indexedChunk, 0, len(chunks))
	var totalFieldLen [searchFieldCount]int
	for _, chunk := range chunks {
		doc := indexedChunk{chunk: chunk}
		texts := chunkFieldTexts(chunk)
		seen := make(map[string]struct{})
		for field := searchField(0); field < searchFieldCount; field++ {
			doc.tf[field] = make(map[string]int)
			for _, token := range tokenizer(texts[field]) {
				if token == "" {
					continue
				}
				doc.tf[field][token]++
				doc.fieldLen[field]++
				totalFieldLen[field]++
				seen[token] = struct{}{}
			}
		}
		for token := range seen {
			index.docFreq[token]++
		}
		index.docs = append(index.docs, doc)
	}
	if count := len(index.docs); count > 0 {
		for field := searchField(0); field < searchFieldCount; field++ {
			index.avgFieldLen[field] = float64(totalFieldLen[field]) / float64(count)
		}
	}
	return index
}

// search 对查询分词后按 BM25F 打分，返回降序候选；无命中时返回空切片，不伪造候选。
func (index *fieldIndex) search(query string, limit int) []chunkMatch {
	tokens := uniqueTokens(index.tokenizer(query))
	if len(tokens) == 0 || len(index.docs) == 0 {
		return nil
	}
	count := float64(len(index.docs))
	idfs := make(map[string]float64, len(tokens))
	for _, token := range tokens {
		docFreq := float64(index.docFreq[token])
		idfs[token] = math.Log(1 + (count-docFreq+0.5)/(docFreq+0.5))
	}
	matches := make([]chunkMatch, 0, len(index.docs))
	for _, doc := range index.docs {
		var score float64
		for _, token := range tokens {
			idf := idfs[token]
			for field := searchField(0); field < searchFieldCount; field++ {
				tf := float64(doc.tf[field][token])
				if tf == 0 {
					continue
				}
				norm := 1.0
				if avg := index.avgFieldLen[field]; avg > 0 {
					norm = 1 - bm25B + bm25B*float64(doc.fieldLen[field])/avg
				}
				score += idf * fieldWeights[field] * tf * (bm25K1 + 1) / (tf + bm25K1*norm)
			}
		}
		if score > 0 {
			matches = append(matches, chunkMatch{chunk: doc.chunk, score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].chunk.ReferenceID < matches[j].chunk.ReferenceID
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// chunkFieldTexts 提取知识条目各检索字段的原始文本。
func chunkFieldTexts(chunk Chunk) [searchFieldCount]string {
	var texts [searchFieldCount]string
	texts[fieldTitle] = chunk.Title
	texts[fieldKeywords] = strings.Join(chunk.Keywords, " ")
	texts[fieldSymptoms] = strings.Join(chunk.Symptoms, " ")
	texts[fieldTopics] = strings.Join(chunk.Topics, " ")
	texts[fieldBody] = chunk.Content
	return texts
}

// uniqueTokens 去除空 token 与重复 token，保留首次出现顺序。
func uniqueTokens(tokens []string) []string {
	seen := make(map[string]struct{}, len(tokens))
	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		result = append(result, token)
	}
	return result
}
