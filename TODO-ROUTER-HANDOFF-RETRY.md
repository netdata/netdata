# TODO: Typed tool errors + executed tool counting (router handoff retry fix)

## TL;DR
- Add **typed tool errors** with a code-level map of meanings (executed vs not, short description).
- Remove **string-matching** on tool error messages; use structured kinds instead.
- Turn failure should happen **only** when **no tool was executed** (schema/tool-existence failures). Tool errors must **not** force a turn failure.
- Add/ensure Phase 1 harness coverage for invalid router handoff params → retry → success.

---

## Status Update (Jan 18 2026)
- **Code changes:** complete.
  - Added typed tool errors (`src/tools/tool-errors.ts`) and wired them through MCP/REST/internal/router providers, `ToolsOrchestrator`, `SessionToolExecutor`, and `TurnRunner`.
  - Updated tool counting to record **executed** tools only.
  - Removed tool-error string matching in turn logic; replaced with `toolFailureKinds` + `hadToolFailure`.
  - Added Phase 2 harness scenario for router invalid params → retry (`run-test-router-handoff-invalid-retry`).
  - Updated docs: `docs/specs/IMPLEMENTATION.md`, `docs/specs/DESIGN.md`, `docs/skills/ai-agent-guide.md`.
- **Tests:** `npm run test:phase1` PASS, `npm run test:phase2` PASS, `npm run lint` PASS, `npm run build` PASS.
- **Reconfirmed decisions:** Option **1.1** (typed `ToolExecutionError`) and **2.1** (keep `ToolExecutor` returning `string`).

---

## Decisions (Made)

### Decision A — **Typed tool error taxonomy (chosen by Costa)**
**Decision:** Use a **small, explicit `ToolErrorKind` union** and a **code-level map of meanings** (executed vs not + description).  
**Reason:** Keeps behavior deterministic and avoids brittle string matching.

**Proposed kinds (draft):**
- `unknown_tool` → tool name does not exist (**executed: false**)
- `not_permitted` → tool blocked by allowlist (**executed: false**)
- `invalid_parameters` → schema validation failed (**executed: false**)
- `limit_exceeded` → per-turn tool limit (**executed: false**)
- `canceled` → stop/abort requested before execution (**executed: false**)
- `timeout` → tool call timed out (**executed: true**)
- `transport_error` → network/JSON-RPC/connection error (**executed: true**)
- `execution_error` → tool returned error at transport layer (**executed: true**)
- `internal_error` → unexpected crash (**executed: true** unless we decide otherwise)

**Requirement:** The map of meanings must live in code (not just docs).

### Decision B — **Turn failure rule**
**Decision:** A turn fails **only** when **no tool was executed**.  
Tool errors (including JSON-RPC errors and timeouts) **must not** force turn failure.

### Decision C — **Token-budget drops**
**Decision:** Token-budget drops **still count as executed tools**.

---

## Analysis (Current Code Behavior)

### 1) Tool call flow and where processing happens
- **LLM tool calls** are handled in `src/session-turn-runner.ts` → uses `SessionToolExecutor.createExecutor(...)`.
- **Tool execution** happens in `src/session-tool-executor.ts` → calls `ToolsOrchestrator.executeWithManagement(...)`.
- **Providers** execute in `src/tools/*-provider.ts`.
- **Post-processing** (UTF-8 sanitization, token guard, accounting, XML transport) happens in `src/session-tool-executor.ts`.

### 2) Tool executor returns only a string
- Tool executor type: `ToolExecutor` in `src/session-tool-executor.ts`.
- Used by `src/session-turn-runner.ts` and `src/tool-call-fallback.ts`.
- Any change in return type impacts those call sites and fallback tool extraction.

### 3) Schema validation happens inside providers
- MCP: `src/tools/mcp-provider.ts` → AJV, throws `ToolExecutionError('invalid_parameters', 'Validation error: ...')`
- REST: `src/tools/rest-provider.ts` → AJV, throws `ToolExecutionError('invalid_parameters', 'Validation error: ...')`
- Router tool: `src/orchestration/router.ts` → throws `ToolExecutionError('invalid_parameters', ...)`
- Internal tools: `src/tools/internal-provider.ts` → validation errors wrapped as `ToolExecutionError('invalid_parameters', ...)`

### 4) Error propagation is structured (tool path)
- Providers/orchestrator throw `ToolExecutionError` with a `kind`.
- `SessionToolExecutor` captures `ToolExecutionError` and stores `toolFailureKinds`.
- `SessionTurnRunner` uses `toolFailureKinds`/`hadToolFailure` (no `TOOL_FAILED_PREFIX` matching).

