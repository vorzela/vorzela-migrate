package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/output"
	"github.com/vorzela/vorzela-migrate/internal/version"
)

var UpgradeCommand = &cli.Command{
	Name:  "upgrade",
	Usage: "Upgrade to the latest version of Vorzela",
	Action: func(c *cli.Context) error {
		newVersion, available, err := version.CheckForUpdate()
		if err != nil {
			output.Error("Failed to check for updates")
			return err
		}

		if !available {
			output.Success(fmt.Sprintf("You're already on the latest version (%s)", version.CurrentVersion))
			return nil
		}

		output.Info(fmt.Sprintf("Upgrading from %s to %s...", version.CurrentVersion, newVersion))

		// Determine the upgrade method based on OS
		upgraded := false

		// Try brew on macOS
		if runtime.GOOS == "darwin" {
			if cmdExists("brew") {
				upgraded = tryBrew()
			}
		}

		// Try apt on Linux
		if !upgraded && runtime.GOOS == "linux" {
			if cmdExists("apt-get") {
				upgraded = tryApt()
			}
		}

		// Fallback: download and install script
		if !upgraded {
			upgraded = tryScriptInstall()
		}

		if upgraded {
			output.Success("Upgrade completed! Run 'vm --version' to verify")
			return nil
		}

		output.Error("Failed to upgrade. Please visit https://github.com/vorzela/vorzela-migrate for manual installation")
		return fmt.Errorf("upgrade failed")
	},
}

// cmdExists checks if a command is available in PATH
func cmdExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// tryBrew attempts to upgrade using Homebrew
func tryBrew() bool {
	output.Info("Attempting upgrade via Homebrew...")
	cmd := exec.Command("brew", "install", "--upgrade", "vorzela/vorzela/vorzela-migrate")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// tryApt attempts to upgrade using apt-get
func tryApt() bool {
	output.Info("Attempting upgrade via apt...")
	cmd := exec.Command("sudo", "apt-get", "update")
	if err := cmd.Run(); err != nil {
		return false
	}

	cmd = exec.Command("sudo", "apt-get", "install", "-y", "vorzela-migrate")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// tryScriptInstall attempts to upgrade using the installation script
func tryScriptInstall() bool {
	output.Info("Attempting upgrade via download script...")

	var scriptURL string
	if runtime.GOOS == "windows" {
		scriptURL = "https://github.com/vorzela/vorzela-migrate/raw/main/install.ps1"
		// PowerShell script
		cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-Command",
			fmt.Sprintf("Invoke-WebRequest -Uri '%s' -OutFile './install.ps1'; ./install.ps1", scriptURL))
		if err := cmd.Run(); err != nil {
			return false
		}
	} else {
		// Bash script for Linux/macOS
		scriptURL = "https://github.com/vorzela/vorzela-migrate/raw/main/install.sh"
		cmd := exec.Command("bash", "-c",
			fmt.Sprintf("curl -sSL '%s' | bash", scriptURL))
		if err := cmd.Run(); err != nil {
			return false
		}
	}

	return true
}
