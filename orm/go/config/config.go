// Package config loads `.vorm` project settings (KEY=value, same spirit as `.vm`).
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	// DefaultFile is the project config filename.
	DefaultFile = ".vorm"

	DefaultPackage     = "gen"
	DefaultOutDir      = "./vorm/gen"
	DefaultQueryDir    = "./queries"
	DefaultModelDir    = "./models"
	DefaultSchemaDir   = "./schema/migrations"
	DefaultModelPkg    = "models"
	DefaultDriver      = "pgx"
	DefaultDialect     = "postgres"
)

// Config is vorm project configuration.
type Config struct {
	// Package is the Go package name for vorm/gen output (default "gen").
	// Change to avoid conflicts with another package named gen.
	Package string

	// OutDir is where queries_gen.go is written (default ./vorm/<Package>).
	OutDir string

	QueryDir     string
	ModelDir     string
	SchemaDir    string
	ModelImport  string // e.g. myapp/models; empty = auto from go.mod
	ModelPackage string // models package name (default models)

	Driver  string // pgx | pq
	Dialect string // postgres | mysql | mariadb

	Path string // path config was loaded from (empty if defaults only)

	outDirSet bool // true if OUT_DIR was explicitly set in file / Set
}

// Default returns built-in defaults.
func Default() *Config {
	return &Config{
		Package:      DefaultPackage,
		OutDir:       DefaultOutDir,
		QueryDir:     DefaultQueryDir,
		ModelDir:     DefaultModelDir,
		SchemaDir:    DefaultSchemaDir,
		ModelPackage: DefaultModelPkg,
		Driver:       DefaultDriver,
		Dialect:      DefaultDialect,
	}
}

// Load reads `.vorm` from dir (or cwd). Missing file → defaults (no error).
func Load(dir string) (*Config, error) {
	if dir == "" {
		dir = "."
	}
	path := filepath.Join(dir, DefaultFile)
	cfg := Default()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	defer f.Close()
	cfg.Path = path

	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: want KEY=value", path, lineNo)
		}
		key = strings.TrimSpace(strings.ToUpper(key))
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if err := cfg.set(key, val); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	cfg.applyDerived()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadOrDefault is Load that never fails on missing file.
func LoadOrDefault(dir string) *Config {
	cfg, err := Load(dir)
	if err != nil {
		d := Default()
		return d
	}
	return cfg
}

func (c *Config) set(key, val string) error {
	switch key {
	case "PACKAGE", "GEN_PACKAGE", "PACKAGE_NAME":
		c.Package = val
	case "OUT_DIR", "GEN_DIR":
		c.OutDir = val
		c.outDirSet = true
	case "QUERY_DIR", "QUERIES_DIR":
		c.QueryDir = val
	case "MODEL_DIR", "MODELS_DIR":
		c.ModelDir = val
	case "SCHEMA_DIR":
		c.SchemaDir = val
	case "MODEL_IMPORT":
		c.ModelImport = val
	case "MODEL_PACKAGE":
		c.ModelPackage = val
	case "DRIVER":
		c.Driver = strings.ToLower(val)
	case "DIALECT":
		c.Dialect = strings.ToLower(val)
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	return nil
}

func (c *Config) applyDerived() {
	if c.Package == "" {
		c.Package = DefaultPackage
	}
	if !c.outDirSet || c.OutDir == "" {
		c.OutDir = "./vorm/" + c.Package
	}
	if c.Driver == "" {
		c.Driver = DefaultDriver
	}
	if c.Dialect == "" {
		c.Dialect = DefaultDialect
	}
	if c.QueryDir == "" {
		c.QueryDir = DefaultQueryDir
	}
	if c.ModelDir == "" {
		c.ModelDir = DefaultModelDir
	}
	if c.SchemaDir == "" {
		c.SchemaDir = DefaultSchemaDir
	}
	if c.ModelPackage == "" {
		c.ModelPackage = DefaultModelPkg
	}
}

// Validate checks package names and known enums.
func (c *Config) Validate() error {
	c.applyDerived()
	if err := validGoPackage(c.Package); err != nil {
		return fmt.Errorf("PACKAGE: %w", err)
	}
	if err := validGoPackage(c.ModelPackage); err != nil {
		return fmt.Errorf("MODEL_PACKAGE: %w", err)
	}
	switch c.Driver {
	case "pgx", "pq":
	default:
		return fmt.Errorf("DRIVER: want pgx or pq, got %q", c.Driver)
	}
	switch c.Dialect {
	case "postgres", "postgresql", "mysql", "mariadb":
	default:
		return fmt.Errorf("DIALECT: want postgres|mysql|mariadb, got %q", c.Dialect)
	}
	return nil
}

func validGoPackage(name string) error {
	if name == "" {
		return fmt.Errorf("empty")
	}
	for i, r := range name {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return fmt.Errorf("%q is not a valid Go package name", name)
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return fmt.Errorf("%q is not a valid Go package name", name)
		}
	}
	// reserved-ish conflicts users often hit
	switch name {
	case "main", "test":
		return fmt.Errorf("%q is not allowed as generated package", name)
	}
	return nil
}

