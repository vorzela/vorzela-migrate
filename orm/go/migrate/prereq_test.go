package migrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vorzela/vorm/query"
)

func prereqRunner(t *testing.T, mutate func(*Options)) (*Runner, *fakeDB, string) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "100_a.sql", upDown("CREATE TABLE a (id INT);", "DROP TABLE a;"))
	writeFile(t, dir, "extensions.sql",
		"-- CREATE EXTENSION IF NOT EXISTS postgis;\nCREATE EXTENSION IF NOT EXISTS citext;\n")
	writeFile(t, dir, "functions.sql", "CREATE OR REPLACE FUNCTION touch() RETURNS trigger AS $$\n"+
		"BEGIN\n  NEW.updated_at = NOW();\n  RETURN NEW;\nEND;\n$$ LANGUAGE plpgsql;\n")
	writeFile(t, dir, "enums.sql", "CREATE TYPE status AS ENUM ('on', 'off');\n")

	opts := Options{Dir: dir, SkipLock: true, RunPrereq: true}
	if mutate != nil {
		mutate(&opts)
	}
	db := newFakeDB()
	return New(db, opts), db, dir
}

func TestPrereqAppliesInOrderThenCaches(t *testing.T) {
	r, db, dir := prereqRunner(t, nil)

	report := mustUp(t, r)

	names := make([]string, 0, len(report.Prereq))
	for _, step := range report.Prereq {
		if !step.Applied {
			t.Errorf("%s: %+v, want applied", step.Name, step)
		}
		names = append(names, step.Name)
	}
	want := []string{"extensions.sql", "functions.sql", "enums.sql"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("prereq order = %v, want %v", names, want)
	}

	statements := db.statements()
	if statements[0] != "CREATE EXTENSION IF NOT EXISTS citext;" {
		t.Errorf("first statement = %q, want the enabled extension only", statements[0])
	}
	for _, statement := range statements {
		if statement == "CREATE EXTENSION IF NOT EXISTS postgis;" {
			t.Error("commented-out extensions must not be applied")
		}
	}
	// The dollar-quoted body has to survive as one statement.
	found := false
	for _, statement := range statements {
		if len(statement) > 30 && statement[:30] == "CREATE OR REPLACE FUNCTION tou" {
			found = true
			if !reflect.DeepEqual(SplitStatements(statement), []string{statement}) {
				t.Errorf("function body was split: %q", statement)
			}
		}
	}
	if !found {
		t.Error("functions.sql was not applied")
	}

	for _, sidecar := range []string{".vm_extensions_hash", ".vm_functions_hash", ".vm_enums_hash"} {
		content, err := os.ReadFile(filepath.Join(dir, sidecar))
		if err != nil {
			t.Errorf("read %s: %v", sidecar, err)
			continue
		}
		if len(content) != 64 {
			t.Errorf("%s = %q, want a bare 64 character hash", sidecar, content)
		}
	}

	// A second run has nothing to do.
	db.log = nil
	if _, err := r.DownAll(context.Background()); err != nil {
		t.Fatalf("DownAll: %v", err)
	}
	db.log = nil
	second := mustUp(t, r)
	for _, step := range second.Prereq {
		if !step.Skipped || step.Reason != "unchanged" {
			t.Errorf("%s: %+v, want skipped as unchanged", step.Name, step)
		}
	}
	for _, statement := range db.statements() {
		if statement == "CREATE EXTENSION IF NOT EXISTS citext;" {
			t.Error("unchanged prerequisites should not be re-applied")
		}
	}
}

