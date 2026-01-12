# TODO: Public Embed Headend + JavaScript Library

## TL;DR

Create a polished public-facing experience for `neda/support.ai` that can be embedded across all Netdata frontends (learn.netdata.cloud, www.netdata.cloud, in-app dashboards, Netdata Cloud). Consists of:
1. New `embed-headend.ts` - SSE streaming with clean event structure
2. `ai-agent-public.js` - self-contained JS library served from the headend

## Requirements

### User Experience Goals
- Frequent, concise status updates from all agents (single item, latest wins)
- Stream final report in real-time (not intermediate assistant content)
- Multi-turn conversation support with clear old/new, user/assistant markers
- Works on any website with minimal integration effort

### Technical Requirements
- JS library served directly from headend (`GET /ai-agent-public.js`)
- SSE streaming for real-time updates
- Configurable status fields display
- Session management for multi-turn conversations
- No external dependencies in JS library

---

## Analysis

### Current Architecture Review

**Existing Headends** (`src/headends/`):
- `openai-completions-headend.ts` - OpenAI-compatible, uses `reasoning_content` for progress
- `rest-headend.ts` - Simple REST, no streaming
- `mcp-headend.ts` - MCP protocol
- `anthropic-completions-headend.ts` - Anthropic-compatible
- `slack-headend.ts` - Slack integration

**Progress Data Available**:
```typescript
// task_status from agent__task_status tool
{
  status: 'starting' | 'in-progress' | 'completed',
  done: string,      // up to 15 words
  pending: string,   // up to 15 words
  now: string,       // up to 15 words
  ready_for_final_report: boolean,
  need_to_run_more_tools: boolean
}

// Progress events from all agents
AgentStartedEvent  { agentPath, agentName?, reason? }
AgentUpdateEvent   { agentPath, agentName?, message }
AgentFinishedEvent { agentPath, agentName?, metrics? }
AgentFailedEvent   { agentPath, agentName?, error?, metrics? }
```

**Final Report Structure**:
```typescript
{
  format: string,           // 'markdown', 'json', 'slack-block-kit', etc.
  content?: string,         // text content
  content_json?: object,    // JSON content (for json format)
  metadata?: object         // additional metadata
}
```

### Key Design Decisions

1. **SSE Event Structure** - Clean, typed events optimized for UI rendering
2. **Session Management** - Server-side session storage for multi-turn
3. **JS Library** - Single file, no dependencies, TypeScript source
4. **Static Serving** - Library served from headend, no CDN needed

---

## Detailed Plan

### Phase 1: SSE Protocol Design

#### Event Types

```typescript
// Status update from any agent
interface StatusEvent {
  event: 'status';
  data: {
    agent: string;        // friendly name (e.g., "web-search", "support")
    agentPath: string;    // full path for debugging
    status: 'starting' | 'working' | 'completed' | 'failed';
    message?: string;     // "now" field or custom message
    done?: string;        // optional, if configured
    pending?: string;     // optional, if configured
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
  sessionId?: string;        // omit for new conversation
  format?: string;           // output format (default: 'markdown')
  agentId?: string;          // which agent (default from config)
}
```

**Session Storage**:
- In-memory Map with TTL (configurable, default 30 minutes)
- Session contains: conversation history, metadata, last activity timestamp
- Cleanup on interval

**Key Implementation Points**:
1. Extract and forward `task_status` as `status` events
2. Intercept `onOutput` for final report streaming only (not intermediate)
3. Use `onProgress` for agent lifecycle events
4. Buffer final report detection (wait for `agent__final_report` tool)

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
- [ ] Add session storage types

### Step 2: Embed Headend Core
- [ ] Create `src/headends/embed-headend.ts`
- [ ] Implement health endpoint
- [ ] Implement static file serving for JS library
- [ ] Implement basic POST /v1/chat with SSE response

### Step 3: Session Management
- [ ] In-memory session store with TTL
- [ ] Session creation on new conversation
- [ ] Session lookup and continuation
- [ ] Cleanup timer

### Step 4: Event Streaming
- [ ] Wire up `onProgress` → `status` events
- [ ] Wire up task_status tool → `status` events
- [ ] Detect and stream final report only (not intermediate content)
- [ ] Send `meta` event at start
- [ ] Send `done` event at completion

### Step 5: JavaScript Library
- [ ] Create `src/headends/embed-public-client.ts`
- [ ] Implement SSE parsing with fetch
- [ ] Implement configuration handling
- [ ] Implement callback dispatching
- [ ] Implement session management (client-side)
- [ ] Build script to compile to `ai-agent-public.js`

### Step 6: Multi-turn Conversation
- [ ] Server: maintain conversation history in session
- [ ] Server: prepend history to new requests
- [ ] Client: track turns and apply markers
- [ ] Client: expose history API

### Step 7: Testing & Polish
- [ ] Unit tests for event parsing
- [ ] Integration test with support.ai
- [ ] Error handling edge cases
- [ ] Documentation

---

## Files to Create/Modify

### New Files
- `src/headends/embed-headend.ts` - Main headend implementation
- `src/headends/embed-types.ts` - Type definitions for SSE protocol
- `src/headends/embed-session-store.ts` - Session management
- `src/headends/embed-public-client.ts` - JavaScript library source

### Modified Files
- `src/headends/headend-manager.ts` - Register new headend type
- `src/agent-registry.ts` - May need updates for session spawning

### Build Artifacts
- `dist/ai-agent-public.js` - Compiled JS library (served by headend)

---

## Configuration

### Headend Configuration (in `.ai-agent.json`)
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

## Open Questions / Decisions Needed

1. **CORS policy**: Which origins should be allowed? (*.netdata.cloud + localhost for dev?)
2. **Rate limiting**: Should we add per-IP rate limiting for public endpoint?
3. **Authentication**: Is this fully public or should there be optional API key support?
4. **Metrics**: Should we emit Prometheus metrics for the embed headend?

---

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Large JS bundle size | Keep dependencies to zero, target <10KB minified |
| Session memory usage | TTL cleanup + max sessions limit |
| Final report detection timing | Use XML transport completion signal |
| Browser compatibility | Target ES2020, test in Chrome/Firefox/Safari |

---

## Success Criteria

1. Status updates appear within 500ms of agent activity
2. Final report streams smoothly without intermediate content
3. Multi-turn conversation works with clear visual separation
4. JS library < 10KB minified
5. Works on learn.netdata.cloud, www.netdata.cloud, and in-app

