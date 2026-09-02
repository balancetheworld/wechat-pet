package user

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/balancetheworld/wechat-pet/internal/model"
)

var ErrNotFound = sql.ErrNoRows

type Repository interface {
	GetByID(ctx context.Context, userID string) (model.User, error)
	GetByOpenID(ctx context.Context, openID string) (model.User, error)
	UpsertByOpenID(ctx context.Context, openID string, unionID *string, loginAt time.Time) (model.User, error)
	UpdateProfile(ctx context.Context, userID string, nickname string, avatarAssetID string) (model.User, error)
}

type SQLRepository struct {
	db     *sql.DB
	driver string
}

func NewRepository(db *sql.DB, driver string) (*SQLRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &SQLRepository{db: db, driver: strings.ToLower(driver)}, nil
}

func (r *SQLRepository) GetByID(ctx context.Context, userID string) (model.User, error) {
	return r.get(ctx, "id", userID)
}

func (r *SQLRepository) GetByOpenID(ctx context.Context, openID string) (model.User, error) {
	return r.get(ctx, "openid", openID)
}

func (r *SQLRepository) get(ctx context.Context, field string, value string) (model.User, error) {
	if strings.TrimSpace(value) == "" {
		return model.User{}, fmt.Errorf("%s is required", field)
	}
	query := "SELECT id, openid, unionid, nickname, avatar_asset_id, last_login_at, created_at, updated_at FROM users WHERE " + field + " = ?"
	if r.driver == "postgres" || r.driver == "postgresql" || r.driver == "pgx" {
		query = strings.Replace(query, "?", "$1", 1)
	}
	var result model.User
	var unionID, nickname, avatar sql.NullString
	err := r.db.QueryRowContext(ctx, query, value).Scan(&result.ID, &result.OpenID, &unionID, &nickname, &avatar, &result.LastLoginAt, &result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return model.User{}, err
	}
	result.UnionID = nullableString(unionID)
	result.Nickname = nullableString(nickname)
	result.AvatarAssetID = nullableString(avatar)
	return result, nil
}

func (r *SQLRepository) UpsertByOpenID(ctx context.Context, openID string, unionID *string, loginAt time.Time) (model.User, error) {
	if strings.TrimSpace(openID) == "" {
		return model.User{}, errors.New("openid is required")
	}
	if loginAt.IsZero() {
		loginAt = time.Now().UTC()
	}
	id, err := newID()
	if err != nil {
		return model.User{}, err
	}
	query := "INSERT INTO users (id, openid, unionid, last_login_at, created_at, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) ON CONFLICT (openid) DO UPDATE SET unionid = COALESCE(excluded.unionid, users.unionid), last_login_at = excluded.last_login_at, updated_at = excluded.updated_at"
	if r.driver == "postgres" || r.driver == "postgresql" || r.driver == "pgx" {
		query = "INSERT INTO users (id, openid, unionid, last_login_at, created_at, updated_at) VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) ON CONFLICT (openid) DO UPDATE SET unionid = COALESCE(EXCLUDED.unionid, users.unionid), last_login_at = EXCLUDED.last_login_at, updated_at = EXCLUDED.updated_at"
	}
	if _, err := r.db.ExecContext(ctx, query, id, openID, nullableValue(unionID), loginAt); err != nil {
		return model.User{}, err
	}
	return r.GetByOpenID(ctx, openID)
}

func (r *SQLRepository) UpdateProfile(ctx context.Context, userID string, nickname string, avatarAssetID string) (model.User, error) {
	if strings.TrimSpace(userID) == "" {
		return model.User{}, errors.New("user id is required")
	}
	query := "UPDATE users SET nickname = ?, avatar_asset_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?"
	args := []any{nullableValue(stringPointer(strings.TrimSpace(nickname))), nullableValue(stringPointer(strings.TrimSpace(avatarAssetID))), userID}
	if r.driver == "postgres" || r.driver == "postgresql" || r.driver == "pgx" {
		query = "UPDATE users SET nickname = $1, avatar_asset_id = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3"
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return model.User{}, err
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		if err != nil {
			return model.User{}, err
		}
		return model.User{}, sql.ErrNoRows
	}
	return r.GetByID(ctx, userID)
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullableValue(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func stringPointer(value string) *string { return &value }

func newID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate user id: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
