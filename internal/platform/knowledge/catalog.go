package knowledge

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// 本文件把已审核的知识条目发布为固定版本的知识目录（设计文档第十一节
// 「知识版本和历史可重放」）：Run 开始时固定目录版本，后续更新只影响新 Run。

// corpusFS 内嵌仓库内已审核的知识条目。发布与撤回随版本化提交完成，
// 一期不开放运营后台编辑，也不提供运行时写接口。
//
//go:embed corpus
var corpusFS embed.FS

// corpusManifest 是知识目录的版本清单。
type corpusManifest struct {
	Version    string `yaml:"version"`
	ReviewedAt string `yaml:"reviewed_at"`
}

// Catalog 是固定版本的知识目录：条目、检索索引与内容摘要。
type Catalog struct {
	version    string
	digest     string
	reviewedAt time.Time
	chunks     []Chunk
	index      *fieldIndex
}

// NewCatalog 发布一份知识目录：补齐引用标识、校验条目并建立 BM25F 索引。
// version 为空时返回错误，避免出现无法重放的匿名知识版本。
func NewCatalog(entries []Chunk, version string, tokenizer func(string) []string) (*Catalog, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil, fmt.Errorf("knowledge: catalog version is required")
	}
	tokens := Tokenizer(DefaultTokenizer())
	if tokenizer != nil {
		tokens = Tokenizer(tokenizer)
	}
	chunks, err := assignReferenceIDs(entries, version)
	if err != nil {
		return nil, err
	}
	return &Catalog{
		version: version,
		digest:  catalogDigest(chunks),
		chunks:  chunks,
		index:   newFieldIndex(chunks, tokens),
	}, nil
}

// EmbeddedCatalog 加载仓库内嵌的知识目录。manifest.yaml 提供人工审核的版本号，
// 条目文件按路径排序解析，保证引用标识可重放。
func EmbeddedCatalog(tokenizer func(string) []string) (*Catalog, error) {
	raw, err := fs.ReadFile(corpusFS, "corpus/manifest.yaml")
	if err != nil {
		return nil, fmt.Errorf("knowledge: read manifest: %w", err)
	}
	var manifest corpusManifest
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("knowledge: parse manifest: %w", err)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return nil, fmt.Errorf("knowledge: manifest has empty version")
	}
	paths, err := fs.Glob(corpusFS, "corpus/*.md")
	if err != nil {
		return nil, fmt.Errorf("knowledge: list corpus: %w", err)
	}
	sort.Strings(paths)
	entries := make([]Chunk, 0, len(paths))
	for _, path := range paths {
		text, readErr := fs.ReadFile(corpusFS, path)
		if readErr != nil {
			return nil, fmt.Errorf("knowledge: read %s: %w", path, readErr)
		}
		chunk, parseErr := ParseEntry(string(text))
		if parseErr != nil {
			return nil, fmt.Errorf("knowledge: %s: %w", path, parseErr)
		}
		chunk.ChunkKey = chunkKeyFromFile(path, chunk)
		entries = append(entries, chunk)
	}
	catalog, err := NewCatalog(entries, manifest.Version, tokenizer)
	if err != nil {
		return nil, err
	}
	if reviewedAt, parseErr := parseOptionalDate(manifest.ReviewedAt, "reviewed_at"); parseErr == nil {
		catalog.reviewedAt = reviewedAt
	}
	return catalog, nil
}

// Version 返回知识目录版本，供 Run 固定与引用校验使用。
func (c *Catalog) Version() string { return c.version }

// Digest 返回目录内容摘要，任一条目正文或版本变化都会改变摘要。
func (c *Catalog) Digest() string { return c.digest }

// ReviewedAt 返回目录清单记录的审核日期，供审计与评测核对。
func (c *Catalog) ReviewedAt() time.Time { return c.reviewedAt }

