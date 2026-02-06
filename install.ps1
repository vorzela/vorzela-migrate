# Vorzela Migration Tool - Automatic Installation Script for Windows
# For Windows PowerShell
# Usage: iex (New-Object Net.WebClient).DownloadString('https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.ps1')

# Requires -Version 3.0

# Color output
function Write-Status {
    param([string]$Message)
    Write-Host "ℹ $Message" -ForegroundColor Cyan
}

function Write-Success {
    param([string]$Message)
    Write-Host "✓ $Message" -ForegroundColor Green
}

function Write-Warning {
    param([string]$Message)
    Write-Host "⚠ $Message" -ForegroundColor Yellow
}

function Write-Error {
    param([string]$Message)
    Write-Host "✗ $Message" -ForegroundColor Red
}

# Main installation function
function Install-Vorzela {
    Write-Host ""
    Write-Host "╔═══════════════════════════════════════════════════╗" -ForegroundColor Cyan
    Write-Host "║   Vorzela Migration Tool - Installation Script    ║" -ForegroundColor Cyan
    Write-Host "╚═══════════════════════════════════════════════════╝" -ForegroundColor Cyan
    Write-Host ""

    # Detect architecture
    $Arch = if ([Environment]::Is64BitProcess) { "amd64" } else { "386" }
    Write-Status "Detected Architecture: Windows ($Arch)"

    # Determine binary name
    $BinaryName = "vm-windows-$Arch.exe"
    $InstallDir = "C:\Program Files\vorzela"
    $GitHubRepo = "vorzela/vorzela-migrate"
    $ReleaseUrl = "https://github.com/$GitHubRepo/releases/download"

    # Check if directory exists, create if not
    if (-not (Test-Path $InstallDir)) {
        Write-Status "Creating installation directory: $InstallDir"
        try {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
            Write-Success "Installation directory created"
        }
        catch {
            Write-Error "Failed to create directory. You may need admin rights."
            Write-Error "Try running PowerShell as Administrator"
            exit 1
        }
    }

    # Get latest release version
    Write-Status "Fetching latest release..."
    try {
        $LatestRelease = (Invoke-RestMethod -Uri "https://api.github.com/repos/$GitHubRepo/releases/latest" -ErrorAction SilentlyContinue).tag_name
        if (-not $LatestRelease) {
            Write-Warning "Could not fetch latest version, using v1.0.0"
            $LatestRelease = "v1.0.0"
        }
        Write-Success "Latest version: $LatestRelease"
    }
    catch {
        Write-Warning "Could not fetch latest version, using v1.0.0"
        $LatestRelease = "v1.0.0"
    }

    # Download binary
    $DownloadUrl = "$ReleaseUrl/$LatestRelease/$BinaryName"
    $BinaryPath = "$InstallDir\vm.exe"

    Write-Status "Downloading from: $DownloadUrl"
    try {
        # Try using Invoke-WebRequest (PowerShell 3.0+)
        Invoke-WebRequest -Uri $DownloadUrl -OutFile $BinaryPath -UseBasicParsing -ErrorAction Stop
        Write-Success "Binary downloaded successfully"
    }
    catch {
        Write-Error "Failed to download binary"
        Write-Warning "Note: Pre-built binaries may not be available yet"
        Write-Status "Try building from source instead:"
        Write-Status "  git clone https://github.com/$GitHubRepo.git"
        Write-Status "  cd vorzela-migrate"
        Write-Status "  go build -o vm.exe main.go"
        Write-Status "  Move-Item vm.exe 'C:\Program Files\vorzela\vm.exe'"
        exit 1
    }

    # Add to PATH if not already there
    Write-Status "Checking PATH environment variable..."
    $EnvPath = [Environment]::GetEnvironmentVariable("Path", "User")
    
    if ($EnvPath -notlike "*$InstallDir*") {
        Write-Status "Adding $InstallDir to PATH..."
        try {
            $NewPath = "$InstallDir;$EnvPath"
            [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
            Write-Success "Added to PATH"
            
            # Update current session PATH
            $env:Path = "$InstallDir;$env:Path"
        }
        catch {
            Write-Warning "Could not add to PATH automatically"
            Write-Status "Please add manually:"
            Write-Status "  1. Open Environment Variables (Win+X, then 'Environment Variables')"
            Write-Status "  2. Click 'Edit the system environment variables'"
            Write-Status "  3. Click 'Environment Variables...'"
            Write-Status "  4. Under 'User variables', click 'New'"
            Write-Status "  5. Variable name: Path"
            Write-Status "  6. Variable value: $InstallDir"
            Write-Status "  7. Click OK and restart PowerShell"
        }
    }
    else {
        Write-Success "$InstallDir already in PATH"
    }

    Write-Host ""
    Write-Status "Verifying installation..."
    try {
        $Version = & $BinaryPath --version 2>$null
        Write-Success "Vorzela installed successfully! ($Version)"
    }
    catch {
        Write-Warning "Could not verify installation"
        Write-Status "Try running: vm --version"
    }

    Write-Host ""
    Write-Status "Quick start:"
    Write-Host "    vm --version" -ForegroundColor White
    Write-Host "    vm --help" -ForegroundColor White
    Write-Host ""

    Write-Status "Next steps:"
    Write-Host "    1. Read INSTALL.md for setup instructions" -ForegroundColor White
    Write-Host "    2. Create .vorzela config in your project" -ForegroundColor White
    Write-Host "    3. Create migration: vm make migration create_users_table" -ForegroundColor White
    Write-Host "    4. Read QUICK_REFERENCE.md for examples" -ForegroundColor White
    Write-Host ""

    # Offer to restart PowerShell
    Write-Host ""
    Write-Status "PowerShell may need to be restarted for PATH changes to take effect"
    $Restart = Read-Host "Restart PowerShell now? (y/n)"
    if ($Restart -eq "y" -or $Restart -eq "yes") {
        Write-Status "Restarting PowerShell..."
        Start-Process pwsh -ArgumentList "-NoExit"
        exit 0
    }
}

# Run installation
try {
    Install-Vorzela
}
catch {
    Write-Error "Installation failed: $_"
    exit 1
}
