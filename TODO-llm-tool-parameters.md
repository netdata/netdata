TL;DR
- Agent log `agent/sanitizer` dropped the `agent__batch` call because assistant `tool_calls[0].function.arguments` arrived as a JSON string and we never converted it into an object before sanitization.
- The only string→object shim lives inside `AIAgentSession.parseJsonRecord`, so providers that surface AI SDK `tool-call` parts (or legacy `tool_calls`) still hand raw strings to downstream code.
- Need a shared JSON-object parser and to run it inside `BaseLLMProvider.parseAISDKMessage` so every tool call (content-embedded or legacy) yields real parameter records before the sanitizer runs.
- Restored run-test-122 in its original form (stringified nested payload) and added run-test-123 to cover the enhanced parsing path without weakening existing coverage.
- 2025-11-14 @ 23:52 UTC: live run against `nova:default-model` still triggered `agent/sanitizer` dropping the `agent__batch` call (`parameters not object`). Latest LLM response logged via TRC (call id `517c763f-f857-41cd-8c15-7bcb5b25174b`) shows `function.arguments` populated with a JSON string even after our earlier fixes, so parsing remains incomplete somewhere in the pipeline.
- Implemented lossy parsing heuristics (strip trailing ellipses/log truncations, close dangling braces) so `parseJsonRecord` can recover structured payloads when models emit incomplete JSON.
- Added `run-test-124` to Phase 1 harness to assert that truncated `agent__batch` payloads still execute successfully once parsing repairs them, and `run-test-131` to confirm ERR logs fire when providers send malformed tool parameters.
- New requirement (2025-11-15): every time we "tolerate" malformed parameters we must **keep the behavior** but also emit an `ERR` log that includes the raw bad data; without this visibility we silently replace "wrong data" with "empty data" and debugging becomes impossible.

Analysis
- Warning came from `AIAgentSession.sanitizeTurnMessages` (`src/ai-agent.ts:3430-3505`). It only accepts tool calls whose `parameters` are records, otherwise it logs `parameters not object` and drops them.
- That method already tries to repair string payloads via `parseJsonRecord`, but the helper is private to `AIAgentSession`. Consumers outside that class (e.g., providers) cannot reuse it.
- Looking at `BaseLLMProvider.parseAISDKMessage` (`src/llm-providers/base.ts:2057-2135`):
  - When AI SDK content parts include `type: 'tool-call'`, we blindly set `parameters: (part.input ?? {}) as Record<string, unknown>`.
  - When falling back to legacy `message.toolCalls`, we set `parameters: (tc.arguments ?? tc.function?.arguments ?? {}) as Record<string, unknown>`.
  - In both cases, if the LLM returned a JSON string (the common path for OpenAI-compatible responses), we never parse it. We simply cast a string to a record. The sanitizer later attempts to parse it, but any invalid JSON (e.g., embedded escape sequences from long prompts) causes the drop we saw in production.
- There is no shared helper outside `AIAgentSession`, so we also cannot normalize strings earlier in the pipeline or when we adopt tool calls elsewhere (`parseNestedCallsFromFinalReport`, `ToolsOrchestrator`, etc.).
- Deterministic test LLM currently emits tool calls with `input: JSON.stringify(...)`, so the gap is not covered by Phase 1 harness. We need coverage where provider messages include literal strings so we catch regressions.
- **New logging mandate:** every place we currently "tolerate" malformed/missing parameters (provider conversion, sanitizer, nested final report calls, `agent__batch`) must emit an `ERR` log that shows the offending data (truncated preview is fine). Behavior stays the same (let it pass) but it must be observable without enabling HTTP trace captures.

Decisions
- ✅ Promote `parseJsonRecord` into `src/utils.ts` so every caller (session sanitizer, providers, batch helpers) reuses the same logic instead of duplicating fragile parsing.
- ✅ Normalize nested `agent__batch` `calls[].parameters` (and similar structured payloads) via the shared helper so deeply nested JSON strings are accepted instead of silently dropped.
- 🚧 Instrument all tolerance points to log malformed payloads with severity `ERR` while still letting execution continue: provider conversion, sanitizer drops, final-report nested calls, and `agent__batch` parsing. Include truncated raw data in each log entry.

Plan
1. ✅ Extract the current `parseJsonRecord` implementation from `AIAgentSession` into a shared helper (`src/utils.ts`) and replace the private method with imports to keep behavior identical.
2. ✅ Update `BaseLLMProvider.parseAISDKMessage` to run every `part.input` / `tc.arguments` / `tc.function?.arguments` value through the shared parser so `ConversationMessage.toolCalls` always hold records.
3. ✅ Audit other spots that assume `parameters` is already a record (e.g., progress hooks, nested-call expansions, `agent__batch`, nested final report parsing) and switch them to the helper if they currently cast blindly.
4. ✅ Extend Phase 1 deterministic harness with a scenario (`run-test-122`) where the LLM emits tool calls whose payloads are JSON strings (including nested `agent__batch` calls) so we confirm the parser works end-to-end.
5. ✅ Add lossy parsing + coverage (`run-test-124`) for truncated JSON blobs so the sanitizer can still recover `agent__batch` payloads that end with logging ellipses / missing braces.
6. ✅ Emit `ERR` logs (with raw previews) for every tolerance case: provider conversion, sanitizer drop, nested final report parsing, `agent__batch` normalization. Ensure the log shows the offending data so debugging does not require HTTP traces.
7. ✅ Run `npm run lint`, `npm run build`, and `npm run test:phase1`. (Full `npm run lint` now passes after prior lint cleanup.)

Implied Decisions
- No headend changes planned; fix is confined to the provider conversion and shared helpers.
- We assume string payloads should be parsed eagerly; binary blobs or non-JSON payloads will continue to be dropped by the sanitizer.
- Tolerance behavior stays (we keep running the tool), but every repair/degenerate case must emit an `ERR` log with a truncated raw preview, so operators always see the malformed data.

Testing Requirements
- `npm run lint`
- `npm run build` ✅
- `npm run test:phase1` ✅ (entire deterministic suite including the new `run-test-122`).

Documentation Updates
- None anticipated unless we touch runtime behavior in a way that affects docs/AI-AGENT-GUIDE.md; confirm after implementation.
