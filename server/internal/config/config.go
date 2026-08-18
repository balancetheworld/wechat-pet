package config

import (
	"fmt"
	"os"
)

// Config contains the API process configuration loaded from environment variables.
type Config struct {
	HTTPAddr    string
	DatabaseURL string
}

// Load reads the API process configuration from environment variables.
func Load() (Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	return Config{
		HTTPAddr:    httpAddr,
		DatabaseURL: databaseURL,
	}, nil
}
