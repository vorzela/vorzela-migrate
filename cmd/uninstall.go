package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/v2/internal/output"
)

var UninstallCommand = &cli.Command{
	Name:  "uninstall",
	Usage: "Remove the per-user vm binary and PATH entry",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "yes",
			Aliases: []string{"y"},
			Usage:   "skip confirmation prompt",
			Value:   false,
		},
		&cli.BoolFlag{
			Name:  "keep-path",
			Usage: "do not remove the PATH export from shell profile / Windows user PATH",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "show what would be removed without deleting anything",
			Value: false,
		},
		&cli.StringFlag{
			Name:  "binary",
			Usage: "explicit path to the vm binary (for custom installs)",
		},
	},
	Action: func(c *cli.Context) error {
		binaryPath, err := findVMBinary(c.String("binary"))
		if err != nil {
			return fmt.Errorf("could not locate vm binary: %w", err)
		}

		fmt.Printf("Found vm at: %s\n", binaryPath)

		if !c.Bool("yes") && !c.Bool("dry-run") {
			if !output.Confirm("Are you sure you want to uninstall vm?") {
				fmt.Println("Uninstall cancelled.")
				return nil
			}
		}

		installDir := filepath.Dir(binaryPath)

		if c.Bool("dry-run") {
			output.Info("DRY RUN - would remove %s", binaryPath)
			output.Info("DRY RUN - would remove vm backup/upgrade files in %s", installDir)
			if !c.Bool("keep-path") {
				output.Info("DRY RUN - would remove PATH entries pointing at %s", installDir)
			}
			return nil
		}

		// Remove the binary. On Windows a running executable cannot be deleted
		// directly; schedule deletion after this process exits.
		if runtime.GOOS == "windows" {
			if err := scheduleWindowsDelete(binaryPath); err != nil {
				return fmt.Errorf("failed to schedule binary removal: %w", err)
			}
			fmt.Printf("Scheduled removal of %s (takes effect after vm exits)\n", binaryPath)
		} else {
			if err := os.Remove(binaryPath); err != nil {
				if os.IsPermission(err) {
					return fmt.Errorf("failed to remove binary %s: %w\nvm is normally per-user; if this is a packaged system copy, remove it with your package manager or run with --binary <path>", binaryPath, err)
				}
				return fmt.Errorf("failed to remove binary: %w", err)
			}
			fmt.Printf("Removed %s\n", binaryPath)
		}

		// Clean up backup and partial upgrade files from the same directory.
		removeInstallArtifacts(installDir)

		// Optionally remove PATH entries from shell profiles / Windows registry.
		if !c.Bool("keep-path") {
			if runtime.GOOS == "windows" {
				cleanWindowsUserPath(installDir)
			} else {
				cleanPathFromProfiles(installDir)
			}
		}

		fmt.Println("\nvm has been uninstalled successfully.")
		if runtime.GOOS == "windows" {
			fmt.Println("The binary will be removed when this terminal exits.")
		}
		fmt.Println("To reinstall, run:")
		fmt.Println("  curl -fsSL https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.sh | bash")
		return nil
	},
}

// findVMBinary returns the path that should be removed.
func findVMBinary(explicit string) (string, error) {
	if explicit != "" {
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("%s: %w", abs, err)
		}
		return abs, nil
	}

	exe, err := os.Executable()
	if err == nil {
		if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
			exe = resolved
		}
		base := strings.ToLower(filepath.Base(exe))
		if base == "vm" || base == "vm.exe" {
			// A `go run` / test binary lives in a temp directory; do not
			// delete that. Everything else that is actually named vm is ours.
			if !strings.Contains(strings.ToLower(exe), "go-build") &&
				!strings.Contains(strings.ToLower(exe), os.TempDir()) {
				return exe, nil
			}
		}
	}

	home, _ := os.UserHomeDir()
	candidates := []string{
		canonicalVMPath(),
		filepath.Join(home, ".local", "bin", "vm"),
		filepath.Join(home, "go", "bin", "vm"),
		filepath.Join(home, "bin", "vm.exe"),
		filepath.Join(home, ".local", "bin", "vm.exe"),
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		candidates = append(candidates, filepath.Join(localAppData, "Programs", "vm", "bin", "vm.exe"))
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("vm binary not found in expected install locations; pass --binary <path>")
}

// removeInstallArtifacts removes vm.bak.*, vm.exe.bak.*, and .vm-upgrade-*.
func removeInstallArtifacts(installDir string) {
	entries, err := os.ReadDir(installDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "vm.bak.") ||
			strings.HasPrefix(name, "vm.exe.bak.") ||
			strings.HasPrefix(name, ".vm-upgrade-") ||
			name == "vm.exe.new" {
			path := filepath.Join(installDir, name)
			if rmErr := os.Remove(path); rmErr == nil {
				fmt.Printf("Removed backup: %s\n", path)
			}
		}
	}
}

func scheduleWindowsDelete(path string) error {
	// ping is used as a portable sleep; `timeout` is unavailable in some
	// non-interactive contexts.
	script := fmt.Sprintf(`ping -n 3 127.0.0.1 >nul & del /f /q "%s"`, path)
	return exec.Command("cmd", "/C", script).Start()
}

// cleanPathFromProfiles removes PATH lines that reference installDir.
func cleanPathFromProfiles(installDir string) {
	home, _ := os.UserHomeDir()
	if home == "" {
		return
	}

	profiles := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".config", "fish", "config.fish"),
	}

	matchDir := strings.ReplaceAll(installDir, home, "$HOME")
	absMatch := installDir

	for _, profile := range profiles {
		info, err := os.Stat(profile)
		if err != nil {
			continue
		}
		content, err := os.ReadFile(profile)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		var kept []string
		removed := 0
		for _, line := range lines {
			hasPath := strings.Contains(line, "PATH")
			refsInstall := strings.Contains(line, matchDir) || strings.Contains(line, absMatch)
			if hasPath && refsInstall {
				removed++
				continue
			}
			kept = append(kept, line)
		}
		if removed == 0 {
			continue
		}

		for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
			kept = kept[:len(kept)-1]
		}
		newContent := strings.Join(kept, "\n") + "\n"

		// Preserve the original file mode instead of forcing 0644.
		if err := os.WriteFile(profile, []byte(newContent), info.Mode().Perm()); err != nil {
			fmt.Printf("Warning: could not update %s: %v\n", profile, err)
		} else {
			fmt.Printf("Removed PATH entry from %s\n", profile)
		}
	}
}

// cleanWindowsUserPath removes installDir from the persistent user PATH.
func cleanWindowsUserPath(installDir string) {
	ps := fmt.Sprintf(
		`$p=[Environment]::GetEnvironmentVariable('Path','User'); `+
			`$parts=($p -split ';') | Where-Object { $_ -and $_.TrimEnd('\') -ne '%s' }; `+
			`[Environment]::SetEnvironmentVariable('Path', ($parts -join ';'), 'User')`,
		strings.ReplaceAll(strings.TrimRight(installDir, `\`), `'`, `''`))

	if err := exec.Command("powershell", "-NoProfile", "-Command", ps).Run(); err != nil {
		fmt.Printf("Warning: could not update Windows user PATH: %v\n", err)
		return
	}
	fmt.Println("Removed install directory from Windows user PATH")
}
