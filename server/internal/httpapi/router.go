package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// New creates the API router and installs process-level middleware.
func New() *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	return router
}
