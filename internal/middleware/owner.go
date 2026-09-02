package middleware

import (
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func RequireOwner() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := GetCurrentFamilyRole(c)
		if !ok || role != "owner" {
			response.Fail(c, appErrors.NotOwner())
			c.Abort()
			return
		}
		c.Next()
	}
}
