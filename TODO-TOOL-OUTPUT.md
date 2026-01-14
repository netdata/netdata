# TODO: Tool Output Module (Replace Truncation)

## TL;DR
- Goal: eliminate blind truncation by introducing a `tool_output` internal tool that extracts from large tool results using a dedicated module.
- Large outputs are stored in a per-session temp directory; extractor decides full-chunked vs read/grep; fallback to truncate only inside the module.
- This keeps the main session loop clean and makes extraction strategies modular and testable.

## Analysis (current state, 2026-01-14)
- **Blind truncation happens in the main session**:
  - `applyToolResponseCap` truncates tool output in `src/ai-agent.ts` and logs warnings. (Evidence: `src/ai-agent.ts:1147-1215`)
  - Context guard drops tool output entirely with `(tool failed: context window budget exceeded)` in `src/session-tool-executor.ts`. (Evidence: `src/session-tool-executor.ts:457-617`)
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

## Plan (implementation, not started)
1) Create `src/tool-output/` module (policy + chunked extraction + read/grep adapter).
2) Add ToolOutputStore (session scoped) + write large tool outputs to configurable `/tmp/ai-agent-{session-trxid}/`.
3) Add internal tool `tool_output` in `internal-provider.ts`.
4) Remove or disable `applyToolResponseCap` in main session; replace with handle creation.
5) Add dynamic sub-agent spawn logic for `Read/Grep` only, pinned to session tmp dir.
6) Wire accounting + logs for: handle creation, extraction mode, failures, truncation fallback.
7) Update Phase 1 + Phase 2 tests.
8) Update documentation.

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

## Documentation Updates
- `docs/SPECS.md`: new tool_output behavior, routing, and storage.
- `docs/AI-AGENT-GUIDE.md`: how models should call tool_output.
- `docs/AI-AGENT-INTERNAL-API.md`: new tool schema.
- `docs/specs/tools-overview.md`: tool_output flow and limits.
- `README.md`: mention tool_output module and no blind truncation.
