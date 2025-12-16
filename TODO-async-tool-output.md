# TODO: Async Tool Output & Extraction

## TL;DR
- Task now: feasibility/code-readiness study (no implementation) for async tool output + extraction.
- Goal state: slow/large tool calls go async; model fetches via `get_tool_output`/`wait_for_tool_output`; large payloads handled by extract/slice with provenance.
- Current behavior remains synchronous with 50/50 truncation; we must judge if architecture can adopt the spec without major rewrites.

## Analysis (current state, 2025-12-14)
- Tool pipeline is fully synchronous: `SessionToolExecutor` → `ToolsOrchestrator.executeWithManagement` blocks until provider returns or times out; on timeout the call fails and no background continuation exists (`tools.ts` with `withTimeout`, `session-tool-executor.ts` failure paths).
- Size control is byte-based: `applyToolResponseCap` (ai-agent.ts) truncates >`toolResponseMaxBytes` via JSON-aware or 50/50 byte split and still returns inline; context-guard drop-inserts `TOOL_NO_OUTPUT` when projected tokens overflow. No token-threshold or async deferral.
- Storage/rehydration: tool outputs are appended directly to conversation and opTree; there is no store keyed by tool ID, no retrieval tools, and no status tracking aside from logs.
- XML notices: `renderXmlNextTemplate` only lists turn info and two options (call tools / final report); no async sections, ready/running states, or retrieval guidance.
- Internal tools: only progress, final_report, and batch exist (internal-provider, internal-tools); no `get_tool_output` or `wait_for_tool_output` schemas.
- Subagents: executed as synchronous subagent provider sessions; queueing/concurrency is handled but they block parent turn; no async spawn surface.
- Token/accounting: ContextGuard operates on projected tokens of full tool outputs; extraction/slice-specific accounting hooks don’t exist. Tokenizer registry is used for estimation but only for guard/budget, not for size caps.
- Telemetry/logging: opTree/log entries record only inline results; no provenance metadata, hashes, or chunk offsets. No TTL/eviction logic because nothing is stored long-term.
- Tests/docs: existing docs describe truncation caps (AI-AGENT-GUIDE, SPECS) and current behavior; no tests cover async retrieval, notices, or extraction map-reduce.

## Decisions (pending from user)
1. Async storage backend & limits  
   - (A) In-memory per session with hard caps (max entries/bytes) and TTL. Pros: simple, fast; Cons: lost on restart.  
   - (B) File-based temp store with caps. Pros: survives crashes, can stream; Cons: I/O cost, cleanup risk.  
   - Recommendation: A with caps + eviction; promote to B only if persistence is required.
2. Backpressure & concurrency policy  
   - (A) Max concurrent async tools/subagents per session (e.g., 3–5) plus queueing.  
   - (B) No cap, rely on provider rate limits.  
   - Recommendation: A; aligns with existing queue-manager pattern and avoids memory/rate blowups.
3. Oversized retrieval behavior  
   - (A) Hard-error raw mode when over threshold (requires extract/slice), never auto-truncate.  
   - (B) Fallback to current truncation after extract/slice failure.  
   - Recommendation: A with explicit error; optional B as feature-flag for degraded mode.
4. Context-budget accounting for extracts  
   - (A) Count only returned snippets/slices + provenance;  
   - (B) Count estimated full-source tokens to be conservative.  
   - Recommendation: A (accurate) plus guardrail that rejects extracts that would overflow.
5. Provenance + cache versioning  
   - (A) Include extraction model ID + prompt hash in cache key/hash;  
   - (B) Hash only content+query.  
   - Recommendation: A to prevent stale/hazardous reuse after prompt/model changes.

## Plan (implementation, not started)
- Add AsyncToolStore with caps/TTL + hash/size metadata.
- Integrate async detection in tools.ts/session-tool-executor (time/token thresholds) → return async handles instead of inline payloads.
- Implement internal tools `get_tool_output` (raw/extract/slice) + `wait_for_tool_output`; wire to store and extraction pipeline.
- Build extraction map-reduce helper (chunking, parallel calls, reducer, provenance, caching).
- Extend XML-NEXT notice rendering for running/ready/examined states and option lists; thread async status from session state.
- Add subagent async spawn path and shared retrieval via store.
- Update accounting/context guard to price returned extracts/slices and enforce limits; add telemetry/logging for async lifecycle.
- Tests: unit + phase1 harness for async flows; docs updates in SPECS/GUIDE/INTERNAL-API/tools-overview/README.

## Implied Decisions
- Default async feature flag state currently assumed ON in design; confirm if rollout should be gated per-agent/CLI and default OFF for safety during initial ship.

## Testing Requirements
- Unit: AsyncToolStore lifecycle/eviction, thresholds, failure states; extraction chunking/map-reduce; new internal tools (raw/extract/slice error cases); time/size-triggered async paths.  
- Integration/harness: slow tool → async → wait; large tool → extract with provenance; subagent async; XML notice rendering with running/ready/examined.  
- Regression: ensure legacy truncation path still works when async is disabled.

## Documentation Updates Required
- SPECS, AI-AGENT-GUIDE, AI-AGENT-INTERNAL-API, docs/specs/tools-overview.md, README to cover async triggers, retrieval tools, provenance, limits, and notice format changes.

## Problem Statement

