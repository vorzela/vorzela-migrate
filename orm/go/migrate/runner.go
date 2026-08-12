// Package migrate applies Vorzela Migrate SQL files natively, so a Go program no
// longer needs the vm binary on the host to migrate its database.
//
// The file layout, the migrations tracking table, batch numbering and checksums
// match vm exactly, which means a project can be migrated with either tool
// interchangeably.
//
//	conn, err := query.OpenPostgres(ctx, os.Getenv("DATABASE_URL"))
//	if err != nil {
//		return err
//	}
//	defer conn.Close()
//
//	opts := migrate.DefaultOptions()
//	opts.Dialect = migrate.DetectDialect(os.Getenv("DATABASE_URL"))
//	report, err := migrate.New(conn, opts).Up(ctx)
//
// Migration files are named {unix_timestamp}_{snake_name}.sql and hold an Up and
// a Down section, delimited by the markers vm writes (⬆/⬇) or by the goose,
// golang-migrate or plain "-- Up" / "-- Down" equivalents.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vorzela/vorm/query"
)

// DefaultDir is the conventional migrations directory.
const DefaultDir = "./migrations"

// DefaultLockTimeout bounds how long a run waits for the migration lock.
const DefaultLockTimeout = 30 * time.Second

var (
	// ErrChecksumMismatch reports that an applied migration file was edited
	// after it ran. Options.Force accepts the new contents.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrMissingFile reports a tracking table row whose file is gone from disk.
	ErrMissingFile = errors.New("applied migration file is missing")

	// ErrLockTimeout reports that another migration still holds the lock.
	ErrLockTimeout = errors.New("timed out waiting for the migration lock: another migration is running")

	// ErrNoMatch is returned by DownByName when no applied migration matches.
	ErrNoMatch = errors.New("no applied migration matches")

	errNoDB = errors.New("vorm/migrate: Runner was created without a database")
)

// Logger receives progress and warning lines. *log.Logger satisfies it.
type Logger interface {
	Printf(format string, args ...any)
}

// Options configures a Runner. The zero value is usable, but DefaultOptions
// carries the recommended settings.
type Options struct {
	// Dir holds the migration files. Defaults to DefaultDir.
	Dir string

	// Dialect selects the bind marker style and the tracking table DDL.
	// Defaults to query.DialectPostgres; "mariadb" counts as MySQL.
	Dialect query.Dialect

	// VerifyChecksum re-hashes already applied files before new ones run, so an
	// edit to history is caught early. DefaultOptions enables it; a zero-value
	// Options does not verify.
	VerifyChecksum bool

	// Force continues past a checksum mismatch — the stored checksums are
	// rewritten to the current contents — and skips rollback steps whose file
	// has disappeared instead of failing.
	Force bool

	// DryRun reports what would run without executing any migration SQL. The
	// tracking table is still created and read, since pending work cannot be
	// determined without it.
	DryRun bool

	// Step caps how many pending migrations Up applies. Zero applies all.
	Step int

	// RunPrereq applies extensions.sql, functions.sql and enums.sql before the
	// migrations. PostgreSQL only; ignored on MySQL and MariaDB.
	RunPrereq bool

	// SkipLock skips the migration lock. Useful when the caller already
	// serialises migrations, or against a database that has no lock functions.
	SkipLock bool

	// LockTimeout bounds the wait for the lock. Defaults to DefaultLockTimeout.
	LockTimeout time.Duration

	// Logger receives progress and warnings. Nil discards them.
	Logger Logger
}

// DefaultOptions returns options for a PostgreSQL project laid out the way vm
// scaffolds it, with checksum verification on.
func DefaultOptions() Options {
	return Options{
		Dir:            DefaultDir,
		Dialect:        query.DialectPostgres,
		VerifyChecksum: true,
		LockTimeout:    DefaultLockTimeout,
	}
}

