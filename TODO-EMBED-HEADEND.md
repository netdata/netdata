# TODO: Public Embed Headend + JavaScript Library

## TL;DR

Create a polished public-facing experience for `neda/support.ai` that can be embedded across all Netdata frontends (learn.netdata.cloud, www.netdata.cloud, in-app dashboards, Netdata Cloud). Consists of:
1. New `embed-headend.ts` - SSE streaming with clean event structure
2. `ai-agent-public.js` - self-contained JS library served from the headend
3. Per-client UUIDs + end-user-visible conversation transcripts (no opTree), stored as `.json.gz` and updated on follow-ups

---

## Original Requirements (Costa, verbatim)

### Context: Current Open WebUI Experience

> "it shows reasonining_content and content of the master agent + task-status updates from all agents, all in append mode, so the output can get really long. It ignores reasoning_content and content of subagents. Generally, for llm-emulation I think this is the best we can have, and generally there is no pause there, it rolls. There must be a bug in this, because sometimes the reasoning_content of the master agent appears both before and after its content."

### Vision: Desired Public Experience

> "ideally, I am thinking of something very polished. Something like status updates from all agents (without much information - the status has 4 fields - probably 1-2 of them) and in streaming the final report (not the other content the assistants send - only the final report). This would allow the user to get frequent status updates and finally receive the final-report in streaming."

### JavaScript Library Purpose

> "a matching javascript file that should be able to render in any website, including our learn.netdata.cloud, our website www.netdata.cloud, in-app agent dashboards, in-app netdata cloud - so the idea is to have a library that will properly extract the information to be renderred and then each of these applications will provide the placement, theming, etc, with options (can the user chat?, are mermaid charts supported? etc)"

> "So, my idea is to somehow have the support agent integrated in all netdata front ends."

### Design Choices

1. **Status fields display**: "configurable in the js library"
2. **Agent hierarchy display**: "also configurable, I would start with a simple: latest update wins, all are shown in a single item, but latest stays longer until the next status update"
3. **Final report streaming**: Stream as chunks arrive (option A)
4. **JS library packaging**: "ai-agent-public.js, downloaded from the headend itself"
5. **Chat/follow-up support**: "B with configurable markers about what is old what is new, what is user what is assistant content" (multi-turn with markers)
6. **Headend approach**: New dedicated `embed-headend.ts` (option A)

---

## Requirements

### User Experience Goals
- Frequent, concise status updates from all agents (single item, latest wins)
- Stream final report in real-time (not intermediate assistant content)
- Multi-turn conversation support with clear old/new, user/assistant markers
- Works on any website with minimal integration effort
- Stable client UUID for follow-ups + conversation extraction

### Technical Requirements
- JS library served directly from headend (`GET /ai-agent-public.js`)
- SSE streaming for real-time updates
- Configurable status fields display
- Stateless multi-turn (client sends history array for prompts)
- No external dependencies in JS library
- Per-client UUIDs returned by headend and required for follow-ups
- Conversation transcript persistence in `.json.gz` per client (end-user-visible only)

---

## Analysis

### Current Architecture Review

**Existing Headends** (`src/headends/`):
- `openai-completions-headend.ts` - OpenAI-compatible, uses `reasoning_content` for progress
- `rest-headend.ts` - Simple REST, no streaming
- `mcp-headend.ts` - MCP protocol
- `anthropic-completions-headend.ts` - Anthropic-compatible
- `slack-headend.ts` - Slack integration

