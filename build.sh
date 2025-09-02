#!/usr/bin/env bash

set -euo pipefail

# Resolve repo root (directory containing this script)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "[build] Building Claude implementation (TypeScript → dist)"
# Install deps only if node_modules missing to avoid unnecessary network usage
if [ ! -d "claude/node_modules" ]; then
  (cd claude && npm install)
fi
(cd claude && npm run build)

echo "[build] Linting Claude implementation (ESLint)"
(cd claude && npm run lint)

echo "[build] Creating standalone binary with pkg"
# Install pkg if not already installed
if ! command -v pkg &> /dev/null; then
  echo "[build] Installing pkg globally..."
  npm install -g pkg
fi

# Install esbuild if not in node_modules
if [ ! -d "claude/node_modules/esbuild" ]; then
  (cd claude && npm install --save-dev esbuild)
fi

# Build the binary
(cd claude && node build-binary.js)

BINARY="$SCRIPT_DIR/ai-agent"
echo "[build] Binary created at $BINARY ($(du -h "$BINARY" | cut -f1))"

# Ensure global symlink in /usr/local/bin using sudo (always)
TARGET_LINK="/usr/local/bin/ai-agent"
echo "[build] Installing symlink with sudo: $TARGET_LINK -> $BINARY"
sudo ln -sf "$BINARY" "$TARGET_LINK"

echo "[build] OK -> ai-agent installed at $TARGET_LINK"