**Current behavior:**
- Tool responses exceeding `toolResponseMaxBytes` are truncated in the middle (50/50 split)
- For web content, the middle often contains the most valuable information
- Slow tools block the entire turn, even if model could work on other things
- Model has no control over what gets preserved vs truncated

**Desired behavior:**
- Model explicitly decides what to extract from large responses
- Slow tools don't block - model can continue working
- No blind truncation - extraction is intentional
- Subagents can run in parallel with parent agent's work

## Design

### Async Trigger Rules

| Condition | Trigger | Model sees |
|-----------|---------|------------|
| Response size > threshold | Auto-async | `ready (id: X, size: Y)` |
| Execution time > X secs | Auto-async | `running (id: X)` |
| Subagent with `async: true` | Explicit | `running (id: X)` |
| Normal tool (small, fast) | None | Inline result (current behavior) |

**Thresholds (configurable, token-based):**

Bytes correlate poorly with context cost (compression, markup density, unicode vary widely).
Use **token estimation** for more accurate thresholds.

| Config | Default | Per-agent | Description |
|--------|---------|-----------|-------------|
| `asyncTokenThreshold` | 10000 | Yes | Auto-async if response exceeds this token count |
| `asyncTimeoutSecs` | 5 | Yes | Auto-async if execution exceeds this time |
| `extractionModel` | `anthropic/claude-haiku-4` | Yes | Model used for extraction |

**Token estimation:**
- Use the session's configured tokenizer (same as context budget)
- If no tokenizer available, fall back to `chars/4` approximation
- Estimation happens after tool completes, before adding to context

**Why tokens over bytes:**
- 100KB JSON might be 30K tokens
- 100KB minified JS might be 50K tokens
- 100KB markdown might be 25K tokens
- Token count = actual context cost

### New Internal Tools

#### 1. `get_tool_output`

Retrieves the output of an async tool.

```typescript
{
  name: "get_tool_output",
  parameters: {
    id: string,           // Tool's async ID
    mode: "raw" | "extract" | "slice",

    // For mode="extract"
    query?: string,       // What to extract (required for extract mode)

    // For mode="slice"
    slice?: {
      start?: number,     // Start byte offset
      length?: number,    // Number of bytes to return
      anchor?: string,    // String or regex to find
      window?: number     // Bytes around anchor (default: 1000)
    }
  }
}
```

**Modes:**

| Mode | Purpose | When to use |
|------|---------|-------------|
| `raw` | Get full content | Small results (under threshold) |
| `extract` | LLM extracts specific info | Large results, need specific facts |
| `slice` | Get raw bytes around anchor | Logs, traces, HTML - know what you're looking for |

**Mode behavior:**

| Mode | Size | Result |
|------|------|--------|
| `raw` | Under threshold | Full content returned |
| `raw` | Over threshold | Error: "Response too large. Use extract or slice mode." |
| `extract` | Any | Cheap model extracts, returns structured provenance |
| `slice` | Any | Raw bytes at offset or around anchor |

**Slice mode details:**
- `start` + `length`: Return bytes from position (e.g., `start: 50000, length: 5000`)
- `anchor` + `window`: Find anchor string/regex, return window bytes around it
- If anchor not found: Error with "Anchor not found in content"
- Multiple matches: Returns first match (can call again with offset)

**Extraction with provenance:**

Extraction returns structured output so results can be verified:

```typescript
interface ExtractionResult {
  // The extracted snippets with source locations
  snippets: Array<{
    text: string;           // Extracted content (verbatim from source)
    start_byte: number;     // Start position in original content
    end_byte: number;       // End position in original content
    chunk_index: number;    // Which chunk this came from (for large content)
  }>;

  // Verification metadata
  source_hash: string;      // SHA-256 of stored content (for integrity)
  source_size_bytes: number;

  // Coverage information
  chunks_searched: number;  // How many chunks were processed
  chunks_total: number;     // Total chunks in content

  // Quality indicator
  confidence: number;       // 0.0-1.0, extraction model's confidence

  // If nothing found
  no_match_reason?: string; // "No content matching query" or similar
}
```

**Why provenance matters:**
- Parent model can quote verbatim with confidence
- Can request slice around a snippet for more context
- Can verify extraction didn't hallucinate (check byte offsets)
- Enables "show me more around this" follow-up queries

**Extraction implementation (Parallel Map-Reduce):**

Uses parallel processing for speed, with a final reduce step for coherence:

```
┌─────────────────────────────────────────────────────────────────┐
│ MAP PHASE (parallel)                                            │
├─────────────────────────────────────────────────────────────────┤
│  Chunk 1 ──→ LLM ──→ snippets[] with byte offsets              │
│  Chunk 2 ──→ LLM ──→ snippets[]                                │
│  Chunk 3 ──→ LLM ──→ snippets[]                                │
│  ...                                                            │
│  (all chunks processed in parallel)                            │
├─────────────────────────────────────────────────────────────────┤
│ REDUCE PHASE (single call)                                      │
├─────────────────────────────────────────────────────────────────┤
│  Input: All extractions + original query                        │
│  Output: Deduplicated, coherent result with merged provenance   │
└─────────────────────────────────────────────────────────────────┘
```

**Chunking:**
- Chunk size = 50% of extraction model's max context window
  - Example: Haiku with 200K context → 100K token chunks
  - Example: GPT-4o-mini with 128K context → 64K token chunks
