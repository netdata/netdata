# Unified Real‑Time Session Tree (Everything in One Tracking System)

Owner: ai-agent core
Status: Implemented (migration complete)
Scope: Entire application (CLI/library/server/Slack), all providers, all tool kinds, sub‑agents

Goal

- Produce a single, canonical, real‑time hierarchical tree for every session that contains EVERYTHING: conversation messages, tool requests/responses, arguments, logs, accounting, traces, sub‑agents. This must update live as events occur (including during sub‑agent execution), and downstream integrations (Slack/web/API) can consume it as the one source of truth.

Non‑Goals

- No UI over-design. A viewer page is out of scope for now; Slack footer may link to it when added later.
- Aggressive/redaction policies beyond basic header redaction are deferred.

Design Choice (One Tracking System)

- Make `SessionTreeBuilder`/`SessionNode` the single canonical tracker (the “opTree”).
- Migration complete: `ExecutionTree` has been removed; opTree is the only source of truth everywhere (CLI/Slack/API).

References (Files and Roles)

- Agent orchestration: `src/ai-agent.ts`
- Single turn LLM client: `src/llm-client.ts`
- Tool orchestration: `src/tools/tools.ts` (ToolsOrchestrator)
- Tool providers: MCP `src/tools/mcp-provider.ts`, REST `src/tools/rest-provider.ts`, Internal `src/tools/internal-provider.ts`, Sub‑agent adapter `src/tools/agent-provider.ts`
- Sub‑agent execution: `src/subagent-registry.ts`
- Hierarchical tree (target): `src/session-tree.ts` (SessionTreeBuilder/SessionNode)
- Legacy tracking (adapter): removed; do not reintroduce `ExecutionTree`.
- Public server / Slack: `src/server/{index.ts,api.ts,session-manager.ts,slack.ts,status-aggregator.ts}`
- Core types: `src/types.ts`
- Utilities (format/cap): `src/utils.ts`

Current Data Flows (Implemented)

1) LLM turn
   - `AIAgentSession.executeAgentLoop()` calls `LLMClient.executeTurn()`.
   - `LLMClient` logs request/response via `onLog` callback (vrb/trc/wrn/err) and records last routed provider, costs (OpenRouter), and cache token splits. These logs are routed through `AIAgentSession.addLog()` → `ExecutionTree.recordLog()`.
   - LLM accounting is generated in `ai-agent.ts` per attempt (ok/failed) and appended to both arrays and `ExecutionTree.recordAccounting()`.
   - opTree: an LLM op is begun via `opTree.beginOp(turn,'llm', {provider,model})` and later ended with status; logs, reasoning, request/response, and accounting are attached under this op.

2) Tool execution
   - `ToolsOrchestrator.executeWithManagement()` resolves provider/kind, logs compact request/response, optional TRC with full args/raw payload, enforces timeouts/size caps, records tool accounting. All these are appended to opTree under the tool op, and also logged to ExecutionTree.
   - For sub‑agents (`provider.id === 'subagent'` + `kind:'agent'`), a tool op is treated as a `session` op in opTree and the child’s opTree is streamed and attached live during child execution (and finalized on completion).

3) Sub‑agent execution
   - `SubAgentRegistry.execute()` launches a child session via a preloaded snapshot from `agent-loader`. It passes prefixed `onLog/onOutput/onThinking/onAccounting` to reuse parent sinks (for visibility). Child returns `conversation`, `accounting`, and `opTree` at the end, which the parent attaches to the parent op node.

4) Real‑time refresh
   - `AIAgentSession` invokes `callbacks.onOpTree(tree)` when updating opTree (turn begin/end, op end in some places).
   - `SessionManager` listens and stores `opTrees` per run, emits `onTreeUpdate(runId)` to live renderers (Slack).
   - Child opTree snapshots during sub‑agent execution are forwarded live and reflected in the parent session tree.

5) Slack footer/stats
   - `src/server/slack.ts` composes a footer using totals (from accounting + optional opTree) and posts it. It can read `opTree` at finish and during updates for counts.

Key Shape (Target): SessionNode

- `SessionNode`={ id, traceId, agentId, callPath, startedAt, endedAt, success, error, attributes, turns: TurnNode[] }
- `TurnNode`={ id, index, startedAt, endedAt, attributes, ops: OperationNode[] }
- `OperationNode`={ opId, kind: 'llm'|'tool'|'session', startedAt, endedAt, status, attributes, logs: LogEntry[], accounting: AccountingEntry[], childSession?: SessionNode }
- We will enrich:
  - TurnNode.attributes: include system/user prompts used; assistant text for the turn (or pointers).
