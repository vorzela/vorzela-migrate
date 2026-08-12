package migrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vorzela/vorm/query"
)

// upDown builds a migration file in the layout vm scaffolds.
func upDown(up, down string) string {
	return "-- ⬆ Up (Run when migrating forward)\n" + up + "\n\n-- ⬇ Down (Run when rolling back)\n" + down + "\n"
}

// testRunner returns a runner over a temp directory holding table-creating
// migrations named 100_a.sql, 200_b.sql and 300_c.sql.
func testRunner(t *testing.T, mutate func(*Options)) (*Runner, *fakeDB, string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		writeFile(t, dir, timestampFor(name)+"_"+name+".sql",
			upDown("CREATE TABLE "+name+" (id INT);", "DROP TABLE "+name+";"))
	}

	opts := Options{Dir: dir, SkipLock: true, VerifyChecksum: true}
	if mutate != nil {
		mutate(&opts)
	}
	db := newFakeDB()
	return New(db, opts), db, dir
}

func timestampFor(name string) string {
	switch name {
	case "a":
		return "100"
	case "b":
		return "200"
	default:
		return "300"
	}
}

func mustUp(t *testing.T, r *Runner) *Report {
	t.Helper()
	report, err := r.Up(context.Background())
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	return report
}

func TestUpAppliesEverythingInOneBatch(t *testing.T) {
	r, db, _ := testRunner(t, nil)

	report := mustUp(t, r)

	if report.Applied != 3 || report.Failed != 0 {
		t.Fatalf("applied = %d, failed = %d, want 3 and 0", report.Applied, report.Failed)
	}
	if report.Batch != 1 {
		t.Errorf("batch = %d, want 1", report.Batch)
	}
	if got, want := db.names(), []string{"100_a.sql", "200_b.sql", "300_c.sql"}; !reflect.DeepEqual(got, want) {
		t.Errorf("recorded migrations = %v, want %v", got, want)
	}
	for _, row := range db.snapshot() {
		if row.batch != 1 {
			t.Errorf("%s: batch = %d, want 1", row.name, row.batch)
		}
		if len(row.checksum) != 64 {
			t.Errorf("%s: checksum = %q, want 64 hex characters", row.name, row.checksum)
		}
	}

	want := []string{
		"BEGIN", "CREATE TABLE a (id INT);", "COMMIT",
		"BEGIN", "CREATE TABLE b (id INT);", "COMMIT",
		"BEGIN", "CREATE TABLE c (id INT);", "COMMIT",
	}
	if got := db.statements(); !reflect.DeepEqual(got, want) {
		t.Errorf("executed = %v, want %v", got, want)
	}
	for _, step := range report.Steps {
		if !step.Transactional {
			t.Errorf("%s: Transactional = false, want true", step.Name)
		}
	}
}

func TestUpIsIdempotent(t *testing.T) {
	r, db, _ := testRunner(t, nil)
	mustUp(t, r)

	report := mustUp(t, r)
	if report.Applied != 0 || len(report.Steps) != 0 {
		t.Errorf("second Up applied %d steps, want none", report.Applied)
	}
	if report.Batch != 0 {
		t.Errorf("batch = %d, want 0 when nothing is pending", report.Batch)
	}
	if len(db.snapshot()) != 3 {
		t.Errorf("tracking table holds %d rows, want 3", len(db.snapshot()))
	}
}

func TestUpStepLimitsAndAdvancesBatch(t *testing.T) {
	r, db, _ := testRunner(t, func(o *Options) { o.Step = 2 })

	first := mustUp(t, r)
	if first.Applied != 2 || first.Batch != 1 {
		t.Fatalf("first Up: applied = %d, batch = %d, want 2 and 1", first.Applied, first.Batch)
	}
	if got, want := db.names(), []string{"100_a.sql", "200_b.sql"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recorded = %v, want %v", got, want)
	}

	second := mustUp(t, r)
	if second.Applied != 1 || second.Batch != 2 {
		t.Fatalf("second Up: applied = %d, batch = %d, want 1 and 2", second.Applied, second.Batch)
	}
	rows := db.snapshot()
	if rows[2].name != "300_c.sql" || rows[2].batch != 2 {
		t.Errorf("third row = %+v, want 300_c.sql in batch 2", rows[2])
	}
}