// StepResult is the outcome of one migration file.
type StepResult struct {
	Name string

	// Down is true when the step reverted a migration rather than applying it,
	// which lets a Fresh report be read one half at a time.
	Down bool

	// Applied is true when the SQL ran and the tracking table was updated: the
	// migration was applied by Up, or reverted by one of the Down calls.
	Applied bool

	// Skipped is true when the file was deliberately not run, see Reason.
	Skipped bool

	// Reason explains a skip, e.g. "dry run" or "no Down section".
	Reason string

	Duration time.Duration

	// Err is set when the step failed; the run stops at the first failure.
	Err error

	// Transactional reports whether the statements ran inside one transaction.
	// It is false for a dry run, when no query.Beginner is available, and for
	// migrations that must run outside a transaction.
	Transactional bool
}

// Report summarises one Up, Down or Fresh call.
type Report struct {
	// Batch is the batch number Up assigned to the migrations it applied. It is
	// zero when nothing was pending.
	Batch int

	// Prereq holds the extensions.sql, functions.sql and enums.sql results.
	Prereq []StepResult

	// Steps is one entry per migration considered, in execution order.
	Steps []StepResult

	// Applied counts the steps that ran successfully.
	Applied int

	// Failed counts the steps that failed. The run stops at the first failure,
	// so it is never larger than one.
	Failed int
}

// StatusRow is one line of Status.
type StatusRow struct {
	Name string

	Applied bool

	// Batch is the batch that applied the migration; zero while pending.
	Batch int

	// ChecksumOK is false when the file is missing, or when it no longer hashes
	// to the value stored in the tracking table. Pending migrations and rows
	// recorded before checksums existed report true.
	ChecksumOK bool

	// Missing is true for a tracking table row with no file on disk.
	Missing bool
}

// Runner applies and reverts migrations against one database.
type Runner struct {
	db   query.DB
	tx   query.Beginner
	opts Options
}

// New returns a Runner for db, filling in the option defaults. When db can start
// transactions — every query.Conn can — Up applies each migration atomically;
// WithTx overrides that choice.
func New(db query.DB, opts Options) *Runner {
	if opts.Dir == "" {
		opts.Dir = DefaultDir
	}
	if opts.Dialect == "" {
		opts.Dialect = query.DialectPostgres
	}
	if opts.LockTimeout <= 0 {
		opts.LockTimeout = DefaultLockTimeout
	}
	r := &Runner{db: db, opts: opts}
	if beginner, ok := db.(query.Beginner); ok {
		r.tx = beginner
	}
	return r
}

// WithTx sets the transaction source used to apply migrations, normally the same
// object passed to New. A nil argument turns transactional apply off.
func (r *Runner) WithTx(b query.Beginner) *Runner {
	r.tx = b
	return r
}

// Options returns the resolved options.
func (r *Runner) Options() Options { return r.opts }

// Up applies every pending migration, in timestamp order, under a single batch
// number.
func (r *Runner) Up(ctx context.Context) (*Report, error) {
	report := &Report{}
	err := r.withLock(ctx, func(ctx context.Context) error {
		return r.up(ctx, report)
	})
	return report, err
}

// Down rolls back the most recent steps batches — batches, not files — newest
// migration first. A steps of zero or less means one batch.
func (r *Runner) Down(ctx context.Context, steps int) (*Report, error) {
	if steps <= 0 {
		steps = 1
	}
	return r.rollback(ctx, func(rows []AppliedRow) ([]AppliedRow, error) {
		return selectBatches(rows, steps), nil
	})
}

// DownAll rolls back every applied migration, newest first.
func (r *Runner) DownAll(ctx context.Context) (*Report, error) {
	return r.rollback(ctx, allRows)
}

// DownByName rolls back the oldest applied migration whose file name contains
// substr, case-insensitively.
func (r *Runner) DownByName(ctx context.Context, substr string) (*Report, error) {
	return r.rollback(ctx, func(rows []AppliedRow) ([]AppliedRow, error) {
		want := strings.ToLower(strings.TrimSpace(substr))
		if want == "" {
			return nil, fmt.Errorf("vorm/migrate: DownByName needs a name to match")
		}
		for _, row := range rows {
			if strings.Contains(strings.ToLower(row.Name), want) {
				return []AppliedRow{row}, nil
			}
		}
		return nil, fmt.Errorf("vorm/migrate: %w %q", ErrNoMatch, substr)
	})
}

