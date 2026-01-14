# REST API Headend

## TL;DR
Simple HTTP REST API exposing registered agents via GET /v1/{agentId} with query parameter input, concurrency limiting, and extensible route registration.

## Source Files
- `src/headends/rest-headend.ts` - Full implementation (466 lines)
- `src/headends/concurrency.ts` - ConcurrencyLimiter
- `src/headends/http-utils.ts` - HTTP utilities
- `src/headends/types.ts` - Headend interfaces

## Headend Identity
- **ID**: `api:{port}` (e.g., `api:8080`)
- **Kind**: `api`
- **Label**: `REST API {port}`

## Configuration

```typescript
interface RestHeadendOptions {
  port: number;              // HTTP server port
  concurrency?: number;      // Max concurrent requests (default: 10)
}
```

## Construction

**Location**: `src/headends/rest-headend.ts:100-111`

```typescript
constructor(registry: AgentRegistry, opts: RestHeadendOptions) {
  this.registry = registry;
  this.options = opts;
  this.id = `api:${String(opts.port)}`;
  this.label = `REST API ${String(opts.port)}`;
  this.closed = this.closeDeferred.promise;
  const limit = opts.concurrency > 0 ? Math.floor(opts.concurrency) : 10;
  this.limiter = new ConcurrencyLimiter(limit);
  this.telemetryLabels = { ...getTelemetryLabels(), headend: this.id };
}
```

## Startup Flow

**Location**: `src/headends/rest-headend.ts:126-175`

1. **Set context and signals**:
   ```typescript
   this.context = context;
   this.shutdownSignal = context.shutdownSignal;
   this.globalStopRef = context.stopRef;
   ```

2. **Create HTTP server**:
   ```typescript
   const server = http.createServer((req, res) => {
     void this.handleRequest(req, res);
   });
   ```

3. **Track sockets**:
   ```typescript
   server.on('connection', (socket) => {
     this.sockets.add(socket);
     socket.on('close', () => {
       this.sockets.delete(socket);
     });
   });
   ```

4. **Attach shutdown handler**:
   ```typescript
   shutdownSignal.addEventListener('abort', handler);
   ```

5. **Handle server events**:
   - `error`: Log and signal closed with error
   - `close`: Signal closed gracefully

6. **Start listening**:
   ```typescript
   await new Promise<void>((resolve, reject) => {
     server.listen(this.options.port);
   });
   ```

## Request Routing

**Location**: `src/headends/rest-headend.ts:200-240`