- OperationNode.logs: include all LLM/tool logs (VRB/WRN/ERR/TRC/THK/FIN as applicable) pertaining to that op.
- THK policy: thinking is rendered from `OperationNode.reasoning.chunks`. THK lines printed to console must include `[txn:<originTxnId>]` and a stable `[path:<label>]` like all logs; do not use a placeholder txn.
- OperationNode.accounting: include LLM/tool accounting events that belong to that op/attempt.
- session/childSession: for sub‑agents, stream the child opTree into the parent op live.

Trace Context (As‑Is)

- `AIAgentSession` assigns `{ txnId, originTxnId, parentTxnId, callPath }` and injects into LLM logs/accounting via `addLog(...)`. Tools logs/accounting do not currently embed all trace fields; opTree context still unambiguously links them.

Open Gaps to Close

1) Attach LLM logs to the current LLM op in opTree (not only to ExecutionTree).
   - Where: `LLMClient.log` → wraps to `onLog` → `AIAgentSession.recordExternalLog`.
   - How: introduce a scoped “current LLM op id” in `AIAgentSession` per attempt/turn and mirror logs to `opTree.appendLog(opId, entry)` as they flow.

2) Persist conversation messages in opTree turns.
   - Capture the last assistant message content and (optionally) tool call scaffolding per successful attempt; add to `TurnNode.attributes`.
   - Include user/system text for the turn (shortened to safe size) to provide end‑to‑end context in the tree.

3) Stream child trees into parent live.
   - Extend `ToolExecuteOptions` with `onChildOpTree?: (t: SessionNode) => void`.
   - In `ToolsOrchestrator.executeWithManagement` when provider is `subagent`, pass an `onChildOpTree` that updates the parent opTree: `attachChildSession(opId, t)` and triggers `onOpTreeSnapshot`.
   - In `AgentProvider.execute`, forward the callback to `SubAgentRegistry.execute`.
   - In `SubAgentRegistry.execute`, when launching `loaded.run(...)`, pass `callbacks.onOpTree` so every child update is bubbled up immediately to the parent.

4) Optionally enrich tool logs/accounting entries with trace fields.
   - Keep opTree as the canonical structure; but we can also add `{ agentId, callPath, txnId, parentTxnId, originTxnId }` to tool logs/accounting in ToolsOrchestrator for self‑describing arrays.

5) Endpoints/UI (Optional but recommended)
   - Add `GET /api/runs/:id/tree` → returns `{ tree: SessionNode, logs, accounting, conversation, childConversations }` for inspection.
   - Add `/runs/:id/view` → a minimal HTML/JS tree viewer with expand/collapse.
   - Configure `publicBaseUrl` so Slack footer can link to the viewer.

Security/Privacy and Size Handling

- Secrets redaction: When trace is enabled, LLM and tool logs may contain headers and tokens. Add a redaction pass in the server API/viewer layer for common header keys (Authorization, x-api-key, etc.).
- Size caps: Already present for tool responses (truncate with prefix). Keep TRC logs on by config; ensure they obey the central cap.
- Slack footer remains compact and only shows counts/totals.

Detailed Impact Map (Where we will touch)

1) `src/ai-agent.ts`
   - Begin LLM op: `opTree.beginOp(currentTurn, 'llm', { provider, model, isFinalTurn })` (already done at ~844 but verify coverage across attempts).
   - Track `llmOpId` for the active attempt; forward every LLM log to `opTree.appendLog(llmOpId, entry)` by bridging `recordExternalLog`.
   - On attempt completion, append the LLM accounting entry to `opTree.appendAccounting(llmOpId, entry)` (already partially done; verify in all branches including errors and timeouts).
   - On turn end: `opTree.endTurn(currentTurn)` and include a snapshot call `callbacks.onOpTree(opTree.getSession())` (already present; ensure after all op updates).
   - Conversation into opTree: after a successful turn attempt, store assistant text (and optionally `toolCalls`) into `TurnNode.attributes` with size guard.

2) `src/llm-client.ts`
   - No behavior change; continue emitting logs through `onLog` (VRB/WRN/ERR/TRC/THK). The session will mirror these into the current LLM op.

3) `src/tools/tools.ts` (ToolsOrchestrator)
   - Add `onChildOpTree` callback plumbed from caller. On `subagent` provider, pass through to AgentProvider.
   - When a child live update arrives, call `opTree.attachChildSession(opId, t)` and `onOpTreeSnapshot(opTree.getSession())`.
   - Confirm all existing tool logs (request/response/warn/error/trace) and accounting are appended via `opTree.appendLog/opTree.appendAccounting` and `endOp` with attributes (latency, size).

4) `src/tools/agent-provider.ts`
   - Extend `execute()` to accept `onChildOpTree` via `ToolExecuteOptions` (extras or a new param) and forward it through the `execFn`.

