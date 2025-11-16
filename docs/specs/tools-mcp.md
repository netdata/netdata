# MCP Tool Provider

## TL;DR
Model Context Protocol client managing multiple servers with shared registry, health probes, automatic restarts, and tool namespacing.

## Source Files
- `src/tools/mcp-provider.ts` - Full implementation (1269 lines)
- `src/types.ts` - MCPServerConfig, MCPServer, MCPTool definitions
- `src/websocket-transport.ts` - WebSocket transport creation
- `src/utils/process-tree.ts` - Process termination

## Data Structures

### MCPServerConfig
```typescript
interface MCPServerConfig {
  type: 'stdio' | 'websocket' | 'http' | 'sse';
  command?: string;           // For stdio
  args?: string[];            // For stdio
  env?: Record<string, string>; // For stdio
  url?: string;               // For websocket/http/sse
  headers?: Record<string, string>; // For websocket/http/sse
  toolsAllowed?: string[];    // Tool whitelist
  toolsDenied?: string[];     // Tool blacklist
  shared?: boolean;           // Use shared registry (default true)
  healthProbe?: 'ping' | 'listTools'; // Health check method
  queue?: string;             // Queue name for concurrency
}
```

### MCPServer
```typescript
interface MCPServer {
  name: string;
  config: MCPServerConfig;
  tools: MCPTool[];
  instructions: string;
}
```

### MCPTool
```typescript
interface MCPTool {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
  instructions?: string;
}
```

## Architecture

### Provider Class
**Location**: `src/tools/mcp-provider.ts:697-1268`

```typescript
class MCPProvider extends ToolProvider {
  readonly kind = 'mcp';
  private serversConfig: Record<string, MCPServerConfig>;
  private clients: Map<string, Client>;
  private servers: Map<string, MCPServer>;
  private failedServers: Set<string>;
  private processes: Map<string, MCPProcessHandle>;
  private sharedHandles: Map<string, SharedRegistryHandle>;
  private toolNameMap: Map<string, { serverName: string; originalName: string }>;
  private toolQueue: Map<string, string>;
}
```

### Shared Registry
**Location**: `src/tools/mcp-provider.ts:136-592`

Singleton pattern for shared MCP server connections:
- Reference counting
- Automatic restart on failure
- Health probing
- Process lifecycle management

## Transport Types

### stdio
- Spawns local process
- Pipes stdin/stdout
- Tracks PID for termination
- Captures stderr for logging

### websocket
- Connects to WebSocket URL
- Supports custom headers
- No process tracking

### http
- Streamable HTTP transport
- Custom request headers
- No process tracking

### sse (Server-Sent Events)
- Legacy SSE transport
- Custom fetch for headers
- No process tracking

## Tool Naming Convention

**Format**: `{namespace}__{originalName}`

Example:
- Server: `github`
- Tool: `search_code`
- Exposed: `github__search_code`

## Initialization

### Provider Construction
**Location**: `src/tools/mcp-provider.ts:717-736`

```typescript
constructor(namespace, servers, opts?) {
  this.serversConfig = servers;
  this.trace = opts?.trace;
  this.verbose = opts?.verbose;
  this.requestTimeoutMs = opts?.requestTimeoutMs;
  this.onLog = opts?.onLog;
  this.initConcurrency = opts?.initConcurrency;
  this.sharedRegistry = opts?.sharedRegistry ?? defaultSharedRegistry;
}
```

### Warmup Process
**Location**: `src/tools/mcp-provider.ts:780-870`

1. Check initialization state
2. Create initialization promise (prevent concurrent)
3. For each server config:
   - Acquire semaphore (respect concurrency limit)
   - Initialize server (shared or dedicated)
   - Log success/failure with timing
4. Track failed servers
5. Mark initialized

### Server Initialization
**Location**: `src/tools/mcp-provider.ts:876-997`

**Shared servers**:
1. Acquire from shared registry
2. Registry handles:
   - Transport creation
   - Client connection
   - Tool listing
   - Instructions gathering
3. Store handle for release later

**Dedicated servers**:
1. Create MCP Client
2. Create appropriate transport
3. Connect client to transport
4. Track process PID (for stdio)
5. List tools and filter
6. Gather prompts for instructions
7. Build tool name map with queue assignments

## Tool Filtering

**Location**: `src/tools/mcp-provider.ts:594-642`

```typescript
filterToolsForServer(name, config, tools) {
  const allowed = normalize(config.toolsAllowed);  // Default: ['*']
  const denied = normalize(config.toolsDenied);   // Default: []

  return tools.filter((tool) => {
    if (!isAllowed(tool.name)) return false;
    if (isDenied(tool.name)) return false;
    return true;
  });
}
```

Supports:
- Wildcard (`*` or `any`)
- Case-insensitive matching
- Allow/deny list combinations

## Tool Execution

**Location**: `src/tools/mcp-provider.ts:1135-1180`

1. Ensure initialized
2. Resolve tool mapping
3. Get client (shared or dedicated)
4. Build request options (timeout)
5. Call tool via MCP client
6. Normalize result to text

### Result Normalization
**Location**: `src/tools/mcp-provider.ts:1182-1209`

```typescript
normalizeToolResult(res) {
  // Extract content array
  // Map text parts
  // Join into single string
  // Include raw JSON for extras
  return { ok, result, latencyMs, kind, namespace, extras };
}
```

## Shared Registry

### Acquisition
**Location**: `src/tools/mcp-provider.ts:141-164`