// Fresh rolls every migration back and applies them again under one lock. The
// report concatenates the rollback and the apply steps.
func (r *Runner) Fresh(ctx context.Context) (*Report, error) {
	report := &Report{}
	err := r.withLock(ctx, func(ctx context.Context) error {
		if err := r.down(ctx, report, allRows); err != nil {
			return err
		}
		return r.up(ctx, report)
	})
	return report, err
}

// Refresh is an alias for Fresh, mirroring vm's command set.
func (r *Runner) Refresh(ctx context.Context) (*Report, error) {
	return r.Fresh(ctx)
}

// Status lists every migration file with its applied state, followed by any
// tracking table row whose file is gone. It creates the tracking table when
// missing so that a fresh database reports cleanly.
func (r *Runner) Status(ctx context.Context) ([]StatusRow, error) {
	if err := r.EnsureTable(ctx); err != nil {
		return nil, err
	}
	files, err := Discover(r.opts.Dir)
	if err != nil {
		return nil, err
	}
	rows, err := r.Applied(ctx)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]AppliedRow, len(rows))
	for _, row := range rows {
		byName[row.Name] = row
	}

	out := make([]StatusRow, 0, len(files)+len(rows))
	onDisk := make(map[string]bool, len(files))
	for _, file := range files {
		onDisk[file.Name] = true
		status := StatusRow{Name: file.Name, ChecksumOK: true}
		if row, ok := byName[file.Name]; ok {
			status.Applied = true
			status.Batch = row.Batch
			status.ChecksumOK = row.Checksum == "" || row.Checksum == file.Checksum
		}
		out = append(out, status)
	}
	for _, row := range rows {
		if onDisk[row.Name] {
			continue
		}
		out = append(out, StatusRow{Name: row.Name, Applied: true, Batch: row.Batch, Missing: true})
	}
	return out, nil
}

func (r *Runner) up(ctx context.Context, report *Report) error {
	if err := r.EnsureTable(ctx); err != nil {
		return err
	}
	files, err := Discover(r.opts.Dir)
	if err != nil {
		return err
	}
	rows, err := r.Applied(ctx)
	if err != nil {
		return err
	}

	if r.opts.VerifyChecksum {
		if err := r.verifyChecksums(ctx, rows, files); err != nil {
			return err
		}
	}

	if r.opts.RunPrereq {
		if isMySQL(r.opts.Dialect) {
			r.logf("migrate: extensions, functions and enums are PostgreSQL-only; skipping")
		} else {
			steps, err := r.runPrereq(ctx)
			report.Prereq = steps
			if err != nil {
				return err
			}
		}
	}

	done := make(map[string]bool, len(rows))
	for _, row := range rows {
		done[row.Name] = true
	}
	pending := make([]Migration, 0, len(files))
	for _, file := range files {
		if !done[file.Name] {
			pending = append(pending, file)
		}
	}
	if r.opts.Step > 0 && r.opts.Step < len(pending) {
		pending = pending[:r.opts.Step]
	}
	if len(pending) == 0 {
		r.logf("migrate: nothing to apply")
		return nil
	}

	batch, err := r.nextBatch(ctx)
	if err != nil {
		return err
	}
	report.Batch = batch

	for _, migration := range pending {
		result := r.applyOne(ctx, migration, batch)
		report.Steps = append(report.Steps, result)
		switch {
		case result.Err != nil:
			report.Failed++
			return result.Err
		case result.Applied:
			report.Applied++
		default:
			r.logf("migrate: skipped %s: %s", result.Name, result.Reason)
		}
	}
	return nil
}

