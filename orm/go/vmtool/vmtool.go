package vmtool

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const installURL = "https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.sh"

// Ensure locates the vm binary. If installIfMissing and vm is not found,
// runs the official install script (curl | bash) then re-resolves.
func Ensure(installIfMissing bool) (string, error) {
	if p, err := exec.LookPath("vm"); err == nil {
		return p, nil
	}
	candidates := []string{"./vm", "../vm", "../../vm", "vm"}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs, nil
		}
	}
	if !installIfMissing {
		return "", fmt.Errorf("vm not found in PATH — install Vorzela Migrate or set EnsureVM=false")
	}
	fmt.Fprintln(os.Stderr, "vorm: vm not found — installing Vorzela Migrate in the background…")
	cmd := exec.Command("bash", "-c", "curl -fsSL "+installURL+" | bash")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("auto-install vm failed: %w (install manually: %s)", err, installURL)
	}
	if p, err := exec.LookPath("vm"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("vm installed but still not on PATH — open a new shell or add the install location to PATH")
}

// DetectDialect reads DATABASE_URL from the environment or nearest .vm file.
func DetectDialect() string {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = readVMDatabaseURL()
	}
	dsn = strings.ToLower(dsn)
	if strings.HasPrefix(dsn, "mysql://") || strings.Contains(dsn, "@tcp") || strings.Contains(dsn, "tcp(") {
		if strings.Contains(dsn, "mariadb") {
			return "mariadb"
		}
		return "mysql"
	}
	return "postgres"
}

func readVMDatabaseURL() string {
	path := findVMFile()
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if before, after, ok := strings.Cut(line, "="); ok {
			if strings.TrimSpace(before) == "DATABASE_URL" {
				val := strings.TrimSpace(after)
				if i := strings.Index(val, " #"); i >= 0 {
					val = strings.TrimSpace(val[:i])
				}
				return val
			}
		}
	}
	return ""
}

func findVMFile() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		cand := filepath.Join(dir, ".vm")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
