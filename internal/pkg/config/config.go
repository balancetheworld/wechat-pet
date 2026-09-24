package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	AppEnv            string `yaml:"app_env"`
	HTTPAddr          string `yaml:"http_addr"`
	DatabaseDriver    string `yaml:"database_driver"`
	DatabaseDSN       string `yaml:"database_dsn"`
	JWTSecret         string `yaml:"jwt_secret"`
	JWTExpireMinutes  int    `yaml:"jwt_expire_minutes"`
	WeChatAppID       string `yaml:"wechat_app_id"`
	WeChatAppSecret   string `yaml:"wechat_app_secret"`
	StorageDriver     string `yaml:"storage_driver"`
	LocalUploadDir    string `yaml:"local_upload_dir"`
	PublicBaseURL     string `yaml:"public_base_url"`
	COSSecretID       string `yaml:"cos_secret_id"`
	COSSecretKey      string `yaml:"cos_secret_key"`
	COSBucket         string `yaml:"cos_bucket"`
	RedisAddr         string `yaml:"redis_addr"`
	AIEnabled         bool   `yaml:"ai_enabled"`
	AIProvider        string `yaml:"ai_provider"`
	AIAPIKey          string `yaml:"ai_api_key"`
	AIBaseURL         string `yaml:"ai_base_url"`
	AIModel           string `yaml:"ai_model"`
	AITimeoutSeconds  int    `yaml:"ai_timeout_seconds"`
	AIReasoningEffort string `yaml:"ai_reasoning_effort"`
}

func Load(yamlPaths ...string) (Config, error) {
	if err := loadDotEnv(); err != nil {
		return Config{}, err
	}
	cfg := Config{AppEnv: "development", HTTPAddr: ":8080", DatabaseDriver: "postgres", JWTExpireMinutes: 120, StorageDriver: "local", LocalUploadDir: "data/uploads", AIProvider: "openai", AIBaseURL: "https://api.openai.com/v1", AITimeoutSeconds: 30}
	yamlPath := ""
	if len(yamlPaths) > 0 {
		yamlPath = yamlPaths[0]
	}
	if yamlPath == "" {
		yamlPath = os.Getenv("CONFIG_FILE")
	}
	if yamlPath != "" {
		data, err := os.ReadFile(yamlPath)
		if err != nil {
			return Config{}, fmt.Errorf("read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse config file: %w", err)
		}
	}
	if err := applyEnvironment(&cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadDotEnv() error {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production") {
		return nil
	}
	values, err := godotenv.Read(".env")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read .env: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(values["APP_ENV"]), "production") {
		return nil
	}
	for name, value := range values {
		if _, exists := os.LookupEnv(name); !exists {
			if err := os.Setenv(name, value); err != nil {
				return fmt.Errorf("load .env variable %s: %w", name, err)
			}
		}
	}
	return nil
}

func applyEnvironment(cfg *Config) error {
	setString("APP_ENV", &cfg.AppEnv)
	setString("HTTP_ADDR", &cfg.HTTPAddr)
	setString("DATABASE_DRIVER", &cfg.DatabaseDriver)
	setString("DATABASE_DSN", &cfg.DatabaseDSN)
	setString("JWT_SECRET", &cfg.JWTSecret)
	setString("WECHAT_APP_ID", &cfg.WeChatAppID)
	setString("WECHAT_APP_SECRET", &cfg.WeChatAppSecret)
	setString("STORAGE_DRIVER", &cfg.StorageDriver)
	setString("LOCAL_UPLOAD_DIR", &cfg.LocalUploadDir)
	setString("PUBLIC_BASE_URL", &cfg.PublicBaseURL)
	setString("COS_SECRET_ID", &cfg.COSSecretID)
	setString("COS_SECRET_KEY", &cfg.COSSecretKey)
	setString("COS_BUCKET", &cfg.COSBucket)
	setString("REDIS_ADDR", &cfg.RedisAddr)
	setString("AI_API_KEY", &cfg.AIAPIKey)
	setString("AI_PROVIDER", &cfg.AIProvider)
	setString("AI_BASE_URL", &cfg.AIBaseURL)
	setString("AI_MODEL", &cfg.AIModel)
	setString("AI_REASONING_EFFORT", &cfg.AIReasoningEffort)
	if value, ok := os.LookupEnv("JWT_EXPIRE_MINUTES"); ok {
		minutes, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("JWT_EXPIRE_MINUTES must be an integer: %w", err)
		}
		cfg.JWTExpireMinutes = minutes
	}
	if value, ok := os.LookupEnv("AI_ENABLED"); ok {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("AI_ENABLED must be a boolean: %w", err)
		}
		cfg.AIEnabled = enabled
	}
	if value, ok := os.LookupEnv("AI_TIMEOUT_SECONDS"); ok {
		seconds, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("AI_TIMEOUT_SECONDS must be an integer: %w", err)
		}
		cfg.AITimeoutSeconds = seconds
	}
	return nil
}