```typescript
handleRequest(req, res): Promise<void> {
  const requestId = crypto.randomUUID();
  const url = new URL(req.url, `http://localhost:${port}`);
  const method = req.method.toUpperCase();
  const normalizedPath = this.normalizePath(url.pathname);

  // Check extra routes first
  const route = this.matchExtraRoute(method, normalizedPath);
  if (route !== undefined) {
    return this.handleExtraRoute(route, ...);
  }

  // Only GET allowed for built-in routes
  if (method !== 'GET') {
    return writeJson(res, 405, { error: 'method_not_allowed' });
  }

  // Health check
  if (path === '/health') {
    return writeJson(res, 200, { status: 'ok' });
  }

  // Agent execution
  if (segments[0] === 'v1' && segments.length === 2) {
    return this.handleAgentRequest(agentId, url, req, res, requestId);
  }

  return writeJson(res, 404, { error: 'not_found' });
}
```

## Built-in Routes

| Method | Path | Handler |
|--------|------|---------|
| GET | `/health` | Health check response |
| GET | `/v1/{agentId}` | Agent execution |

## Agent Request Handling

**Location**: `src/headends/rest-headend.ts:242-358`

1. **Validate agent exists**:
   ```typescript
   if (!this.registry.has(agentId)) {
     writeJson(res, 404, { error: `Agent '${agentId}' not registered` });
   }
   ```

2. **Extract parameters**:
   ```typescript
   const prompt = url.searchParams.get('q');
   const formatParam = url.searchParams.get('format');
   if (prompt === null || prompt.trim().length === 0) {
     writeJson(res, 400, { error: 'Query parameter q is required' });
   }
   ```

3. **Setup abort handling**:
   ```typescript
   const abortController = new AbortController();
   const stopRef = { stopping: globalStopRef?.stopping };
   req.on('aborted', onAbort);
   res.on('close', onAbort);
   shutdownSignal?.addEventListener('abort', onShutdown);
   ```

4. **Acquire concurrency slot**:
   ```typescript
   release = await this.limiter.acquire({ signal: abortController.signal });
   ```

5. **Setup callbacks**:
   ```typescript
   const handoffSessions = new Set<string>();
   const sessionKey = (meta: AIAgentEventMeta): string | undefined =>
     meta.sessionId ?? meta.callPath ?? meta.agentId;

   const callbacks: AIAgentEventCallbacks = {
     onEvent: (event, meta) => {
       if (event.type === 'output') {
         if (!meta.isMaster) return;
         if (meta.source === 'finalize' && meta.pendingHandoffCount > 0) return;
         output += event.text;
         return;
       }
       if (event.type === 'thinking') {
         if (!meta.isMaster) return;
         reasoningLog += event.text;
         return;
       }
       if (event.type === 'handoff') {
         const key = sessionKey(meta);
         if (key !== undefined) handoffSessions.add(key);
         return;
       }
       if (event.type === 'final_report') {
         if (!meta.isMaster || !meta.isFinal) return;
         const key = sessionKey(meta);
         if (key !== undefined && handoffSessions.has(key)) return;
         finalReport = event.report;
         return;
       }
       if (event.type === 'log') {
         event.entry.headendId = this.id;
         this.logEntry(event.entry);
       }
     },
   };
   ```

6. **Spawn and run session**:
   ```typescript
   const session = await this.registry.spawnSession({
     agentId,
     payload: { prompt, format: formatParam },
     callbacks,
     abortSignal: abortController.signal,
     stopRef,
     headendId: this.id,
     telemetryLabels: this.telemetryLabels,
     wantsProgressUpdates: true,
   });
   const result = await session.run();
   ```

7. **Return response**:
   ```typescript
   if (result.success) {
     writeJson(res, 200, {
       success: true,
       output,
       finalReport: result.finalReport,
       reasoning: reasoningLog.length > 0 ? reasoningLog : undefined,
     });
   } else {
     writeJson(res, 500, {
       success: false,
       output,
       finalReport: result.finalReport,
       error: result.error ?? 'session_failed',
       reasoning: reasoningLog.length > 0 ? reasoningLog : undefined,
     });
   }
   ```

8. **Cleanup**:
   ```typescript
   finally {
     release();
     cleanup();
   }
   ```

## Response Schemas

### Health Response
```typescript
interface HealthResponse {
  status: 'ok';
}
```

### Success Response
```typescript
interface AgentSuccessResponse {
  success: true;
  output: string;
  finalReport: unknown;
  reasoning?: string;
}
```

### Error Response
```typescript
interface AgentErrorResponse {
  success: false;
  output: string;
  finalReport: unknown;
  error: unknown;
  reasoning?: string;
}
```

## Extra Route Registration

**Location**: `src/headends/rest-headend.ts:117-124`

```typescript
registerRoute(route: RestExtraRoute): void {
  if (this.server !== undefined) {
    throw new Error('Cannot register REST route after server start');
  }
  const method = route.method.toUpperCase();
  const path = this.normalizePath(route.path);
  this.extraRoutes.push({ method, path, handler: route.handler, originalPath: route.path });
}
```

### RestExtraRoute
```typescript
interface RestExtraRoute {
  method: 'GET' | 'POST' | 'PUT' | 'DELETE';
  path: string;
  handler: (args: {
    req: http.IncomingMessage;
    res: http.ServerResponse;
    url: URL;
    requestId: string;
  }) => Promise<void>;
}
```

Extra routes also subject to concurrency limiting.

## Path Normalization

**Location**: `src/headends/rest-headend.ts:438-442`

```typescript
normalizePath(pathname: string): string {
  const cleaned = pathname.replace(/\\+/g, '/');
  if (cleaned === '' || cleaned === '/') return '/';
  return cleaned.endsWith('/') ? cleaned.slice(0, -1) : cleaned;
}
```

Removes trailing slashes, normalizes backslashes.

## Shutdown Handling

**Location**: `src/headends/rest-headend.ts:177-198`

```typescript
async stop(): Promise<void> {
  this.stopping = true;
  this.closeActiveSockets(true);
  await new Promise<void>((resolve) => {
    this.server?.close(() => { resolve(); });
  });
  this.server = undefined;
  this.shutdownSignal.removeEventListener('abort', this.shutdownListener);
  this.signalClosed({ reason: 'stopped', graceful: true });
}
```

### Socket Cleanup
**Location**: `src/headends/rest-headend.ts:421-430`

```typescript
closeActiveSockets(force = false): void {
  this.sockets.forEach((socket) => {
    try { socket.end(); } catch { /* ignore */ }
    setTimeout(() => {
      try { socket.destroy(); } catch { /* ignore */ }
    }, force ? 0 : 1000).unref();
  });
}
```

Graceful end then destroy after timeout.

## Concurrency Control

- Default limit: 10 concurrent requests
- Per-request slot acquisition with abort signal
- Automatic release on completion/error
- 503 response when limit reached

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `port` | HTTP server listen port |
| `concurrency` | Max concurrent requests |
| `registry` | Available agents |

## Telemetry

**Labels added**:
- `headend: api:{port}`
- All base telemetry labels

## Logging

**Via context.log**:
- `VRB`: Starting, listening, started
- `VRB`: Request received, response sent
- `WRN`: Client abort
- `ERR`: Handler failures (fatal=true)

**Log format**:
```typescript
{
  timestamp: Date.now(),
  severity: 'VRB',
  turn: 0,
  subturn: 0,
  direction: 'response',
  type: 'tool',
  remoteIdentifier: 'headend:api',
  fatal: false,
  message: `REST API {port}: {message}`,
  headendId: this.id,
}
```

## Events

**Headend lifecycle**:
- `closed`: Promise resolving on termination
- Graceful vs error termination

## Invariants

1. **Port binding**: Server must bind successfully
2. **Route registration order**: Before server start only
3. **Concurrency enforcement**: All requests limited
4. **Graceful shutdown**: Sockets closed, server stopped
5. **Request ID tracking**: UUID per request
6. **Abort propagation**: Client disconnect stops session

## Business Logic Coverage (Verified 2025-11-16)

- **Format passthrough**: Query parameter `format=json` merely sets the session payload; no separate `schema` query parameter exists, so JSON validation relies entirely on the agent’s own frontmatter schema (`src/headends/rest-headend.ts:242-314`).
- **Progress + thinking streams**: Headend wiring forwards `onEvent` output/thinking/log events into the HTTP response payload (fields `output`, `reasoning`, `finalReport`) so clients get both streamed text and isolated reports (`src/headends/rest-headend.ts:282-358`).
- **Concurrency limiter**: Every request acquires a slot from `ConcurrencyLimiter`; aborted clients release slots immediately so hung HTTP clients cannot starve the process (`src/headends/rest-headend.ts:214-234`).
- **Extra routes**: `registerRoute()` lets plugins add custom handlers before `start()` (e.g., Slack slash commands); REST headend checks these routes before built-ins, enabling extensions without forking core logic (`src/headends/rest-headend.ts:176-208`).
- **Unauthenticated surface**: The headend does not inspect `Authorization` headers; deployments must place it behind an API gateway or restrict network access if authentication is required (`src/headends/rest-headend.ts`).

## Test Coverage

**Phase 2**:
- Health check endpoint
- Agent execution (success/error)
- Extra route registration
- Concurrency limiting
- Shutdown procedures
- Path normalization

**Gaps**:
- Large payload handling
- Connection timeout scenarios
- Malformed request recovery
- Socket leak detection

## Troubleshooting

### Port already in use
- Check port availability
- Verify no other instances
- Review server error logs

### Agent not found
- Check registry.has(agentId)
- Verify agent registration
- Review agent ID spelling

### Concurrency blocked
- Check limiter configuration
- Verify release calls
- Review active connections

### Client abort not detected
- Check abort listeners attached
- Verify abortController propagation
- Review cleanup calls

### Extra routes not working
- Verify registered before start()
- Check method and path matching
- Review handler errors
