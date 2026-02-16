#!/bin/sh
set -e

# Box of Rocks installer
# Usage: curl -fsSL https://raw.githubusercontent.com/jmaddaus/boxofrocks/main/install.sh | sh

REPO="jmaddaus/boxofrocks"
INSTALL_DIR="${BOR_INSTALL_DIR:-/usr/local/bin}"

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       echo "unsupported"; return 1 ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *)             echo "unsupported"; return 1 ;;
    esac
}

main() {
    OS=$(detect_os)
    ARCH=$(detect_arch)

    if [ "$OS" = "unsupported" ] || [ "$ARCH" = "unsupported" ]; then
        echo "Error: unsupported platform $(uname -s)/$(uname -m)" >&2
        exit 1
    fi

    echo "Detected platform: ${OS}/${ARCH}"

    # Get latest release tag
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
    if [ -z "$VERSION" ]; then
        echo "Error: could not determine latest release" >&2
        exit 1
    fi

    echo "Latest version: ${VERSION}"

    ARCHIVE="bor-${OS}-${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"
    CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    echo "Downloading ${ARCHIVE}..."
    curl -fsSL -o "${TMPDIR}/${ARCHIVE}" "$DOWNLOAD_URL"

    echo "Downloading checksums..."
    curl -fsSL -o "${TMPDIR}/checksums.txt" "$CHECKSUMS_URL"

    echo "Verifying checksum..."
    EXPECTED=$(grep "${ARCHIVE}" "${TMPDIR}/checksums.txt" | awk '{print $1}')
    if [ -z "$EXPECTED" ]; then
        echo "Error: archive not found in checksums.txt" >&2
        exit 1
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        ACTUAL=$(sha256sum "${TMPDIR}/${ARCHIVE}" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        ACTUAL=$(shasum -a 256 "${TMPDIR}/${ARCHIVE}" | awk '{print $1}')
    else
        echo "Error: no sha256sum or shasum found" >&2
        exit 1
    fi

    if [ "$EXPECTED" != "$ACTUAL" ]; then
        echo "Error: checksum mismatch" >&2
        echo "  expected: ${EXPECTED}" >&2
        echo "  actual:   ${ACTUAL}" >&2
        exit 1
    fi

    echo "Checksum verified."

    tar -xzf "${TMPDIR}/${ARCHIVE}" -C "${TMPDIR}"

    echo "Installing bor to ${INSTALL_DIR}..."
    if [ -w "$INSTALL_DIR" ]; then
        cp "${TMPDIR}/bor" "${INSTALL_DIR}/bor"
        chmod +x "${INSTALL_DIR}/bor"
    else
        sudo cp "${TMPDIR}/bor" "${INSTALL_DIR}/bor"
        sudo chmod +x "${INSTALL_DIR}/bor"
    fi

    echo "bor ${VERSION} installed to ${INSTALL_DIR}/bor"
}

main
