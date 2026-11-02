package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

// Config holds application configuration.
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
	Strict          bool // Treat safety warnings as failures

	// Auto-run dependencies before migrations
	AutoRunExtensions bool // Run 'vm extensions migrate' before 'vm migrate'
	AutoRunFunctions  bool // Run 'vm functions migrate' before 'vm migrate'
	AutoRunEnums      bool // Run 'vm enums migrate' before 'vm migrate'

	// Drift handling: auto, reject, prompt
	DriftHandling string

	// Internal: track which fields were explicitly set in the .vm file
	explicitVerbose           bool
	explicitEnhanced          bool
	explicitOnline            bool
	explicitDetectDrift       bool
	explicitVerifyChecksums   bool
	explicitAutoRunExtensions bool
	explicitAutoRunFunctions  bool
	explicitAutoRunEnums      bool
	migrationPathFromVM       bool

	// configDir is the directory containing the .vm file that was loaded.
	configDir string
}

// LoadConfig loads configuration with optional DSN and path overrides.
//
// Precedence (highest first):
//  1. CLI flags
//  2. Real process environment variables (DATABASE_URL, MIGRATION_PATH)
//  3. .vm config file (current directory or any parent)
//  4. .env file (current directory only)
//  5. Built-in defaults
func LoadConfig(dsnOverride, pathOverride string) (*Config, error) {
	cfg := &Config{
		MigrationPath: "./migrations",
		Environment:   "development", // default to development
		DriftHandling: "prompt",      // default to interactive prompt
	}

	// .env has the lowest file precedence. Read it instead of loading it into
	// the process environment: godotenv.Load would make .env override .vm,
	// which is the opposite of what the docs promise.
	if envValues, err := godotenv.Read(".env"); err == nil {
		for key, value := range envValues {
			if _, known := knownKeys[key]; known {
				applyConfigValue(cfg, key, value)
			}
		}
		// .env MIGRATION_PATH is relative to cwd, not to a .vm file.
		cfg.migrationPathFromVM = false
	}

	// .vm overrides .env.
	if err := loadVorzelaConfig(cfg); err != nil {
		return nil, err
	}

	// Real environment variables override file config.
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("MIGRATION_PATH"); v != "" {
		cfg.MigrationPath = v
		cfg.migrationPathFromVM = false
	}

	// CLI overrides win over everything.
	if dsnOverride != "" {
		cfg.DatabaseURL = dsnOverride
	}
	if pathOverride != "" && pathOverride != "./migrations" {
		cfg.MigrationPath = pathOverride
		cfg.migrationPathFromVM = false
	}

	// A MIGRATION_PATH in a parent .vm is relative to that .vm file, not to
	// the directory the agent happened to run from.
	if cfg.migrationPathFromVM && cfg.configDir != "" && !filepath.IsAbs(cfg.MigrationPath) {
		cwd, cwdErr := os.Getwd()
		if cwdErr == nil {
			cfgDirAbs, _ := filepath.Abs(cfg.configDir)
			cwdAbs, _ := filepath.Abs(cwd)
			if cfgDirAbs != "" && cfgDirAbs != cwdAbs {
				cfg.MigrationPath = filepath.Clean(filepath.Join(cfg.configDir, cfg.MigrationPath))
			}
		}
	}

	cfg.ApplyEnvironmentDefaults()
	return cfg, nil
}

// LoadConfigWithOverrides is deprecated, use LoadConfig instead.
func LoadConfigWithOverrides(dsnOverride, envOverride, pathOverride string) (*Config, error) {
	// Ignore envOverride, just delegate to LoadConfig.
	return LoadConfig(dsnOverride, pathOverride)
}

// loadVorzelaConfig loads configuration from .vm, searching the current
// directory and then parents.
func loadVorzelaConfig(cfg *Config) error {
	if err := loadVorzelaFile(".vm", cfg); err == nil {
		return nil
	}

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

// loadVorzelaFile loads a specific .vm file.
func loadVorzelaFile(path string, cfg *Config) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return err
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}
	cfg.configDir = filepath.Dir(path)

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

		if _, known := knownKeys[key]; !known {
			output.Warning(".vm line %d: unknown key '%s' — run 'vm lint' for details", lineNum, key)
			continue
		}
		applyConfigValue(cfg, key, value)
	}

	return nil
}