5) `src/subagent-registry.ts`
   - `execFn` should accept a second param `opts?: { onChildOpTree?: (t: SessionNode) => void }`.
   - When calling `loaded.run(...)`, provide `callbacks.onOpTree = (t) => opts.onChildOpTree?.(t as SessionNode)` so each child snapshot is bubbled to parent.

6) `src/server/session-manager.ts`
   - Already stores `opTree` snapshots and emits `onTreeUpdate` to live renderers. No change beyond ensuring updates are frequent enough.

7) `src/server/index.ts` / `src/server/api.ts`
   - Add `GET /api/runs/:id/tree` to return the live opTree (with optional redaction).
   - Optional `/runs/:id/view` static page for expand/collapse.

8) `src/server/slack.ts`
   - Append a viewer link in the final footer and possibly in progress updates.
   - No change to posting logic; it already refreshes via `onTreeUpdate`.

Validation Plan (No Surprises)

1) Unit smoke via manual runs
   - Run a session with tools (MCP + REST + sub‑agent) and verify:
     - opTree emits turn begin/end, LLM op begin/end per attempt, tool ops with request/response logs, accounting entries, and child session embedding.
     - Live updates fire during sub‑agent execution (add an artificial sleep in a sub‑agent tool to visually confirm).

2) Shape assertions
   - Verify `SessionNode` always has:
     - Non‑empty `turns[]`, each with `ops[]`.
     - Each op has `logs[]` and `accounting[]` arrays (possibly empty but present) and status when ended.
     - Child sessions attach in `kind:'session'` ops and evolve live.

3) Backward compatibility
   - Keep `ExecutionTree` emissions as-is so existing consumers remain functional. Mark opTree as preferred for new consumers in docs.

4) Size/Privacy guards
   - Confirm TRC logs are capped; confirm redaction in `/api/runs/:id/tree` for Authorization/x-api-key/… headers.

Risks & Mitigations

- Double counting accounting: Parent should NOT re-inject child accounting in parent tree; we will only attach the child session’s own accounting under the child session subtree. Parent keeps only parent tool accounting for the sub‑agent call itself.
- Performance of frequent snapshots: Gate live snapshot frequency (e.g., throttle to ~250–500ms) if needed; but first wire direct streaming for correctness, then tune.
- Viewer exposure: Guard `/api/runs/:id/tree` with bearer auth if exposed beyond localhost; redact secrets.

Step-by-Step Execution Plan (When approved)

1) Wire LLM logs into opTree
   - Track current `llmOpId` in `ai-agent.ts`; mirror `recordExternalLog` to `opTree.appendLog` when type === 'llm' and op is active.
   - Ensure all LLM accounting paths append to opTree.

2) Store conversation snippets in TurnNode
   - On successful attempt, attach assistant text and toolCall scaffolding into `TurnNode.attributes` with a byte cap (e.g., 8–16KB per turn).

3) Live child opTree streaming
   - Extend types: `ToolExecuteOptions` add `onChildOpTree?`.
   - Orchestrator → AgentProvider → SubAgentRegistry → child session callbacks path; on each child `onOpTree` snapshot, update parent opTree and call `onOpTreeSnapshot`.

4) Endpoints (optional)
   - Implement `GET /api/runs/:id/tree`.
   - Add `/runs/:id/view` with a simple tree view.
   - Add `publicBaseUrl` to config and Slack link in footer.

5) Docs
   - Update docs/AI-AGENT-INTERNAL-API.md to describe opTree as canonical and live. Document trace fields.
Open Questions

- Viewer UI and `/runs/:id/view` are deferred until after stability.

Appendix: What We Reviewed (no surprises)

- `src/ai-agent.ts` — turn loop, addLog(), accounting, FIN summaries, sub‑agent integration, opTree begin/end turn, begin LLM op, tool executor, planning subturns.
- `src/llm-client.ts` — traced fetch, routed provider/model and cost extraction, request/response VRB/TRC logs, tokens enrichment.
- `src/tools/tools.ts` — request/response/trace logging, size caps, accounting, spans, opTree append, child tree attach (at finish), concurrency.
- `src/tools/mcp-provider.ts` — transports init, list tools, callTool, request/response normalization.
- `src/tools/rest-provider.ts` — REST execution, optional streaming combine, return tokens string, TRC handled by orchestrator.
- `src/tools/internal-provider.ts` — append_notes/title/final_report/batch; Slack Block Kit validation; final report saved to session.
- `src/tools/agent-provider.ts` — bridges orchestrator to SubAgentRegistry.
- `src/subagent-registry.ts` — child runner execution, prefixed logs, returns conversation/accounting/opTree after completion (to be extended for live updates).
- `src/execution-tree.ts` — current source of truth for arrays + spans; will be adapter once opTree becomes canonical.
- `src/session-tree.ts` — the target canonical structure; supports logs/accounting embedding and child sessions.
- `src/server/session-manager.ts` — stores result/logs/accounting/opTree; publishes `onTreeUpdate` to live renderers.
- `src/server/slack.ts` — live updates and final footer; already reads `opTree` for counts; will link to viewer.
- `src/server/index.ts` / `api.ts` — existing minimal API; ready to add `/api/runs/:id/tree`.
- `src/utils.ts` — compact tool request formatter; UTF‑8 truncation with notice; helpful for consistent previews.

