package file

import (
	"bytes"
	"context"
	"io"
	"testing"
)

type memoryStorage struct {
	objects map[string][]byte
}

func (m *memoryStorage) Upload(_ context.Context, key string, content io.Reader, _ int64, _ string) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	m.objects[key] = data
	return nil
}

func (m *memoryStorage) Delete(_ context.Context, key string) error {
	delete(m.objects, key)
	return nil
}

func (m *memoryStorage) URL(_ context.Context, key string) (string, error) {
	return "memory://" + key, nil
}

func TestUploadUsesMemoryStorage(t *testing.T) {
	store := &memoryStorage{objects: map[string][]byte{}}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	url, err := service.Upload(context.Background(), "pets/a.txt", bytes.NewBufferString("hello"), 5, "text/plain")
	if err != nil || url != "memory://pets/a.txt" || string(store.objects["pets/a.txt"]) != "hello" {
		t.Fatalf("url=%q objects=%v err=%v", url, store.objects, err)
	}
}
