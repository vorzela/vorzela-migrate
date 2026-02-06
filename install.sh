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

# Helper: try to get latest tag robustly
get_latest_version() {
    GITHUB_REPO="$1"
    # Try GitHub API first
    ver=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep -oP '"tag_name": "\K[^"]+' | head -1 || true)
    if [ -n "$ver" ]; then
        echo "$ver"
        return 0
    fi

    # Fallback: follow redirect from /releases/latest page
    redirect=$(curl -fsSLI -o /dev/null -w "%{redirect_url}" "https://github.com/${GITHUB_REPO}/releases/latest" 2>/dev/null || true)
    if [ -n "$redirect" ]; then
        # last path component is the tag
        echo "${redirect##*/}"
        return 0
    fi

    return 1
}

build_from_source() {
    GITHUB_REPO="$1"
    INSTALL_DIR="$2"
    BINARY_PATH="$3"

    if ! command -v go >/dev/null 2>&1; then
        return 1
    fi

    tmpdir=$(mktemp -d)
    print_status "Cloning source to $tmpdir"
    if ! git clone --depth 1 "https://github.com/${GITHUB_REPO}.git" "$tmpdir" >/dev/null 2>&1; then
        rm -rf "$tmpdir"
        return 1
    fi

    pushd "$tmpdir" >/dev/null
    print_status "Building from source (this may take a moment)..."
    if go mod tidy >/dev/null 2>&1 && go build -o vm main.go >/dev/null 2>&1; then
        mkdir -p "$(dirname "$BINARY_PATH")"
        mv vm "$BINARY_PATH"
        chmod +x "$BINARY_PATH"
        popd >/dev/null
        rm -rf "$tmpdir"
        return 0
    fi
    popd >/dev/null
    rm -rf "$tmpdir"
    return 1
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
    LATEST_VERSION=$(get_latest_version "${GITHUB_REPO}" || true)

    if [ -z "$LATEST_VERSION" ]; then
        print_warning "Could not fetch latest version from GitHub"
    else
        print_success "Latest version: $LATEST_VERSION"
    fi

    # Create install directory if it doesn't exist
    mkdir -p "$INSTALL_DIR"

    # Prepare download path
    DOWNLOAD_URL="${RELEASE_URL}/${LATEST_VERSION}/${BINARY_NAME}"
    BINARY_PATH="${INSTALL_DIR}/vm"

    # Try to download if we have a version
    if [ -n "$LATEST_VERSION" ]; then
        print_status "Attempting to download: ${DOWNLOAD_URL}"
        if curl -fsSL "$DOWNLOAD_URL" -o "$BINARY_PATH"; then
            chmod +x "$BINARY_PATH"
            print_success "Binary downloaded and made executable"
        else
            print_warning "Pre-built binary not available for ${LATEST_VERSION}/${BINARY_NAME}"
            rm -f "$BINARY_PATH" || true
        fi
    fi

    # If download failed or no prebuilt, attempt to build from source if go is available
    if [ ! -x "$BINARY_PATH" ]; then
        print_status "Attempting to build from source (requires Go)..."
        if build_from_source "${GITHUB_REPO}" "$INSTALL_DIR" "$BINARY_PATH"; then
            print_success "Built and installed vm from source"
        else
            print_warning "Automatic build failed or Go not installed"
            print_status "Fallback instructions:"
            print_status "  1) Install Go (https://go.dev/dl/) and re-run this script"
            print_status "  2) Or build manually:"
            print_status "     git clone https://github.com/${GITHUB_REPO}.git"
            print_status "     cd vorzela-migrate"
            print_status "     go build -o vm main.go"
            print_status "     mv vm ${INSTALL_DIR}/vm # or /usr/local/bin"
            exit 1
        fi
    fi

    # Add to PATH if needed
    if ! command -v vm &> /dev/null || [ "$(command -v vm)" != "$BINARY_PATH" ]; then
        # Check if ~/.local/bin is in PATH
        if [[ ":$PATH:" == *":${INSTALL_DIR}:"* ]]; then
            print_success "Installation complete!"
        else
            print_warning "${INSTALL_DIR} is not in your PATH"
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
    if "$BINARY_PATH" --version > /dev/null 2>&1; then
        VERSION=$("$BINARY_PATH" --version 2>&1 | head -1)
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
