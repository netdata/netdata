# agent__batch Tool

## TL;DR
Meta-tool for executing multiple tool calls in single request. Builds dynamic schemas from available tools. Returns structured results array.

## Source Files
- `src/tools/internal-provider.ts:104-137` - Tool definition
- `src/tools/internal-provider.ts:661-817` - Execution logic
- `src/tools/internal-provider.ts:821-871` - Schema building
- `src/tools/internal-provider.ts:177-199` - Instructions generation

## Tool Definition

**Name**: `agent__batch`
**Short Name**: `batch`

### Input Schema (Dynamic)
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["calls"],
  "properties": {
    "calls": {
      "type": "array",
      "minItems": 1,
      "items": { "anyOf": [...] }  // Generated from available tools
    }
  }
}
```

### Call Item Schema
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["id", "tool", "parameters"],
  "properties": {
    "id": { "oneOf": [{ "type": "string", "minLength": 1 }, { "type": "number" }] },
    "tool": { "type": "string", "const": "tool_name" },
    "parameters": { ... }  // Cloned from tool's inputSchema
  }
}
```

## Enablement Conditions

**Location**: `src/tools/internal-provider.ts:104`

```typescript
if (this.opts.enableBatch) {
  tools.push(this.buildBatchTool());
}
```

Controlled by `InternalToolProviderOptions.enableBatch`.

## Schema Building

**Location**: `src/tools/internal-provider.ts:821-871`

### Process
1. List all tools from orchestrator
2. For each tool:
   - Skip `agent__final_report` and `agent__batch`
   - Clone tool's inputSchema
   - Wrap in call item schema with id, tool, parameters
   - Extract required parameters for summaries
3. Add task_status if enabled and not already present
4. Cache schemas and summaries

### Schema Cloning
**Location**: `src/tools/internal-provider.ts:873-894`

```typescript
cloneJsonSchema(schema: unknown): Record<string, unknown> {
  // Deep clone via JSON parse/stringify
  // Default type to 'object'
  // Default additionalProperties to true
  // Default description to 'Parameters for selected tool'
}
```

### Fallback Schema
When no tools available:
```json
{
  "type": "object",
  "additionalProperties": true,
  "required": ["id", "tool", "parameters"],
  "properties": {
    "id": { "oneOf": [{ "type": "string", "minLength": 1 }, { "type": "number" }] },
    "tool": { "type": "string", "minLength": 1 },
    "parameters": { "type": "object", "additionalProperties": true }
  }
}
```

## Execution Flow

### 1. Input Parsing
**Location**: `src/tools/internal-provider.ts:663-727`

Handles both formats:
- Array of call objects
- JSON string (with recovery for truncated input)

String parsing algorithm:
1. Trim input
2. Track bracket nesting and string state
3. Find last valid closing bracket
4. Extract and parse JSON substring
5. Handle parsing errors gracefully

### 2. Validation

**Empty batch check**:
```typescript
if (calls.length === 0) {
  throw new Error('empty_batch: ...');
}
```

**Call normalization**:
```typescript
normalizedCalls = calls.map((c) => ({
  id: string or stringified number,
  tool: string,
  parameters: parseJsonRecord(c.parameters) ?? {}
}));
```

**Invalid entry check**:
```typescript
if (entry.id.trim().length === 0 || entry.tool.trim().length === 0) {
  throw new Error('invalid_batch_input: each call requires non-empty id and tool');
}
```

### 3. Execution
**Location**: `src/tools/internal-provider.ts:776-814`

```typescript
const results = await Promise.all(normalizedCalls.map(async ({ id, tool, parameters }, index) => {
  // Disallow final_report and nested batch
  if (tool === 'agent__final_report' || tool === 'agent__batch') {
    return { ok: false, error: { code: 'INTERNAL_NOT_ALLOWED', message: '...' } };
  }

  // Handle task_status directly
  if (tool === 'agent__task_status') {
    const status = parameters.status || '';
    const done = parameters.done || '';
    const pending = parameters.pending || '';
    const now = parameters.now || '';

    const statusMessage = [status, done, pending, now].filter(Boolean).join(' | ');
    this.opts.updateStatus(statusMessage);
    const taskStatusPayload = {
      status,
      taskStatusCompleted: status === 'completed',
      taskStatusData: {
        status,
        done,
        pending,
        now,
      },
    };
    return { ok: true, output: JSON.stringify(taskStatusPayload) };
  }

  // Delegate to orchestrator
  if (!orchestrator.hasTool(tool)) {
    return { ok: false, error: { code: 'UNKNOWN_TOOL', message: '...' } };
  }

  const subturnForCall = baseSubturn + index + 1;
  const managed = await orchestrator.executeWithManagement(tool, parameters, context, options);

  return {
    id, tool,
    ok: managed.ok,
    elapsedMs: managed.latency,
    output: managed.result,
    dropped: managed.dropped,
    tokens: managed.tokens,
    reason: managed.reason,
    error: managed.dropped ? { code: managed.reason, message: managed.result } : undefined
  };
}));
```

### 4. Response
```json
{
  "results": [
    {
      "id": "1",
      "tool": "tool_name",
      "ok": true,
      "elapsedMs": 150,
      "output": "result string",
      "dropped": false,
      "tokens": 50
    },
    {
      "id": "2",
      "tool": "another_tool",
      "ok": false,
      "elapsedMs": 0,
      "error": { "code": "UNKNOWN_TOOL", "message": "Unknown tool: another_tool" }
    }
  ]
}
```

