#!/bin/sh
# klaudiush installer script
# Downloads and installs the latest release of klaudiush
#
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/smykla-labs/klaudiush/main/install.sh | sh
#   curl -sSfL https://raw.githubusercontent.com/smykla-labs/klaudiush/main/install.sh | sh -s -- -b /custom/path
#   curl -sSfL https://raw.githubusercontent.com/smykla-labs/klaudiush/main/install.sh | sh -s -- -v v1.0.0
#
# Options:
#   -b DIR    Install binary to DIR (default: ~/.local/bin)
#   -v VER    Install specific version (default: latest)
#   -h        Show help

set -e

GITHUB_REPO="smykla-labs/klaudiush"
BINARY_NAME="klaudiush"
DEFAULT_INSTALL_DIR="${HOME}/.local/bin"

# Colors for output (disabled if not a TTY)
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

info() {
    printf "${BLUE}info${NC}: %s\n" "$1"
}

success() {
    printf "${GREEN}success${NC}: %s\n" "$1"
}

warn() {
    printf "${YELLOW}warning${NC}: %s\n" "$1"
}

error() {
    printf "${RED}error${NC}: %s\n" "$1" >&2
    exit 1
}

usage() {
    cat <<EOF
klaudiush installer

Usage:
    install.sh [OPTIONS]

Options:
    -b DIR    Install binary to DIR (default: ${DEFAULT_INSTALL_DIR})
    -v VER    Install specific version (default: latest)
    -h        Show this help message

Examples:
    # Install latest version to ~/.local/bin
    curl -sSfL https://raw.githubusercontent.com/smykla-labs/klaudiush/main/install.sh | sh

    # Install specific version
    curl -sSfL https://raw.githubusercontent.com/smykla-labs/klaudiush/main/install.sh | sh -s -- -v v1.0.0

    # Install to custom directory
    curl -sSfL https://raw.githubusercontent.com/smykla-labs/klaudiush/main/install.sh | sh -s -- -b /usr/local/bin
EOF
}

detect_os() {
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        linux) echo "linux" ;;
        darwin) echo "darwin" ;;
        mingw*|msys*|cygwin*) echo "windows" ;;
        *) error "Unsupported operating system: $os" ;;
    esac
}

detect_arch() {
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) error "Unsupported architecture: $arch" ;;
    esac
}

get_latest_version() {
    version=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | \
        grep '"tag_name":' | \
        sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$version" ]; then
        error "Failed to get latest version from GitHub"
    fi

    echo "$version"
}

download_and_install() {
    version=$1
    install_dir=$2
    os=$3
    arch=$4

    # Determine file extension
    if [ "$os" = "windows" ]; then
        ext="zip"
        binary="${BINARY_NAME}.exe"
    else
        ext="tar.gz"
        binary="${BINARY_NAME}"
    fi

    # Remove 'v' prefix for archive name if present
    version_num="${version#v}"

    archive_name="${BINARY_NAME}_${version_num}_${os}_${arch}.${ext}"
    download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${archive_name}"

    info "Downloading ${archive_name}..."

    # Create temp directory
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    # Download archive
    if ! curl -fsSL -o "${tmp_dir}/${archive_name}" "$download_url"; then
        error "Failed to download ${download_url}"
    fi

    # Verify checksum
    checksums_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/checksums.txt"
    if curl -fsSL -o "${tmp_dir}/checksums.txt" "$checksums_url" 2>/dev/null; then
        info "Verifying checksum..."
        cd "$tmp_dir"

        if command -v sha256sum >/dev/null 2>&1; then
            grep "${archive_name}" checksums.txt | sha256sum -c - >/dev/null 2>&1 || \
                error "Checksum verification failed"
        elif command -v shasum >/dev/null 2>&1; then
            grep "${archive_name}" checksums.txt | shasum -a 256 -c - >/dev/null 2>&1 || \
                error "Checksum verification failed"
        else
            warn "No checksum tool available, skipping verification"
        fi
        cd - >/dev/null
    else
        warn "Checksums not available, skipping verification"
    fi

    # Extract archive
    info "Extracting..."
    cd "$tmp_dir"

    if [ "$ext" = "zip" ]; then
        unzip -q "${archive_name}"
    else
        tar -xzf "${archive_name}"
    fi

    # Install binary
    mkdir -p "$install_dir"

    if [ -f "$binary" ]; then
        mv "$binary" "${install_dir}/${BINARY_NAME}"
    else
        error "Binary not found in archive"
    fi

    chmod +x "${install_dir}/${BINARY_NAME}"

    cd - >/dev/null
}

main() {
    install_dir="$DEFAULT_INSTALL_DIR"
    version=""

    # Parse arguments
    while getopts "b:v:h" opt; do
        case "$opt" in
            b) install_dir="$OPTARG" ;;
            v) version="$OPTARG" ;;
            h) usage; exit 0 ;;
            *) usage; exit 1 ;;
        esac
    done

    # Detect platform
    os=$(detect_os)
    arch=$(detect_arch)

    info "Detected platform: ${os}/${arch}"

    # Get version
    if [ -z "$version" ]; then
        info "Getting latest version..."
        version=$(get_latest_version)
    fi

    info "Installing ${BINARY_NAME} ${version}..."

    # Download and install
    download_and_install "$version" "$install_dir" "$os" "$arch"

    success "Installed ${BINARY_NAME} ${version} to ${install_dir}/${BINARY_NAME}"

    # Check if install_dir is in PATH
    case ":$PATH:" in
        *":${install_dir}:"*)
            ;;
        *)
            echo ""
            warn "${install_dir} is not in your PATH"
            echo "Add the following to your shell profile:"
            echo ""
            echo "    export PATH=\"${install_dir}:\$PATH\""
            echo ""
            ;;
    esac

    # Verify installation
    if command -v "${install_dir}/${BINARY_NAME}" >/dev/null 2>&1; then
        echo ""
        info "Verifying installation..."
        "${install_dir}/${BINARY_NAME}" version
    fi
}

main "$@"
