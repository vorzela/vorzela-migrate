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

norm_ver() {
    echo "$1" | sed -E 's/^[vV]//'
}

# Prefer non-API discovery — unauthenticated api.github.com is often rate-limited.
get_latest_version() {
    GITHUB_REPO="$1"

    # 1) git ls-remote (no API quota; version-sorted — more reliable than
    #    /releases/latest while a new release is still publishing assets)
    if command -v git >/dev/null 2>&1; then
        ver=$(git ls-remote --refs --tags "https://github.com/${GITHUB_REPO}.git" 'v*' 2>/dev/null \
            | awk -F/ '{print $NF}' \
            | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
            | sort -V \
            | tail -1 || true)
        if [ -n "$ver" ]; then
            echo "$ver"
            return 0
        fi
    fi

    # 2) HTML redirect from /releases/latest (no API quota). Do NOT use -L:
    #    with -L, curl's %{redirect_url} is empty after the final hop.
    redirect=$(curl -fsSI --connect-timeout 10 --max-time 20 -o /dev/null -w "%{redirect_url}" \
        "https://github.com/${GITHUB_REPO}/releases/latest" 2>/dev/null || true)
    if [ -z "$redirect" ]; then
        redirect=$(curl -fsSI --connect-timeout 10 --max-time 20 \
            "https://github.com/${GITHUB_REPO}/releases/latest" 2>/dev/null \
            | awk 'BEGIN{IGNORECASE=1} /^location:/{print $2}' | tr -d '\r' | tail -1 || true)
    fi
    if [ -n "$redirect" ]; then
        ver="${redirect##*/}"
        if [ -n "$ver" ] && [ "$ver" != "latest" ]; then
            echo "$ver"
            return 0
        fi
    fi

    # 3) GitHub API (may fail with 403 rate limit)
    ver=$(curl -fsSL --connect-timeout 10 --max-time 20 -A "vorzela-migrate-install" \
        "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null \
        | grep -oP '"tag_name": "\K[^"]+' | head -1 || true)
    if [ -n "$ver" ]; then
        echo "$ver"
        return 0
    fi

    return 1
}

# Download a release asset with a visible progress bar and hard timeouts.
# Silent curl makes a ~20MB GitHub CDN fetch look "stuck".
download_release_binary() {
    url="$1"
    dest="$2"
    print_status "Downloading ~20 MB from GitHub (progress below; may take 1–3 min on slow links)..."
    # -# progress bar on stderr; no -s so the user sees activity
    if curl -fL --connect-timeout 20 --max-time 300 --retry 2 --retry-delay 3 \
        -# "$url" -o "$dest"; then
        return 0
    fi
    return 1
}

build_from_source() {
    GITHUB_REPO="$1"
    INSTALL_DIR="$2"
    BINARY_PATH="$3"
    BUILD_VERSION="$4"

    if ! command -v go >/dev/null 2>&1; then
        return 1
    fi

    tmpdir=$(mktemp -d)
    print_status "Cloning source to $tmpdir"

    # Prefer a tagged shallow clone so ldflags get a real version.
    if [ -n "$BUILD_VERSION" ]; then
        if ! git clone --depth 1 --branch "$BUILD_VERSION" \
            "https://github.com/${GITHUB_REPO}.git" "$tmpdir" >/dev/null 2>&1; then
            rm -rf "$tmpdir"
            return 1
        fi
    else
        if ! git clone --depth 1 "https://github.com/${GITHUB_REPO}.git" "$tmpdir" >/dev/null 2>&1; then
            rm -rf "$tmpdir"
            return 1
        fi
        # depth-1 clones omit tags; fetch them so we can stamp a version.
        git -C "$tmpdir" fetch --tags --depth 1 >/dev/null 2>&1 || true
        BUILD_VERSION=$(git -C "$tmpdir" tag --list 'v*.*.*' --sort=v:refname | tail -1 || true)
    fi

    pushd "$tmpdir" >/dev/null
    print_status "Building from source (this may take a moment)..."

    if [ -z "$BUILD_VERSION" ]; then
        BUILD_VERSION="dev"
    fi

    LDFLAGS="-X 'github.com/vorzela/vorzela-migrate/internal/version.CurrentVersion=${BUILD_VERSION}'"

    if go mod tidy >/dev/null 2>&1 && go build -ldflags "$LDFLAGS" -o vm main.go >/dev/null 2>&1; then
        mkdir -p "$(dirname "$BINARY_PATH")"

        # Backup existing binary if present
        if [ -f "$BINARY_PATH" ]; then
            ts=$(date +%s)
            mv "$BINARY_PATH" "${BINARY_PATH}.bak.${ts}" 2>/dev/null || true
            print_status "Backed up existing binary to ${BINARY_PATH}.bak.${ts}"
        fi

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
    INSTALLED=0

    # Already on this version — skip the slow CDN download
    if [ -n "$LATEST_VERSION" ] && [ -x "$BINARY_PATH" ]; then
        CURRENT_OUT=$("$BINARY_PATH" --version 2>/dev/null | head -1 || true)
        CURRENT_VER=$(echo "$CURRENT_OUT" | awk '{print $NF}')
        if [ -n "$CURRENT_VER" ] && [ "$(norm_ver "$CURRENT_VER")" = "$(norm_ver "$LATEST_VERSION")" ]; then
            print_success "Already on ${LATEST_VERSION} — nothing to download"
            INSTALLED=1
        fi
    fi

    # Try to download if we have a version and still need to install
    if [ "$INSTALLED" -ne 1 ] && [ -n "$LATEST_VERSION" ]; then
        print_status "Attempting to download: ${DOWNLOAD_URL}"
        tmpbin=$(mktemp)
        if download_release_binary "$DOWNLOAD_URL" "$tmpbin"; then
            chmod +x "$tmpbin"
            if [ -f "$BINARY_PATH" ]; then
                ts=$(date +%s)
                mv "$BINARY_PATH" "${BINARY_PATH}.bak.${ts}" 2>/dev/null || true
            fi
            mv "$tmpbin" "$BINARY_PATH"
            print_success "Binary downloaded and made executable"
            INSTALLED=1
        else
            print_warning "Download failed or timed out for ${LATEST_VERSION}/${BINARY_NAME}"
            rm -f "$tmpbin" || true
        fi
    fi

    # If download failed, attempt to build from source if go is available.
    # Do NOT treat a pre-existing binary as a successful install — that left
    # users stuck on old versions when version discovery failed (API rate limit).
    if [ "$INSTALLED" -ne 1 ]; then
        print_status "Attempting to build from source (requires Go)..."
        if build_from_source "${GITHUB_REPO}" "$INSTALL_DIR" "$BINARY_PATH" "$LATEST_VERSION"; then
            print_success "Built and installed vm from source"
            INSTALLED=1
        else
            print_warning "Automatic build failed or Go not installed"
            if [ -x "$BINARY_PATH" ]; then
                OLD=$("$BINARY_PATH" --version 2>&1 | head -1 || true)
                print_error "Left existing binary in place (${OLD:-$BINARY_PATH})"
                print_error "Upgrade did not run — fix network/rate-limit and re-run, or install Go and retry."
            fi
            print_status "Fallback instructions:"
            print_status "  1) Install Go (https://go.dev/dl/) and re-run this script"
            print_status "  2) Or build manually:"
            print_status "     git clone https://github.com/${GITHUB_REPO}.git"
            print_status "     cd vorzela-migrate"
            print_status "     go build -ldflags \"-X 'github.com/vorzela/vorzela-migrate/internal/version.CurrentVersion=vX.Y.Z'\" -o vm main.go"
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
            PROFILE=""
            case "$SHELL" in
                */bash) PROFILE="$HOME/.bashrc";;
                */zsh) PROFILE="$HOME/.zshrc";;
            esac

            if [ -z "$PROFILE" ]; then
                if [ -f "$HOME/.bashrc" ]; then
                    PROFILE="$HOME/.bashrc"
                elif [ -f "$HOME/.zshrc" ]; then
                    PROFILE="$HOME/.zshrc"
                elif [ -f "$HOME/.profile" ]; then
                    PROFILE="$HOME/.profile"
                else
                    PROFILE="$HOME/.bashrc"
                fi
            fi

            if [ ! -f "$PROFILE" ]; then
                print_status "Creating $PROFILE and adding ${INSTALL_DIR} to PATH"
                echo "export PATH=\"\$HOME/.local/bin:\$PATH\"" >> "$PROFILE"
            elif ! grep -qs '\.local/bin' "$PROFILE"; then
                print_status "Adding ${INSTALL_DIR} to PATH in $PROFILE"
                echo "" >> "$PROFILE"
                echo "export PATH=\"\$HOME/.local/bin:\$PATH\"" >> "$PROFILE"
            else
                print_status "${INSTALL_DIR} already referenced in $PROFILE"
            fi

            export PATH="$HOME/.local/bin:$PATH"
            print_success "Added ${INSTALL_DIR} to PATH"
            print_status "Restart your shell or run: source $PROFILE"
        fi
    else
        print_success "Installation complete!"
    fi

    echo ""
    print_status "Verifying installation..."
    # Verify installation and check version
    if "$BINARY_PATH" --version > /dev/null 2>&1; then
        VERSION_OUTPUT=$("$BINARY_PATH" --version 2>&1 | head -1)
        # Extract just the version number (last field) from output like "vm version v1.1.0"
        VERSION=$(echo "$VERSION_OUTPUT" | awk '{print $NF}')
        print_success "Vorzela installed successfully! ($VERSION_OUTPUT)"
        # If we have a latest version and it mismatches, warn
        if [ -n "$LATEST_VERSION" ]; then
            if [ "$(norm_ver "$VERSION")" != "$(norm_ver "$LATEST_VERSION")" ]; then
                print_warning "Installed binary version ($VERSION) does not match expected $LATEST_VERSION"
            fi
        fi
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
    echo "    2. Create a .vm config in your project (vm init)"
    echo "    3. Create your first migration: vm make migration create_users_table"
    echo "    4. Read QUICK_REFERENCE.md for examples"
    echo ""
}

# Run main function
main "$@"
