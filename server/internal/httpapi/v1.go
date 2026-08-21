package httpapi

import (
	authapi "github.com/balancetheworld/wechat-pet/server/internal/httpapi/auth"
	userapi "github.com/balancetheworld/wechat-pet/server/internal/httpapi/user"
	"github.com/gin-gonic/gin"
)

func registerV1Routes(router *gin.RouterGroup, dependencies Dependencies) {
	authapi.RegisterRoutes(router, dependencies.AuthService)
	userapi.RegisterRoutes(router, dependencies.TokenSigner, dependencies.UserService)
}