func TestUpSkipsFileWithoutUpSection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "100_a.sql", "CREATE TABLE a (id INT);\n")
	db := newFakeDB()
	r := New(db, Options{Dir: dir, SkipLock: true})

	report := mustUp(t, r)
	if report.Applied != 0 {
		t.Fatalf("applied = %d, want 0", report.Applied)
	}
	if len(report.Steps) != 1 || !report.Steps[0].Skipped || report.Steps[0].Reason != "no Up section" {
		t.Fatalf("steps = %+v, want one skip explaining the missing Up section", report.Steps)
	}
	if len(db.statements()) != 0 {
		t.Errorf("executed %v, want nothing", db.statements())
	}
}

func TestUpDryRunTouchesNothing(t *testing.T) {
	r, db, _ := testRunner(t, func(o *Options) { o.DryRun = true })

	report := mustUp(t, r)
	if report.Applied != 0 || len(report.Steps) != 3 {
		t.Fatalf("applied = %d with %d steps, want 0 and 3", report.Applied, len(report.Steps))
	}
	for _, step := range report.Steps {
		if !step.Skipped || step.Reason != "dry run" {
			t.Errorf("%s: %+v, want a dry run skip", step.Name, step)
		}
	}
	if len(db.statements()) != 0 {
		t.Errorf("executed %v, want nothing", db.statements())
	}
	if len(db.snapshot()) != 0 {
		t.Errorf("recorded %v, want nothing", db.names())
	}
}

func TestUpRunsConcurrentIndexOutsideTransaction(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "100_a.sql", upDown("CREATE TABLE a (id INT);", "DROP TABLE a;"))
	writeFile(t, dir, "200_index.sql", upDown(
		"CREATE INDEX CONCURRENTLY a_id_idx ON a (id);",
		"DROP INDEX CONCURRENTLY a_id_idx;"))
	db := newFakeDB()
	r := New(db, Options{Dir: dir, SkipLock: true})

	report := mustUp(t, r)
	if report.Applied != 2 {
		t.Fatalf("applied = %d, want 2", report.Applied)
	}
	if !report.Steps[0].Transactional {
		t.Error("plain migration should run in a transaction")
	}
	if report.Steps[1].Transactional {
		t.Error("CONCURRENTLY migration should not run in a transaction")
	}

	want := []string{
		"BEGIN", "CREATE TABLE a (id INT);", "COMMIT",
		"CREATE INDEX CONCURRENTLY a_id_idx ON a (id);",
	}
	if got := db.statements(); !reflect.DeepEqual(got, want) {
		t.Errorf("executed = %v, want %v", got, want)
	}
}

func TestUpWithoutBeginnerSkipsTransaction(t *testing.T) {
	r, db, _ := testRunner(t, nil)
	r.WithTx(nil)

	mustUp(t, r)
	for _, statement := range db.statements() {
		if statement == "BEGIN" {
			t.Fatalf("executed %v, want no transaction", db.statements())
		}
	}
	if len(db.snapshot()) != 3 {
		t.Errorf("recorded %v, want three migrations", db.names())
	}
}

func TestUpStopsAndRollsBackOnFailure(t *testing.T) {
	r, db, _ := testRunner(t, nil)
	boom := errors.New("syntax error at or near \"TABLE\"")
	db.failOn["CREATE TABLE b"] = boom

	report, err := r.Up(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("Up error = %v, want it to wrap %v", err, boom)
	}
	if !strings.Contains(err.Error(), "vorm/migrate: 200_b.sql: statement 1") {
		t.Errorf("error = %q, want the file and statement number", err)
	}
	if report.Applied != 1 || report.Failed != 1 {
		t.Errorf("applied = %d, failed = %d, want 1 and 1", report.Applied, report.Failed)
	}
	if len(report.Steps) != 2 {
		t.Fatalf("steps = %d, want 2 (the run stops at the failure)", len(report.Steps))
	}
	if got, want := db.names(), []string{"100_a.sql"}; !reflect.DeepEqual(got, want) {
		t.Errorf("recorded = %v, want %v: the failed migration must not be recorded", got, want)
	}
	if got := db.statements(); got[len(got)-1] != "ROLLBACK" {
		t.Errorf("last statement = %q, want ROLLBACK", got[len(got)-1])
	}
}

