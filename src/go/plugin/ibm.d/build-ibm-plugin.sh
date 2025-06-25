#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# Build script for ibm.d.plugin
# This plugin requires CGO and IBM DB2 client libraries
# It cannot be built with the regular Go build process

set -euo pipefail

# Function to check if Go is installed
check_go() {
    if ! command -v go >/dev/null 2>&1; then
        echo "ERROR: Go is not installed or not in PATH"
        echo "Please install Go 1.19 or later from https://golang.org/dl/"
        exit 1
    fi
    
    local go_version=$(go version | awk '{print $3}' | sed 's/go//')
    local major_version=$(echo $go_version | cut -d. -f1)
    local minor_version=$(echo $go_version | cut -d. -f2)
    
    if [ "$major_version" -lt 1 ] || ([ "$major_version" -eq 1 ] && [ "$minor_version" -lt 19 ]); then
        echo "ERROR: Go version $go_version is too old. Go 1.19 or later is required."
        exit 1
    fi
    
    echo "Go version $go_version found"
}

# Function to check if GCC is installed
check_gcc() {
    if ! command -v gcc >/dev/null 2>&1; then
        echo "ERROR: GCC is not installed"
        echo "Please install GCC:"
        echo "  Ubuntu/Debian: sudo apt-get install gcc build-essential"
        echo "  CentOS/RHEL:   sudo yum install gcc gcc-c++"
        echo "  Fedora:        sudo dnf install gcc gcc-c++"
        exit 1
    fi
    echo "GCC found: $(gcc --version | head -n1)"
}

# Function to download and setup IBM DB2 CLI driver
setup_clidriver() {
    local temp_dir=$(mktemp -d)
    local clidriver_path
    
    echo "Setting up IBM DB2 CLI driver..."
    
    # Check if we already have a proper clidriver installation
    if [ -n "${IBM_DB_HOME:-}" ] && [ -d "$IBM_DB_HOME/lib" ] && [ -d "$IBM_DB_HOME/include" ]; then
        echo "Using existing IBM_DB_HOME: $IBM_DB_HOME"
        return 0
    fi
    
    # Try to find existing clidriver in Go module cache
    local go_mod_cache=$(go env GOMODCACHE 2>/dev/null || echo "$HOME/go/pkg/mod")
    clidriver_path="$go_mod_cache/github.com/ibmdb/clidriver"
    
    if [ -d "$clidriver_path/lib" ] && [ -d "$clidriver_path/include" ]; then
        echo "Found existing clidriver in Go module cache: $clidriver_path"
        export IBM_DB_HOME="$clidriver_path"
        return 0
    fi
    
    # Download using the IBM Go driver installer
    echo "Downloading IBM DB2 CLI driver with development headers..."
    
    cd "$temp_dir"
    
    # Install the IBM driver installer
    go install github.com/ibmdb/go_ibm_db/installer@latest
    
    # Run the installer to download clidriver
    local installer_path=$(go env GOPATH 2>/dev/null || echo "$HOME/go")/bin/installer
    if [ ! -f "$installer_path" ]; then
        installer_path="$HOME/go/bin/installer"
    fi
    
    if [ ! -f "$installer_path" ]; then
        echo "ERROR: Could not find IBM installer binary"
        echo "Please ensure GOPATH/bin is in your PATH or run:"
        echo "  go install github.com/ibmdb/go_ibm_db/installer@latest"
        exit 1
    fi
    
    # Run installer which downloads to Go module cache
    "$installer_path"
    
    # Find the downloaded clidriver
    clidriver_path="$go_mod_cache/github.com/ibmdb/clidriver"
    
    if [ ! -d "$clidriver_path" ] || [ ! -d "$clidriver_path/include" ]; then
        echo "ERROR: Failed to download IBM DB2 CLI driver with headers"
        echo "Please manually download and install IBM Data Server Driver Package"
        echo "from https://www.ibm.com/support/pages/db2-data-server-drivers"
        exit 1
    fi
    
    export IBM_DB_HOME="$clidriver_path"
    echo "Successfully downloaded IBM DB2 CLI driver to: $IBM_DB_HOME"
    
    # Cleanup
    rm -rf "$temp_dir"
}

# Check prerequisites
echo "Checking build prerequisites..."
check_go
check_gcc

# Setup IBM DB2 CLI driver
setup_clidriver

echo "Building ibm.d.plugin with CGO enabled..."
echo "IBM_DB_HOME: $IBM_DB_HOME"

# Set build environment
export CGO_ENABLED=1
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib"
export LD_LIBRARY_PATH="$IBM_DB_HOME/lib:${LD_LIBRARY_PATH:-}"

# Get the directory of this script
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
# Go to the Go source root  
cd "$SCRIPT_DIR/../.."

# Verify we're in the right directory
if [ ! -f "go.mod" ] || [ ! -d "cmd/ibmdplugin" ]; then
    echo "ERROR: Not in the correct Go module directory"
    echo "Expected to find go.mod and cmd/ibmdplugin directory"
    echo "Current directory: $(pwd)"
    exit 1
fi

# Build the plugin
echo "Building ibm.d.plugin..."
echo "Build environment:"
echo "  CGO_ENABLED=$CGO_ENABLED"
echo "  CGO_CFLAGS=$CGO_CFLAGS"
echo "  CGO_LDFLAGS=$CGO_LDFLAGS"
echo "  LD_LIBRARY_PATH=$LD_LIBRARY_PATH"
echo ""

# Build with timeout to avoid hanging
timeout 600 go build -v -o ibm.d.plugin ./cmd/ibmdplugin

if [ -f "ibm.d.plugin" ]; then
    echo ""
    echo "✅ Build successful!"
    echo ""
    echo "Plugin details:"
    echo "  Size: $(du -h ibm.d.plugin | cut -f1)"
    echo "  Location: $(pwd)/ibm.d.plugin"
    echo ""
    echo "To install the plugin:"
    echo "  sudo cp $(pwd)/ibm.d.plugin /usr/libexec/netdata/plugins.d/"
    echo "  sudo chmod +x /usr/libexec/netdata/plugins.d/ibm.d.plugin"
    echo ""
    echo "Runtime environment setup:"
    echo "  export IBM_DB_HOME=$IBM_DB_HOME"
    echo "  export LD_LIBRARY_PATH=$IBM_DB_HOME/lib:\$LD_LIBRARY_PATH"
    echo ""
    echo "For systemd (add to netdata service environment):"
    echo "  Environment=\"IBM_DB_HOME=$IBM_DB_HOME\""
    echo "  Environment=\"LD_LIBRARY_PATH=$IBM_DB_HOME/lib\""
    echo ""
    echo "Test the plugin:"
    echo "  export IBM_DB_HOME=$IBM_DB_HOME"
    echo "  export LD_LIBRARY_PATH=$IBM_DB_HOME/lib:\$LD_LIBRARY_PATH"
    echo "  ./ibm.d.plugin -h"
    echo ""
    echo "Configuration files:"
    echo "  /etc/netdata/ibm.d.conf (main config)"
    echo "  /etc/netdata/ibm.d/db2.conf (DB2 collector)"
    echo "  /etc/netdata/ibm.d/as400.conf (AS/400 collector)"
else
    echo ""
    echo "❌ Build failed!"
    echo ""
    echo "Common issues:"
    echo "  1. Missing unused import in main.go - this should be fixed in source"
    echo "  2. Missing CGO dependencies - ensure GCC is installed"
    echo "  3. IBM DB2 headers not found - check IBM_DB_HOME path"
    echo ""
    echo "Debug with:"
    echo "  go build -v -x ./cmd/ibmdplugin"
    exit 1
fi