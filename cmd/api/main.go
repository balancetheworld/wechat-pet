package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
	appauth "github.com/balancetheworld/wechat-pet/internal/app/auth"
	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	userapp "github.com/balancetheworld/wechat-pet/internal/app/user"
	"github.com/balancetheworld/wechat-pet/internal/httpapi"
	"github.com/balancetheworld/wechat-pet/internal/logging"
	"github.com/balancetheworld/wechat-pet/internal/pkg/config"
	"github.com/balancetheworld/wechat-pet/internal/pkg/database"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	aiplatform "github.com/balancetheworld/wechat-pet/internal/platform/ai"
	"github.com/balancetheworld/wechat-pet/internal/platform/storage"
	"github.com/balancetheworld/wechat-pet/internal/platform/wechat"
	fileservice "github.com/balancetheworld/wechat-pet/internal/service/file"
)

func newStorage(cfg config.Config) (storage.Storage, error) {
	if cfg.StorageDriver == "local" {
		publicBaseURL := strings.TrimRight(cfg.PublicBaseURL, "/")
		if publicBaseURL == "" {
			publicBaseURL = "http://127.0.0.1" + cfg.HTTPAddr
		}
		return storage.NewLocalStorage(cfg.LocalUploadDir, publicBaseURL+"/api/v1/uploads")
	}
	return storage.NewCOSStorage(cfg.COSBucket, cfg.COSSecretID, cfg.COSSecretKey, 10*time.Second, 15*time.Minute)
}

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

	store, err := newStorage(cfg)
	if err != nil {
		logger.Error("create storage", "error", err)
		os.Exit(1)
	}
	userRepository, err := userapp.NewRepository(db, cfg.DatabaseDriver)
	if err != nil {
		logger.Error("create user repository", "error", err)
		os.Exit(1)
	}
	assetRepository, err := fileservice.NewRepository(db, cfg.DatabaseDriver)
	if err != nil {
		logger.Error("create asset repository", "error", err)
		os.Exit(1)
	}
	fileService, err := fileservice.NewService(store, assetRepository)
	if err != nil {
		logger.Error("create file service", "error", err)
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
	authService, err := appauth.NewService(wechatClient, userRepository, familyRepository, tokenSigner, store)
	if err != nil {
		logger.Error("create auth service", "error", err)
		os.Exit(1)
	}
	userService, err := userapp.NewService(userRepository, familyRepository, store)
	if err != nil {
		logger.Error("create user service", "error", err)
		os.Exit(1)
	}
	userService.SetAssetAuthorizer(fileService)
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
	petService, err := petapp.NewService(petRepository, fileService)
	if err != nil {
		logger.Error("create pet service", "error", err)
		os.Exit(1)
	}
	calendarRepository, err := calendarapp.NewRepository(db, cfg.DatabaseDriver)
	if err != nil {
		logger.Error("create calendar repository", "error", err)
		os.Exit(1)
	}
	calendarService, err := calendarapp.NewService(calendarRepository, store)
	if err != nil {
		logger.Error("create calendar service", "error", err)
		os.Exit(1)
	}
	calendarService.SetAssetAuthorizer(fileService)
	if templateID := strings.TrimSpace(cfg.WeChatReminderTemplateID); templateID != "" {
		miniprogramState := "formal"
		if cfg.AppEnv != "production" {
			miniprogramState = "developer"
		}
		subscribeClient, subscribeErr := wechat.NewSubscribeClient(cfg.WeChatAppID, cfg.WeChatAppSecret, miniprogramState, 5*time.Second)
		if subscribeErr != nil {
			logger.Error("create wechat subscribe client", "error", subscribeErr)
			os.Exit(1)
		}
		reminderNotifier, notifierErr := wechat.NewReminderNotifier(subscribeClient, templateID)
		if notifierErr != nil {
			logger.Error("create wechat reminder notifier", "error", notifierErr)
			os.Exit(1)
		}
		calendarService.SetReminderPush(reminderNotifier, templateID)
	}
	calendarService.SetReminderSendHour(cfg.ReminderSendHour)
	askRepository, err := askapp.NewRepository(db, cfg.DatabaseDriver)
	if err != nil {
		logger.Error("create ask repository", "error", err)
		os.Exit(1)
	}
	providerConfig := aiplatform.OpenAIConfig{APIKey: cfg.AIAPIKey, BaseURL: cfg.AIBaseURL, Model: cfg.AIModel, Timeout: time.Duration(cfg.AITimeoutSeconds) * time.Second, ReasoningEffort: cfg.AIReasoningEffort, Observer: func(observation aiplatform.OpenAIObservation) {
		logger.Info("ask ai provider", "provider", cfg.AIProvider, "operation", observation.Operation, "model", observation.Model, "duration_ms", observation.Duration.Milliseconds(), "input_tokens", observation.InputTokens, "output_tokens", observation.OutputTokens, "total_tokens", observation.TotalTokens, "status", observation.Status, "error_code", observation.ErrorCode, "retryable", observation.Retryable, "strict_fallback", observation.StrictFallback)
	}}
	askService, err := askapp.NewService(askRepository, petRepository)
	if err != nil {
		logger.Error("create ask service", "error", err)
		os.Exit(1)
	}
	if cfg.AppEnv == "development" {
		askService.SetDebugLogger(logger)
	}
	askService.SetCalendarRepository(calendarRepository)
	askService.SetCalendarWriter(calendarService)
	// v2 工具目录与业务读取端口（不依赖 AI 启用，供决策循环工具执行使用）。
	askCatalog, err := askapp.DefaultCatalog(askapp.DefaultToolVersion)
	if err != nil {
		logger.Error("create ask tool catalog", "error", err)
		os.Exit(1)
	}
	askService.SetToolCatalog(askCatalog)
	askService.SetBusinessReadRepository(askapp.NewBusinessReadRepository(petRepository, calendarRepository, askRepository))
	askService.SetImageAssetReader(fileService)
	// v2 决策循环模型端口（依赖 AI 启用）。
	if cfg.AIEnabled {
		askProvider, providerErr := aiplatform.NewOpenAIProvider(providerConfig)
		if providerErr != nil {
			logger.Error("create ask v2 provider", "error", providerErr)
			os.Exit(1)
		}
		askProfile := aiplatform.Profile{
			ProviderID:     "openai",
			Model:          cfg.AIModel,
			Version:        "ask-profile-v1",
			AdapterVersion: "openai-responses-v1",
			Capabilities: aiplatform.Capabilities{
				TextInput:          aiplatform.CapabilitySupported,
				ImageInput:         aiplatform.CapabilitySupported,
				ToolCalling:        aiplatform.CapabilitySupported,
				StreamingText:      aiplatform.CapabilitySupported,
				StreamingToolCall:  aiplatform.CapabilitySupported,
				StructuredOutput:   aiplatform.CapabilitySupported,
				Cancellation:       aiplatform.CapabilitySupported,
				UsageNormalization: aiplatform.CapabilitySupported,
			},
			Parameters: []aiplatform.Parameter{{Name: "max_output_tokens", Value: 8192, Required: true}},
		}
		askService.SetAgentModel(aiplatform.NewAgentModelAdapter(askProvider, askProfile, askRepository))
	}
	askWorker, err := askapp.NewRunWorker(askService, askRepository, askapp.RunWorkerConfig{
		QueueSize:        100,
		MaxAttempts:      3,
		RetryDelay:       500 * time.Millisecond,
		PollInterval:     time.Second,
		LeaseDuration:    30 * time.Second,
		ExecutionTimeout: 2 * time.Minute,
		BatchSize:        20,
		OnError: func(job askapp.RunJob, err error) {
			logger.Error("process ask run", "family_id", job.FamilyID, "session_id", job.SessionID, "run_id", job.RunID, "row_version", job.RowVersion, "error", err)
		},
	})
	if err != nil {
		logger.Error("create ask run worker", "error", err)
		os.Exit(1)
	}
	askWorker.Start(context.Background())
	localUploadDir := ""
	if cfg.StorageDriver == "local" {
		localUploadDir = cfg.LocalUploadDir
	}
	router := httpapi.NewWithDependencies(httpapi.Dependencies{AuthService: authService, UserService: userService, FamilyService: familyService, FamilyRepository: familyRepository, PetService: petService, PetRepository: petRepository, CalendarService: calendarService, AskService: askService, AskRunQueue: askWorker, TokenSigner: tokenSigner, FileService: fileService, LocalUploadDir: localUploadDir}, logger)
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: router}
	serverErrors := make(chan error, 1)
	logger.Info("api server starting", "addr", cfg.HTTPAddr)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	shutdownSignal, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			dispatchCtx, cancelDispatch := context.WithTimeout(shutdownSignal, 30*time.Second)
			result, dispatchErr := calendarService.DispatchReminderNotifications(dispatchCtx, time.Now())
			cancelDispatch()
			if dispatchErr != nil {
				logger.Error("dispatch reminder notifications", "error", dispatchErr)
			} else if result.Scanned > 0 || result.Retried > 0 {
				logger.Info("dispatch reminder notifications", "scanned", result.Scanned, "sent", result.Sent, "skipped", result.Skipped, "failed", result.Failed, "retried", result.Retried)
			}
			select {
			case <-shutdownSignal.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	select {
	case <-shutdownSignal.Done():
		logger.Info("api server stopping")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown api server", "error", err)
		}
		askWorker.Close()
	case err := <-serverErrors:
		askWorker.Close()
		if errors.Is(err, http.ErrServerClosed) {
			return
		}
		logger.Error("api server stopped", "error", err)
		os.Exit(1)
	}
}
