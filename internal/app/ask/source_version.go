package ask

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrSourceVersionNotFound：来源集合版本不存在。
var ErrSourceVersionNotFound = errors.New("ask source version not found")

// SourceVersionRepository：来源集合版本的最小读写。
// 版本值由业务侧（工具复用 P4、记忆回源 P7）生成并传入，
// 此处仅提供持久化约束，供后续检索失效判断使用，避免后补过滤。
type SourceVersionRepository interface {
	UpsertSourceVersion(context.Context, string, string, string) error
	GetSourceVersion(context.Context, string, string) (SourceVersion, error)
}

// UpsertSourceVersion 记录或更新来源集合版本。
// source_type + source_id 唯一，已存在时覆盖 version 并刷新 versioned_at。
func (r *SQLRepository) UpsertSourceVersion(ctx context.Context, sourceType, sourceID, version string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.query(`INSERT INTO ask_source_versions (source_type, source_id, version, versioned_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (source_type, source_id) DO UPDATE SET version = ?, versioned_at = ?`), sourceType, sourceID, version, now, version, now)
	return err
}

// GetSourceVersion 读取来源集合版本；不存在时返回 ErrSourceVersionNotFound。
func (r *SQLRepository) GetSourceVersion(ctx context.Context, sourceType, sourceID string) (SourceVersion, error) {
	var value SourceVersion
	err := r.db.QueryRowContext(ctx, r.query("SELECT source_type, source_id, version, versioned_at FROM ask_source_versions WHERE source_type = ? AND source_id = ?"), sourceType, sourceID).Scan(&value.SourceType, &value.SourceID, &value.Version, &value.VersionedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SourceVersion{}, ErrSourceVersionNotFound
	}
	if err != nil {
		return SourceVersion{}, err
	}
	return value, nil
}
