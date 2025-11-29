# TODO: Fallback Tool Extraction from Leaked XML-like Content

## TL;DR

- Models still emit tool calls inside `<tool_call>`, `<tools>`, etc., so native `toolCalls` stays empty and we hit `TURN_FAILED_NO_TOOLS_NO_REPORT_CONTENT_PRESENT` in `src/session-turn-runner.ts:1086-1110`.
- No fallback currently parses these leaked calls; `sanitizeTurnMessages` only handles native calls and AI-Agent XML (`<ai-agent-...>`). We need a parser + integration to convert leaked calls into real `ToolCall` objects.
- Complexity: medium (new parser + integration + tests + docs). Estimate ~1–1.5 engineering days.

## ⚠️ ARCHITECTURE REQUIREMENT: ISOLATED MODULE

**This feature MUST be implemented as a completely isolated, self-contained module.**

- **Single file**: `src/tool-call-fallback.ts` - all extraction logic lives here
- **Single hook point**: One call site in `session-turn-runner.ts` where the module is invoked
- **No blending**: Do NOT scatter this logic across existing code
- **Clean interface**: The module exports one function, takes content in, returns extracted tools out
- **Zero coupling**: The module knows nothing about session state, providers, or turn logic

```
┌─────────────────────────────────────────────────────────────┐
│  session-turn-runner.ts                                     │
│                                                             │
│  ... existing code ...                                      │
│                                                             │
│  // SINGLE HOOK POINT                                       │
│  if (noTools && noFinalReport && hasContent) {              │
│      const result = tryExtractLeakedToolCalls(content);     │──────┐
│      if (result) {                                          │      │
│          // use result.toolCalls, result.cleanedContent     │      │
│      }                                                      │      │
│  }                                                          │      │
│                                                             │      │
│  ... existing code ...                                      │      │
└─────────────────────────────────────────────────────────────┘      │
                                                                     │
┌─────────────────────────────────────────────────────────────┐      │
│  tool-call-fallback.ts (ISOLATED MODULE)                    │◄─────┘
│                                                             │
│  - Pattern matching for 5 tag types                         │
│  - JSON parsing with field normalization                    │
│  - UUID generation for tool IDs                             │
│  - Returns { toolCalls, cleanedContent, pattern }           │
│                                                             │
│  NO dependencies on session state, providers, or turns      │
└─────────────────────────────────────────────────────────────┘
```

---

## Problem Evidence

### Example Trace (from production)

The model tries multiple XML formats, all rejected:

```json
{
  "role": "user",
  "content": "how many users did sign up in the last 7d?"
},
{
  "role": "assistant",
  "content": "\n\n<tool_call>\n{\"name\": \"agent__batch\", \"arguments\": {\"calls\": [{\"id\": \"1\", \"tool\": \"bigquery__execute_sql\", \"parameters\": {\"sql\": \"SELECT MAX(bd_data_ingested_at) AS last_ingested_at, TIMESTAMP_DIFF(CURRENT_TIMESTAMP(), MAX(bd_data_ingested_at), MINUTE) AS age_minutes, 'watch_towers.spaces_latest' AS source_table FROM `netdata-analytics-bi.watch_towers.spaces_latest`\"}, {\"id\": \"2\", \"tool\": \"bigquery__execute_sql\", \"parameters\": {\"sql\": \"SELECT COUNT(*) AS new_users FROM `netdata-analytics-bi.app_db_replication.account_accounts_latest` WHERE created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)\"}}]} }\n</tool_call>"
},
{
  "role": "user",
  "content": "TURN-FAILED: No tools called, no final report/answer provided, but unexpected content is present.\n- If you believe you called tools, the system did not detect any tool calls. This usually means tool calls were not recognized (did you call them with the right prefix (check your instructions on how you are expected to call tools, usually: <tools>, <tool_calls>, etc - use the right prefix per your instructions)\n- If you believe you provided a final report/answer, it was not detected either. Ensure the final report is correctly formatted as instructed.\nRetry now: pay attention to the required syntax for tool calls and final report."
},
{
  "role": "assistant",
  "content": "\n\n<tools>\n{\"name\": \"bigquery__execute_sql\", \"arguments\": {\"sql\": \"SELECT MAX(bd_data_ingested_at) AS last_ingested_at, TIMESTAMP_DIFF(CURRENT_TIMESTAMP(), MAX(bd_data_ingested_at), MINUTE) AS age_minutes, 'watch_towers.spaces_latest' AS source_table FROM `netdata-analytics-bi.watch_towers.spaces_latest`\"}}\n</tools>\n\n<tools>\n{\"name\": \"bigquery__execute_sql\", \"arguments\": {\"sql\": \"SELECT COUNT(*) AS new_users FROM `netdata-analytics-bi.app_db_replication.account_accounts_latest` WHERE created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)\"}}\n</tools>"
}
```