**Progress Data Available (validated)**:
```typescript
// agent__task_status schema (tool is only available when progress updates are enabled
// AND the agent has external tools or sub-agents)
{
  status: 'starting' | 'in-progress' | 'completed',
  done: string,
  pending: string,
  now: string,
  ready_for_final_report: boolean,
  need_to_run_more_tools: boolean
}

// onProgress events (from SessionProgressReporter)
AgentStartedEvent {
  type: 'agent_started',
  callPath, agentId, agentPath, agentName?, timestamp, txnId?, parentTxnId?, originTxnId?, reason?
}
AgentUpdateEvent {
  type: 'agent_update',
  callPath, agentId, agentPath, agentName?, message, timestamp, txnId?, parentTxnId?, originTxnId?
}
AgentFinishedEvent {
  type: 'agent_finished',
  callPath, agentId, agentPath, agentName?, metrics?, timestamp, txnId?, parentTxnId?, originTxnId?
}
AgentFailedEvent {
  type: 'agent_failed',
  callPath, agentId, agentPath, agentName?, error?, metrics?, timestamp, txnId?, parentTxnId?, originTxnId?
}
ToolStartedEvent {
  type: 'tool_started',
  callPath, agentId, agentPath, tool: { name, provider }, metrics?, timestamp
}
ToolFinishedEvent {
  type: 'tool_finished',
  callPath, agentId, agentPath, tool: { name, provider }, status, error?, metrics?, timestamp
}
```
Notes:
- `onProgress` emits only a single `message` string for task_status (status | done | pending | now).
  Structured fields are only present in tool parameters (opTree request payload).

**Final Report Structure (validated)**:
```typescript
{
  format: 'json' | 'markdown' | 'markdown+mermaid' | 'slack-block-kit' | 'tty' | 'pipe' | 'sub-agent' | 'text',
  content?: string,
  content_json?: Record<string, unknown>,
  metadata?: Record<string, unknown>,
  ts: number
}
```

## Validation (Code Evidence)

### Verified in code
- Existing headends live in `src/headends/*` (openai, anthropic, rest, mcp, slack).
- Progress event types/fields: `src/types.ts:176-234`.
- Progress emission: `src/session-progress-reporter.ts:64-171`.
- task_status schema + status message composition: `src/tools/internal-provider.ts:107-145` and `src/tools/internal-provider.ts:462-520`.
- task_status tool availability is gated (progress updates only when external tools or sub-agents exist): `src/ai-agent.ts:642-656`.
- Final report result shape includes `ts` and extended format set: `src/types.ts:688-721`.
- Final report is streamed to `onOutput` at finalize (dedupe by tail): `src/session-turn-runner.ts:2502-2538`.
- XML streaming filter passes through content outside the final tag unless filtered: `src/xml-transport.ts:570-664` and `src/session-turn-runner.ts:3464-3544`.
- OpenAI headend emits `reasoning_content` SSE deltas: `src/headends/openai-completions-headend.ts:356-375`.
- SSE helper only writes `data: <json>` and `data: [DONE]`: `src/headends/http-utils.ts:61-69`.
- Headends are configured via CLI flags (no embed headend config today): `src/cli.ts:885-925`.
- REST headend supports extra routes (can host additional endpoints): `src/headends/rest-headend.ts:118-125`.
- AgentRegistry supports `history` and `wantsProgressUpdates`: `src/agent-registry.ts:99-132`.

### Corrections / gaps found
- Status enum in the draft SSE event should use `'in-progress'` (not `'working'`) to match `agent__task_status`.
- `onProgress` does not include structured `done/pending/now`; only a `message` string is emitted today.
- Final report includes a required `ts` timestamp and more formats than listed above.
- No CORS headers exist in current headends (no `Access-Control-Allow-Origin` in `src/`).
- Headend settings are CLI-driven today; the `.ai-agent.json` headend block in this TODO is not implemented (Slack is the only headend reading config sections).

### Key Design Decisions

1. **SSE Event Structure** - Clean, typed events optimized for UI rendering
2. **Session Management** - Stateless server; client sends history for prompts
3. **JS Library** - Single file, no dependencies, TypeScript source
4. **Static Serving** - Library served from headend, no CDN needed

---

## Detailed Plan

### Implementation Plan (Concrete)

1) **Configuration + CLI**
- Add `embed` config block to `Configuration` (`src/types.ts`) and Zod schema (`src/config.ts`).
- CLI: add `--embed <port>` + `--embed-concurrency <n>` (enablement + port via CLI).
- CLI: pass `config.embed` into new `EmbedHeadend` (config via config).

