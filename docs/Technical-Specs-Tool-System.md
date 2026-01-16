# Tool System

Tool providers, execution, and orchestration.

---

## TL;DR

Abstract ToolProvider interface with four implementations: MCP, REST, Internal, Agent. ToolsOrchestrator routes calls, manages queues, applies timeouts and size caps.

---

## Tool Provider Architecture

### ToolProvider (Abstract Base)

```typescript
abstract class ToolProvider {
  abstract readonly kind: ToolKind;  // 'mcp' | 'rest' | 'agent'
  abstract readonly namespace: string;
  abstract listTools(): MCPTool[];
  abstract hasTool(name: string): boolean;
  abstract execute(name, parameters, opts?): Promise<ToolExecuteResult>;
  async warmup(): Promise<void>;
  getInstructions(): string;
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

---

## ToolsOrchestrator

**State**:
- `providers` - Registered providers
- `mapping` - Tool → provider mapping
- `aliases` - Tool name aliases
- `canceled` - Cancellation flag
- `pendingQueueControllers` - Active queue controllers

**Key Methods**:
- `register(provider)` - Add provider
- `listTools()` - Get all available tools
- `warmup()` - Initialize providers
- `executeWithManagement()` - Execute with queueing/budgeting
- `cancel()` - Cancel all pending
- `cleanup()` - Release resources

**Execution Flow**:
1. Validate tool exists
2. Check tool response cache
3. Acquire queue slot
4. Begin opTree operation
5. Run timeout wrapper
6. Call `provider.execute()`
7. Apply response size cap
8. Record accounting
9. End opTree operation

---

## Concrete Providers

### MCPProvider

**Kind**: `mcp`

**Features**:
- Protocol support: stdio, websocket, http, sse
- Connection management with reconnection
- Tool discovery via `tools/list`
- Tool execution via `tools/call`
- Parameter validation via AJV
- Server instructions
- Concurrency queues

**Tool Naming**: `{namespace}__{toolname}`

**Configuration**:
```json
{
  "type": "stdio",
  "command": "npx",
  "args": ["-y", "@mcp/server"],
  "env": { "API_KEY": "${API_KEY}" },
  "toolsAllowed": ["search", "get"],
  "cache": 300000,
  "queue": "mcp-queue"
}
```

---

### RestProvider

**Kind**: `rest`
**Namespace**: `rest`

**Features**:
- REST API tool invocation
- OpenAPI schema import
- URL template expansion
- Query parameter handling
- Request body construction
- JSON streaming support

**Tool Naming**: `rest__{toolname}`

**Configuration**:
```json
{
  "description": "Get weather data",
  "method": "GET",
  "url": "https://api.weather.com/v1/${parameters.city}",
  "headers": { "Authorization": "Bearer ${API_KEY}" },
  "parametersSchema": {
    "type": "object",
    "properties": {
      "city": { "type": "string" }
    }
  }
}
```

---

### InternalToolProvider

**Kind**: (tool-specific)
**Namespace**: `agent`

**Tools**:

| Tool | Purpose |
|------|---------|
| `agent__final_report` | Deliver final answer |
| `agent__task_status` | Track task progress (optional) |
| `agent__batch` | Batch tool execution (optional) |

**final_report Parameters**:
- `status`: `'success' | 'failure' | 'partial'`
- `format`: Output format identifier
- `content_json`: For JSON format (validated)
- `content`: For text/markdown formats
- `metadata`: Optional metadata

**task_status Parameters**:
- `status`: `'starting' | 'in-progress' | 'completed'`
- `done`: What has been completed
- `pending`: What remains
- `now`: Current immediate step
- `ready_for_final_report`: Boolean
- `need_to_run_more_tools`: Boolean

---

### RouterToolProvider

**Kind**: `agent`
**Tool**: `router__handoff-to`

**Parameters**:
```typescript
{
  agent: string;     // Must be in router.destinations
  message?: string;  // Optional advisory text
}
```

Registered only when `router.destinations` is configured.

---

### AgentProvider

**Kind**: `agent`
**Namespace**: `subagent`

**Features**:
- Sub-agent invocation
- Recursive composition
- Trace context propagation
- OpTree hierarchy
- Accounting aggregation

**Tool Naming**: `agent__{agentname}`

**Execution**:
1. Resolve sub-agent definition
2. Create child session with inherited config
3. Propagate trace context
4. Execute child session
5. Capture child opTree and accounting
6. Return final_report content

---

## Tool Namespacing

**Format**: `{provider}__{toolname}`

**Examples**:
- `github__search_code` - MCP server 'github'
- `rest__weather_api` - REST tool
- `agent__researcher` - Sub-agent
- `agent__final_report` - Internal tool
- `router__handoff-to` - Router tool

**Sanitization**:
- Replace invalid characters with underscores
- Truncate to max length
- Normalize casing

---

## Queue Management

**Features**:
- Per-queue concurrency limits
- Priority queueing
- Abort signal propagation
- Timeout enforcement

**Queue Assignment**:
- Per-server queue: All tools share queue
- Per-tool queue: Individual tool queue
- Default: No queue (immediate)

**Configuration**:
```json
{
  "queues": {
    "github": { "concurrent": 3 }
  },
  "mcpServers": {
    "github": { "queue": "github" }
  }
}
```

---

## Tool Output Handling

When tool outputs exceed limits:

1. **Size cap** (`toolResponseMaxBytes`):
   - Output stored to disk
   - Handle message inserted
   - Warning logged with bytes/lines/tokens

2. **Token budget** (context guard):
   - Output dropped
   - Failure stub inserted
   - `toolBudgetExceeded` flag set

**Handle Format**: `session-<uuid>/<file-uuid>`
**Storage Root**: `/tmp/ai-agent-<run-hash>/`

---

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `toolTimeout` | Global timeout per tool |
| `toolResponseMaxBytes` | Triggers tool_output storage |
| `traceMCP` | Enable MCP tracing logs |
| `mcpInitConcurrency` | Parallel MCP init |
| `toolsAllowed` | Per-server allow list |
| `toolsDenied` | Per-server deny list |

---

## Telemetry

**Per Tool Execution**:
- Latency (ms)
- Characters in/out
- Success/failure status
- Provider namespace

---

## Logging

**Request Logs**:
- Severity: VRB
- Direction: request
- Type: tool
- Remote: `namespace:toolname`

**Response Logs**:
- Severity: VRB (success), WRN/ERR (failure)
- Direction: response
- Details: latency, output size, error

---

## Events

- `tool_started` - Execution began
- `tool_finished` - Execution completed

---

## Troubleshooting

### Tool not found
- Check tool name matches registered name
- Check server enabled flag
- Check toolsAllowed/toolsDenied lists

### Tool timeout
- Check toolTimeout setting
- Check requestTimeoutMs per server
- Check network latency

### Large response stored
- Check toolResponseMaxBytes
- Confirm tool_output handle in response
- Review warning log details

### MCP connection failed
- Check command/args for stdio
- Check url for websocket/http/sse
- Check env variables
- Check spawn permissions

---

## See Also

- [Configuration-MCP-Tools](Configuration-MCP-Tools) - MCP configuration
- [Configuration-REST-Tools](Configuration-REST-Tools) - REST configuration
- [docs/specs/tools-overview.md](../docs/specs/tools-overview.md) - Full spec

