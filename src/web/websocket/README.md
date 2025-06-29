# WebSocket

Netdata supports WebSocket connections for real-time data streaming and interactive features.

## WebSocket Protocols

Netdata supports the following WebSocket protocols:

- `echo`: A simple echo protocol for testing WebSocket functionality (available only when compiled with internal checks)
- `mcp`: Model-Context Protocol for AI/ML interactions

These protocols can be specified either in the WebSocket protocol header during the handshake or through the URL path (e.g., `ws://localhost:19999/echo`).

## Configuration Options

### Maximum Frame Size

WebSocket communication is done through frames, with most browsers having limitations on maximum frame size (typically around 16MB).

#### Global Configuration

Netdata sets a default outgoing frame size limit of 4MB to ensure browser compatibility while maintaining good performance.

For larger messages, Netdata automatically fragments them into multiple frames for transmission.

#### Per-Connection Configuration

Users can customize the maximum outgoing frame size on a per-connection basis by adding the `max_frame_size` parameter to the WebSocket URL:

```
ws://localhost:19999/echo?max_frame_size=32768
```

This allows specific clients to set their preferred frame size, which is especially useful for:

- Resource-constrained devices that may need smaller frames
- Environments with specific network limitations
- Custom applications with specific buffer size requirements

The accepted range for `max_frame_size` is between 1KB (1024 bytes) and 20MB. Values outside this range will be automatically adjusted to the nearest bound.

### WebSocket Compression

Netdata supports permessage-deflate compression per RFC 7692. Compression is negotiated during the WebSocket handshake and can significantly reduce bandwidth usage for text-based protocols.

The compression settings can be configured through the standard WebSocket extension negotiation mechanism.

## Limitations

- Control frames (ping, pong, close) cannot be compressed and must be less than 125 bytes
- Control frames cannot be fragmented
- The maximum size for decompressed incoming messages is 200MB
- Incoming frames are limited to 20MB

## Examples

### Basic Connection
```javascript
// Connect to echo protocol
const ws = new WebSocket('ws://localhost:19999/echo');

// Event handlers
ws.onopen = () => console.log('Connected');
ws.onmessage = (event) => console.log('Received:', event.data);
ws.onerror = (error) => console.error('Error:', error);
ws.onclose = () => console.log('Disconnected');

// Send a message
ws.send('Hello, Netdata!');
```

### Connection with Custom Frame Size
```javascript
// Connect with smaller frame size (32KB)
const ws = new WebSocket('ws://localhost:19999/echo?max_frame_size=32768');

// Send a large message (will be automatically fragmented)
const largeMessage = new Array(1000000).join('a');
ws.send(largeMessage);
```

## Debugging

Netdata logs WebSocket connection details including compression settings and frame size limits at the DEBUG log level. To view these logs, run Netdata with increased verbosity or check the error log file.
