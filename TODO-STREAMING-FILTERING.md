# TODO: Streaming Filtering for Orchestration Children + Duplicate Thinking Bug

## TL;DR

1. Advisors, routers, and handoff sessions stream output/thinking to headends incorrectly
2. Master agent thinking is emitted twice at the final turn (streaming + after final report)
3. Need to fix based on actual tests, not theories
4. Handoff chain must stream thinking/output for all agents but only stream the final-report of the LAST handoff

## Status

- **Implementation started** (alignment + docs + tests).
- **Deferred**: duplicate‑thinking bug + snapshot analysis until after alignment is complete.
  - **Update (2026-01-14):** applied common-gate helper usage in CLI/REST/MCP/Embed headends; added Phase 2 scenarios for advisor/subagent stream isolation; removed review file.
- **Post-restart verification (2026-01-14):**
  - Lint/build + Phase1/Phase2/Phase3:tier1 all ran and passed (see “Testing requirements” below for details + warnings).
 - **Update (2026-01-14):** Implemented per-attempt `turn_started` event payload + attempt-aware thinking replay guard, updated LLM headend turn headings, added retry coverage, and synced docs/tests.

### Repo Review (2026-01-14)

**Facts (verified from code):**

- **Shared filtering exists and is used only in OpenAI/Anthropic.**
  - `src/headends/shared-event-filter.ts` (new shared helpers).
  - OpenAI imports: `src/headends/openai-completions-headend.ts` (import at top).
  - Anthropic imports: `src/headends/anthropic-completions-headend.ts` (import at top).
- **Inline filtering still exists (duplicates) in CLI/REST/MCP/Embed.**
  - CLI: `src/cli.ts:2183-2192` (`if (!meta.isMaster) return;` + `pendingHandoffCount` check).
  - REST: `src/headends/rest-headend.ts:305-327` (inline output + final_report filtering).
  - MCP: `src/headends/mcp-headend.ts:393-409` (inline output + final_report filtering).
  - Embed: `src/headends/embed-headend.ts:492-544` (inline output + final_report filtering).
- **New unit tests for shared filter exist, but are untracked.**
  - `src/tests/unit/shared-event-filter.spec.ts` (untracked file in `git status`).
- **Phase2 harness gained handoff + stream-dedupe tests.**
  - `src/tests/phase2-harness-scenarios/phase2-runner.ts:11990-12270` (handoff event chain + router handoff event + stream-dedupe final report).

**Unknowns (need verification):**

- Whether advisor output suppression is covered by tests (no clear coverage found yet).
- Whether subagent/master stream isolation is covered by tests (no clear coverage found yet).
- Lint/build status after the restart (not re-run yet).

**Artifacts currently untracked (git status):**

- `TODO-STREAMING-FILTERING.md` (this plan file)
- `src/headends/shared-event-filter.ts`
- `src/tests/unit/shared-event-filter.spec.ts`

---

## Post-restart Plan (Next Steps)

1. **Verify remaining consumer alignment**  
   - Confirm any remaining non-headend sinks (e.g., SessionManager API output buffer) match the **common gate** rules or adopt the shared helper where safe.
2. **Docs consistency pass**  
   - Ensure every updated doc explicitly states `isFinal` is authoritative only on `final_report` events and uses **common gate** wording consistently.
3. **Resume deferred bug**  
   - Only after alignment is confirmed: investigate duplicate-thinking bug (per deferral).

## User Requirements (Verbatim)

### Current Expected Behavior (Before Advisors/Routers/Handoff)

**llm-headends (openai, anthropic):**
- The master agent thinking and output is directly visible in llm headends as output and thinking respectively.
- The headends also show all task-status reports from all agents, as thinking, in append-only mode
- The headends also have logic to multiplex true master agent thinking and task-status reports to the same thinking section

**slack headend:**
- This is stateful and exposes only the task-status reports from all agents, grouped in a way

**mcp headend:**
- Does not use any of them

**cli headend:**
- I think it matches closely the llm-headend, although the code is different

### What We Need (New Requirements)

1. **Advisors should be treated as subagents**
2. **Handoff (either from routers, or straight handoff) should inherit the same (master or subagent) from their agent that hands off to them**
3. **Handoff chain output rule (NEW):**
   - If the master agent initiates a handoff, its final-report is input to the handoff target and MUST NOT be streamed to clients
   - In a multi-handoff chain, stream thinking/output for all agents in the chain, but ONLY stream the final-report of the LAST handoff agent
   - Otherwise clients (open-webui) see intermediate final reports, which is wrong
4. **Standardize event flow and filtering (NEW):**
   - All headends must have a simple, consistent way to filter master vs subagent output
   - System must distinguish “master” vs “final” (handoff means master can be non-final)

