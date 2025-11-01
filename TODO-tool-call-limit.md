# TL;DR
- Phase 1 (final-turn enforcement + deterministic coverage) is complete: the final-turn instruction flows through the retry queue, remains ephemeral, and tests confirm only `agent__final_report` stays available.
- Phase 2 has LLM payload logging in place (logs, journald, snapshots). Tool payload capture and cross-provider integration remain pending.
- Outstanding items focus on tooling payload capture and expanding snapshot coverage for tool requests/responses; journald label duplication is already resolved.

# Analysis
- LLM logging now attaches the raw HTTP/SSE bodies to every request/response log. Structured sinks (logfmt/json/journald) expose them, and session snapshots persist base64 copies.
- Tool execution logging (`tools.ts`) still records previews (`preview`, `ok preview: …`) and emits full parameter/result bodies only when `traceTools` is on. Error logs surface message strings but omit the serialized payload that triggered the error.
- `SessionTree` now merges payload descriptors instead of overwriting them; LLM entries carry both summary metadata and the base64-encoded raw payload. Tools still lack raw payload coverage.
- `truncateUtf8WithNotice` continues to shorten tool outputs before returning them to the LLM; until tool payload capture lands, we still lose the original raw response for post-mortems.

# Decisions
## Confirmed
1. Remote identifiers follow the `protocol:namespace:tool` pattern; namespaces replace provider IDs in logs.
2. Rich formatter owns agent context; sources never prefix messages themselves.
3. Hierarchical identifiers stay `<severity> <turnPath> <direction> <kind> <agentPath>` with tool name appended to `callPath`.
4. Batch tools allocate unique subturns so logs stay deterministic.
5. Final-turn instruction must be injected via the retry queue only; saved conversation history remains untouched.
6. All log payloads and snapshots must retain full bodies—no truncation, no redaction.

## Pending
1. Decide whether to retire `toolResponseMaxBytes` entirely or keep it for LLM safety while still logging the full raw payload.

# Plan
## Phase 1 – Restore Limits & Tests ✅
1. Enforce per-turn max tool calls and restrict to `agent__final_report` once exceeded.
2. Inject the mandated final-turn instruction via retry queue so it remains ephemeral.
3. Add deterministic coverage for both per-turn and max-turn limits.

## Phase 2 – Logging & Payload Capture (current focus)
4. ✅ Extend `LogEntry`/`StructuredLogEvent` and sink emitters so request/response payloads travel end-to-end (LLM complete; tool fields reserved).
5. ✅ Update `LLMClient.logRequest`/`logResponse` to serialize the exact HTTP/SSE payloads and attach them to the new fields.
6. Update tool execution logging to capture full parameters/results (including batch members) before any truncation or normalization and attach them to log payload fields.
7. ✅ Store raw LLM payloads in opTree snapshots (base64 `{ format, encoding, value }`); extend the same mechanism to tools once available.
8. Deterministic coverage: LLM payload fields asserted in coverage scenarios; add tool coverage alongside tool payload capture.

## Phase 3 – Snapshot Retention & Final QA (upcoming)
9. Update docs (`docs/LOGS.md`, `docs/IMPLEMENTATION.md`, `docs/DESIGN.md`) to describe payload availability and troubleshooting workflow.
10. Full lint/build regression plus manual sanity check for CLI/journald parity.

# Follow-ups
- Monitor OTLP/logfmt consumers after payload fields land to catch schema adjustments early.
- Reassess `toolResponseMaxBytes` once full logging is live to ensure model safety vs. debugging needs.

# Testing Requirements
- `npm run lint`
- `npm run build`
- Phase 1 harness scenarios (limits) plus new deterministic checks for payload logging.

# Documentation Updates
- Update logging and troubleshooting docs once payload capture is implemented.
