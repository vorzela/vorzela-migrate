package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
)

var UninstallCommand = &cli.Command{
	Name:  "uninstall",
	Usage: "Remove vm from your system",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "yes",
			Aliases: []string{"y"},
			Usage:   "skip confirmation prompt",
			Value:   false,
		},
		&cli.BoolFlag{
			Name:  "keep-path",
			Usage: "do not remove the PATH export from shell profile files",
			Value: false,
		},
	},
	Action: func(c *cli.Context) error {
		// Locate the binary — prefer the one that is actually running, then
		// fall back to the standard install locations written by install.sh.
		binaryPath, err := findVMBinary()
		if err != nil {
			return fmt.Errorf("could not locate vm binary: %w", err)
		}

		fmt.Printf("Found vm at: %s\n", binaryPath)

		if !c.Bool("yes") {
			fmt.Print("Are you sure you want to uninstall vm? [yes/no]: ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "yes" && answer != "y" {
				fmt.Println("Uninstall cancelled.")
				return nil
			}
		}

		// Remove the binary
		if err := os.Remove(binaryPath); err != nil {
			return fmt.Errorf("failed to remove binary: %w", err)
		}
		fmt.Printf("Removed %s\n", binaryPath)

		// Clean up any .bak files from upgrade in the same directory
		installDir := filepath.Dir(binaryPath)
		if entries, err := os.ReadDir(installDir); err == nil {
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), "vm.bak.") {
					bakPath := filepath.Join(installDir, e.Name())
					if rmErr := os.Remove(bakPath); rmErr == nil {
						fmt.Printf("Removed backup: %s\n", bakPath)
					}
				}
			}
		}

		// Optionally remove PATH entries from shell profiles
		if !c.Bool("keep-path") {
			cleanPathFromProfiles(installDir)
		}

		fmt.Println("\nvm has been uninstalled successfully.")
		fmt.Println("To reinstall, run:")
		fmt.Println("  curl -fsSL https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.sh | bash")
		return nil
	},
}

// findVMBinary returns the path that should be removed. It first checks the
// currently running executable; if that looks like a proper install location
// it uses it. Otherwise it falls back to the well-known install paths.
func findVMBinary() (string, error) {
	// os.Executable returns the resolved path of the running process
	exe, err := os.Executable()
	if err == nil {
		// Only trust it when it lives in one of the standard dirs
		dir := filepath.Dir(exe)
		base := filepath.Base(exe)
		if base == "vm" && (strings.Contains(dir, ".local/bin") || dir == "/usr/local/bin" || dir == "/usr/bin") {
			return exe, nil
		}
	}

	// Fallback: check standard install locations
	candidates := []string{
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "vm"),
		"/usr/local/bin/vm",
		"/usr/bin/vm",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("vm binary not found in expected install locations (%s)",
		strings.Join(candidates, ", "))
}

// cleanPathFromProfiles removes the line 'export PATH="$HOME/.local/bin:$PATH"'
// (or equivalent) that install.sh added to shell profiles.
func cleanPathFromProfiles(installDir string) {
	home := os.Getenv("HOME")
	if home == "" {
		return
	}

	profiles := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".bash_profile"),
	}

	// Build a pattern to match lines that export our install directory
	matchDir := strings.ReplaceAll(installDir, home, "$HOME")

	for _, profile := range profiles {
		if _, err := os.Stat(profile); err != nil {
			continue // file doesn't exist
		}

		content, err := os.ReadFile(profile)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		var kept []string
		removed := 0
		for _, line := range lines {
			if strings.Contains(line, matchDir) && strings.Contains(line, "PATH") {
				removed++
				continue
			}
			kept = append(kept, line)
		}

		if removed == 0 {
			continue
		}

		// Trim trailing blank lines that may have been left by our removal
		for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
			kept = kept[:len(kept)-1]
		}
		newContent := strings.Join(kept, "\n") + "\n"

		if err := os.WriteFile(profile, []byte(newContent), 0o644); err != nil {
			fmt.Printf("Warning: could not update %s: %v\n", profile, err)
		} else {
			fmt.Printf("Removed PATH entry from %s\n", profile)
		}
	}
}
