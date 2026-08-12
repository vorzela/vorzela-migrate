package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/vorzela/vorm/config"
	"github.com/vorzela/vorm/migrate"
	"github.com/vorzela/vorm/query"
	"github.com/vorzela/vorm/vmtool"
)

// migrateFlags are the flags shared by the migration commands. They mirror the
// vm CLI so muscle memory carries over.
type migrateFlags struct {
	dsn          string
	path         string
	steps        string
	migration    string
	force        bool
	dryRun       bool
	verbose      bool
	skipLock     bool
	dropDisabled bool
	step         int
	rest         []string
}

func parseMigrateFlags(args []string) (migrateFlags, error) {
	var f migrateFlags
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func(name string) (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", name)
			}
			i++
			return args[i], nil
		}
		var err error
		switch {
		case a == "--dsn", a == "-d":
			f.dsn, err = next(a)
		case strings.HasPrefix(a, "--dsn="):
			f.dsn = strings.TrimPrefix(a, "--dsn=")
		case a == "--path", a == "-p":
			f.path, err = next(a)
		case strings.HasPrefix(a, "--path="):
			f.path = strings.TrimPrefix(a, "--path=")
		case a == "--steps", a == "--step", a == "-n":
			f.steps, err = next(a)
		case strings.HasPrefix(a, "--steps="):
			f.steps = strings.TrimPrefix(a, "--steps=")
		case strings.HasPrefix(a, "--step="):
			f.steps = strings.TrimPrefix(a, "--step=")
		case a == "--migration", a == "-m":
			f.migration, err = next(a)
		case strings.HasPrefix(a, "--migration="):
			f.migration = strings.TrimPrefix(a, "--migration=")
		case a == "--force":
			f.force = true
		case a == "--dry-run":
			f.dryRun = true
		case a == "--verbose", a == "-v":
			f.verbose = true
		case a == "--skip-lock":
			f.skipLock = true
		case a == "--drop-disabled":
			f.dropDisabled = true
		default:
			f.rest = append(f.rest, a)
		}
		if err != nil {
			return f, err
		}
	}
	if f.steps != "" && !strings.EqualFold(f.steps, "all") {
		n, err := strconv.Atoi(f.steps)
		if err != nil {
			return f, fmt.Errorf("--steps wants a number or \"all\", got %q", f.steps)
		}
		f.step = n
	}
	return f, nil
}