func (r *Runner) applyOne(ctx context.Context, migration Migration, batch int) StepResult {
	result := StepResult{Name: migration.Name}

	content, err := os.ReadFile(migration.Path)
	if err != nil {
		result.Err = fmt.Errorf("vorm/migrate: read %s: %w", migration.Path, err)
		return result
	}
	statements := SplitStatements(ExtractUp(string(content)))
	if len(statements) == 0 {
		result.Skipped = true
		result.Reason = "no Up section"
		return result
	}
	// Hash what is about to run rather than what Discover saw, so the recorded
	// checksum always describes the executed bytes.
	checksum := ChecksumBytes(content)

	result.Transactional = r.tx != nil && !requiresNoTransaction(statements, r.opts.Dialect)
	if r.opts.DryRun {
		result.Skipped = true
		result.Reason = "dry run"
		result.Transactional = false
		return result
	}

	start := time.Now()
	if result.Transactional {
		err = query.Transaction(ctx, r.tx, func(ctx context.Context, tx query.Tx) error {
			if err := execStatements(ctx, tx, migration.Name, statements); err != nil {
				return err
			}
			return r.insertRow(ctx, tx, migration.Name, batch, checksum, time.Since(start).Milliseconds())
		})
	} else {
		err = execStatements(ctx, r.db, migration.Name, statements)
		if err == nil {
			err = r.insertRow(ctx, r.db, migration.Name, batch, checksum, time.Since(start).Milliseconds())
		}
	}
	result.Duration = time.Since(start)
	if err != nil {
		result.Err = err
		return result
	}

	result.Applied = true
	r.logf("migrate: applied %s (batch %d, %s)", migration.Name, batch, result.Duration.Round(time.Millisecond))
	return result
}

// rowSelector picks the tracking table rows a rollback should revert.
type rowSelector func([]AppliedRow) ([]AppliedRow, error)

func allRows(rows []AppliedRow) ([]AppliedRow, error) { return rows, nil }

func (r *Runner) rollback(ctx context.Context, selector rowSelector) (*Report, error) {
	report := &Report{}
	err := r.withLock(ctx, func(ctx context.Context) error {
		return r.down(ctx, report, selector)
	})
	return report, err
}

func (r *Runner) down(ctx context.Context, report *Report, selector rowSelector) error {
	if err := r.EnsureTable(ctx); err != nil {
		return err
	}
	rows, err := r.Applied(ctx)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		r.logf("migrate: nothing to roll back")
		return nil
	}
	selected, err := selector(rows)
	if err != nil {
		return err
	}
	chosen := append([]AppliedRow(nil), selected...)
	// Highest id first: undo in the reverse of the order the migrations ran.
	sort.SliceStable(chosen, func(i, j int) bool { return chosen[i].ID > chosen[j].ID })

	for _, row := range chosen {
		result := r.revertOne(ctx, row)
		report.Steps = append(report.Steps, result)
		switch {
		case result.Err != nil:
			report.Failed++
			return result.Err
		case result.Applied:
			report.Applied++
		default:
			r.logf("migrate: skipped rollback of %s: %s", result.Name, result.Reason)
		}
	}
	return nil
}

func (r *Runner) revertOne(ctx context.Context, row AppliedRow) StepResult {
	result := StepResult{Name: row.Name, Down: true}

	path := filepath.Join(r.opts.Dir, row.Name)
	content, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if r.opts.Force {
			result.Skipped = true
			result.Reason = "file is missing"
			return result
		}
		result.Err = fmt.Errorf("vorm/migrate: %w: %s (set Options.Force to skip it)", ErrMissingFile, row.Name)
		return result
	case err != nil:
		result.Err = fmt.Errorf("vorm/migrate: read %s: %w", path, err)
		return result
	}

	statements := SplitStatements(ExtractDown(string(content)))
	if len(statements) == 0 {
		result.Skipped = true
		result.Reason = "no Down section"
		return result
	}
	if r.opts.DryRun {
		result.Skipped = true
		result.Reason = "dry run"
		return result
	}

	// vm runs Down sections without a wrapping transaction; keeping that means a
	// rollback of a non-transactional migration behaves the same under both tools.
	start := time.Now()
	if err := execStatements(ctx, r.db, row.Name, statements); err != nil {
		result.Duration = time.Since(start)
		result.Err = err
		return result
	}
	if err := r.deleteRow(ctx, row.ID); err != nil {
		result.Duration = time.Since(start)
		result.Err = err
		return result
	}

	result.Duration = time.Since(start)
	result.Applied = true
	r.logf("migrate: rolled back %s (batch %d, %s)", row.Name, row.Batch, result.Duration.Round(time.Millisecond))
	return result
}

