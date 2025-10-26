# ai-agent Logging Guide

This document describes how ai-agent produces logs, the transport options in play, and every field that can appear in emitted records. The guidance reflects the current implementation in `src/logging/` and the log emitters throughout the codebase.

## Logging Pipeline Overview

- **Sources** – Operational code (LLM/tool orchestration, headends, CLI) emits `LogEntry` objects defined in `src/types.ts`.
- **Structured conversion** – `StructuredLogger` (see `src/logging/structured-logger.ts`) converts each `LogEntry` into a `StructuredLogEvent` via `buildStructuredLogEvent`.
- **Sinks** – Depending on runtime context, the logger writes events to:
  - logfmt text on stderr (CLI default and fallback when journald is unavailable).
  - systemd journald via a single shared `systemd-cat-native` helper (Journal Export Format over stdin) when detection succeeds and the helper is available.
  - newline-delimited JSON on stderr when `telemetry.logging.formats` (or CLI `--telemetry-log-format`) selects `json`.
  - OTLP logs (gRPC) when telemetry is enabled and `telemetry.logging.extra` (or CLI `--telemetry-log-extra`) includes `otlp`; this runs in parallel with the local sink.
- **Telemetry labels** – When telemetry is initialised, `getTelemetryLabels()` injects `telemetry.labels` plus runtime metadata (e.g., `mode`, `headend`) into every structured event.

## `LogEntry` Source Schema

Every log originates as a `LogEntry`. The table below lists the fields (all values are strings unless noted).

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `timestamp` | number (ms since epoch) | ✓ | Capture time recorded by the emitter. |
| `severity` | `'VRB'  'WRN'  'ERR'  'TRC'  'THK'  'FIN'` | ✓ | Severity code used across sinks (see levels below). |
| `turn` | number | ✓ | Sequential turn index within the agent session. |
| `subturn` | number | ✓ | Sequential tool invocation index inside the turn. |
| `direction` | `'request'  'response'` | ✓ | Indicates if the log describes outbound or inbound work. |
| `type` | `'llm'  'tool'` | ✓ | Whether the log concerns a model call or a tool operation. |
| `remoteIdentifier` | string | ✓ | Identifier of the upstream component (`provider:model` for LLMs, `server:tool` for tools). |
| `fatal` | boolean | ✓ | True when the event caused the session to stop. |
| `message` | string | ✓ | Human-readable summary stored in sinks. |
| `path` | string | ✗ | Hierarchical op-tree path (e.g., `1.2`). |
| `toolKind` | `'mcp'  'rest'  'agent'  'command'` | ✗ | Enriches tool logs with a precise classification. |
| `headendId` | string | ✗ | Identifies the emitting headend (e.g., `cli`, `api:8080`). |
| `agentId` | string | ✗ | Agent identifier used for correlation. |
| `callPath` | string | ✗ | Opaque path representing the call stack within the session tree. |
| `txnId` | string | ✗ | Correlation id for the current operation. |
| `parentTxnId` | string | ✗ | Parent correlation id. |
| `originTxnId` | string | ✗ | Root correlation id for the conversational trace. |
| `bold` | boolean | ✗ | Hint for TTY renderers to emphasise the message. |
| `max_turns` | number | ✗ | Planning metadata for UI surfaces. |
| `max_subturns` | number | ✗ | Planned tool calls within the current turn. |

### Severity Levels

- `ERR` – Error conditions, mapped to journald priority 3.
- `WRN` – Warnings, mapped to priority 4.
- `FIN` – End-of-run summary information, priority 5.
- `VRB`, `THK` – Verbose or thank-you chatter, priority 6. `THK` entries are suppressed from TTY output.
- `TRC` – Trace output gated by trace flags, priority 7.

### Filtering Behaviour

- CLI logging (`makeTTYLogCallbacks`) suppresses `VRB` unless `--verbose` is set, and drops `TRC` unless the relevant `--trace-llm`/`--trace-mcp` flag is enabled.
- `THK` entries are never emitted to the TTY sink.

## Structured Event Schema

`buildStructuredLogEvent` produces the intermediate representation below (see `src/logging/structured-log-event.ts`). Each property maps to sink-specific key names; journald keys obey uppercase ASCII semantics, while logfmt/JSON keys retain lowercase snake_case.