func TestPrereqReappliesAfterEdit(t *testing.T) {
	r, db, dir := prereqRunner(t, nil)
	mustUp(t, r)

	writeFile(t, dir, "extensions.sql", "CREATE EXTENSION IF NOT EXISTS citext;\nCREATE EXTENSION IF NOT EXISTS hstore;\n")
	db.log = nil

	report := mustUp(t, r)
	for _, step := range report.Prereq {
		if step.Name == "extensions.sql" && !step.Applied {
			t.Errorf("extensions.sql = %+v, want re-applied after the edit", step)
		}
	}
	if got := db.statements(); len(got) != 2 || got[1] != "CREATE EXTENSION IF NOT EXISTS hstore;" {
		t.Errorf("executed = %v, want both extensions", got)
	}
}

func TestPrereqSkipsFilesWithNothingEnabled(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "extensions.sql", "-- CREATE EXTENSION IF NOT EXISTS citext;\n-- nothing enabled\n")
	db := newFakeDB()
	r := New(db, Options{Dir: dir, SkipLock: true, RunPrereq: true})

	report := mustUp(t, r)
	if len(report.Prereq) != 1 {
		t.Fatalf("prereq steps = %+v, want one", report.Prereq)
	}
	if !report.Prereq[0].Skipped || report.Prereq[0].Reason != "nothing enabled" {
		t.Errorf("step = %+v, want skipped with nothing enabled", report.Prereq[0])
	}
	if len(db.statements()) != 0 {
		t.Errorf("executed %v, want nothing", db.statements())
	}
	if _, err := os.Stat(filepath.Join(dir, ".vm_extensions_hash")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a file with nothing enabled should not write a hash sidecar")
	}
}

func TestPrereqSkippedOnMySQL(t *testing.T) {
	r, db, dir := prereqRunner(t, func(o *Options) { o.Dialect = query.DialectMySQL })

	report := mustUp(t, r)
	if len(report.Prereq) != 0 {
		t.Errorf("prereq = %+v, want none on MySQL", report.Prereq)
	}
	if got := db.statements(); len(got) != 3 || got[1] != "CREATE TABLE a (id INT);" {
		t.Errorf("executed = %v, want only the migration", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".vm_extensions_hash")); !errors.Is(err, os.ErrNotExist) {
		t.Error("MySQL runs should not write prerequisite hashes")
	}
}

func TestPrereqDryRun(t *testing.T) {
	r, db, dir := prereqRunner(t, func(o *Options) { o.DryRun = true })

	report := mustUp(t, r)
	for _, step := range report.Prereq {
		if !step.Skipped || step.Reason != "dry run" {
			t.Errorf("%s: %+v, want a dry run skip", step.Name, step)
		}
	}
	if len(db.statements()) != 0 {
		t.Errorf("executed %v, want nothing", db.statements())
	}
	if _, err := os.Stat(filepath.Join(dir, ".vm_enums_hash")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a dry run should not write hash sidecars")
	}
}

func TestPrereqFailureStopsRun(t *testing.T) {
	r, db, dir := prereqRunner(t, nil)
	boom := errors.New("permission denied to create extension")
	db.failOn["CREATE EXTENSION"] = boom

	report, err := r.Up(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("Up error = %v, want it to wrap %v", err, boom)
	}
	if len(report.Prereq) != 1 || report.Prereq[0].Err == nil {
		t.Errorf("prereq = %+v, want the failure recorded", report.Prereq)
	}
	if len(report.Steps) != 0 || len(db.snapshot()) != 0 {
		t.Error("no migration should run when a prerequisite fails")
	}
	if _, err := os.Stat(filepath.Join(dir, ".vm_extensions_hash")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a failed prerequisite must not record its hash")
	}
}

func TestPrereqMissingFilesAreFine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "100_a.sql", upDown("CREATE TABLE a (id INT);", "DROP TABLE a;"))
	db := newFakeDB()
	r := New(db, Options{Dir: dir, SkipLock: true, RunPrereq: true})

	report := mustUp(t, r)
	if len(report.Prereq) != 0 {
		t.Errorf("prereq = %+v, want none", report.Prereq)
	}
	if report.Applied != 1 {
		t.Errorf("applied = %d, want 1", report.Applied)
	}
}
