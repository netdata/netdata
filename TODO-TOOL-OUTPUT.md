# TODO: Tool Output Module (Replace Truncation)

## TL;DR
- Goal: eliminate blind truncation by introducing a `tool_output` internal tool that extracts from large tool results using a dedicated module.
- Large outputs are stored in a per-session temp directory; extractor decides full-chunked vs read/grep; fallback to truncate only inside the module.
- This keeps the main session loop clean and makes extraction strategies modular and testable.

## Analysis (current state, 2026-01-14)
- **Blind truncation happens in the main session**:
  - `applyToolResponseCap` truncates tool output in `src/ai-agent.ts` and logs warnings. (Evidence: `src/ai-agent.ts:1147-1215`)
  - Context guard drops tool output entirely with `(tool failed: context window budget exceeded)` in `src/session-tool-executor.ts`. (Evidence: `src/session-tool-executor.ts:457-617`)
- **Tool outputs are already truncated/dropped inside ToolsOrchestrator**:
  - Size cap + truncation happens before session-tool-executor sees the result. (Evidence: `src/tools/tools.ts:876-973`)
  - Token-budget enforcement can truncate or drop after execution. (Evidence: `src/tools/tools.ts:980-1033`)
- **Context guard enforces final turn on tool preflight**:
  - Tool budget reservation enforces final turn when exceeded. (Evidence: `src/context-guard.ts:391-409`)
  - ToolsOrchestrator uses those callbacks for external tool results. (Evidence: `src/ai-agent.ts:586-607`)
- **Batch tool schema is cached and based on current tool list**:
  - `cachedBatchSchemas` is built once from `orchestrator.listTools()` and reused. (Evidence: `src/tools/internal-provider.ts:932-988`)
  - Cache is only refreshed via `warmupWithOrchestrator()` (not called today). (Evidence: `src/tools/internal-provider.ts:1014-1017`)
- **Tool output is sanitized after execution**:
  - Tool output is sanitized for Unicode surrogate safety before any extra capping. (Evidence: `src/session-tool-executor.ts:457-467`)
- **No tool-output store exists**:
  - Tool results are appended directly to conversation/opTree; no handle, no retrieval path. (Evidence: `src/ai-agent.ts`, `src/session-tool-executor.ts`)
- **Internal tools are fixed**:
  - Only `agent__task_status`, `agent__final_report`, `agent__batch` exist. (Evidence: `src/tools/internal-provider.ts:48-120`)
- **Filesystem MCP guardrails exist**:
  - Root confinement, no `..`, no absolute paths enforced. (Evidence: `mcp/fs/fs-mcp-server.js:5-109`)
  - `Read`/`Grep` operate on relative paths under root. (Evidence: `mcp/fs/fs-mcp-server.js:332-402`)

## Problem Statement
- Large tool outputs are truncated or dropped, losing critical information.
- The model cannot control what is preserved.
- We need a consistent, single internal tool to extract from large outputs, using a dedicated extraction module.

## Non‑Negotiable Constraints (Costa)
- **All features must be tested across main components and their features.**
- **All truncation‑dependent tests must be adapted** because truncation moves into `tool_output`.
- **`tool_output` must remain isolated** (no spreading into core).
- **Core loops must not grow**; they must **shrink** (move logic out).
- **`tool_output` accounting must be aggregated at the session level**, similar to sub‑agents.
- **opTree + snapshots must include tool_output sub‑agent and extraction LLM calls.**
- **Phase 3 Tier1 must cover all `tool_output` modes with a real LLM.**
- **Remove `nova/gpt-oss-20b` from Phase 3 Tier1 model list.**

## Pre‑Change Test Baseline (2026-01-15T01:46:04+02:00)
- `npm run lint` — **PASS**
- `npm run build` — **PASS**
- `npm run test:phase1` — **PASS**
- `npm run test:phase2` — **PASS** (268/268)
- Observed warnings during Phase 2 (suite still passed):
  - MODEL_ERROR_DIAGNOSTIC (expected simulated errors in fixtures)
  - traced fetch clone warnings
  - unreadable env file warning (fixture)
  - lingering handles warning (fixture teardown)
  - persistence EEXIST warnings (fixture)

