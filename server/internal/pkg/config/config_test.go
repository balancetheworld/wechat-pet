package config

import (
	"os"
	"strings"
	"testing"
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

func TestStringRedactsSecrets(t *testing.T) {
	cfg := Config{JWTSecret: "jwt", WeChatAppSecret: "wechat", COSSecretKey: "cos"}
	value := cfg.String()
	if strings.Contains(value, "\"JWTSecret\":\"jwt\"") || strings.Contains(value, "\"WeChatAppSecret\":\"wechat\"") || strings.Contains(value, "\"COSSecretKey\":\"cos\"") {
		t.Fatalf("config string contains secret: %s", value)
	}
}