### Bug: Duplicate Thinking at Final Turn

There is also a bug in the existing codebase, in both llm headends and cli:
- The real thinking of the master agent is emitted twice
- Once via streaming in real-time
- Once when the agent finishes (after its final-report is received)
- There is proper filtering when the agent has not provided its final report (so thinking is not duplicated in middle turns)
- The problem appears only at the final turn, after the final report has been streamed, we get a thinking section with previously streamed thinking duplicated

---

## Analysis Results

### Bug 1: Duplicate Thinking at Final Turn - NEEDS MORE INVESTIGATION

**User Clarification:** The bug appears in **open-webui**, which uses the **OpenAI headend** (NOT Anthropic).

**Symptoms (from user):**
- Master agent thinking is emitted twice
- Once via streaming in real-time
- Once when agent finishes (after final-report is received)
- Problem appears ONLY at the final turn, after the final report has been streamed
- Middle turns are fine (proper filtering exists there)
- Additional clarification: the *second* thinking dump happens **after** the final report and appears all at once (not streamed)

**Current State:**
- OpenAI headend HAS `reasoningEmitted` flag (line 356)
- OpenAI headend HAS guard `if (!reasoningEmitted)` at line 841
- Yet thinking is still duplicated somehow
- **Deferred:** Do **not** investigate or fix until the `onEvent` refactor + tests + docs are complete (per user).

**New Finding (evidence-based):**
- There is no obvious re-emit path in OpenAI headend once `reasoningEmitted` is true (guard at `src/headends/openai-completions-headend.ts:841-842`)
- This makes the duplicate-thinking bug likely dependent on runtime call order or a different layer (needs concrete capture to confirm)
- Snapshot `/opt/neda/.ai-agent/sessions/e4ea679a-84be-42b5-b9de-6bdb4d2588db.json.gz` does **not** show duplicate reasoning in opTree/logs (headend output is not captured in snapshot)
  - Evidence from snapshot: last turn index `9` has a single LLM op with `reasoningChunks=229` and no `reasoning.final` (single reasoning stream recorded)

**Working theory (speculation):**
- The post-turn “reasoning replay” path in TurnRunner can dump reasoning at once if `shownThinking` is false, even when a UI already displayed thinking
  - Evidence: `src/session-turn-runner.ts:750-784` emits reasoning chunks after the turn when `turnResult.shownThinking` is false
  - This would occur only at the end of a turn, aligning with “after final report” behavior
- Cross-headend symptom: CLI shows the same duplicate-thinking behavior even though its headend code path is different, pointing to a core emission issue rather than a headend-specific bug

**Investigation needed:**
- `flushReasoning()` is called from multiple places (lines 481, 520, 566, 579, 666, 695, 842, 872)
- Need to find which call path causes the duplicate at final turn
- The guard at line 841 may not cover all paths, or `reasoningEmitted` may be reset somewhere
- **Deferred:** Snapshot analysis is explicitly postponed.

---

### Bug 2: Advisor Output Streaming - VERIFICATION

**User report (open-webui):**
- Advisor **OUTPUT** streams to output (should not).
- Advisor **THINKING** is not visible (expected).

**Current code reality (evidence):**
- Advisors spawn with `isMaster: false` (`src/orchestration/advisors.ts#L83-L90`).
- OpenAI/Anthropic filtering uses `isMaster` (`src/headends/shared-event-filter.ts#L20-L27`).
- Output is suppressed when `isMaster` is false.

**Implication:**
- If advisor output still appears, **either**:
  1) `isMaster` is not `false` on advisor events, **or**
  2) a headend path bypasses `shared-event-filter`, **or**
  3) events are being mis‑tagged as master in meta.

**Verification needed (after onEvent alignment):**
- Capture event meta for advisor runs (ensure `isMaster=false`).
- Confirm the headend path used by open‑webui is the OpenAI headend + `shared-event-filter`.

### Code Paths

**Normal sub-agents (SubAgentRegistry.execute - subagent-registry.ts:236-262):**
- Creates wrapper callbacks with explicit metadata
- Passes `agentId: childAgentPath` in metadata
- Has `parentTxnId` set, so `isRootSession = false`

**Note:** SubAgentRegistry wrapper *does* send meta to headends (it builds meta itself).
This is the only path where headend agentId filtering can work today.

**Orchestration children (spawnOrchestrationChild - spawn-child.ts:173-185):**
- Passes `callbacks: parentSession.callbacks` directly (no wrapper)
- Has `parentTxnId` set via `trace.parentId` (line 165)
- So `isRootSession = false` for advisors

---

