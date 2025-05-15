#!/bin/bash
set -e

# Get the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "Working directory: $SCRIPT_DIR"

# Check if Node.js is installed
if ! command -v node &> /dev/null; then
    echo "Error: Node.js is not installed or not in the PATH"
    echo "Please install Node.js from https://nodejs.org/"
    exit 1
fi

echo "Node.js is installed: $(node --version)"
echo "npm version: $(npm --version)"

# Check if package.json exists, if not create it
if [ ! -f "package.json" ]; then
    echo "Creating package.json..."
    npm init -y
    
    # Update package description and add script
    tmp=$(mktemp)
    jq '.description = "Netdata MCP WebSocket bridge for Node.js" | 
        .scripts.start = "node nd-mcp.js" | 
        .private = true' package.json > "$tmp" && mv "$tmp" package.json
    
    echo "Created package.json"
else
    echo "package.json already exists"
fi

# Install dependencies if not already installed
if [ ! -d "node_modules" ]; then
    echo "Installing dependencies..."
    npm install ws --save
else
    echo "Dependencies already installed"
    echo "To reinstall dependencies, delete the node_modules directory and run this script again"
fi

# Get the full path to the script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_PATH="$SCRIPT_DIR/nd-mcp.js"

# Make sure the script is executable
chmod +x "$SCRIPT_PATH"

echo "Build complete!"
echo ""
echo "You can now run the bridge using the full path:"
echo "$SCRIPT_PATH ws://ip:19999/mcp"
echo ""
echo "Or using Node.js with the full path:"
echo "node $SCRIPT_PATH ws://ip:19999/mcp"
echo ""
echo "Or from this directory:"
echo "./nd-mcp.js ws://ip:19999/mcp"