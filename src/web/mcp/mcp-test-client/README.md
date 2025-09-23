# Netdata MCP Web Client

A web-based client for testing and interacting with Netdata's Model Context Protocol (MCP) server over WebSocket, streamable HTTP, or Server-Sent Events (SSE).

## Features

- **Multi-transport support**: Connect to MCP over WebSocket, HTTP chunked responses, or SSE
- **Schema Validation**: Validates tool schemas against MCP specification
- **Custom UI Generator**: Lightweight form generator for tool parameters
- **JSON Pretty Printing**: Advanced formatting with syntax highlighting
- **Request/Response Logging**: Full JSON-RPC message logging with colors
- **Tool Discovery**: Automatically discovers and lists available tools

## Files

- `index.html` - Main web client interface
- `mcp-schema-ui-generator.js` - Custom JSON Schema form generator
- `json-pretty-printer.js` - JSON formatter with syntax highlighting

## Usage

1. Open `index.html` in a web browser
2. Enter your MCP endpoint URL (defaults to `ws://localhost:19999/mcp`)
   - WebSocket URLs (`ws://` / `wss://`) connect automatically over WebSocket
   - HTTP/HTTPS URLs show a selector to choose between **Streamable HTTP** and **SSE**
3. Click "Connect" or "Connect and Handshake" to run the full capability discovery flow
4. Use the interface to:
   - Initialize the connection and fetch tool, prompt, and resource lists automatically
   - Call tools with parameters
   - View formatted responses

## Features Details

### Schema UI Generator
- Validates schemas against MCP specification
- Generates forms for all JSON Schema types
- Supports arrays, objects, enums, and constraints
- Shows validation errors clearly

### JSON Pretty Printer
- Syntax highlighting with colors
- Visualizes newlines in strings (`\n` shown as badges)
- Detects and formats nested JSON in string values
- Handles large, complex JSON structures

### WebSocket Client
- Full JSON-RPC 2.0 support
- Request history tracking
- Dynamic tool discovery
- Tab-based interface for raw JSON and form editing

## Development

To extend or modify the client:

1. **Add new flows**: Edit the `flows` object in `index.html`
2. **Customize colors**: Modify the color scheme in `json-pretty-printer.js`
3. **Enhance validation**: Update schema validation in `mcp-schema-ui-generator.js`

## Browser Requirements

- Modern browser with WebSocket support
- JavaScript enabled
- No external dependencies required