// cmdMigrate runs migrations in-process. It only shells out to vm when the
// project pins RUNNER=vm.
func cmdMigrate(cmd string, args []string) error {
	cfg, err := config.Load(".")
	if err != nil {
		return err
	}
	if !cfg.UseNativeRunner() {
		return vmtool.Run(true, append([]string{cmd}, args...)...)
	}

	flags, err := parseMigrateFlags(args)
	if err != nil {
		return err
	}

	// `vorm migrate status` and `vorm status` mean the same thing; accepting
	// both spellings avoids silently running a migration when the user asked
	// for something else.
	if cmd == "migrate" && len(flags.rest) > 0 {
		switch sub := strings.ToLower(flags.rest[0]); sub {
		case "up":
			flags.rest = flags.rest[1:]
		case "down":
			cmd, flags.rest = "rollback", flags.rest[1:]
		case "status", "rollback", "fresh", "refresh":
			cmd, flags.rest = sub, flags.rest[1:]
		}
	}
	if len(flags.rest) > 0 {
		return fmt.Errorf("unexpected argument %q for %s", flags.rest[0], cmd)
	}

	url := flags.dsn
	if url == "" {
		url = cfg.ResolveDatabaseURL()
	}
	if url == "" {
		return fmt.Errorf("no database connection: set DATABASE_URL in the environment, add it to %s, or pass --dsn", config.DefaultFile)
	}

	opts := cfg.ToMigrateOptions()
	if flags.path != "" {
		opts.Dir = flags.path
	}
	opts.Dialect = migrate.DetectDialect(url)
	opts.RunPrereq = opts.Dialect == query.DialectPostgres
	opts.Force = flags.force
	opts.DryRun = flags.dryRun
	opts.SkipLock = flags.skipLock
	opts.Step = 0
	if cmd == "migrate" && flags.step > 0 {
		opts.Step = flags.step
	}
	if flags.verbose {
		opts.Logger = log.New(os.Stderr, "vorm: ", 0)
	}

	ctx := context.Background()
	conn, err := query.Open(ctx, url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()

	runner := migrate.New(conn, opts)

	switch cmd {
	case "status":
		rows, err := runner.Status(ctx)
		if err != nil {
			return err
		}
		printStatus(rows)
		return nil

	case "migrate":
		report, err := runner.Up(ctx)
		printReport(cmd, report, opts.DryRun)
		return err

	case "rollback":
		var report *migrate.Report
		switch {
		case flags.migration != "":
			report, err = runner.DownByName(ctx, flags.migration)
		case strings.EqualFold(flags.steps, "all"):
			report, err = runner.DownAll(ctx)
		default:
			report, err = runner.Down(ctx, flags.step)
		}
		printReport(cmd, report, opts.DryRun)
		return err

	case "fresh", "refresh":
		if !flags.force && !flags.dryRun {
			return fmt.Errorf("%s drops and re-applies every migration — pass --force to confirm", cmd)
		}
		report, err := runner.Fresh(ctx)
		printReport(cmd, report, opts.DryRun)
		return err
	}
	return fmt.Errorf("unknown migration command %q", cmd)
}

func printReport(cmd string, report *migrate.Report, dryRun bool) {
	if report == nil {
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, s := range report.Prereq {
		fmt.Fprintf(w, "  prereq\t%s\t%s\n", s.Name, stepStatus(s))
	}
	// A fresh run reverts and re-applies, so label each half rather than
	// reporting one file twice under the same verb.
	var reverted, applied int
	for _, s := range report.Steps {
		label := cmd
		if s.Down {
			label = "rollback"
		}
		if s.Applied {
			if s.Down {
				reverted++
			} else {
				applied++
			}
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\n", label, s.Name, stepStatus(s))
	}
	w.Flush()

	prefix := ""
	if dryRun {
		prefix = "dry run: "
	}
	switch {
	case report.Failed > 0:
		fmt.Printf("%s%s: %d applied, %d failed\n", prefix, cmd, report.Applied, report.Failed)
	case report.Applied == 0:
		fmt.Printf("%s%s: nothing to do\n", prefix, cmd)
	case reverted > 0 && applied > 0:
		fmt.Printf("%s%s: %d reverted, %d re-applied in batch %d\n", prefix, cmd, reverted, applied, report.Batch)
	case report.Batch > 0:
		fmt.Printf("%s%s: %d migration(s) in batch %d\n", prefix, cmd, report.Applied, report.Batch)
	default:
		fmt.Printf("%s%s: %d migration(s)\n", prefix, cmd, report.Applied)
	}
}

func stepStatus(s migrate.StepResult) string {
	switch {
	case s.Err != nil:
		return "FAILED: " + s.Err.Error()
	case s.Skipped:
		if s.Reason != "" {
			return "skipped (" + s.Reason + ")"
		}
		return "skipped"
	case s.Applied:
		return fmt.Sprintf("ok (%s)", s.Duration.Round(time.Millisecond))
	default:
		return "pending"
	}
}

func printStatus(rows []migrate.StatusRow) {
	if len(rows) == 0 {
		fmt.Println("no migrations found")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MIGRATION\tSTATUS\tBATCH")
	pending := 0
	for _, r := range rows {
		status := "pending"
		batch := ""
		switch {
		case r.Missing:
			status = "MISSING FILE"
			batch = strconv.Itoa(r.Batch)
		case r.Applied && !r.ChecksumOK:
			status = "CHANGED SINCE APPLIED"
			batch = strconv.Itoa(r.Batch)
		case r.Applied:
			status = "applied"
			batch = strconv.Itoa(r.Batch)
		default:
			pending++
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, status, batch)
	}
	w.Flush()
	fmt.Printf("\n%d migration(s), %d pending\n", len(rows), pending)
}