- 5% overlap between chunks (handles info split across boundaries)
- Each chunk knows its `chunk_index` and `start_byte` offset
- Larger chunks = fewer LLM calls = faster extraction

**Map phase (per chunk, parallel):**
- Input: system prompt + query + chunk payload + chunk metadata
- Output per chunk:
  ```json
  {
    "snippets": [
      {
        "text": "verbatim extracted text",
        "start_offset": 1234,    // offset within chunk
        "end_offset": 1456
      }
    ],
    "chunk_index": 2,
    "chunk_start_byte": 8192
  }
  ```
- Each mapper calculates absolute byte offsets: `chunk_start_byte + start_offset`

**Reduce phase (single call):**
- Input: All mapper outputs + original query
- Tasks:
  - Deduplicate overlapping snippets (by byte range or text similarity)
  - Merge adjacent snippets from consecutive chunks
  - Produce coherent summary if requested
  - Preserve provenance (byte offsets, chunk sources)
- Output: Final `ExtractionResult` with clean snippets array

**Performance:**
- 5 chunks: ~2s parallel vs ~10s sequential
- Fixed context per mapper (no growth)
- n + 1 LLM calls total (n mappers + 1 reducer)

**Caching:**
- Cache results by `(content_hash, query_hash)` for repeated queries
- Cache individual chunk extractions for partial reuse

#### 2. `wait_for_tool_output`

Blocks until at least one async tool completes.

```typescript
{
  name: "wait_for_tool_output",
  parameters: {}  // No parameters
}
```

**Behavior:**
- Blocks until any running async tool completes
- Returns list of newly completed tools with their IDs and sizes
- If all async tools already complete: returns immediately with list
- If no async tools: returns immediately with empty list

**Return format:**
```
Completed tools:
- fetcher (id: abc123, size: 45KB)
- search (id: def456, size: 800KB)
```

### Async Status in XML-NEXT

The async tool status is integrated into the existing XML-NEXT system notice (ephemeral, excluded from caching).

**Updated XML-NEXT structure:**

```markdown
# System Notice

This is turn 3 of 10.
Your context window is 45% full.

## Async Tool Status

### Running (2)
- fetcher (id: abc123, running 7.2s)
- search (id: def456, running 3.1s)

### Ready (1)
- jina-reader (id: ghi789, ~45K tokens, exceeds threshold)

### Examined (1)
- serper (id: jkl012, ~12K tokens, 2 extractions performed)

---

You now need to decide your next move:
1. Call tools to advance your task
2. Retrieve/examine async tool outputs using get_tool_output(id, mode) or wait_for_tool_output()
3. Provide your final report/answer

Use get_tool_output(id, "raw"|"extract"|"slice") to retrieve.
Use wait_for_tool_output() to block until a running tool completes.
```

**Conditional sections:**

| Section | Appears when |
|---------|--------------|
| Running | Tools currently executing in background |
| Ready | Tools completed but not yet retrieved |
| Examined | Tools retrieved but still available for more extraction |
| (none) | No async tools - current behavior unchanged |

**Conditional options:**

| Async state | Options shown |
|-------------|---------------|
| No async tools | 1. Call tools, 2. Final report (current) |
| Has Ready/Examined | 1. Call tools, 2. Retrieve async outputs, 3. Final report |
| Only Running | 1. Call tools, 2. Wait for tools, 3. Final report |

**Why XML-NEXT integration:**
- Single location for all turn guidance
- Ephemeral (excluded from Anthropic caching)
- Model sees complete state at decision time
- No separate notice mechanism needed
- Consistent with existing architecture

### Turn Mechanics

**Async tools do NOT consume turns while running.**

```
Turn 1: Model calls tools A, B, C
        A: inline (returned immediately)
        B: goes async (slow)
        C: goes async (large)

        Model sees results, turn ends

Turn 2: Model processes A, calls get_tool_output("C", "extract", "...")
        Extraction runs
        Turn ends with extracted content

[B completes, system notice injected]

Turn 3: Model sees notice, calls get_tool_output("B", "raw")
        Turn ends with B's content
```

### Subagent Explicit Async

Subagents can be spawned with explicit async mode:

```typescript
// Current (blocking)
spawn_agent("research competitor pricing")

// New (async)
spawn_agent("research competitor pricing", { async: true })
→ Returns immediately with ID
→ Parent continues working
→ Parent calls get_tool_output(id, ...) when ready
```

This enables true parallel work between parent and child agents.

## Flow Diagrams

### Normal Tool (small, fast)
```
Model calls tool → Tool executes → Result inline → Model continues
(No change from current behavior)
```

### Large Tool Response
```
Model calls tool
    ↓
Tool executes, returns ~15K tokens
    ↓
System detects: tokens > threshold (10K default, configurable)
    ↓
Store result with ID, mark as "ready"
    ↓
Model sees: "Tool ready (id: abc123, ~15K tokens, exceeds threshold)"
    ↓
Model calls: get_tool_output("abc123", "extract", "get pricing info")
    ↓
Cheap model extracts, returns structured result with provenance
    ↓
Model receives: snippets[] with byte offsets + source hash
```

