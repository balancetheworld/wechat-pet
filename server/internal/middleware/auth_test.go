package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	familyapp "github.com/balancetheworld/wechat-pet/server/internal/app/family"
	appErrors "github.com/balancetheworld/wechat-pet/server/internal/pkg/errors"
	jwtpkg "github.com/balancetheworld/wechat-pet/server/internal/pkg/jwt"
	"github.com/balancetheworld/wechat-pet/server/internal/pkg/response"
	"github.com/gin-gonic/gin"
	jwt "github.com/golang-jwt/jwt"
)

type fakeFamilyRepository struct {
	summary *familyapp.Summary
}

func (f fakeFamilyRepository) GetActiveFamilySummary(context.Context, string) (*familyapp.Summary, error) {
	return f.summary, nil
}

func TestRequireAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer, err := jwtpkg.NewSigner("test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/protected", RequireAuth(signer), func(c *gin.Context) {
		userID, ok := GetCurrentUserID(c)
		if !ok || userID != "user-1" {
			response.Fail(c, appErrors.Unauthorized())
			return
		}
		response.Success(c, map[string]string{"user_id": userID})
	})

	tests := []struct {
		name       string
		authorize  string
		wantStatus int
		wantCode   string
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized, wantCode: `"code":40101`},
		{name: "invalid", authorize: "Bearer invalid", wantStatus: http.StatusUnauthorized, wantCode: `"code":40101`},
		{name: "valid", authorize: mustSign(t, signer, "user-1"), wantStatus: http.StatusOK, wantCode: `"user_id":"user-1"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if test.authorize != "" {
				request.Header.Set("Authorization", test.authorize)
			}
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), test.wantCode) {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestRequireAuthReturnsExpiredCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer, err := jwtpkg.NewSigner("test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	expired := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "user-1", "iat": time.Now().Add(-2 * time.Hour).Unix(), "exp": time.Now().Add(-time.Hour).Unix()})
	token, err := expired.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/protected", RequireAuth(signer), func(c *gin.Context) { response.Success(c, struct{}{}) })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), `"code":40102`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRequireFamilyAndOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer, err := jwtpkg.NewSigner("test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/owner", RequireAuth(signer), RequireFamily(fakeFamilyRepository{summary: &familyapp.Summary{ID: "family-1", Role: "owner"}}), RequireOwner(), func(c *gin.Context) {
		familyID, _ := GetCurrentFamilyID(c)
		response.Success(c, map[string]string{"family_id": familyID})
	})
	router.GET("/member", RequireAuth(signer), RequireFamily(fakeFamilyRepository{summary: &familyapp.Summary{ID: "family-1", Role: "member"}}), RequireOwner(), func(c *gin.Context) {
		response.Success(c, struct{}{})
	})
	router.GET("/guest", RequireAuth(signer), RequireFamily(fakeFamilyRepository{}), func(c *gin.Context) { response.Success(c, struct{}{}) })

	for _, test := range []struct {
		path string
		code string
	}{
		{path: "/owner", code: `"family_id":"family-1"`},
		{path: "/member", code: `"code":40302`},
		{path: "/guest", code: `"code":40301`},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.Header.Set("Authorization", mustSign(t, signer, "user-1"))
		router.ServeHTTP(recorder, request)
		if !strings.Contains(recorder.Body.String(), test.code) {
			t.Fatalf("path=%s body=%s", test.path, recorder.Body.String())
		}
	}
}

func mustSign(t *testing.T, signer *jwtpkg.Signer, userID string) string {
	t.Helper()
	token, err := signer.Sign(userID)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + token
}