## Refactor Scope Analysis (onXXXX → onEvent)

**Update (code reality):** `onEvent` is already implemented and in use.  
Treat the items below as **verification + alignment** work, not a full rewrite.  
Evidence:
- `src/types.ts#L620-L652` defines `AIAgentEventMeta` (`isFinal`, `pendingHandoffCount`, `handoffConfigured`) and `AIAgentEvent` includes `handoff`.
- `src/agent-loader.ts#L770-L796` increments `pendingHandoffCount` when static handoff is configured.
- `src/ai-agent.ts#L2393-L2513` propagates `pendingHandoffCount` to router targets and decrements for static handoff.
- `src/session-turn-runner.ts#L1778-L1790` and `#L2496-L2522` emit `handoff` vs `final_report` based on router/pending count.

### Current onEvent Surfaces (verified)
- `src/types.ts#L635-L652` defines `AIAgentEvent` + `AIAgentEventCallbacks.onEvent`.
- Headends already consume `onEvent`:  
  - OpenAI `src/headends/openai-completions-headend.ts#L709-L771`  
  - Anthropic `src/headends/anthropic-completions-headend.ts#L698-L744`  
  - CLI `src/cli.ts#L2161-L2200`  
  - REST `src/headends/rest-headend.ts#L296-L344`  
  - MCP `src/headends/mcp-headend.ts#L390-L430`  
  - Embed `src/headends/embed-headend.ts#L489-L552`
- `SessionManager` already relays `onEvent` (`src/server/session-manager.ts#L164-L208`).
- `rg` shows **no** `onOutput/onThinking/onProgress/onLog` callback surfaces in `src/` (only `onEvent`).

### Verification / Alignment Tasks (no migration)
- Confirm **all** headends share equivalent filtering semantics (isMaster / pendingHandoffCount / isFinal / handoffSeen).
- Ensure `handoffConfigured` and `pendingHandoffCount` are populated on **every** event.
- Confirm `handoff` event is ignored by default but still marks `handoffSeen`.
- Update docs to match the **existing** onEvent model and the `isFinal` caveat.

---

## Alignment Analysis (Planning Only — No Code Yet)

### Goal (per user decisions)
- Align the **existing** `onEvent` pipeline with handoff/finality rules.
- Keep headend filtering consistent; no new interfaces.
- Update tests + docs to reflect the current event model and required behavior.

### Design Constraints (must honor)
- **No new public interfaces** (no additional callbacks beyond `onEvent`).
- **Single event stream** used everywhere (core + headends + providers + embed client + tests).
- **Finality awareness**: distinguish “master” from “final” (handoff chain rule).
- **Consistency**: all headends must implement identical filtering logic.

---

## Exhaustive Impact Checklist (No Surprises)

### 1) Types / Public API
- `src/types.ts`: replace `AIAgentCallbacks` with `onEvent`
- `AIAgentSessionConfig` consumers across CLI/headends/tests must migrate
- `AIAgentResult` remains, but ensure no reliance on removed callbacks in docs/tests

### 2) Core Runtime Emission
- `src/session-turn-runner.ts`: streaming output/thinking, final report emission, reasoning replay, turn started
- `src/session-progress-reporter.ts`: progress emission
- `src/ai-agent.ts`: log, accounting, opTree, snapshot, accounting_flush dispatch

### 3) Orchestration / Multi-agent
- `src/subagent-registry.ts`: meta-wrapping must emit `onEvent` with full meta
- `src/orchestration/spawn-child.ts`: propagate finality context
- Router early-exit path must produce events without final report

### 4) Headends
- OpenAI, Anthropic, CLI, REST, MCP, Embed, Slack (via SessionManager)

### 5) Persistence / Snapshots
- `src/persistence.ts`: migrate to onEvent
- `src/session-tree.ts`: opTree updates remain canonical; must also emit events

### 6) Provider / Tool Layer
- `src/llm-client.ts` already emits `onEvent(log)`
- `src/tools/mcp-provider.ts` already emits `onEvent(log)`
- `src/tools/tools.ts` already emits `onEvent(log/accounting)`

### 7) Tests / Harnesses
- Phase2 harness, Phase2 runner, Phase3 runner, unit tests, fixtures

### 8) Docs / Specs (must update same commit)
- `docs/AI-AGENT-INTERNAL-API.md`
- `docs/DESIGN.md`
- `docs/AI-AGENT-GUIDE.md`
- `docs/REASONING.md`
- `docs/SPECS.md`
- `docs/specs/accounting.md`
- `docs/specs/architecture.md`
- `docs/specs/headend-rest.md`
- `docs/specs/headends-overview.md`
- `docs/specs/headend-openai.md`
- `docs/specs/tools-final-report.md`
- `docs/specs/tools-task-status.md`
- `docs/specs/optree.md`
- `docs/specs/snapshots.md`
- `docs/specs/library-api.md`
- `docs/specs/logging-overview.md`
- `docs/specs/session-lifecycle.md`
- `README.md` (if callback usage shown)

