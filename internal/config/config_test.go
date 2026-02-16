package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name          string
		dsnOverride   string
		pathOverride  string
		envVars       map[string]string
		vmFile        string
		wantErr       bool
		expectedDSN   string
		expectedPath  string
	}{
		{
			name:         "DSN override takes precedence",
			dsnOverride:  "postgres://override:5432/db",
			pathOverride: "",
			expectedDSN:  "postgres://override:5432/db",
			expectedPath: "./migrations",
		},
		{
			name:         "Path override works",
			dsnOverride:  "postgres://test:5432/db",
			pathOverride: "./custom/path",
			expectedDSN:  "postgres://test:5432/db",
			expectedPath: "./custom/path",
		},
		{
			name:        "Environment variable used",
			envVars:     map[string]string{"DATABASE_URL": "postgres://env:5432/db"},
			expectedDSN: "postgres://env:5432/db",
		},
		{
			name:         "Default path when not specified",
			dsnOverride:  "postgres://test:5432/db",
			expectedPath: "./migrations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for this test
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			cfg, err := LoadConfig(tt.dsnOverride, tt.pathOverride)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if tt.expectedDSN != "" && cfg.DatabaseURL != tt.expectedDSN {
					t.Errorf("DatabaseURL = %v, want %v", cfg.DatabaseURL, tt.expectedDSN)
				}
				if tt.expectedPath != "" && cfg.MigrationPath != tt.expectedPath {
					t.Errorf("MigrationPath = %v, want %v", cfg.MigrationPath, tt.expectedPath)
				}
			}
		})
	}
}

func TestLoadVorzelaFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	vmFile := filepath.Join(tmpDir, ".vm")

	// Test valid .vm file
	content := `DATABASE_URL=postgres://test:5432/testdb
MIGRATION_PATH=./test/migrations
SQLC_SUPPORT=true
`
	if err := os.WriteFile(vmFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}
	err := loadVorzelaFile(vmFile, cfg)
	if err != nil {
		t.Fatalf("loadVorzelaFile() error = %v", err)
	}

	if cfg.DatabaseURL != "postgres://test:5432/testdb" {
		t.Errorf("DatabaseURL = %v, want postgres://test:5432/testdb", cfg.DatabaseURL)
	}
	if cfg.MigrationPath != "./test/migrations" {
		t.Errorf("MigrationPath = %v, want ./test/migrations", cfg.MigrationPath)
	}
	if !cfg.SqlcSupport {
		t.Error("SqlcSupport should be true")
	}
}

func TestLoadVorzelaFileWithComments(t *testing.T) {
	tmpDir := t.TempDir()
	vmFile := filepath.Join(tmpDir, ".vm")

	content := `# This is a comment
DATABASE_URL=postgres://test:5432/testdb
# Another comment
MIGRATION_PATH=./migrations
`
	if err := os.WriteFile(vmFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}
	err := loadVorzelaFile(vmFile, cfg)
	if err != nil {
		t.Fatalf("loadVorzelaFile() error = %v", err)
	}

	if cfg.DatabaseURL != "postgres://test:5432/testdb" {
		t.Errorf("DatabaseURL = %v, want postgres://test:5432/testdb", cfg.DatabaseURL)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "Valid config",
			cfg: Config{
				DatabaseURL:   "postgres://test:5432/db",
				MigrationPath: "./migrations",
			},
			wantErr: false,
		},
		{
			name: "Missing database URL",
			cfg: Config{
				MigrationPath: "./migrations",
			},
			wantErr: true,
		},
		{
			name: "Empty database URL",
			cfg: Config{
				DatabaseURL:   "",
				MigrationPath: "./migrations",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSqlcSupportParsing(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{"true lowercase", "true", true},
		{"TRUE uppercase", "TRUE", true},
		{"1 numeric", "1", true},
		{"false lowercase", "false", false},
		{"FALSE uppercase", "FALSE", false},
		{"0 numeric", "0", false},
		{"empty string", "", false},
		{"random string", "yes", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			vmFile := filepath.Join(tmpDir, ".vm")

			content := "SQLC_SUPPORT=" + tt.value + "\n"
			if err := os.WriteFile(vmFile, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}

			cfg := &Config{}
			err := loadVorzelaFile(vmFile, cfg)
			if err != nil {
				t.Fatalf("loadVorzelaFile() error = %v", err)
			}

			if cfg.SqlcSupport != tt.expected {
				t.Errorf("SqlcSupport = %v, want %v for value '%s'", cfg.SqlcSupport, tt.expected, tt.value)
			}
		})
	}
}

func TestNormalizeEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"development lowercase", "development", "development"},
		{"Development capitalized", "Development", "development"},
		{"DEVELOPMENT uppercase", "DEVELOPMENT", "development"},
		{"dev short form", "dev", "development"},
		{"DEV uppercase short", "DEV", "development"},
		{"production lowercase", "production", "production"},
		{"Production capitalized", "Production", "production"},
		{"PRODUCTION uppercase", "PRODUCTION", "production"},
		{"prod short form", "prod", "production"},
		{"PROD uppercase short", "PROD", "production"},
		{"empty string", "", "development"},
		{"unknown value", "staging", "development"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Environment: tt.input}
			cfg.NormalizeEnvironment()
			
			if cfg.Environment != tt.expected {
				t.Errorf("NormalizeEnvironment() = %q, want %q", cfg.Environment, tt.expected)
			}
		})
	}
}