2) **Progress event struct fields (Decision 2B)**
- Extend `AgentUpdateEvent` with optional `taskStatus?: TaskStatusData`.
- Extend `SessionProgressReporter.agentUpdate` payload to accept `taskStatus`.
- Change `InternalToolProvider.updateStatus` signature to accept `(text, taskStatusData)`.
- On `agent__task_status`, call updateStatus with structured data.
- Update any unit tests that rely on the event shape.

3) **SSE writer (browser‑native events)**
- Add helper `writeSseEvent(res, eventName, payload)` that emits:
  - `event: <name>\n`
  - `data: <json>\n\n`
- Keep existing `writeSseChunk` for OpenAI/Anthropic headends (no behavior change).

4) **Final‑report‑only streaming**
- Implement a strict XML final-tag filter usable by embed headend.
- Plan: add a new filter in `xml-transport.ts` that **only returns content inside the final tag**.
- Wire TurnRunner to emit metadata (`CallbackMeta`) indicating final‑tag chunks, or expose a dedicated `onFinalReportChunk` callback.

5) **Embed headend**
- New headend class `EmbedHeadend` (HeadendKind `embed`) using HTTP + SSE.
- Endpoints: `/health`, `/ai-agent-public.js`, `/v1/chat`.
- Request handling:
  - Generate `clientId` UUID if missing.
  - Build prompt history from `history[]` (user/assistant only).
  - Spawn session with `renderTarget: 'embed'` (new target).
  - Stream events: `client`, `meta`, `status`, `report`, `done`, `error`.

6) **Transcript persistence (server source of truth)**
- New module `embed-transcripts.ts`:
  - Load existing transcript by `clientId` from `sessionsDir/embed-conversations/`.
  - Append a new turn with entries:
    - user message
    - status updates (role: `status`)
    - assistant final report
  - Write `.json.gz` via temp file + rename.
- Overwrite file each turn (no client-side merging).

7) **Public JS library**
- Implement `NetdataSupport` in a single self-contained file.
- Use `fetch` + `ReadableStream` to parse `event:` + `data:` SSE.
- Store `clientId` and expose `getClientId()/setClientId()`.
- Accept `history[]` array for prompt context, but keep transcript server‑side.

8) **Docs + Specs**
- Add `docs/specs/headend-embed.md`.
- Update:
  - `docs/SPECS.md` (headend list + CLI flags)
  - `docs/AI-AGENT-GUIDE.md` (embed headend + clientId + transcript persistence)
  - `docs/specs/headends-overview.md` (new headend)

9) **Tests**
- Unit tests:
  - SSE event writer
  - Final‑tag streaming filter
  - Progress event struct fields
- Phase 2 harness (if core turn/stream logic changes).
- Run `npm run lint` + `npm run build`.

### Phase 1: SSE Protocol Design

#### Event Types

```typescript
// Status update from any agent
interface StatusEvent {
  event: 'status';
  data: {
    agent: string;        // friendly name (e.g., "web-search", "support")
    agentPath: string;    // full path for debugging
    status: 'starting' | 'in-progress' | 'completed' | 'failed';
    message?: string;     // derived from task_status or progress event
    done?: string;        // optional, if we add structured status fields
    pending?: string;     // optional, if we add structured status fields
    timestamp: number;
  }
}

// Final report chunk (streaming)
interface ReportEvent {
  event: 'report';
  data: {
    chunk: string;        // markdown chunk
    index: number;        // chunk sequence number
  }
}

// Session/conversation metadata
interface MetaEvent {
  event: 'meta';
  data: {
    sessionId: string;
    turn: number;         // conversation turn (1 = first question)
    agentId: string;
  }
}

// Completion
interface DoneEvent {
  event: 'done';
  data: {
    success: boolean;
    metrics?: {
      durationMs: number;
      tokensIn: number;
      tokensOut: number;
      agentsRun: number;
      toolsRun: number;
      costUsd?: number;
    };
    reportLength: number;  // total characters in report
  }
}

// Error
interface ErrorEvent {
  event: 'error';
  data: {
    code: string;
    message: string;
    recoverable: boolean;
  }
}
```