## Implementation Status (2026-01-15)
- **Module scaffold created** under `src/tool-output/`:
  - `types.ts`, `config.ts`, `store.ts`, `formatter.ts`, `stats.ts`, `chunking.ts`, `handler.ts`, `extractor.ts`, `provider.ts`.
- **Context guard budget preview added**:
  - `previewToolOutput` in `src/context-guard.ts` (used to evaluate handle text without forcing final turn).
- **ToolsOrchestrator types extended**:
  - `ToolOutputHandler` + handle request/response types in `src/tools/tools.ts`.
- **Core truncation paths still active** (not yet replaced):
  - size cap + token budget truncation still in `src/tools/tools.ts`.
  - `applyToolResponseCap` still in `src/ai-agent.ts` / `src/session-tool-executor.ts`.
- **Internal tool registration not done yet**:
  - `tool_output` not yet wired in `src/tools/internal-provider.ts` or `src/ai-agent.ts`.

## Implementation Status Update (2026-01-15)
- **tool_output is wired end-to-end**:
  - ToolOutputStore + handler + provider + extractor integrated and registered. (Evidence: `src/ai-agent.ts`, `src/tool-output/*`)
  - Oversized outputs are stored and replaced with handle messages; context guard previews handle text. (Evidence: `src/tools/tools.ts`, `src/context-guard.ts`, `src/tool-output/handler.ts`)
  - When budget reservation fails after storing a handle, the handle is returned instead of the failure stub to preserve the handle for the model. (Evidence: `src/tools/tools.ts`)
- **Test fixture update**:
  - Added `context-guard-budget` payload (10,000 bytes) to reliably trigger token-budget storage without size-cap. (Evidence: `src/tests/mcp/test-stdio-server.ts`, `src/tests/fixtures/test-llm-scenarios.ts`)
- **opTree payload sizing updated**:
  - tool_output extraction LLM ops now store message count + byte size and cap response previews to `TRUNCATE_PREVIEW_BYTES` to avoid memory blowups. (Evidence: `src/tool-output/extractor.ts`)
- **Legacy size cap fallback restored when tool_output disabled**:
  - toolResponseMaxBytes truncation re-applied when tool_output handler is absent. (Evidence: `src/tools/tools.ts`)
- **Phase 2 OOM resolved**:
  - Guarded InternalToolProvider batch schema rebuild to avoid recursive listTools during listTools. (Evidence: `src/tools/internal-provider.ts`)
- **Shared tool_output MCP root + handle format updated**:
  - Per-run root `/tmp/ai-agent-<run-hash>` with per-session `session-<uuid>/` subdirs.
  - Handles are `session-<uuid>/<file-uuid>`; read/grep prompts updated. (Evidence: `src/tool-output/runtime.ts`, `src/tool-output/store.ts`, `src/tool-output/extractor.ts`)
- **Harness stability + handle detection updates**:
  - Batch tool_output handle detection now parses JSON payloads. (Evidence: `src/tests/phase2-harness-scenarios/phase2-runner.ts`)
  - context-token-double-count selects post-tool request log by turn/subturn instead of log order. (Evidence: `src/tests/phase2-harness-scenarios/phase2-runner.ts`)
  - run-test-budget-truncation-preserves-output moved to sequential to avoid shared MCP restart contention. (Evidence: `src/tests/phase2-harness-scenarios/phase2-runner.ts`)

## Post-Change Test Runs (2026-01-15)
- `npm run lint` — **PASS**
- `npm run build` — **PASS**
- `npm run test:phase1` — **PASS** (240 tests)
- `npm run test:phase2` / `PHASE2_MODE=sequential node dist/tests/phase2-harness.js` — **FAIL (OOM)**  
  - Node hits heap limit after ~5 sequential scenarios.  
  - Evidence: `/tmp/phase2-seq.log` (OOM stack trace, heap ~4GB).

## Post-Change Test Runs (2026-01-15, latest)
- `npm run lint` — **PASS**
- `npm run build` — **PASS**
- `npm run test:phase1` — **PASS** (240 tests, Vitest poolOptions deprecation warning)
- `npm run test:phase2` — **PASS** (268/268; 158 parallel, 110 sequential)
  - Expected warnings during Phase 2:
    - MODEL_ERROR_DIAGNOSTIC (fixture coverage)
    - traced fetch clone warnings (fixtures)
    - unreadable env file warning (fixture)
    - lingering handles warning (fixture teardown)

