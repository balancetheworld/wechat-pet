package asset

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	fileservice "github.com/balancetheworld/wechat-pet/internal/service/file"
	"github.com/gin-gonic/gin"
)

type testStorage struct{ uploaded bool }

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
	RegisterRoutes(router.Group("/api/v1"), signer, service)
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
