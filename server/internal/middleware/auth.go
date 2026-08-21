package middleware

import (
	"strings"

	appErrors "github.com/balancetheworld/wechat-pet/server/internal/pkg/errors"
	jwtpkg "github.com/balancetheworld/wechat-pet/server/internal/pkg/jwt"
	"github.com/balancetheworld/wechat-pet/server/internal/pkg/response"
	"github.com/gin-gonic/gin"
	jwt "github.com/golang-jwt/jwt"
)

const userIDKey = "user_id"

func RequireAuth(signer *jwtpkg.Signer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if signer == nil {
			response.Fail(c, appErrors.Internal(nil))
			c.Abort()
			return
		}
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Fail(c, appErrors.Unauthorized())
			c.Abort()
			return
		}
		claims, err := signer.Verify(parts[1])
		if err != nil {
			if validation, ok := err.(*jwt.ValidationError); ok && validation.Errors&jwt.ValidationErrorExpired != 0 {
				response.Fail(c, appErrors.TokenExpired())
			} else {
				response.Fail(c, appErrors.Unauthorized())
			}
			c.Abort()
			return
		}
		c.Set(userIDKey, claims.UserID)
		c.Next()
	}
}

func GetCurrentUserID(c *gin.Context) (string, bool) {
	value, exists := c.Get(userIDKey)
	if !exists {
		return "", false
	}
	userID, ok := value.(string)
	return userID, ok && strings.TrimSpace(userID) != ""
}
