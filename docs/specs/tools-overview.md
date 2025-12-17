# Tools Overview

## TL;DR
Abstract ToolProvider interface with four implementations: MCP, REST, Internal, Agent (subagents). ToolsOrchestrator routes calls, manages queues, applies timeouts and size caps.

## Source Files
- `src/tools/tools.ts` - ToolsOrchestrator (~46KB)
- `src/tools/types.ts` - Core tool interfaces
- `src/tools/mcp-provider.ts` - MCP protocol provider (~51KB)
- `src/tools/rest-provider.ts` - REST/OpenAPI provider
- `src/tools/internal-provider.ts` - Built-in tools (~44KB)
- `src/tools/agent-provider.ts` - Subagent invocation
- `src/tools/openapi-importer.ts` - OpenAPI schema import
- `src/tools/queue-manager.ts` - Concurrency queue management

## Tool Provider Architecture
**XML transport impact**: Transport is fixed to `xml-final`: provider tools remain native (tool_calls) while the final report travels via XML with the session nonce. Budgets/validation remain identical.

### ToolProvider (Abstract Base)
**Location**: `src/tools/types.ts:37-56`

```typescript
abstract class ToolProvider {
  abstract readonly kind: ToolKind;  // 'mcp' | 'rest' | 'agent'
  abstract readonly namespace: string;
  abstract listTools(): MCPTool[];
  abstract hasTool(name: string): boolean;
  abstract execute(name: string, parameters: Record<string, unknown>, opts?: ToolExecuteOptions): Promise<ToolExecuteResult>;
  async warmup(): Promise<void> { /* no-op */ }
  async cancelTool(_name: string, _opts?: ToolCancelOptions): Promise<void> { /* no-op */ }
  getInstructions(): string { return ''; }
  resolveLogProvider(_name: string): string { return this.namespace; }
  resolveToolIdentity(name: string): { namespace: string; tool: string }
  resolveQueueName(_name: string): string | undefined
}
```

### ToolExecuteOptions
```typescript
interface ToolExecuteOptions {
  timeoutMs?: number;
  bypassConcurrency?: boolean;
  disableGlobalTimeout?: boolean;
  trace?: boolean;
  onChildOpTree?: (tree: SessionNode) => void;
  parentOpPath?: string;
  parentContext?: ToolExecutionContext;
}
```

### ToolExecuteResult
```typescript
interface ToolExecuteResult {
  ok: boolean;
  result?: string;
  error?: string;
  latencyMs: number;
  kind: ToolKind;
  namespace: string;
  extras?: Record<string, unknown>;
}
```

## ToolsOrchestrator

**Location**: `src/tools/tools.ts:65+`

**State**:
- `providers: ToolProvider[]` - Registered providers
- `mapping: Map<string, { provider, kind, queueName }>` - Tool → provider mapping
- `aliases: Map<string, string>` - Tool name aliases
- `canceled: boolean` - Cancellation flag
- `pendingQueueControllers: Set<AbortController>` - Active queue controllers

**Key Methods**:
- `register(provider)` - Add provider to registry
- `listTools()` - Get all available tools (refreshes mapping)
- `hasTool(name)` - Check tool availability
- `warmup()` - Initialize providers and refresh tool map
- `getCombinedInstructions()` - Merge provider instructions
- `executeWithManagement(name, parameters, ctx, opts)` - Execute a single tool call with queueing, budgeting, logging, and accounting
- `cancel()` - Cancel all pending queue waits/tool executions
- `cleanup()` - Release resources

**Execution Flow (executeWithManagement)**:
1. Validate tool exists and resolve provider/kind/queue
2. Acquire queue slot (unless `bypassConcurrency`)
3. Begin opTree operation + progress event
4. Run centralized timeout wrapper
5. Call `provider.execute()`
6. Apply response size cap + context-guard reservation
7. Record accounting + telemetry
8. End opTree operation, release queue slot, return result

## Concrete Providers

### MCPProvider
**Kind**: `mcp`
**Namespace**: Per-server (e.g., `github`, `filesystem`)

**Features**:
- Protocol support: stdio, websocket, http, sse
- Connection management with reconnection
- Tool discovery via `tools/list`
- Tool execution via `tools/call`
- Server instructions via `server/get_prompt`
- Concurrency queues per server or tool
- Health probes (ping, listTools)
- Request timeout configuration

**Tool Naming**: `{namespace}__{toolname}` (e.g., `github__search_code`)

**Configuration** (`MCPServerConfig`):
```typescript
interface MCPServerConfig {
  type: 'stdio' | 'websocket' | 'http' | 'sse';
  command?: string;      // stdio
  args?: string[];       // stdio
  url?: string;          // websocket/http/sse
  headers?: Record<string, string>;
  env?: Record<string, string>;
  enabled?: boolean;
  toolSchemas?: Record<string, unknown>;
  toolsAllowed?: string[];
  toolsDenied?: string[];
  queue?: string;
  shared?: boolean;
  healthProbe?: 'ping' | 'listTools';
  requestTimeoutMs?: number;
}
```