---

## Proposed Internal Event Model (Draft — updated per decisions)

### Event Types (minimum set)
- `output` — assistant output chunk
- `thinking` — reasoning/thinking chunk
- `progress` — progress event (task-status, agent updates)
- `final_report` — final report content + metadata
- `handoff` — handoff payload (full upstream content) for **static** or **router tool** handoff
- `log` — structured logs
- `accounting` — accounting entry
- `turn_started` — turn boundary
- `snapshot` — session snapshot (persistence)
- `accounting_flush` — ledger flush
- `op_tree` — opTree updates
- `status` — universal UX status (headends may ignore)

### Required Metadata (per event)
- `agentId`, `callPath`, `sessionId`, `parentId`, `originId`
- `headendId`
- `renderTarget` (cli/slack/api/web/embed/sub-agent)
- `isRoot`
- `isMaster`
- `handoffConfigured`
- `pendingHandoffCount`
- `isFinal` (**authoritative only on `final_report` events**)
- `source` (e.g., `stream | replay | finalize`)
- `sequence` (monotonic per session)
- `eventTs`

### Payloads (per event)
- `output`: `{ text: string }`
- `thinking`: `{ text: string }`
- `progress`: `{ event: ProgressEvent }`
- `final_report`: `{ format: string; content?: string; content_json?: object; metadata?: object }`
- `handoff`: `{ format: string; content?: string; content_json?: object; metadata?: object }`
- `log`: `{ entry: LogEntry }`
- `accounting`: `{ entry: AccountingEntry }`
- `turn_started`: `{ turn: number }`
- `snapshot`: `{ payload: SessionSnapshotPayload }`
- `accounting_flush`: `{ payload: AccountingFlushPayload }`
- `op_tree`: `{ tree: SessionNode }`
- `status`: `{ message: string; phase?: string }`

**Resolved model constraints (no open questions):**
- Log/accounting share the same stream; persistence events are also event types.
- `onEvent` is synchronous; ordering is best‑effort with `sequence`.
- `source` tag is required to distinguish stream vs replay vs finalize (handoff is identified by event type).
- Core emits each logical content chunk **once** (no duplicate stream/replay unless explicitly tagged and selectable).
- `isFinal` is guaranteed correct **only** for `final_report` events.
- **Note:** `EventSource` is currently `stream | replay | finalize` (`src/types.ts`), so `handoff` events use `source: 'finalize'` today.

---

## Event Emission Sources (Current → Proposed)

### Core
- `src/session-turn-runner.ts`  
  - **Streaming**: emits `output` + `thinking` events  
  - **Final report stream**: `final_report` event (must respect `isFinal`)  
  - **Turn boundaries**: `turn_started`  
  - **Reasoning replay**: should emit `thinking` with explicit source tag (to avoid duplicate UI)
- `src/session-progress-reporter.ts`  
  - emits `progress` events via `onEvent`
- `src/ai-agent.ts`  
  - emits `log/accounting/op_tree/snapshot/accounting_flush` events

### Providers / Tooling
- `src/llm-client.ts` emits `onEvent(log)`  
- `src/tools/mcp-provider.ts` emits `onEvent(log)`  
- `src/tools/tools.ts` emits `onEvent(log/accounting)`

### Orchestration & Subagents
- `src/subagent-registry.ts` meta-wrapping must emit `onEvent` with full meta  
- `src/orchestration/spawn-child.ts` must propagate finality context
- `src/orchestration/handoff.ts` + router tool: emit `handoff` events (not `final_report`)

---

## Event Consumers (Current → Proposed)

### Headends (all must consume onEvent)
- OpenAI: `src/headends/openai-completions-headend.ts`
- Anthropic: `src/headends/anthropic-completions-headend.ts`
- CLI: `src/cli.ts`
- REST: `src/headends/rest-headend.ts`
- MCP: `src/headends/mcp-headend.ts`
- Embed: `src/headends/embed-headend.ts`
- Slack (via SessionManager): `src/headends/slack-headend.ts` + `src/server/session-manager.ts`

### Server Manager / UI Sync
- `src/server/session-manager.ts`  
  - Must consume events to build outputs, forward thinking/progress, and update opTree

### Persistence / Ledger
- `src/persistence.ts` should attach to `snapshot/accounting_flush` events