func TestUpDetectsChecksumMismatch(t *testing.T) {
	r, db, dir := testRunner(t, nil)
	mustUp(t, r)

	writeFile(t, dir, "200_b.sql", upDown("CREATE TABLE b (id BIGINT);", "DROP TABLE b;"))

	_, err := r.Up(context.Background())
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Up error = %v, want ErrChecksumMismatch", err)
	}
	if !strings.Contains(err.Error(), "200_b.sql") {
		t.Errorf("error = %q, want the offending file name", err)
	}

	before := db.snapshot()
	r.opts.VerifyChecksum = false
	if _, err := r.Up(context.Background()); err != nil {
		t.Fatalf("Up without verification: %v", err)
	}
	if after := db.snapshot(); !reflect.DeepEqual(before, after) {
		t.Error("disabling verification should not rewrite stored checksums")
	}
}

func TestUpForceAcceptsAndRewritesChecksums(t *testing.T) {
	r, db, dir := testRunner(t, func(o *Options) { o.Force = true })
	mustUp(t, r)

	edited := upDown("CREATE TABLE b (id BIGINT);", "DROP TABLE b;")
	writeFile(t, dir, "200_b.sql", edited)
	want := ChecksumBytes([]byte(edited))

	if _, err := r.Up(context.Background()); err != nil {
		t.Fatalf("Up with Force: %v", err)
	}
	for _, row := range db.snapshot() {
		if row.name == "200_b.sql" && row.checksum != want {
			t.Errorf("stored checksum = %q, want the current file hash %q", row.checksum, want)
		}
	}
	if _, err := r.Up(context.Background()); err != nil {
		t.Fatalf("Up after Force should verify cleanly: %v", err)
	}
}

