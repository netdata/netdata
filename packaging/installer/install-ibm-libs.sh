#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
# This script downloads and installs IBM DB2 client libraries
# required for the ibm.d.plugin to work.

set -e

DRIVER_URL="https://public.dhe.ibm.com/ibmdl/export/pub/software/data/db2/drivers/odbc_cli/v11.5.9/linuxx64_odbc_cli.tar.gz"

# Determine the correct library directory based on Netdata installation
if [ -n "${NETDATA_PREFIX}" ]; then
    # Custom installation prefix
    DRIVER_DIR="${NETDATA_PREFIX}/lib/netdata/ibm-clidriver"
elif [ -d "/opt/netdata" ]; then
    # Static installation
    DRIVER_DIR="/opt/netdata/lib/netdata/ibm-clidriver"
elif [ -d "/usr/libexec/netdata" ]; then
    # System package installation
    DRIVER_DIR="/usr/lib/netdata/ibm-clidriver"
else
    # Default to /usr
    DRIVER_DIR="/usr/lib/netdata/ibm-clidriver"
fi

# Check if already installed
if [ -f "$DRIVER_DIR/lib/libdb2.so" ]; then
    echo "IBM DB2 client libraries are already installed in $DRIVER_DIR"
    exit 0
fi

echo "Installing IBM DB2 client libraries to: $DRIVER_DIR"

# Check for root permissions
if [ "$EUID" -ne 0 ]; then
    echo "ERROR: This script must be run as root"
    exit 1
fi

# Create directory
mkdir -p "$DRIVER_DIR"
cd "$DRIVER_DIR"

# Download
echo "Downloading IBM DB2 client libraries..."
if command -v wget >/dev/null 2>&1; then
    wget -q --show-progress -O clidriver.tar.gz "$DRIVER_URL" || {
        echo "ERROR: Failed to download IBM DB2 client libraries"
        exit 1
    }
elif command -v curl >/dev/null 2>&1; then
    curl -# -L -o clidriver.tar.gz "$DRIVER_URL" || {
        echo "ERROR: Failed to download IBM DB2 client libraries"
        exit 1
    }
else
    echo "ERROR: Neither wget nor curl found. Cannot download IBM libraries."
    echo "Please install wget or curl and try again."
    exit 1
fi

# Extract
echo "Extracting IBM DB2 client libraries..."
tar -xzf clidriver.tar.gz || {
    echo "ERROR: Failed to extract IBM DB2 client libraries"
    rm -f clidriver.tar.gz
    exit 1
}
rm -f clidriver.tar.gz

# Move clidriver contents to parent directory
if [ -d "clidriver" ]; then
    mv clidriver/* .
    rmdir clidriver
fi

# Set proper permissions
chown -R root:root "$DRIVER_DIR"
chmod -R 755 "$DRIVER_DIR"

echo "IBM DB2 client libraries installed successfully."
echo ""
echo "The ibm.d.plugin should now be able to connect to IBM DB2, AS/400, and WebSphere systems."
echo ""
echo "Note: You may need to restart the Netdata service for the changes to take effect:"
echo "  systemctl restart netdata    # For systemd systems"
echo "  /etc/init.d/netdata restart  # For SysV init systems"