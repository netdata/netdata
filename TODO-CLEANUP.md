This is a live application developed by ai assistants. I have the impression that over time the application lost a few key principles:

1. separation of concerns (isolated modules that do not touch the internals of each other)
2. simplicity (refactors introduced transformations instead of completing the refactoring)
3. single point of reference (multiple similar functions with tiny or no differences)
4. unecessary complexity (features that could implemented in a much simpler way, but the assistants overcomplicated things)
5. bad practices, smelly code

I need you to:

1. Complete the thorough code review to identify a single, risk-free, consolidation, simplification, improvement on separation of concerns
2. Make sure this aspect is well tested before touching the code - if you need to add tests, add them
3. Improve it
4. Make sure the tests still pass
5. Make sure documentation is updated as necessary

Do not ask any questions. If you have to ask questions, the candidate you picked is not right. We want only low-risk candidates that are obvious how to fix without breaking any of the existing features and functionality.

Append all you findings in this document.

---

## TL;DR
- Consolidate duplicated prompt-section formatting used by the OpenAI and Anthropic completions headends into a single helper to keep one source of truth.
- Add a focused unit test for the shared helper, then refactor both headends to use it with no behavior change.
- Run lint + build + unit tests to confirm zero regressions.

## Analysis (facts only)
- Duplicate prompt section prefixes and assembly exist in both completions headends:
  - `src/headends/openai-completions-headend.ts:75-77` and `src/headends/openai-completions-headend.ts:999-1027`
  - `src/headends/anthropic-completions-headend.ts:75-77` and `src/headends/anthropic-completions-headend.ts:964-991`
- The formatting is identical: build `System context`, `Conversation so far`, and `User request` sections, joined with `\n\n`.
- This duplication violates the “single point of reference” principle and risks divergence if one headend changes the format while the other does not.

## Decisions
- No user decisions required. Scope is a safe refactor with no behavior change.

## Plan
1. Add `src/headends/completions-prompt.ts` with a single `buildPromptSections()` helper that formats the three sections exactly as today.
2. Add a unit test for `buildPromptSections()` under `src/tests/unit/` to lock behavior.
3. Replace the duplicated prompt assembly in `openai-completions-headend.ts` and `anthropic-completions-headend.ts` to call the helper.
4. Run `npm run lint`, `npm run build`, and `npm run test:phase1`.

## Implied Decisions
- Keep the existing error messages and validation logic in each headend (the helper only formats strings).
- No change to output schemas, routing, or streaming behavior.

## Testing requirements
- Unit test: new test for shared prompt formatting helper.
- Required checks: `npm run lint`, `npm run build`, `npm run test:phase1`.

## Documentation updates required
- None. This is a refactor with no behavior or schema changes.

## Findings (append-only)
- Duplicated prompt-prefix constants and prompt assembly logic in both completions headends:
  - `src/headends/openai-completions-headend.ts:75-77`, `src/headends/openai-completions-headend.ts:999-1027`
  - `src/headends/anthropic-completions-headend.ts:75-77`, `src/headends/anthropic-completions-headend.ts:964-991`
- Instructions reference `docs/docs/AI-AGENT-INTERNAL-API.md`, but the file lives at `docs/AI-AGENT-INTERNAL-API.md` (path mismatch only; not in scope for this change).

---

## Iteration 2 - Consolidate final report content selection

### TL;DR
- Deduplicate the `resolveContent` logic in OpenAI/Anthropic completions headends into a shared helper.
- Add a unit test for the shared helper, then refactor both headends to use it with no behavior change.

### Analysis (facts only)
- Identical `resolveContent` methods exist in both headends:
  - `src/headends/openai-completions-headend.ts:1122-1131`
  - `src/headends/anthropic-completions-headend.ts:1004-1012`
- Both implementations:
  - Prefer JSON when `finalReport.format === 'json'` and `content_json` exists.
  - Otherwise return `content` when non-empty.
  - Fallback to the streamed `output`.
- This is a classic “single point of reference” violation with no behavioral differences.

### Decisions
- No user decisions required. Scope is a safe refactor with no behavior change.