### Phase 2: Embed Headend Implementation

#### File: `src/headends/embed-headend.ts`

**Endpoints**:
```
GET  /ai-agent-public.js     → Serve JS library (with caching headers)
GET  /health                  → Health check
POST /v1/chat                 → Start/continue conversation (SSE response)
```

**POST /v1/chat Request**:
```typescript
interface ChatRequest {
  message: string;           // user message
  clientId?: string;         // stable UUID (server returns new UUID if omitted)
  history?: { role: 'user' | 'assistant' | 'status'; content: string }[]; // for prompt only (not transcript)
  sessionId?: string;        // omit for new conversation
  format?: string;           // output format (default: 'markdown')
  agentId?: string;          // which agent (default from config)
}
```

**Session Storage**:
- Stateless on server for conversation history (client sends `history[]` for LLM prompt).
- Server maintains only per-request state; transcripts are rebuilt from server events and overwritten.

**Key Implementation Points**:
1. Emit status updates from `agent__task_status` (decision: parse opTree vs extend progress events)
2. Filter output so only final report content is streamed (XML final tag only)
3. Use `onProgress` for agent lifecycle + tool events (optional)
4. Use `result.finalReport` for completion payloads + metrics
5. Generate/return stable `clientId` UUID and persist end-user transcript `.json.gz`

### Phase 3: JavaScript Library

#### File: `src/headends/embed-public-client.ts` (compiled to `ai-agent-public.js`)

**Configuration Interface**:
```typescript
interface NetdataSupportConfig {
  // Required
  endpoint: string;              // headend URL

  // Agent selection
  agentId?: string;              // default agent to use

  // Status display
  statusFields?: ('status' | 'now' | 'done' | 'pending')[];  // default: ['status', 'now']

  // Multi-turn
  enableChat?: boolean;          // default: true
  // Client identity (stable UUID)
  clientId?: string;             // optional, headend returns if missing

  // Content markers (for styling)
  markers?: {
    userClass?: string;          // CSS class for user messages
    assistantClass?: string;     // CSS class for assistant messages
    oldClass?: string;           // CSS class for previous turns
    newClass?: string;           // CSS class for current turn
    statusClass?: string;        // CSS class for status updates
  };

  // Features
  supportsMermaid?: boolean;     // hint for mermaid chart rendering

  // Callbacks
  onStatus?: (status: StatusData) => void;
  onReportChunk?: (chunk: string, fullReport: string) => void;
  onComplete?: (result: CompletionResult) => void;
  onError?: (error: ErrorData) => void;
  onTurnStart?: (turn: number, isUser: boolean) => void;
}
```

**Public API**:
```typescript
class NetdataSupport {
  constructor(config: NetdataSupportConfig);

  // Start new conversation or continue existing
  ask(message: string): Promise<void>;

  // Get current session ID (for persistence)
  getSessionId(): string | null;

  // Restore session (e.g., from localStorage)
  setSessionId(sessionId: string): void;

  // Get current client ID (stable UUID)
  getClientId(): string | null;

  // Restore client ID (e.g., from localStorage)
  setClientId(clientId: string): void;

  // Get conversation history
  getHistory(): ConversationTurn[];

  // Clear conversation
  reset(): void;

  // Abort current request
  abort(): void;

  // Check if request in progress
  isLoading(): boolean;
}
```

**Internal Implementation**:
- Uses native `fetch` with `ReadableStream` for SSE parsing
- No EventSource (allows POST with body)
- Accumulates report chunks internally
- Fires callbacks at appropriate times
- Handles reconnection on network errors (configurable)
- Emits `history[]` as an array of messages (user/assistant/status) for prompt context

### Phase 4: Integration Examples

#### Minimal Integration

