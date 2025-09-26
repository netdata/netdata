# Netdata MCP Bridge - Go Implementation

This Go bridge converts MCP stdio communication to Netdata's MCP over WebSocket.

## Requirements

- Go 1.16+
- github.com/coder/websocket package

## Installation

The easiest way to build the Go bridge is to use the included build script:

```bash
# Make the build script executable (if needed)
chmod +x build.sh

# Run the build script
./build.sh
```

The script will:
1. Check if Go is installed
2. Initialize a Go module if needed
3. Add required dependencies
4. Build the binary as `nd-mcp`

Alternatively, you can build it manually:

```bash
# Initialize a new module in this directory (only needed once)
go mod init netdata/nd-mcp-bridge

# Add dependencies
go get github.com/coder/websocket
go mod tidy

# Build the binary
go build -o nd-mcp nd-mcp.go
```

## Usage

```bash
./nd-mcp ws://<ip>:19999/mcp
```

Where `<ip>` is either `localhost` or the IP address where a Netdata instance is listening.

## Example with Claude Desktop

To use this bridge with Claude Desktop:

1. In Claude Desktop settings, configure the Custom Command option:

```bash
/path/to/stdio-golang/nd-mcp ws://localhost:19999/mcp
```

2. If your Netdata instance is running on a different machine, replace `localhost` with the appropriate IP address.

## How It Works

The bridge:
1. Establishes a WebSocket connection to the specified Netdata MCP endpoint
2. Reads from standard input and sends to the WebSocket
3. Receives messages from the WebSocket and writes to standard output
4. Handles both directions simultaneously
5. Automatically reconnects if the connection is lost, with exponential backoff

## Connection Reliability

This bridge implements robust connection handling:

- **Automatic Reconnection**: If the WebSocket connection is lost, the bridge will automatically attempt to reconnect
- **Exponential Backoff**: Reconnection attempts use exponential backoff with jitter to avoid overwhelming the server
- **Message Queuing**: Messages sent while disconnected are queued and delivered once reconnected
- **Connection Status Logging**: The bridge logs connection status to stderr for monitoring
- **Thread-Safe Operation**: Uses goroutines and mutexes to ensure thread-safe operation

The reconnection algorithm starts with a 1-second delay and doubles the wait time with each attempt, up to a maximum of 60 seconds. Random jitter is added to prevent connection storms from multiple clients reconnecting simultaneously.

## Protocol Compatibility

- Netdata MCP implements the JSON-RPC 2.0 protocol
- Messages that don't conform to the JSON-RPC 2.0 format are silently ignored
- The bridge passes messages directly without any modification

## Implementation Notes

This implementation:
- Uses the github.com/coder/websocket library for WebSocket communication
- Explicitly sets WebSocket headers including Sec-WebSocket-Key and Sec-WebSocket-Version
- Implements proper WebSocket handshake to ensure compatibility with Netdata's WebSocket server
- Sends and receives raw text messages directly (without JSON serialization/deserialization)
- Preserves the exact format of JSON-RPC 2.0 messages without adding extra quotes or escaping
- Uses Go's concurrency model with channels for efficient message processing