### Plan
1. Add `src/headends/completions-response.ts` with `resolveFinalReportContent()` that matches current logic.
2. Add a unit test for the helper under `src/tests/unit/`.
3. Replace both headend `resolveContent` implementations to call the helper.
4. Run `npm run lint`, `npm run build`, `npm run test:phase1`.

### Implied Decisions
- Keep the existing input validation and error paths unchanged.
- No changes to schemas, routing, or streaming behavior.

### Testing requirements
- Unit test: shared final report content resolver helper.
- Required checks: `npm run lint`, `npm run build`, `npm run test:phase1`.

### Documentation updates required
- None. Refactor only.

### Findings (append-only)
- Duplicated final report content resolution logic in completions headends:
  - `src/headends/openai-completions-headend.ts:1122-1131`
  - `src/headends/anthropic-completions-headend.ts:1004-1012`

---

## Iteration 3 - Consolidate LLM usage aggregation

### TL;DR
- Move duplicated LLM usage aggregation logic to a shared helper.
- Keep headend response shapes the same by mapping the shared totals.

### Analysis (facts only)
- Both completions headends implement near-identical LLM usage aggregation:
  - `src/headends/openai-completions-headend.ts:81-91`
  - `src/headends/anthropic-completions-headend.ts:77-88`
- Each filters `AccountingEntry` to LLM entries and sums `inputTokens`/`outputTokens`.
- Differences are only naming (`prompt/completion` vs `input/output`).

### Decisions
- No user decisions required. Safe refactor with no behavior change.

### Plan
1. Add `src/headends/completions-usage.ts` with `collectLlmUsage()` returning `{ input, output, total }`.
2. Replace the duplicated reduce logic in both headends with the helper.
3. Add a unit test for the helper.
4. Run `npm run lint`, `npm run build`, `npm run test:phase1`.

### Implied Decisions
- Preserve OpenAI headend’s `prompt/completion` naming by mapping from shared totals.
- Preserve Anthropic headend’s `input/output` naming.

### Testing requirements
- Unit test for `collectLlmUsage`.
- Required checks: `npm run lint`, `npm run build`, `npm run test:phase1`.

### Documentation updates required
- None. Refactor only.

### Findings (append-only)
- Duplicated LLM usage aggregation in completions headends:
  - `src/headends/openai-completions-headend.ts:81-91`
  - `src/headends/anthropic-completions-headend.ts:77-88`

---

## Iteration 4 - Consolidate completions headend log entry builder

### TL;DR
- Move the duplicated `buildLog` function into a shared helper for completions headends.
- Keep the remote identifier per headend to preserve log identity.

### Analysis (facts only)
- `buildLog` is duplicated with identical structure in:
  - `src/headends/openai-completions-headend.ts:52-73`
  - `src/headends/anthropic-completions-headend.ts:52-73`
- Only the `remoteIdentifier` value differs (`headend:openai-completions` vs `headend:anthropic-completions`).
- This is a clear single‑source‑of‑truth violation for log construction.

### Decisions
- No user decisions required. Safe refactor with no behavior change.

### Plan
1. Add `src/headends/completions-log.ts` with `buildCompletionsLogEntry()` helper.
2. Update both headends’ `buildLog` wrappers to call the helper with their identifier.
3. Run `npm run lint`, `npm run build`, `npm run test:phase1`.

### Implied Decisions
- Preserve log formatting and log metadata fields unchanged.
- Keep per-headend remote identifiers as-is.

### Testing requirements
- No new tests required (pure refactor), but still run `npm run lint`, `npm run build`, `npm run test:phase1`.

### Documentation updates required
- None. Refactor only.

### Findings (append-only)
- Duplicated completions headend log entry builder:
  - `src/headends/openai-completions-headend.ts:52-73`
  - `src/headends/anthropic-completions-headend.ts:52-73`

---

## Iteration 5 - Consolidate completions socket cleanup

### TL;DR
- Extract shared socket shutdown logic into a helper used by both completions headends.
- Add a small unit test to lock the helper behavior.

### Analysis (facts only)
- Identical `closeActiveSockets` implementations exist in both headends:
  - `src/headends/openai-completions-headend.ts:1132-1136`
  - `src/headends/anthropic-completions-headend.ts:1013-1017`
- The method logic is purely mechanical: `socket.end()` then `socket.destroy()` after a timeout.
- This duplication risks future divergence if the shutdown behavior needs to change.

