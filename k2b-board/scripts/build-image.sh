#!/usr/bin/env bash
# Build the Armbian image for K2B with application overlay.
# Run from the repo root. Requires Docker.
# Usage: ./build-image.sh [--no-cache]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE_DIR="${SCRIPT_DIR}/image"
ARMBIAN_DIR="${IMAGE_DIR}/armbian-build"

# Ensure the application binary is built and placed in overlay
if [ ! -f "${IMAGE_DIR}/userpatches-overlay/overlay/usr/local/bin/app" ]; then
    echo "ERROR: No application binary in overlay. Build it first:"
    echo "  cross build --release -p my-app --target aarch64-unknown-linux-gnu"
    echo "  cp target/aarch64-unknown-linux-gnu/release/my-app ${IMAGE_DIR}/userpatches-overlay/overlay/usr/local/bin/app"
    exit 1
fi

echo "=== Building Armbian image for KickPi K2B V2 ==="
cd "$ARMBIAN_DIR"

./compile.sh \
    BOARD=kickpi-k2b-v2 \
    BRANCH=current \
    RELEASE=noble \
    BUILD_MINIMAL=yes \
    BUILD_DESKTOP=no \
    KERNEL_CONFIGURE=no \
    USERPATCHES_PATH="${IMAGE_DIR}/userpatches" \
    "$@"

echo ""
echo "Image built. Check: ${IMAGE_DIR}/armbian-build/output/images/"
