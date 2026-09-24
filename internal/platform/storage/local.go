package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type LocalStorage struct {
	RootDir   string
	PublicURL string
}

func NewLocalStorage(rootDir, publicURL string) (*LocalStorage, error) {
	if strings.TrimSpace(rootDir) == "" {
		return nil, fmt.Errorf("local storage root directory is required")
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("create local storage directory: %w", err)
	}
	return &LocalStorage{RootDir: rootDir, PublicURL: strings.TrimRight(publicURL, "/")}, nil
}

func (s *LocalStorage) Upload(_ context.Context, key string, content io.Reader, _ int64, _ string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create upload directory: %w", err)
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create local object: %w", err)
	}
	defer file.Close()
	if _, err := io.Copy(file, content); err != nil {
		return fmt.Errorf("write local object: %w", err)
	}
	return nil
}

func (s *LocalStorage) Read(_ context.Context, key string) ([]byte, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (s *LocalStorage) Delete(_ context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete local object: %w", err)
	}
	return nil
}

func (s *LocalStorage) URL(_ context.Context, key string) (string, error) {
	if _, err := s.path(key); err != nil {
		return "", err
	}
	if s.PublicURL == "" {
		return "file://" + filepath.Join(s.RootDir, filepath.FromSlash(key)), nil
	}
	return s.PublicURL + "/" + url.PathEscape(strings.TrimLeft(key, "/")), nil
}

func (s *LocalStorage) path(key string) (string, error) {
	cleanKey := filepath.Clean(filepath.FromSlash(key))
	if cleanKey == "." || cleanKey == ".." || strings.HasPrefix(cleanKey, ".."+string(filepath.Separator)) || filepath.IsAbs(cleanKey) {
		return "", fmt.Errorf("invalid storage key")
	}
	return filepath.Join(s.RootDir, cleanKey), nil
}