### Embed Public Client
- `src/headends/embed-public-client.ts` already uses `onEvent` and SSE event types (`status`/`report`/`done`) → must map from new internal `onEvent` stream

---

## Headend Behavior Preservation (No Regressions)

### OpenAI Headend (`src/headends/openai-completions-headend.ts`)
- Streams `output` as SSE content chunks.
- Streams `thinking` as `reasoning_content`.
- Builds reasoning transcript from `progress` events + thinking.
- Filters by agent identity today (meta.agentId); must move to `isMaster`.

### Anthropic Headend (`src/headends/anthropic-completions-headend.ts`)
- Streams `thinking` in `content_block_delta` for thinking blocks.
- Streams `output` in text blocks.
- Builds reasoning transcript from `progress` events + thinking.

### CLI (`src/cli.ts`)
- `output` prints only in verbose mode.
- `thinking` prints only in TTY mode and interleaves with logs.
- Uses `onTurnStarted` for turn headers in some scenarios.

### REST (`src/headends/rest-headend.ts`)
- Accumulates `output` and `thinking` for response payload (no streaming).
- Returns `finalReport` plus `reasoning`.

### MCP (`src/headends/mcp-headend.ts`)
- Accumulates `output` for response payload (no thinking today).

### Embed (`src/headends/embed-headend.ts`)
- Streams `report` chunks (from output).
- Emits `status` events from `progress`.
- Emits `meta` events from `turn_started` and sessionId.

### Slack (via `SessionManager`)
- Uses opTree updates to render state; no direct output/thinking streaming today.
- Must continue to receive progress/status updates correctly.

**Implication:** event stream must carry enough data to reproduce current behaviors above.

---

## Filtering & Finality Semantics (must be enforced centrally)

### Master vs Subagent
- `isMaster` determines **thinking/output** visibility.
- All headends must use the **same rule** for display filtering.
- `isMaster` is **not equivalent** to `isRootSession`:
  - Handoff targets can be master if the handing-off agent was master.
  - Advisors must always be subagent (never master), even if root in their own session.

### Final vs Non-Final
- `isFinal` determines **final_report** visibility.
- **Important:** `isFinal` is authoritative **only** on `final_report` events; it may be inconsistent on other events.
- Master agents can be **non-final** during handoff chains.
- Final report streaming must use `isFinal` only.

### Handoff Chain Rule (restated)
- Stream thinking/output for every agent in the chain (if master).
- Only the **last** agent’s final report is streamed to clients.

### Handoff Event Rule
- When a handoff occurs, emit a **`handoff`** event (not `final_report`).
- Headends ignore `handoff` by default, but may use it for debugging or UI.

### Advisor Rule
- Advisors behave as subagents (no output/thinking to headends).

---

## Finality Determination (Planning)

**Definitions (agreed):**
- `isFinal` is computed **once** when a session starts and does **not** change at runtime.
- `isFinal` is used **only** to decide `final_report` visibility.

**Algorithm (pendingHandoffCount):**
- `AIAgent.run()` receives `pendingHandoffCount` (default `0`).
- If the current agent has **static handoff configured**, increment `pendingHandoffCount` for its **own session**.
- When executing the **static handoff target**, pass `pendingHandoffCount` **without incrementing** (consumes one pending handoff).
- For **router tool handoff**:
  - Emit a `handoff` event for the caller (not a `final_report`).
  - Start the target with the **caller’s `pendingHandoffCount`** (if caller is not final, target is not final).
  - The target then applies its **own** static-handoff increment (if configured).
- Final formula (authoritative on `final_report` events):  
  - `isFinal = (pendingHandoffCount === 0) && isMaster`

**Router-specific rule (per user):**
- Router **without** handoff configured:
  - Tool handoff → emit `handoff`, no `final_report`.
  - Answer directly → `final_report` visible.
- Router **with** handoff configured:
  - `final_report` suppressed even if it answers directly.

**Where information lives today (note):**
- `routerSelection` exists in `TurnRunner.state.routerSelection` but is **no longer** the finality signal.
- Finality must be derived from `pendingHandoffCount` + `handoffConfigured` at session start.

---

## Alignment Impact Summary (Depth + Spread)

**Likely touchpoints (alignment + verification):**
- `src/types.ts` event/meta docs + any schema updates
- `src/session-turn-runner.ts` event emission (output/final_report/handoff)
- `src/ai-agent.ts` meta builder (`isFinal` + `pendingHandoffCount`)
- `src/server/session-manager.ts` relay logic
- All headends listed above (filter consistency)
- Subagent registry wrapper + orchestration helpers
- Persistence hooks (events are already routed; ensure docs match)

