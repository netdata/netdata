# Agent Tool Provider

## TL;DR
Tool provider wrapping sub-agent execution as tool calls. Bridges SubAgentRegistry to ToolsOrchestrator.

## Source Files
- `src/tools/agent-provider.ts` - Full implementation (39 lines)
- `src/subagent-registry.ts` - SubAgentRegistry interface
- `src/ai-agent.ts:700-740` - Sub-agent execution

## Data Structures

### ExecFn Type
**Location**: `src/tools/agent-provider.ts:8-14`

```typescript
type ExecFn = (
  name: string,
  parameters: Record<string, unknown>,
  opts?: {
    onChildOpTree?: (tree: SessionNode) => void;
    parentOpPath?: string;
    parentContext?: ToolExecutionContext;
  }
) => Promise<{
  result: string;
  childAccounting?: readonly unknown[];
  childConversation?: unknown;
  childOpTree?: unknown;
}>;
```

### Provider Class
**Location**: `src/tools/agent-provider.ts:16-38`

```typescript
class AgentProvider extends ToolProvider {
  readonly kind = 'agent';
  constructor(
    namespace: string,
    agents: SubAgentRegistry,
    execFn: ExecFn
  );
}
```

## Operations

### listTools()
**Location**: `src/tools/agent-provider.ts:20`

```typescript
listTools(): MCPTool[] {
  return this.agents.getTools();
}
```

Delegates to SubAgentRegistry to get agent tool definitions.

### hasTool()
**Location**: `src/tools/agent-provider.ts:21`

```typescript
hasTool(name: string): boolean {
  return this.agents.hasTool(name);
}
```

Checks if agent name registered.

### resolveToolIdentity()
**Location**: `src/tools/agent-provider.ts:23-26`

```typescript
resolveToolIdentity(name: string): { namespace: string; tool: string } {
  const tool = name.startsWith('agent__') ? name.slice('agent__'.length) : name;
  return { namespace: this.namespace, tool };
}
```

Strips `agent__` prefix if present.

### execute()
**Location**: `src/tools/agent-provider.ts:28-37`

```typescript
async execute(name, parameters, opts?) {
  const start = Date.now();
  const out = await this.execFn(name, parameters, {
    onChildOpTree: opts?.onChildOpTree,
    parentOpPath: opts?.parentOpPath,
    parentContext: opts?.parentContext,
  });
  const latency = Date.now() - start;
  return {
    ok: true,
    result: out.result,
    latencyMs: latency,
    kind: this.kind,
    namespace: this.namespace,
    extras: {
      childAccounting: out.childAccounting,
      childConversation: out.childConversation,
      childOpTree: out.childOpTree,
    }
  };
}
```

## Execution Flow

### 1. Tool Registration
SubAgentRegistry provides tool definitions:
- Tool name (agent identifier)
- Description
- Input schema (parameters expected by agent)

### 2. Execution Invocation
When orchestrator calls agent tool:
1. AgentProvider.execute() called
2. Delegates to execFn (parent session method)
3. Parent session spawns sub-agent
4. Sub-agent runs complete session
5. Result returned with extras

### 3. Result Structure
**Success response**:
```typescript
{
  ok: true,
  result: "Sub-agent final report content",
  latencyMs: 5000,
  kind: 'agent',
  namespace: 'agent',
  extras: {
    childAccounting: [...],      // Sub-agent's accounting entries
    childConversation: [...],    // Sub-agent's messages
    childOpTree: { ... }         // Sub-agent's operation tree
  }
}
```

## Integration Points

### SubAgentRegistry
Provides:
- Tool definitions for sub-agents
- Configuration for each agent
- Input schema validation

### Parent Session
Provides:
- execFn implementation
- Context propagation (opTree, trace IDs)
- Accounting aggregation

### ToolsOrchestrator
Sees agent as:
- Regular tool provider
- Subject to same execution patterns
- Result management applies

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `subAgents` | Available agent tools |
| Parent trace context | Propagated to child |
| Parent opTree | Child attached via onChildOpTree |

## Telemetry

**Per execution**:
- Total latency (includes full sub-agent session)
- Child accounting (LLM and tool costs)
- Child operation tree

## Logging

No direct logging in provider. Child session logs independently with inherited trace context.

## Events

**Captured via extras**:
- Child accounting entries
- Child conversation history
- Child opTree structure

## Invariants

1. **Synchronous wrapper**: Execution waits for complete sub-agent session
2. **Context propagation**: Parent context passed to child
3. **OpTree attachment**: Child tree linked via callback
4. **Accounting aggregation**: Child costs included in parent totals
5. **Result serialization**: Final report becomes tool result
6. **Reason metadata**: `reason` remains required for tool calls but is stripped before JSON prompt construction; it is only used for user-facing titles and never reaches the sub-agent prompt.

## Business Logic Coverage (Verified 2025-11-16)

- **Recursion safeguards**: SubAgentRegistry maintains ancestry stacks and recursion depth limits, so calling agents cannot form cycles (A→B→A) and depth defaults keep nested executions manageable (`src/subagent-registry.ts:120-220`).
- **Trace propagation**: execFn passes `{ originId, parentId, callPath, turnPath }` so child logs, accounting, and final reports retain full ancestry, enabling Slack/REST headends to render nested timelines (`src/ai-agent.ts:700-820`).
- **Extras fan-out**: Child accounting/conversations/opTree snapshots are attached to the tool result so headends can inspect child activity after the fact (`src/tools/agent-provider.ts:28-58`).

## Test Coverage

**Phase 2**:
- Tool listing delegation
- Tool existence checking
- Identity resolution
- Basic execution flow

**Gaps**:
- Error handling during sub-agent execution
- Timeout propagation
- Large child opTree handling
- Concurrent sub-agent execution

## Troubleshooting

### Agent not found
- Check SubAgentRegistry contains definition
- Verify agent name spelling
- Confirm registration before warmup

### Execution timeout
- Check sub-agent maxTurns
- Review child session logs
- Verify context budget

### Missing child data
- Check onChildOpTree callback registered
- Verify extras extraction
- Review parent context passing

### Result too large
- Check child final report size
- Verify result not truncated
- Review accounting entry count
