package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/vorzela/vorm/config"
	"github.com/vorzela/vorm/migrate"
	"github.com/vorzela/vorm/query"
)

// prereqKind describes one of the declarative PostgreSQL files that live
// alongside the migrations.
type prereqKind struct {
	cmd   string // extensions | enums | functions
	file  string // extensions.sql | enums.sql | functions.sql
	hash  string // sidecar caching the last applied checksum
	label string // singular noun for messages
}

var prereqKinds = map[string]prereqKind{
	"extensions": {"extensions", migrate.ExtensionsFile, ".vm_extensions_hash", "extension"},
	"enums":      {"enums", migrate.EnumsFile, ".vm_enums_hash", "enum type"},
	"functions":  {"functions", migrate.FunctionsFile, ".vm_functions_hash", "function"},
}

// cmdPrereq runs `vorm extensions|enums|functions [sync|status|drop]` natively.
// Sync is the default: create the file from a template when it is missing, then
// apply whatever it enables.
func cmdPrereq(cmd string, args []string) error {
	kind := prereqKinds[cmd]

	flags, err := parseMigrateFlags(args)
	if err != nil {
		return err
	}
	action := "sync"
	if len(flags.rest) > 0 {
		switch sub := strings.ToLower(flags.rest[0]); sub {
		case "sync", "migrate", "up":
			action, flags.rest = "sync", flags.rest[1:]
		case "status", "drop":
			action, flags.rest = sub, flags.rest[1:]
		}
	}
	if len(flags.rest) > 0 {
		return fmt.Errorf("unexpected argument %q for %s", flags.rest[0], cmd)
	}

	cfg, err := config.Load(".")
	if err != nil {
		return err
	}
	dir := cfg.MigrationPath
	if flags.path != "" {
		dir = flags.path
	}
	if dir == "" {
		dir = migrate.DefaultDir
	}

	path, created, err := migrate.EnsurePrereqFile(dir, kind.file)
	if err != nil {
		return err
	}
	if created {
		fmt.Printf("wrote %s\n", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	url := flags.dsn
	if url == "" {
		url = cfg.ResolveDatabaseURL()
	}
	if url == "" {
		return fmt.Errorf("no database connection: set DATABASE_URL in the environment, add it to %s, or pass --dsn", config.DefaultFile)
	}
	if d := migrate.DetectDialect(url); d != query.DialectPostgres {
		return fmt.Errorf("vorm %s is PostgreSQL-only (detected %s)", cmd, d)
	}

	ctx := context.Background()
	db, err := query.Open(ctx, url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer db.Close()

	switch action {
	case "status":
		return prereqStatus(ctx, db, kind, string(content))
	case "drop":
		if !flags.force && !flags.dryRun {
			return fmt.Errorf("vorm %s drop removes every %s listed in %s — pass --force to confirm", cmd, kind.label, kind.file)
		}
		return prereqDrop(ctx, db, kind, string(content), flags.dryRun)
	default:
		return prereqSync(ctx, db, kind, dir, path, content, flags)
	}
}

func prereqSync(ctx context.Context, db query.Conn, kind prereqKind, dir, path string, content []byte, flags migrateFlags) error {
	enabled, disabled := prereqNames(kind, string(content))
	if len(enabled) == 0 {
		fmt.Printf("%s: nothing enabled — uncomment a line and rerun\n", path)
		// Commenting the last entry out is still a request to remove it.
		if len(disabled) > 0 && flags.dropDisabled {
			return dropNames(ctx, db, kind, disabled, flags.dryRun, "  ")
		}
		return nil
	}

	sum := migrate.ChecksumBytes(content)
	hashPath := filepath.Join(dir, kind.hash)
	cached, _ := os.ReadFile(hashPath)
	unchanged := string(cached) == sum

	if unchanged && !flags.force && !flags.dryRun {
		fmt.Printf("%s: unchanged since the last sync (%d enabled) — pass --force to re-apply\n", kind.file, len(enabled))
		return nil
	}

	statements := migrate.PrereqStatements(kind.file, string(content))
	if flags.dryRun {
		for _, s := range statements {
			fmt.Println(s)
		}
		fmt.Printf("dry run: %s would apply %d statement(s)\n", kind.file, len(statements))
		return nil
	}

	for _, s := range statements {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("%s: %w", kind.file, err)
		}
	}
	if err := os.WriteFile(hashPath, []byte(sum), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: write %s: %v\n", hashPath, err)
	}
	fmt.Printf("%s: %d %s(s) applied\n", kind.file, len(enabled), kind.label)

	// Commented-out entries are only removed on request: dropping an extension
	// or a function cascades, and that is not something to do by surprise.
	if len(disabled) > 0 {
		if !flags.dropDisabled {
			fmt.Printf("  %d disabled (%s) — remove from the database with --drop-disabled\n",
				len(disabled), strings.Join(disabled, ", "))
			return nil
		}
		return dropNames(ctx, db, kind, disabled, flags.dryRun, "  ")
	}
	return nil
}

// dropNames drops each object and then reports what actually went away: an
// enum still used by a column is deliberately kept, and saying "dropped" when
// nothing was dropped would be a lie.
func dropNames(ctx context.Context, db query.Conn, kind prereqKind, names []string, dryRun bool, indent string) error {
	if dryRun {
		for _, name := range names {
			fmt.Println(dropSQL(kind, name))
		}
		fmt.Printf("%sdry run: %d %s(s) would be dropped\n", indent, len(names), kind.label)
		return nil
	}

	for _, name := range names {
		if _, err := db.ExecContext(ctx, dropSQL(kind, name)); err != nil {
			return fmt.Errorf("drop %s %s: %w", kind.label, name, err)
		}
	}
	live, err := liveNames(ctx, db, kind)
	if err != nil {
		return err
	}
	for _, name := range names {
		if live[name] {
			fmt.Printf("%skept %s %s (still in use)\n", indent, kind.label, name)
			continue
		}
		fmt.Printf("%sdropped %s %s\n", indent, kind.label, name)
	}
	return nil
}

func prereqDrop(ctx context.Context, db query.Conn, kind prereqKind, content string, dryRun bool) error {
	enabled, _ := prereqNames(kind, content)
	if len(enabled) == 0 {
		fmt.Printf("%s: nothing to drop\n", kind.file)
		return nil
	}
	return dropNames(ctx, db, kind, enabled, dryRun, "")
}

func prereqStatus(ctx context.Context, db query.Conn, kind prereqKind, content string) error {
	enabled, disabled := prereqNames(kind, content)
	live, err := liveNames(ctx, db, kind)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "%s\tFILE\tDATABASE\n", strings.ToUpper(kind.label))
	seen := map[string]bool{}
	for _, name := range enabled {
		seen[name] = true
		fmt.Fprintf(w, "%s\tenabled\t%s\n", name, presence(live[name]))
	}
	for _, name := range disabled {
		seen[name] = true
		fmt.Fprintf(w, "%s\tdisabled\t%s\n", name, presence(live[name]))
	}
	for _, name := range sortedNames(live) {
		if !seen[name] {
			fmt.Fprintf(w, "%s\t-\tinstalled (not in %s)\n", name, kind.file)
		}
	}
	w.Flush()
	return nil
}

func presence(ok bool) string {
	if ok {
		return "installed"
	}
	return "missing"
}

func prereqNames(kind prereqKind, content string) (enabled, disabled []string) {
	switch kind.cmd {
	case "extensions":
		return migrate.ParseExtensions(content)
	case "functions":
		return migrate.ParseFunctions(content)
	default:
		specs, off := migrate.ParseEnums(content)
		for _, s := range specs {
			enabled = append(enabled, s.Name)
		}
		return enabled, off
	}
}

func dropSQL(kind prereqKind, name string) string {
	switch kind.cmd {
	case "extensions":
		return fmt.Sprintf("DROP EXTENSION IF EXISTS %s CASCADE;", quoteIdent(name))
	case "functions":
		return fmt.Sprintf("DROP FUNCTION IF EXISTS %s CASCADE;", quoteIdent(name))
	default:
		return migrate.DropEnumIfUnusedSQL(name)
	}
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// liveNames reads what the database actually has, for status output.
func liveNames(ctx context.Context, db query.Conn, kind prereqKind) (map[string]bool, error) {
	var sql string
	switch kind.cmd {
	case "extensions":
		sql = `SELECT extname FROM pg_extension ORDER BY extname`
	case "functions":
		sql = `SELECT p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		       WHERE n.nspname = current_schema() ORDER BY p.proname`
	default:
		sql = `SELECT t.typname FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
		       WHERE n.nspname = current_schema() AND t.typtype = 'e' ORDER BY t.typname`
	}

	rows, err := db.QueryContext(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// sortedNames keeps status output stable regardless of map iteration order.
func sortedNames(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
