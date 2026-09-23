package storage

import (
	"context"
	"io"
)

type Storage interface {
	Upload(ctx context.Context, key string, content io.Reader, size int64, contentType string) error
	Read(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	URL(ctx context.Context, key string) (string, error)
}