| Structured property | Journald key | Logfmt/JSON key | Notes |
| --- | --- | --- | --- |
| `isoTimestamp` | `AI_TIMESTAMP` | `ts` | ISO8601 timestamp derived from `timestamp` (the numeric timestamp is not emitted directly). |
| `severity` | `AI_SEVERITY` | `level` | Journald keeps the uppercase severity code; logfmt/json lower-case it. |
| `priority` | `PRIORITY` | `priority` | Numeric priority mapped from severity (`ERR`→3, `WRN`→4, `FIN`→5, `VRB/THK`→6, `TRC`→7). |
| `message` | `MESSAGE` | `message` | Original message; journald escapes newlines as `\n`. |
| `messageId` | `MESSAGE_ID` | `message_id` | Optional UUID resolved by `resolveMessageId`. |
| `type` | `AI_TYPE` | `type` | Log type (`llm` or `tool`). |
| `direction` | `AI_DIRECTION` | `direction` | `request` vs `response`. |
| `turn` | `AI_TURN` | `turn` | Sequential turn index (stringified). |
| `subturn` | `AI_SUBTURN` | `subturn` | Sequential tool index within the turn. |
| `toolKind` | `AI_TOOL_KIND` | `tool_kind` | Emitted when defined. |
| `toolProvider` | `AI_TOOL_PROVIDER` | `tool_provider` | Present for tool events; mirrors `provider` when the entry type is `tool`. |
| `tool` | `AI_TOOL` | `tool` | Tool name for tool events (also mirrored in `model` for compatibility). |
| `headendId` | `AI_HEADEND` | `headend` | Headend identifier such as `cli` or `api:8080`. |
| `agentId` | `AI_AGENT` | `agent` | Agent identifier for correlation. |
| `callPath` | `AI_CALL_PATH` | `call_path` | Hierarchical path in the session tree. |
| `txnId` | `AI_TXN_ID` | `txn_id` | Current transaction identifier. |
| `parentTxnId` | `AI_PARENT_TXN_ID` | `parent_txn_id` | Parent transaction identifier. |
| `originTxnId` | `AI_ORIGIN_TXN_ID` | `origin_txn_id` | Root transaction identifier. |
| `remoteIdentifier` | `AI_REMOTE` | `remote` | `provider:model` (LLM) or `server:tool` (tool). |
| `provider` | `AI_PROVIDER` | `provider` | Derived from `remoteIdentifier` when available. |
| `model` | `AI_MODEL` | `model` | Parsed model/tool name. |
| `labels` | `AI_LABEL_<UPPERCASE>` | individual `key=value` entries | Journald uppercases and prefixes each label, logfmt/json keep original lowercase keys. |

### Label Aggregation Rules

- Base labels seeded from the `LogEntry`: `agent`, `call_path`, `severity`, `type`, `direction`, `turn`, `subturn`, optional `tool_kind`, `headend`, parsed `provider`, and `model`. Tool events also receive `tool_provider` and `tool` labels that mirror the dedicated fields.
- Global labels passed to `StructuredLogger` (typically telemetry labels such as `mode`, `environment`, or per-headend `headend`) are merged in.
- Custom labels supplied at the telemetry layer (configuration `telemetry.labels`) appear verbatim. Label values must be non-empty strings.
- Entry-level `details` (attached via `LogEntry.details`) are surfaced as additional labels, so use stable snake_case keys when recording structured metrics like `input_tokens` or `latency_ms`.

### Implied Context Categories

Structured fields are inherited from the runtime component that emits a log. Each category contributes implied keys and removes the need to restate that context in the human-readable message.

- **Headend context** – `mode`, `headend`, and ingress metadata (Slack team/channel, API request ids) supplied by the active headend or session manager.
- **Agent/session context** – `agent`, `call_path`, `turn`, `subturn`, reasoning metadata, and transaction ids maintained by `AIAgentSession`.
- **LLM context** – `provider`, `model`, token/cache counters, latency, cost, and stop reasons gathered during the LLM turn.
- **Tool context** – `tool`, `tool_kind`, tool provider, byte counters, latency, and error classification gathered by the tool orchestrator.

Messages should focus on incremental information; implied fields live in the structured metadata and are automatically populated from the owning runtime object.

## Sink Behaviour and Field Sets

### Journald Sink

- Detection: `isJournaldAvailable()` checks for `process.env.JOURNAL_STREAM` and ensures `/run/systemd/journal/socket` exists. When true, servers/headends prefer journald; otherwise they fall back to logfmt.
- Transport: the shared journald sink writes newline-separated `KEY=value` pairs in a single datagram. Keys conform to journald semantics: built-ins (`MESSAGE`, `PRIORITY`, `SYSLOG_IDENTIFIER`, `MESSAGE_ID`) remain uppercase, while custom metadata uses uppercase ASCII with the `AI_` prefix (`AI_SEVERITY`, `AI_TOOL_KIND`, `AI_TOOL_PROVIDER`, `AI_TOOL`, `AI_LABEL_<KEY>`, etc.). Only defined values are emitted; absent fields are omitted.
- Resilience: if the helper exits, the sink restarts it once; if that restart attempt fails (spawn error or the replacement helper immediately terminates), the logger disables journald output and promotes logfmt on stderr so operators continue to receive logs.
- Stack traces: warnings and errors capture the originating Node.js stack (`AI_STACKTRACE`). Stack traces are withheld from other sinks to keep CLI/JSON output concise.
- Example entry (journald key/value framing):

```
MESSAGE=Tool response ok (agent__final_report)
PRIORITY=6
SYSLOG_IDENTIFIER=ai-agent
AI_SEVERITY=VRB
AI_TIMESTAMP=2025-10-24T21:53:31.201Z
AI_HEADEND=cli
AI_AGENT=web-fetch
AI_CALL_PATH=web-fetch
AI_REMOTE=agent:tool
AI_TOOL_KIND=agent
AI_LABEL_result_chars=11
AI_LABEL_latency_ms=42
```

