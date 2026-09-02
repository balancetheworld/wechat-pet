package file

import (
	"context"
	"fmt"
	"io"

	"github.com/balancetheworld/wechat-pet/internal/platform/storage"
)

type Service struct {
	storage storage.Storage
}

func NewService(store storage.Storage) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("storage is required")
	}
	return &Service{storage: store}, nil
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
