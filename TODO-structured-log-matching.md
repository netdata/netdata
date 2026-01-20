# TODO: Structured Log Matching in Tests

## TL;DR
- Convert all test assertions that match log **message strings** (substring/regex) into **structured log matches**.
- Use existing structured logging fields (`LogEntry.details`, `remoteIdentifier`, `severity`, etc.) and extend them where missing.
- Update tests to rely on deterministic fields instead of message text.

## Analysis
- Existing structured logging foundation:
  - `LogEntry.details` exists for structured fields and is part of the API. Evidence: `src/types.ts:115-156`.
  - `buildStructuredLogEvent` converts `LogEntry.details` into labels for structured output. Evidence: `src/logging/structured-log-event.ts:75-120`.
  - Message ID registry is currently minimal and keyed only by `remoteIdentifier`. Evidence: `src/logging/message-ids.ts:5-23`.
- Current tests still match on **message substrings** (non-deterministic):
  - `expectLogIncludes` helper uses `entry.message.includes(...)` for log assertions. Evidence: `src/tests/phase2-harness-scenarios/phase2-runner.ts:516-523`.
  - Unknown tool / auto-correct log matches use message substrings. Evidence: `src/tests/phase2-harness-scenarios/phase2-runner.ts:1587-1597`.
  - Orchestrator collapse log uses message substring. Evidence: `src/tests/phase2-harness-scenarios/phase2-runner.ts:10385-10388`.
  - XML transport test matches `logWarning` message. Evidence: `src/tests/unit/xml-transport.spec.ts:160-176`.
  - Journald sink test matches output string content. Evidence: `src/tests/unit/journald-sink.test.ts:181-184`.
- Log sites that **lack** structured `details` (tests currently rely on message text):
  - Unknown tool warning in tool executor has no `details`. Evidence: `src/session-tool-executor.ts:756-769`.
  - Auto-corrected tool name warning has no `details`. Evidence: `src/session-tool-executor.ts:273-293`.
  - Orchestrator “collapse remaining turns” warning has no `details`. Evidence: `src/session-turn-runner.ts:370-399`.
  - Rate-limit/backoff warnings have no `details`. Evidence: `src/session-turn-runner.ts:1710-1733`.
  - Sanitizer/extraction warnings have no `details`. Evidence: `src/session-turn-runner.ts:1096-1108`.
- Log sites that **already** provide structured details (tests can migrate without code changes):
  - Context guard warnings include `projected_tokens`, `limit_tokens`, `provider`, `model`. Evidence: `src/session-turn-runner.ts:500-533` and `680-724`.
  - Final turn entry includes `final_turn_reason`, `original_max_turns`, `active_max_turns`. Evidence: `src/session-turn-runner.ts:2107-2150`.
  - Tool output store/drop/skip logs include structured `details` already (`reason`, `tool`, `bytes`, etc.). Evidence: `src/tools/tools.ts:520-620` and `900-1105`.
- Sub-agent tool names exposed to the model use the `agent__` namespace:
  - `SubAgentRegistry.getTools()` returns names like `agent__${toolName}`. Evidence: `src/subagent-registry.ts:131-142`.
  - `SubAgentRegistry.listToolNames()` also returns `agent__...`. Evidence: `src/subagent-registry.ts:157-159`.

## Decisions
1. **Log event identifier strategy** (needed to replace message-substring checks)
   - **A)** Add `details.event` (string) to each log entry used in tests; keep `LogEntry` shape unchanged.
     - Pros: no API break; integrates with existing `details` → labels; minimal code churn.
     - Cons: not type-enforced; relies on constants to avoid typos.
   - **B)** Add a new top-level `LogEntry.event` field (typed union), update emitters + StructuredLogEvent.
     - Pros: strong typing; clear contract.
     - Cons: API change; broad refactor; more risk.
   - **Recommendation:** A (lowest risk, aligns with current `details` usage).
   - **Decision:** A (Costa).

2. **XML transport `logWarning` callbacks** (unit tests currently match message text)
   - **A)** Extend callbacks to accept structured fields (`event`, `details`) or a `LogEntry`-like object.
     - Pros: consistent with structured logging; tests can match deterministically.
     - Cons: interface change for callers.
   - **B)** Keep callback shape, and update tests to assert on `recordTurnFailure` slugs only (drop log message check).
     - Pros: minimal code change.
     - Cons: loses explicit log assertion coverage.
   - **Recommendation:** A if we want full structured coverage; B if we want minimal change.
   - **Decision:** A (Costa).