Ingress Metadata Schema (SessionNode root attributes)

We will standardize a minimal, privacy‑aware metadata block at the SessionNode root to capture how/where a run started. This block is set once at run start and does not change.

Common fields (all sources)
- `source`: `'cli' | 'slack' | 'api' | 'web' | 'sub-agent'`
- `runId`: string (from SessionManager for server‑initiated runs; for CLI, a locally generated UUID)
- `startedAt`: number (epoch ms)
- `agentId`: string (prompt path or logical id; already present on SessionNode)
- `traceId`: string (same as `SessionNode.traceId` for consistency)

Slack-specific (when `source === 'slack'`)
- `slack`: {
  `teamId`: string,
  `channelId`: string,
  `channelName?`: string,
  `threadTs?`: string,           // thread timestamp
  `messageTs?`: string,          // original message ts
  `userId?`: string,
  `userLabel?`: string,          // derived via users.info (display/real/email)
  `ingress`: 'mention' | 'dm' | 'channel-post' | 'auto-engage',
  `routingRule?`: string         // identifier of matched routing rule (if any)
}

CLI-specific (when `source === 'cli'`)
- `cli`: {
  `cwd?`: string,
  `argv0?`: string,
  `pid?`: number
}

API-specific (when `source === 'api'`)
- `api`: {
  `route`: string,               // e.g., '/api/ask'
  `requestId?`: string,
  `ip?`: string,                 // redacted if configured
  `auth`: 'bearer' | 'none',
}

Web-specific (when `source === 'web'`)
- `web`: {
  `sessionId?`: string,
  `ip?`: string,                 // redacted if configured
  `userId?`: string
}

Sub‑agent (when `source === 'sub-agent'`)
- `subAgent`: {
  `parentTraceId`: string,
  `parentCallPath`: string,
  `toolName`: string
}

Notes
- Values are populated by `SessionManager` at `startRun` (server) or `AIAgentSession.create` (CLI) and passed into `SessionTreeBuilder` as `attributes` on construction.
- Development policy: store everything we have (full channel names, user labels, etc.) in the saved session files to accelerate debugging.
- Sensitive fields (IP, headers, bearer tokens) must be redacted in any public API response or viewer — apply a redaction pass in the new `/api/runs/:id/tree` route.

Redaction Policy (for trace logs and ingress)
- Remove/replace values for headers keys: `authorization`, `x-api-key`, `x-openai-api-key`, `x-slack-signature`, etc.
- Mask IPs and requestIds when `config.api.redactSensitive === true`.
- Enforce size caps on trace bodies; never store multi‑MB payloads in opTree.

Persistence Plan (Save/Load Sessions via opTree)

Principle
- Always persist sessions. opTree is the single persisted artifact for session state. A saved session contains the master SessionNode and its entire subtree of sub‑agents (child SessionNodes), plus minimal run metadata.

Save (end of run and sub‑agent finish; no manual save option)
- Serialize a versioned payload:
  - `version`: number (e.g., 1)
  - `session`: SessionNode (canonical opTree, including all turns/ops/logs/accounting/childSession subtrees)
  - `meta`: {
      `result`: selected fields from `AIAgentResult` (finalReport, success/error),
      `ingress`: SessionNode root attributes (source identifiers),
      `createdAt`: epoch ms
    }
  - Optional: derived flat `logs[]` and `accounting[]` for compatibility (computed once from opTree at save time)
- Store as gzipped JSON in the configured local store. Apply redaction only when explicitly configured.

Load (resume and inspection)
- Deserialize payload and mount `session` back into `SessionManager` as a read‑only run snapshot:
  - Expose via `/api/runs/:id/tree` and the viewer; disable live updates since it is static.
  - Derived arrays (`logs[]`, `accounting[]`) can be regenerated from `session` if not stored.
- CLI supports `--resume <originTxnId>` to load a saved session, append a new user prompt, and continue execution as a new run. Resume is not a replay; streaming isn’t reconstructed.

Live Snapshotting (optional, phase 2)
- For very long sessions, periodically snapshot `SessionNode` (e.g., every 1–5s) to allow near‑real‑time recovery on crash. Keep only latest N snapshots or a rolling window.

Guarantee
- Because sub‑agent sessions are embedded as `childSession` subtrees at their parent tool op(s), a saved opTree always includes the complete hierarchy of the master session and all sub‑agents.

Always‑On Session Saving (file per session)