### Slow Tool
```
Model calls tool
    ↓
Tool starts executing...
    ↓
5 seconds pass, tool still running
    ↓
System: mark as async, unblock model
    ↓
Model sees: "Tool running (id: abc123)"
    ↓
Model continues other work...
    ↓
[Tool completes]
    ↓
[SYSTEM notice at next turn]:
  "Async tools completed: fetcher (id: abc123, ~10K tokens)"
    ↓
Model calls: get_tool_output("abc123", "raw")
    ↓
Model receives full content (under threshold, so raw OK)
```

### Parallel Subagents
```
Model spawns: agent_a(async: true), agent_b(async: true)
    ↓
Model sees: "agent_a running (id: a1)", "agent_b running (id: b1)"
    ↓
Model does other work (calls other tools, thinks)
    ↓
[agent_a completes]
[SYSTEM notice at next turn]:
  "Async tools completed: agent_a (id: a1, ~8K tokens)"
    ↓
Model calls: get_tool_output("a1", "raw")
    ↓
Model continues, still waiting for agent_b
    ↓
Model calls: wait_for_tool_output()
    ↓
[Blocks until agent_b completes]
    ↓
Model calls: get_tool_output("b1", "extract", "key findings only")
    ↓
Model receives: snippets[] with provenance
```

## Implementation Plan

### Phase 1: Async Tool Storage

1. Create `AsyncToolStore` class:
   - Store tool outputs by ID
   - Track status: `running | ready | retrieved`
   - Track size, content type
   - TTL for cleanup (prevent memory leaks)

2. Storage location options:
   - In-memory (simple, per-session)
   - File-based (survives restarts, but adds I/O)
   - Recommendation: in-memory with session lifecycle

### Phase 2: Auto-Async Detection

1. Modify tool execution in `tools/tools.ts`:
   - After tool completes, check response size
   - If > threshold: store in AsyncToolStore, return placeholder

2. Add execution timeout detection:
   - Track tool start time
   - If exceeds timeout AND not complete: mark async, unblock model
   - Tool continues in background

3. Return format for async tools:
   ```
   Tool "fetcher" → async (id: abc123, size: 500KB, status: ready)
   Tool "search" → async (id: def456, status: running)
   ```

### Phase 3: Implement `get_tool_output`

1. Add as internal tool (like `final_report`, `progress_report`)

2. Implementation:
   ```typescript
   async function getToolOutput(id: string, mode: 'raw' | 'extract', query?: string) {
     const stored = asyncToolStore.get(id);
     if (!stored) throw new Error(`Unknown tool ID: ${id}`);
     if (stored.status === 'running') throw new Error(`Tool not ready. Use wait_for_tool_output.`);

     if (mode === 'raw') {
       if (stored.sizeBytes > threshold) {
         throw new Error(`Response too large (${stored.sizeBytes} bytes). Use extract mode.`);
       }
       return stored.content;
     }

     if (mode === 'extract') {
       if (!query) throw new Error('Query required for extract mode');
       return await extractContent(stored.content, query);
     }
   }
   ```

3. Extraction implementation (Parallel Map-Reduce):
   ```typescript
   async function extractContent(content: string, query: string): Promise<ExtractionResult> {
     // 1. Get extraction model's context window
     const modelInfo = getModelInfo(config.extractionModel);
     const chunkTokens = Math.floor(modelInfo.maxContextTokens * 0.5);

     // 2. Chunk the content
     const chunks = chunkContent(content, {
       chunkTokens,        // 50% of model's context
       overlapPercent: 5
     });

     // 3. MAP: Extract from all chunks in parallel
     const mapResults = await Promise.all(
       chunks.map((chunk, index) =>
         extractFromChunk(chunk, query, index, config.extractionModel)
       )
     );

     // 4. REDUCE: Merge and deduplicate
     const reduced = await reduceExtractions(mapResults, query, config.extractionModel);

     return reduced;
   }
   ```

   - **Chunk size:** 50% of extraction model's context window
   - **Map phase:** Parallel LLM calls, one per chunk
   - **Reduce phase:** Single LLM call to deduplicate and merge
   - **Caching:** Store by `(content_hash, query_hash)`

### Phase 4: Implement `wait_for_tool_output`

1. Add as internal tool

2. Implementation:
   ```typescript
   async function waitForToolOutput() {
     const running = asyncToolStore.getRunning();
     if (running.length === 0) {
       return "No async tools running.";
     }

     // Wait for any to complete
     const completed = await asyncToolStore.waitForAny();

     return formatCompletedList(completed);
   }
   ```

3. Waiting mechanism:
   - Use Promise.race on all running tool promises
   - Return when first one completes
   - Include all that completed (may be multiple)

### Phase 5: XML-NEXT Integration

1. Extend `XmlNextPayload` interface:
   ```typescript
   interface XmlNextPayload {
     // ... existing fields ...
     asyncTools?: {
       running: Array<{ id: string; toolName: string; runningSecs: number }>;
       ready: Array<{ id: string; toolName: string; tokens: number; exceedsThreshold: boolean }>;
       examined: Array<{ id: string; toolName: string; tokens: number; extractionCount: number }>;
     };
   }
   ```

2. Update `renderXmlNextTemplate()` in `src/llm-messages.ts`:
   - Add "Async Tool Status" section when `asyncTools` has entries
   - Show Running/Ready/Examined subsections conditionally
   - Update options list to include async retrieval option

