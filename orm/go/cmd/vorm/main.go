package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/vorzela/vorm/config"
	"github.com/vorzela/vorm/generate"
	"github.com/vorzela/vorm/lint"
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
		if cmd == "migrate" {
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
			if !noLint {
				res, err := lint.Dir("./migrations")
				if err != nil {
					fatal(err)
				}
				if len(res.Findings) > 0 {
					fmt.Fprint(os.Stderr, lint.Format(res))
				}
				if res.HasErrors() {
					fatal(fmt.Errorf("vorm lint failed — fix errors or pass --no-lint"))
				}
			}
		}
		if err := vmtool.Run(true, append([]string{cmd}, args...)...); err != nil {
			fatal(err)
		}
	case "extensions", "enums", "functions":
		if err := vmtool.Run(true, append([]string{cmd}, args...)...); err != nil {
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
	if d := vmtool.DetectDialect(); d != "" {
		cfg.Dialect = d
	}
	if err := cfg.Write(path); err != nil {
		return err
	}
	fmt.Printf("wrote %s (PACKAGE=%s)\n", path, cfg.Package)
	fmt.Println("edit PACKAGE to avoid conflicts, e.g. vorm config set PACKAGE=vormgen")
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
		f := &schema.Facade{MigrationPath: "./migrations", AutoMigrate: false, EnsureVM: true, Dialect: vmtool.DetectDialect()}
		return f.CreateEnum(args[1], splitCSV(args[2])...)
	case "extension":
		if len(args) < 2 {
			return fmt.Errorf("usage: vorm make extension <name>")
		}
		f := &schema.Facade{MigrationPath: "./migrations", AutoMigrate: false, EnsureVM: true, Dialect: vmtool.DetectDialect()}
		return f.CreateExtension(args[1])
	default:
		return fmt.Errorf("unknown make target %q — try: vorm make migration posts", args[0])
	}
}

func cmdGenerate(args []string) error {
	cfg, err := config.Load(".")
	if err != nil {
		return err
	}
	// CLI overrides: --driver= --package=
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
		filtered = append(filtered, a)
	}
	args = filtered

	what := "all"
	if len(args) > 0 {
		what = args[0]
	}
	switch what {
	case "models":
		mopts := cfg.ToModelOptions()
		res, err := generate.Models(mopts)
		if err != nil {
			return err
		}
		fmt.Printf("vorm generate models: %d table(s) package=%s (DO NOT EDIT)\n", len(res.Tables), cfg.ModelPackage)
		for _, f := range res.Files {
			fmt.Println(" ", f)
		}
		return nil
	case "queries", "all":
		if what == "all" {
			mres, err := generate.Models(cfg.ToModelOptions())
			if err != nil {
				return err
			}
			if len(mres.Files) > 0 {
				fmt.Printf("vorm generate models: %d table(s)\n", len(mres.Tables))
				for _, f := range mres.Files {
					fmt.Println(" ", f)
				}
			}
		}
		opts := cfg.ToGenerateOptions()
		if opts.Dialect == "" || opts.Dialect == "postgres" {
			if d := vmtool.DetectDialect(); d != "" {
				opts.Dialect = d
			}
		}
		res, err := generate.Run(&opts)
		if err != nil {
			return err
		}
		fmt.Printf("vorm generate queries: %d → %s (package %s, %s/%s)\n",
			res.Queries, opts.OutDir, opts.Package, res.Dialect, res.Driver)
		for _, f := range res.GoFiles {
			fmt.Println(" ", f)
		}
		return nil
	default:
		return fmt.Errorf("usage: vorm generate [models|queries|all] [--driver=pgx|pq] [--package=gen]")
	}
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
	fmt.Print(`vorm — Schema.Create → models (DO NOT EDIT) → vorm/<package>

Config (.vorm):
  vorm init                         # write .vorm (PACKAGE=gen by default)
  vorm config                       # show effective config
  vorm config set PACKAGE=vormgen   # avoid conflicts with another "gen"
  vorm config get DRIVER

Layout:
  schema/migrations/   Blueprint Go (you)
  queries/             // vorm:query stubs (you)
  models/              vorm generate models — NEVER hand-edit
  migrations/          SQL for vm
  vorm/gen/            default OUT_DIR (package gen)

  vorm init
  vorm config set PACKAGE=vormgen
  vorm config lint
  vorm generate models && vorm generate

Docs: README.md  LLM.md  examples/generated/

`)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
