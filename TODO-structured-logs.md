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

## Decisions (2025-11-13)
- All non-fatal LLM failures remain `WRN`; requirement updated accordingly per Costa.
- MCP restart-in-progress logs (probe failures, attempt notices) must be downgraded to `WRN`, while restart failures stay `ERR`.

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
