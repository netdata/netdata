#!/usr/bin/env bash

set -e

# Get the directory where this script resides
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "Building ai-agent..."
cd claude && npm install && npm run build
cd - > /dev/null

echo "Build completed successfully."
