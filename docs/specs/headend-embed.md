# Embed Headend

## TL;DR
Browser-friendly SSE headend with a public JS client. Streams only the final report content, plus lightweight status updates. Supports stable client UUIDs and end-user transcript persistence in `.json.gz` files.

## Source Files
- `src/headends/embed-headend.ts` - HTTP + SSE headend
- `src/headends/embed-public-client.ts` - Browser client served as `/ai-agent-public.js`
- `src/headends/embed-transcripts.ts` - Transcript persistence helpers
- `src/headends/embed-metrics.ts` - Metrics exporter
- `src/headends/http-utils.ts` - SSE helpers (`writeSseEvent`)
- `src/xml-transport.ts` + `src/session-turn-runner.ts` - strict final-report streaming for `renderTarget='embed'`

## Endpoints

### `GET /health`
```json
{ "status": "ok" }
```

### `GET /ai-agent-public.js`
Serves the public JS client (cacheable).

### `POST /v1/chat` (SSE)
**Request**
```json
{
  "message": "How do I configure alerts?",
  "clientId": "optional-uuid",
  "history": [
    { "role": "user", "content": "Previous question" },
    { "role": "assistant", "content": "Previous answer" }
  ],
  "format": "markdown",
  "agentId": "support.ai"
}
```

**Headers**
- `Origin` (browser): validated against `embed.corsOrigins` or auth tier origins.
- `X-Netdata-Agent-GUID`: required when `auth.tiers.agentDashboards.requireGuid=true`.
- `Authorization: Bearer <token>`: required when `auth.signedTokens.enabled=true` (static secret).

**Notes**
- `history` is used only for prompt context (stateless server).
- `clientId` is stable across follow-ups; server generates one if omitted.

**Response (SSE events)**
- `event: client` → `{ clientId, isNew }`
- `event: meta` → `{ sessionId, turn, agentId }`
- `event: status` → `{ agent, agentPath, status, message, done, pending, now, timestamp }`
- `event: report` → `{ chunk, index }` (final report only)
- `event: done` → `{ success, metrics, reportLength }`
- `event: error` → `{ code, message, recoverable }`

### `GET /metrics` (optional)
Exposes Prometheus text format when enabled (path configurable).

Metrics:
- `embed_requests_total{agent,origin,status}`
- `embed_sessions_active`
- `embed_session_duration_seconds_sum|_count`
- `embed_report_chunks_total`
- `embed_errors_total{code}`

## Final-Report Streaming
- `renderTarget='embed'` uses a strict XML streaming filter.
- Only content inside `<ai-agent-*-FINAL>` is emitted in `report` events.
- If streaming yields no chunks, the headend emits one final `report` event using `finalReport`.

## Status Updates
`agent__task_status` updates flow into `agent_update` events and expose structured fields:
```ts
{
  status: "starting" | "in-progress" | "completed",
  done: string,
  pending: string,
  now: string,
  ready_for_final_report: boolean,
  need_to_run_more_tools: boolean
}
```

## Transcript Persistence
Stored under `sessionsDir/embed-conversations/{clientId}.json.gz`:
```json
{
  "version": 1,
  "clientId": "uuid",
  "origin": "embed",
  "updatedAt": "2026-01-12T12:34:56.789Z",
  "turns": [
    {
      "turn": 1,
      "ts": "2026-01-12T12:30:00.000Z",
      "entries": [
        { "role": "user", "content": "Question..." },
        { "role": "status", "content": "in-progress | ..."},
        { "role": "assistant", "content": "Answer..." }
      ]
    }
  ]
}
```

## Configuration (`.ai-agent.json`)
```json
{
  "embed": {
    "defaultAgent": "support.ai",
    "corsOrigins": ["*.netdata.cloud", "localhost:*"],
    "rateLimit": {
      "enabled": true,
      "requestsPerMinute": 10,
      "burstSize": 5
    },
    "auth": {
      "tiers": {
        "netdataProperties": {
          "origins": ["*.netdata.cloud"],
          "rateLimit": { "requestsPerMinute": 60 }
        },
        "agentDashboards": {
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
        "secret": "shared-secret",
        "ttlSeconds": 3600
      }
    },
    "metrics": {
      "enabled": true,
      "path": "/metrics"
    }
  }
}
```

**CLI enablement**: `--embed <port>` (repeatable). Concurrency: `--embed-concurrency <n>`.