## Feasibility Study (2026-01-14)
### Facts (with evidence)
- Tool outputs are truncated in **two** places today (ToolsOrchestrator + `applyToolResponseCap`), so replacing truncation requires changing multiple layers. (Evidence: `src/tools/tools.ts:876-973`, `src/ai-agent.ts:1147-1215`, `src/session-tool-executor.ts:465-467`)
- Context-guard enforcement happens **before** the session can see the oversized content, so a handle-based approach must integrate at or before the tool budget reservation step. (Evidence: `src/context-guard.ts:391-409`, `src/tools/tools.ts:980-1033`)
- `agent__batch` uses a cached tool schema built from the tool list; dynamic tools (like `tool_output`) will not be reflected unless we refresh that cache. (Evidence: `src/tools/internal-provider.ts:932-1017`)

### Stress points already covered by existing decisions
- Store outputs on disk with a handle and only for oversized cases → avoids memory blowups and keeps the main loop clean. (Decisions 1, 10, 11)
- Use a dedicated `tool_output` tool with explicit handle + extract instructions → gives the model control and preserves context budget. (Decisions 2, 2a, 2b)
- Route extraction strategies in a separate module and fall back to truncation only inside the module → isolates risk to the new module. (Decisions 3, 4, 5)
- Limit the read/grep sub-agent to Read + Grep only → keeps blast radius small. (Decision 8)

### Speculation (clearly labeled)
- **Speculation**: The design is feasible but will require a refactor in `src/tools/tools.ts`, `src/session-tool-executor.ts`, and `src/context-guard.ts`, plus updates to batch tool schema caching and tests. This is medium-to-high risk because these are core flow paths.

## Proposed Design (brainstorming, not final)
### 1) ToolOutputStore (per session)
- Create `/tmp/ai-agent-{session-trxid}/` (path configurable in `ai-agent.json`).
- Store each large tool output as a file with a random UUID filename.
- Directory removed on session end (success, failure, abort).
- Store metadata (bytes, lines, tokens) for routing decisions.

### 2) Internal Tool: `tool_output`
- Enabled only when ToolOutputStore is non-empty.
- Input:
  - `handle` (UUID)
  - `extract` (what to extract)
  - Optional: `mode` (`auto` | `full-chunked` | `read-grep` | `truncate`)
- Output:
  - **Plain text only** (no JSON), to avoid double-escaping.
  - Success format (exact):
    ```
    ABSTRACT FROM TOOL OUTPUT <TOOL_NAME> WITH HANDLE <UUID>, STRATEGY:<mode_used>:

    <natural language abstract>
    ```
  - Failure format (exact):
    ```
    TOOL_OUTPUT FAILED FOR <TOOL_NAME> WITH HANDLE <UUID>, STRATEGY:<mode_used>:

    <error message>
    ```
  - Not-found should be a **successful** response that explains what was searched and that no match was found.

### 2a) LLM-visible tool result when output is too large (exact message)
```
Tool output is too large (123456 bytes, 789 lines, 45678 tokens).
Call tool_output(handle = "<uuid>", extract = "what to extract").
Provide precise and detailed instructions in `extract` about what you are looking for.
```

### 2b) tool_output schema (exact)
```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["handle", "extract"],
  "properties": {
    "handle": {
      "type": "string",
      "minLength": 1,
      "description": "UUID handle of the stored tool output (provided in the tool-result message: tool_output(handle = \"<uuid>\", ...))."
    },
    "extract": {
      "type": "string",
      "minLength": 1,
      "description": "Provide precise, detailed instructions about what you need from the tool output (be specific, include keys/fields/sections if known)."
    },
    "mode": {
      "type": "string",
      "enum": ["auto", "full-chunked", "read-grep", "truncate"],
      "description": "Optional override. auto=module decides; full-chunked=LLM chunk+reduce; read-grep=dynamic sub-agent with Read/Grep; truncate=keeps top and bottom, truncates in the middle."
    }
  }
}
```

