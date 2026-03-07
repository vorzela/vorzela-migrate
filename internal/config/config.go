package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/vorzela/vorzela-migrate/internal/output"
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

	// Auto-run dependencies before migrations
	AutoRunExtensions bool // Run 'vm extensions migrate' before 'vm migrate'
	AutoRunFunctions  bool // Run 'vm functions migrate' before 'vm migrate'
	AutoRunEnums      bool // Run 'vm enums migrate' before 'vm migrate'

	// Drift handling: auto, reject, prompt
	DriftHandling string

	// Internal: track which fields were explicitly set in the .vm file
	explicitVerbose           bool
	explicitEnhanced          bool
	explicitDetectDrift       bool
	explicitVerifyChecksums   bool
	explicitAutoRunExtensions bool
	explicitAutoRunFunctions  bool
	explicitAutoRunEnums      bool
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
	for lineNum, line := range lines {
		lineNum++ // 1-based
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		// Strip inline # comments from values: `true   # comment` → `true`
		rawValue := parts[1]
		if idx := strings.Index(rawValue, " #"); idx != -1 {
			rawValue = rawValue[:idx]
		}
		value := strings.TrimSpace(rawValue)

		parseBool := func(v string) bool {
			return strings.ToLower(v) == "true" || v == "1"
		}

		// Warn about unknown keys so users catch typos without running vm lint.
		if _, known := knownKeys[key]; !known {
			output.Warning(".vm line %d: unknown key '%s' — run 'vm lint' for details", lineNum, key)
			continue
		}

		switch key {
		case "DATABASE_URL":
			cfg.DatabaseURL = value
		case "MIGRATION_PATH":
			cfg.MigrationPath = value
		case "SQLC_SUPPORT":
			cfg.SqlcSupport = parseBool(value)
		case "ENVIRONMENT", "ENV":
			cfg.Environment = strings.ToLower(value)
		case "ENHANCED":
			cfg.Enhanced = parseBool(value)
			cfg.explicitEnhanced = true
		case "ONLINE":
			cfg.Online = parseBool(value)
		case "VERIFY_CHECKSUMS":
			cfg.VerifyChecksums = parseBool(value)
			cfg.explicitVerifyChecksums = true
		case "DETECT_DRIFT":
			cfg.DetectDrift = parseBool(value)
			cfg.explicitDetectDrift = true
		case "VERBOSE":
			cfg.Verbose = parseBool(value)
			cfg.explicitVerbose = true
		case "AUTO_RUN_EXTENSIONS":
			cfg.AutoRunExtensions = parseBool(value)
			cfg.explicitAutoRunExtensions = true
		case "AUTO_RUN_FUNCTIONS":
			cfg.AutoRunFunctions = parseBool(value)
			cfg.explicitAutoRunFunctions = true
		case "AUTO_RUN_ENUMS":
			cfg.AutoRunEnums = parseBool(value)
			cfg.explicitAutoRunEnums = true
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
			// Only set Verbose to true if not explicitly set by user
			if !c.explicitVerbose {
				c.Verbose = true
			}
			if c.DriftHandling == "" {
				c.DriftHandling = "prompt"
			}
		}
	}

	// Set auto-run defaults (both environments) — only when not explicitly set by user.
	if !c.explicitAutoRunExtensions {
		c.AutoRunExtensions = true
	}
	if !c.explicitAutoRunFunctions {
		c.AutoRunFunctions = true
	}
	if !c.explicitAutoRunEnums {
		c.AutoRunEnums = true
	}
}

// hasExplicitAutoRunSettings returns true if the user explicitly configured any
// AUTO_RUN_* key in their .vm file so that ApplyEnvironmentDefaults doesn't
// overwrite the user's intent.
func (c *Config) hasExplicitAutoRunSettings() bool {
	return c.explicitAutoRunExtensions || c.explicitAutoRunFunctions || c.explicitAutoRunEnums
}

// hasExplicitSettings returns true if the user explicitly set any migration
// strategy key (ENHANCED, DETECT_DRIFT, VERIFY_CHECKSUMS) in their .vm file.
// When true, ApplyEnvironmentDefaults will not override those values.
func (c *Config) hasExplicitSettings() bool {
	return c.explicitEnhanced || c.explicitDetectDrift || c.explicitVerifyChecksums
}
