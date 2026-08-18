package config

import "testing"

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("HTTP_ADDR", "127.0.0.1:8081")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DatabaseURL != "postgres://example" {
		t.Errorf("Config.DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://example")
	}
	if cfg.HTTPAddr != "127.0.0.1:8081" {
		t.Errorf("Config.HTTPAddr = %q, want %q", cfg.HTTPAddr, "127.0.0.1:8081")
	}
}