// verifyChecksums compares the recorded checksums with the files on disk.
func (r *Runner) verifyChecksums(ctx context.Context, rows []AppliedRow, files []Migration) error {
	onDisk := make(map[string]string, len(files))
	for _, file := range files {
		onDisk[file.Name] = file.Checksum
	}

	var missing, mismatched []string
	current := make(map[string]string)
	for _, row := range rows {
		sum, ok := onDisk[row.Name]
		if !ok {
			// The row may name a file Discover ignores, e.g. one applied before it
			// was renamed, so hash the path directly before calling it gone.
			var err error
			sum, err = Checksum(filepath.Join(r.opts.Dir, row.Name))
			switch {
			case errors.Is(err, fs.ErrNotExist):
				missing = append(missing, row.Name)
				continue
			case err != nil:
				return err
			}
		}
		if row.Checksum == "" {
			continue // recorded before checksums existed, nothing to compare
		}
		if row.Checksum != sum {
			mismatched = append(mismatched, row.Name)
			current[row.Name] = sum
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("vorm/migrate: %w: %s", ErrMissingFile, strings.Join(missing, ", "))
	}
	if len(mismatched) == 0 {
		return nil
	}
	if !r.opts.Force {
		return fmt.Errorf("vorm/migrate: %w in %s (set Options.Force to accept the new contents)",
			ErrChecksumMismatch, strings.Join(mismatched, ", "))
	}

	r.logf("migrate: warning: %s changed after being applied; continuing because Force is set",
		strings.Join(mismatched, ", "))
	if r.opts.DryRun {
		return nil
	}
	for _, name := range mismatched {
		if err := r.updateChecksum(ctx, name, current[name]); err != nil {
			return err
		}
	}
	return nil
}

// selectBatches returns the rows belonging to the steps highest distinct batch
// numbers. vm counts batches, so one Down undoes a whole Up regardless of how
// many files it applied.
func selectBatches(rows []AppliedRow, steps int) []AppliedRow {
	seen := make(map[int]bool, len(rows))
	batches := make([]int, 0, len(rows))
	for _, row := range rows {
		if !seen[row.Batch] {
			seen[row.Batch] = true
			batches = append(batches, row.Batch)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(batches)))
	if steps < len(batches) {
		batches = batches[:steps]
	}

	wanted := make(map[int]bool, len(batches))
	for _, batch := range batches {
		wanted[batch] = true
	}
	out := make([]AppliedRow, 0, len(rows))
	for _, row := range rows {
		if wanted[row.Batch] {
			out = append(out, row)
		}
	}
	return out
}

func execStatements(ctx context.Context, exec query.DB, name string, statements []string) error {
	for i, statement := range statements {
		if _, err := exec.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("vorm/migrate: %s: statement %d (%s): %w",
				name, i+1, truncate(statement, 160), err)
		}
	}
	return nil
}

func (r *Runner) ready() error {
	if r == nil || r.db == nil {
		return errNoDB
	}
	return nil
}

func (r *Runner) logf(format string, args ...any) {
	if r.opts.Logger != nil {
		r.opts.Logger.Printf(format, args...)
	}
}

// truncate collapses a statement onto one line for error messages.
func truncate(s string, limit int) string {
	s = normalizeSpace(s)
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}
