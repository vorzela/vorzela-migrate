package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/output"
	"github.com/vorzela/vorzela-migrate/internal/version"
)

var UpgradeCommand = &cli.Command{
	Name:  "upgrade",
	Usage: "Upgrade vm to the latest GitHub release",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "check",
			Usage: "only check whether a newer version exists",
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "reinstall even when the installed version is already current",
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "show what would be downloaded without replacing the binary",
		},
	},
	Action: func(c *cli.Context) error {
		newVersion, available, err := version.CheckForUpdate()
		if err != nil {
			output.Error("Could not check for updates: %v", err)
			output.Info("Your installed version is %s", version.CurrentVersion)
			output.Info("If a newer release exists, install with:")
			output.Info("  curl -fsSL https://github.com/vorzela/vorzela-migrate/raw/main/install.sh | bash")
			return fmt.Errorf("update check failed: %w", err)
		}

		if !available && !c.Bool("force") {
			output.Success("You're already on the latest version (%s)", version.CurrentVersion)
			return nil
		}
		if c.Bool("check") {
			if available {
				output.Info("Update available: %s → %s", version.CurrentVersion, newVersion)
			} else {
				output.Success("Already up to date (%s)", version.CurrentVersion)
			}
			return nil
		}

		target := newVersion
		if target == "" {
			target = version.CurrentVersion
		}
		output.Info("Upgrading from %s to %s...", version.CurrentVersion, target)

		asset := releaseAssetName(runtime.GOOS, runtime.GOARCH)
		if asset == "" {
			output.Warning("No pre-built binary is published for %s/%s; falling back to the install script", runtime.GOOS, runtime.GOARCH)
			if c.Bool("dry-run") {
				output.Info("DRY RUN - would run the install script")
				return nil
			}
			if err := runInstallScript(target); err != nil {
				output.Error("Failed to upgrade: %v", err)
				return err
			}
			output.Success("Upgrade completed. Run 'vm --version' to verify")
			return nil
		}

		url := fmt.Sprintf("https://github.com/vorzela/vorzela-migrate/releases/download/%s/%s", target, asset)
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("could not locate the running vm binary: %w", err)
		}
		exe, _ = filepath.EvalSymlinks(exe)

		output.Info("Downloading %s", url)
		if c.Bool("dry-run") {
			output.Info("DRY RUN - would replace %s", exe)
			return nil
		}

		if runtime.GOOS == "windows" {
			// A running .exe cannot be replaced on Windows. Let the install
			// script stage/verify the binary instead.
			if err := runInstallScript(target); err != nil {
				output.Error("Failed to upgrade: %v", err)
				return err
			}
			output.Success("Upgrade completed. Restart your terminal and run 'vm --version'")
			return nil
		}

		if err := replaceBinaryFromURL(url, exe); err != nil {
			output.Warning("Direct download failed (%v); falling back to the install script", err)
			if err := runInstallScript(target); err != nil {
				output.Error("Failed to upgrade: %v", err)
				return err
			}
		}

		installed, err := binaryVersion(exe)
		if err != nil {
			output.Warning("Could not verify the new binary: %v", err)
			output.Success("Upgrade downloaded. Run 'vm --version' to verify")
			return nil
		}
		if !sameVersion(installed, target) {
			return fmt.Errorf("upgrade verification failed: expected %s, installed binary reports %s", target, installed)
		}
		output.Success("Upgrade completed: %s", installed)
		return nil
	},
}

// releaseAssetName maps Go's OS/arch to the asset names published by the
// release workflow.
func releaseAssetName(goos, goarch string) string {
	switch goos {
	case "linux":
		if goarch == "amd64" || goarch == "arm64" {
			return "vm-linux-" + goarch
		}
	case "darwin":
		if goarch == "amd64" || goarch == "arm64" {
			return "vm-macos-" + goarch
		}
	case "windows":
		if goarch == "amd64" || goarch == "386" {
			return "vm-windows-" + goarch + ".exe"
		}
	}
	return ""
}

// replaceBinaryFromURL downloads a binary next to target and atomically
// replaces it. The temporary file lives in the same directory so rename works
// across filesystems and permissions are preserved.
func replaceBinaryFromURL(url, target string) error {
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".vm-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("write download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close download: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("chmod download: %w", err)
	}

	// Sanity check: a vm binary is much larger than an HTML error page.
	info, err := os.Stat(tmpPath)
	if err != nil {
		return err
	}
	if info.Size() < 1<<20 {
		return fmt.Errorf("downloaded file is only %d bytes; refusing to replace vm", info.Size())
	}

	if err := os.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}

// runInstallScript downloads install.sh/install.ps1 and runs it with
// VM_VERSION pinned to target.
func runInstallScript(target string) error {
	var (
		url  string
		ext  string
		args []string
	)
	if runtime.GOOS == "windows" {
		url = "https://github.com/vorzela/vorzela-migrate/raw/main/install.ps1"
		ext = ".ps1"
		args = []string{"-ExecutionPolicy", "Bypass", "-File"}
	} else {
		url = "https://github.com/vorzela/vorzela-migrate/raw/main/install.sh"
		ext = ".sh"
		args = nil
	}

	script, err := os.CreateTemp("", "vm-install-*"+ext)
	if err != nil {
		return err
	}
	scriptPath := script.Name()
	defer os.Remove(scriptPath)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		script.Close()
		return fmt.Errorf("download install script: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		script.Close()
		return fmt.Errorf("download install script: HTTP %d", resp.StatusCode)
	}
	if _, err := io.Copy(script, resp.Body); err != nil {
		script.Close()
		return fmt.Errorf("write install script: %w", err)
	}
	if err := script.Close(); err != nil {
		return err
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", append(args, scriptPath)...)
	} else {
		cmd = exec.Command("bash", scriptPath)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(), "VM_VERSION="+target)
	return cmd.Run()
}

// binaryVersion runs `<path> --version` and returns the last whitespace-
// separated token.
func binaryVersion(path string) (string, error) {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty --version output")
	}
	return fields[len(fields)-1], nil
}

func sameVersion(a, b string) bool {
	a = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(a), "v"), "V")
	b = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(b), "v"), "V")
	return a == b
}
