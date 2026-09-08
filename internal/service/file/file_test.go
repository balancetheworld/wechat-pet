package file

import (
	"bytes"
	"context"
	"errors"
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

type memoryRepository struct {
	assets    map[string]Asset
	createErr error
}

func (r *memoryRepository) Create(_ context.Context, asset Asset) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.assets[asset.ID] = asset
	return nil
}

func (r *memoryRepository) Get(_ context.Context, id string) (Asset, error) {
	asset, ok := r.assets[id]
	if !ok {
		return Asset{}, errors.New("asset not found")
	}
	return asset, nil
}

func TestAuthorizeAssetOwner(t *testing.T) {
	repository := &memoryRepository{assets: map[string]Asset{
		"family-a": {ID: "family-a", UserID: "user-a", FamilyID: "family-a"},
		"user-a":   {ID: "user-a", UserID: "user-a"},
	}}
	service, err := NewService(&memoryStorage{objects: map[string][]byte{}}, repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AuthorizeFamily(context.Background(), "family-a", "family-b"); !errors.Is(err, ErrAssetNotOwnedByFamily) {
		t.Fatalf("error=%v", err)
	}
	if err := service.AuthorizeUser(context.Background(), "user-a", "user-b"); !errors.Is(err, ErrAssetNotOwnedByUser) {
		t.Fatalf("error=%v", err)
	}
}

func TestUploadForOwnerDeletesObjectWhenMetadataCreationFails(t *testing.T) {
	store := &memoryStorage{objects: map[string][]byte{}}
	service, err := NewService(store, &memoryRepository{assets: map[string]Asset{}, createErr: errors.New("create asset")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.UploadForOwner(context.Background(), "assets/a.png", bytes.NewBufferString("image"), 5, "image/png", Asset{ID: "assets/a.png"})
	if err == nil {
		t.Fatal("expected metadata creation error")
	}
	if _, ok := store.objects["assets/a.png"]; ok {
		t.Fatal("uploaded object was not deleted")
	}
}
