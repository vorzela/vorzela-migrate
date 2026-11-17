package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/vorzela/vorzela-migrate/v2/internal/migration"
)

const maxCommandOutput = 2 << 20 // 2 MiB per stream

// Runner executes the vm binary with a strict, non-interactive environment.
type Runner struct {
	vmPath string
}

// RunResult captures a completed vm invocation.
type RunResult struct {
	Args       []string `json:"args"`
	ExitCode   int      `json:"exitCode"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
	DurationMS int64    `json:"durationMs"`
	TimedOut   bool     `json:"timedOut"`
	Truncated  bool     `json:"truncated"`
}

// Success reports a clean exit.
func (r RunResult) Success() bool { return r.ExitCode == 0 && !r.TimedOut }

// DisplayCommand returns a copy-pasteable command without secrets.
func (r RunResult) DisplayCommand() string {
	parts := make([]string, 0, len(r.Args)+1)
	parts = append(parts, "vm")
	parts = append(parts, r.Args...)
	return strings.Join(parts, " ")
}

func newRunner(vmPath string) (*Runner, error) {
	vmPath = strings.TrimSpace(vmPath)
	if vmPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("locate vm binary: %w", err)
		}
		vmPath = exe
	}
	if !filepath.IsAbs(vmPath) {
		found, err := exec.LookPath(vmPath)
		if err != nil {
			return nil, fmt.Errorf("vm binary %q not found: %w", vmPath, err)
		}
		vmPath = found
	}
	info, err := os.Stat(vmPath)
	if err != nil {
		return nil, fmt.Errorf("vm binary: %w", err)
	}
	if info.IsDir() {
		return nil, errors.New("vm binary path is a directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return nil, errors.New("vm binary is not executable")
	}
	return &Runner{vmPath: vmPath}, nil
}

// Run executes vm inside projectDir. The database password is injected through
// the child environment (never argv) so it does not appear in process listings.
func (r *Runner) Run(ctx context.Context, projectDir string, conn *Connection, password string, args ...string) (RunResult, error) {
	result := RunResult{Args: append([]string(nil), args...)}
	start := time.Now()

	cmd := exec.CommandContext(ctx, r.vmPath, args...)
	cmd.Dir = projectDir
	cmd.WaitDelay = 5 * time.Second
	env, err := buildChildEnv(conn, password)
	if err != nil {
		return result, err
	}
	cmd.Env = env

	stdout := &limitedBuffer{limit: maxCommandOutput}
	stderr := &limitedBuffer{limit: maxCommandOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()
	result.DurationMS = time.Since(start).Milliseconds()
	result.Stdout = stripANSI(redactSecrets(stdout.String(), password))
	result.Stderr = stripANSI(redactSecrets(stderr.String(), password))
	result.Truncated = stdout.truncated || stderr.truncated

	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.ExitCode = -1
		return result, ctx.Err()
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, runErr
	}
	result.ExitCode = 0
	return result, nil
}

func buildChildEnv(conn *Connection, password string) ([]string, error) {
	blocked := map[string]bool{
		"DATABASE_URL":       true,
		"MIGRATION_PATH":     true,
		"VM_NON_INTERACTIVE": true,
		"VM_NO_COLOR":        true,
		"VM_DIALECT":         true,
		"NO_COLOR":           true,
		"TERM":               true,
	}
	env := make([]string, 0, len(os.Environ())+8)
	for _, entry := range os.Environ() {
		key := entry
		if idx := strings.IndexByte(entry, '='); idx >= 0 {
			key = entry[:idx]
		}
		if blocked[key] {
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		"VM_NON_INTERACTIVE=1",
		"VM_NO_COLOR=1",
		"NO_COLOR=1",
		"TERM=dumb",
		"CI=1",
	)
	if conn != nil {
		dsn, err := buildDSN(conn, password)
		if err != nil {
			return nil, err
		}
		env = append(env, "DATABASE_URL="+dsn)
		engine, _ := normalizeEngine(conn.Engine)
		if engine == "mariadb" {
			env = append(env, "VM_DIALECT=mariadb")
		} else if engine == "mysql" {
			env = append(env, "VM_DIALECT=mysql")
		} else {
			env = append(env, "VM_DIALECT=postgres")
		}
	}
	return env, nil
}

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len() >= b.limit {
		b.truncated = true
		return len(p), nil
	}
	remain := b.limit - b.buf.Len()
	if len(p) > remain {
		b.truncated = true
		_, _ = b.buf.Write(p[:remain])
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string { return b.buf.String() }

var urlPasswordRe = regexp.MustCompile(`(://[^:/@\s]+:)[^@\s]+@`)

func redactSecrets(text, password string) string {
	if text == "" {
		return text
	}
	if password != "" && len(password) >= 4 {
		text = strings.ReplaceAll(text, password, "***")
		text = strings.ReplaceAll(text, url.QueryEscape(password), "***")
	}
	text = urlPasswordRe.ReplaceAllString(text, "${1}***@")
	return text
}

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func stripANSI(text string) string {
	return ansiEscapeRe.ReplaceAllString(text, "")
}

// Check runs `vm check --json`. It works without a database connection.
func (r *Runner) Check(ctx context.Context, projectDir string, conn *Connection, password string) (*migration.SafetyReport, RunResult, error) {
	res, err := r.Run(ctx, projectDir, conn, password, "check", "--json")
	if err != nil {
		return nil, res, err
	}
	report := &migration.SafetyReport{}
	if err := decodeJSONOutput(res.Stdout, report); err != nil {
		return nil, res, fmt.Errorf("parse vm check output: %w", err)
	}
	return report, res, nil
}

// Status runs `vm status --json`. It requires a reachable database.
func (r *Runner) Status(ctx context.Context, projectDir string, conn *Connection, password string) (*migration.StatusReport, RunResult, error) {
	res, err := r.Run(ctx, projectDir, conn, password, "status", "--json")
	if err != nil {
		return nil, res, err
	}
	report := &migration.StatusReport{}
	if err := decodeJSONOutput(res.Stdout, report); err != nil {
		if res.ExitCode != 0 {
			return nil, res, fmt.Errorf("vm status failed: %s", strings.TrimSpace(res.Stderr))
		}
		return nil, res, fmt.Errorf("parse vm status output: %w", err)
	}
	return report, res, nil
}

func decodeJSONOutput(output string, target any) error {
	output = strings.TrimSpace(output)
	if output == "" {
		return errors.New("empty output")
	}
	dec := json.NewDecoder(strings.NewReader(output))
	if err := dec.Decode(target); err != nil {
		return err
	}
	return nil
}