### Logfmt Sink

- Renderer: `formatLogfmt` produces a single line per event with `key=value` pairs separated by spaces. Quoting follows logfmt conventions (space, equals, or quote in value triggers quoting with `"` escaping).
- Mandatory keys: `ts`, `level`, `priority`, `type`, `direction`, `turn`, `subturn`, `message` (rendered last to keep the free-form text easy to spot).
- Optional keys: `message_id`, `remote`, `tool_kind`, `tool_provider`, `tool`, `headend`, `agent`, `call_path`, `txn_id`, `parent_txn_id`, `origin_txn_id`, `provider`, `model`, plus every label (lowercase, unprefixed).
- Colour: When `color` is true (TTY default), `ERR`/`WRN`/`FIN`/`VRB`/`THK`/`TRC` lines receive ANSI colours.
- Writers: CLI sink writes to `stderr`; server headends inject their logfmt output through per-headend writers or fallback to `stderr` if no custom writer exists. When the journald sink disables itself, the structured logger auto-registers the logfmt writer so messages keep flowing without operator action.

## Message ID Registry

`src/logging/message-ids.ts` maps well-known `remoteIdentifier` values to stable journald `MESSAGE_ID` UUIDs. Current registrations:

| Identifier | UUID |
| --- | --- |
| `agent:init` | `8f8c1c67-7f2f-4a63-a632-0dbdfdd41d39` |
| `agent:fin` | `4cb03727-5d3a-45d7-9b85-4e6940d3d2c4` |
| `agent:pricing` | `f53bb387-032c-4b79-9b0f-4c6f1ad20a41` |
| `agent:limits` | `50a01c4a-86b2-4d1d-ae13-3d0b4504b597` |
| `agent:EXIT-FINAL-ANSWER` | `6f3db0fb-a931-47cb-b060-9f4881ae9b14` |
| `agent:batch` | `c9a43c6d-fd32-4687-9a72-a673a4c3c303` |
| `agent:progress` | `b8e3466f-ef86-4337-8a4b-09d5f5e0be1f` |

Unregistered identifiers do not emit `MESSAGE_ID`. Tools that require new identifiers must register them here to guarantee stability.

## Runtime Configuration Touchpoints

- **Telemetry labels:** `telemetry.labels` in configuration and CLI overrides propagate directly into log labels via the telemetry runtime (`src/telemetry/index.ts`). Headends extend the map with their own identifier (for example, REST headends add `headend=api:<port>`).
- **CLI verbosity:** `--verbose` enables `VRB` logs on the TTY; `--trace-llm` and `--trace-mcp` surface `TRC` logs for LLMs or tools respectively.
- **Telemetry disable:** Setting `AI_TELEMETRY_DISABLE=1` bypasses telemetry initialisation; loggers still emit but without additional telemetry labels.

## Extending Logging

When adding new log-producing code:

- Populate the relevant `LogEntry` fields rather than inlining strings. Doing so guarantees that sinks and telemetry capture the metadata automatically.
- Keep label keys lowercase with snake_case. The journald sink uppercases keys, while logfmt preserves the original case.
- Register any new stable `remoteIdentifier` values in `message-ids.ts` and document them in this file (update both tables).
- Ensure tests that assert on log output account for filtering (trace/verbose flags) and the default sink selection rules described above.
- **JSON Sink**
  - Renderer: serialises a structured payload mirroring the logfmt keys plus `timestamp` into a single-line JSON object per event, with `message` placed last for readability when streaming.
  - Selection: opt-in via `telemetry.logging.formats`/`--telemetry-log-format json`; defaults follow the journald/logfmt auto-detection described above.
  - Writers: defaults to `stderr`; can be overridden by passing a custom `jsonWriter` to `createStructuredLogger`.

### OTLP Log Exporter (Optional)

- Activation: enable telemetry and add `otlp` to `telemetry.logging.extra` (or `--telemetry-log-extra otlp`). The exporter uses `telemetry.logging.otlp.endpoint`/`timeoutMs` when provided, otherwise falls back to the global OTLP settings.
- Behaviour: batches log records via OpenTelemetry’s `BatchLogRecordProcessor`. CLI mode applies drop-on-failure semantics (errors are logged and the batch is discarded); server mode retries per OpenTelemetry defaults.
- Payload: emits `body` with the original message, severity number/text mapped from ai-agent severity codes, and attributes equivalent to the JSON payload (labels appear under `label.<key>` attributes).

### Default Selection Rules

- When no explicit format is supplied, ai-agent picks `journald` if systemd journald is detected (matching `JOURNAL_STREAM`) and `systemd-cat-native` launches successfully; otherwise it falls back to `logfmt`.
- Setting `telemetry.logging.formats` (config) or `--telemetry-log-format` (CLI) overrides the default. Accepted values: `journald`, `logfmt`, `json`, `none` (to suppress local output). The first valid entry wins; if an unavailable format (e.g., `journald` without systemd) is chosen, ai-agent automatically falls back to `logfmt` to keep logs flowing.
- Additional sinks such as OTLP are layered on top of the default via `telemetry.logging.extra`/`--telemetry-log-extra`.