### Decisions
- No user decisions required. Safe refactor with no behavior change.

### Plan
1. Add `src/headends/socket-utils.ts` with `closeSockets(sockets, force)` helper.
2. Replace both `closeActiveSockets` bodies to delegate to the helper.
3. Add a unit test for `closeSockets` using fake timers.
4. Run `npm run lint`, `npm run build`, `npm run test:phase1`.

### Implied Decisions
- Preserve timing (0ms when forced, 1000ms otherwise) and error swallowing behavior.
- Keep public behavior unchanged.

### Testing requirements
- Unit test: `closeSockets` helper.
- Required checks: `npm run lint`, `npm run build`, `npm run test:phase1`.

### Documentation updates required
- None. Refactor only.

### Findings (append-only)
- Duplicated socket shutdown logic in completions headends:
  - `src/headends/openai-completions-headend.ts:1132-1136`
  - `src/headends/anthropic-completions-headend.ts:1013-1017`

---

## Iteration 6 - Reuse socket shutdown helper in REST + MCP headends

### TL;DR
- Replace duplicated socket shutdown logic in REST and MCP headends with the shared `closeSockets` helper.
- Keep per-headend behavior unchanged (MCP still clears its socket sets).

### Analysis (facts only)
- REST headend has an inline socket shutdown helper:
  - `src/headends/rest-headend.ts:431-439`
- MCP headend has its own `closeSockets` method with the same logic:
  - `src/headends/mcp-headend.ts:522-531`
- Both match the shared implementation already used by completions headends.

### Decisions
- No user decisions required. Safe refactor with no behavior change.

### Plan
1. Import `closeSockets` in REST and MCP headends.
2. Replace the inline socket shutdown logic with the helper.
3. Keep MCP’s `set.clear()` behavior after calling the helper.
4. Run `npm run lint`, `npm run build`, `npm run test:phase1`.

### Implied Decisions
- Preserve the 0ms vs 1000ms destroy timing and error swallowing.
- Preserve MCP’s socket set clearing semantics.

### Testing requirements
- Existing `socket-utils` unit test covers the helper; still run `npm run lint`, `npm run build`, `npm run test:phase1`.

### Documentation updates required
- None. Refactor only.

### Findings (append-only)
- Duplicated socket shutdown logic in REST/MCP headends:
  - `src/headends/rest-headend.ts:431-439`
  - `src/headends/mcp-headend.ts:522-531`

---

## Iteration 7 - Consolidate completions agent resolution

### TL;DR
- Extract the identical `resolveAgent` logic from OpenAI and Anthropic completions headends into a shared helper.
- Add a focused unit test for the helper.

### Analysis (facts only)
- `resolveAgent` implementations are identical in both completions headends:
  - `src/headends/openai-completions-headend.ts:986-996`
  - `src/headends/anthropic-completions-headend.ts:951-961`
- Logic flow:
  - Refresh model map.
  - Check direct model ID mapping.
  - Fallback to registry alias resolution.

### Decisions
- No user decisions required. Safe refactor with no behavior change.

### Plan
1. Add `src/headends/completions-agent-resolution.ts` with `resolveCompletionsAgent()`.
2. Replace both headends’ `resolveAgent` bodies to use the helper.
3. Add a unit test under `src/tests/unit/`.
4. Run `npm run lint`, `npm run build`, `npm run test:phase1`.

### Implied Decisions
- Keep refresh timing and resolution order unchanged.

### Testing requirements
- Unit test: shared `resolveCompletionsAgent` helper.
- Required checks: `npm run lint`, `npm run build`, `npm run test:phase1`.

### Documentation updates required
- None. Refactor only.

### Findings (append-only)
- Duplicated `resolveAgent` logic in completions headends:
  - `src/headends/openai-completions-headend.ts:986-996`
  - `src/headends/anthropic-completions-headend.ts:951-961`

---

## Iteration 8 - Consolidate shutdown signal handling for headends

### TL;DR
- Extract shared shutdown handling logic used by OpenAI/Anthropic/REST headends into one helper.
- Add a unit test for the helper.

