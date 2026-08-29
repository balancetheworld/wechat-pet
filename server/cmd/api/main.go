package main

import (
	"context"
	"os"
	"time"

	appauth "github.com/balancetheworld/wechat-pet/server/internal/app/auth"
	familyapp "github.com/balancetheworld/wechat-pet/server/internal/app/family"
	petapp "github.com/balancetheworld/wechat-pet/server/internal/app/pet"
	userapp "github.com/balancetheworld/wechat-pet/server/internal/app/user"
	"github.com/balancetheworld/wechat-pet/server/internal/httpapi"
	"github.com/balancetheworld/wechat-pet/server/internal/logging"
	"github.com/balancetheworld/wechat-pet/server/internal/pkg/config"
	"github.com/balancetheworld/wechat-pet/server/internal/pkg/database"
	jwtpkg "github.com/balancetheworld/wechat-pet/server/internal/pkg/jwt"
	"github.com/balancetheworld/wechat-pet/server/internal/platform/wechat"
)

func main() {
	logger := logging.New()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.Open(ctx, cfg)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	userRepository, err := userapp.NewRepository(db, cfg.DatabaseDriver)
	if err != nil {
		logger.Error("create user repository", "error", err)
		os.Exit(1)
	}
	familyRepository, err := familyapp.NewRepository(db, cfg.DatabaseDriver)
	if err != nil {
		logger.Error("create family repository", "error", err)
		os.Exit(1)
	}
	tokenSigner, err := jwtpkg.NewSigner(cfg.JWTSecret, time.Duration(cfg.JWTExpireMinutes)*time.Minute)
	if err != nil {
		logger.Error("create jwt signer", "error", err)
		os.Exit(1)
	}
	wechatClient := wechat.NewHTTPClient(cfg.WeChatAppID, cfg.WeChatAppSecret, 5*time.Second)
	authService, err := appauth.NewService(wechatClient, userRepository, familyRepository, tokenSigner)
	if err != nil {
		logger.Error("create auth service", "error", err)
		os.Exit(1)
	}
	userService, err := userapp.NewService(userRepository, familyRepository)
	if err != nil {
		logger.Error("create user service", "error", err)
		os.Exit(1)
	}
	familyService, err := familyapp.NewService(familyRepository)
	if err != nil {
		logger.Error("create family service", "error", err)
		os.Exit(1)
	}
	petRepository, err := petapp.NewRepository(db, cfg.DatabaseDriver)
	if err != nil {
		logger.Error("create pet repository", "error", err)
		os.Exit(1)
	}
	petService, err := petapp.NewService(petRepository)
	if err != nil {
		logger.Error("create pet service", "error", err)
		os.Exit(1)
	}
	server := httpapi.NewWithDependencies(httpapi.Dependencies{AuthService: authService, UserService: userService, FamilyService: familyService, FamilyRepository: familyRepository, PetService: petService, PetRepository: petRepository, TokenSigner: tokenSigner}, logger)
	logger.Info("api server starting", "addr", cfg.HTTPAddr)
	if err := server.Run(cfg.HTTPAddr); err != nil {
		logger.Error("api server stopped", "error", err)
		os.Exit(1)
	}
}