3. Update options logic:
   ```typescript
   if (hasAsyncReady || hasAsyncExamined) {
     lines.push('1. Call tools to advance your task');
     lines.push('2. Retrieve/examine async tool outputs using get_tool_output(id, mode)');
     lines.push('3. Provide your final report/answer');
   } else if (hasAsyncRunning) {
     lines.push('1. Call tools to advance your task');
     lines.push('2. Wait for running tools using wait_for_tool_output()');
     lines.push('3. Provide your final report/answer');
   } else {
     // Current behavior - 2 options
   }
   ```

4. Pass async state from session to XML transport at turn start

### Phase 6: Subagent Async Mode

1. Add `async` option to subagent spawning

2. When `async: true`:
   - Start subagent in background
   - Return immediately with ID
   - Subagent result stored in AsyncToolStore when complete

3. Parent agent uses same `get_tool_output` / `wait_for_tool_output` tools

### Phase 7: Configuration & Documentation

1. **Feature toggle (like streaming):**

   CLI flags:
   ```bash
   ai-agent --async agent.ai        # enable async tool handling
   ai-agent --no-async agent.ai     # disable, use current truncation behavior
   ```

   Frontmatter:
   ```yaml
   async: true   # or false
   ```

   Config file (`~/.ai-agent/config.yaml`):
   ```yaml
   async: false  # global default
   ```

   **Precedence:** CLI > frontmatter > config file > default (ON)

2. New configuration options (all configurable per-agent via frontmatter):
   ```yaml
   # Feature toggle
   async: true                       # default ON

   # Token-based threshold (more accurate than bytes)
   asyncTokenThreshold: 10000        # auto-async if exceeds ~10K tokens

   # Time-based threshold
   asyncTimeoutSecs: 5               # auto-async if slower than 5s

   # Extraction configuration
   extractionModel: "anthropic/claude-haiku-4"  # model for extraction
   # Chunk size: 50% of extraction model's context window (automatic)

   # Lifecycle
   asyncToolTTL: 3600                # cleanup after 1 hour
   ```

   **Per-agent override example (frontmatter):**
   ```yaml
   ---
   async: true
   asyncTokenThreshold: 50000   # this agent handles larger responses inline
   asyncTimeoutSecs: 10         # allow slower tools before async
   extractionModel: "openai/gpt-4o-mini"
   ---
   ```

2. Token estimation integration:
   - Reuse session's tokenizer (from context-guard)
   - Fallback: `Math.ceil(chars / 4)` approximation
   - Log actual vs estimated for tuning

3. Update documentation:
   - docs/SPECS.md - async tool behavior
   - docs/AI-AGENT-GUIDE.md - how models should use these tools
   - docs/AI-AGENT-INTERNAL-API.md - tool schemas

## Edge Cases & Error Handling

| Scenario | Behavior |
|----------|----------|
| `get_tool_output` on unknown ID | Error: "Unknown tool ID: X" |
| `get_tool_output` on running tool | Error: "Tool not ready. Use wait_for_tool_output." |
| `get_tool_output("id", "raw")` on oversized | Error: "Response too large (X bytes). Use extract mode." |
| `get_tool_output("id", "extract")` without query | Error: "Query required for extract mode." |
| `wait_for_tool_output` with nothing running | Returns immediately: "No async tools running." |
| Extraction finds nothing | Returns: "No content matching query found in tool output." |
| Tool fails while async | Status becomes "failed", get_tool_output returns error |
| Session ends with pending async | Cleanup, no action needed |
| Async tool TTL expires | Removed from store, get_tool_output returns "Tool output expired" |

## Retry & Fallback

1. **Extraction retries:** If extraction model fails, retry up to 2 times

2. **Final fallback:** If all else fails, fall back to current truncation behavior:
   - Log warning
   - Return truncated content with marker
   - Model can still work (degraded)

3. **Model doesn't call get_tool_output:**
   - System notices remind model of pending results
   - If model ignores and ends session, results are lost (acceptable)

## Testing Requirements

### Unit Tests

1. `AsyncToolStore`:
   - Store and retrieve by ID
   - Status transitions: running → ready → retrieved
   - TTL expiration
   - Multiple concurrent tools

2. `get_tool_output`:
   - Raw mode under threshold
   - Raw mode over threshold (error)
   - Extract mode with valid query
   - Extract mode without query (error)
   - Invalid ID (error)
   - Running tool (error)

3. `wait_for_tool_output`:
   - Single tool completes
   - Multiple tools, one completes
   - All already complete
   - None running

4. Extraction:
   - Small content, simple query
   - Large content, chunked extraction
   - No matching content
   - Extraction model failure + retry

### Integration Tests

1. End-to-end: tool returns large response → model extracts
2. End-to-end: slow tool → model continues → waits → retrieves
3. Parallel subagents: spawn two async → work → retrieve both
4. System notices: verify injection timing and format

### Phase 1 Harness Tests

1. Add scenarios for async tool behavior
2. Verify extraction quality
3. Verify fallback to truncation

## Documentation Updates

After implementation, update:

1. `docs/SPECS.md` - async tool execution model
2. `docs/AI-AGENT-GUIDE.md` - model instructions for async tools
3. `docs/AI-AGENT-INTERNAL-API.md` - `get_tool_output`, `wait_for_tool_output` schemas
4. `docs/specs/tools-overview.md` - async tool behavior
5. `README.md` - mention async capability if user-facing

## Decisions Made

