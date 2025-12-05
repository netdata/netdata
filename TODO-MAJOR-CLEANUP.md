# TODO: Major Codebase Cleanup - Simplify Tools & Transport

> NOTE (2025-12-05): Transport cleanup to a single xml-final path is now complete. The transport-related items below are kept for historical context; batch redesign items remain future work.

## TL;DR

Major simplification of ai-agent architecture:
1. **Final-report**: Always XML-based (never as a tool)
2. **Tools**: Either native OR batch - not both simultaneously
3. **Batch**: When enabled, all tools live inside batch; progress-report is mandatory batch option
4. **Streaming**: Only streaming mode - non-streaming removed
5. **XML-NEXT**: Minimal role - only datetime, turns/retries, context window %
6. **Transport**: Remove `--tool-transport` option entirely

---

## Part 1: Transport & Tool Architecture Simplification

### 1.1 Remove `--tool-transport` Option

**Current state**: Multiple transport modes (`native`, `xml`, `xml-final`) with complex switching logic.

**Target state**: Fixed architecture:
- Final-report: Always via XML tag (`<ai-agent-{nonce}-FINAL>`)
- Tools: Native or Batch (per model config)

**Files to modify**:
- `src/options-registry.ts` - Remove `toolingTransport` option definition
- `src/options-schema.ts` - Remove from schema
- `src/types.ts` - Remove from interfaces (`ToolingTransport` type, `toolingTransport` fields)
- `src/cli.ts` - Remove `--tool-transport` flag handling
- `src/agent-loader.ts` - Remove transport resolution
- `src/ai-agent.ts` - Remove transport switching logic
- `src/session-turn-runner.ts` - Remove transport conditionals
- `src/xml-transport.ts` - Keep for XML parsing, remove transport mode logic
- `src/llm-messages.ts` - Remove transport-specific messages

### 1.2 Remove XML Tools Mode

**Current state**: `xml` transport allows all tools via XML tags.

**Target state**: XML only for final-report. Tools are native or batch.

**Removal scope**:
- Remove ability to call MCP/REST tools via XML tags
- Keep XML parsing only for final-report extraction
- Keep `<tools>` tag detection in output (for leaked tool fallback extraction)

**Files to modify**:
- `src/xml-tools.ts` - Simplify to final-report only
- `src/xml-transport.ts` - Remove non-final tool parsing
- `src/session-turn-runner.ts` - Remove XML tool call handling

### 1.3 Final-Report: Always XML

**Current state**: Final-report can be delivered as native tool OR XML tag depending on transport.

**Target state**: Final-report ALWAYS delivered as XML tag, never as a tool.

**Changes**:
- Remove `agent__final_report` from tool definitions entirely
- Final-report instructions move to system-prompt (not XML-NEXT)
- XML final-report parsing remains in `xml-transport.ts`
- Schema/format validation remains

**Files to modify**:
- `src/tools/internal-provider.ts` - Remove final-report tool definition and execution
- `src/session-turn-runner.ts` - Remove native final-report tool handling
- System prompt generation - Add final-report instructions with schema

---

## Part 2: Batch Tool Redesign

### 2.1 Batch as Per-Model Option

**Current state**: Batch is session-wide boolean.

**Target state**: Hierarchical configuration with defaults.

```typescript
// Config structure
{
  defaults: {
    batch: true  // Global default
  },
  providers: {
    anthropic: {
      models: {
        "claude-3-opus": { batch: false },  // Per-model override
        "*": { batch: true }  // Provider default
      }
    }
  }
}
```

**Resolution order** (highest to lowest):
1. Per-model override
2. Provider default
3. Global default
4. Built-in default (true)

**Files to modify**:
- `src/config.ts` - Add batch to model overrides schema
- `src/config-resolver.ts` - Add batch resolution
- `src/types.ts` - Add batch to `ProviderModelOverrides`
- `src/options-resolver.ts` - Add batch to effective options
- `src/ai-agent.ts` - Wire batch resolution

### 2.2 Tools Only Inside Batch (When Batch Enabled)

**Current state**: When batch enabled, tools available both top-level AND inside batch.

**Target state**: When batch enabled:
- Top-level tools: ONLY `agent__batch`
- Inside batch: All MCP/REST tools (NOT `agent__batch`, NOT `agent__final_report`)
- `agent__final_report`: NEVER exists as tool (always XML)

**Rationale**: Forces model to use batch for parallelism; prevents confusion about where to call tools.