func setString(name string, target *string) {
	if value, ok := os.LookupEnv(name); ok {
		*target = value
	}
}

func (c Config) Validate() error {
	if c.AppEnv != "development" && c.AppEnv != "test" && c.AppEnv != "production" {
		return errors.New("APP_ENV must be development, test, or production")
	}
	if strings.TrimSpace(c.HTTPAddr) == "" {
		return errors.New("HTTP_ADDR is required")
	}
	if strings.TrimSpace(c.DatabaseDriver) == "" {
		return errors.New("DATABASE_DRIVER is required")
	}
	if strings.TrimSpace(c.DatabaseDSN) == "" {
		return errors.New("DATABASE_DSN is required")
	}
	if c.JWTExpireMinutes <= 0 {
		return errors.New("JWT_EXPIRE_MINUTES must be greater than zero")
	}
	if c.StorageDriver == "local" && strings.TrimSpace(c.LocalUploadDir) == "" {
		return errors.New("LOCAL_UPLOAD_DIR is required for local storage")
	}
	if c.StorageDriver == "cos" && (c.COSSecretID == "" || c.COSSecretKey == "" || c.COSBucket == "") {
		return errors.New("COS_SECRET_ID, COS_SECRET_KEY, and COS_BUCKET are required for COS storage")
	}
	if c.StorageDriver != "local" && c.StorageDriver != "cos" {
		return errors.New("STORAGE_DRIVER must be local or cos")
	}
	if c.AIEnabled {
		provider := c.AIProvider
		if provider == "" {
			provider = "openai"
		}
		if provider != "openai" && provider != "chat_completion" {
			return errors.New("AI_PROVIDER must be openai or chat_completion")
		}
		if strings.TrimSpace(c.AIAPIKey) == "" {
			return errors.New("AI_API_KEY is required when AI is enabled")
		}
		if strings.TrimSpace(c.AIModel) == "" {
			return errors.New("AI_MODEL is required when AI is enabled")
		}
		if c.AITimeoutSeconds <= 0 {
			return errors.New("AI_TIMEOUT_SECONDS must be greater than zero")
		}
		switch c.AIReasoningEffort {
		case "", "minimal", "low", "medium", "high":
		default:
			return errors.New("AI_REASONING_EFFORT must be minimal, low, medium, or high")
		}
	}
	if c.AppEnv == "production" {
		if strings.TrimSpace(c.JWTSecret) == "" {
			return errors.New("JWT_SECRET is required in production")
		}
		if strings.TrimSpace(c.WeChatAppID) == "" || strings.TrimSpace(c.WeChatAppSecret) == "" {
			return errors.New("WECHAT_APP_ID and WECHAT_APP_SECRET are required in production")
		}
		if c.StorageDriver == "local" && strings.TrimSpace(c.PublicBaseURL) == "" {
			return errors.New("PUBLIC_BASE_URL is required for local storage in production")
		}
	}
	return nil
}

func (c Config) Redacted() Config {
	c.DatabaseDSN = mask(c.DatabaseDSN)
	c.JWTSecret = mask(c.JWTSecret)
	c.WeChatAppSecret = mask(c.WeChatAppSecret)
	c.COSSecretID = mask(c.COSSecretID)
	c.COSSecretKey = mask(c.COSSecretKey)
	c.AIAPIKey = mask(c.AIAPIKey)
	return c
}

func (c Config) String() string {
	data, err := json.Marshal(c.Redacted())
	if err != nil {
		return "{}"
	}
	return string(data)
}

func mask(value string) string {
	if value == "" {
		return ""
	}
	return "******"
}
