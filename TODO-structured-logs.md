# TODO - Structured Logs Inventory

## TL;DR
- Identify every structured logging field emitted by ai-agent and document their sources/values.
- Catalog all registered journald `MESSAGE_ID`s (UUIDs) and explain when each fires.
- Deliver findings plus any gaps/risks before proposing schema changes.

- Core schema lives in `src/types.ts` (`LogEntry`, payload capture types) and `src/logging/structured-log-event.ts` (derived fields + reserved label filtering). These are the authoritative definitions of every structured log field we emit.
- Sinks (journald/logfmt/json/console) under `src/logging/` translate the structured event into transport-specific key names; journald adds `AI_*` prefixes while logfmt/json keep lowercase snake_case per `docs/LOGS.md`.
- `resolveMessageId` (`src/logging/message-ids.ts`) currently registers only seven `agent:*` identifiers; all other `remoteIdentifier`s emit no `MESSAGE_ID`, explaining missing IDs in MCP/tool/queue logs.
- Emitters: `src/ai-agent.ts` contributes context-guard and tool-accounting labels; `src/llm-client.ts` adds latency/token/cost fields; `src/tools/tools.ts` adds request/response previews, queue stats, truncation metadata, etc. These determine actual values, so reviewing them prevents guessing.
- Field/value snapshot (Nov 12 2025):
  * **Base LogEntry fields** – required values (`timestamp`, `severity`, `turn`, `subturn`, `direction`, `type`, `remoteIdentifier`, `fatal`, `message`) come directly from orchestrator/tool runtimes; optional routing metadata (`headendId`, `agentId`, `callPath`, txn IDs, `agentPath`, `turnPath`) are populated when upstream context exists; otherwise they’re omitted.
  * **Structured-only fields** – `isoTimestamp`, numeric `priority`, `messageId` (when registered), parsed `provider`/`model`/`tool`/`toolNamespace`, filtered `labels` map, plus captured payloads (`llm/tool request/response`).
  * **Per-sink keys** – journald adds uppercase `AI_*` variants and `AI_LABEL_<KEY>` copies; logfmt/json surface the structured keys verbatim; console sink just prettifies the same event.
- Observation: label keys that collide with the reserved set are dropped silently; we need to codify expected names to avoid losing telemetry.

## Decisions Needed
1. Do we only report existing fields or recommend additions (e.g., journald `MESSAGE_ID`s for MCP tools)?
2. Should telemetry-provided labels (from config) be included in "fields" list or treated separately?
3. Confirm whether "LLM response received" should ever annotate non-LLM events or if enricher filters must tighten.

## Decisions (2025-11-13)
- All non-fatal LLM failures remain `WRN`; requirement updated accordingly per Costa.
- MCP restart-in-progress logs (probe failures, attempt notices) must be downgraded to `WRN`, while restart failures stay `ERR`.

## Decisions (2025-11-14)
- Every failed LLM response must log with severity `WRN`.
- Every failed session (including sub-agents) must log severity `ERR` once failure synthesis runs.
- Every tool failure originating from JSON-RPC errors must log severity `WRN`.
- Audit "LLM response received" text usage; correct any sink/enricher conditions that append it to unrelated logs.

## Plan
1. Trace logging pipeline: enumerate `LogEntry` properties, derived structured fields, and sink-specific key names.
2. Review emitters under `src/` to see which optional fields get populated (agent metadata, txn ids, payload captures, etc.).
3. Extract `MESSAGE_ID` registry, confirm actual usage by searching for corresponding `remoteIdentifier` strings.
4. Produce report summarizing findings, highlighting actual values/examples, unknowns, and risks.
5. Align MCP restart logs with severity policy (WRN for in-progress, ERR for failures) and verify associated tests/telemetry.