func TestUpReportsMissingFile(t *testing.T) {
	r, _, dir := testRunner(t, nil)
	mustUp(t, r)

	if err := os.Remove(filepath.Join(dir, "200_b.sql")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, err := r.Up(context.Background())
	if !errors.Is(err, ErrMissingFile) {
		t.Fatalf("Up error = %v, want ErrMissingFile", err)
	}
	if !strings.Contains(err.Error(), "200_b.sql") {
		t.Errorf("error = %q, want the missing file name", err)
	}
}

func TestUpIgnoresLegacyRowWithoutChecksum(t *testing.T) {
	r, db, dir := testRunner(t, nil)
	db.rows = append(db.rows, trackRow{id: 1, name: "100_a.sql", batch: 1})
	db.nextID = 1
	writeFile(t, dir, "100_a.sql", upDown("CREATE TABLE a (id BIGINT);", "DROP TABLE a;"))

	report := mustUp(t, r)
	if report.Applied != 2 {
		t.Fatalf("applied = %d, want 2: the edited legacy row must not block the run", report.Applied)
	}
}

func TestDownRollsBackLastBatch(t *testing.T) {
	r, db, _ := testRunner(t, func(o *Options) { o.Step = 1 })
	mustUp(t, r) // batch 1: 100_a
	r.opts.Step = 0
	mustUp(t, r) // batch 2: 200_b and 300_c
	db.log = nil

	report, err := r.Down(context.Background(), 1)
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if report.Applied != 2 {
		t.Fatalf("rolled back %d, want 2 (the whole batch)", report.Applied)
	}
	if got, want := db.statements(), []string{"DROP TABLE c;", "DROP TABLE b;"}; !reflect.DeepEqual(got, want) {
		t.Errorf("executed = %v, want %v (newest first, no transaction)", got, want)
	}
	if got, want := db.names(), []string{"100_a.sql"}; !reflect.DeepEqual(got, want) {
		t.Errorf("remaining = %v, want %v", got, want)
	}
}

func TestDownStepsCountBatchesNotFiles(t *testing.T) {
	r, db, _ := testRunner(t, func(o *Options) { o.Step = 1 })
	mustUp(t, r)
	r.opts.Step = 0
	mustUp(t, r)

	report, err := r.Down(context.Background(), 2)
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if report.Applied != 3 {
		t.Fatalf("rolled back %d, want 3", report.Applied)
	}
	if len(db.snapshot()) != 0 {
		t.Errorf("remaining = %v, want none", db.names())
	}
}

func TestDownDefaultsToOneBatch(t *testing.T) {
	r, db, _ := testRunner(t, nil)
	mustUp(t, r)

	if _, err := r.Down(context.Background(), 0); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if len(db.snapshot()) != 0 {
		t.Errorf("remaining = %v, want none: one Up is one batch", db.names())
	}
}

func TestDownAllRevertsNewestFirst(t *testing.T) {
	r, db, _ := testRunner(t, nil)
	mustUp(t, r)
	db.log = nil

	report, err := r.DownAll(context.Background())
	if err != nil {
		t.Fatalf("DownAll: %v", err)
	}
	if report.Applied != 3 {
		t.Fatalf("rolled back %d, want 3", report.Applied)
	}
	want := []string{"DROP TABLE c;", "DROP TABLE b;", "DROP TABLE a;"}
	if got := db.statements(); !reflect.DeepEqual(got, want) {
		t.Errorf("executed = %v, want %v", got, want)
	}
}

func TestDownAllOnEmptyTable(t *testing.T) {
	r, _, _ := testRunner(t, nil)
	report, err := r.DownAll(context.Background())
	if err != nil {
		t.Fatalf("DownAll on empty table: %v", err)
	}
	if report.Applied != 0 || len(report.Steps) != 0 {
		t.Errorf("report = %+v, want nothing rolled back", report)
	}
}

func TestDownByName(t *testing.T) {
	r, db, _ := testRunner(t, nil)
	mustUp(t, r)
	db.log = nil

	report, err := r.DownByName(context.Background(), "_B")
	if err != nil {
		t.Fatalf("DownByName: %v", err)
	}
	if report.Applied != 1 {
		t.Fatalf("rolled back %d, want 1", report.Applied)
	}
	if got, want := db.statements(), []string{"DROP TABLE b;"}; !reflect.DeepEqual(got, want) {
		t.Errorf("executed = %v, want %v", got, want)
	}
	if got, want := db.names(), []string{"100_a.sql", "300_c.sql"}; !reflect.DeepEqual(got, want) {
		t.Errorf("remaining = %v, want %v", got, want)
	}
}

func TestDownByNameErrors(t *testing.T) {
	r, _, _ := testRunner(t, nil)
	mustUp(t, r)

	if _, err := r.DownByName(context.Background(), "nope"); !errors.Is(err, ErrNoMatch) {
		t.Errorf("DownByName(nope) error = %v, want ErrNoMatch", err)
	}
	if _, err := r.DownByName(context.Background(), "  "); err == nil {
		t.Error("DownByName with a blank name should fail")
	}
}

func TestDownMissingFile(t *testing.T) {
	r, db, dir := testRunner(t, nil)
	mustUp(t, r)
	if err := os.Remove(filepath.Join(dir, "300_c.sql")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	r.opts.VerifyChecksum = false

	_, err := r.DownAll(context.Background())
	if !errors.Is(err, ErrMissingFile) {
		t.Fatalf("DownAll error = %v, want ErrMissingFile", err)
	}
	if len(db.snapshot()) != 3 {
		t.Errorf("remaining = %v, want all three rows kept", db.names())
	}

	r.opts.Force = true
	report, err := r.DownAll(context.Background())
	if err != nil {
		t.Fatalf("DownAll with Force: %v", err)
	}
	if report.Applied != 2 || len(report.Steps) != 3 {
		t.Fatalf("applied = %d over %d steps, want 2 and 3", report.Applied, len(report.Steps))
	}
	if report.Steps[0].Reason != "file is missing" {
		t.Errorf("first step = %+v, want a missing-file skip", report.Steps[0])
	}
	if got, want := db.names(), []string{"300_c.sql"}; !reflect.DeepEqual(got, want) {
		t.Errorf("remaining = %v, want %v: a skipped rollback keeps its row", got, want)
	}
}

func TestDownSkipsFileWithoutDownSection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "100_a.sql", "-- ⬆ Up\nCREATE TABLE a (id INT);\n")
	db := newFakeDB()
	r := New(db, Options{Dir: dir, SkipLock: true})
	mustUp(t, r)

	report, err := r.DownAll(context.Background())
	if err != nil {
		t.Fatalf("DownAll: %v", err)
	}
	if report.Applied != 0 || report.Steps[0].Reason != "no Down section" {
		t.Fatalf("report = %+v, want a skip explaining the missing Down section", report.Steps)
	}
	if len(db.snapshot()) != 1 {
		t.Error("a migration that cannot be reverted must keep its row")
	}
}

func TestDownDryRun(t *testing.T) {
	r, db, _ := testRunner(t, nil)
	mustUp(t, r)
	db.log = nil
	r.opts.DryRun = true

	report, err := r.DownAll(context.Background())
	if err != nil {
		t.Fatalf("DownAll: %v", err)
	}
	if report.Applied != 0 || len(report.Steps) != 3 {
		t.Fatalf("report = %+v, want three dry run skips", report)
	}
	if len(db.statements()) != 0 || len(db.snapshot()) != 3 {
		t.Error("dry run must not execute or delete anything")
	}
}