### 3) Extraction Module (separate module)
- Accepts `(documentPointer, query, policyConfig)`.
- Computes stats: bytes, lines, tokens, avg line size.
- Routing policy (initial proposal):
  - If tokens fit within `MAX_CHUNKS_FOR_FULL_MODE` OR avg line size > 1000 bytes → **full-chunked**.
  - Else → **read-grep**.
  - If chosen strategy fails → **truncate** (with explicit warning).
- **full-chunked (hardcoded)**:
  - Chunk size = `maxContextTokens - maxOutputTokens - promptTokens`.
  - 10% overlap between chunks (configurable; default 10%).
  - Chunk balancing: estimate number of chunks using max chunk size, then **rebalance** by dividing document size by chunk count to get a uniform chunk size (avoids a tiny last chunk).
  - Map: 1 LLM call per chunk; Reduce: 1 LLM call to synthesize.
  - **LLM output contract**: emit the **exact** XML final-report wrapper (no status attribute), using the session nonce:
    - `<ai-agent-<NONCE>-FINAL format="text"> ... </ai-agent-<NONCE>-FINAL>`
    - If found: content = extracted result.
    - If not found: content should explain what was searched and why no match was found.
    - Failure = no final-report produced.
  - Prefer reusing existing final-report parsing utilities to avoid interleaved thinking.
  - **read-grep**:
    - Spawn dynamic sub-agent with filesystem MCP pinned to the session tmp dir.
    - Expose only `Read` and `Grep` tools.
    - Accept sub-agent final report if non-synthetic; treat synthetic final-report as failure.
    - **Prompt requirement**: explicitly tell the sub-agent that the **handle is the filename** for Read/Grep.

### 3a) Prompts (draft, must be finalized)
#### Full-chunked (map) system prompt
```
You are an internal extraction agent. You must extract information from a document chunk.

SOURCE TOOL
- Name: <TOOL_NAME>
- Arguments (verbatim JSON): <TOOL_ARGS_JSON>

DOCUMENT STATS
- Bytes: <DOC_BYTES>
- Lines: <DOC_LINES>
- Tokens (estimate): <DOC_TOKENS>

CHUNK INFO
- Index: <CHUNK_INDEX> of <CHUNK_TOTAL>
- Overlap: <CHUNK_OVERLAP_PERCENT>%

WHAT TO EXTRACT
<EXTRACT_INSTRUCTIONS>

OUTPUT FORMAT (required)
- Emit exactly one XML final-report wrapper:
  <ai-agent-<NONCE>-FINAL format="text"> ... </ai-agent-<NONCE>-FINAL>
- Put your extracted result inside the wrapper.
- If no relevant data exists, output:
  NO RELEVANT DATA FOUND
  <short description of what kind of information is available in this chunk>
```

#### Full-chunked (reduce) system prompt
```
You are an internal extraction agent. You must synthesize multiple chunk extractions into one final answer.

SOURCE TOOL
- Name: <TOOL_NAME>
- Arguments (verbatim JSON): <TOOL_ARGS_JSON>

WHAT TO EXTRACT
<EXTRACT_INSTRUCTIONS>

CHUNK OUTPUTS
<CHUNK_OUTPUTS>

OUTPUT FORMAT (required)
- Emit exactly one XML final-report wrapper:
  <ai-agent-<NONCE>-FINAL format="text"> ... </ai-agent-<NONCE>-FINAL>
- If no relevant data exists, output:
  NO RELEVANT DATA FOUND
  <short description of what kind of information is available across the chunks>
```

#### Read-grep sub-agent system prompt
```
You are an internal extraction agent. You can only use Read and Grep tools.

IMPORTANT: The handle is the filename.
Use the filename exactly as given when calling Read or Grep.

SOURCE TOOL
- Name: <TOOL_NAME>
- Arguments (verbatim JSON): <TOOL_ARGS_JSON>
- Handle/filename: <HANDLE_FILENAME>
- Root directory: <SESSION_TMP_DIR>

WHAT TO EXTRACT
<EXTRACT_INSTRUCTIONS>

OUTPUT FORMAT (required)
- Emit exactly one XML final-report wrapper:
  <ai-agent-<NONCE>-FINAL format="text"> ... </ai-agent-<NONCE>-FINAL>
- If no relevant data exists, output:
  NO RELEVANT DATA FOUND
  <short description of what kind of information is available in the file>
```

