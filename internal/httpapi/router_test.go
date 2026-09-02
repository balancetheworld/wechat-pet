package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHealthzUsesRequestIDAndCommonResponse(t *testing.T) {
	router := New()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-ID", "request-123")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Header().Get("X-Request-ID") != "request-123" {
		t.Fatalf("response request id = %q", recorder.Header().Get("X-Request-ID"))
	}
	if !strings.Contains(recorder.Body.String(), `"request_id":"request-123"`) {
		t.Fatalf("response body does not contain request id: %s", recorder.Body.String())
	}
}

func TestRecoveryReturnsCommonInternalError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	router := New(logger)
	router.GET("/panic", func(*gin.Context) { panic("secret panic") })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "secret panic") {
		t.Fatalf("panic value leaked in response: %s", recorder.Body.String())
	}
	if !strings.Contains(logs.String(), "http panic") || !strings.Contains(logs.String(), "secret panic") || !strings.Contains(logs.String(), "http request") {
		t.Fatalf("panic was not logged: %s", logs.String())
	}
}