func TestFreshReappliesFromBatchOne(t *testing.T) {
	r, db, _ := testRunner(t, func(o *Options) { o.Step = 1 })
	mustUp(t, r)
	r.opts.Step = 0
	mustUp(t, r)
	db.log = nil

	report, err := r.Fresh(context.Background())
	if err != nil {
		t.Fatalf("Fresh: %v", err)
	}
	if report.Batch != 1 {
		t.Errorf("batch = %d, want 1 after a full rollback", report.Batch)
	}
	if report.Applied != 6 {
		t.Errorf("applied = %d, want 6 (three reverted, three applied)", report.Applied)
	}
	if len(report.Steps) != 6 {
		t.Errorf("steps = %d, want 6", len(report.Steps))
	}
	for _, row := range db.snapshot() {
		if row.batch != 1 {
			t.Errorf("%s: batch = %d, want 1", row.name, row.batch)
		}
	}
	statements := db.statements()
	if statements[0] != "DROP TABLE c;" {
		t.Errorf("first statement = %q, want the newest rollback", statements[0])
	}
}

func TestRefreshMatchesFresh(t *testing.T) {
	r, db, _ := testRunner(t, nil)
	mustUp(t, r)

	report, err := r.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if report.Applied != 6 || len(db.snapshot()) != 3 {
		t.Errorf("applied = %d with %d rows, want 6 and 3", report.Applied, len(db.snapshot()))
	}
}

func TestStatus(t *testing.T) {
	r, db, dir := testRunner(t, func(o *Options) { o.Step = 2 })
	mustUp(t, r)

	// 200_b.sql edited after it ran, plus a row whose file never existed.
	writeFile(t, dir, "200_b.sql", upDown("CREATE TABLE b (id BIGINT);", "DROP TABLE b;"))
	db.rows = append(db.rows, trackRow{id: 99, name: "050_gone.sql", batch: 1, checksum: helloSum})

	rows, err := r.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	want := []StatusRow{
		{Name: "100_a.sql", Applied: true, Batch: 1, ChecksumOK: true},
		{Name: "200_b.sql", Applied: true, Batch: 1, ChecksumOK: false},
		{Name: "300_c.sql", Applied: false, Batch: 0, ChecksumOK: true},
		{Name: "050_gone.sql", Applied: true, Batch: 1, ChecksumOK: false, Missing: true},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("Status()\n got: %+v\nwant: %+v", rows, want)
	}
}

func TestStatusLegacyRowReportsOK(t *testing.T) {
	r, db, _ := testRunner(t, nil)
	db.rows = append(db.rows, trackRow{id: 1, name: "100_a.sql", batch: 1})

	rows, err := r.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !rows[0].Applied || !rows[0].ChecksumOK {
		t.Errorf("row = %+v, want applied with no checksum complaint", rows[0])
	}
}

func TestEnsureTableUsesDialectDDL(t *testing.T) {
	db := newFakeDB()
	if err := New(db, Options{Dialect: query.DialectMySQL}).EnsureTable(context.Background()); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	if db.creates != 1 {
		t.Errorf("create statements = %d, want 1", db.creates)
	}
}

func TestPostgresLockLifecycle(t *testing.T) {
	r, db, _ := testRunner(t, func(o *Options) { o.SkipLock = false })

	mustUp(t, r)

	calls := db.lockCalls()
	if len(calls) != 2 {
		t.Fatalf("lock calls = %v, want an acquire and a release", calls)
	}
	if calls[0] != "SELECT pg_try_advisory_lock($1)" {
		t.Errorf("acquire = %q", calls[0])
	}
	if calls[1] != "SELECT pg_advisory_unlock($1)" {
		t.Errorf("release = %q", calls[1])
	}
	if db.lockHeld {
		t.Error("lock still held after Up")
	}
}

func TestMySQLLockLifecycle(t *testing.T) {
	r, db, _ := testRunner(t, func(o *Options) {
		o.SkipLock = false
		o.Dialect = query.DialectMySQL
	})

	mustUp(t, r)

	calls := db.lockCalls()
	if len(calls) != 2 {
		t.Fatalf("lock calls = %v, want an acquire and a release", calls)
	}
	if calls[0] != "SELECT GET_LOCK(?, ?)" {
		t.Errorf("acquire = %q", calls[0])
	}
	if calls[1] != "SELECT RELEASE_LOCK(?)" {
		t.Errorf("release = %q", calls[1])
	}
}