Note: When rendering markdown as HTML, ALWAYS sanitize the output using DOMPurify
or equivalent library to prevent XSS attacks. The examples below show proper sanitization.

```html
<script src="https://support.netdata.cloud/ai-agent-public.js"></script>
<script src="https://cdn.jsdelivr.net/npm/dompurify@3/dist/purify.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
<div id="support-status"></div>
<div id="support-response"></div>
<script>
  var responseEl = document.getElementById('support-response');

  var support = new NetdataSupport({
    endpoint: 'https://support.netdata.cloud',
    // clientId: localStorage.getItem('netdata-support-client-id') || undefined,
    onStatus: function(s) {
      document.getElementById('support-status').textContent =
        s.agent + ': ' + s.message;
    },
    onReportChunk: function(chunk, full) {
      // SECURITY: Sanitize before rendering - required for XSS prevention
      var rawHtml = marked.parse(full);
      var safeHtml = DOMPurify.sanitize(rawHtml);

      // Clear and rebuild content safely
      responseEl.textContent = '';
      var template = document.createElement('template');
      template.innerHTML = safeHtml;
      responseEl.appendChild(template.content.cloneNode(true));
    }
  });

  support.ask('How do I configure alerts?');
  // Example persistence:
  // support.onClientId = function(id) { localStorage.setItem('netdata-support-client-id', id); };
</script>
```

#### React Integration Pattern

For React apps, use a markdown component that handles sanitization internally,
or sanitize before rendering. Never render unsanitized HTML.

```tsx
// Example using a safe markdown rendering approach
import { useNetdataSupport } from '@netdata/support-client';
import ReactMarkdown from 'react-markdown';  // react-markdown is safe by default

function SupportWidget() {
  const { ask, status, report, isLoading, history } = useNetdataSupport({
    endpoint: 'https://support.netdata.cloud'
  });

  return (
    <div>
      {history.map((turn, i) => (
        <div key={i} className={turn.role}>
          <ReactMarkdown>{turn.content}</ReactMarkdown>
        </div>
      ))}
      {isLoading && <div className="status">{status?.message}</div>}
      {report && <ReactMarkdown>{report}</ReactMarkdown>}
      <input onKeyDown={(e) => e.key === 'Enter' && ask(e.currentTarget.value)} />
    </div>
  );
}
```

---

## Implementation Order

### Step 1: SSE Protocol Types
- [ ] Create `src/headends/embed-types.ts` with event interfaces

### Step 2: Embed Headend Core
- [ ] Create `src/headends/embed-headend.ts`
- [ ] Implement health endpoint
- [ ] Implement static file serving for JS library
- [ ] Implement basic POST /v1/chat with SSE response

### Step 3: Client UUID + Transcript Persistence
- [ ] Generate/return stable `clientId` UUID per user
- [ ] Persist transcript to `.json.gz` per clientId (overwrite each turn)
- [ ] Store transcripts under `sessionsDir/embed-conversations/`

### Step 4: Event Streaming
- [ ] Wire up `onProgress` → `status` events
- [ ] Wire up task_status tool → `status` events
- [ ] Detect and stream final report only (not intermediate content)
- [ ] Send `meta` event at start
- [ ] Send `done` event at completion
- [ ] Send `client` event with stable UUID (new/confirmed)

### Step 5: JavaScript Library
- [ ] Create `src/headends/embed-public-client.ts`
- [ ] Implement SSE parsing with fetch
- [ ] Implement configuration handling
- [ ] Implement callback dispatching
- [ ] Implement session management (client-side)
- [ ] Build script to compile to `ai-agent-public.js`

### Step 6: Multi-turn Conversation
- [ ] Server: stateless; client sends `history[]` for prompt context
- [ ] Client: track turns and apply markers
- [ ] Server: build transcript from server events + overwrite `.json.gz` per clientId
- [ ] Client: expose history API

### Step 7: Testing & Polish
- [ ] Unit tests for event parsing
- [ ] Integration test with support.ai
- [ ] Error handling edge cases
- [ ] Documentation