### 4) Remove main-session truncation
- Move truncation logic out of `applyToolResponseCap` into the extraction module.
- Main session returns a handle instead of truncated content.

## Decisions / Questions (need user input)
1) **Storage type**
   - A) `/tmp/ai-agent-{session-trxid}/` (current proposal)
   - B) In-memory only
   - C) Hybrid
   - Decision: **A** (uses existing MCP guardrails + dynamic sub-agent)
   - Note: base path must be configurable in `ai-agent.json`.
   - Directory name fixed: `ai-agent-{session-trxid}`. Each tool output uses a random UUID filename/handle.

2) **ToolOutputStore limits**
   - A) No TTL, delete only on session end
   - B) Add caps (max files/bytes)
   - Decision: **A** (no caps; data already held in memory by tool result)

3) **Routing thresholds**
   - Default `MAX_CHUNKS_FOR_FULL_MODE = 1` (single-pass only; no reduce).
   - Keep line-size threshold (avg line size > 1000 bytes → full-chunked).
   - Both thresholds should be configurable (config file + frontmatter override).

4) **Fallback behavior**
   - If extraction fails, should `truncate` be returned or a hard error?
   - Decision: **A** (return truncate with explicit warning + log)
   - Definition: **failure only means the strategy could not execute** (e.g., chunked LLM call failed, sub-agent spawn failed). “Not found” is **not** a failure.

5) **Fallback modes naming**
   - Current spec uses `truncate` as a single fallback mode.
   - Proposal: replace `truncate` with explicit modes:
     - `keep-top`, `keep-bottom`, `keep-top-and-bottom`, `keep-middle`.
   - Decision: **A** (keep `truncate` as a single mode for now).

6) **Full-chunked XML wrapper strictness**
   - Use the **exact final-report XML wrapper** (no status attribute):
     - `<ai-agent-<NONCE>-FINAL format="text"> ... </ai-agent-<NONCE>-FINAL>`
   - Decision: **closing tag is optional** (models may omit it).
   - Implication: extractor path must use lenient parsing or a special parser that accepts missing closing tags.

7) **Auto-invoke behavior**
   - A) Return handle only; model calls `tool_output` explicitly.
   - B) Auto-invoke `tool_output` with a default extract.
   - Decision: **A** (return handle only).

8) **Dynamic sub-agent tool allowlist**
   - A) Only `Read` and `Grep`
   - B) `Read`, `Grep`, `ListDir`
   - Decision: **A** (Read + Grep only).

9) **Session tmp directory naming**
   - Decision: **A** (`ai-agent-{session-trxid}`).
   - Each tool output uses a random UUID filename/handle.

10) **Store policy**
   - A) Store only oversized outputs
   - B) Store all outputs
   - Decision: **A** (only oversized outputs).

11) **Oversized detection**
   - A) `toolResponseMaxBytes` only
   - B) token budget overflow only
   - C) both (bytes cap OR projected token overflow)
   - Decision: **C** (both).

12) **Handle message visibility**
   - A) Tool result only
   - B) Tool result + log entry
   - C) Tool result + log + XML-NEXT notice
   - Decision: **B** (tool result + log entry).

13) **ToolOutputStore lifetime**
   - A) Keep until session end
   - B) Delete immediately after successful extraction
   - Decision: **A** (keep until session end; allows multiple extractions).

14) **Final turn availability**
   - A) Not available in final turn
   - B) Available in final turn
   - Decision: **A** (not available in final turn).

15) **Tool visibility**
   - A) Listed when ToolOutputStore is non-empty
   - B) Only mentioned in tool-result message
   - Decision: **A** (listed when store non-empty).

16) **Batch tool compatibility**
   - A) Allow `tool_output` inside `agent__batch`
   - B) Disallow inside batch
   - Decision: **A** (allow in batch).

17) **Extractor prompt context**
   - A) Include tool name + arguments (metadata)
   - B) Only query + doc stats
   - Decision: **A** (include tool name + arguments; low complexity).
   - Tool arguments format: **A** (verbatim JSON string).