- Policy: Every session is saved automatically at end‑of‑run and after each sub‑agent finishes (and optionally on crash/stop) to a target directory as a single gzipped JSON file containing the entire opTree (unfiltered by default), plus minimal meta.
- Filename: the master session’s `originTxnId` (root transaction id); e.g., `<originTxnId>.json.gz`.
- Location: configurable directory.
  - Config: `persistence.sessionsDir` (string directory path)
  - CLI: `--sessions-dir <dir>` overrides config when provided.
- Contents: the versioned payload described above (full `SessionNode` including all sub‑agents, logs, accounting, and ingress metadata). No filtering/redaction on disk unless a `redactedSave` flag is explicitly set.
- Crash safety: if the process terminates unexpectedly, we may optionally write a last snapshot if available; otherwise the session may remain unsaved (phase 2 snapshotting mitigates this).

Global Billing Accounting Export (separate aggregated file)

- Purpose: A consolidated ledger of accounting events across all sessions for billing/chargeback.
- Source of truth: derived from the saved opTree (or the in‑memory opTree at run end) — no separate runtime tracker.
- Format: JSONL file, one line per flattened accounting record, with a stable schema:
  - Common: `timestamp`, `status`, `latency`, `type: 'llm'|'tool'`, `originTxnId`, `agentId`, `callPath`
  - LLM: `provider`, `model`, `actualProvider?`, `actualModel?`, `tokens { inputTokens, outputTokens, cacheReadInputTokens?, cacheWriteInputTokens?, totalTokens }`, `costUsd?`, `upstreamInferenceCostUsd?`, `error?`
  - Tool: `mcpServer|providerLabel`, `command`, `charactersIn`, `charactersOut`, `error?`
- File: configurable path.
  - Config: `persistence.billingFile` (string file path)
  - CLI: `--billing-file <file>` overrides config when provided.
- Rotation: optional size/time‑based rotation to be defined (phase 2); at minimum, append‑only with periodic external rotation.
- Emission time: at session end (success/failure/stop), flatten the opTree’s accounting (including all child sessions) and append to the JSONL file.

CLI Controls

- Remove legacy manual save/export flags. There is no manual save.
  - Removed: `--save`, `--save-all`, `--accounting`, `--save-session`.
- Add `--resume <originTxnId>`: load a saved session and continue by appending a new user prompt.
- Add path overrides for persistence:
  - `--sessions-dir <dir>` sets the sessions directory for this run.
  - `--billing-file <file>` sets the accounting JSONL file path for this run.

Persistence Migration (Replace Old Save/Load)

Existing behaviors to replace:
- CLI `--save` and `--save-all` currently persist conversation arrays (master and children) as JSON files.
- Accounting JSONL writing via `config.accounting.file` or `--accounting` persists flat accounting events.
- Types still expose `saveConversation`/`loadConversation` options (legacy idea).

Target behaviors:
- A single save artifact: opTree JSON (plus small `meta`) that captures the entire session hierarchy, all logs/accounting, and ingress data.
- Deprecated features:
  - Deprecate `--save`, `--save-all`, and `--accounting` in favor of `--save-session <file>` (writes opTree JSON) and an optional `--export-compat` to also emit derived arrays if needed.
  - Remove `saveConversation/loadConversation` from types after migration or re-purpose to `saveSession/loadSession` that operate on opTree.

Steps:
1) Add CLI flag `--save-session <file>` and write the versioned opTree payload (see Save above).
2) When `--accounting`/`config.accounting.file` are set, write a warning and suggest `--save-session` (keep temporarily during Phase A).
3) When `--save`/`--save-all` are set, write a warning and suggest `--save-session` (keep temporarily during Phase A).
4) Update docs (SPECS/README/AI-AGENT-INTERNAL-API) to make opTree the only persistence path.
5) Phase B: remove JSONL and conversation JSON saving paths; retain an `opTree → arrays` exporter if some external tooling requires it.

Config and Locations (persistence)

- Single base directory for AI Agent: `${HOME}/.ai-agent/`
  - Configuration files live here (as today), and all persistence also lives under this base to avoid scattering files.

- Hardcoded subpaths (defaults; only the base directory is configurable):
  - Sessions directory: `${HOME}/.ai-agent/sessions/` (mandatory per‑session saves)
  - Global billing accounting: `${HOME}/.ai-agent/accounting.jsonl`
  - Optional gzip: `${HOME}/.ai-agent/sessions/<originTxnId>.json.gz` when enabled

- Minimal config additions:
  - `config.persistence.baseDir?`: default `${HOME}/.ai-agent` (override only if absolutely necessary)
  - `config.persistence.gzipSessions?`: boolean to enable gzip for session files (default: false)
  - `config.persistence.redactedSave?`: boolean; when true, redact sensitive fields in saved opTree (default: false)