### Testing Status (latest)
- Phase 1: ✅ `npm run test:phase1`
- Phase 2: ✅ `npm run test:phase2`
- Phase 3 tier 1: ⚠️ partial
  - `nova/minimax-m2.1`: ✅ full pass (9 scenarios, all PASS).
  - `nova/gpt-oss-20b`: ❌ flaky multi-turn (missing `agent__test-agent2`) + timeouts at 120s/300s/600s runs.

Decision needed:
1) Rerun Phase 3 tier1 with a longer timeout (e.g., 15–20 minutes) to get a full pass/fail summary.
2) Run Phase 3 tier1 per-model to isolate flakiness (gpt-oss-20b first).
3) Accept current results (report partial + known flake) and proceed.

### Decision needed (unexpected changes)
Evidence: `git status` shows unrelated edits in:
- `neda/support-request.ai`
- `neda/support.ai`

Options:
1) Exclude them from embed headend commits (recommended).
2) Include them in the same commit.
3) Leave them uncommitted for now (no action).

Decision: Option 3 — leave them uncommitted for now.

### Decision (embed config resolution)
Decision: Option 1 — keep strict embed config resolution (missing placeholders fail at startup).

---

## Files to Create/Modify

### New Files
- `src/headends/embed-headend.ts` - Main headend implementation
- `src/headends/embed-types.ts` - Type definitions for SSE protocol
- `src/headends/embed-public-client.ts` - JavaScript library source
- `src/headends/embed-transcripts.ts` - Transcript persistence (end-user-visible only)

### Modified Files
- `src/headends/headend-manager.ts` - Register new headend type
- `src/headends/types.ts` - Add `embed` kind
- `src/cli.ts` - Add `--embed <port>` + concurrency option + config loading for embed settings
- `src/headends/http-utils.ts` - Add SSE writer that supports `event:` + `data:`
- `src/session-progress-reporter.ts` + `src/types.ts` - Extend progress events with structured task_status fields

### Build Artifacts
- `dist/ai-agent-public.js` - Compiled JS library (served by headend)

---

## Configuration

### Current reality (validated)
- Headends are started via CLI flags (`--api`, `--mcp`, `--openai-completions`, `--anthropic-completions`, `--slack`) in `src/cli.ts:885-925`.
- Slack is the only headend that reads config sections (`slack`, `api`) from layered `.ai-agent.json` (`src/headends/slack-headend.ts:673-770`).

### Embed configuration (implemented)
```json
{
  "headends": {
    "embed": {
      "type": "embed",
      "port": 8080,
      "concurrency": 20,
      "defaultAgent": "support.ai",
      "sessionTtlMs": 1800000,
      "corsOrigins": ["*.netdata.cloud", "localhost:*"]
    }
  }
}
```

---

## Decisions (Costa)

### Confirmed

1) **Embed surface location**
- Decision: **Option A** (new headend + new CLI flag `--embed <port>`).

2) **Structured status payloads**
- Decision: **Option B** (extend progress events to include task_status fields).

4) **Session storage / persistence**
- Decision: **No change to persistence**. Keep session snapshots to `.json.gz` as today.
- Note: This keeps current snapshot behavior but does not by itself provide multi-turn history state.

5) **Configuration source**
- Decision: **CLI for enablement + port**, **config for embed settings** (same pattern you described).
- Note (validated): current headends do *not* read headend config except Slack; this is new work.

### Confirmed (Costa)

3) **Final report streaming policy**
Evidence: XML streaming filter can pass content outside the final tag (`src/xml-transport.ts:570-664`).
- Decision: **Option A** (strictly stream only inside `<ai-agent-...-FINAL>`).
- Note: this requires a core change (new strict filter or callback metadata), because current `onOutput` does not mark which chunks are inside the final tag.