## Analysis (2025-11-13)
- **Session failure severity** – `src/ai-agent.ts:1321-1334` logs `remoteIdentifier: 'agent:error'` with `severity: 'ERR'` and `fatal: true` whenever the session throws, so overall session crashes emit ERR as required.
- **Tool failure severity** – `src/tools/tools.ts:640-690` routes every orchestrated failure through `errLog` where `severity: 'ERR'` and `type: 'tool'`, ensuring downstream sinks see ERR for tool errors.
- **LLM failure severity gap** – `src/llm-client.ts:630-659` maps failures to `severity: fatal ? 'ERR' : 'WRN'`, meaning only `auth_error` and `quota_exceeded` statuses surface as ERR while rate limits, bad requests, etc., stay WRN; this does not meet the “all LLM failures are ERR” expectation.
- **MCP restart logging updated** – `src/tools/mcp-provider.ts:174-243` now emits WRN when a restart sequence is initiated or an attempt runs, aligning in-progress logs with the WRN requirement, while true failures remain ERR.
- **MCP restart failure severity** – The same block (and `restartServer` callers) log true restart failures with `severity: 'ERR'`, so that portion of the requirement already holds.

### Risks / Gaps
- Need confirmation whether non-fatal LLM failures should ever escalate from WRN to ERR or if the current split is permanently intentional.

## Analysis (2025-11-14)
- **LLM failure severity mismatch** – `src/llm-client.ts:640-705` flags `auth_error` and `quota_exceeded` results as `fatal`, emitting `severity: 'ERR'` while all other failure types remain `WRN`. Requirement 1 calls for every failed LLM response to log as `WRN`, so the current fatal split conflicts with the directive.
- **Session failure log severity** – Synthetic exits such as `EXIT-FINAL-ANSWER`, `EXIT-TOKEN-LIMIT`, `EXIT-NO-LLM-RESPONSE`, etc., all flow through `logExit` (`src/ai-agent.ts:800-840`), which hardcodes `severity: 'VRB'` even though `fatal: true`. Only uncaught exceptions log an explicit `ERR` (`src/ai-agent.ts:1320-1345`). Requirement 2 (failed sessions ERR) is not met for controlled failure paths, including sub-agent sessions because they share the same `AIAgentSession` implementation.
- **Tool failure severity** – Tool orchestration emits `severity: 'ERR'` for every JSON-RPC/tool execution failure via `errLog` (`src/tools/tools.ts:640-702`). Requirement 3 wants these to be `WRN`, so behavior diverges today.
- **"LLM response received" contamination** – Console/journal formatting builds that context string for *any* event with `type === 'llm' && direction === 'response'` (`src/logging/rich-format.ts:116-154`), so agent lifecycle logs (`agent:init`, `agent:fin`, subagent summaries, etc.) inherit the phrase despite not being actual model responses. This matches Costa's observation that the text shows up on unrelated entries.

## Progress (2025-11-14)
- `logExit` now derives severity from the `fatal` flag (`src/ai-agent.ts:840-870`) and success exits pass `{ fatal: false }`, so every synthesized failure (including sub-agents) emits `ERR` while real completions remain non-fatal.
- JSON-RPC/tool failures log `WRN` instead of `ERR` (`src/tools/tools.ts:640-700`), matching the “tool failures are warnings” directive without changing telemetry payloads.
- `buildContext` restricts the `LLM response received` prefix to actual `LLM response received`/`LLM response failed` messages (`src/logging/rich-format.ts:101-155`), preventing agent lifecycle logs from inheriting the misleading context string.
- Phase 1 harness enforces the new severities/formatter behaviour (scenarios `run-test-2`, `run-test-6`, `run-test-101`, plus formatter parity checks in `src/tests/phase1-harness.ts:500-8200`).
- Updated docs (`docs/LOGS.md`, `docs/AI-AGENT-GUIDE.md`, `docs/DESIGN.md`) now describe the severity split and the constrained LLM response context.

## Implied Decisions
- Any new `MESSAGE_ID`s or schema changes need user approval post-report.
- MCP restart severity adjustments are now in place; further schema or logging alterations still require design approval.

## Testing Requirements
- None (analysis task). If future changes alter logging behavior, will need `npm run lint` + `npm run build` + targeted scenario logs.

## Documentation Updates Needed
- Likely update `docs/LOGS.md` (and potentially `docs/AI-AGENT-GUIDE.md`) once analysis reveals gaps; for now report first.


---

# Costa Requirements

1. agent:init → 8f8c1c67-7f2f-4a63-a632-0dbdfdd41d39 (startup logs).

agent:init must provide complete details on how this session was initialited. This include full description of the headend that triggered it, the system and user prompts, even the slack user, channel, etc.