### RestProvider
**Kind**: `rest`
**Namespace**: `rest`

**Features**:
- REST API tool invocation
- OpenAPI schema import
- URL template expansion
- Query parameter handling
- Request body construction
- Authentication (API key, bearer token)

**Tool Naming**: `rest__{toolname}` (e.g., `rest__weather_api`)

**Configuration** (`RestToolConfig`):
```typescript
interface RestToolConfig {
  url: string;
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH';
  headers?: Record<string, string>;
  auth?: { type: 'apiKey' | 'bearer'; key: string; in?: 'header' | 'query'; name?: string };
  bodyTemplate?: string;
  description?: string;
  inputSchema?: Record<string, unknown>;
}
```

### InternalToolProvider
**Kind**: (internally mapped as tool-specific)
**Namespace**: `agent`

**Tools**:
1. **agent__final_report** - Deliver final answer
2. **agent__task_status** - Track task progress and state (optional)
3. **agent__batch** - Batch tool execution (optional)

**final_report Parameters**:
- `status`: `'success' | 'failure' | 'partial'`
- `format`: const matching requested output format (e.g., `'json'`, `'markdown'`, `'slack-block-kit'`, `'tty'`, `'pipe'`, `'text'`, `'sub-agent'`)
- `content_json`: required when `format='json'` (validated against frontmatter schema via AJV)
- `content`: required when `format` is markdown/text-like
- `metadata`: optional map passed through to consumers
- `ts`: auto-populated timestamp (ms)

**task_status Parameters**:
```typescript
{
  status: 'starting' | 'in-progress' | 'completed';  // Task status
  done: string;    // What has been completed
  pending: string; // What remains to be done
  now: string;     // Current immediate step
}
```

**batch Parameters**:
```typescript
{
  calls: Array<{ tool: string; parameters: object }>;  // Tools to execute
}
```

### AgentProvider
**Kind**: `agent`
**Namespace**: `subagent`

**Features**:
- Sub-agent invocation
- Recursive composition
- Trace context propagation
- OpTree hierarchy
- Accounting aggregation

**Tool Naming**: `agent__{agentname}` (e.g., `agent__researcher`)

**Execution**:
1. Resolve sub-agent definition from SubAgentRegistry
2. Create child session with inherited config
3. Propagate trace context (originId, parentId, callPath)
4. Execute child session.run()
5. Capture child opTree and accounting
6. Return final_report content as result

## Tool Namespacing

**Format**: `{provider}__{toolname}`

**Examples**:
- `github__search_code` - MCP server 'github', tool 'search_code'
- `rest__weather_api` - REST tool 'weather_api'
- `agent__researcher` - Sub-agent 'researcher'
- `agent__final_report` - Internal tool

**Sanitization**:
- Replace invalid characters with underscores
- Truncate to max length (TOOL_NAME_MAX_LENGTH)
- Normalize casing

## Queue Management

**Location**: `src/tools/queue-manager.ts`

**Features**:
- Per-queue concurrency limits
- Priority queueing
- Abort signal propagation
- Timeout enforcement

**Queue Assignment**:
- Per-server queue: All tools from server share queue
- Per-tool queue: Individual tool has own queue
- Default: No queue (immediate execution)

**Configuration**:
```typescript
{
  queue: 'my-queue',        // Queue name
  maxConcurrency: 3,        // Max parallel executions
  defaultTimeoutMs: 30000   // Default timeout
}
```

## Tool Schema

**MCPTool Structure**:
```typescript
interface MCPTool {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;  // JSON Schema
  instructions?: string;
  queue?: string;
}
```

**Schema Validation**:
- JSON Schema draft-07 compliant
- Optional strict mode via AJV
- Format validation for string types
- Nested object support

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `toolTimeout` | Global timeout per tool execution |
| `toolResponseMaxBytes` | Max response size cap |
| `traceMCP` | Enable MCP tracing logs |
| `mcpInitConcurrency` | Parallel MCP server initialization |
| `tools` (frontmatter) | Enabled tool list filter |
| `toolsAllowed` (server) | Per-server allow list |
| `toolsDenied` (server) | Per-server deny list |

## Telemetry

**Per Tool Execution**:
- Span: Tool operation in opTree
- Latency (ms)
- Characters in/out
- Success/failure status
- Provider namespace

## Logging

**Request Logs**:
- Severity: VRB
- Direction: request
- Type: tool
- ToolKind: mcp/rest/agent/command
- Remote: `namespace:toolname`
- Details: parameters, queue name