4b) **Multi-turn state (new, required by decision #4)**
Evidence: Session snapshots are write-once and do not provide live history.
- Decision: **Option A** (stateless; client sends full history each request).
- Note: revisit later if we want server‑side memory to preserve context.

6) **SSE framing**
Evidence: `writeSseChunk` only emits `data:` lines (`src/headends/http-utils.ts:61-69`).
- Decision: **Option B** (browser‑native `event:` + `data:` lines).

7) **Client UUID + transcript persistence (new requirement)**
Evidence:
- Current persistence writes opTree snapshots only (`src/persistence.ts:47-68`).
- `originId`/`txnId` exist per session, but no stable client UUID today (`src/ai-agent.ts:557-568`).
- Headends do not persist end-user-visible transcripts.

Decisions confirmed:
- 7A) **Transcript format**
  - Decision: **Option A** (single JSON object with `turns: []`, rewrite file each turn).
- 7B) **Transcript contents**
  - Decision: **Option C** (user message + final report + status updates).
- 7C) **Transcript directory**
  - Decision: **Option B** (reuse `sessionsDir` with `sessions/embed-conversations/`).

Additional notes from Costa:
- Client UUID should be stable; overwrite the transcript file instead of merging when possible.
- Client should send history as an array of messages with `role` (user/assistant/...) to simplify stateless follow-ups.
- **History source of truth**: server ignores client history for transcripts; server is source of truth.
- **Role for status entries**: `role: "status"`.

### Transcript JSON shape (example)
```json
{
  "version": 1,
  "clientId": "550e8400-e29b-41d4-a716-446655440000",
  "origin": "embed",
  "updatedAt": "2026-01-12T12:34:56.789Z",
  "turns": [
    {
      "turn": 1,
      "ts": "2026-01-12T12:30:00.000Z",
      "entries": [
        { "role": "user", "content": "How do I configure alerts?" },
        { "role": "status", "content": "starting | Planning | gather alert config docs" },
        { "role": "status", "content": "in-progress | Found docs | verify examples" },
        { "role": "assistant", "content": "Here is how to configure alerts..." }
      ]
    },
    {
      "turn": 2,
      "ts": "2026-01-12T12:34:00.000Z",
      "entries": [
        { "role": "user", "content": "Can you show a config example?" },
        { "role": "status", "content": "in-progress | Locate examples | parse YAML" },
        { "role": "assistant", "content": "Example YAML:\n..." }
      ]
    }
  ]
}
```

---

## Decisions Made

> "All these must be configurable. Especially for authorization, I would love to have a way to make it difficult to integrate it into third party sites, but I understand for a public API this would be difficult to achieve." — Costa

NOTE (validated): None of the CORS/auth/rate-limit behavior exists in current headends; this is all new work.

### 1. CORS Policy
**Decision**: Configurable whitelist
```json
{
  "corsOrigins": ["*.netdata.cloud", "localhost:*"]
}
```

### 2. Rate Limiting
**Decision**: Configurable, per-origin and per-IP
```json
{
  "rateLimit": {
    "enabled": true,
    "requestsPerMinute": 10,
    "burstSize": 5
  }
}
```

### 3. Authentication / Third-Party Protection
**Decision**: Configurable, with two-tier access model

#### The Constraint (Costa, verbatim)

> "we need to support embedding into netdata agent dashboards. These can be on any URL and their dashboard cannot have it embedded"