// Write writes config to path (default ./.vorm).
func (c *Config) Write(path string) error {
	if path == "" {
		path = DefaultFile
	}
	c.applyDerived()
	if err := c.Validate(); err != nil {
		return err
	}
	body := Format(c)
	return os.WriteFile(path, []byte(body), 0o644)
}

// Format renders KEY=value with comments.
func Format(c *Config) string {
	c = clone(c)
	c.applyDerived()
	var b strings.Builder
	b.WriteString("# vorm project config — edit or: vorm config set KEY=value\n")
	b.WriteString("# Docs: orm/go/README.md\n\n")
	b.WriteString("# Generated Go package name (default gen). Change if gen conflicts.\n")
	fmt.Fprintf(&b, "PACKAGE=%s\n\n", c.Package)
	b.WriteString("# Output directory for queries_gen.go\n")
	fmt.Fprintf(&b, "OUT_DIR=%s\n\n", c.OutDir)
	b.WriteString("# Postgres client: pgx (default) | pq\n")
	fmt.Fprintf(&b, "DRIVER=%s\n\n", c.Driver)
	b.WriteString("# SQL dialect: postgres | mysql | mariadb\n")
	fmt.Fprintf(&b, "DIALECT=%s\n\n", c.Dialect)
	fmt.Fprintf(&b, "QUERY_DIR=%s\n", c.QueryDir)
	fmt.Fprintf(&b, "MODEL_DIR=%s\n", c.ModelDir)
	fmt.Fprintf(&b, "SCHEMA_DIR=%s\n", c.SchemaDir)
	fmt.Fprintf(&b, "MODEL_PACKAGE=%s\n", c.ModelPackage)
	if c.ModelImport != "" {
		fmt.Fprintf(&b, "MODEL_IMPORT=%s\n", c.ModelImport)
	} else {
		b.WriteString("# MODEL_IMPORT= # empty = <module>/models from go.mod\n")
	}
	return b.String()
}

func clone(c *Config) *Config {
	if c == nil {
		return Default()
	}
	cp := *c
	return &cp
}

// Set updates one key (uppercase) and returns the new value.
func (c *Config) Set(key, val string) error {
	key = strings.TrimSpace(strings.ToUpper(key))
	val = strings.TrimSpace(val)
	if err := c.set(key, val); err != nil {
		return err
	}
	c.applyDerived()
	return c.Validate()
}

// Get returns the string value for a key.
func (c *Config) Get(key string) (string, error) {
	key = strings.TrimSpace(strings.ToUpper(key))
	c.applyDerived()
	switch key {
	case "PACKAGE", "GEN_PACKAGE", "PACKAGE_NAME":
		return c.Package, nil
	case "OUT_DIR", "GEN_DIR":
		return c.OutDir, nil
	case "QUERY_DIR", "QUERIES_DIR":
		return c.QueryDir, nil
	case "MODEL_DIR", "MODELS_DIR":
		return c.ModelDir, nil
	case "SCHEMA_DIR":
		return c.SchemaDir, nil
	case "MODEL_IMPORT":
		return c.ModelImport, nil
	case "MODEL_PACKAGE":
		return c.ModelPackage, nil
	case "DRIVER":
		return c.Driver, nil
	case "DIALECT":
		return c.Dialect, nil
	default:
		return "", fmt.Errorf("unknown key %q", key)
	}
}

// Keys lists configurable keys.
func Keys() []string {
	return []string{
		"PACKAGE", "OUT_DIR", "DRIVER", "DIALECT",
		"QUERY_DIR", "MODEL_DIR", "SCHEMA_DIR", "MODEL_PACKAGE", "MODEL_IMPORT",
	}
}