1. **Threshold values:** Token-based, default 10K tokens for size, 5s for time (both per-agent configurable)

2. **Extraction model:** Configurable per-agent via `extractionModel` in frontmatter

3. **Backpressure:** Not implementing now - will address if it becomes a problem

4. **Status reporting:** Integrated into XML-NEXT (ephemeral, excluded from caching)

5. **Extraction output:** Structured with provenance (snippets + byte offsets + hash)

6. **Feature toggle:** Controlled via `--async` / `--no-async` flags (like streaming), default ON

7. **Subagent async:** Tool parameter `spawn_agent(..., {async: true})`

8. **Slice anchor:** String matching only for v1 (no regex)

9. **Multi-match slice:** Add `match_index` parameter (default 0)

10. **Extraction cache TTL:** Same as content TTL

11. **Config location:** Global defaults, frontmatter overrides, CLI wins

12. **UTF-8 encoding:** Byte offsets are UTF-8 positions, hash is SHA-256 of raw bytes

13. **Async tool execution:** Use Promise.race to return async ID while tool continues in background. Response is structured instruction (not placeholder):
    - **Timeout case:** `Tool Output ID: {id}\nTool: {name}\n\nWhen the System Notice shows this tool is ready, use get_tool_output("{id}", mode) to retrieve its output.`
    - **Size threshold case:** `Tool Output ID: {id}\nTool: {name}\nSize: ~{tokens} tokens\n\nUse get_tool_output("{id}", mode) to retrieve this output:\n- "extract": Extract specific information you need\n- "slice": Get bytes around an anchor string`
    - No status field (immutable response)
    - No "raw" option when over threshold (would error anyway)

14. **Turn boundary async status:** Extend `XmlNextPayload` interface with async tool status. Query `AsyncToolStore` at turn start and pass status to XML-NEXT builder. Keeps all turn state in one place, consistent with existing pattern (turn number, context %, tools).

15. **Extraction model invocation:** Use AI SDK `generateText()` directly. No LLMClient needed. Simple one-shot calls:
    - No final_report, no XML wrapper, no schema validation
    - Each chunk extraction: call LLM with query + chunk, return raw text
    - Reducer/aggregator makes sense of all chunk outputs
    - Wrap calls to record usage for accounting

16. **Turn counting:** Every turn counts normally. No exceptions for async tools, retrieval tools, or any other special cases. The spec claim "async tools do NOT consume turns while running" is invalid for this implementation.

17. **Time vs size trigger:** No priority needed. Promise.race design handles it naturally - time trigger fires before size is known, so they can't conflict. Sequential: (1) race tool against timer, (2) if timer wins → async, (3) if tool wins → check size.

18. **Background tool lifecycle:**
    - Use existing `toolTimeout` for max duration. No new mechanism.
    - Session crash = lost state (acceptable for in-memory design).
    - **Stop signal (graceful):** Background tools complete normally. Session enters final-turn mode, model wraps up. Async results available if tools finish in time.
    - **Abort signal (cancel):** Kill background tools immediately. AbortController propagates to pending promises. Total interrupt, no cleanup.

19. **Extraction model context window:** Use existing per-provider/per-model configuration system. Same mechanism that provides context window to `ContextGuard`. No separate lookup table needed.

20. **Tool output retention:** All async tool outputs maintained for lifetime of session. No TTL, no expiration. Multiple extractions allowed on same output, even in parallel.

21. **Tool output status:** No "Examined" state. Just a counter per tool output showing how many successful `get_tool_output` calls made. Displayed in XML-NEXT for model awareness.