**Tests impacted:**
- Phase2 harness (`src/tests/phase2-harness-scenarios/phase2-runner.ts`)
- Phase2 runner (`src/tests/phase2-runner.ts`)
- Phase3 runner (`src/tests/phase3-runner.ts`)
- Unit tests (`src/tests/unit/session-progress-reporter.spec.ts`)
- Any fixtures that stub callbacks

**Docs impacted (must update with refactor):**
- **PROMINENT NOTE REQUIRED IN DOCS:** `isFinal` is **authoritative only on `final_report` events** and may be inconsistent on other event types. This must be explicitly stated in all relevant docs below.
- `docs/AI-AGENT-INTERNAL-API.md`
- `docs/DESIGN.md`
- `docs/AI-AGENT-GUIDE.md`
- `docs/REASONING.md`
- `docs/SPECS.md`
- `docs/specs/accounting.md`
- `docs/specs/architecture.md`
- `docs/specs/headend-rest.md`
- `docs/specs/headends-overview.md`
- `docs/specs/headend-openai.md`
- `docs/specs/tools-final-report.md`
- `docs/specs/tools-task-status.md`
- `docs/specs/optree.md`
- `docs/specs/snapshots.md`
- `docs/specs/library-api.md`
- `docs/specs/logging-overview.md`
- `docs/specs/session-lifecycle.md`
- `README.md` (if callback usage shown)

---

## Risks & Edge Cases (must plan for)

- **Duplicate thinking**: replay path must be tagged or suppressed when already streamed.
- **Streaming gaps**: some providers don’t stream reasoning; `thinking` may need fallback.
- **Embed client UX**: existing SSE `status/report/done` behavior must be replicated via events.
- **Progress flood**: task-status events must remain append-only in headends.
- **Handoff finality**: ensure final report suppression doesn’t hide the last agent’s report.
- **Finality propagation**: `pendingHandoffCount` must be passed correctly for static **and** tool handoffs.
- **Handoff visibility**: `handoff` events should not leak into UI unless explicitly enabled.
- **Meta propagation**: ensure every event includes correct `agentId/callPath` across subagents.
- **SessionManager**: preserves output buffers and opTree updates without losing events.
- **Router early exit**: must not emit final_report event; event stream should indicate router handoff.
- **Ordering**: log/accounting/progress events interleaving with output/thinking must be deterministic.
- **Backpressure**: async sinks must not stall streaming output.

---

## Test Plan (high level — no code yet)

- Phase2 deterministic scenarios:
  - master vs subagent streaming visibility
  - handoff chain final-report suppression
  - advisor output suppression
  - progress update behavior unchanged
- Phase3 live scenarios:
  - advisors/router/handoff flows (ensure output + finality)
- Regression: ensure accounting/log events still emitted and persisted

## Files to Investigate

### Streaming/Filtering Logic
- `src/headends/shared-event-filter.ts` - canonical `isMaster` + `isFinal` filtering
- `src/headends/openai-completions-headend.ts` / `anthropic-completions-headend.ts` - shared filter usage
- `src/cli.ts`, `src/headends/rest-headend.ts`, `src/headends/mcp-headend.ts`, `src/headends/embed-headend.ts` - inline filtering
- `src/session-turn-runner.ts#L2470-L2522` - final_report vs handoff emission
- `src/ai-agent.ts#L980-L1016` - event meta build (`isFinal`, `pendingHandoffCount`)
- `src/agent-loader.ts#L770-L796` - pendingHandoffCount increment for static handoff

### Orchestration
- `src/orchestration/spawn-child.ts` - spawnOrchestrationChild
- `src/orchestration/advisors.ts` - executeAdvisors
- `src/orchestration/handoff.ts` - executeHandoff
- `src/subagent-registry.ts` - SubAgentRegistry.execute (working reference)

### Duplicate Thinking Bug
- Where is thinking emitted at final turn?
- What filtering exists for already-streamed content?

---

## Tests to Check

- [ ] Phase 2/3 tests for streaming filtering
- [ ] Tests for sub-agent streaming isolation
- [ ] Tests for final turn thinking emission
- [ ] Tests for orchestration child streaming

---

## Implementation Plan

### Phase 1: Define event types + metadata (no runtime changes yet)
- Replace `AIAgentCallbacks` with `onEvent` in `src/types.ts`.
- Add event type `handoff`.
- Add metadata fields: `isMaster`, `handoffConfigured`, `pendingHandoffCount`, `isFinal`, `source`, `sequence`.
- Document `isFinal` as authoritative **only** for `final_report`.

### Phase 2: Core event emission
- `src/session-turn-runner.ts`: output/thinking/final_report/turn_started → events.
- `src/session-progress-reporter.ts`: progress → events.
- `src/ai-agent.ts`: log/accounting/op_tree/snapshot/accounting_flush → events.
- Ensure no duplicate emission (stream vs replay) unless explicitly tagged.

