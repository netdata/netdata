# MCP Headend

## TL;DR
Model Context Protocol server exposing ai-agent as MCP tool provider via stdio, HTTP, SSE, or WebSocket transports.

## Source Files
- `src/headends/mcp-headend.ts` - Full implementation
- `@modelcontextprotocol/sdk/server` - MCP server SDK
- `src/agent-registry.ts` - Agent registry
- `src/headends/mcp-ws-transport.ts` - WebSocket transport

## Headend Identity
- **Kind**: `mcp`
- **ID**: Computed from transport type (e.g., `mcp:stdio`, `mcp:http:8080`)

## Configuration

### McpHeadendOptions
```typescript
interface McpHeadendOptions {
  registry: AgentRegistry;       // Agent definitions
  instructions?: string;         // Server instructions
  transport: McpTransportSpec;   // Transport configuration
  concurrency?: number;          // Max concurrent executions
  verboseLogging?: boolean;      // Enable verbose logs
}
```

### McpTransportSpec
```typescript
type McpTransportSpec =
  | { type: 'stdio' }
  | { type: 'streamable-http'; port: number }
  | { type: 'sse'; port: number }
  | { type: 'ws'; port: number };
```

## Transport Types

### stdio
- Standard input/output pipes
- Single connection
- No concurrency limiting
- Suitable for CLI integration

### streamable-http
- HTTP streaming transport
- Multiple sessions via header
- Session-based concurrency
- Uses `mcp-session-id` header

### sse (Server-Sent Events)
- Legacy SSE transport
- Multiple concurrent clients
- Session tracking
- Backwards compatibility

### ws (WebSocket)
- WebSocket server
- Bi-directional streaming
- Session management
- Real-time communication

## Construction

**Location**: `src/headends/mcp-headend.ts:112-126`

```typescript
constructor(options) {
  this.registry = options.registry;
  this.instructions = options.instructions;
  this.transportSpec = options.transport;
  this.id = this.computeId(options.transport);
  this.label = this.describeLabel();
  this.closed = this.closeDeferred.promise;
  this.verboseLogging = options.verboseLogging;

  if (transport.type !== 'stdio') {
    const limit = options.concurrency ?? 10;
    this.limiter = new ConcurrencyLimiter(limit);
  }
}
```

## Server Instance Creation

### MCP Server Setup
```typescript
createServerInstance() {
  const server = new McpServer({
    name: 'ai-agent',
    version: '1.0.0'
  });

  // Register tool handlers
  this.registerToolHandlers(server);

  // Register instructions if provided
  if (this.instructions) {
    server.setInstructions(this.instructions);
  }

  return server;
}
```

### Tool Registration
For each agent in registry:
1. Create MCP tool definition
2. Map parameters to Zod schema
3. Register execution handler
4. Track tool-to-agent mapping

## Startup Flow

**Location**: `src/headends/mcp-headend.ts:132-170`

1. **Set context**:
   - Store HeadendContext
   - Attach shutdown signal
   - Set global stop reference

2. **Create transport**:
   ```typescript
   switch (transportSpec.type) {
     case 'stdio':
       transport = new StdioServerTransport();
       break;
     case 'streamable-http':
       await startHttpTransport(port);
       break;
     case 'sse':
       await startSseTransport(port);
       break;
     case 'ws':
       startWsTransport(port);
       break;
   }
   ```

3. **Connect server**:
   - For stdio: Single server instance
   - For network: Per-session instances

4. **Attach lifecycle handlers**:
   - onclose: Signal graceful close
   - onerror: Log and handle errors

## Session Management

### HTTP Sessions
```typescript
private readonly httpContexts = new Map<string, {
  transport: StreamableHTTPServerTransport;
  server: McpServer;
  release?: () => void;
}>();
```

Session header: `mcp-session-id`

### SSE Sessions
```typescript
private readonly sseContexts = new Map<string, {
  transport: SSEServerTransport;
  server: McpServer;
  release?: () => void;
}>();
```

### WebSocket Sessions
```typescript
private readonly wsContexts = new Map<string, {
  transport: McpWebSocketServerTransport;
  server: McpServer;
  release?: () => void;
}>();
```

