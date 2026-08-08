package generate

import (
	"os"
	"path/filepath"
	"strings"
)

// Options controls code generation (vorm/gen only — no sqlc).
type Options struct {
	QueryDir     string // default ./queries
	OutDir       string // default ./vorm/gen
	ModelDir     string // default ./models — for column checks + Scan
	ModelImport  string // import path for models package (e.g. myapp/models)
	Package      string // generated Go package name (default gen)
	ModelPackage string // models package name used in types (default models)
	Dialect      string // postgres|mysql|mariadb
	Driver       string // pgx (default) | pq — documented in generated header
}

// Result summarizes generation.
type Result struct {
	Queries   int
	GoFiles   []string
	StubsSeen []string
	Dialect   string
	Driver    string
}

// Run scans // vorm:query stubs, lowers fluent chains to parameterized SQL,
// and emits typed Go under vorm/gen (return models, never SELECT *).
func Run(opts *Options) (*Result, error) {
	if opts == nil {
		opts = &Options{}
	}
	if opts.QueryDir == "" {
		opts.QueryDir = DefaultQueryDir
	}
	if opts.OutDir == "" {
		opts.OutDir = DefaultOutDir
	}
	if opts.ModelDir == "" {
		opts.ModelDir = DefaultModelDir
	}
	if opts.Dialect == "" {
		opts.Dialect = "postgres"
	}
	if opts.Driver == "" {
		opts.Driver = "pgx"
	}
	if opts.Package == "" {
		opts.Package = "gen"
	}
	if opts.ModelPackage == "" {
		opts.ModelPackage = "models"
	}
	if opts.ModelImport == "" {
		opts.ModelImport = detectModelImport()
	}
	dialect := strings.ToLower(opts.Dialect)
	driver := strings.ToLower(opts.Driver)

	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return nil, err
	}

	models, err := parseModelsDir(opts.ModelDir)
	if err != nil {
		return nil, err
	}

	stubs, err := findAnnotatedStubs(opts.QueryDir, models)
	if err != nil {
		return nil, err
	}

	res := &Result{
		Queries:   len(stubs),
		StubsSeen: stubNames(stubs),
		Dialect:   dialect,
		Driver:    driver,
	}
	if len(stubs) == 0 {
		return res, nil
	}

	body, err := emitQueries(opts, stubs, models)
	if err != nil {
		return nil, err
	}
	var scanners strings.Builder
	emitScanners(&scanners, stubs, models, opts.ModelPackage)
	body += scanners.String()

	goFile := filepath.Join(opts.OutDir, "queries_gen.go")
	if err := os.WriteFile(goFile, []byte(body), 0o644); err != nil {
		return nil, err
	}
	res.GoFiles = append(res.GoFiles, goFile)
	return res, nil
}

func stubNames(stubs []StubFunc) []string {
	out := make([]string, len(stubs))
	for i, s := range stubs {
		out[i] = s.Name
	}
	return out
}

func parseQueryNameLine(line string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "vorm:query"))
	for _, part := range strings.Fields(rest) {
		if after, ok := strings.CutPrefix(part, "name="); ok {
			return after
		}
	}
	return ""
}