### 5) Tool counts now increment on execution
- `SessionToolExecutor` records execution **after** tool invocation starts (or for error kinds marked `executed: true`).
- Schema validation / unknown tool **do not** count as executed.

### 6) Remaining string matches (non-tool path)
- LLM error classification still matches strings in `src/llm-providers/base.ts` (timeouts/rate limits).
- MCP shared registry timeout detection still checks message text in `src/tools/mcp-provider.ts` (request timed out).
- Some tests assert on specific error message substrings (intentional).

---

## Decisions (Pending)

### Decision 0 — **Optimization priority for this change**
**Context:** Costa asked what we are optimizing for (minimal changes vs clarity/maintainability).

**Options:**
1) **Minimal change / lowest blast radius**
   - **Pros:** Lower risk, faster to ship.
   - **Cons:** May leave some string-matching or edge cases.
2) **Clarity + maintainability (preferred by assistant)**
   - **Pros:** Removes brittle string parsing; cleaner error handling.
   - **Cons:** Slightly larger refactor (providers + orchestrator + executor).
3) **Balance (targeted refactor only where needed)**
   - **Pros:** Reasonable risk, improves key paths.
   - **Cons:** Some inconsistencies may remain.

**Recommendation:** Option 2 (clarity + maintainability), because typed errors are long‑term infrastructure and reduce future failure modes.

**Decision (Costa, Jan 18 2026):** **Option 2 — Clarity + maintainability.**

### Decision 1 — **Where to carry typed errors**
**Context:** We need structured errors without breaking tool-output strings and without massive signature changes.

**Options:**
1) **Throw `ToolExecutionError` class** (preferred): providers/orchestrator throw a typed error with `kind`, `message`, optional `code`, and `executed` derived from kind.
   - **Pros:** Minimal surface change; keep tool executor returning string; no major API signature change.
   - **Cons:** Requires wrapping/propagating errors across layers.
2) **Extend `ToolExecuteResult`** with `errorKind` and `errorCode`, and return `ok:false` from providers.
   - **Pros:** Structured at the provider boundary; less reliance on thrown errors.
   - **Cons:** Requires more refactors in `ToolsOrchestrator` and may touch more call sites.

**Recommendation:** Option 1 (typed error class + map of meanings).

**Decision (Costa, Jan 18 2026):** **Option 1 — Throw `ToolExecutionError` class.**

### Decision 2 — **Should we change `ToolExecutor` return type?**
**Context:** We can attach structured info via state/error objects without changing return type, or we can return `{ output, errorKind?, executed? }`.

**Options:**
1) **Keep return type = string** and record structured error metadata in `ToolExecutionState`.
   - **Pros:** Low blast radius; keeps `tool-call-fallback` intact.
   - **Cons:** Need extra state plumbing for metadata.
2) **Return structured object** from executor.
   - **Pros:** Explicit; easier to reason about.
   - **Cons:** Larger refactor across `session-turn-runner` and `tool-call-fallback`.

**Recommendation:** Option 1 (keep string return).

**Decision (Costa, Jan 18 2026):** **Option 1 — Keep `ToolExecutor` returning `string`.**

---

## Plan (After Decisions)
1) Add `ToolErrorKind` + `ToolExecutionError` + `TOOL_ERROR_KIND_MEANINGS` map (new file, likely `src/tools/tool-errors.ts`).
2) Update providers (MCP/REST/router/internal) to throw or return typed errors.
3) Update `ToolsOrchestrator` to preserve typed errors (avoid `new Error(msg)`).
4) Update `SessionToolExecutor` catch block:
   - Use typed errors for `unknown_tool`, `invalid_parameters`, `limit_exceeded`, etc.
   - Adjust executed tool counters accordingly (decrement when `executed=false`).
   - Remove string matching in favor of structured checks.
5) Update `SessionTurnRunner` and `llm-providers/base.ts` to stop string matching where possible.
6) Add/confirm Phase 1 harness test for invalid router handoff → retry.
7) Run tests (all except phase3): `npm run test:phase1`, `npm run test:phase2`, `npm run lint`, `npm run build`.

---

## Implied Decisions
- Decide how to classify edge cases like `timeout`, `transport_error`, `internal_error` as executed vs not.
- Decide how far to apply typed errors (tools only vs LLM provider errors too).

---

## Testing Requirements
- `npm run test:phase1`
- `npm run test:phase2`
- `npm run lint`
- `npm run build`

---

## Documentation Updates Required
If tool error semantics or turn failure rules change:
- `docs/specs/IMPLEMENTATION.md`
- `docs/specs/DESIGN.md`
- `docs/skills/ai-agent-guide.md`