## Concurrency Control

**Location**: Uses ConcurrencyLimiter

```typescript
private readonly limiter?: ConcurrencyLimiter;

// On new connection
const release = await this.limiter.acquire();
context.release = release;

// On connection close
context.release?.();
```

Default limit: 10 concurrent sessions (except stdio)

## Tool Execution

### Request Flow
1. MCP client calls tool
2. Server receives tool call
3. Map tool name to agent
4. Execute agent session
5. Return result to client

### Schema Adaptation
Uses `isSimplePromptSchema()` to detect simple prompt inputs:
- Type: object
- Properties: prompt (string), optional format
- No extra properties

## Shutdown Handling

**Location**: `src/headends/mcp-headend.ts:172-200`

```typescript
async stop() {
  if (this.stopping) return;
  this.stopping = true;

  // Close transport-specific resources
  switch (transportSpec.type) {
    case 'stdio':
      await this.closeServer(this.server);
      await this.stdioTransport.close();
      break;
    case 'streamable-http':
      await this.stopHttpTransport();
      break;
    case 'sse':
      await this.stopSseTransport();
      break;
    case 'ws':
      await this.stopWsTransport();
      break;
  }

  this.signalClosed({ reason: 'stopped', graceful: true });
}
```

## Signal Handling

```typescript
handleShutdownSignal() {
  // Triggered by abort signal
  // Initiate graceful shutdown
  this.stop();
}
```

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `registry` | Available agents as tools |
| `instructions` | Server-level instructions |
| `transport` | Communication method |
| `concurrency` | Max concurrent sessions |
| `verboseLogging` | Detailed log output |

## Telemetry

**Labels**:
- Headend ID
- Transport type
- Base telemetry labels

## Logging

**Via context.log**:
- Startup/shutdown events
- Transport errors
- Session lifecycle
- Tool execution

**Severity levels**:
- `VRB`: Startup progress
- `ERR`: Fatal errors (with fatal=true)

## Events

**Headend lifecycle**:
- `closed`: Promise resolving on termination
- Graceful vs error termination
- Socket cleanup tracking

## Invariants

1. **Single stdio**: Only one stdio connection
2. **Session isolation**: Network transports have per-session servers
3. **Concurrency enforcement**: Network connections limited
4. **Graceful cleanup**: All sessions closed on stop
5. **Signal propagation**: Abort signal triggers shutdown
6. **Schema validation**: Tool schemas via Zod

## Business Logic Coverage (Verified 2025-11-16)

- **Format enforcement**: Every MCP tool definition includes a `format` enum; at runtime `parseCallPayload` rejects tool calls missing `format`, and JSON mode requires a `schema` field before forwarding to the session (`src/headends/mcp-headend.ts:298-420`).
- **Session maps**: HTTP/SSE/WS transports maintain session maps keyed by `mcp-session-id`, each storing its own `McpServer` instance and limiter release callback so reconnects can't leak concurrency slots (`src/headends/mcp-headend.ts:188-287`).
- **Instruction plumbing**: Static `instructions` strings (from config or CLI) propagate to the MCP server and appear in `getInstructions()`, keeping remote IDEs aligned with agent prompts (`src/headends/mcp-headend.ts:132-210`).
- **Verbose tracing**: When `verboseLogging` is true, the headend emits per-request logs for `tools/list`, `tools/call`, and transport events, enabling debugging without enabling global CLI verbose mode (`src/headends/mcp-headend.ts:58-110, 420-470`).

## Test Coverage

**Phase 2**:
- Transport creation
- Tool registration
- Session management
- Shutdown procedures
- Schema adaptation

**Gaps**:
- Multi-client scenarios
- Connection recovery
- Large response handling
- Error propagation accuracy

## Troubleshooting

### Tool not appearing
- Check agent registry
- Verify tool registration
- Review schema conversion

### Connection refused
- Check port availability
- Verify transport type
- Review firewall settings

### Session timeout
- Check concurrency limits
- Verify limiter release
- Review session cleanup

### Shutdown hanging
- Check active sessions
- Verify socket closure
- Review signal handlers
