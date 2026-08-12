package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Finding is one lint issue in a .vorm file.
type Finding struct {
	Line    int
	Key     string
	Message string
	Level   string // error | warning
}

// LintFile validates KEY=value lines in a .vorm file (same rules as the editor).
func LintFile(path string) ([]Finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	known := map[string]bool{}
	for _, k := range Keys() {
		known[k] = true
	}
	// aliases accepted by set()
	for _, k := range []string{
		"GEN_PACKAGE", "PACKAGE_NAME", "GEN_DIR", "QUERIES_DIR", "MODELS_DIR",
		"DSN", "MIGRATIONS_DIR", "SOURCE",
	} {
		known[k] = true
	}

	var findings []Finding
	seen := map[string]int{}
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			findings = append(findings, Finding{Line: lineNo, Level: "warning", Message: "malformed line — expected KEY=VALUE"})
			continue
		}
		key = strings.TrimSpace(strings.ToUpper(key))
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if !known[key] {
			msg := fmt.Sprintf("unknown key %q", key)
			if s := suggestKey(key); s != "" {
				msg += fmt.Sprintf(" — did you mean %s?", s)
			}
			findings = append(findings, Finding{Line: lineNo, Key: key, Level: "error", Message: msg})
			continue
		}
		if prev, ok := seen[key]; ok {
			findings = append(findings, Finding{Line: lineNo, Key: key, Level: "warning", Message: fmt.Sprintf("duplicate key (first on line %d)", prev)})
		} else {
			seen[key] = lineNo
		}
		cfg := Default()
		if err := cfg.Set(key, val); err != nil {
			findings = append(findings, Finding{Line: lineNo, Key: key, Level: "error", Message: err.Error()})
		}
	}
	return findings, sc.Err()
}

func suggestKey(key string) string {
	best, bestD := "", 99
	for _, k := range Keys() {
		d := levenshtein(strings.ToUpper(key), k)
		if d < bestD && d <= 3 {
			bestD, best = d, k
		}
	}
	return best
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	m, n := len(ra), len(rb)
	dp := make([]int, n+1)
	for j := 0; j <= n; j++ {
		dp[j] = j
	}
	for i := 1; i <= m; i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= n; j++ {
			tmp := dp[j]
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			ins := dp[j-1] + 1
			del := dp[j] + 1
			sub := prev + cost
			dp[j] = min(ins, del, sub)
			prev = tmp
		}
	}
	return dp[n]
}

// LintPath lints path or ./.vorm when empty.
func LintPath(path string) ([]Finding, error) {
	if path == "" {
		path = filepath.Join(".", DefaultFile)
	}
	return LintFile(path)
}

// FormatFindings prints lint results.
func FormatFindings(fs []Finding) string {
	if len(fs) == 0 {
		return "vorm config: ok\n"
	}
	var b strings.Builder
	for _, f := range fs {
		fmt.Fprintf(&b, "%s: line %d: %s\n", f.Level, f.Line, f.Message)
	}
	return b.String()
}

func HasErrors(fs []Finding) bool {
	for _, f := range fs {
		if f.Level == "error" {
			return true
		}
	}
	return false
}