18) **Interception point for ToolOutputStore + handle replacement**
   - Context: tool outputs are currently size‑capped and budget‑checked in ToolsOrchestrator **before** SessionToolExecutor sees them. (Evidence: `src/tools/tools.ts:876-1033`)
   - A) Intercept in `ToolsOrchestrator.executeWithManagement` right after raw tool output, before size cap/budget/cache
     - Pros: preserves raw output; avoids double caps; lets context guard evaluate the handle message.
     - Cons: deepest core change; touches cache + budget flow.
   - B) Intercept in `SessionToolExecutor` after managed result
     - Pros: smaller surface change.
     - Cons: only sees already‑truncated output; context guard already fired; defeats goal.
   - C) Intercept inside provider (MCP/REST) before ToolsOrchestrator
     - Pros: preserves raw output.
     - Cons: duplicated logic per provider; misses internal/sub‑agent tools.
   - Recommendation: **A** (single authoritative place; preserves full output).
   - Decision: **A** (best maintainability: one authoritative path in ToolsOrchestrator).

19) **Context‑guard behavior when returning a handle**
   - Context: tool budget reservation currently enforces a forced final turn on overflow. (Evidence: `src/context-guard.ts:391-409`)
   - A) Evaluate budget on the handle message and **do not** force final turn unless the handle itself overflows
     - Pros: preserves normal flow; handle stays small; aligns with goal.
     - Cons: requires new guard path and tests.
   - B) Keep forced final turn even when returning a handle
     - Pros: minimal guard changes.
     - Cons: negates benefit (model can’t use tool_output if final turn disables tools).
   - Recommendation: **A**.
   - Decision: **A**.

20) **Extraction model selection (full‑chunked path)**
   - A) Use the **current** provider/model pair for the active turn (default)
     - Pros: minimal config; consistent behavior.
     - Cons: might be expensive; retries tied to current provider health.
   - B) Use the session’s full `targets` list (in order)
     - Pros: resiliency across providers.
     - Cons: more variability; may change output quality.
   - C) Add a dedicated config override for extraction targets
     - Pros: explicit control; can choose cheaper model.
     - Cons: new config surface + docs.
   - Recommendation: **C** with default fallback to **A** when not configured.
   - Decision: **Configurable**; if absent, default to current session provider/model.

21) **Tool cache interaction for oversized outputs**
   - Context: cache stores the **post‑processed** tool result. (Evidence: `src/tools/tools.ts:1248-1273`)
   - A) Skip caching when output is stored in ToolOutputStore
     - Pros: avoids caching handles/truncated results; simpler behavior.
     - Cons: less cache reuse for large outputs.
   - B) Cache the handle message only
     - Pros: consistent caching layer; avoids storing large payloads.
     - Cons: handle may expire or be session‑scoped; stale reuse risk.
   - C) Cache raw output separately (metadata + content) and re‑emit handle on hit
     - Pros: preserves large outputs across cache hits.
     - Cons: more complexity; potential storage bloat.
   - Recommendation: **A** (simple + safe for session‑scoped handles).
   - Decision: Cache behavior is unchanged for normal tools; if a cached or live tool output is oversized we still store it in ToolOutputStore and return the handle message; **tool_output results are never cached**.

22) **Stored content fidelity (raw vs sanitized)**
   - Context: tool output is sanitized before LLM use. (Evidence: `src/session-tool-executor.ts:457-467`)
   - A) Store **raw** tool output
     - Pros: maximum fidelity.
     - Cons: may contain invalid surrogates; file write or downstream parsing risk.
   - B) Store **sanitized** output only
     - Pros: safer for file I/O and LLM extraction.
     - Cons: tiny fidelity loss for invalid codepoints.
   - C) Store both (raw + sanitized)
     - Pros: fidelity + safety.
     - Cons: extra storage + complexity.
   - Recommendation: **B** unless you need exact raw fidelity.
   - Decision: **B** (store sanitized output).

23) **Metadata storage format**
   - A) In‑memory map keyed by handle + output file only
     - Pros: simplest; no extra files.
     - Cons: lost on crash; can’t inspect later.
   - B) Sidecar JSON file per handle
     - Pros: debuggable; survives crashes during session.
     - Cons: more files; cleanup complexity.
   - C) Both in‑memory + sidecar
     - Pros: fastest lookups + persistence.
     - Cons: most complex.
   - Recommendation: **A** (session‑scoped storage).
   - Decision: **A**.