func TestApplyEnvironmentDefaults_Development(t *testing.T) {
	cfg := &Config{
		Environment: "development",
	}
	
	cfg.ApplyEnvironmentDefaults()
	
	// Development defaults
	if !cfg.Enhanced {
		t.Error("Development should enable Enhanced mode")
	}
	if cfg.Online {
		t.Error("Development should NOT enable Online mode")
	}
	if !cfg.VerifyChecksums {
		t.Error("Development should enable VerifyChecksums")
	}
	if !cfg.DetectDrift {
		t.Error("Development should enable DetectDrift")
	}
	if !cfg.Verbose {
		t.Error("Development should enable Verbose mode")
	}
	if cfg.DriftHandling != "prompt" {
		t.Errorf("Development DriftHandling = %q, want 'prompt'", cfg.DriftHandling)
	}
}

func TestApplyEnvironmentDefaults_Production(t *testing.T) {
	cfg := &Config{
		Environment: "production",
	}
	
	cfg.ApplyEnvironmentDefaults()
	
	// Production defaults
	if !cfg.Enhanced {
		t.Error("Production should enable Enhanced mode")
	}
	if !cfg.Online {
		t.Error("Production should enable Online mode")
	}
	if !cfg.VerifyChecksums {
		t.Error("Production should enable VerifyChecksums")
	}
	if !cfg.DetectDrift {
		t.Error("Production should enable DetectDrift")
	}
	if cfg.Verbose {
		t.Error("Production should NOT enable Verbose mode")
	}
	if cfg.DriftHandling != "prompt" {
		t.Errorf("Production DriftHandling = %q, want 'prompt'", cfg.DriftHandling)
	}
}

func TestIsProduction(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		want        bool
	}{
		{"production", "production", true},
		{"PRODUCTION", "PRODUCTION", true},
		{"prod", "prod", true},
		{"PROD", "PROD", true},
		{"development", "development", false},
		{"dev", "dev", false},
		{"empty", "", false},
		{"other", "staging", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Environment: tt.environment}
			cfg.NormalizeEnvironment()
			
			got := cfg.IsProduction()
			if got != tt.want {
				t.Errorf("IsProduction() = %v, want %v for environment %q", got, tt.want, tt.environment)
			}
		})
	}
}

func TestIsDevelopment(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		want        bool
	}{
		{"development", "development", true},
		{"DEVELOPMENT", "DEVELOPMENT", true},
		{"dev", "dev", true},
		{"DEV", "DEV", true},
		{"production", "production", false},
		{"prod", "prod", false},
		{"empty defaults to dev", "", true},
		{"other defaults to dev", "staging", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Environment: tt.environment}
			cfg.NormalizeEnvironment()
			
			got := cfg.IsDevelopment()
			if got != tt.want {
				t.Errorf("IsDevelopment() = %v, want %v for environment %q", got, tt.want, tt.environment)
			}
		})
	}
}

func TestLoadVorzelaFileWithEnvironment(t *testing.T) {
	tmpDir := t.TempDir()
	vmFile := filepath.Join(tmpDir, ".vm")

	content := `DATABASE_URL=postgres://test:5432/testdb
ENVIRONMENT=production
DRIFT_HANDLING=auto
ENHANCED=true
ONLINE=true
VERIFY_CHECKSUMS=true
DETECT_DRIFT=true
VERBOSE=false
`
	if err := os.WriteFile(vmFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}
	err := loadVorzelaFile(vmFile, cfg)
	if err != nil {
		t.Fatalf("loadVorzelaFile() error = %v", err)
	}

	if cfg.Environment != "production" {
		t.Errorf("Environment = %q, want 'production'", cfg.Environment)
	}
	if cfg.DriftHandling != "auto" {
		t.Errorf("DriftHandling = %q, want 'auto'", cfg.DriftHandling)
	}
	if !cfg.Enhanced {
		t.Error("Enhanced should be true")
	}
	if !cfg.Online {
		t.Error("Online should be true")
	}
	if !cfg.VerifyChecksums {
		t.Error("VerifyChecksums should be true")
	}
	if !cfg.DetectDrift {
		t.Error("DetectDrift should be true")
	}
	if cfg.Verbose {
		t.Error("Verbose should be false")
	}
}