**Implications:**
- Agent dashboards run on arbitrary URLs (`localhost:19999`, `192.168.x.x:19999`, custom domains)
- Agent serves static files - no backend to generate signed tokens
- CORS whitelist impossible (can't enumerate all user URLs)
- ~30% of agents are connected to Netdata Cloud
- Agent dashboards have access to the agent's GUID

#### Two-Tier Access Model

```
┌─────────────────────────────────────────────────────────────────────┐
│  TIER 1: Netdata Properties (cloud, learn, www)                     │
│                                                                     │
│  Identification: Origin header matches whitelist                    │
│  Protection: CORS whitelist + Origin validation                     │
│  Rate limits: Higher (e.g., 60 req/min per session)                 │
│  Optional: Signed tokens for authenticated cloud users              │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│  TIER 2: Agent Dashboards (any origin)                              │
│                                                                     │
│  Identification: X-Netdata-Agent-GUID header                        │
│  Protection: Per-GUID rate limiting                                 │
│  Rate limits: Lower (e.g., 10 req/min per GUID)                     │
│  Optional: Verify GUID exists in Cloud registry (for 30% connected) │
└─────────────────────────────────────────────────────────────────────┘
```

#### How Agent Dashboard Requests Work

```
Agent Dashboard (any URL)
    │
    │  POST /v1/chat
    │  X-Netdata-Agent-GUID: 550e8400-e29b-41d4-a716-446655440000
    │  Origin: http://192.168.1.50:19999
    │
    ▼
Embed Headend
    │
    ├─ GUID present? Yes
    │   ├─ Rate limit by GUID (not IP)
    │   ├─ Optional: verify GUID in Cloud registry
    │   │   ├─ Found: higher trust, higher limits
    │   │   └─ Not found: allow anyway, lower limits
    │   └─ Process request
    │
    └─ GUID absent?
        ├─ Check Origin against whitelist
        │   ├─ Match: Tier 1 (Netdata property)
        │   └─ No match: reject or very strict limits
        └─ Rate limit by IP
```

#### Why This Works Against Third Parties

| Attack | Defense |
|--------|---------|
| Copy JS library, no GUID | Rejected (no GUID, unknown origin) |
| Guess/enumerate GUIDs | GUIDs are UUIDs - 2^122 possibilities |
| Steal one GUID | That GUID has its own rate limit (10 req/min) |
| Steal many GUIDs | Each has separate limit; abuse is detectable |
| Proxy with spoofed GUID | Works, but rate-limited per GUID; not scalable |

#### Configuration

```json
{
  "auth": {
    "tiers": {
      "netdata-properties": {
        "origins": ["*.netdata.cloud", "localhost:*"],
        "rateLimit": { "requestsPerMinute": 60 }
      },
      "agent-dashboards": {
        "requireGuid": true,
        "verifyGuidInCloud": false,
        "rateLimit": { "requestsPerMinutePerGuid": 10 }
      },
      "unknown": {
        "allow": false,
        "rateLimit": { "requestsPerMinutePerIp": 2 }
      }
    },
    "signedTokens": {
      "enabled": false,
      "secret": "...",
      "ttlSeconds": 3600
    }
  }
}
```

#### JS Library Integration

```javascript
// For Netdata properties (automatic)
const support = new NetdataSupport({
  endpoint: 'https://support.netdata.cloud'
  // Origin header sent automatically by browser
});

// For Agent dashboards
const support = new NetdataSupport({
  endpoint: 'https://support.netdata.cloud',
  agentGuid: NETDATA.registry.machine_guid  // from agent's JS globals
});
```

### 4. Prometheus Metrics
**Decision**: Configurable, enabled by default
```json
{
  "metrics": {
    "enabled": true,
    "path": "/metrics"
  }
}
```

Metrics to expose:
- `embed_requests_total{agent, origin, status}`
- `embed_sessions_active`
- `embed_session_duration_seconds`
- `embed_report_chunks_total`
- `embed_errors_total{code}`

---

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Large JS bundle size | Keep dependencies to zero, target <10KB minified |
| Session memory usage | TTL cleanup + max sessions limit |
| Final report stream includes non-final content | Add strict XML final-tag filter before emitting chunks |
| Missing structured status fields | Parse opTree tool requests or extend progress events |
| CORS/auth not present in current headends | Implement CORS + auth/rate limits in embed headend |
| Browser compatibility | Target ES2020, test in Chrome/Firefox/Safari |

---

## Success Criteria

1. Status updates appear within 500ms of agent activity
2. Final report streams smoothly without intermediate content
3. Multi-turn conversation works with clear visual separation
4. JS library < 10KB minified
5. Works on learn.netdata.cloud, www.netdata.cloud, and in-app
