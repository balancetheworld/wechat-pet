package httpapi

import "github.com/gin-gonic/gin"

func registerV1Routes(router *gin.RouterGroup) {
	registerModuleRoutes(router)
}

func registerModuleRoutes(router *gin.RouterGroup) {}