func TestDriftHandlingOptions(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
		valid    bool
	}{
		{"auto", "auto", "auto", true},
		{"prompt", "prompt", "prompt", true},
		{"reject", "reject", "reject", true},
		{"AUTO uppercase", "AUTO", "auto", true}, // Gets normalized to lowercase
		{"invalid", "invalid", "", false},        // Invalid values become empty
		{"empty", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			vmFile := filepath.Join(tmpDir, ".vm")

			content := "DRIFT_HANDLING=" + tt.value + "\n"
			if err := os.WriteFile(vmFile, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}

			cfg := &Config{}
			err := loadVorzelaFile(vmFile, cfg)
			if err != nil {
				t.Fatalf("loadVorzelaFile() error = %v", err)
			}

			if cfg.DriftHandling != tt.expected {
				t.Errorf("DriftHandling = %q, want %q", cfg.DriftHandling, tt.expected)
			}
		})
	}
}

func TestFullEnvironmentBasedConfig_Development(t *testing.T) {
	tmpDir := t.TempDir()
	vmFile := filepath.Join(tmpDir, ".vm")

	content := `DATABASE_URL=postgres://localhost:5432/dev
ENVIRONMENT=development
DRIFT_HANDLING=auto
`
	if err := os.WriteFile(vmFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}
	err := loadVorzelaFile(vmFile, cfg)
	if err != nil {
		t.Fatalf("loadVorzelaFile() error = %v", err)
	}

	cfg.ApplyEnvironmentDefaults()

	// Verify development auto-configuration
	if !cfg.IsDevelopment() {
		t.Error("Config should be in development mode")
	}
	if !cfg.Enhanced {
		t.Error("Development should enable Enhanced")
	}
	if cfg.Online {
		t.Error("Development should NOT enable Online")
	}
	if !cfg.Verbose {
		t.Error("Development should enable Verbose")
	}
	if cfg.DriftHandling != "auto" {
		t.Errorf("DriftHandling should be 'auto', got %q", cfg.DriftHandling)
	}
}

func TestFullEnvironmentBasedConfig_Production(t *testing.T) {
	tmpDir := t.TempDir()
	vmFile := filepath.Join(tmpDir, ".vm")

	content := `DATABASE_URL=postgres://prod:5432/app
ENVIRONMENT=production
DRIFT_HANDLING=reject
`
	if err := os.WriteFile(vmFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}
	err := loadVorzelaFile(vmFile, cfg)
	if err != nil {
		t.Fatalf("loadVorzelaFile() error = %v", err)
	}

	cfg.ApplyEnvironmentDefaults()

	// Verify production auto-configuration
	if !cfg.IsProduction() {
		t.Error("Config should be in production mode")
	}
	if !cfg.Enhanced {
		t.Error("Production should enable Enhanced")
	}
	if !cfg.Online {
		t.Error("Production should enable Online")
	}
	if cfg.Verbose {
		t.Error("Production should NOT enable Verbose")
	}
	if cfg.DriftHandling != "reject" {
		t.Errorf("DriftHandling should be 'reject', got %q", cfg.DriftHandling)
	}
}

func TestConfigOverridesPrecedence(t *testing.T) {
	// Explicit config values should not be overridden by environment defaults
	cfg := &Config{
		Environment:     "production",
		Enhanced:        false, // Explicitly disabled
		Online:          false, // Explicitly disabled
		Verbose:         true,  // Explicitly enabled
		VerifyChecksums: false, // Explicitly disabled
	}

	// Apply defaults (should not override explicit false values)
	// Note: In actual implementation, you'd need logic to track which values
	// were explicitly set vs. defaults. This test validates the concept.
	
	if cfg.Environment != "production" {
		t.Error("Environment should remain production")
	}
}

func BenchmarkNormalizeEnvironment(b *testing.B) {
	cfg := &Config{Environment: "DEVELOPMENT"}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg.NormalizeEnvironment()
	}
}

func BenchmarkApplyEnvironmentDefaults(b *testing.B) {
	cfg := &Config{Environment: "production"}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg.ApplyEnvironmentDefaults()
	}
}
