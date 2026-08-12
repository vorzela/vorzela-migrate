package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// prereqFile pairs a prerequisite SQL file with the sidecar file caching its
// hash. vm keeps these sidecars next to the SQL so unchanged prerequisites are
// not re-applied on every run.
type prereqFile struct {
	sql      string
	hashFile string
}

// prereqFiles is ordered: functions and enums may depend on extensions.
var prereqFiles = []prereqFile{
	{"extensions.sql", ".vm_extensions_hash"},
	{"functions.sql", ".vm_functions_hash"},
	{"enums.sql", ".vm_enums_hash"},
}

// PrereqStatements turns a prerequisite file into statements that are safe to
// re-run. Enum types get a create-or-add-values block because a plain CREATE
// TYPE fails the second time the file is applied, and extensions get the
// IF NOT EXISTS they may be missing.
func PrereqStatements(name, content string) []string {
	if name == EnumsFile {
		enums, _ := ParseEnums(content)
		out := make([]string, 0, len(enums))
		for _, e := range enums {
			if sql := EnumSyncSQL(e); sql != "" {
				out = append(out, sql)
			}
		}
		return out
	}

	stmts := SplitStatements(content)
	if name == ExtensionsFile {
		for i, s := range stmts {
			stmts[i] = normalizeExtension(s)
		}
	}
	return stmts
}

// runPrereq applies the PostgreSQL prerequisite files that changed since the
// last run. Commented-out entries are disabled entries and are simply skipped.
func (r *Runner) runPrereq(ctx context.Context) ([]StepResult, error) {
	var steps []StepResult

	for _, p := range prereqFiles {
		path := filepath.Join(r.opts.Dir, p.sql)
		content, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return steps, fmt.Errorf("vorm/migrate: read %s: %w", path, err)
		}

		sum := ChecksumBytes(content)
		hashPath := filepath.Join(r.opts.Dir, p.hashFile)
		if cached, err := os.ReadFile(hashPath); err == nil && string(cached) == sum {
			steps = append(steps, StepResult{Name: p.sql, Skipped: true, Reason: "unchanged"})
			continue
		}

		stmts := PrereqStatements(p.sql, string(content))
		if len(stmts) == 0 {
			steps = append(steps, StepResult{Name: p.sql, Skipped: true, Reason: "nothing enabled"})
			continue
		}
		if r.opts.DryRun {
			steps = append(steps, StepResult{Name: p.sql, Skipped: true, Reason: "dry run"})
			continue
		}

		start := time.Now()
		if err := execStatements(ctx, r.db, p.sql, stmts); err != nil {
			steps = append(steps, StepResult{Name: p.sql, Duration: time.Since(start), Err: err})
			return steps, err
		}
		if err := os.WriteFile(hashPath, []byte(sum), 0o644); err != nil {
			r.logf("migrate: warning: write %s: %v", hashPath, err)
		}
		steps = append(steps, StepResult{Name: p.sql, Applied: true, Duration: time.Since(start)})
		r.logf("migrate: applied %s", p.sql)
	}

	return steps, nil
}
