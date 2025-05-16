# Netdata MCP Bridges

This directory contains bridge programs that convert MCP stdio communication to Netdata's MCP over WebSocket. These bridges allow you to use standard I/O based AI models and tools with Netdata's Model Context Protocol.

## Available Bridges

Three implementations are provided to accommodate different environments, each in its own subdirectory:

- [`stdio-python/`](stdio-python/) - Python implementation
- [`stdio-nodejs/`](stdio-nodejs/) - JavaScript (Node.js) implementation 
- [`stdio-golang/`](stdio-golang/) - Go implementation

Each subdirectory contains:
- The bridge implementation
- A README.md with detailed installation and usage instructions specific to that implementation

## Common Usage

All bridges follow the same usage pattern, accepting a WebSocket URL as their only parameter:

```
<bridge-program> ws://<ip>:19999/mcp
```

Where `<ip>` is either `localhost` or the IP address where a Netdata instance is listening.

The bridge reads from standard input and writes to standard output, allowing you to pipe AI model output through it to connect with Netdata MCP.

## Claude Desktop Configuration

To use Claude Desktop with Netdata's MCP, you can configure it to use one of these bridges. Here's how to set it up:

1. Select and install one of the bridge implementations from the subdirectories
2. In Claude Desktop settings, configure the Custom Command option to point to your chosen bridge
3. If your Netdata instance is running on a different machine, replace `localhost` with the appropriate IP address

This configuration allows Claude Desktop to communicate directly with your Netdata instance, providing it with contextual information from your monitoring environment.

For detailed instructions specific to each implementation, refer to the README.md in the corresponding subdirectory.

## Connection Reliability

All bridge implementations include robust connection handling features:

- **Automatic Reconnection**: If the WebSocket connection is lost for any reason, the bridges will automatically attempt to reconnect
- **Exponential Backoff**: Reconnection attempts use exponential backoff with jitter to avoid overwhelming the server
- **Message Queuing**: Messages sent while disconnected are queued and delivered once reconnected
- **Connection Status Logging**: All bridges log connection status to stderr for monitoring

These features ensure that temporary network issues or Netdata server restarts don't disrupt your AI model's integration, providing a resilient connection that automatically recovers from failures.

## Protocol Compatibility

Netdata MCP implements the JSON-RPC 2.0 protocol for communication. The bridges pass messages directly between the AI model's standard I/O and the WebSocket connection without modification.

Important compatibility notes:
- Netdata MCP expects JSON-RPC 2.0 compatible messages
- Messages that don't conform to the JSON-RPC 2.0 format are silently ignored
- Ensure your AI model or tool produces properly formatted JSON-RPC 2.0 messages for full compatibility