// applyConfigValue applies one KEY=VALUE pair from .vm/.env. Unknown keys are
// ignored by callers that have already validated them.
func applyConfigValue(cfg *Config, key, value string) {
	parseBool := func(v string) bool {
		return strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
	}

	switch key {
	case "DATABASE_URL":
		cfg.DatabaseURL = value
	case "MIGRATION_PATH":
		cfg.MigrationPath = value
		cfg.migrationPathFromVM = true
	case "SQLC_SUPPORT":
		cfg.SqlcSupport = parseBool(value)
	case "ENVIRONMENT", "ENV":
		cfg.Environment = strings.ToLower(value)
	case "ENHANCED":
		cfg.Enhanced = parseBool(value)
		cfg.explicitEnhanced = true
	case "ONLINE":
		cfg.Online = parseBool(value)
		cfg.explicitOnline = true
	case "VERIFY_CHECKSUMS":
		cfg.VerifyChecksums = parseBool(value)
		cfg.explicitVerifyChecksums = true
	case "DETECT_DRIFT":
		cfg.DetectDrift = parseBool(value)
		cfg.explicitDetectDrift = true
	case "VERBOSE":
		cfg.Verbose = parseBool(value)
		cfg.explicitVerbose = true
	case "STRICT":
		cfg.Strict = parseBool(value)
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
		val := strings.ToLower(strings.TrimSpace(value))
		if val == "auto" || val == "reject" || val == "prompt" {
			cfg.DriftHandling = val
		}
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("database URL is required. Set DATABASE_URL env var, create .vm config file, or use --dsn flag")
	}
	if strings.TrimSpace(c.MigrationPath) == "" {
		return fmt.Errorf("MIGRATION_PATH is empty")
	}
	switch c.DriftHandling {
	case "", "auto", "prompt", "reject":
	default:
		return fmt.Errorf("invalid DRIFT_HANDLING %q (expected auto, prompt, or reject)", c.DriftHandling)
	}

	c.NormalizeEnvironment()
	return nil
}

// NormalizeEnvironment normalizes environment names.
func (c *Config) NormalizeEnvironment() {
	env := strings.ToLower(c.Environment)

	if env == "dev" || env == "develop" || env == "development" {
		c.Environment = "development"
		return
	}
	if env == "prod" || env == "production" {
		c.Environment = "production"
		return
	}
	c.Environment = "development"
}

// IsProduction returns true if running in production environment.
func (c *Config) IsProduction() bool { return c.Environment == "production" }

// IsDevelopment returns true if running in development environment.
func (c *Config) IsDevelopment() bool { return c.Environment == "development" }

// ApplyEnvironmentDefaults applies defaults based on environment. Explicit
// .vm values always win; this fixes the previous behaviour where ONLINE=true
// in development was silently overwritten by the development default.
func (c *Config) ApplyEnvironmentDefaults() {
	c.NormalizeEnvironment()

	var (
		defEnhanced bool
		defOnline   bool
		defVerify   bool
		defDrift    bool
		defVerbose  bool
	)
	if c.IsProduction() {
		defEnhanced, defOnline, defVerify, defDrift, defVerbose = true, true, true, true, false
	} else {
		defEnhanced, defOnline, defVerify, defDrift, defVerbose = true, false, true, true, true
	}

	if !c.explicitEnhanced {
		c.Enhanced = defEnhanced
	}
	if !c.explicitOnline {
		c.Online = defOnline
	}
	if !c.explicitVerifyChecksums {
		c.VerifyChecksums = defVerify
	}
	if !c.explicitDetectDrift {
		c.DetectDrift = defDrift
	}
	if !c.explicitVerbose {
		c.Verbose = defVerbose
	}
	if c.DriftHandling == "" {
		c.DriftHandling = "prompt"
	}

	// Auto-run defaults (both environments) — only when not explicitly set.
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
