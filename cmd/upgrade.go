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
			output.Error("Could not check for updates: %v", err)
			output.Info("Your installed version is %s", version.CurrentVersion)
			output.Info("If a newer release exists, install with:")
			output.Info("  curl -sSL https://github.com/vorzela/vorzela-migrate/raw/main/install.sh | bash")
			return fmt.Errorf("update check failed: %w", err)
		}

		if !available {
			output.Success(fmt.Sprintf("You're already on the latest version (%s)", version.CurrentVersion))
			return nil
		}

		output.Info(fmt.Sprintf("Upgrading from %s to %s...", version.CurrentVersion, newVersion))

		// Use the install script directly since vm is distributed via GitHub releases
		upgraded := tryScriptInstall()

		if upgraded {
			output.Success("Upgrade completed! Run 'vm --version' to verify")
			return nil
		}

		output.Error("Failed to upgrade. Please visit https://github.com/vorzela/vorzela-migrate for manual installation")
		return fmt.Errorf("upgrade failed")
	},
}

// tryScriptInstall attempts to upgrade using the installation script
func tryScriptInstall() bool {
	output.Info("Downloading and installing latest version...")

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