**Files to modify**:
- `src/tools/internal-provider.ts` - Restructure tool exposure
- `src/session-turn-runner.ts` - Update tool selection logic

### 2.3 Progress-Report as Mandatory Batch Option

**Current state**: `agent__progress_report` is an optional tool callable inside batch.

**Target state**:
- When batch ENABLED: `progress_report` is a mandatory top-level field of batch (not a tool inside calls[])
- When batch DISABLED: `agent__progress_report` remains a normal standalone tool

**New batch schema (when batch enabled)**:
```json
{
  "type": "object",
  "required": ["progress_report", "calls"],
  "properties": {
    "progress_report": {
      "type": "string",
      "minLength": 1,
      "description": "Brief progress message (max 15 words)"
    },
    "calls": {
      "type": "array",
      "minItems": 1,
      "items": { ... }
    }
  }
}
```

**Key changes**:
- `progress_report` is NOT counted as a tool (it's metadata about the batch)
- `calls[]` must have minItems: 1 (no empty batches)
- `agent__progress_report` removed from batch calls schema

**Files to modify**:
- `src/tools/internal-provider.ts` - Restructure batch schema and execution
- Instructions in system prompt - Update batch usage rules

### 2.4 Empty Batches Not Allowed

**Current state**: Schema allows empty `calls[]` array.

**Target state**: Schema enforces `minItems: 1` on `calls[]`.

**Implementation**: Already partially shown in 2.3. The batch tool schema sets `minItems: 1`.

### 2.5 Duplicate Tool Calls Fail

**Current state**: Identical tool calls (same tool + same parameters) execute multiple times.

**Target state**: Deduplicate within single batch:
- First occurrence: Executes normally
- Subsequent identical calls: Fail with error "duplicate tool call not run"

**Duplicate definition**: Same `tool` name AND identical `parameters` (deep equality).

**Slight variations ARE allowed**: If parameters differ at all, both execute.

**Files to modify**:
- `src/tools/internal-provider.ts` - Add deduplication logic in batch execution

### 2.6 Batch Schema States Max Tool Count

**Current state**: Batch doesn't advertise the tool limit in schema.

**Target state**: Batch schema includes `maxItems` on `calls[]` based on `maxToolCallsPerTurn` config.

```json
{
  "calls": {
    "type": "array",
    "minItems": 1,
    "maxItems": 10,  // From maxToolCallsPerTurn
    "description": "Tool calls to execute (max 10, progress_report does not count)"
  }
}
```

**Files to modify**:
- `src/tools/internal-provider.ts` - Add maxItems to calls array schema

---

## Part 3: XML-NEXT Simplification

### 3.1 Move Final-Report Instructions to System Prompt

**Current state**: Final-report schema and instructions in XML-NEXT.

**Target state**: Final-report instructions in system prompt (static, not per-turn).

**Rationale**: Final-report format is constant throughout session; no need to repeat in XML-NEXT.

**Files to modify**:
- `src/xml-tools.ts` - Remove final-report from XML-NEXT rendering
- `src/session-turn-runner.ts` or system prompt builder - Add final-report instructions

### 3.2 XML-NEXT Minimal Content

**Current state**: XML-NEXT contains tools schemas, final-report instructions, datetime, turns, context.

**Target state**: XML-NEXT contains ONLY:
1. Current date and time
2. Current turn number and max turns
3. Retry count (if any)
4. Context window percentage used

**Example minimal XML-NEXT**:
```xml
<ai-agent-{nonce}-NEXT>
datetime: 2025-12-01T14:30:00Z
turn: 3/10
context: 45%
</ai-agent-{nonce}-NEXT>
```

**Files to modify**:
- `src/xml-tools.ts` - Simplify `renderXmlNext()` function

---

## Part 4: Remove Non-Streaming Mode

### 4.1 Remove Non-Streaming Code Path

**Current state**: Two code paths - `executeStreamingTurn()` and `executeNonStreamingTurn()`.

**Target state**: Only streaming. Remove all non-streaming code.

**Files to modify**:

| File | Changes |
|------|---------|
| `src/llm-providers/base.ts` | Remove `executeNonStreamingTurn()` (~200 lines), remove `generateText` import |
| `src/llm-providers/anthropic.ts` | Remove conditional, always call streaming |
| `src/llm-providers/openai.ts` | Remove conditional, always call streaming |
| `src/llm-providers/google.ts` | Remove conditional, always call streaming |
| `src/llm-providers/ollama.ts` | Remove conditional, always call streaming |
| `src/llm-providers/openrouter.ts` | Remove conditional, always call streaming |
| `src/llm-providers/test-llm.ts` | Remove conditional, always call streaming |
| `src/session-turn-runner.ts` | Remove stream resolution logic |
| `src/options-registry.ts` | Remove `stream` option |
| `src/options-schema.ts` | Remove `stream` from schema |
| `src/types.ts` | Remove `stream` from interfaces |

### 4.2 Remove `shouldAutoEnableReasoningStream()`

**Current state**: Hook method to auto-enable streaming for reasoning models.

**Target state**: Remove - streaming is always on.

**Files to modify**:
- `src/llm-providers/base.ts` - Remove method
- `src/llm-providers/anthropic.ts` - Remove override

---

## Part 5: Stop Reason Handling

### 5.1 Detect Truncation (stop=length)

**Problem**: When `stop=length` occurs, response is truncated but not detected as error.

**Solution**:
1. Check `stopReason` for `length` (OpenAI) or `max_tokens` (Anthropic)
2. Log as ERR: "Response truncated (stop=length) - tool calls or final report may have been cut off"
3. Update retry message to indicate truncation as root cause

**Files to modify**:
- `src/session-turn-runner.ts` - Add truncation detection
- `src/llm-messages.ts` - Add truncation-specific error message

---

## Implementation Order

### Phase 1: Remove Non-Streaming (Isolated change)
1. Remove non-streaming code path
2. Remove `stream` option from all layers
3. Verify build/lint/tests

### Phase 2: Remove Tool Transport Option
1. Remove `--tool-transport` CLI flag
2. Remove transport mode switching
3. Simplify to fixed architecture

### Phase 3: Final-Report Always XML
1. Remove `agent__final_report` tool
2. Move final-report instructions to system prompt
3. Keep XML parsing for final-report

### Phase 4: Batch Redesign
1. Add batch as per-model option
2. Restructure tool exposure (tools only in batch when enabled)
3. Redesign batch schema (mandatory progress_report, minItems, maxItems)
4. Add duplicate detection
5. Update instructions

### Phase 5: XML-NEXT Simplification
1. Strip down XML-NEXT to minimal content
2. Verify streaming output still works

### Phase 6: Stop Reason Handling
1. Add truncation detection
2. Update error messages

---

## Files Impact Summary

### Major Changes
- `src/tools/internal-provider.ts` - Batch redesign, remove final-report tool
- `src/session-turn-runner.ts` - Remove transport conditionals, streaming-only
- `src/llm-providers/base.ts` - Remove non-streaming
- `src/xml-tools.ts` - Simplify XML-NEXT
- `src/ai-agent.ts` - Remove transport switching

### Moderate Changes
- `src/cli.ts` - Remove flags
- `src/options-registry.ts` - Remove options
- `src/types.ts` - Simplify types
- `src/config.ts` - Add batch to model overrides
- All provider files - Remove streaming conditionals

### Minor Changes
- `src/llm-messages.ts` - Update messages
- `src/xml-transport.ts` - Simplify parsing
- `src/options-schema.ts` - Remove fields

---

## Testing Requirements

- [ ] `npm run build` - Zero errors
- [ ] `npm run lint` - Zero warnings
- [ ] All Phase 1 tests pass
- [ ] Manual test with real LLM (ollama on nova with gpt-oss:20b)

### New Test Scenarios Needed
- Batch with mandatory progress_report
- Duplicate tool call rejection
- Empty batch rejection (schema validation)
- Final-report via XML only
- Truncation detection (stop=length)

---

## Documentation Updates Required

| Document | Updates |
|----------|---------|
| README.md | Remove `--tool-transport`, `--stream` flags; update batch docs |
| docs/SPECS.md | Update transport, batch, final-report sections |
| docs/AI-AGENT-GUIDE.md | Update tool calling instructions |
| docs/DESIGN.md | Simplify architecture description |
| docs/specs/tools-*.md | Update all tools specs |

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking existing agents | High | Thorough testing, clear migration path |
| Model confusion with new batch | Medium | Clear instructions in system prompt |
| Test failures | Medium | Update tests incrementally per phase |

---

## Decisions Made

### D1: Batch default value
**Decision**: All models/providers default to `batch: true`. Consistent behavior across the board.

### D2: Progress report when batch disabled
**Decision**: Optional when no batch (current behavior preserved). Failing turns for missing progress when batch is disabled would be too strict.

### D3: XML-NEXT content
**Decision**: Keep visible to model as simple text. The purpose is for the model to know datetime, turns/retries, and context window usage so it can adjust behavior accordingly.

### D4: Final report success rule (confirmed 2025-12-04)
**Decision**: Any received final report is treated as success; its content/status field is not used to mark failure.

### D5: XML transport cleanup (confirmed 2025-12-04)
**Decision**: Proceed with planned removal/cleanup of XML tool transport; missing XML-mode tool instructions are acceptable until removal.

### D6: Progress updates default (confirmed 2025-12-04)
**Decision**: Progress updates should be requested/enabled by default (wantsProgressUpdates=true).

---

## Code Review Findings (2025-12-04)

### Verified Current State

| Item | Location | Status |
|------|----------|--------|
| `--tool-transport` option | `options-registry.ts:241-248` | ✓ Exists, needs removal |
| `toolingTransport` in types | `types.ts:604` (AIAgentSessionConfig) | ✓ Exists, needs removal |
| `ToolTransportEnum` in config | `config.ts:34` | ✓ Exists, needs removal |
| Non-streaming code path | `base.ts:1634` (`executeNonStreamingTurn`) | ✓ Exists (~300 lines) |
| Provider streaming conditionals | All 6 providers | ✓ Each has `if stream/else` pattern |
| `shouldAutoEnableReasoningStream` | `base.ts:97`, `anthropic.ts:151` | ✓ Exists, needs removal |
| `stream` option | `options-registry.ts:260-269` | ✓ Exists, needs removal |
| `agent__final_report` tool | `internal-provider.ts:126`, `295-445` | ✓ Exists, needs removal |
| `agent__batch` tool | `internal-provider.ts:127-160` | ✓ Exists |
| Batch `calls[]` minItems | `internal-provider.ts:154` | ✓ Has `minItems: 1` |
| Batch `calls[]` maxItems | `internal-provider.ts:154` | ✗ MISSING - not set |
| Duplicate tool call detection | `internal-provider.ts:907-945` | ✗ MISSING - not implemented |
| Progress as mandatory batch field | `internal-provider.ts:147-158` | ✗ Not implemented (it's just a tool) |
| Truncation stop reasons | `xml-transport.ts:50-53` | ✓ Defined but only used in XML mode |

### Key Code Locations for Each Phase

**Phase 1 (Non-Streaming):**
- `src/llm-providers/base.ts:1634-1800` - `executeNonStreamingTurn()` method
- `src/llm-providers/*.ts` - 6 providers with streaming conditionals
- `src/options-registry.ts:260-269` - `stream` option
- `src/session-turn-runner.ts` - stream resolution logic

**Phase 2 (Tool Transport):**
- `src/options-registry.ts:241-248` - `toolingTransport` option
- `src/config.ts:34` - `ToolTransportEnum`
- `src/types.ts:426, 604` - type definitions
- `src/xml-transport.ts` - full file (but keep for xml-final parsing)
- `src/xml-tools.ts` - keep for final-report XML rendering

**Phase 3 (Final-Report XML):**
- `src/tools/internal-provider.ts:295-445` - `buildFinalReportTool()`
- `src/tools/internal-provider.ts:457-783` - final_report execution
- `src/llm-messages.ts` - final report instructions

**Phase 4 (Batch Redesign):**
- `src/tools/internal-provider.ts:127-160` - batch tool definition
- `src/tools/internal-provider.ts:784-948` - batch execution
- `src/config.ts` - add batch to model config schema

**Phase 5 (XML-NEXT):**
- `src/xml-tools.ts:121-246` - `renderXmlNext()` function

**Phase 6 (Stop Reason):**
- `src/xml-transport.ts:50-53` - already has stop reason sets
- `src/session-turn-runner.ts` - need to add truncation detection for native mode

### Gaps/Issues Found

1. **Batch schema missing maxItems**: Current batch schema has `minItems: 1` but no `maxItems` to communicate the tool limit
2. **No duplicate detection**: Batch execution doesn't check for duplicate tool+parameters combinations
3. **Progress not mandatory**: Progress report is an optional tool inside batch, not a mandatory top-level field
4. **Truncation detection incomplete**: `TRUNCATION_STOP_REASONS` defined in xml-transport.ts but not used in native tool mode

---

## Notes

- This is a breaking change for agents using `--tool-transport xml`
- Batch schema change (mandatory progress_report) requires model prompt updates
- Non-streaming removal is low risk since streaming is already default
- Implement in phases to catch regressions early