- Behavior:
  - On startup, ensure `${HOME}/.ai-agent/` exists.
  - On first save, ensure `${HOME}/.ai-agent/sessions/` exists.
  - Writes are atomic (temp file + rename) to avoid partial corruption.





Comprehensive Use‑Case Coverage (What opTree must capture)

- Conversation messages
  - System and user prompts used per turn (post‑variable expansion; with byte caps for storage)
  - Assistant content per successful attempt (the message that yielded tool calls or final_report)
  - Optional: streamed “thinking” text (THK) — controlled by a flag; otherwise, preserve only THK headers in logs

- LLM requests and responses
  - Request: provider+model, message count/size, final‑turn flag
  - Response (success): token usage (prompt/output/cache read/write), latency, response size, cost (computed or provider‑reported), final answer flag
  - Response (failures): auth/quota/network/model/timeout/invalid; latency, mapped message
  - Streaming: THK headers and content chunks through `onChunk` with type `'thinking'`
  - Routing: OpenRouter actual provider/model + upstream cost when applicable

- Tool requests and responses (all kinds)
  - MCP tools: compact request line, optional TRC with full JSON args; response size, truncation notices, raw payload trace (when enabled), latency, errors
  - REST tools: same as MCP (providerId 'rest'); includes both manifest and OpenAPI‑generated endpoints
  - Internal tools: `agent__append_notes`, `agent__final_report`, `agent__batch` (request args and results; batch orchestrates inner calls via orchestrator → inner ops appear in opTree)
  - Sub‑agents (AgentProvider): treat as `kind:'session'` ops; attach child SessionNode and stream child updates live

- Accounting
  - LLM per attempt: tokens (prompt/output/cacheR/cacheW/total), cost (computed/provider), latency, status (ok/failed)
  - Tool per call: characters in/out (args/result), latency, status (ok/failed)

- Logs
  - VRB/WRN/ERR/TRC/THK/FIN across LLM and tools; compact and trace logs (request/response, HTTP/SSE summaries) with size caps
  - All logs linked to the correct op (LLM or specific tool op)

- Traces/IDs
  - agentId, callPath, txnId, parentTxnId, originTxnId recorded on session and optionally in entries for self‑describing arrays

All tool kinds covered at runtime:
- MCP (stdio/http/sse/websocket) via MCPProvider
- REST via RestProvider (manifest + OpenAPI generated)
- Internal via InternalToolProvider
- Sub‑agents via AgentProvider + SubAgentRegistry (child SessionNode)

Migration Strategy: Replace and Eliminate Previous Tracking

- Phase A (dual‑write, switch consumers)
  - Continue feeding `ExecutionTree`, but update Slack/status/CLI to rely on opTree (use `buildSnapshotFromOpTree`).
  - Provide a deterministic flattener to derive `logs[]` and `accounting[]` from opTree for `AIAgentResult` during transition.

- Phase B (hard switch)
  - Remove direct writes to `ExecutionTree`; route all events into opTree and fan out via `onOpTree` only.
  - If `AIAgentResult.logs/accounting` remain required, populate them once from opTree at run end (no dual sources).

- Phase C (cleanup)
  - Delete `execution-tree.ts` and references after confirming no consumers.
  - Update docs to state opTree is canonical.

Event Linking Under Concurrency and Interleaving

- Turn lifecycle
  - On turn begin: `beginTurn(index)`; then `llmOpId = beginOp(index,'llm', { provider, model, isFinalTurn })`.
  - Map all LLM logs and LLM accounting to `llmOpId` and end it with status/latency when attempt completes.

- Tools executed while LLM is running
  - AI SDK invokes tools mid‑turn. Each call creates its own `opId = beginOp(index,'tool', { name, providerId, kind })`.
  - All tool logs/accounting append under that `opId`. Parallel calls simply produce multiple ops under the same turn with distinct timestamps; viewers sort by `startedAt`.
  - Internal `agent__batch` triggers inner tool ops via orchestrator; those ops will appear as siblings in the same turn.

- Sub‑agent streaming
  - Parent creates a `kind:'session'` op and passes `onChildOpTree` through Orchestrator→AgentProvider→SubAgentRegistry. Child `onOpTree` snapshots attach to the parent op’s `childSession` live.

- Deterministic anchors
  - LLM: `llmOpId` per attempt.
  - Tools: per‑call `opId` (turn and opId uniquely identify the node).
  - Sub‑agents: parent op hosts `childSession` subtree.

Consumer Migration Notes

- Slack status/footer: use opTree counts (`buildSnapshotFromOpTree`) and link to `/runs/:id/view` if exposed.
- CLI: prefer `opTreeAscii` for hierarchy dumps.
- API: add `/api/runs/:id/tree`; treat `logs[]/accounting[]` in results as legacy/derived.