### Phase 3: Orchestration + handoff propagation
- `src/subagent-registry.ts`: wrap child callbacks with event meta.
- `src/orchestration/spawn-child.ts`: propagate `isMaster` + `pendingHandoffCount`.
- `src/orchestration/handoff.ts` + router tool handoff: emit `handoff` events and pass counters.
- Apply **pendingHandoffCount** algorithm from “Finality Determination”.

### Phase 4: Headends + SessionManager
- OpenAI + Anthropic share filter helper; others implement local filters.
- Update CLI/REST/MCP/Embed/Slack/SessionManager to consume events.
- Default ignore `handoff` events; optional debug path only.

### Phase 5: Providers / tools
- `src/llm-client.ts`, `src/tools/mcp-provider.ts`, `src/tools/tools.ts` migrate onXXX to onEvent.

### Phase 6: Tests + fixtures
- Update Phase2/Phase3 harnesses, unit tests, fixtures.
- Add new tests for handoff chains, router cases, and metadata correctness.

### Phase 7: Documentation (same commit)
- Update listed docs to reflect onEvent, handoff event, finality rules, and `isFinal` caveat.

### Phase 8: **Deferred bug work**
- Only after Phases 1‑7: inspect snapshot and investigate duplicate‑thinking bug.

## Decisions (User)

1. **Standardization approach:** single internal `onEvent` stream; remove **all** `onXXXX` callback surfaces.  
   - **Constraint:** no new public interfaces.
2. **Scope:** all headends + internal consumers + tests + providers (OpenAI, Anthropic, CLI, Embed, REST, MCP, Slack/SessionManager, LLMClient, MCPProvider, embed public client).
3. **Event stream composition:** log/accounting on same stream; persistence events (`snapshot`, `accounting_flush`, `op_tree`) are event types; `status` is universal.
4. **Event delivery:** `onEvent` is **synchronous**.
5. **Ordering/meta:** include `sequence` (monotonic), `source` tag; ordering is best‑effort interleaving; core emits each logical chunk **once**.
6. **Filtering:** headends filter; core emits all events. OpenAI+Anthropic share helper; others implement their own. `handoff` is ignored by default.
7. **Role/finality flags on all events:** `isMaster`, `handoffConfigured`, `pendingHandoffCount`, `isFinal`.  
   - No `routerHandoffUsed`.  
   - `isFinal` is authoritative **only** on `final_report` events.
8. **Handoff semantics:**  
   - Upstream `final_report` is treated as **handoff input** (not client‑visible).  
   - Emit a **`handoff`** event for both **static** and **router tool** handoffs, with **full content**.  
   - Handoff target **inherits `isMaster`** from the caller.
9. **Handoff config rule (master):** if a master has handoff configured, its `final_report` is **always** suppressed, even if it does not hand off.
10. **Router cases:**  
   - Router **without** handoff config: tool handoff → `handoff`, no `final_report`; answer directly → `final_report` visible.  
   - Router **with** handoff config: `final_report` suppressed even if it answers directly.
11. **Finality algorithm:** `pendingHandoffCount` propagation (see “Finality Determination”).  
   - `isFinal = (pendingHandoffCount === 0) && isMaster`.
12. **Headend state:** track `handoffSeen` to suppress intermediate `final_report` when needed.
13. **Snapshot to analyze (deferred):** `/opt/neda/.ai-agent/sessions/e4ea679a-84be-42b5-b9de-6bdb4d2588db.json.gz`.
14. **Bug triage ordering:** do **not** investigate or fix the duplicate‑thinking bug until the `onEvent` refactor + tests + docs are complete.
15. **Delivery expectation:** full implementation, tests, docs updated; lint/build pass before finishing.
16. **Terminology:** keep existing **“common gate/gating”** wording; do **not** perform a doc-wide terminology sweep.
17. **Review file:** delete `TODO-STREAMING-FILTERING-REVIEW.md`.

## Decisions Resolved (2026-01-14)

1. **Headend filter consistency:** Applied shared helper across CLI/REST/MCP/Embed.  
   - **Evidence:**  
     - `src/cli.ts:2183-2194`  
     - `src/headends/rest-headend.ts:301-321`  
     - `src/headends/mcp-headend.ts:393-409`  
     - `src/headends/embed-headend.ts:491-535`

2. **Review file:** Deleted `TODO-STREAMING-FILTERING-REVIEW.md` (confirmed missing on disk).

