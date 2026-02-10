#!/bin/sh
# klaudiush installer script
# Downloads and installs the latest release of klaudiush
#
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/smykla-skalski/klaudiush/main/install.sh | sh
#   curl -sSfL https://raw.githubusercontent.com/smykla-skalski/klaudiush/main/install.sh | sh -s -- -b /custom/path
#   curl -sSfL https://raw.githubusercontent.com/smykla-skalski/klaudiush/main/install.sh | sh -s -- -v v1.0.0
#
# Options:
#   -b DIR    Install binary to DIR (default: ~/.local/bin)
#   -v VER    Install specific version (default: latest)
#   -h        Show help

set -e

GITHUB_REPO="smykla-skalski/klaudiush"
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
    curl -sSfL https://raw.githubusercontent.com/smykla-skalski/klaudiush/main/install.sh | sh

    # Install specific version
    curl -sSfL https://raw.githubusercontent.com/smykla-skalski/klaudiush/main/install.sh | sh -s -- -v v1.0.0

    # Install to custom directory
    curl -sSfL https://raw.githubusercontent.com/smykla-skalski/klaudiush/main/install.sh | sh -s -- -b /usr/local/bin
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
    # Fetch latest release info from GitHub API
    response=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>&1)
    if [ $? -ne 0 ]; then
        error "Failed to fetch release information from GitHub API. Check your internet connection."
    fi

    # Extract tag_name from JSON response
    version=$(echo "$response" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$version" ]; then
        error "Failed to parse version from GitHub API response. The API response format may have changed."
    fi

    # Validate version format (should start with 'v' followed by numbers)
    if ! echo "$version" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+'; then
        error "Invalid version format received: ${version}"
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
    tmp_dir=$(mktemp -d 2>/dev/null)
    if [ -z "$tmp_dir" ] || [ ! -d "$tmp_dir" ]; then
        error "Failed to create temporary directory. Check /tmp permissions and available disk space."
    fi
    trap 'rm -rf "$tmp_dir"' EXIT

    # Download archive with retry logic (3 attempts)
    max_retries=3
    retry_delay=2
    attempt=1

    while [ $attempt -le $max_retries ]; do
        if curl -fsSL -o "${tmp_dir}/${archive_name}" "$download_url"; then
            # Verify downloaded file is not empty
            if [ -s "${tmp_dir}/${archive_name}" ]; then
                break
            else
                warn "Downloaded file is empty (attempt ${attempt}/${max_retries})"
            fi
        else
            warn "Download failed (attempt ${attempt}/${max_retries})"
        fi

        if [ $attempt -lt $max_retries ]; then
            info "Retrying in ${retry_delay} seconds..."
            sleep $retry_delay
        fi

        attempt=$((attempt + 1))
    done

    # Final check after all retries
    if [ ! -s "${tmp_dir}/${archive_name}" ]; then
        error "Failed to download ${download_url} after ${max_retries} attempts. Check your internet connection and that the release exists."
    fi

    # Verify checksum
    checksums_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/checksums.txt"
    if curl -fsSL -o "${tmp_dir}/checksums.txt" "$checksums_url" 2>/dev/null; then
        info "Verifying checksum..."
        cd "$tmp_dir"

        # Extract expected checksum for this archive (exact match only)
        expected_line=$(grep "  ${archive_name}$" checksums.txt)

        if [ -z "$expected_line" ]; then
            error "Archive '${archive_name}' not found in checksums.txt. This may indicate a corrupted download or version mismatch."
        fi

        # Verify checksum using available tool
        if command -v sha256sum >/dev/null 2>&1; then
            if ! echo "$expected_line" | sha256sum -c - 2>&1 | grep -q "OK"; then
                error "Checksum verification failed for ${archive_name}. Expected checksum does not match downloaded file."
            fi
        elif command -v shasum >/dev/null 2>&1; then
            if ! echo "$expected_line" | shasum -a 256 -c - 2>&1 | grep -q "OK"; then
                error "Checksum verification failed for ${archive_name}. Expected checksum does not match downloaded file."
            fi
        else
            warn "No checksum tool available (sha256sum or shasum), skipping verification"
        fi

        info "Checksum verified successfully"
        cd - >/dev/null
    else
        warn "Checksums not available, skipping verification"
    fi

    # Extract archive
    info "Extracting..."
    cd "$tmp_dir"

    if [ "$ext" = "zip" ]; then
        if ! unzip -q "${archive_name}"; then
            error "Failed to extract ${archive_name}. Archive may be corrupted."
        fi
    else
        if ! tar -xzf "${archive_name}"; then
            error "Failed to extract ${archive_name}. Archive may be corrupted."
        fi
    fi

    # Verify binary exists in archive
    if [ ! -f "$binary" ]; then
        error "Binary '${binary}' not found in archive. Expected files may be missing or archive structure may have changed."
    fi

    # Install binary
    if ! mkdir -p "$install_dir"; then
        error "Failed to create installation directory: ${install_dir}"
    fi

    if ! mv "$binary" "${install_dir}/${BINARY_NAME}"; then
        error "Failed to install binary to ${install_dir}/${BINARY_NAME}. Check directory permissions."
    fi

    if ! chmod +x "${install_dir}/${BINARY_NAME}"; then
        error "Failed to make binary executable at ${install_dir}/${BINARY_NAME}"
    fi

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
