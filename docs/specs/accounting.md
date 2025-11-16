# Accounting

## TL;DR
Token usage and cost tracking for LLM requests and tool executions. Entries include status, latency, tokens, cost, and trace context. Flushed at session end.

## Source Files
- `src/types.ts:634-680` - AccountingEntry definitions
- `src/ai-agent.ts:1765-1821` - LLM accounting entry creation
- `src/tools/tools.ts` - Tool accounting entry creation
- `src/ai-agent.ts:388-408` - flushAccounting()
- `src/session-tree.ts` - opTree accounting aggregation

## Data Structures

### AccountingEntry
**Location**: `src/types.ts:634`

```typescript
type AccountingEntry = LLMAccountingEntry | ToolAccountingEntry;
```

### BaseAccountingEntry
**Location**: `src/types.ts:636-647`

```typescript
interface BaseAccountingEntry {
  timestamp: number;
  status: 'ok' | 'failed';
  latency: number;
  agentId?: string;
  callPath?: string;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
  details?: Record<string, LogDetailValue>;
}
```

### LLMAccountingEntry
**Location**: `src/types.ts:649-670`

```typescript
interface LLMAccountingEntry extends BaseAccountingEntry {
  type: 'llm';
  provider: string;
  model: string;
  actualProvider?: string;
  actualModel?: string;
  costUsd?: number;
  upstreamInferenceCostUsd?: number;
  stopReason?: string;
  tokens: TokenUsage;
  error?: string;
}
```

### ToolAccountingEntry
**Location**: `src/types.ts:664-680`

```typescript
interface ToolAccountingEntry extends BaseAccountingEntry {
  type: 'tool';
  mcpServer: string;
  command: string;
  charactersIn: number;
  charactersOut: number;
  error?: string;
}
```

## LLM Accounting

### Entry Creation
**Location**: `src/ai-agent.ts:1765-1821`

```typescript
const accountingEntry: AccountingEntry = {
  type: 'llm',
  timestamp: Date.now(),
  status: turnResult.status.type === 'success' ? 'ok' : 'failed',
  latency: turnResult.latencyMs,
  provider,
  model,
  actualProvider: metadata?.actualProvider,
  actualModel: metadata?.actualModel,
  costUsd: metadata?.reportedCostUsd ?? computed.costUsd,
  upstreamInferenceCostUsd: metadata?.upstreamCostUsd,
  stopReason: turnResult.stopReason,
  tokens,
  error: turnResult.status.type !== 'success' ? statusMessage : undefined,
  agentId: this.sessionConfig.agentId,
  callPath: this.callPath,
  txnId: this.txnId,
  parentTxnId: this.parentTxnId,
  originTxnId: this.originTxnId
};
```

### Token Normalization
**Location**: `src/ai-agent.ts:1769-1774`

```typescript
const r = tokens.cacheReadInputTokens ?? tokens.cachedTokens ?? 0;
const w = tokens.cacheWriteInputTokens ?? 0;
const totalWithCache = tokens.inputTokens + tokens.outputTokens + r + w;
tokens.totalTokens = totalWithCache;
```

### Cost Computation
**Location**: `src/ai-agent.ts:1777-1794`

```typescript
const computeCost = (): { costUsd?: number } => {
  const pricing = this.sessionConfig.config.pricing;
  const modelTable = pricing[effectiveProvider][effectiveModel];
  const denom = modelTable.unit === 'per_1k' ? 1000 : 1_000_000;
  const cost = (
    pIn * tokens.inputTokens
    + pOut * tokens.outputTokens
    + pRead * cacheReadTokens
    + pWrite * cacheWriteTokens
  ) / denom;
  return { costUsd: cost };
};
```

## Tool Accounting

### Entry Creation
**Location**: `src/tools/tools.ts:430-1010`

```typescript
const accountingEntry: ToolAccountingEntry = {
  type: 'tool',
  timestamp: start,
  status: exec.ok ? 'ok' : 'failed',
  latency,
  mcpServer: kind === 'mcp' ? (exec.namespace ?? logProviderNamespace) : logProviderNamespace,
  command: name,
  charactersIn,
  charactersOut: exec.ok ? result.length : 0,
  error: exec.ok ? undefined : errorMessage,
  agentId,
  callPath,
  txnId,
  parentTxnId,
  originTxnId,
};
```

## Accounting Flow

### 1. Entry Recording
**Per LLM attempt**:
1. Create LLMAccountingEntry
2. Push to session accounting array
3. Append to opTree operation
4. Emit via onAccounting callback

**Per tool execution**:
1. Create ToolAccountingEntry
2. Append to opTree operation
3. Emit via onAccounting callback

