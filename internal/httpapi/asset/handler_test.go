package asset

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	fileservice "github.com/balancetheworld/wechat-pet/internal/service/file"
	"github.com/gin-gonic/gin"
)

type testStorage struct{ uploaded bool }

type testRepository struct{ assets map[string]fileservice.Asset }

func (r testRepository) Create(context.Context, fileservice.Asset) error { return nil }
func (r testRepository) Get(_ context.Context, id string) (fileservice.Asset, error) {
	asset, ok := r.assets[id]
	if !ok {
		return fileservice.Asset{}, os.ErrNotExist
	}
	return asset, nil
}

func (s *testStorage) Upload(context.Context, string, io.Reader, int64, string) error {
	s.uploaded = true
	return nil
}
func (s *testStorage) Delete(context.Context, string) error { return nil }
func (s *testStorage) URL(context.Context, string) (string, error) {
	return "http://example.test/avatar.jpg", nil
}

func TestUpload(t *testing.T) {
	storage := &testStorage{}
	service, err := fileservice.NewService(storage)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jwtpkg.NewSigner("secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), signer, service, "")
	token, err := signer.Sign("user-1")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("type", "avatar"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "avatar.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("png")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/assets/upload", &body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !storage.uploaded {
		t.Fatalf("status=%d body=%s uploaded=%v", recorder.Code, recorder.Body.String(), storage.uploaded)
	}
}

func TestDownloadRequiresAssetOwner(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "avatar.png"), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	storage := &testStorage{}
	service, err := fileservice.NewService(storage, testRepository{assets: map[string]fileservice.Asset{
		"avatar.png": {ID: "avatar.png", UserID: "user-a"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jwtpkg.NewSigner("secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, err := signer.Sign("user-b")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), signer, service, dir)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/uploads/avatar.png?access_token="+token, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
