package errors

import "net/http"

const (
	CodeInvalidParam = 40001
	CodeUnauthorized = 40101
	CodeTokenExpired = 40102
	CodeForbidden    = 40301
	CodeNotOwner     = 40302
	CodeInternal     = 50000
)

type AppError struct {
	HTTPStatus int
	Code       int
	Message    string
	Cause      error
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func InvalidParam(message string) *AppError {
	return &AppError{HTTPStatus: http.StatusBadRequest, Code: CodeInvalidParam, Message: message}
}

func Unauthorized() *AppError {
	return &AppError{HTTPStatus: http.StatusUnauthorized, Code: CodeUnauthorized, Message: "未认证"}
}

func TokenExpired() *AppError {
	return &AppError{HTTPStatus: http.StatusUnauthorized, Code: CodeTokenExpired, Message: "登录已过期"}
}

func Forbidden() *AppError {
	return &AppError{HTTPStatus: http.StatusForbidden, Code: CodeForbidden, Message: "无权限"}
}

func NotOwner() *AppError {
	return &AppError{HTTPStatus: http.StatusForbidden, Code: CodeNotOwner, Message: "仅家庭拥有者可操作"}
}

func Internal(cause error) *AppError {
	return &AppError{HTTPStatus: http.StatusInternalServerError, Code: CodeInternal, Message: "服务器内部错误", Cause: cause}
}