3. **Formatting-only log tests (journald/rich-format)** — are these in scope?
   - **A)** Keep string asserts for formatter output (treat as formatting tests, not log-logic tests).
     - Pros: minimal; keeps coverage of output format.
     - Cons: still string-based.
   - **B)** Parse output into structured key/value assertions or test structured event builders instead of output strings.
     - Pros: no string matching; deterministic.
     - Cons: more work; may require logfmt parser or refactor.
   - **Recommendation:** A unless you want *zero* string matches across the repo.
   - **Decision:** A (Costa).

4. **Where to define event codes and detail keys**
   - **A)** Centralize in `src/logging/log-events.ts` (constants used by both code + tests).
     - Pros: single source of truth; less drift.
     - Cons: new file + imports.
   - **B)** Inline strings at call sites/tests.
     - Pros: minimal upfront changes.
     - Cons: drift risk; harder to refactor.
   - **Recommendation:** A for long-term stability.
   - **Decision:** A (Costa).

5. **Sub-agent tool naming mismatch between logs and accounting** (logs show `subagent__...`, accounting shows `agent__...`)
   - **A)** Keep as-is (logs use composed tool identity, accounting keeps requested tool name).
     - Pros: preserves current external API expectations; lower risk.
     - Cons: inconsistency across telemetry channels; tests must account for mismatch.
   - **B)** Normalize logs to use `agent__...` for sub-agents (match accounting).
     - Pros: consistent naming across logs/accounting; simpler consumers.
     - Cons: changes existing log semantics; potential breaking change for log-based tooling.
   - **C)** Normalize accounting to use `subagent__...` (match logs).
     - Pros: consistent naming with tool identity; clearer provider namespace.
     - Cons: accounting format change; potential breaking change for consumers.
   - **Recommendation:** B (model-facing tool name is the source of truth; align logs with it).
   - **Decision:** B (Costa).

6. **Sub-agent provider namespace (must match model-facing tool name)**  
   - **Decision:** Namespace must be `agent` because the model sees `agent__...` (Costa).
   - **Impact:** This removes `subagent` as a provider namespace and requires an alternate way to identify sub-agent calls for session-op tracking.

7. **How to identify sub-agent calls after namespace becomes `agent`** (needed for session ops & child opTree stubs)
   - **A)** Detect sub-agent provider by `instanceof AgentProvider`.
     - Pros: minimal API change; explicit to provider class.
     - Cons: runtime class check; adds import dependency in `tools.ts`.
   - **B)** Add `isSubAgentProvider: boolean` on `ToolProvider` base class and set it in `AgentProvider`.
     - Pros: explicit contract; no `instanceof`.
     - Cons: touches all providers; API change.
   - **C)** Heuristic by tool name (`agent__*` except known internal tools).
     - Pros: no API change.
     - Cons: brittle; breaks if we add new internal tools.
   - **Recommendation:** A (lowest churn; still explicit).
   - **Decision:** A (Costa).

## Plan
1. Inventory all log-message string matches in tests and map each to its log-emitting site.
2. Define stable event codes + required detail keys per log category (unknown tool, auto-correct, collapse, rate limit, sanitizer, SDK payload, etc.).
3. Add `details.event` + structured fields to the missing log sites (and keep message text for humans).
4. Create test helpers (e.g., `expectLogEvent`, `expectLogDetail`) and replace substring/regex matches.
5. If chosen, refactor XML transport callback and formatting tests (journald/rich-format).
6. Run `npm run lint`, `npm run build`, `npm run test:phase1`, `npm run test:phase2` (deterministic harness).

## Implied decisions
- Preserve existing human-readable `message` text for runtime logs.
- Prefer structured details + event codes for deterministic test matching.
- Keep log severity/type/remoteIdentifier semantics unchanged.

## Testing requirements
- `npm run lint`
- `npm run build`
- `npm run test:phase1`
- `npm run test:phase2`

## Documentation updates required
- If we add new log event codes or extend structured logging contract, update `docs/specs/AI-AGENT-INTERNAL-API.md` (LogEntry / log details) and any logging-related docs.
