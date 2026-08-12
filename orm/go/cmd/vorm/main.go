package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vorzela/vorm/config"
	"github.com/vorzela/vorm/generate"
	"github.com/vorzela/vorm/lint"
	"github.com/vorzela/vorm/query"
	"github.com/vorzela/vorm/scaffold"
	"github.com/vorzela/vorm/schema"
	"github.com/vorzela/vorm/vmtool"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "init":
		if err := cmdInit(args); err != nil {
			fatal(err)
		}
	case "config":
		if err := cmdConfig(args); err != nil {
			fatal(err)
		}
	case "make":
		if err := cmdMake(args); err != nil {
			fatal(err)
		}
	case "lint":
		path := "./migrations"
		if len(args) > 0 {
			path = args[0]
		}
		res, err := lint.Dir(path)
		if err != nil {
			fatal(err)
		}
		fmt.Print(lint.Format(res))
		if res.HasErrors() {
			os.Exit(1)
		}
	case "migrate", "rollback", "status", "fresh", "refresh":
		args = lintBeforeMigrate(cmd, args)
		if err := cmdMigrate(cmd, args); err != nil {
			fatal(err)
		}
	case "extensions", "enums", "functions":
		if err := cmdPrereq(cmd, args); err != nil {
			fatal(err)
		}
	case "introspect", "schema":
		if err := cmdIntrospect(args); err != nil {
			fatal(err)
		}
	case "generate":
		if err := cmdGenerate(args); err != nil {
			fatal(err)
		}
	case "ensure-vm":
		p, err := vmtool.Ensure(true)
		if err != nil {
			fatal(err)
		}
		fmt.Println(p)
	case "version", "--version":
		fmt.Println("vorm 0.1.0-dev (Vorzela v3)")
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
}

func cmdInit(args []string) error {
	force := false
	for _, a := range args {
		if a == "--force" || a == "-f" {
			force = true
		}
	}
	path := config.DefaultFile
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("%s already exists (use vorm init --force to overwrite)", path)
	}
	cfg := config.Default()
	// A live DATABASE_URL is the most reliable dialect signal; fall back to
	// whatever the vm config says.
	if url := os.Getenv("DATABASE_URL"); url != "" {
		cfg.Dialect = string(query.DetectDialect(url))
	} else if d := vmtool.DetectDialect(); d != "" {
		cfg.Dialect = d
	}
	if err := cfg.Write(path); err != nil {
		return err
	}
	fmt.Printf("wrote %s (PACKAGE=%s, DIALECT=%s, RUNNER=%s)\n", path, cfg.Package, cfg.Dialect, cfg.Runner)
	fmt.Println("next: export DATABASE_URL=… && vorm migrate && vorm generate")
	return nil
}

func cmdConfig(args []string) error {
	if len(args) == 0 {
		cfg, err := config.Load(".")
		if err != nil {
			return err
		}
		src := "(defaults)"
		if cfg.Path != "" {
			src = cfg.Path
		}
		fmt.Printf("# effective config from %s\n", src)
		fmt.Print(config.Format(cfg))
		return nil
	}
	switch args[0] {
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("usage: vorm config get KEY")
		}
		cfg, err := config.Load(".")
		if err != nil {
			return err
		}
		v, err := cfg.Get(args[1])
		if err != nil {
			return err
		}
		fmt.Println(v)
		return nil
	case "set":
		if len(args) < 2 {
			return fmt.Errorf("usage: vorm config set KEY=value")
		}
		cfg, err := config.Load(".")
		if err != nil {
			return err
		}
		kv := args[1]
		key, val, ok := strings.Cut(kv, "=")
		if !ok {
			if len(args) < 3 {
				return fmt.Errorf("usage: vorm config set KEY=value")
			}
			key, val = args[1], args[2]
		}
		if err := cfg.Set(key, val); err != nil {
			return err
		}
		path := config.DefaultFile
		if cfg.Path != "" {
			path = cfg.Path
		}
		if err := cfg.Write(path); err != nil {
			return err
		}
		fmt.Printf("set %s=%s → %s\n", strings.ToUpper(key), val, path)
		return nil
	case "keys":
		for _, k := range config.Keys() {
			fmt.Println(k)
		}
		return nil
	case "lint":
		path := config.DefaultFile
		if len(args) > 1 {
			path = args[1]
		}
		fs, err := config.LintPath(path)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("vorm config: no .vorm (ok — defaults apply); run vorm init to create one")
				return nil
			}
			return err
		}
		fmt.Print(config.FormatFindings(fs))
		if config.HasErrors(fs) {
			return fmt.Errorf("vorm config lint failed")
		}
		return nil
	default:
		return fmt.Errorf("usage: vorm config | get KEY | set KEY=value | keys | lint [.vorm]")
	}
}

