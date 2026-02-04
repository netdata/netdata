# TL;DR
- Verify whether minimax-m2.1’s XML tool-call format is unsupported by the fallback parser.
- Decide if we should add fallback support for tool-name-as-tag + XML parameters.

# Analysis
- Fallback parser has two paths:
  - Wrapper tags (`<tool_calls>`, `<tool_call>`, `<tools>`, `<function_call>`, `<function>`) that expect JSON inside them (`src/tool-call-fallback.ts:104-110`, `src/tool-call-fallback.ts:347-361`).
  - Tool-name tags (`<agent__task_status>...</agent__task_status>`) for tools allowed in the current turn (`src/tool-call-fallback.ts:178-206`, `src/tool-call-fallback.ts:365-379`).
- User observed Minimax wrapper in assistant **content** (not native tool calls): `<minimax:tool_call><invoke name="agent__task_status">...</invoke></minimax:tool_call>`. This does not match current fallback patterns, so it is ignored when leaked in text.
- Snapshot evidence (assistant content, not native tool calls):
  - `0e8d79e3-b146-4d46-99ca-1b1d2c3bdd00.json.gz`:
    - `opTree.turns[1].attributes.assistant.content` contains `<minimax:tool_call><invoke name="filesystem_repos__RGrep">...`
    - `opTree.turns[1].ops[2].response.payload.textPreview` repeats the same block.
    - Same turn shows system failure text in conversation: `TURN-FAILED: Text output detected without any tool calls or final report/answer...` (evidence of leaked tool call not executing).
  - `24e707db-750e-4cac-ad01-6ae53280521b.json.gz`:
    - `opTree.turns[2].ops[0].response.payload.textPreview` contains final report XML followed by `<minimax:tool_call><invoke name="agent__task_status">...`.
  - `031ec822-2ce7-4f5c-b227-f04701c430be.json.gz` (nested session):
    - `opTree.turns[2].ops[2].childSession` is `freshdesk` (`callPath: neda:freshdesk`).
    - Inside `childSession.turns[8].ops[2].childSession` (agentId `tool_output.read_grep`) the LLM response textPreview/logs include `<minimax:tool_call><invoke name="tool_output_fs__Read">...` and `<invoke name="tool_output_fs__Grep">...` in `logs[].details.raw_response` and `response.payload.textPreview`.
- Minimax sometimes emits namespaced wrapper tags and `<invoke name="...">` (example: `<minimax:tool_call><invoke name="agent__task_status">...</invoke></minimax:tool_call>`), which are not matched by `TAG_PATTERNS` and are not parsed by the tool-name tag extractor (which expects `<agent__task_status>...</agent__task_status>`):
  - `src/tool-call-fallback.ts:104-110` only matches exact `<tool_call>` variants.
  - `src/tool-call-fallback.ts:178-206` only extracts `<toolName>...</toolName>` blocks for allowed tool names.
- JSON-only parsing:
  - `src/tool-call-fallback.ts:133-182` uses `parseJsonValueDetailed` and normalizes JSON objects/arrays into tool calls.
- User-provided minimax-m2.1 examples use tool-name-as-tag + XML parameter elements:
  - `<agent__task_status> ... <parameter name="status">in-progress</parameter> ... </agent__task_status>`
  - Some params are bare tags like `<pending>...</pending>` and `<now>...</now>`.
  - Malformed XML occurs: `<parameter name="need_to_run_more_tools">true</need_to_run_more_tools>` (mismatched closing tag).
  - Preamble text may appear before the XML block.
- Retry exhaustion forcing a final turn (not necessarily “5 retries”):
  - `src/session-turn-runner.ts:1738-1784` sets `contextGuard.setRetryExhaustedReason()` when attempts are exhausted and the turn is not already final.
  - Final turn tool restriction is handled in `src/session-turn-runner.ts:2007-2040` (final turn defaults to allowing only `agent__final_report`).

# Decisions
1. Fix strategy for minimax-m2.1 XML tool calls:
   - Chosen: **A) Add tool-name-as-tag parsing + `<parameter name="x">value</parameter>` support in fallback parser.**
2. How to merge repeated fields (e.g., multiple `<pending>` tags) into the single-string schema:
   - Chosen: **A) Join with newlines (one item per line).**
3. Bare tag handling for `agent__task_status`:
   - Chosen: **A) Accept only known task_status fields.**
4. Malformed `<parameter>` closing tags (`</x>` instead of `</parameter>`):
   - Chosen: **A) Accept `</x>` as valid close when `name="x"`.**
5. Boolean parsing for `"true"/"false"` values:
   - Chosen: **A) Convert to boolean (matches schema).**
6. Tool-name tag detection scope:
   - Chosen: **A) Only parse tags matching tools allowed for the turn.**
7. Malformed bare tag names with stray quote (e.g., `<ready_for_final_report">`):
   - Chosen: **A) Tolerate a trailing quote and treat as the known field name.**
8. Work alongside other in-progress changes:
   - Chosen: **A) Proceed with this feature only; ignore and do not delete/commit unrelated changes.**
9. Minimax wrapper support:
   - Chosen: **A) Option 1 — parse `<minimax:tool_call><invoke name="...">...</invoke></minimax:tool_call>` in leaked text.**

# Plan
- If A: extend fallback parser to detect `<tool_name>...</tool_name>` tags where tool name matches known tools, and parse `<parameter>` into a JSON object.
- Add unit test(s) in `src/tests/unit/tool-call-fallback.spec.ts` for the new XML pattern.
- Add Phase 2 harness scenario for tool-name-as-tag XML fallback parsing.
- Update documentation if runtime behavior changes.

# Implied Decisions
- None yet.

# Testing Requirements
- `npm run lint`
- `npm run build`
- Add/adjust unit tests (fallback parser).
- `npm run test:phase2`

# Documentation Updates Required
- If we change fallback behavior, update relevant docs (likely `docs/specs/IMPLEMENTATION.md` and/or `docs/specs/AI-AGENT-INTERNAL-API.md`).
