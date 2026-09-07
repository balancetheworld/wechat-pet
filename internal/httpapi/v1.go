package httpapi

import (
	assetapi "github.com/balancetheworld/wechat-pet/internal/httpapi/asset"
	authapi "github.com/balancetheworld/wechat-pet/internal/httpapi/auth"
	calendarapi "github.com/balancetheworld/wechat-pet/internal/httpapi/calendar"
	familyapi "github.com/balancetheworld/wechat-pet/internal/httpapi/family"
	petapi "github.com/balancetheworld/wechat-pet/internal/httpapi/pet"
	userapi "github.com/balancetheworld/wechat-pet/internal/httpapi/user"
	"github.com/gin-gonic/gin"
)

func registerV1Routes(router *gin.RouterGroup, dependencies Dependencies) {
	assetapi.RegisterRoutes(router, dependencies.TokenSigner, dependencies.FileService, dependencies.LocalUploadDir, dependencies.FamilyRepository)
	authapi.RegisterRoutes(router, dependencies.AuthService)
	userapi.RegisterRoutes(router, dependencies.TokenSigner, dependencies.UserService)
	familyapi.RegisterRoutes(router, dependencies.TokenSigner, dependencies.FamilyService, dependencies.FamilyRepository)
	petapi.RegisterRoutes(router, dependencies.TokenSigner, dependencies.FamilyRepository, dependencies.PetService)
	calendarapi.RegisterRoutes(router, dependencies.TokenSigner, dependencies.FamilyRepository, dependencies.CalendarService)
}
