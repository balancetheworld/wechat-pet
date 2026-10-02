package config

import (
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

func TestLoadReadsYAMLAndEnvironmentOverrides(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("app_env: test\nhttp_addr: ':8081'\ndatabase_dsn: yaml-dsn\njwt_expire_minutes: 30\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("DATABASE_DSN", "env-dsn")

	cfg, err := Load(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":9090" || cfg.DatabaseDSN != "env-dsn" || cfg.JWTExpireMinutes != 30 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadReadsDotEnvWithoutOverridingEnvironment(t *testing.T) {
	directory := t.TempDir()
	file, err := os.Create(directory + "/.env")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("app_env=development\nDATABASE_DRIVER=sqlite\nDATABASE_DSN=:memory:\nHTTP_ADDR=:8082\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(workingDirectory) })
	t.Setenv("HTTP_ADDR", ":9090")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseDriver != "sqlite" || cfg.DatabaseDSN != ":memory:" || cfg.HTTPAddr != ":9090" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestProductionRequiresSecrets(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_DSN", "postgres://example")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("WECHAT_APP_ID", "app-id")
	t.Setenv("WECHAT_APP_SECRET", "")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("Load() error = %v, want JWT_SECRET validation error", err)
	}
}

func TestProductionLocalStorageRequiresPublicBaseURL(t *testing.T) {
	cfg := Config{
		AppEnv:           "production",
		HTTPAddr:         ":8080",
		DatabaseDriver:   "postgres",
		DatabaseDSN:      "postgres://example",
		JWTSecret:        "secret",
		JWTExpireMinutes: 120,
		WeChatAppID:      "app-id",
		WeChatAppSecret:  "app-secret",
		StorageDriver:    "local",
		LocalUploadDir:   "data/uploads",
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "PUBLIC_BASE_URL") {
		t.Fatalf("Validate() error = %v, want PUBLIC_BASE_URL validation error", err)
	}
}

func TestStringRedactsSecrets(t *testing.T) {
	cfg := Config{JWTSecret: "jwt", WeChatAppSecret: "wechat", COSSecretKey: "cos", AIAPIKey: "ai"}
	value := cfg.String()
	if strings.Contains(value, "\"JWTSecret\":\"jwt\"") || strings.Contains(value, "\"WeChatAppSecret\":\"wechat\"") || strings.Contains(value, "\"COSSecretKey\":\"cos\"") || strings.Contains(value, "\"AIAPIKey\":\"ai\"") {
		t.Fatalf("config string contains secret: %s", value)
	}
}

func TestAIConfigRequiresCredentialsModelAndTimeout(t *testing.T) {
	cfg := Config{AppEnv: "test", HTTPAddr: ":8080", DatabaseDriver: "sqlite", DatabaseDSN: ":memory:", JWTExpireMinutes: 120, StorageDriver: "local", LocalUploadDir: "data/uploads", AIEnabled: true, AITimeoutSeconds: 30}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "AI_API_KEY") {
		t.Fatalf("Validate() error = %v, want AI_API_KEY validation error", err)
	}
	cfg.AIAPIKey = "key"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "AI_MODEL") {
		t.Fatalf("Validate() error = %v, want AI_MODEL validation error", err)
	}
	cfg.AIModel = "model"
	cfg.AITimeoutSeconds = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "AI_TIMEOUT_SECONDS") {
		t.Fatalf("Validate() error = %v, want AI_TIMEOUT_SECONDS validation error", err)
	}
}

func TestAIConfigRejectsUnsupportedProvider(t *testing.T) {
	cfg := Config{AppEnv: "test", HTTPAddr: ":8080", DatabaseDriver: "sqlite", DatabaseDSN: ":memory:", JWTExpireMinutes: 120, StorageDriver: "local", LocalUploadDir: "data/uploads", AIEnabled: true, AIProvider: "chat_completion", AIAPIKey: "key", AIModel: "deepseek-chat", AITimeoutSeconds: 30}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "AI_PROVIDER") {
		t.Fatalf("Validate() error = %v, want AI_PROVIDER validation error", err)
	}
	cfg.AIProvider = "openai"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want openai accepted", err)
	}
}

func TestExampleConfigFilesMatchAIProviderValidation(t *testing.T) {
	yamlConfig, err := Load("../../../config.example.yaml")
	if err != nil {
		t.Fatalf("Load(config.example.yaml) error = %v", err)
	}
	envValues, err := godotenv.Read("../../../.env.example")
	if err != nil {
		t.Fatalf("read .env.example error = %v", err)
	}
	// 示例默认关闭 AI，这里按示例给出的 Provider、地址与模型打开 AI，确认示例组合本身能通过校验。
	for _, example := range []struct {
		file     string
		provider string
		baseURL  string
		model    string
	}{
		{"config.example.yaml", yamlConfig.AIProvider, yamlConfig.AIBaseURL, yamlConfig.AIModel},
		{".env.example", envValues["AI_PROVIDER"], envValues["AI_BASE_URL"], envValues["AI_MODEL"]},
	} {
		cfg := Config{AppEnv: "test", HTTPAddr: ":8080", DatabaseDriver: "sqlite", DatabaseDSN: ":memory:", JWTExpireMinutes: 120, StorageDriver: "local", LocalUploadDir: "data/uploads", AIEnabled: true, AIProvider: example.provider, AIAPIKey: "key", AIBaseURL: example.baseURL, AIModel: example.model, AITimeoutSeconds: 30}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("%s AI config is rejected: %v", example.file, err)
		}
	}
}
