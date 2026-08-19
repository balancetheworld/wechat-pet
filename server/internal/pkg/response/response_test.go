package response

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	appErrors "github.com/balancetheworld/wechat-pet/server/internal/pkg/errors"
	"github.com/gin-gonic/gin"
)

func TestSuccessUsesCommonShape(t *testing.T) {
	c, recorder := testContext("request-123")
	Success(c, map[string]string{"name": "pet"})

	var body Response[map[string]string]
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || body.Code != 0 || body.Message != "ok" || body.RequestID != "request-123" {
		t.Fatalf("unexpected response: %+v status=%d", body, recorder.Code)
	}
}

func TestFailMapsErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    *appErrors.AppError
		status int
		code   int
	}{
		{name: "invalid param", err: appErrors.InvalidParam("参数错误"), status: 400, code: appErrors.CodeInvalidParam},
		{name: "unauthorized", err: appErrors.Unauthorized(), status: 401, code: appErrors.CodeUnauthorized},
		{name: "forbidden", err: appErrors.Forbidden(), status: 403, code: appErrors.CodeForbidden},
		{name: "internal", err: appErrors.Internal(assertErr{}), status: 500, code: appErrors.CodeInternal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, recorder := testContext("")
			Fail(c, test.err)
			var body Response[any]
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != test.status || body.Code != test.code || body.RequestID == "" {
				t.Fatalf("unexpected response: %+v status=%d", body, recorder.Code)
			}
			if test.name == "internal" && body.Message != "服务器内部错误" {
				t.Fatalf("internal error leaked message: %q", body.Message)
			}
		})
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "database password leaked" }

func testContext(requestID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/", nil)
	if requestID != "" {
		c.Request.Header.Set("X-Request-ID", requestID)
	}
	return c, recorder
}