func cmdMake(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vorm make migration <posts|post_user|…>")
	}
	switch args[0] {
	case "migration":
		if len(args) < 2 {
			return fmt.Errorf("usage: vorm make migration posts | vorm make migration post_user")
		}
		cfg, err := config.Load(".")
		if err != nil {
			return err
		}
		res, err := scaffold.MakeMigration(args[1], scaffold.MigrationDirs{
			SchemaDir: cfg.SchemaDir,
			ModelDir:  cfg.ModelDir,
			QueryDir:  cfg.QueryDir,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "vorm: wrote %s\n", res.MigrationFile)
		fmt.Fprintf(os.Stderr, "vorm: wrote %s\n", res.ModelFile)
		fmt.Fprintf(os.Stderr, "vorm: wrote %s\n", res.QueryFile)
		fmt.Fprintf(os.Stderr, "vorm: next → edit Blueprint in %s → call %s(nil) → vorm generate models && vorm generate\n",
			res.MigrationFile, res.FuncName)
		return nil
	case "enum":
		if len(args) < 3 {
			return fmt.Errorf("usage: vorm make enum <type_name> value1,value2,...")
		}
		return makeFacade().CreateEnum(args[1], splitCSV(args[2])...)
	case "extension":
		if len(args) < 2 {
			return fmt.Errorf("usage: vorm make extension <name>")
		}
		return makeFacade().CreateExtension(args[1])
	default:
		return fmt.Errorf("unknown make target %q — try: vorm make migration posts", args[0])
	}
}

// resolveDialect prefers the project config, then the connection string, and
// only then a neighbouring .vm file. It never needs the vm binary.
func resolveDialect(cfg *config.Config) string {
	if cfg != nil {
		if cfg.Dialect != "" {
			return cfg.Dialect
		}
		if url := cfg.ResolveDatabaseURL(); url != "" {
			return string(query.DetectDialect(url))
		}
	}
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return string(query.DetectDialect(url))
	}
	return vmtool.DetectDialect()
}

// makeFacade builds the schema writer used by `vorm make enum|extension`. It
// only writes migration files; nothing here runs SQL or shells out.
func makeFacade() *schema.Facade {
	cfg, err := config.Load(".")
	if err != nil {
		cfg = config.Default()
	}
	dir := cfg.MigrationPath
	if dir == "" {
		dir = "./migrations"
	}
	return &schema.Facade{MigrationPath: dir, AutoMigrate: false, Dialect: resolveDialect(cfg)}
}

func cmdGenerate(args []string) error {
	cfg, err := config.Load(".")
	if err != nil {
		return err
	}
	// CLI overrides: --driver= --package= --dsn= --from-db --from-blueprint
	var dsn string
	filtered := args[:0]
	for _, a := range args {
		if after, ok := strings.CutPrefix(a, "--driver="); ok {
			cfg.Driver = after
			continue
		}
		if after, ok := strings.CutPrefix(a, "--package="); ok {
			if err := cfg.Set("PACKAGE", after); err != nil {
				return err
			}
			continue
		}
		if after, ok := strings.CutPrefix(a, "--dsn="); ok {
			dsn = after
			continue
		}
		switch a {
		case "--from-db":
			if err := cfg.Set("MODEL_SOURCE", config.SourceDB); err != nil {
				return err
			}
			continue
		case "--from-blueprint":
			if err := cfg.Set("MODEL_SOURCE", config.SourceBlueprint); err != nil {
				return err
			}
			continue
		}
		filtered = append(filtered, a)
	}
	args = filtered

	what := "all"
	if len(args) > 0 {
		what = args[0]
	}
	switch what {
	case "models":
		res, source, err := generateModels(cfg, dsn)
		if err != nil {
			return err
		}
		fmt.Printf("vorm generate models: %d table(s) from %s, package=%s (DO NOT EDIT)\n", len(res.Tables), source, cfg.ModelPackage)
		for _, f := range res.Files {
			fmt.Println(" ", f)
		}
		return nil
	case "queries", "all":
		if what == "all" {
			mres, source, err := generateModels(cfg, dsn)
			if err != nil {
				return err
			}
			if len(mres.Files) > 0 {
				fmt.Printf("vorm generate models: %d table(s) from %s\n", len(mres.Tables), source)
				for _, f := range mres.Files {
					fmt.Println(" ", f)
				}
			}
		}
		opts := cfg.ToGenerateOptions()
		if opts.Dialect == "" {
			opts.Dialect = resolveDialect(cfg)
		}
		res, err := generate.Run(&opts)
		if err != nil {
			return err
		}
		fmt.Printf("vorm generate queries: %d → %s (package %s, %s/%s)\n",
			res.Queries-len(res.Pending), opts.OutDir, opts.Package, res.Dialect, res.Driver)
		for _, f := range res.GoFiles {
			fmt.Println(" ", f)
		}
		if len(res.Pending) > 0 {
			fmt.Fprintf(os.Stderr, "\n%d stub(s) stayed on the runtime builder — call them directly:\n", len(res.Pending))
			for _, p := range res.Pending {
				fmt.Fprintf(os.Stderr, "  %s (%s): %s\n", p.Name, filepath.Base(p.File), p.Reason)
			}
		}
		return nil
	default:
		return fmt.Errorf("usage: vorm generate [models|queries|all] [--driver=pgx|pq] [--package=gen]")
	}
}

