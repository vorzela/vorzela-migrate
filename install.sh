#!/bin/bash

# Vorzela Migration Tool - Automatic Installation Script
# For macOS and Linux
# Usage: curl -fsSL https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.sh | bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Functions
print_status() {
    echo -e "${CYAN}ℹ ${1}${NC}"
}

print_success() {
    echo -e "${GREEN}✓ ${1}${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ ${1}${NC}"
}

print_error() {
    echo -e "${RED}✗ ${1}${NC}"
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "linux";;
        Darwin*)    echo "macos";;
        *)          echo "unknown";;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64)     echo "amd64";;
        arm64)      echo "arm64";;
        aarch64)    echo "arm64";;
        *)          echo "unknown";;
    esac
}

# Main installation
main() {
    echo ""
    echo -e "${CYAN}╔═══════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║   Vorzela Migration Tool - Installation Script    ║${NC}"
    echo -e "${CYAN}╚═══════════════════════════════════════════════════╝${NC}"
    echo ""

    # Detect OS and architecture
    OS=$(detect_os)
    ARCH=$(detect_arch)

    if [ "$OS" == "unknown" ]; then
        print_error "Unsupported operating system"
        exit 1
    fi

    if [ "$ARCH" == "unknown" ]; then
        print_error "Unsupported architecture"
        exit 1
    fi

    print_status "Detected OS: $OS ($ARCH)"

    # Determine binary name
    if [ "$OS" == "macos" ]; then
        if [ "$ARCH" == "arm64" ]; then
            BINARY_NAME="vm-macos-arm64"
        else
            BINARY_NAME="vm-macos-amd64"
        fi
    else
        BINARY_NAME="vm-linux-${ARCH}"
    fi

    INSTALL_DIR="${HOME}/.local/bin"
    GITHUB_REPO="vorzela/vorzela-migrate"
    RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/download"

    # Get latest release version
    print_status "Fetching latest release..."
    LATEST_VERSION=$(curl -sL https://api.github.com/repos/${GITHUB_REPO}/releases/latest | grep -oP '"tag_name": "\K[^"]+' | head -1)

    if [ -z "$LATEST_VERSION" ]; then
        print_warning "Could not fetch latest version from GitHub, using v1.0.0"
        LATEST_VERSION="v1.0.0"
    fi

    print_success "Latest version: $LATEST_VERSION"

    # Create install directory if it doesn't exist
    mkdir -p "$INSTALL_DIR"

    # Download binary
    DOWNLOAD_URL="${RELEASE_URL}/${LATEST_VERSION}/${BINARY_NAME}"
    BINARY_PATH="${INSTALL_DIR}/vm"

    print_status "Downloading from: $DOWNLOAD_URL"

    if ! curl -fsSL "$DOWNLOAD_URL" -o "$BINARY_PATH"; then
        print_error "Failed to download binary"
        print_warning "Note: Pre-built binaries may not be available yet"
        print_status "Try building from source instead:"
        print_status "  git clone https://github.com/${GITHUB_REPO}.git"
        print_status "  cd vorzela-migrate"
        print_status "  go build -o vc main.go"
        print_status "  sudo mv vc /usr/local/bin/"
        exit 1
    fi

    # Make binary executable
    chmod +x "$BINARY_PATH"
    print_success "Binary downloaded and made executable"

    # Add to PATH if needed
    if ! command -v vm &> /dev/null || [ "$(command -v vm)" != "$BINARY_PATH" ]; then
        # Check if ~/.local/bin is in PATH
        if [[ ":$PATH:" == *":${INSTALL_DIR}:"* ]]; then
            print_success "Installation complete!"
        else
            print_warning "~/.local/bin is not in your PATH"
            print_status "Add this line to your shell profile (~/.bash_profile, ~/.zshrc, etc.):"
            echo ""
            echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
            echo ""
            print_status "Then run: source ~/.bashrc  (or ~/.zshrc)"
        fi
    else
        print_success "Installation complete!"
    fi

    echo ""
    print_status "Verifying installation..."
    if $BINARY_PATH --version > /dev/null 2>&1; then
        VERSION=$($BINARY_PATH --version 2>&1 | head -1)
        print_success "Vorzela installed successfully! ($VERSION)"
    else
        print_warning "Could not verify installation"
    fi

    echo ""
    print_status "Quick start:"
    echo "    vm --version"
    echo "    vm --help"
    echo ""
    print_status "Next steps:"
    echo "    1. Read INSTALL.md for platform-specific setup"
    echo "    2. Create .vorzela config in your project"
    echo "    3. Create your first migration: vm make migration create_users_table"
    echo "    4. Read QUICK_REFERENCE.md for examples"
    echo ""
}

# Run main function
main "$@"
