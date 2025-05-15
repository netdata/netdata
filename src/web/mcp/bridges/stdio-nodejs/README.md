# Netdata MCP Bridge - Node.js Implementation

This Node.js bridge converts MCP stdio communication to Netdata's MCP over WebSocket.

## Requirements

- Node.js 14+
- ws library

## Installation

The easiest way to set up the Node.js bridge is to use the included build script:

```bash
# Make the build script executable (if needed)
chmod +x build.sh

# Run the build script
./build.sh
```

The script will:
1. Check if Node.js is installed
2. Create a package.json file if it doesn't exist
3. Install the required ws dependency

Alternatively, you can set it up manually:

```bash
# Create a package.json file (optional)
npm init -y

# Install the WebSocket library
npm install ws --save
```

## Usage

The script can be run directly as an executable (the shebang line will use the system's Node.js):

```bash
./nd-mcp.js ws://<ip>:19999/mcp
```

Or explicitly with Node.js:

```bash
node nd-mcp.js ws://<ip>:19999/mcp
```

Where `<ip>` is either `localhost` or the IP address where a Netdata instance is listening.

## Example with Claude Desktop

To use this bridge with Claude Desktop:

1. In Claude Desktop settings, configure the Custom Command option:

```bash
node /path/to/stdio-nodejs/nd-mcp.js ws://localhost:19999/mcp
```

2. If your Netdata instance is running on a different machine, replace `localhost` with the appropriate IP address.

## How It Works

The bridge:
1. Establishes a WebSocket connection to the specified Netdata MCP endpoint
2. Reads from standard input and sends to the WebSocket
3. Receives messages from the WebSocket and writes to standard output
4. Handles both directions simultaneously

## Protocol Compatibility

- Netdata MCP implements the JSON-RPC 2.0 protocol
- Messages that don't conform to the JSON-RPC 2.0 format are silently ignored
- The bridge passes messages directly without any modification