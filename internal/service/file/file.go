package file

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/balancetheworld/wechat-pet/internal/platform/storage"
)

var (
	ErrAssetNotOwnedByFamily = errors.New("asset is not owned by family")
	ErrAssetNotOwnedByUser   = errors.New("asset is not owned by user")
)

type Service struct {
	storage storage.Storage
	assets  Repository
}

type Asset struct {
	ID       string
	UserID   string
	FamilyID string
	Type     string
}

type Repository interface {
	Create(context.Context, Asset) error
	Get(context.Context, string) (Asset, error)
}

func NewService(store storage.Storage, repositories ...Repository) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("storage is required")
	}
	var repository Repository
	if len(repositories) > 0 {
		repository = repositories[0]
	}
	return &Service{storage: store, assets: repository}, nil
}

func (s *Service) UploadForOwner(ctx context.Context, key string, content io.Reader, size int64, contentType string, asset Asset) (string, error) {
	url, err := s.Upload(ctx, key, content, size, contentType)
	if err != nil {
		return "", err
	}
	if s.assets != nil {
		if err := s.assets.Create(ctx, asset); err != nil {
			_ = s.storage.Delete(ctx, key)
			return "", err
		}
	}
	return url, nil
}

func (s *Service) AuthorizeFamily(ctx context.Context, assetID, familyID string) error {
	if s.assets == nil || strings.TrimSpace(assetID) == "" || strings.TrimSpace(familyID) == "" {
		return nil
	}
	asset, err := s.assets.Get(ctx, assetID)
	if err != nil || asset.FamilyID != familyID {
		return ErrAssetNotOwnedByFamily
	}
	return nil
}

func (s *Service) AuthorizeUser(ctx context.Context, assetID, userID string) error {
	if s.assets == nil || strings.TrimSpace(assetID) == "" || strings.TrimSpace(userID) == "" {
		return nil
	}
	asset, err := s.assets.Get(ctx, assetID)
	if err != nil || asset.UserID != userID {
		return ErrAssetNotOwnedByUser
	}
	return nil
}

func (s *Service) Get(ctx context.Context, assetID string) (Asset, error) {
	if s.assets == nil {
		return Asset{}, errors.New("asset repository unavailable")
	}
	return s.assets.Get(ctx, assetID)
}

func (s *Service) Upload(ctx context.Context, key string, content io.Reader, size int64, contentType string) (string, error) {
	if err := s.storage.Upload(ctx, key, content, size, contentType); err != nil {
		return "", err
	}
	return s.storage.URL(ctx, key)
}

func (s *Service) Delete(ctx context.Context, key string) error {
	return s.storage.Delete(ctx, key)
}

type SQLRepository struct {
	db     *sql.DB
	driver string
}

func NewRepository(db *sql.DB, driver string) (*SQLRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	return &SQLRepository{db: db, driver: strings.ToLower(driver)}, nil
}

func (r *SQLRepository) Create(ctx context.Context, asset Asset) error {
	_, err := r.db.ExecContext(ctx, r.query("INSERT INTO assets (id, user_id, family_id, type, created_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)"), asset.ID, asset.UserID, nullable(asset.FamilyID), asset.Type)
	return err
}

func (r *SQLRepository) Get(ctx context.Context, id string) (Asset, error) {
	var asset Asset
	var family sql.NullString
	err := r.db.QueryRowContext(ctx, r.query("SELECT id, user_id, family_id, type FROM assets WHERE id = ?"), id).Scan(&asset.ID, &asset.UserID, &family, &asset.Type)
	if family.Valid {
		asset.FamilyID = family.String
	}
	return asset, err
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func (r *SQLRepository) query(value string) string {
	if r.driver != "postgres" && r.driver != "postgresql" && r.driver != "pgx" {
		return value
	}
	var result strings.Builder
	index := 0
	for _, char := range value {
		if char != '?' {
			result.WriteRune(char)
			continue
		}
		index++
		result.WriteByte('$')
		result.WriteString(fmt.Sprint(index))
	}
	return result.String()
}