```typescript
async acquire(serverName, config, opts) {
  // Check not shutting down
  // Get existing entry or create new
  // Handle concurrent initialization
  // Increment reference count
  return SharedServerHandle;
}
```

### Health Probing
**Location**: `src/tools/mcp-provider.ts:574-591`

```typescript
async runProbe(entry, logger) {
  // Try ping first (if available)
  // Fall back to listTools
  // Timeout: 3000ms
  return healthy;
}
```

### Restart Strategy
**Location**: `src/tools/mcp-provider.ts:265-338`

Backoff schedule: `[0, 1000, 2000, 5000, 10000, 30000, 60000]` ms

Process:
1. Detect probe failure or transport exit
2. Start restart loop (single instance)
3. For each attempt:
   - Apply backoff delay
   - Kill existing process
   - Close client and transport
   - Reinitialize entry
   - Resolve latch for waiting callers
4. Loop continues until success

### Transport Exit Handling
**Location**: `src/tools/mcp-provider.ts:239-255`

```typescript
handleTransportExit(entry) {
  // Check not shutting down
  // Check entry still registered
  // Check not managed close
  // Schedule restart loop
}
```

## Error Types

### MCPRestartError
Base class for restart-related errors

### MCPRestartFailedError
Restart attempt failed permanently (code: `mcp_restart_failed`)

### MCPRestartInProgressError
Restart currently running (code: `mcp_restart_in_progress`)

## Cancellation

**Location**: `src/tools/mcp-provider.ts:1114-1133`

```typescript
async cancelTool(name, opts) {
  const mapping = this.toolNameMap.get(name);
  const reason = opts?.reason ?? 'timeout';

  // For shared: delegate to handle
  // For dedicated stdio: restart server
}
```

## Instructions

**Location**: `src/tools/mcp-provider.ts:1211-1238`

Combines:
- Server-level instructions from MCP
- Prompts from server (if any)
- Tool-specific instructions

Format:
```
#### MCP Server: {name}
{server instructions}

##### Tool: {namespace}__{tool}
{tool instructions}
```

## Cleanup

**Location**: `src/tools/mcp-provider.ts:1240-1267`

1. Close all clients
2. Release shared handles
3. Kill tracked processes
4. Clear all maps
5. Reset initialization state

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `type` | Transport selection |
| `command/args/env` | Stdio process setup |
| `url/headers` | Network transport config |
| `toolsAllowed/toolsDenied` | Tool filtering |
| `shared` | Shared vs dedicated connection |
| `healthProbe` | Probe method (ping/listTools) |
| `queue` | Concurrency queue assignment |

## Telemetry

**Per server initialization**:
- Elapsed time
- Success/failure status
- Tool count
- Error details

**Per tool execution**:
- Latency
- Success status
- Raw request/response

## Logging

**Severity levels**:
- `TRC`: Initialization details, tool lists
- `VRB`: Filtered tools, latency summaries
- `WRN`: Failed servers, restart attempts
- `ERR`: Connection failures, probe failures

**Remote identifier format**: `mcp:{serverName}`

## Events

**Logged events**:
- Server initialization (start/end)
- Tool listing
- Probe success/failure
- Restart attempts
- Process termination

## Invariants

1. **Tool namespacing**: All tools prefixed with server namespace
2. **Initialization order**: Servers initialized before tool execution
3. **Shared by default**: config.shared !== false means shared
4. **Restart isolation**: Dedicated restarts isolated, shared coordinated
5. **Process tracking**: Stdio PIDs tracked for cleanup
6. **Reference counting**: Shared handles released on cleanup

## Business Logic Coverage (Verified 2025-11-16)

- **Shared registry restarts**: Shared transports watch `onclose` and respawn stdio processes with exponential backoff, logging each attempt so operators can trace MCP recoveries (`src/tools/mcp-provider.ts:136-430`).
- **Process tree cleanup**: Dedicated stdio servers track child PIDs and kill the entire process tree during shutdown/restart to prevent orphan processes (`src/tools/mcp-provider.ts:430-520`).
- **Queue binding**: Server-level `queue` config (and per-tool overrides) populate `toolQueue`, letting heavy MCP servers throttle concurrency independent of agent sessions (`src/tools/mcp-provider.ts:1000-1100`).
- **Health probes**: `healthProbe: 'ping'|'listTools'` determines whether initialization verifies connectivity via RPC or tool listing, stopping failed servers from polluting the mapping (`src/tools/mcp-provider.ts:520-590`).
- **Timeout layering**: `requestTimeoutMs` ensures MCP call timeouts surface before session-level tool timeouts, enabling consistent `(tool failed: timeout)` messaging (`src/tools/mcp-provider.ts:1135-1180`).

## Test Coverage

**Phase 1**:
- Transport type selection
- Tool namespacing
- Tool filtering
- Initialization with concurrency
- Cleanup procedures
- Error propagation

**Gaps**:
- Shared registry restart under load
- Multiple provider instances
- Transport reconnection scenarios
- Large tool counts performance

## Troubleshooting

### Server failed to initialize
- Check transport type and config
- Verify command/url accessible
- Check environment variables
- Review stderr output

### Tool not found
- Verify server initialized successfully
- Check tool not filtered by allow/deny
- Confirm namespace prefix

### Timeout during execution
- Check requestTimeoutMs setting
- Verify server responsiveness
- Check for probe failures
- Review restart logs

### Restart loop
- Check backoff timing
- Verify process termination
- Review probe health
- Check transport stability

### Memory/process leak
- Ensure cleanup() called
- Verify shared handles released
- Check process tracking maps
- Review PID termination logs