24) **tool_output opTree payload sizing**
   - Context: tool_output extraction LLM calls can include very large chunk text in opTree.
   - Decision: **Store summary-only requests** (message count + byte size) and **truncate response previews** using `TRUNCATE_PREVIEW_BYTES`.
   - Rationale: avoids memory blowups while still including extraction LLM calls in opTree/snapshots.

25) **Size cap when tool_output is disabled**
  - Context: without tool_output handler, toolResponseMaxBytes is no longer enforced.
  - Decision: **Restore legacy truncation when tool_output is disabled** (preserve size cap only in that path).
  - Rationale: maintain backward compatibility and prevent unbounded tool outputs (e.g., read/grep sub-agent).

26) **Shared tool_output MCP root + handle format**
  - Decision: **Use a shared `tool_output_fs` MCP server** rooted at a per-run base directory (`/tmp/ai-agent-<run-hash>`).
  - Each session writes to `session-<uuid>/...` under that base root.
  - Handles will be `session-<uuid>/<file-uuid>` (relative path under the shared root).
  - Tools exposed: **Read + Grep only**.
  - Implication: tool_output must not depend on `.ai-agent.json`; it injects its own MCP config.

## Plan (implementation in progress)
1) Decisions finalized (18–23 complete). ✅
2) Create `src/tool-output/` module (store + policy + extraction strategies). ✅ (scaffolded)
3) Add ToolOutputStore (session scoped) + write oversized tool outputs to configurable `/tmp/ai-agent-{session-trxid}/`. ✅
4) Refactor tool-output pipeline:
   - Intercept raw tool outputs at the chosen layer. ✅
   - Replace truncation with handle message (per 2a) when oversized. ✅
   - Ensure context guard reserves tokens for handle text, not full output. ✅
   - Remove/disable redundant `applyToolResponseCap` path. ✅
5) Add internal tool `tool_output` in `internal-provider.ts` and wire to module. ✅
6) Implement extraction strategies:
   - full‑chunked (map/reduce) using selected extraction model targets. ✅
   - read/grep via dynamic sub‑agent with filesystem MCP pinned to tmp dir. ✅
7) Fix batch schema caching so `tool_output` is included when enabled. ✅
8) Wire accounting + logs for: handle creation, extraction mode, failures, truncation fallback. ✅
9) Update Phase 1 + Phase 2 tests (size cap + context‑guard scenarios), plus any truncation‑dependent fixtures. ✅
10) Run full lint/build and test suites per project rules. ✅ (lint/build/phase1/phase2 pass)
11) Update documentation (specs + guide + internal API + README). ✅
12) Update Phase 3 Tier1:
   - Remove `nova/gpt-oss-20b` from Tier1 model list.
   - Add Tier1 scenarios that exercise all `tool_output` modes with a real LLM.
   - Update documentation in `docs/TESTING.md` to reflect the Tier1 model list. ⬜
13) Update tool_output shared MCP:
   - Use shared base root with run-level hash.
   - Update handle format to `session-<uuid>/<file-uuid>`.
   - Ensure read/grep sub-agent uses the shared MCP config (no config-file dependency). ✅

## Implied Decisions
- Truncation is now an **extraction fallback**, not a main-session default.
- Extraction module owns all strategy choices and policies.

## Testing Requirements
- Unit:
  - ToolOutputStore lifecycle + cleanup
  - `tool_output` routing rules
  - full-chunked extract (map + reduce)
  - read/grep path via dynamic sub-agent
  - truncate fallback when extraction fails
- Phase 2 harness:
  - Large tool output → handle → tool_output extract
  - Huge single-line JSON → forces full-chunked
  - Standard multi-line text → read/grep path
  - Update all existing truncation scenarios to validate handle + tool_output behavior
- Phase 3 Tier1:
  - Real LLM coverage for all `tool_output` modes:
    - `auto` → full-chunked
    - `auto` → read-grep
    - `mode=full-chunked`
    - `mode=read-grep`
    - `mode=truncate`

## Documentation Updates
- `docs/SPECS.md`: new tool_output behavior, routing, and storage.
- `docs/AI-AGENT-GUIDE.md`: how models should call tool_output.
- `docs/AI-AGENT-INTERNAL-API.md`: new tool schema.
- `docs/specs/tools-overview.md`: tool_output flow and limits.
- `README.md`: mention tool_output module and no blind truncation.
