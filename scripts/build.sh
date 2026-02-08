#!/usr/bin/env bash

set -euo pipefail

# Configuration
VERSION="${1:-dev}"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
DIST_DIR="dist"
BINARY_NAME="uac-socializer"

# Platform configuration
GOOS="linux"
GOARCH="amd64"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Cleanup function
cleanup() {
    if [ -d "${DIST_DIR}/tmp" ]; then
        rm -rf "${DIST_DIR}/tmp"
    fi
}

trap cleanup EXIT

# Main build function
main() {
    log_info "Building UAC Socializer v${VERSION}"
    log_info "Commit: ${COMMIT}"
    log_info "Build Time: ${BUILD_TIME}"
    log_info "Target: ${GOOS}/${GOARCH}"

    # Create dist directory
    mkdir -p "${DIST_DIR}"
    
    # Create temporary build directory
    BUILD_DIR="${DIST_DIR}/tmp/${BINARY_NAME}-${VERSION}"
    mkdir -p "${BUILD_DIR}"

    # Build binary
    log_info "Building binary..."
    CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build \
        -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_TIME}" \
        -o "${BUILD_DIR}/${BINARY_NAME}" \
        .

    if [ $? -ne 0 ]; then
        log_error "Build failed"
        exit 1
    fi

    log_info "Binary built successfully"

    # Copy data files
    log_info "Copying data files..."
    mkdir -p "${BUILD_DIR}/data"
    cp data/posts_by_date.json "${BUILD_DIR}/data/"

    # Copy prompt templates
    log_info "Copying prompt templates..."
    mkdir -p "${BUILD_DIR}/prompts"
    cp -r internal/infrastructure/prompts/*.txt "${BUILD_DIR}/prompts/" 2>/dev/null || true

    # Copy documentation
    log_info "Copying documentation..."
    cp LICENSE "${BUILD_DIR}/" 2>/dev/null || true
    cp README.md "${BUILD_DIR}/" 2>/dev/null || true

    # Create tarball
    TARBALL_NAME="${BINARY_NAME}-${VERSION}-${GOOS}-${GOARCH}.tar.gz"
    log_info "Creating tarball: ${TARBALL_NAME}"
    
    cd "${DIST_DIR}/tmp"
    tar -czf "../${TARBALL_NAME}" "${BINARY_NAME}-${VERSION}"
    cd - > /dev/null

    # Generate SHA256 checksum
    log_info "Generating SHA256 checksum..."
    cd "${DIST_DIR}"
    shasum -a 256 "${TARBALL_NAME}" > "${TARBALL_NAME}.sha256"
    cd - > /dev/null

    # Print summary
    log_info "Build complete!"
    echo ""
    echo "Artifacts:"
    echo "  - ${DIST_DIR}/${TARBALL_NAME}"
    echo "  - ${DIST_DIR}/${TARBALL_NAME}.sha256"
    echo ""
    echo "Size: $(du -h "${DIST_DIR}/${TARBALL_NAME}" | cut -f1)"
    echo ""
    log_info "SHA256: $(cat "${DIST_DIR}/${TARBALL_NAME}.sha256")"
}

# Run main function
main
