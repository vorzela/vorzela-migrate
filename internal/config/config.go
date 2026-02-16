package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds application configuration
type Config struct {
	DatabaseURL   string
	MigrationPath string
	SqlcSupport   bool // Enable goose markers for sqlc compatibility
}

// LoadConfig loads configuration with optional DSN and path overrides
func LoadConfig(dsnOverride, pathOverride string) (*Config, error) {
	cfg := &Config{
		MigrationPath: "./migrations",
	}

	// Load .env file if it exists (doesn't error if file doesn't exist)
	_ = godotenv.Load()

	// Load .vm config file if it exists
	if err := loadVorzelaConfig(cfg); err != nil {
		return nil, err
	}

	// Override with environment variables
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		cfg.DatabaseURL = databaseURL
	}

	// Apply CLI overrides
	if dsnOverride != "" {
		cfg.DatabaseURL = dsnOverride
	}
	if pathOverride != "" && pathOverride != "./migrations" {
		cfg.MigrationPath = pathOverride
	}

	return cfg, nil
}

// LoadConfigWithOverrides is deprecated, use LoadConfig instead
func LoadConfigWithOverrides(dsnOverride, envOverride, pathOverride string) (*Config, error) {
	// Ignore envOverride, just delegate to LoadConfig
	return LoadConfig(dsnOverride, pathOverride)
}

// loadVorzelaConfig loads configuration from .vm file
func loadVorzelaConfig(cfg *Config) error {
	// Check current directory
	if err := loadVorzelaFile(".vm", cfg); err == nil {
		return nil
	}

	// Check parent directories
	currentDir, err := os.Getwd()
	if err != nil {
		return nil // Don't fail if we can't read cwd
	}

	for {
		dir := filepath.Dir(currentDir)
		if dir == currentDir {
			break // Reached root
		}

		if err := loadVorzelaFile(filepath.Join(dir, ".vm"), cfg); err == nil {
			return nil
		}

		currentDir = dir
	}

	return nil // Config file is optional
}

// loadVorzelaFile loads a specific .vm file
func loadVorzelaFile(filepath string, cfg *Config) error {
	content, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return err
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "DATABASE_URL":
			cfg.DatabaseURL = value
		case "MIGRATION_PATH":
			cfg.MigrationPath = value
		case "SQLC_SUPPORT":
			cfg.SqlcSupport = strings.ToLower(value) == "true" || value == "1"
		}
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("database URL is required. Set DATABASE_URL env var, create .vm config file, or use --dsn flag")
	}

	return nil
}
