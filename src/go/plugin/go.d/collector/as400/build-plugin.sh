#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# Build script for as400-db2.plugin
# This plugin requires CGO and IBM DB2 client libraries

set -euo pipefail

# Check for IBM_DB_HOME
if [ -z "${IBM_DB_HOME:-}" ]; then
    echo "ERROR: IBM_DB_HOME environment variable is not set"
    echo "Please set it to the path of your IBM DB2 client installation"
    echo "Example: export IBM_DB_HOME=/opt/ibm/db2/V11.5"
    exit 1
fi

# Check if DB2 client libraries exist
if [ ! -d "$IBM_DB_HOME/lib" ] || [ ! -d "$IBM_DB_HOME/include" ]; then
    echo "ERROR: IBM DB2 client libraries not found at $IBM_DB_HOME"
    echo "Please ensure IBM DB2 client is properly installed"
    exit 1
fi

echo "Building as400-db2.plugin with CGO enabled..."
echo "IBM_DB_HOME: $IBM_DB_HOME"

# Set build environment
export CGO_ENABLED=1
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib"
export LD_LIBRARY_PATH="$IBM_DB_HOME/lib:${LD_LIBRARY_PATH:-}"

# Get the directory of this script
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
# Go to the Go source root
cd "$SCRIPT_DIR/../../../.."

# Build the plugin
echo "Building as400-db2.plugin..."
go build -v -o as400-db2.plugin ./cmd/as400db2plugin

if [ -f "as400-db2.plugin" ]; then
    echo "Build successful!"
    echo ""
    echo "To install the plugin:"
    echo "  sudo cp as400-db2.plugin /usr/libexec/netdata/plugins.d/"
    echo "  sudo chmod +x /usr/libexec/netdata/plugins.d/as400-db2.plugin"
    echo ""
    echo "To run the plugin, ensure LD_LIBRARY_PATH includes DB2 libraries:"
    echo "  export LD_LIBRARY_PATH=$IBM_DB_HOME/lib:\$LD_LIBRARY_PATH"
else
    echo "Build failed!"
    exit 1
fi