### 2. OpTree Attachment
**Location**: `src/ai-agent.ts:1819`

```typescript
if (llmOpId !== undefined) {
  this.opTree.appendAccounting(llmOpId, accountingEntry);
}
```

### 3. Callback Emission
**Location**: `src/ai-agent.ts:1820`

```typescript
this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
```

### 4. Session Flush
**Location**: `src/ai-agent.ts:388-408`

```typescript
private async flushAccounting(entries: AccountingEntry[]): Promise<void> {
  const sink = this.sessionConfig.callbacks?.onAccountingFlush;
  const payload: AccountingFlushPayload = {
    sessionId: this.txnId,
    originId: this.originTxnId ?? this.txnId,
    timestamp: Date.now(),
    entries: entries.map((entry) => ({ ...entry })),
  };
  await sink(payload);
}
```

## Pricing Configuration

### Pricing Table
```typescript
interface PricingTable {
  [provider: string]: {
    [model: string]: {
      unit?: 'per_1k' | 'per_1m';
      currency?: 'USD';
      prompt?: number;
      completion?: number;
      cacheRead?: number;
      cacheWrite?: number;
    }
  }
}
```

### Cost Priority
1. Provider-reported cost (metadata.reportedCostUsd)
2. Computed cost from pricing table
3. undefined if not available

## Aggregation

### Per Turn
- Sum of LLM attempts
- All tool executions

### Per Session
- Total LLM cost
- Total tool count
- Aggregate token usage
- Cache statistics

### OpTree Totals
```typescript
interface SessionTotals {
  tokensIn: number;
  tokensOut: number;
  tokensCacheRead: number;
  tokensCacheWrite: number;
  toolsRun: number;
  costUsd: number;
  agentsRun: number;
}
```

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `pricing` | Cost computation table |
| `onAccounting` | Real-time entry callback |
| `onAccountingFlush` | Batch flush callback |
| `persistence.billingFile` | File persistence path |

## Telemetry

**Per Entry**:
- Timestamp
- Operation type (llm/tool)
- Status (ok/failed)
- Latency
- Provider/model (LLM)
- Tool name (tool)
- Token counts (LLM)
- Character counts (tool)
- Cost (when available)

## Logging

**Not directly logged**. Accounting data flows through:
1. Callback emission
2. OpTree attachment
3. Session result

## Invariants

1. **Every LLM attempt**: Creates accounting entry (even failures)
2. **Every tool execution**: Creates accounting entry
3. **Token normalization**: totalTokens includes cache
4. **Cost precedence**: Reported > computed
5. **Trace context**: Always attached to entries

## Test Coverage

**Phase 1**:
- Entry creation for LLM
- Entry creation for tools
- Cost computation
- Token aggregation
- Flush mechanism

**Gaps**:
- Pricing edge cases
- Large-scale aggregation
- Persistence validation

## Business Logic Coverage (Verified 2025-11-16)

- **Cache-aware totals**: LLM entries normalize `totalTokens` by adding cache read/write tokens when providers omit them, ensuring downstream dashboards match actual billing (`src/ai-agent.ts:1777-1797`).
- **Cost resolution order**: Router metadata (e.g., OpenRouter `actualProvider`/`actualModel`, upstream and downstream costs) always overrides pricing-table estimates; only when metadata lacks cost do we evaluate `config.pricing` entries (`src/ai-agent.ts:1798-1844`).
- **Ledger persistence**: `flushAccounting` merely invokes `callbacks.onAccountingFlush`; when `persistence.billingFile` is configured, `createPersistenceCallbacks` hooks that callback and appends each batch as JSONL lines on disk (`src/ai-agent.ts:388-408`, `src/persistence.ts:1-75`).
- **Tool character counts**: Tool entries store the serialized parameter/result `.length` (approximate character count) for in/out payloads, so even truncated responses show the number of characters originally returned (`src/tools/tools.ts:330-520`).
- **Trace propagation**: Entries capture `txnId`, `parentTxnId`, `originTxnId`, and `callPath` so multi-agent billing can be correlated through the session tree (`src/ai-agent.ts:1765-1868`, `src/types.ts:636-680`).

## Troubleshooting

### Missing cost information
- Check pricing table completeness
- Verify actual provider/model names
- Check metadata extraction

### Token count mismatch
- Check normalization logic
- Verify cache token reporting
- Compare with provider response

### Flush not called
- Check onAccountingFlush callback
- Verify session completion
- Check error paths

### Entries not appearing
- Check onAccounting callback
- Verify opTree attachment
- Check accounting array population

### Wrong trace context
- Check session configuration
- Verify txnId propagation
- Check parent/origin IDs