### Analysis (facts only)
- Identical `handleShutdownSignal` logic exists in three headends:
  - `src/headends/openai-completions-headend.ts:1126-1131`
  - `src/headends/anthropic-completions-headend.ts:1007-1012`
  - `src/headends/rest-headend.ts:424-429`
- Each method sets `globalStopRef.stopping = true` (if present) and then closes sockets.

### Decisions
- No user decisions required. Safe refactor with no behavior change.

### Plan
1. Add `src/headends/shutdown-utils.ts` with a helper to set stopRef and close sockets.
2. Replace the three `handleShutdownSignal` bodies to call the helper.
3. Add a unit test for the helper.
4. Run `npm run lint`, `npm run build`, `npm run test:phase1`.

### Implied Decisions
- Preserve exact shutdown behavior and call ordering.

### Testing requirements
- Unit test for shutdown helper.
- Required checks: `npm run lint`, `npm run build`, `npm run test:phase1`.

### Documentation updates required
- None. Refactor only.

### Findings (append-only)
- Duplicated shutdown signal handling in OpenAI/Anthropic/REST headends:
  - `src/headends/openai-completions-headend.ts:1126-1131`
  - `src/headends/anthropic-completions-headend.ts:1007-1012`
  - `src/headends/rest-headend.ts:424-429`

---

## Iteration 9 - Consolidate model ID generation for completions headends

### TL;DR
- Extract the shared model ID generation logic from OpenAI and Anthropic completions headends into a helper.
- Keep separator differences (`-` vs `_`) as explicit parameters.

### Analysis (facts only)
- Both headends generate model IDs with the same algorithm (pick a base label, strip `.ai`, de-dup with suffix):
  - `src/headends/openai-completions-headend.ts:1058-1079`
  - `src/headends/anthropic-completions-headend.ts:1034-1061`
- Only behavioral difference: OpenAI uses `-` while Anthropic uses `_` for de-dup suffixes.

### Decisions
- No user decisions required. Safe refactor with no behavior change.

### Plan
1. Add `src/headends/model-id-utils.ts` with `buildHeadendModelId(sources, seen, separator)`.
2. Replace both headend `buildModelId` bodies to call the helper with the correct separator.
3. Add a unit test covering base selection and de-dup behavior for both separators.
4. Run `npm run lint`, `npm run build`, `npm run test:phase1`.

### Implied Decisions
- Preserve current source preference order and suffix separator per headend.

### Testing requirements
- Unit test for model ID helper.
- Required checks: `npm run lint`, `npm run build`, `npm run test:phase1`.

### Documentation updates required
- None. Refactor only.

### Findings (append-only)
- Duplicated model ID generation in completions headends:
  - `src/headends/openai-completions-headend.ts:1058-1079`
  - `src/headends/anthropic-completions-headend.ts:1034-1061`

---

## Iteration 10 - Consolidate completions model map refresh

### TL;DR
- Extract the shared `refreshModelMap` logic into a helper used by OpenAI and Anthropic completions headends.
- Add a unit test to lock the mapping behavior.

### Analysis (facts only)
- Both completions headends implement identical `refreshModelMap` logic:
  - `src/headends/openai-completions-headend.ts:1055-1061`
  - `src/headends/anthropic-completions-headend.ts:1025-1031`
- Each clears the map, builds a `seen` set, iterates registry agents, and fills the map with `buildModelId(meta, seen)`.

### Decisions
- No user decisions required. Safe refactor with no behavior change.

### Plan
1. Add `src/headends/model-map-utils.ts` with `refreshModelIdMap(registry, map, buildModelId)`.
2. Replace both headends’ `refreshModelMap` bodies to call the helper.
3. Add a unit test under `src/tests/unit/`.
4. Run `npm run lint`, `npm run build`, `npm run test:phase1`.

### Implied Decisions
- Preserve the same `seen`-based de-dup semantics.

### Testing requirements
- Unit test for `refreshModelIdMap`.
- Required checks: `npm run lint`, `npm run build`, `npm run test:phase1`.

### Documentation updates required
- None. Refactor only.

### Findings (append-only)
- Duplicated `refreshModelMap` logic in completions headends:
  - `src/headends/openai-completions-headend.ts:1055-1061`
  - `src/headends/anthropic-completions-headend.ts:1025-1031`
