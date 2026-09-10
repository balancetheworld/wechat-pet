package httpapi

import (
	"log/slog"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
	appauth "github.com/balancetheworld/wechat-pet/internal/app/auth"
	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	appuser "github.com/balancetheworld/wechat-pet/internal/app/user"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	"github.com/balancetheworld/wechat-pet/internal/pkg/response"
	fileservice "github.com/balancetheworld/wechat-pet/internal/service/file"
	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	AuthService      *appauth.Service
	UserService      *appuser.Service
	FamilyService    *familyapp.Service
	FamilyRepository familyapp.Repository
	PetService       *petapp.Service
	PetRepository    petapp.Repository
	CalendarService  *calendarapp.Service
	AskService       *askapp.Service
	AskRunQueue      askapp.RunEnqueuer
	TokenSigner      *jwtpkg.Signer
	FileService      *fileservice.Service
	LocalUploadDir   string
}

// New creates the API router and installs process-level middleware.
func New(loggers ...*slog.Logger) *gin.Engine {
	return NewWithDependencies(Dependencies{}, loggers...)
}

func NewWithDependencies(dependencies Dependencies, loggers ...*slog.Logger) *gin.Engine {
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	router := gin.New()
	router.Use(requestID(), accessLog(logger), recovery(logger))
	router.GET("/healthz", func(c *gin.Context) {
		response.Success(c, map[string]string{"status": "ok"})
	})
	v1 := router.Group("/api/v1")
	registerV1Routes(v1, dependencies)

	return router
}