Acceptance Criteria (Expectations to Verify Before Merging)

1) Single tracking source (no logs outside opTree)
- All LLM/tool logs and accounting events are appended to opTree only.
- Any `AIAgentResult.logs`/`accounting` are derived from opTree at the end (not written during runtime).
- Remove `ExecutionTree` writers and code paths; optional post-build grep ensures no remaining imports/usages.

2) Start events exist even without finishes
- `beginTurn` and `beginOp` record `startedAt` immediately for LLM attempts and all tool calls.
- If a run is canceled/crashes, unfinished nodes remain with `startedAt` only; viewers display them as in-progress/unknown.

3) All LLM requests/responses (full data) and accounting in opTree
- Request nodes include provider/model, message count/size, final-turn flag; when tracing, include detailed HTTP/SSE trace (capped/redacted).
- Response nodes include tokens (prompt/output/cacheR/cacheW/total), latency, response size, cost (computed/provider), status (success/failure) and reason.
- No separate accounting tracker exists; per-attempt LLM accounting is stored under the LLM op.

4) All tool requests/executions/responses/failures and accounting in opTree (all tool kinds)
- Request nodes record compact request lines; when tracing, full JSON args (capped).
- Response nodes record raw payload or preview and truncation notices; latency; ok/failed status; error message on failure.
- Accounting nodes record charactersIn/Out, latency, status for every tool call.
- Tool kinds covered: MCP, REST (manifest + OpenAPI), Internal (agent__), Sub‑agents (as `kind:'session'`).
- Sub‑agents: parent has a single tool op and a `childSession` subtree that streams live; child subtree includes its own LLM/tool logs/accounting.

5) Source/ingress, outputs, cumulative accounting (incl. sub‑agents)
- SessionNode root attributes include run source and metadata (CLI vs Slack: auto‑engage/mention/channel, team/channel/thread/user ids).
- Final outputs are recorded (final_report content and, for Slack, posted message text/blocks) for auditability.
- Cumulative accounting across the whole tree (parent + children) is derivable by opTree traversal; Slack/footer and APIs use opTree totals.

6) Single persistence path (save/load via opTree only)
- Old conversation JSON saving (`--save`, `--save-all`) and JSONL accounting saving are removed once migration phases complete.
- A single `--save-session` (and corresponding API) persists the versioned opTree payload (master + all sub‑agents).
- Any compatibility exports (flat logs/accounting arrays) are derived from opTree at save time only.

7) Real‑time updates from all agents and sub‑agents
- Parent opTree is updated live not only for master LLM/tools, but also for sub‑agent child sessions via streamed `onOpTree` snapshots from every child.
- Slack/UI consumers see nested progress as it happens; no more end‑only child attachment.

8) Console logging is self‑identifiable and opTree‑driven
- Every emitted log line (stderr) includes a compact hierarchical position that maps 1:1 to a node in opTree, across arbitrary sub‑agent depth (e.g., masterTurn.opIdx[.childTurn.opIdx...]).
- The log line also includes the kind (llm/tool/session) and minimal IDs (provider:model or provider:tool) to disambiguate.
- No direct logging anywhere; all log rendering goes through opTree events.

Console Logging Semantics (stdout/stderr behavior via opTree)

- Log line format (always)
  - Every stderr line includes master transaction and path: `[txn:<originTxnId>] <path> <kind/id>: <message>`.
  - This format is invariant across verbosity levels; flags control content volume, not the presence of txn/path.

- Defaults (no verbose/trace flags)
  - stdout: assistant’s final or streamed LLM output only.
  - stderr: only warnings/errors (WRN/ERR), minimal and self‑identifiable by path.
  - process return value reflects success/failure; not coupled to logging volume.

- Verbose (`--verbose`)
  - stderr: include opTree‑driven summaries for each LLM request/response and each tool request/response with accounting (latency, sizes), all tagged with hierarchical path.

- Tracing
  - `--trace-llm`: emit deep HTTP/SSE traces for LLM requests/responses (capped/redacted), attached to the corresponding LLM op path.
  - `--trace-tools`: emit deep request/response traces for tools (args and raw payloads, capped/redacted), attached to the tool op path.

- Rendering
  - The TTY renderer builds each line from the opTree node context (turn/op hierarchy), ensuring that any line can be traced back to exactly one opTree node.
  - Numeric path labels (e.g., 1.1.2.3) are acceptable for compactness; exact format is a renderer detail, but it must be bijective with the opTree position.

Final Verification Steps
- Manual runs covering: (a) MCP tool; (b) REST tool; (c) Internal tools (append_notes/final_report/batch with inner calls); (d) Sub‑agent with long‑running child emitting live snapshots; (e) OpenAPI‑generated REST tool.
- Confirm live opTree updates in Slack and via `/api/runs/:id/tree` during in‑flight execution.
- Confirm no references to `ExecutionTree` remain (static check) after Phase B.
- Confirm secrets redaction on trace logs via the new tree API.

