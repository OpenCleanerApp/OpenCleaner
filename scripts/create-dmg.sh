#!/bin/bash
set -euo pipefail

# Create DMG for OpenCleaner distribution
# Usage: ./scripts/create-dmg.sh [path/to/OpenCleaner.app] [output-dir]

APP_PATH="${1:-build/export/OpenCleaner.app}"
OUTPUT_DIR="${2:-.}"
APP_NAME="OpenCleaner"
VERSION=$(defaults read "$(pwd)/${APP_PATH}/Contents/Info.plist" CFBundleShortVersionString 2>/dev/null || echo "0.0.0")
DMG_NAME="${APP_NAME}-${VERSION}.dmg"
DMG_PATH="${OUTPUT_DIR}/${DMG_NAME}"
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "${TEMP_DIR}"' EXIT

echo "==> Creating DMG for ${APP_NAME} v${VERSION}"
echo "    App: ${APP_PATH}"
echo "    Output: ${DMG_PATH}"

if [ ! -d "${APP_PATH}" ]; then
    echo "ERROR: ${APP_PATH} not found"
    exit 1
fi

mkdir -p "${TEMP_DIR}/${APP_NAME}"
cp -R "${APP_PATH}" "${TEMP_DIR}/${APP_NAME}/"
ln -s /Applications "${TEMP_DIR}/${APP_NAME}/Applications"

hdiutil create \
    -volname "${APP_NAME}" \
    -srcfolder "${TEMP_DIR}/${APP_NAME}" \
    -ov -format UDZO -imagekey zlib-level=9 \
    "${DMG_PATH}"

echo "==> DMG created: ${DMG_PATH}"
echo "    SHA-256: $(shasum -a 256 "${DMG_PATH}" | awk '{print $1}')"
