#!/bin/bash
set -e

# Get the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "Working directory: $SCRIPT_DIR"

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed or not in the PATH"
    echo "Please install Go from https://golang.org/doc/install"
    exit 1
fi

echo "Go is installed: $(go version)"
echo "Creating Go module for Netdata MCP bridge..."

# Initialize Go module if it doesn't exist
if [ ! -f "go.mod" ]; then
  go mod init netdata/nd-mcp-bridge
  echo "Initialized new Go module"
else
  echo "Go module already exists"
fi

# Add required dependencies
echo "Adding dependencies..."
go get github.com/coder/websocket
go mod tidy

# Build the binary
echo "Building nd-mcp binary..."
go build -o nd-mcp nd-mcp.go

# Make binary executable
chmod +x nd-mcp

# Get the full path to the executable
EXECUTABLE_PATH="$SCRIPT_DIR/nd-mcp"

echo "Build complete!"
echo ""
echo "You can now run the bridge using the full path:"
echo "$EXECUTABLE_PATH ws://ip:19999/mcp"
echo ""
echo "Or from this directory:"
echo "./nd-mcp ws://ip:19999/mcp"