3. **Test scope:** Added advisor/subagent stream-isolation coverage in Phase 2 harness.  
   - **Evidence:** `src/tests/phase2-harness-scenarios/phase2-runner.ts:12273-12513`  
   - **Unit helper coverage:** `src/tests/unit/shared-event-filter.spec.ts:1-46`

4. **Duplicate thinking replay (retry path):** Implement attempt-aware “emit once” ledger.  
   - **Decision (2026-01-14):** Each attempt’s thinking must be emitted exactly once (stream OR replay), never skipped or double-emitted.  
   - **Rationale:** Avoid duplicate thinking on final turn while preserving per-attempt visibility.

5. **LLM headend turn labels:** Show retry/forced-final context.  
   - **Decision (2026-01-14):** LLM headends must surface `Turn X, Attempt Y, {reason}`.
   - **Reason format:** use raw reason/slug values (no friendly names).

6. **Turn annotation event shape:** enrich `turn_started`.  
   - **Decision (2026-01-14):** Single `turn_started` event emitted per attempt with annotations (`attempt`, `isRetry`, `isFinalTurn`, `forcedFinalReason`, `retrySlugs`); tests ignore `isRetry=true` when counting turns.

## Decisions Pending (2026-01-14)

None.

---

## Standardization Plan (Draft)

**Goal:** One internal event stream (`onEvent`) used by all headends and internal consumers (no new public interfaces).

**Core events needed:**
- `output` (text chunk)
- `thinking` (text chunk)
- `progress` (event)
- `final_report` (full final report content + metadata)
- `handoff` (full upstream content for static/tool handoff)
- `log` (LogEntry)
- `accounting` (AccountingEntry)
- `turn_started` (turn boundary)
- `snapshot` (SessionSnapshotPayload)
- `accounting_flush` (AccountingFlushPayload)
- `op_tree` (SessionNode)
- `status` (universal; headends can ignore)

**Required metadata on each event:**
- `agentId`, `callPath`, `sessionId`, `parentId`, `originId`
- `headendId`
- `renderTarget`
- `isRoot`, `isMaster`, `handoffConfigured`
- `pendingHandoffCount`, `isFinal` (**authoritative only on `final_report`**)
- `source`, `sequence`
- `eventTs`

**Enforcement:**
- Headends filter based on `isMaster` (streaming output/thinking)
- Final report streaming must use `isFinal` only
- `handoff` events are ignored by default unless a headend explicitly opts in
- All existing `onXXXX` callback surfaces are removed after migration; only `onEvent` remains

---

## Testing Requirements

- [ ] Test `onEvent` end‑to‑end emission + metadata (sequence/source/isMaster/isFinal/pendingHandoffCount).
- [x] Test advisor output isolation (no output/thinking to headends).
- [x] Test router handoff cases (with and without handoff config).
- [x] Test static handoff chains (final-report only from last agent).
- [ ] Verify task-status/progress behavior unchanged across headends.
- [ ] **Deferred:** duplicate‑thinking regression test (after refactor).
- [x] **Run all tests except `phase3:tier2`. All others must pass.**
- **Last run (2026-01-14):**
  - `npm run lint` ✅
  - `npm run build` ✅
  - `npm run test:phase1` ✅ (Vitest deprecation warning for `test.poolOptions`)
  - `npm run test:phase2` ✅ (expected harness warnings: simulated model errors, env permission, traced fetch clone warnings, lingering handles)
  - `npm run test:phase3:tier1` ✅

---

## Implied Decisions

- `handoff` event payload will carry the upstream report content only (`format`, `content`, `content_json`, `metadata`). No extra target identifiers unless later approved.
- Apply the **common gate** checks via shared helpers where possible, while keeping headend‑specific behavior (TTY/SSE/buffering) intact.
- Add Phase 2 scenarios to validate advisor/subagent stream isolation via event metadata.

---

## Handoff Inheritance Requirement

Per user requirement: "handoff (either from routers, or straight handoff) should inherit the same (master or subagent) from their agent that hands off to them"

**Current behavior:** Handoff targets are spawned as orchestration children, so they would be treated as sub-agents.

**Expected behavior:**
- If main agent hands off → target should stream as master
- If sub-agent hands off → target should stream as sub-agent

**Implementation:** Need to track "master vs sub-agent" status and propagate it through handoff.

---

## NEW: Handoff Final-Report Suppression Rule

**Requirement:**
- When a master agent hands off, its final-report MUST NOT be streamed to clients (it becomes the input to the next agent)
- Only the final-report of the last agent in a handoff chain should be streamed to clients

**Scope:**
- Applies to open-webui (OpenAI headend), but should be consistent across headends (OpenAI, Anthropic, CLI)
- Applies even when intermediate agents are "master" for streaming thinking/output