**Response Logs**:
- Severity: VRB (success), WRN/ERR (failure)
- Direction: response
- Type: tool
- Remote: `namespace:toolname`
- Details: latency, output size, error message

**Trace Logs** (when traceMCP enabled):
- JSON-RPC request/response payloads
- Connection events
- Server lifecycle

## Events

- `tool_started` - Execution began
- `tool_finished` - Execution completed (ok/failed)

## Undocumented Behaviors

### MCPProvider
1. **Process tree killing**:
   - `killProcessTree()` for stdio servers
   - Ensures all child processes terminated
   - Location: `src/tools/mcp-provider.ts:1044-1061`

2. **Shared registry**:
   - Persistent server instances across sessions
   - `SharedRegistry` manages server lifecycle
   - Location: `src/tools/mcp-provider.ts:136-592`

3. **Automatic restart with backoff**:
   - onclose monitoring and automatic restart
   - `SHARED_RESTART_BACKOFF_MS = [0, 1000, 2000, 5000, 10000, 30000, 60000]`
   - Location: `src/tools/mcp-provider.ts:98, 233-255`

4. **Server name validation**:
   - Regex `[A-Za-z0-9_-]+` enforcement
   - Location: `src/tools/mcp-provider.ts:762-774`

5. **Tool name collision resolution**:
   - Numeric suffixes for duplicate names
   - Location: `src/tools/mcp-provider.ts:546-567`

### RestProvider
6. **Complex query parameter serialization**:
   - JSON arrays/objects in query strings
   - Location: `src/tools/rest-provider.ts:26-61, 124-149`

7. **JSON streaming support**:
   - Handles streaming JSON responses
   - Location: `src/tools/rest-provider.ts:241-288`

8. **Body templating**:
   - String templates for request bodies
   - Location: `src/tools/rest-provider.ts:324-339`

### ToolsOrchestrator
9. **Session tree builder integration**:
   - opTree tracking for all tool operations
   - Child session attachment for sub-agents
   - Location: `src/tools/tools.ts:217-219`

10. **OpenTelemetry tracing**:
    - `runWithSpan()` for tool spans
    - Location: `src/tools/tools.ts:217`

## Business Logic Coverage (Verified 2025-11-16)

- **Queue manager integration**: Every tool provider advertises `queue` metadata. `ToolsOrchestrator.executeWithManagement` acquires queue slots via `QueueManager`, honoring abort signals so canceled sessions release slots immediately (`src/tools/queue-manager.ts:10-160`, `src/tools/tools.ts:217-360`).
- **Tool filtering**: MCP servers and providers respect `toolsAllowed` / `toolsDenied` lists so administrators can expose only safe operations (`src/tools/mcp-provider.ts:120-220`). Internal helper `sanitizeToolName` ensures comparisons ignore `<|prefix|>` wrappers (`src/utils.ts`).
- **Process tree killing**: Stdio MCP provider tracks child PIDs and uses `killProcessTree` to terminate servers on shutdown/restart, preventing orphaned processes (`src/tools/mcp-provider.ts:460-520`).
- **Shared MCP registry**: Shared servers (`shared !== false`) keep a reference count so multiple sessions reuse the same transport and automatically restart if `onclose` fires (`src/tools/mcp-provider.ts:540-680`).
- **Tool budget + truncation**: Orchestrator enforces `toolResponseMaxBytes` before committing outputs, prepending `(tool failed: response exceeded max size)` and logging warnings when truncation occurs (`src/tools/tools.ts:330-420`).

### OpenAPI Importer
11. **Automatic REST tool generation**:
    - YAML/JSON OpenAPI spec parsing
    - `$ref` resolution
    - Tag and method filtering
    - URL template generation
    - Location: `src/tools/openapi-importer.ts`

## Test Coverage

**Phase 1**:
- Tool registration
- Tool discovery
- Execution routing
- Timeout enforcement
- Size cap application
- Error handling
- Queue management

**Gaps**:
- Complex MCP protocol edge cases
- OpenAPI schema import variations
- Batch tool execution scenarios
- Process tree killing edge cases
- Shared registry persistence

## Troubleshooting

### Tool not found
- Check tool name matches registered name (with namespace)
- Check server enabled flag
- Check toolsAllowed/toolsDenied lists

### Tool timeout
- Check toolTimeout setting
- Check requestTimeoutMs per server
- Check network latency

### Large response truncated
- Check toolResponseMaxBytes setting
- Response uses 50/50 split with marker: `[···TRUNCATED N bytes···]`

### MCP connection failed
- Check command/args for stdio
- Check url for websocket/http/sse
- Check env variables
- Check process spawn permissions

### Queue deadlock
- Check concurrency limits
- Check pending controller set
- Verify abort signals propagate

### Sub-agent fails
- Check ancestor list (no cycles)
- Check trace context propagation
- Verify child session config