// Chunks 返回目录内的条目副本，供评测与审计读取。
func (c *Catalog) Chunks() []Chunk {
	result := make([]Chunk, len(c.chunks))
	copy(result, c.chunks)
	return result
}

// assignReferenceIDs 为条目补齐 document_id / chunk_id / reference_id 与版本、状态默认值。
// 同一 document_id 下的条目按输入顺序编号 001、002，便于把红旗条目单独拆出。
func assignReferenceIDs(entries []Chunk, version string) ([]Chunk, error) {
	counts := make(map[string]int)
	seen := make(map[string]struct{}, len(entries))
	chunks := make([]Chunk, 0, len(entries))
	for _, entry := range entries {
		entry.Version = strings.TrimSpace(entry.Version)
		if entry.Version == "" {
			entry.Version = version
		}
		if entry.Status == "" {
			entry.Status = StatusPublished
		}
		if entry.Species == "" {
			return nil, fmt.Errorf("knowledge: entry %q has empty 适用宠物", entry.Title)
		}
		if strings.TrimSpace(entry.DocumentID) == "" {
			entry.DocumentID = documentID(entry.Title, entry.Species)
		}
		// 已显式声明 chunk_id 的条目（评测用例）保留原标识；内嵌条目用文件级稳定后缀
		// 生成标识，新增条目不会改写既有 reference_id；其余按文档内顺序编号。
		if strings.TrimSpace(entry.ChunkID) == "" {
			suffix := strings.Trim(strings.ToLower(strings.TrimSpace(entry.ChunkKey)), "-.")
			if suffix == "" {
				counts[entry.DocumentID]++
				suffix = fmt.Sprintf("%03d", counts[entry.DocumentID])
			}
			entry.ChunkID = entry.DocumentID + "." + suffix
		}
		if strings.TrimSpace(entry.ReferenceID) == "" {
			entry.ReferenceID = entry.ChunkID
		}
		if _, ok := seen[entry.ReferenceID]; ok {
			return nil, fmt.Errorf("knowledge: duplicate reference_id %q", entry.ReferenceID)
		}
		seen[entry.ReferenceID] = struct{}{}
		if err := entry.Validate(); err != nil {
			return nil, err
		}
		chunks = append(chunks, entry)
	}
	return chunks, nil
}

// catalogDigest 计算目录内容摘要：条目按 reference_id 排序后拼接
// reference_id|version|来源|内容哈希，取 SHA-256。
func catalogDigest(chunks []Chunk) string {
	parts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := sha256.Sum256([]byte(chunk.Content))
		parts = append(parts, chunk.ReferenceID+"|"+chunk.Version+"|"+string(chunk.Status)+"|"+hex.EncodeToString(content[:]))
	}
	sort.Strings(parts)
	digest := sha256.New()
	for _, part := range parts {
		digest.Write([]byte(part))
		digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// chunkKeyFromFile 从条目文件名提取稳定后缀：去掉文档标识里已有的主题码与物种码，
// 剩下的部分作为 chunk 后缀。例如 cat-vomiting-observation.md（文档
// pet_health.vomiting.cat）得到 observation，对应 pet_health.vomiting.cat.observation。
// 这样新增条目不会改写已有条目的 reference_id。
func chunkKeyFromFile(path string, chunk Chunk) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	docID := strings.TrimSpace(chunk.DocumentID)
	if docID == "" {
		docID = documentID(chunk.Title, chunk.Species)
	}
	skip := make(map[string]struct{})
	for _, part := range strings.Split(strings.ToLower(docID), ".") {
		skip[part] = struct{}{}
		for _, token := range strings.Split(part, "_") {
			skip[token] = struct{}{}
		}
	}
	remaining := make([]string, 0, 4)
	for _, token := range strings.Split(strings.ToLower(base), "-") {
		if token == "" {
			continue
		}
		if _, ok := skip[token]; ok {
			continue
		}
		remaining = append(remaining, token)
	}
	return strings.Join(remaining, "-")
}