Key observations (confirmed in logs):
1. No `toolCalls` array – only `content` is populated.
2. The model cycles through `<tool_call>`, `<tools>`, `<tool_calls>` variants but never the exact Hermes-required format.
3. Retries exhaust because the orchestrator keeps returning `TURN_FAILED_NO_TOOLS_NO_REPORT_CONTENT_PRESENT`.

---

## Analysis (current state, factual)

- `TURN_FAILED_NO_TOOLS_NO_REPORT_CONTENT_PRESENT` fires when `finalReport` is undefined, `sanitizedHasToolCalls` is false, and `sanitizedHasText` is true (`src/session-turn-runner.ts:1086-1110`).
- `sanitizeTurnMessages` (`src/session-turn-runner.ts:2168-2252`) parses native `toolCalls` and AI-Agent XML via `XmlToolTransport`; it drops malformed parameters and counts final_report attempts but ignores `<tools>/<tool_call>` text blobs.
- Tool selection per turn (`selectToolsForTurn` at `src/session-turn-runner.ts:1686-1708`) yields `allowedToolNames`; we should reuse that set when validating any recovered calls.
- Final-report fallbacks already exist (`tryAdoptFinalReportFromText` and tool-message fallback at `src/session-turn-runner.ts:779-1065`), so the new fallback must not interfere with that flow.
- XML transport already parses `<ai-agent-*>` tags. Hermes-style leaked tags appear only in native transport responses, so the fallback should stay native-only.

## Feasibility & complexity

- Scope: one parsing utility, one integration hook, unit + Phase 1 harness coverage, and docs sync.
- Complexity: medium; low cross-surface risk. Estimated effort ~1–1.5 engineering days including tests and documentation.

## Decisions (DECIDED)

1. **Extraction strictness when some calls are invalid**
   - ~~(a) Drop the entire payload if any call is invalid.~~
   - ~~(b) Extract valid calls, drop invalid, log warning.~~
   - **DECIDED: (c) Extract ALL tool calls - do NOT validate tool names during extraction.**

   **Rationale:** Let tool execution fail for unknown tools. This way:
   - The `<tools>` XML is removed from conversation history (no longer pollutes context)
   - The toolCalls array is properly structured in the assistant message
   - The chat template formats it correctly for the next turn
   - Model sees the "correct" tool call format in history and learns from it
   - Unknown tool errors are handled normally by tool executor (returns error result)

2. **When to run fallback**
   - **DECIDED:** Always when `sanitizedHasToolCalls === false` **and** `sanitizedHasText === true`.
   - No transport mode restriction - XML parser uses `<ai-agent-...>` tags, fallback uses `<tools>`/`<tool_call>` etc. No overlap, no double-parsing risk.

3. **Max calls enforcement**
   - **DECIDED: No truncation.** Extract ALL tool calls from leaked XML.
   - Let tool executor or provider handle any limits downstream.
   - Rationale: If model leaked 5 tool calls, it intended to call 5 tools. Extract all.

4. **ID generation strategy**
   - **DECIDED: (a)** `crypto.randomUUID()` per extracted call.
   - The leaked XML format does NOT include tool IDs, so we MUST generate them.
   - Simpler than deterministic hashing; retries already reissue calls.

## Implied decisions / defaults

- **Do NOT validate tool names during extraction** - let tool execution handle unknown tools (returns error).
- **Do NOT truncate** - extract all tool calls, let executor/provider handle limits.
- Preserve non-tool text as `cleanedContent` so downstream logging still shows model output, but keep tool calls in `toolCalls`.
- Log extraction with `remoteIdentifier='agent:sanitizer'`, severity `WRN`, including matched pattern and any JSON repair steps.
- **Always generate tool IDs** using `crypto.randomUUID()` since leaked formats don't include them.

---

## Proposed Solution

### New File: `src/tool-call-fallback.ts` (ISOLATED MODULE)

**This module is completely self-contained. It has:**
- No imports from session-turn-runner, ai-agent, or provider code
- No knowledge of session state, turns, or conversation
- Only utility imports: `crypto` for UUID, `parseJsonValueDetailed` from utils, `sanitizeToolName` from utils

**Interface:**
```typescript
// Input: raw content string from assistant message
// Output: always returns an object (never undefined)

export interface LeakedToolExtractionResult {
  content: string | null;  // cleaned content, or null if all content was XML tags
  toolCalls: ToolCall[];   // extracted tools, empty array if none found
}

export function tryExtractLeakedToolCalls(input: string): LeakedToolExtractionResult;
```