## Business Logic Coverage (Verified 2025-11-16)

- **Truncated JSON recovery**: When the LLM emits partial JSON arrays (common with long batches), the parser walks the string, tracks bracket depth, and slices off the trailing garbage so valid calls still execute (`src/tools/internal-provider.ts:663-727`).
- **Internal tool guardrails**: `agent__final_report` and nested `agent__batch` calls are rejected with `INTERNAL_NOT_ALLOWED`, while `agent__task_status` is handled inline without queue acquisition to avoid deadlocks (`src/tools/internal-provider.ts:776-814`).
- **Per-call opTree context**: Each call runs in its own opTree child with `subturn` offsets so cost accounting and telemetry remain accurate even when dozens of calls execute inside a single turn (`src/tools/internal-provider.ts:735-780`).
- **Schema caching**: Generated batch schemas are cached per orchestrator instance, so repeated invocations reflect newly-registered tools without rebuilding schemas on every call (`src/tools/internal-provider.ts:821-912`).

## Result Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Call identifier from request |
| `tool` | string | Tool name executed |
| `ok` | boolean | Success status |
| `elapsedMs` | number | Execution time |
| `output` | string? | Tool result (on success) |
| `error` | object? | Error details (on failure) |
| `dropped` | boolean? | Whether result was dropped |
| `tokens` | number? | Token count for result |
| `reason` | string? | Drop reason if dropped |

## Error Codes

| Code | Cause |
|------|-------|
| `INTERNAL_NOT_ALLOWED` | Attempting final_report or nested batch |
| `UNKNOWN_TOOL` | Tool not found in orchestrator |
| `EXECUTION_ERROR` | Tool threw exception |

## Subturn Management

**Location**: `src/tools/internal-provider.ts:775-788`

```typescript
const baseSubturn = parentContext?.subturn ?? 0;
const subturnForCall = baseSubturn + index + 1;
```

Each call in batch gets sequential subturn:
- Call 0: baseSubturn + 1
- Call 1: baseSubturn + 2
- Call n: baseSubturn + n + 1

## Turn Context

**Location**: `src/tools/internal-provider.ts:665-672`

```typescript
const batchTurn = (() => {
  const t = this.opts.getCurrentTurn();
  return Number.isFinite(t) && t > 0 ? Math.trunc(t) : 1;
})();
```

Uses current turn from session, defaults to 1.

## Warmup

**Location**: `src/tools/internal-provider.ts:896-901`

```typescript
warmupWithOrchestrator(): void {
  if (!this.opts.enableBatch) return;
  this.cachedBatchSchemas = undefined;
  this.ensureBatchSchemas();
  this.instructions = this.buildInstructions();
}
```

Called after orchestrator setup to:
1. Clear cached schemas
2. Rebuild schemas from current tools
3. Update instructions

## Instructions

**Location**: `src/tools/internal-provider.ts:177-199`

Provides example usage:
```json
{
  "calls": [
    { "id": 1, "tool": "agent__task_status", "parameters": { "status": "in-progress", "done": "Collected data", "pending": "Analyze results" } },
    { "id": 2, "tool": "tool1", "parameters": { ... } },
    { "id": 3, "tool": "tool2", "parameters": { ... } }
  ]
}
```

Includes warning: "Do not combine task_status with final_report in same request"

## Restrictions

1. **No nested batch**: Cannot call agent__batch from within batch
2. **No final_report in batch**: Must be standalone call
3. **Task status with other tools**: Can include task_status alongside regular tools
4. **Requires enableBatch**: Must be explicitly enabled

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `enableBatch` | Tool availability |
| `disableProgressTool` | Whether task_status included in examples |
| `toolTimeoutMs` | Timeout for each call |

## Telemetry

**Per batch execution**:
- Total latency
- Individual call results
- Error counts
- Drop statistics

## Logging

**Error cases logged**:
- Empty batch
- Invalid parameters
- Parse failures
- Unknown tools

**Location**: Uses `this.opts.logError(message)`

## Events

- Each inner tool execution emits standard tool events
- Subturn assignments tracked in opTree
- Accounting entries created per call

## Invariants

1. **Non-empty batch**: At least one call required
2. **Valid IDs**: Each call must have non-empty id
3. **Valid tools**: Each call must have non-empty tool name
4. **Parallel execution**: All calls run concurrently
5. **Result ordering**: Results match call ordering by index
6. **Schema consistency**: Schemas match available tools

## Test Coverage

**Phase 1**:
- Basic batch execution
- Error handling (unknown tool, invalid input)
- Schema generation
- Parameter validation
- Progress report in batch
- Result structure

**Gaps**:
- Large batch performance
- Concurrent timeout handling
- Schema versioning across sessions
- JSON string parsing edge cases

## Troubleshooting

### Schema not including all tools
- Check warmupWithOrchestrator called after tools registered
- Verify orchestrator.listTools() returns expected tools
- Check tool names are valid

### Batch parsing failure
- Check JSON syntax
- Verify array format
- Check for truncated input
- Review error logs for preview

### Tool not found
- Verify tool registered with orchestrator
- Check exact tool name match
- Confirm namespace resolution

### Results missing output
- Check tool execution succeeded
- Verify result not dropped due to budget
- Check for execution errors

### Subturn conflicts
- Verify parentContext passed correctly
- Check baseSubturn calculation
- Ensure sequential assignment