func TestLockRetriesUntilFree(t *testing.T) {
	r, db, _ := testRunner(t, func(o *Options) { o.SkipLock = false })
	db.busyFor = 2

	mustUp(t, r)

	if db.lockAttempts != 3 {
		t.Errorf("lock attempts = %d, want 3", db.lockAttempts)
	}
}

func TestLockTimeout(t *testing.T) {
	r, db, _ := testRunner(t, func(o *Options) {
		o.SkipLock = false
		o.LockTimeout = 150 * time.Millisecond
	})
	db.busyFor = 1 << 30

	_, err := r.Up(context.Background())
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("Up error = %v, want ErrLockTimeout", err)
	}
	if len(db.snapshot()) != 0 {
		t.Error("nothing should be applied when the lock cannot be taken")
	}
}

func TestMySQLLockUnavailable(t *testing.T) {
	r, db, _ := testRunner(t, func(o *Options) {
		o.SkipLock = false
		o.Dialect = query.DialectMySQL
	})
	db.busyFor = 1

	if _, err := r.Up(context.Background()); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("Up error = %v, want ErrLockTimeout: GET_LOCK already waited", err)
	}
}

func TestReleaseSurvivesCancelledContext(t *testing.T) {
	r, db, _ := testRunner(t, func(o *Options) { o.SkipLock = false })

	ctx, cancel := context.WithCancel(context.Background())
	err := r.withLock(ctx, func(context.Context) error {
		cancel()
		return nil
	})
	if err != nil {
		t.Fatalf("withLock: %v", err)
	}
	if db.lockHeld {
		t.Error("lock must be released even when the caller's context is cancelled")
	}
}

func TestSelectBatches(t *testing.T) {
	rows := []AppliedRow{
		{ID: 1, Name: "a", Batch: 1},
		{ID: 2, Name: "b", Batch: 2},
		{ID: 3, Name: "c", Batch: 2},
		{ID: 4, Name: "d", Batch: 5},
	}
	tests := []struct {
		steps int
		want  []string
	}{
		{1, []string{"d"}},
		{2, []string{"b", "c", "d"}},
		{3, []string{"a", "b", "c", "d"}},
		{9, []string{"a", "b", "c", "d"}},
	}
	for _, tc := range tests {
		var got []string
		for _, row := range selectBatches(rows, tc.steps) {
			got = append(got, row.Name)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("selectBatches(steps=%d) = %v, want %v", tc.steps, got, tc.want)
		}
	}
}

func TestNewFillsDefaults(t *testing.T) {
	r := New(newFakeDB(), Options{})
	opts := r.Options()
	if opts.Dir != DefaultDir {
		t.Errorf("Dir = %q, want %q", opts.Dir, DefaultDir)
	}
	if opts.Dialect != query.DialectPostgres {
		t.Errorf("Dialect = %q, want postgres", opts.Dialect)
	}
	if opts.LockTimeout != DefaultLockTimeout {
		t.Errorf("LockTimeout = %s, want %s", opts.LockTimeout, DefaultLockTimeout)
	}
	if r.tx == nil {
		t.Error("New should use a DB that can begin transactions")
	}

	defaults := DefaultOptions()
	if !defaults.VerifyChecksum {
		t.Error("DefaultOptions should verify checksums")
	}
}

func TestNilDatabaseReturnsError(t *testing.T) {
	r := New(nil, Options{})
	for name, call := range map[string]func() error{
		"Up":          func() error { _, err := r.Up(context.Background()); return err },
		"Down":        func() error { _, err := r.Down(context.Background(), 1); return err },
		"Status":      func() error { _, err := r.Status(context.Background()); return err },
		"Applied":     func() error { _, err := r.Applied(context.Background()); return err },
		"EnsureTable": func() error { return r.EnsureTable(context.Background()) },
	} {
		if err := call(); !errors.Is(err, errNoDB) {
			t.Errorf("%s with a nil DB: err = %v, want errNoDB", name, err)
		}
	}
}

func TestLoggerReceivesProgress(t *testing.T) {
	var lines []string
	r, _, _ := testRunner(t, func(o *Options) {
		o.Logger = loggerFunc(func(format string, args ...any) {
			lines = append(lines, format)
		})
	})

	mustUp(t, r)
	if len(lines) != 3 {
		t.Errorf("logged %d lines, want one per applied migration: %v", len(lines), lines)
	}
}

type loggerFunc func(format string, args ...any)

func (f loggerFunc) Printf(format string, args ...any) { f(format, args...) }
