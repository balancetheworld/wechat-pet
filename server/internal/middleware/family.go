package middleware

import (
	"strings"

	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

const (
	familyIDKey   = "family_id"
	familyRoleKey = "family_role"
)

func RequireFamily(repository familyapp.ActiveFamilyRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := GetCurrentUserID(c)
		if !ok || repository == nil {
			response.Fail(c, appErrors.Forbidden())
			c.Abort()
			return
		}
		summary, err := repository.GetActiveFamilySummary(c.Request.Context(), userID)
		if err != nil {
			response.Fail(c, appErrors.Internal(err))
			c.Abort()
			return
		}
		if summary == nil || strings.TrimSpace(summary.ID) == "" {
			response.Fail(c, appErrors.Forbidden())
			c.Abort()
			return
		}
		c.Set(familyIDKey, summary.ID)
		c.Set(familyRoleKey, summary.Role)
		c.Next()
	}
}

func GetCurrentFamilyID(c *gin.Context) (string, bool) {
	value, exists := c.Get(familyIDKey)
	if !exists {
		return "", false
	}
	id, ok := value.(string)
	return id, ok && strings.TrimSpace(id) != ""
}

func GetCurrentFamilyRole(c *gin.Context) (string, bool) {
	value, exists := c.Get(familyRoleKey)
	if !exists {
		return "", false
	}
	role, ok := value.(string)
	return role, ok && strings.TrimSpace(role) != ""
}