// generateModels prefers live introspection — it is the only source that knows
// nullability, enums, indexes and foreign keys — and falls back to parsing
// Blueprint files when no database is reachable.
func generateModels(cfg *config.Config, dsn string) (*generate.ModelResult, string, error) {
	if !cfg.IntrospectModels() && dsn == "" {
		res, err := generate.Models(cfg.ToModelOptions())
		return res, "schema blueprints", err
	}

	schema, err := loadSchema(context.Background(), cfg, dsn)
	if err != nil {
		if cfg.ModelSource == config.SourceDB && cfg.ResolveDatabaseURL() == "" && dsn == "" {
			return nil, "", err
		}
		fmt.Fprintf(os.Stderr, "vorm: %v\n", err)
		fmt.Fprintln(os.Stderr, "vorm: falling back to schema blueprints (set DATABASE_URL for full fidelity)")
		res, ferr := generate.Models(cfg.ToModelOptions())
		return res, "schema blueprints", ferr
	}

	opts := cfg.ToSchemaOptions()
	opts.Schema = schema
	opts.Dialect = schema.Dialect
	res, err := generate.ModelsFromSchema(opts)
	return res, "database introspection", err
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func usage() {
	fmt.Print(`vorm — migrations, database-introspected models, and generated typed queries

Setup:
  vorm init                         # write .vorm (dialect detected from DATABASE_URL)
  vorm config                       # show effective config
  vorm config set PACKAGE=vormgen   # avoid conflicts with another "gen"
  vorm config lint

Migrations (native runner — no vm binary needed):
  vorm make migration posts         # scaffold Blueprint + model + query stub
  vorm migrate [--dry-run] [--steps=N]
  vorm rollback [--steps=1] [--migration=name] [--all]
  vorm status
  vorm fresh | vorm refresh         # destructive: drop/recreate, then re-apply
  vorm config set RUNNER=vm         # opt back into shelling out to the vm CLI

PostgreSQL prerequisites (declarative files next to the migrations):
  vorm extensions                   # sync migrations/extensions.sql
  vorm enums                        # sync enums.sql (creates types, adds values)
  vorm functions                    # sync functions.sql (CREATE OR REPLACE)
  vorm enums status                 # compare the file with the database
  vorm enums --drop-disabled        # also remove commented-out entries
    --force re-applies an unchanged file; --dry-run prints the SQL

Code generation:
  vorm introspect [--json]          # what vorm reads from the live database
  vorm generate models              # structs, enums, indexes, relations, functions
  vorm generate                     # models + typed queries from // vorm:query stubs
    --from-db | --from-blueprint    # model source (default: db when reachable)
    --dsn=… --driver=pgx|pq --package=gen

Layout:
  schema/migrations/   Blueprint Go (you)
  queries/             // vorm:query stubs (you)
  models/              vorm generate models — NEVER hand-edit
  migrations/          SQL applied by the runner
  vorm/gen/            default OUT_DIR (package gen)

Docs: README.md  docs/USAGE.md  docs/MIGRATIONS.md  LLM.md

`)
}

// lintBeforeMigrate runs the migration linter before SQL is applied and returns
// args with --no-lint stripped. Read-only commands are left alone: nothing is
// about to run, so lint findings are just noise.
func lintBeforeMigrate(cmd string, args []string) []string {
	noLint := false
	filtered := args[:0]
	for _, a := range args {
		if a == "--no-lint" {
			noLint = true
			continue
		}
		filtered = append(filtered, a)
	}
	args = filtered

	if cmd == "status" || (cmd == "migrate" && len(args) > 0 && args[0] == "status") {
		return args
	}
	if noLint {
		return args
	}

	dir := "./migrations"
	if cfg, err := config.Load("."); err == nil && cfg.MigrationPath != "" {
		dir = cfg.MigrationPath
	}
	res, err := lint.Dir(dir)
	if err != nil {
		fatal(err)
	}
	if len(res.Findings) > 0 {
		fmt.Fprint(os.Stderr, lint.Format(res))
	}
	if res.HasErrors() {
		fatal(fmt.Errorf("vorm lint failed — fix errors or pass --no-lint"))
	}
	return args
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
