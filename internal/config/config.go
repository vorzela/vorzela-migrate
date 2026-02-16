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
	SqlcSupport   bool   // Enable goose markers for sqlc compatibility
	Environment   string // dev, development, prod, production
	
	// Migration strategy settings
	Enhanced        bool
	Online          bool
	VerifyChecksums bool
	DetectDrift     bool
	Verbose         bool
	
	// Drift handling: auto, reject, prompt
	DriftHandling string
}

// LoadConfig loads configuration with optional DSN and path overrides
func LoadConfig(dsnOverride, pathOverride string) (*Config, error) {
	cfg := &Config{
		MigrationPath: "./migrations",
		Environment:   "development", // default to development
		DriftHandling: "prompt",      // default to interactive prompt
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

	// Apply environment-based defaults
	cfg.ApplyEnvironmentDefaults()

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
		case "ENVIRONMENT", "ENV":
			cfg.Environment = strings.ToLower(value)
		case "ENHANCED":
			cfg.Enhanced = strings.ToLower(value) == "true" || value == "1"
		case "ONLINE":
			cfg.Online = strings.ToLower(value) == "true" || value == "1"
		case "VERIFY_CHECKSUMS":
			cfg.VerifyChecksums = strings.ToLower(value) == "true" || value == "1"
		case "DETECT_DRIFT":
			cfg.DetectDrift = strings.ToLower(value) == "true" || value == "1"
		case "VERBOSE":
			cfg.Verbose = strings.ToLower(value) == "true" || value == "1"
		case "DRIFT_HANDLING":
			val := strings.ToLower(value)
			if val == "auto" || val == "reject" || val == "prompt" {
				cfg.DriftHandling = val
			}
		}
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("database URL is required. Set DATABASE_URL env var, create .vm config file, or use --dsn flag")
	}

	// Normalize environment
	c.NormalizeEnvironment()

	return nil
}

// NormalizeEnvironment normalizes environment names
func (c *Config) NormalizeEnvironment() {
	env := strings.ToLower(c.Environment)
	
	// Normalize dev/development
	if env == "dev" || env == "develop" || env == "development" {
		c.Environment = "development"
		return
	}
	
	// Normalize prod/production
	if env == "prod" || env == "production" {
		c.Environment = "production"
		return
	}
	
	// Default to development if unknown
	c.Environment = "development"
}

// IsProduction returns true if running in production environment
func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}

// IsDevelopment returns true if running in development environment
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

// ApplyEnvironmentDefaults applies default settings based on environment
func (c *Config) ApplyEnvironmentDefaults() {
	c.NormalizeEnvironment()
	
	// If no explicit settings provided, use environment-based defaults
	if c.IsProduction() {
		// Production defaults: all safety features enabled
		if !c.hasExplicitSettings() {
			c.Enhanced = true
			c.Online = true
			c.VerifyChecksums = true
			c.DetectDrift = true
			c.Verbose = false // Less verbose in production
			if c.DriftHandling == "" {
				c.DriftHandling = "prompt"
			}
		}
	} else {
		// Development defaults: enhanced features with verbose logging
		if !c.hasExplicitSettings() {
			c.Enhanced = true
			c.Online = false // No need for online migrations in dev
			c.VerifyChecksums = true
			c.DetectDrift = true
			c.Verbose = true
			if c.DriftHandling == "" {
				c.DriftHandling = "prompt"
			}
		}
	}
}

// hasExplicitSettings checks if any migration settings were explicitly configured
func (c *Config) hasExplicitSettings() bool {
	// If any setting was explicitly set in config, don't apply defaults
	// This is a simplified check - in a real scenario, you'd track which were set
	return false // For now, always apply environment-based defaults
}