22. **Event loop and LLM request rules:**
    - **ALL tool calls must have responses** before next LLM request (provider validation). Response = output OR async ID OR failure.
    - **Async IDs are NOT new information** - they satisfy validation but don't trigger model run.
    - **Only actual tool outputs are NEW information.**
    - **Model runs when:** (all tool calls have responses) AND (at least one actual output exists that model hasn't seen).
    - **If all tools go async with no outputs yet:** Model waits until at least one completes.
    - This is a significant change to the current turn loop architecture.

23. **Slice mode uses characters, not bytes.** Simple string slicing: `content.slice(start, start + length)`. No UTF-8 boundary issues. Character positions are natural for model to reason about. Byte offset provenance is theoretical overhead with no practical benefit.

24. **Context budget for extractions:** `get_tool_output` is a regular tool. All limits and accounting apply. Context guard applies to all outputs, no exceptions. The only special case: `get_tool_output` itself can never turn async (no recursion into async).

25. **Async storage:** In-memory, session lifetime, no limits on total store size. However, individual tool outputs capped at 10MB - anything larger is truncated to 10MB before storing/processing.

## Open Questions

(All resolved - see Decisions Made section)

## Codebase Contradictions (2025-12-14)

The spec assumes architectural patterns that fundamentally don't exist in the current codebase.

### Contradiction #1: XML-NEXT Structure Mismatch

**Spec assumes:**
```markdown
## Async Tool Status
### Running (2)
- fetcher (id: abc123, running 7.2s)
### Ready (1)
- jina-reader (id: ghi789, ~45K tokens)
```

**Actual (`llm-messages.ts:398-424`):**
```typescript
lines.push('# System Notice');
lines.push(`This is turn ${turn}...`);
lines.push(`Your context window is ${contextPercentUsed}% full.`);
// Only 2 options: call tools OR final report. NO async sections.
```

**Impact:** Entire async notification system must be built from scratch.

### Contradiction #2: Synchronous Tool Execution Contract

**Spec assumes (Lines 415-421):**
```
5 seconds pass, tool still running → System: mark as async, unblock model
Model continues other work...
```

**Actual (Vercel AI SDK constraint):**
```typescript
// session-turn-runner.ts:2871
const toolExecutor = async (name, params): Promise<string> => {
  // MUST return completed result before SDK continues
  return await this.executeWithManagement(...);
};
```

The AI SDK expects `toolExecutor` to resolve when tool completes. **You cannot "unblock model" mid-tool** - the model doesn't continue until callback returns.

**Workaround required:** Race tool execution against timer; if timer wins, store Promise in AsyncToolStore and return placeholder. Tool continues in background but model already received response. This changes the semantic contract.

### Contradiction #3: Subagent Async Mode Doesn't Exist

**Spec assumes (Lines 369-377):**
```typescript
spawn_agent("research competitor pricing", { async: true })
→ Returns immediately with ID
```

**Actual (`subagent-registry.ts:293-307`):**
```typescript
result = await loaded.run(...); // Parent WAITS until child completes
```

No `async` parameter exists. No mechanism to return early with ID. Subagents always block.

### Contradiction #4: Token vs Byte Threshold Mismatch

**Spec assumes:** `asyncTokenThreshold: 10000` (token-based)

**Actual (`session-tool-executor.ts:231`):**
```typescript
const toolOutput = this.applyToolResponseCap(uncappedToolOutput, {...});
// Uses truncateToBytes() with toolResponseMaxBytes
```

Token infrastructure exists (`truncateToTokens()`, `ContextGuard.estimateTokens()`) but tool capping uses bytes.

### Contradiction #5: Internal Tool Registration is Hardcoded

**Spec assumes:** `get_tool_output` and `wait_for_tool_output` as "internal tools"

**Actual (`internal-provider.ts`):** Only these exist:
- `agent__progress_report`
- `agent__final_report`
- `agent__batch`

No infrastructure for dynamic internal tool registration.

### Contradiction #6: AsyncToolStore Doesn't Exist

**Spec assumes:**
```typescript
AsyncToolStore class:
- Store tool outputs by ID
- Track status: running | ready | retrieved
- TTL for cleanup
```

**Actual:** No such class. No session-level storage for pending tool results. No ID-based retrieval.

---

## Architectural Feasibility (2025-12-14)

### Must Build From Scratch

| Component | Complexity | Lines Est. |
|-----------|------------|------------|
| `AsyncToolStore` class | Medium | 200-300 |
| Async detection wrapper (time/token race) | High | 150-200 |
| `get_tool_output` internal tool | High | 300-400 |
| `wait_for_tool_output` internal tool | Medium | 100-150 |
| XML-NEXT async sections | Medium | 100-150 |
| Extraction map-reduce pipeline | Very High | 400-600 |
| Token-based threshold integration | Medium | 100-150 |
| Subagent async mode | High | 200-300 |

**Total new code estimate:** 1,550-2,250 lines

### Must Significantly Modify

| Component | Change Type |
|-----------|-------------|
| `SessionToolExecutor` | Add async detection, storage hooks, timer racing |
| `XmlToolTransport.buildMessages()` | Add async status rendering |
| `session-turn-runner.ts` main loop | Handle async completions between turns |
| `ContextGuard` | Handle extracted content budget |
| `SubAgentRegistry.execute()` | Support non-blocking mode |
| `internal-provider.ts` | Register new tools dynamically |
| Config schema (`options-schema.ts`) | Add `async`, `asyncTokenThreshold`, etc. |

### Architectural Blockers

#### Blocker #1: AI SDK Synchronous Contract
The Vercel AI SDK's `toolExecutor` callback expects `Promise<string>` that resolves when complete.

**Workaround:**
```typescript
const toolExecutor = async (name, params) => {
  const toolPromise = this.executeWithManagement(name, params);

  // Race against timeout
  const result = await Promise.race([
    toolPromise,
    sleep(asyncTimeoutSecs * 1000).then(() => ASYNC_PLACEHOLDER)
  ]);

  if (result === ASYNC_PLACEHOLDER) {
    const id = generateId();
    asyncToolStore.addRunning(id, toolPromise);
    toolPromise.then(r => asyncToolStore.markReady(id, r));
    return `Tool running (id: ${id})`;
  }
  // Check size threshold for completed results...
};
```

**Risk:** Changes semantic contract. SDK might not expect placeholder responses.

#### Blocker #2: Turn Boundary State Management
Current loop (`session-turn-runner.ts`):
```
FOR turn = 1 TO maxTurns:
  buildMessages → executeTurn → processResult
  // No hook for "between turns check async completions"
```

Need to inject async completion checking at turn boundaries.

#### Blocker #3: Extraction Model Invocation
Extraction pipeline requires invoking a **different model** than session model. Current architecture has one `llmClient` per session.

Need:
- Separate extraction client (Haiku, GPT-4o-mini)
- API key routing for different provider
- Rate limit handling
- Accounting integration

---

## Gray Areas & Undefined Behaviors (2025-12-14)

### Turn Counting Rules
**Spec (Line 345):** "Async tools do NOT consume turns while running."

**Undefined:**
- Does `get_tool_output` call consume a turn?
- Does `wait_for_tool_output` consume a turn?
- Do extraction LLM calls count toward turn limits?

### Time vs Size Trigger Priority
Tool runs 4.9s, returns 15K tokens at 5.1s.
- Is this async due to time (>5s)?
- Is this async due to size (>10K tokens)?
- What's the priority when both could apply?

### Tool Continues in Background - How?
**Spec (Line 417):** "System: mark as async, unblock model"

**Undefined:**
- What Promise resolution does tool executor return?
- Maximum background execution time?
- Resource limits for background tools?
- What if session/process crashes?

### Extraction Model Context Window Discovery
**Spec (Lines 217-219):** "Chunk size = 50% of extraction model's max context window"

**Problem:** Codebase has no mechanism to query arbitrary model context windows. `ContextGuard.getTargets()` returns session targets only.

### Multiple Extractions on Same Output
**Spec implies (Line 306):** "2 extractions performed"

**Undefined:**
- Can model call extract multiple times with different queries?
- Is content preserved after `raw` mode retrieval?
- What's the status transition state machine?

### "Examined" Status Definition
**When does a tool become "Examined"?**
- After any `get_tool_output` call?
- Only after `extract` mode (not `raw` or `slice`)?
- Can it transition back to "Ready"?

### wait_for_tool_output Semantics Contradiction
**Spec (Line 272):** "Blocks until any running async tool completes"
**Spec (Line 569):** "Include all that completed (may be multiple)"

If it blocks until **any** (singular) completes, how can it return **multiple**?

### UTF-8 Byte Boundary in Slice Mode
Byte offsets can split multi-byte UTF-8 characters. Current `truncation.ts` has `findSafeUtf8Boundary()` but spec doesn't address this for slice mode.

---

## Visibility Findings (2025-12-14)

### Clarifications Needed
- How extracted content counts against context budget and window accounting (pre/post-reduction, provenance metadata included?).
- What happens when extraction or slice returns > context budget; is there secondary truncation or a forced summarization pass?
- Exact lifecycle for async tool storage: eviction order, backpressure when many large outputs accumulate, and limits per session.
- Failure semantics: if an async tool errors after being marked running, what status and payload do models receive? Can outputs be partially saved?
- Concurrency rules for subagents: how many can be async simultaneously; does parent throttling exist to prevent tool fan-out exhaustion?
- Token estimation source of truth: which tokenizer implementations are supported and how mismatches across providers are handled.
- Security/PII handling for stored outputs (especially if future file-based storage is enabled) and whether hashes are persisted beyond session TTL.

### Risk Hotspots
- **Unbounded memory**: large outputs cached in-memory with soft TTL can exhaust process memory before cleanup; no max-bytes or max-entries cap is specified.
- **Fallback truncation**: reverting to blind truncation after extraction failures undermines the spec goal and may reintroduce silent data loss.
- **Timeout promotion**: auto-async after 5s may leave tools running indefinitely without watchdogs; long-running external calls could pile up.
- **Extraction cost/latency**: map-reduce extraction multiplies LLM calls; default model and chunk sizing may be expensive without guardrails.
- **Slice mode correctness**: byte offsets are UTF-8, but anchors operate on strings; multi-byte chars can misalign offsets and anchors.
- **Status drift**: model guidance assumes XML-NEXT notice is injected reliably; missing notice would strand async outputs with no other surfacing.
- **Cache staleness**: caching by content+query hash without versioning of extraction prompt/model could return incompatible results after prompt changes.
- **Tool failure paths**: spec mentions `failed` status but lacks return shape; callers may not know whether to retry or fall back.
- **Backpressure**: no limits on concurrent async tools or subagents; risk of overwhelming downstream providers or hitting rate limits.

### Spec Smells
- Extraction provenance uses byte offsets but slice mode uses anchors/match_index; mixed primitives may complicate verification UX.
- Default `async` ON with fallback to truncation may create surprising behavior differences between runs where extraction model availability changes.
- `wait_for_tool_output` blocks until any completes but return format is free-form text, not structured JSON; brittle for tool callers/parsers.
- `get_tool_output` errors for oversized raw responses but does not provide size metadata in the error; forces extra call or logging peek.
- No explicit guidance on logging/telemetry for async transitions; hard to debug in production when outputs expire or fail.

## Summary

This design:
- **Feature toggle** - `--async` / `--no-async` flags (like streaming), default ON
- **Eliminates blind truncation** - model decides what to extract via structured modes
- **Enables parallel execution** - slow tools don't block, subagents run concurrently
- **Token-based thresholds** - 10K tokens default (per-agent configurable), not arbitrary byte limits
- **Three retrieval modes:**
  - `raw` - full content for small results
  - `extract` - LLM extraction with provenance (byte offsets, hashes)
  - `slice` - raw bytes around anchor for logs/traces (string match, `match_index` param)
- **Provenance for trust** - extraction returns snippets with source locations, verifiable
- **XML-NEXT integration** - async status in ephemeral system notice, 3 options when async tools exist
- **Two tools only** - `get_tool_output`, `wait_for_tool_output`
- **Graceful fallback** - truncation if extraction fails after retries
- **Works for tools and subagents** - same interface for both
- **Config precedence** - CLI > frontmatter > config file > default