Open Gaps, Assumptions, and Follow‑Ups (from repo re‑scan)

1) Sub‑agent recursion depth (clarified)
- Policy: arbitrary nesting depth is allowed as long as agents do not recurse. “Max recursion = 0” means no agent may call itself (A→A) nor form cycles (A→B→A). Use ancestry tracking to block cycles; otherwise allow depth N.
- Action: ensure `SubAgentRegistry` blocks cycles only; opTree naturally supports arbitrary depth.

2) TTY log rendering & grepability
- Requirement: greppable by `originTxnId` to get the entire session tree (master+children). Every console line must include `[txn:<originTxnId>]` and a stable compact path label derived from opTree (e.g., `1.2.3.1`). THK lines use the same labels.
- Implementation: render from opTree events; attach stable `path` to `LogEntry` at append time.

3) Consumers wired to ExecutionTree
- CLI and SessionManager currently depend on `onLog` arrays and ExecutionTree. Migration must switch them to opTree events for: live status, Slack progress, and final footers. Ensure `status-aggregator.ts` uses `buildSnapshotFromOpTree` everywhere.

4) OpenAPI tool generation
- Types/config reference `openapiSpecs`, but the current implementation relies on REST tools. Clarify:
  - In this phase, OpenAPI‑generated tools are exposed via RestProvider; opTree treats them as REST.
  - A true generator will still route through RestProvider and be indistinguishable at runtime.

5) Assistant thinking (THK) stream policy
- Policy update: THK is logging, but opTree must store structured reasoning per node. Introduce `reasoning` on LLM and tool ops:
  - `reasoning`: { chunks: [{ text: string, ts: number }], final?: string }
  - THK logs are produced from `reasoning.chunks` for visualization. This decouples opTree data from how logs are rendered.
  - Default: store chunks; apply size caps. A config flag may suppress saving content for privacy later.

6) Save size and performance
- Full opTree can be large. Save format uses gzip; atomic writes (tmp+rename) are mandatory. Other compressions (zstd/brotli) may come later.

7) Crash‑time partial saves
- Mandatory: snapshot at least on every sub‑agent finish. Optional: periodic snapshots (e.g., 1–5s) for long runs.

8) Billing JSONL rotation/locking
- Appends at session end; must use append‑only atomic writes. Rotation policy can be added later.

9) Slack headend behavior (clarified)
- Slack integrations enforce Slack Block Kit output via `$FORMAT` when the headend is Slack. Even if an agent declares JSON/markdown in frontmatter, the master session invoked by Slack is coerced to Slack Block Kit. The existing JSON guard in server code is effectively moot in practice; tracking is unaffected as opTree captures the session regardless of effective format.

10) Privacy of Slack user labels
- We store `userLabel` (derived). Confirm this is acceptable for persistence; or allow redacted save mode.

11) Viewer path labels
- Numeric path format (e.g., 1.2.3) must be generated from opTree indices (turn/op and child depth) and be stable/bijective. Do not compute from external arrays.

opTree Structure Revisions (to meet universality/consumer decoupling)

- OperationNode additions
  - `reasoning?: { chunks: { text: string; ts: number }[]; final?: string }` — structured store for LLM/tool reasoning text (source of THK logs)
  - `request?: { kind: 'llm'|'tool'; payload: unknown; size?: number }` — normalized request parameters (messages/tool args) with size caps
  - `response?: { payload: unknown; size?: number; truncated?: boolean }` — normalized response body or preview (post-cap)
  - Existing `logs` remain for event history; renderers should prefer `reasoning/request/response` for content and use `logs` for events

- TurnNode additions
  - `prompts?: { system: string; user: string }` (capped) — capture the effective prompts used for the turn
  - Optionally `assistant?: { content: string }` (capped) — assistant text that preceded tool calls or final report

- SessionNode attributes
  - Keep `ingress` metadata; include `originTxnId` for filename stability and grep
  - Include `pricingSnapshot?` for cost computations used during the run (optional)

Decisions (locked)

- Path label format: default to compact numeric labels (e.g., `1.2.3.1`). Optional kind hints (e.g., `L`/`T`) may be added under verbose modes if needed, but keep lines short by default.
- Reasoning persistence cap: no default cap now; we may add caps later as an option. Reasoning chunks are stored as emitted.
- Default compression: gzip first (easiest); add zstd/brotli later with auto‑detect.
- Snapshot policy: snapshot at every sub‑agent finish; no periodic snapshot by default.

Error Handling & Logging Policy

- Do not silently swallow errors. Any caught error in persistence, snapshotting, or opTree updates should be surfaced at least as a warning to stderr/console with a short reason to aid troubleshooting.