**Valid states:**

| State | content | toolCalls | Meaning |
|-------|---------|-----------|---------|
| 1 | `=== input` | `[]` | Nothing found, content unchanged |
| 2 | `"remaining"` | `[...]` | Extracted tools, some content remains |
| 3 | `null` | `[...]` | Extracted tools, no content left |

**Caller logic:**
```typescript
const result = tryExtractLeakedToolCalls(content);
if (result.toolCalls.length > 0) {
  // Use extracted tools and cleaned content
} else {
  // Nothing extracted, proceed with original flow
}
```

**The module:**
1. Scans text content for known XML-like tool call wrappers.
2. Extracts JSON payloads; supports single objects, arrays, and `agent__batch` shapes.
3. Normalizes field names (`function`→`name`, `parameters`→`arguments`).
4. Generates tool IDs using `crypto.randomUUID()` (leaked formats don't include IDs).
5. Does NOT validate tool names or truncate - extract all, let executor handle.
6. Returns extracted `ToolCall[]`, cleaned content, and matched pattern name.

### Patterns to Support

| Pattern | Example |
|---------|---------|
| `<tool_calls>...</tool_calls>` | Hermes format |
| `<tool_call>...</tool_call>` | Common variation |
| `<tools>...</tools>` | Another variation |
| `<function_call>...</function_call>` | Older format |
| `<function>...</function>` | Another older format |

### Field Normalization

Normalize all variations to AI SDK `ToolCall` format:

```typescript
{
  id: string,                        // generated via crypto.randomUUID()
  name: string,                      // from "name" | "function" | "tool"
  arguments: Record<string, unknown> // from "arguments" | "parameters"
}
```

| Input field | Normalized to |
|-------------|---------------|
| `name` | `name` |
| `function` | `name` |
| `tool` | `name` |
| `arguments` | `arguments` |
| `parameters` | `arguments` |

### JSON Formats to Support

**Single tool call:**
```json
{"name": "tool_name", "arguments": {...}}
```

**Multiple tool calls (array):**
```json
[
  {"name": "tool1", "arguments": {...}},
  {"name": "tool2", "arguments": {...}}
]
```

**Batch tool call:**
```json
{"name": "agent__batch", "arguments": {"calls": [...]}}
```

### Integration Point

In `src/session-turn-runner.ts`, before adding `TURN_FAILED_NO_TOOLS_NO_REPORT_CONTENT_PRESENT`, invoke the fallback when native transport has text but no tool calls. On success, replace `assistantForAdoption.toolCalls`, trim `assistantForAdoption.content`, update `sanitizedMessages`, and recompute `sanitizedHasToolCalls`/`sanitizedHasText` so the normal tool execution path proceeds.

### Transformation example

Input content:
```
\n\n<tools>{"name": "foo", "arguments": {"bar": 1}}</tools>\n\nSome other text
```

Output to orchestrator:
```typescript
{
  toolCalls: [ { id: "generated-uuid", name: "foo", parameters: { bar: 1 } } ],
  cleanedContent: "Some other text",
  pattern: "<tools>"
}
```

---

## Plan

1. **Create `src/tool-call-fallback.ts` (ISOLATED MODULE)**
   - Self-contained module with NO dependencies on session/turn/provider code.
   - Regex parser for the five tag variants; tolerate whitespace and repeated tags.
   - Use `parseJsonValueDetailed` to parse payloads; normalize field names.
   - Use `sanitizeToolName` to normalize tool names.
   - Generate UUIDs for tool IDs (required - leaked formats don't include them).
   - Do NOT validate tool names - extract all, let executor handle unknown tools.
   - Return result or undefined (no side effects, no logging inside module).

2. **Wire into `session-turn-runner` (SINGLE HOOK POINT)**
   - Add ONE call site, just before `TURN_FAILED_NO_TOOLS_NO_REPORT_CONTENT_PRESENT`.
   - Only when native transport, no toolCalls, but has text content.
   - If extraction succeeds: update `sanitizedMessages`, `assistantForAdoption`, flags; log WRN.
   - If extraction fails: proceed with existing TURN-FAILED logic unchanged.
3. **Unit Tests (standalone vitest)**
   - Test the module in isolation using examples from this document.
   - Pattern coverage, array/single/batch parsing, mixed text, invalid JSON.
   - Field normalization (`function`→`name`, `parameters`→`arguments`).
   - No harness needed - pure function testing.

4. **Integration Tests (Phase 1 harness - hook point only)**
   - Scripted assistant response with `<tools>` call → execution succeeds, no TURN-FAILED.
   - Unknown tool name in leaked XML → tool error returned (not TURN-FAILED).

5. **Docs**
   - Update `docs/specs/turn-validation.md` and `src/llm-messages.ts` comment for the new fallback path.

---

## Verification (2025-11-29)

- ✅ Isolated module exists at `src/tool-call-fallback.ts` and matches the intended tag coverage (`<tool_calls>`, `<tool_call>`, `<tools>`, `<function_call>`, `<function>`), using `parseJsonValueDetailed`, `sanitizeToolName`, and `crypto.randomUUID`.
- ✅ Single hook point wired into `src/session-turn-runner.ts` under native transport when text is present but no native tool calls were detected; logs with `remoteIdentifier=agent:sanitizer` on success.
- ✅ Unit coverage present at `src/tests/unit/tool-call-fallback.spec.ts` with pattern, normalization, mixed-content, and production-trace cases.
- ⚠️ `sanitizedHasText` is not recomputed after extraction; when all content is stripped the old `true` value persists. This can trigger the synthetic failure path even though `assistantForAdoption.content` is now empty.
- ⚠️ Extraction logging omits pattern/repair details (`repairs` from `parseJsonValueDetailed` and which tag matched); only count is reported.
- ⚠️ No Phase 1 harness scenario exercises the fallback; only unit tests exist. Contract coverage for the new behavior is missing from deterministic harness.
- ⚠️ Documentation updates called out in the plan (`docs/specs/turn-validation.md`, comments) are not present.

## Decisions Needed (record before implementation)

1. Recompute text flag after cleanup?
   - 1️⃣ Yes: set `sanitizedHasText = cleanedContent?.length > 0` after extraction so purely-tag outputs avoid synthetic failure when tool calls are recovered or removed. Reduces false synthetic retries; matches intent to reflect cleaned content.
   - 2️⃣ No: keep current behavior (flag derived from pre-clean text) so any leaked tags still count as “text present,” forcing a retry even when content becomes empty. Simpler but can misclassify tag-only payloads.

2. Extraction logging detail
   - 1️⃣ Include pattern + repair steps in warning (e.g., pattern name, `repairs` array from `parseJsonValueDetailed`, count of parsed vs dropped). Improves observability/debugging when JSON repair fires or parsing fails.
   - 2️⃣ Keep minimal log (count only) to reduce log noise. Lower verbosity but harder to debug malformed payloads.

3. Test & docs gap closure
   - 1️⃣ Add Phase 1 harness scenario(s) proving leaked `<tools>` calls execute and unknown-tool errors surface without `TURN_FAILED_NO_TOOLS_NO_REPORT_CONTENT_PRESENT`; update docs per plan. Aligns with quality rules and plan.
   - 2️⃣ Defer harness/doc updates. Faster now but violates documented “docs in same commit” rule and leaves regression gap.

## Testing Requirements

### Unit Tests (standalone vitest - NO harness needed)

Since the module is completely isolated, it can be tested directly with vitest using the example traces from this document.

**File:** `src/tests/unit/tool-call-fallback.spec.ts`

**Coverage:**
- All 5 tag patterns (`<tool_calls>`, `<tool_call>`, `<tools>`, `<function_call>`, `<function>`)
- Field normalization (`function`→`name`, `parameters`→`arguments`, `tool`→`name`)
- Single tool call, array of tools, batch format
- Mixed content (text + XML tags)
- No patterns found → `{ content: input, toolCalls: [] }`
- Invalid JSON in tags → `{ content: "remaining", toolCalls: [] }` (strips invalid XML)
- All content is valid XML → `{ content: null, toolCalls: [...] }`

**Test data:** Use the exact examples from "Problem Evidence" section above.

### Integration Tests (Phase 1 harness - for hook point only)

- Phase 1 scenario: scripted assistant response with `<tools>` call → tool executes, no TURN-FAILED.
- Phase 1 scenario: unknown tool name in leaked XML → tool error returned (not TURN-FAILED).

### Build Requirements

- Run `npm run lint` and `npm run build` (zero warnings/errors required).

## Documentation Updates

- `docs/specs/turn-validation.md`: describe the leaked-tool fallback path and when it runs.
- `src/llm-messages.ts`: clarify TURN-FAILED message comment mentioning fallback.

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| False positive extraction | Require valid JSON structure with `name` and `arguments` fields. |
| Performance impact | Regex scan over single-turn content (O(n)); content sizes are small. |
| Behavioral regression | Fallback is gated to native transport and only triggers where we would have failed; tests cover both paths. |
| Unknown tool names | Not a risk - tool executor returns error, model learns correct format from conversation structure. |
| Tool injection | Parameters parsed via `parseJsonValueDetailed`; unknown tools fail at execution; no special privileges. |
