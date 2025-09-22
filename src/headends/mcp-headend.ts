import { randomUUID } from 'node:crypto';
import http from 'node:http';
import { createRequire } from 'node:module';

import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { SSEServerTransport } from '@modelcontextprotocol/sdk/server/sse.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';
import { WebSocketServer } from 'ws';

import type { AgentMetadata, AgentRegistry } from '../agent-registry.js';
import type { AIAgentCallbacks, LogEntry } from '../types.js';
import type { Headend, HeadendClosedEvent, HeadendContext, HeadendDescription } from './types.js';
import type { AuthInfo } from '@modelcontextprotocol/sdk/server/auth/types.js';

/* eslint-disable-next-line @typescript-eslint/consistent-type-imports -- Need runtime type mapping for SDK's bundled Zod */
type ZodExports = typeof import('zod');

import { describeFormat } from '../formats.js';
import { resolveToolName, type AgentSchemaSummary } from '../schema-adapters.js';

import { ConcurrencyLimiter } from './concurrency.js';
import { McpWebSocketServerTransport } from './mcp-ws-transport.js';

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason?: unknown) => void;
}

const createDeferred = <T>(): Deferred<T> => {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
};

const STREAMABLE_HTTP = 'streamable-http' as const;
const require = createRequire(import.meta.url);
const sdkPackagePath = require.resolve('@modelcontextprotocol/sdk/package.json');
const sdkRequire = createRequire(sdkPackagePath);
const { z: sdkZ } = sdkRequire('zod') as ZodExports;
const MCP_SESSION_HEADER = 'mcp-session-id' as const;

const isPlainObject = (value: unknown): value is Record<string, unknown> => (
  value !== null && typeof value === 'object' && !Array.isArray(value)
);

const isSimplePromptSchema = (schema: Record<string, unknown>): boolean => {
  if (schema.type !== 'object') return false;
  const properties = isPlainObject(schema.properties) ? schema.properties : undefined;
  if (properties === undefined) return false;
  if (!('prompt' in properties)) return false;
  const extraKeys = Object.keys(properties).filter((key) => key !== 'prompt' && key !== 'format');
  if (extraKeys.length > 0) return false;
  const prompt = properties.prompt;
  if (!isPlainObject(prompt)) return false;
  if (prompt.type !== undefined && prompt.type !== 'string') return false;
  return true;
};

export type McpTransportSpec =
  | { type: 'stdio' }
  | { type: typeof STREAMABLE_HTTP; port: number }
  | { type: 'sse'; port: number }
  | { type: 'ws'; port: number };

interface McpHeadendOptions {
  registry: AgentRegistry;
  instructions?: string;
  transport: McpTransportSpec;
  concurrency?: number;
  verboseLogging?: boolean;
}

export class McpHeadend implements Headend {
  public readonly kind = 'mcp' as const;
  public readonly id: string;
  public readonly closed: Promise<HeadendClosedEvent>;

  private readonly registry: AgentRegistry;
  private readonly instructions?: string;
  private readonly transportSpec: McpTransportSpec;
  private readonly closeDeferred = createDeferred<HeadendClosedEvent>();
  private server?: McpServer;
  private context?: HeadendContext;
  private stdioTransport?: StdioServerTransport;
  private httpServer?: http.Server;
  private readonly httpContexts = new Map<string, { transport: StreamableHTTPServerTransport; server: McpServer; release?: () => void }>();
  private sseServer?: http.Server;
  private readonly sseContexts = new Map<string, { transport: SSEServerTransport; server: McpServer; release?: () => void }>();
  private wsServer?: WebSocketServer;
  private readonly wsContexts = new Map<string, { transport: McpWebSocketServerTransport; server: McpServer; release?: () => void }>();
  private readonly closingServers = new WeakSet<McpServer>();
  private stopping = false;
  private closedSignaled = false;
  private readonly limiter?: ConcurrencyLimiter;
  private readonly verboseLogging: boolean;

  public constructor(options: McpHeadendOptions) {
    this.registry = options.registry;
    this.instructions = options.instructions;
    this.transportSpec = options.transport;
    this.id = this.computeId(options.transport);
    this.closed = this.closeDeferred.promise;
    this.verboseLogging = options.verboseLogging === true;
    if (options.transport.type !== 'stdio') {
      const limit = typeof options.concurrency === 'number' && Number.isFinite(options.concurrency) && options.concurrency > 0
        ? Math.floor(options.concurrency)
        : 10;
      this.limiter = new ConcurrencyLimiter(limit);
    }
  }

  public describe(): HeadendDescription {
    return { id: this.id, kind: this.kind, label: this.describeLabel() };
  }

  public async start(context: HeadendContext): Promise<void> {
    if (this.context !== undefined) return;
    this.context = context;

    switch (this.transportSpec.type) {
      case 'stdio': {
        const server = this.createServerInstance();
        const transport = new StdioServerTransport();
        this.server = server;
        this.stdioTransport = transport;
        transport.onclose = () => { this.signalClosed({ reason: 'stopped', graceful: true }); };
        transport.onerror = (err) => { this.log(`transport error: ${err instanceof Error ? err.message : String(err)}`, 'ERR', true); };
        server.server.onclose = () => { this.signalClosed({ reason: 'stopped', graceful: true }); };
        server.server.onerror = (err) => { this.log(`server error: ${err.message}`, 'ERR', true); };
        await server.connect(transport);
        break;
      }
      case STREAMABLE_HTTP: {
        await this.startHttpTransport(this.transportSpec.port);
        break;
      }
      case 'sse': {
        await this.startSseTransport(this.transportSpec.port);
        break;
      }
      case 'ws': {
        this.startWsTransport(this.transportSpec.port);
        break;
      }
    }
    this.log('started');
  }

  public async stop(): Promise<void> {
    if (this.stopping) return;
    this.stopping = true;

    switch (this.transportSpec.type) {
      case 'stdio': {
        if (this.server !== undefined) {
          await this.closeServer(this.server);
        }
        if (this.stdioTransport !== undefined) {
          try { await this.stdioTransport.close(); } catch (err) { this.logCloseFailure('transport', err); }
        }
        this.signalClosed({ reason: 'stopped', graceful: true });
        return;
      }
      case STREAMABLE_HTTP:
        await this.stopHttpTransport();
        this.signalClosed({ reason: 'stopped', graceful: true });
        return;
      case 'sse':
        await this.stopSseTransport();
        this.signalClosed({ reason: 'stopped', graceful: true });
        return;
      case 'ws':
        await this.stopWsTransport();
        this.signalClosed({ reason: 'stopped', graceful: true });
        return;
      default:
        this.signalClosed({ reason: 'stopped', graceful: true });
    }
  }

  private computeId(spec: McpTransportSpec): string {
    switch (spec.type) {
      case 'stdio':
        return 'mcp:stdio';
      case STREAMABLE_HTTP:
        return `mcp:http:${String(spec.port)}`;
      case 'sse':
        return `mcp:sse:${String(spec.port)}`;
      case 'ws':
        return `mcp:ws:${String(spec.port)}`;
    }
  }

  private describeLabel(): string {
    switch (this.transportSpec.type) {
      case 'stdio':
        return 'MCP stdio headend';
      case STREAMABLE_HTTP:
        return `MCP HTTP headend (port ${String(this.transportSpec.port)})`;
      case 'sse':
        return `MCP SSE headend (port ${String(this.transportSpec.port)})`;
      case 'ws':
        return `MCP WS headend (port ${String(this.transportSpec.port)})`;
    }
  }

  private describeRemote(req: http.IncomingMessage): string {
    const address = req.socket.remoteAddress ?? 'unknown';
    const port = req.socket.remotePort;
    return port !== undefined ? `${address}:${String(port)}` : address;
  }

  private extractRpcMethods(payload: unknown): string[] {
    const gather = (message: unknown): string | undefined => {
      if (message === null || typeof message !== 'object') return undefined;
      const method = (message as { method?: unknown }).method;
      return typeof method === 'string' ? method : undefined;
    };
    if (Array.isArray(payload)) {
      return Array.from(new Set(payload.map(gather).filter((m): m is string => m !== undefined)));
    }
    const single = gather(payload);
    return single !== undefined ? [single] : [];
  }

  private async ensureSession(sessionId?: string): Promise<{ sessionId: string; context: { transport: StreamableHTTPServerTransport; server: McpServer } }> {
    if (sessionId !== undefined) {
      const existing = this.httpContexts.get(sessionId);
      if (existing !== undefined) return { sessionId, context: existing };
    }

    const serverInstance = this.createServerInstance();
    let resolvedSessionId = sessionId ?? randomUUID();
    let context: { transport: StreamableHTTPServerTransport; server: McpServer } | undefined;
    const transport = new StreamableHTTPServerTransport({
      sessionIdGenerator: () => resolvedSessionId,
      enableJsonResponse: true,
      onsessioninitialized: (sid) => {
        resolvedSessionId = sid;
        if (context !== undefined) this.httpContexts.set(sid, context);
      },
      onsessionclosed: (sid) => {
        this.httpContexts.delete(sid);
        void this.closeServer(serverInstance);
      }
    });
    context = { transport, server: serverInstance };

    await serverInstance.connect(transport);
    const internal = transport as unknown as { sessionId?: string; _initialized?: boolean };
    internal.sessionId = resolvedSessionId;
    internal._initialized = true;
    this.httpContexts.set(resolvedSessionId, context);
    return { sessionId: resolvedSessionId, context };
  }

  private async closeServer(server: McpServer): Promise<void> {
    if (this.closingServers.has(server)) return;
    this.closingServers.add(server);
    try {
      await server.close();
    } catch (error) {
      this.logCloseFailure('server', error);
    } finally {
      this.closingServers.delete(server);
    }
  }

  private registerTools(server: McpServer): void {
    const metadataList = this.registry.list();
    const formatValues = ['markdown', 'markdown+mermaid', 'slack-block-kit', 'tty', 'pipe', 'json', 'sub-agent'] as const;
    const jsonSchemaRequired = 'payload.schema is required when format is "json"';
    const usedNames = new Set<string>();

    metadataList.forEach((meta) => {
      const normalized = this.reserveToolName(meta, usedNames);
      const description = meta.description ?? meta.usage ?? `Agent ${meta.id}`;
      const promptDescription = meta.usage ?? 'User prompt for the agent';
      const formatDetails = formatValues
        .map((id) => `${id}: ${describeFormat(id)}`)
        .join('\n');
      const formatField = sdkZ.enum(formatValues).describe(`Output format to request from the agent. Choose one of:\n${formatDetails}`);

      const isPromptOnly = meta.input.format === 'text' || isSimplePromptSchema(meta.input.schema);

      const { paramsShape, paramsSchema } = (() => {
        const responseSchemaField = sdkZ
          .record(sdkZ.string(), sdkZ.unknown())
          .describe('Optional JSON schema describing the expected JSON response format')
          .optional();

        if (!isPromptOnly) {
          const payloadField = sdkZ
            .record(sdkZ.string(), sdkZ.unknown())
            .describe(`${promptDescription}. Provide the JSON payload matching the agent's input schema.`);
          const shape = {
            format: formatField,
            payload: payloadField,
            schema: responseSchemaField,
          } as const;
          return { paramsShape: shape, paramsSchema: sdkZ.object(shape) };
        }

        const promptField = sdkZ
          .string()
          .min(1, 'prompt is required')
          .describe(promptDescription);
        const extrasField = sdkZ
          .record(sdkZ.string(), sdkZ.unknown())
          .describe('Optional extra parameters forwarded unchanged to the agent')
          .optional();
        const shape = {
          format: formatField,
          prompt: promptField,
          payload: extrasField,
          schema: responseSchemaField,
        } as const;
        return { paramsShape: shape, paramsSchema: sdkZ.object(shape) };
      })();

      server.tool(normalized, description, paramsShape, async (rawArgs, extra) => {
        const requestId = randomUUID();
        const parsed = paramsSchema.safeParse(rawArgs);
        if (!parsed.success) {
          const message = parsed.error.issues.map((issue) => issue.message).join('; ');
          throw new Error(message);
        }
        const { format } = parsed.data as { format: typeof formatValues[number] };
        let rawPayload: Record<string, unknown> | undefined;
        if (isPromptOnly) {
          rawPayload = (parsed.data as { payload?: Record<string, unknown> }).payload;
        } else {
          rawPayload = (parsed.data as { payload: Record<string, unknown> }).payload;
        }
        const responseSchema = (parsed.data as { schema?: Record<string, unknown> }).schema;

        const abortSignal = extra.signal;
        const stopRef = { stopping: false };
        abortSignal.addEventListener('abort', () => { stopRef.stopping = true; }, { once: true });

        let output = '';
        const callbacks: AIAgentCallbacks = {
          onOutput: (chunk) => { output += chunk; },
          onLog: (entry) => { this.logEntry(entry); },
        };
        try {
          if (format === 'json') {
            const schemaCandidate = responseSchema ?? (rawPayload !== undefined ? (rawPayload as { schema?: unknown }).schema : undefined);
            const hasSchema = schemaCandidate !== undefined
              && schemaCandidate !== null
              && typeof schemaCandidate === 'object'
              && !Array.isArray(schemaCandidate);
            if (!hasSchema) {
              throw new Error(jsonSchemaRequired);
            }
          }

          let payload: Record<string, unknown>;
          if (isPromptOnly) {
            const extras = rawPayload !== undefined ? { ...rawPayload } : {};
            const promptValue = (parsed.data as unknown as { prompt: string }).prompt;
            extras.prompt = promptValue;
            payload = extras;
          } else {
            payload = { ...(rawPayload ?? {}) };
          }
          if (responseSchema !== undefined) {
            payload.schema = responseSchema;
          }
          payload.format = format;

          const session = await this.registry.spawnSession({
            agentId: meta.id,
            payload,
            callbacks,
            abortSignal,
            stopRef,
          });
          const result = await session.run();
          let text = output;
          const finalReport = result.finalReport;
          if (finalReport?.content !== undefined && typeof finalReport.content === 'string' && finalReport.content.trim().length > 0) {
            text = text.length > 0 ? `${text}\n\n${finalReport.content}` : finalReport.content;
          }
          if (finalReport?.content_json !== undefined) {
            try {
              const jsonText = JSON.stringify(finalReport.content_json, null, 2);
              if (jsonText.length > 0) {
                text = text.length > 0 ? `${text}\n\n${jsonText}` : jsonText;
              }
            } catch (jsonErr) {
              const jsonMessage: string = jsonErr instanceof Error ? jsonErr.message : String(jsonErr);
              const logMessage = ['response', requestId, 'tool', normalized, `json_stringify_failed:${jsonMessage}`].join(' ');
              this.logVerbose(logMessage, 'response', 'WRN');
            }
          }
          if (result.success) {
            const logMessage = ['response', requestId, 'tool', normalized, 'status=ok'].join(' ');
            this.logVerbose(logMessage, 'response');
            return {
              content: text.length > 0 ? [{ type: 'text' as const, text }] : [],
            };
          }
          const message: string = result.error ?? 'Agent execution failed';
          const logMessage = ['response', requestId, 'tool', normalized, `status=error`, `message:${message}`].join(' ');
          this.logVerbose(logMessage, 'response', 'ERR');
          const errorText: string = text.length > 0 ? text : message;
          return {
            isError: true,
            content: [{ type: 'text' as const, text: errorText }],
          };
        } catch (err) {
          const message: string = err instanceof Error ? err.message : String(err);
          const logMessage = ['response', requestId, 'tool', normalized, `status=error`, `message:${message}`].join(' ');
          this.logVerbose(logMessage, 'response', 'ERR');
          return {
            isError: true,
            content: [{ type: 'text' as const, text: message }],
          };
        }
      });
    });
  }

  private log(message: string, severity: LogEntry['severity'] = 'VRB', fatal = false, direction: LogEntry['direction'] = 'response'): void {
    if (this.context === undefined) return;
    this.context.log({
      timestamp: Date.now(),
      severity,
      turn: 0,
      subturn: 0,
      direction,
      type: 'tool',
      remoteIdentifier: 'headend:mcp',
      fatal,
      message,
      headendId: this.id,
    });
  }

  private logEntry(entry: LogEntry): void {
    if (this.context === undefined) return;
    this.context.log(entry);
  }

  private logVerbose(message: string, direction: LogEntry['direction'] = 'response', severity: LogEntry['severity'] = 'VRB'): void {
    if (!this.verboseLogging) return;
    this.log(message, severity, false, direction);
  }

  private logVerboseError(message: string): void {
    this.logVerbose(message, 'response', 'ERR');
  }

  private logHttpRequestVerbose(requestId: string, remote: string, ...parts: string[]): void {
    if (!this.verboseLogging) return;
    this.logVerbose(['http request', requestId, ...parts, `remote=${remote}`].join(' '), 'request');
  }

  private logHttpResponseVerbose(requestId: string, remote: string, severity: LogEntry['severity'], ...parts: string[]): void {
    this.logVerbose(['http response', requestId, ...parts, `remote=${remote}`].join(' '), 'response', severity);
  }

  private reserveToolName(meta: AgentMetadata, used: Set<string>): string {
    const base = this.buildToolNameBase(meta);
    return this.pickToolNameVariant(meta, base, 0, used);
  }

  private pickToolNameVariant(meta: AgentMetadata, base: string, attempt: number, used: Set<string>): string {
    const candidateBase = attempt === 0 ? base : `${base}-${String(attempt + 1)}`;
    const summary = {
      id: meta.id,
      toolName: candidateBase,
      description: meta.description,
      usage: meta.usage,
      input: meta.input,
      outputSchema: meta.outputSchema,
    } satisfies AgentSchemaSummary;
    const resolved = resolveToolName(summary);
    if (used.has(resolved)) {
      return this.pickToolNameVariant(meta, base, attempt + 1, used);
    }
    used.add(resolved);
    return resolved;
  }

  private buildToolNameBase(meta: AgentMetadata): string {
    const normalizeCandidate = (value: string | undefined): string | undefined => {
      if (value === undefined) return undefined;
      const trimmed = value.trim();
      if (trimmed.length === 0) return undefined;
      const parts = trimmed.split(/[\\/]/);
      const tail = parts[parts.length - 1] ?? trimmed;
      const withoutSuffix = tail.replace(/\.ai$/i, '').trim();
      return withoutSuffix.length > 0 ? withoutSuffix : undefined;
    };

    const candidates = [
      normalizeCandidate(typeof meta.toolName === 'string' ? meta.toolName : undefined),
      normalizeCandidate(typeof meta.promptPath === 'string' ? meta.promptPath : undefined),
      normalizeCandidate(meta.id),
    ].filter((value): value is string => value !== undefined);

    return candidates[0] ?? 'agent';
  }

  private signalClosed(event: HeadendClosedEvent): void {
    if (this.closedSignaled) return;
    this.closedSignaled = true;
    this.closeDeferred.resolve(event);
  }

  private createServerInstance(): McpServer {
    const server = new McpServer({ name: 'ai-agent-mcp-headend', version: '1.0.0' }, {
      instructions: this.instructions ?? 'AI Agent MCP headend. Use registered tools to invoke agents.',
      capabilities: { logging: {} },
    });
    this.registerTools(server);
    return server;
  }

  private async startHttpTransport(port: number): Promise<void> {
    if (this.httpServer !== undefined) return;

    const server = http.createServer((req, res) => {
      void this.handleHttpRequest(req, res);
    });

    this.httpServer = server;

    server.on('error', (err) => {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`streamable http server error: ${message}`, 'ERR', true);
      this.signalClosed({ reason: 'error', error: err instanceof Error ? err : new Error(message) });
    });

    await new Promise<void>((resolve, reject) => {
      server.listen(port, () => { resolve(); });
      server.once('error', (err) => { reject(err); });
    });
  }

  private async stopHttpTransport(): Promise<void> {
    const contexts = Array.from(this.httpContexts.values());
    this.httpContexts.clear();
    // eslint-disable-next-line functional/no-loop-statements
    for (const ctx of contexts) {
      try { ctx.release?.(); } catch (err) { this.logCloseFailure('transport', err); }
      await this.closeServer(ctx.server);
      try { await ctx.transport.close(); } catch (err) { this.logCloseFailure('transport', err); }
    }
    if (this.httpServer !== undefined) {
      await new Promise<void>((resolve) => {
        this.httpServer?.close(() => { resolve(); });
      });
      this.httpServer = undefined;
    }
  }

  private async startSseTransport(port: number): Promise<void> {
    if (this.sseServer !== undefined) return;
    const server = http.createServer((req, res) => {
      void this.handleSseRequest(req, res);
    });
    this.sseServer = server;
    server.on('error', (err) => {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`sse server error: ${message}`, 'ERR', true);
      this.signalClosed({ reason: 'error', error: err instanceof Error ? err : new Error(message) });
    });
    await new Promise<void>((resolve, reject) => {
      server.listen(port, () => { resolve(); });
      server.once('error', (err) => { reject(err); });
    });
  }

  private async stopSseTransport(): Promise<void> {
    const contexts = Array.from(this.sseContexts.values());
    this.sseContexts.clear();
    // eslint-disable-next-line functional/no-loop-statements
    for (const ctx of contexts) {
      try { ctx.release?.(); } catch (err) { this.logCloseFailure('transport', err); }
      try { await ctx.transport.close(); } catch (err) { this.logCloseFailure('transport', err); }
      await this.closeServer(ctx.server);
    }
    if (this.sseServer !== undefined) {
      await new Promise<void>((resolve) => { this.sseServer?.close(() => { resolve(); }); });
      this.sseServer = undefined;
    }
  }

  private startWsTransport(port: number): void {
    if (this.wsServer !== undefined) return;
    const server = new WebSocketServer({ port });
    this.wsServer = server;
    server.on('error', (err) => {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`ws server error: ${message}`, 'ERR', true);
      this.signalClosed({ reason: 'error', error: err instanceof Error ? err : new Error(message) });
    });
    server.on('connection', (socket) => {
      void (async () => {
        const limiter = this.limiter;
        let release: (() => void) | undefined;
        if (limiter !== undefined) {
          try {
            release = await limiter.acquire();
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err);
            this.log(`ws concurrency failure: ${message}`, 'ERR');
            try { socket.close(); } catch { /* ignore */ }
            return;
          }
        }
        const serverInstance = this.createServerInstance();
        const transport = new McpWebSocketServerTransport(socket);
        const sessionId = transport.sessionId;
        this.wsContexts.set(sessionId, { transport, server: serverInstance, release });
        transport.onerror = (err) => {
          this.log(`ws transport error: ${err instanceof Error ? err.message : String(err)}`, 'ERR');
        };
        transport.onclose = () => {
          const ctx = this.wsContexts.get(sessionId);
          const releaseFn = ctx?.release;
          if (typeof releaseFn === 'function') {
            try { releaseFn(); } catch { /* ignore */ }
          }
          this.wsContexts.delete(sessionId);
          void this.closeServer(serverInstance);
        };
        try {
          await transport.start();
          await serverInstance.connect(transport);
        } catch (err) {
          this.log(`ws connection failed: ${err instanceof Error ? err.message : String(err)}`, 'ERR');
          this.wsContexts.delete(sessionId);
          if (release !== undefined) release();
          await transport.close();
          await this.closeServer(serverInstance);
        }
      })();
    });
  }

  private async stopWsTransport(): Promise<void> {
    const contexts = Array.from(this.wsContexts.values());
    this.wsContexts.clear();
    // eslint-disable-next-line functional/no-loop-statements
    for (const ctx of contexts) {
      try { ctx.release?.(); } catch (err) { this.logCloseFailure('transport', err); }
      try { await ctx.transport.close(); } catch (err) { this.logCloseFailure('transport', err); }
      await this.closeServer(ctx.server);
    }
    if (this.wsServer !== undefined) {
      await new Promise<void>((resolve) => {
        this.wsServer?.close(() => { resolve(); });
      });
      this.wsServer = undefined;
    }
  }

  private extractSessionId(headerValue: string | string[] | undefined): string | undefined {
    if (headerValue === undefined) return undefined;
    if (Array.isArray(headerValue)) return headerValue[0];
    return headerValue;
  }

  private async readRequestBody(req: http.IncomingMessage): Promise<string> {
    const chunks: Buffer[] = [];
    return await new Promise<string>((resolve, reject) => {
      req.on('data', (chunk: Buffer | string) => {
        chunks.push(typeof chunk === 'string' ? Buffer.from(chunk) : chunk);
      });
      req.on('end', () => {
        resolve(Buffer.concat(chunks).toString('utf8'));
      });
      req.on('error', (err) => {
        reject(err instanceof Error ? err : new Error(String(err)));
      });
    });
  }

  private async handleHttpRequest(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
    const requestId = randomUUID();
    const method = req.method ?? 'GET';
    const url = req.url ?? '';
    const remote = this.describeRemote(req);
    this.logHttpRequestVerbose(requestId, remote, method, url);

    if (req.url !== '/mcp') {
      this.logHttpResponseVerbose(requestId, remote, 'ERR', 'status=404', `path=${url}`);
      this.writeJsonError(res, 404, -32601, 'Not Found');
      return;
    }

    if (method !== 'POST') {
      this.logHttpResponseVerbose(requestId, remote, 'ERR', 'status=405', `method=${method}`);
      res.statusCode = 405;
      res.setHeader('Allow', 'POST');
      res.end('Method Not Allowed');
      return;
    }

    let sessionId = this.extractSessionId(req.headers[MCP_SESSION_HEADER]);
    const limiter = this.limiter;
    let release: (() => void) | undefined;
    let closeListener: (() => void) | undefined;
    const abortController = new AbortController();
    if (limiter !== undefined) {
      closeListener = () => {
        if (!abortController.signal.aborted) abortController.abort();
      };
      req.on('close', closeListener);
      try {
        release = await limiter.acquire({ signal: abortController.signal });
      } catch (err) {
        req.removeListener('close', closeListener);
        if (abortController.signal.aborted) return;
        const message = err instanceof Error ? err.message : String(err);
        this.log(`streamable http concurrency failure: ${message}`, 'ERR');
        this.logHttpResponseVerbose(requestId, remote, 'ERR', 'status=503', 'concurrency_unavailable');
        this.writeJsonError(res, 503, -32001, 'concurrency_unavailable');
        return;
      }
    }

    let parsedBody: unknown;
    let methods: string[] = [];
    try {
      const rawBody = await this.readRequestBody(req);
      parsedBody = rawBody.length > 0 ? JSON.parse(rawBody) : undefined;
      methods = this.extractRpcMethods(parsedBody);
      if (methods.length > 0) {
        this.logHttpRequestVerbose(requestId, remote, `methods=${methods.join(',')}`);
      }
    } catch (err) {
      if (closeListener !== undefined) req.removeListener('close', closeListener);
      if (release !== undefined) release();
      const message = err instanceof Error ? err.message : String(err);
      this.log(`failed to parse MCP request body: ${message}`, 'ERR', true);
      this.logHttpResponseVerbose(requestId, remote, 'ERR', 'status=400', 'invalid_json');
      this.writeJsonError(res, 400, -32700, 'Invalid JSON');
      return;
    }

    const isInitializationRequest = methods.includes('initialize');
    let context: { transport: StreamableHTTPServerTransport; server: McpServer } | undefined = sessionId !== undefined ? this.httpContexts.get(sessionId) : undefined;

    if (!isInitializationRequest) {
      const ensured = await this.ensureSession(sessionId);
      sessionId = ensured.sessionId;
      context = ensured.context;
      (req.headers as NodeJS.Dict<string>)[MCP_SESSION_HEADER] = sessionId;
      res.setHeader(MCP_SESSION_HEADER, sessionId);
      this.logVerbose(['http session', requestId, `using=${sessionId}`, `remote=${remote}`].join(' '), 'request');
    }

    try {
      if (context !== undefined) {
        await context.transport.handleRequest(req as http.IncomingMessage & { auth?: AuthInfo }, res, parsedBody);
        return;
      }

      const serverInstance = this.createServerInstance();
      const transport = new StreamableHTTPServerTransport({
        sessionIdGenerator: () => randomUUID(),
        enableJsonResponse: true,
        onsessioninitialized: (sid) => {
          this.httpContexts.set(sid, { transport, server: serverInstance });
        },
        onsessionclosed: (sid) => {
          this.httpContexts.delete(sid);
          void this.closeServer(serverInstance);
        }
      });
      transport.onerror = (err) => { this.log(`transport error: ${err instanceof Error ? err.message : String(err)}`, 'ERR', true); };
      transport.onclose = () => {
        const sid = transport.sessionId;
        if (typeof sid === 'string') {
          this.httpContexts.delete(sid);
        }
        void this.closeServer(serverInstance);
      };
      await serverInstance.connect(transport);
      await transport.handleRequest(req as http.IncomingMessage & { auth?: AuthInfo }, res, parsedBody);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`streamable http request failed: ${message}`, 'ERR', true);
        this.logHttpResponseVerbose(requestId, remote, 'ERR', 'status=500', `message:${message}`);
      this.writeJsonError(res, 500, -32603, message);
    } finally {
      if (closeListener !== undefined) req.removeListener('close', closeListener);
      if (release !== undefined) release();
    }
  }

  private async handleSseRequest(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
    const base = `http://localhost:${String(this.transportSpec.type === 'sse' ? this.transportSpec.port : 0)}`;
    let parsedUrl: URL;
    try {
      parsedUrl = new URL(req.url ?? '/', base);
    } catch {
      res.statusCode = 400;
      res.end('Bad Request');
      return;
    }
    const path = parsedUrl.pathname;
    if (req.method === 'GET' && path === '/mcp/sse') {
      const limiter = this.limiter;
      let release: (() => void) | undefined;
      if (limiter !== undefined) {
        try {
          release = await limiter.acquire();
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          this.log(`sse concurrency failure: ${message}`, 'ERR');
          res.statusCode = 503;
          res.end('Too many concurrent sessions');
          return;
        }
      }
      const serverInstance = this.createServerInstance();
      const transport = new SSEServerTransport('/mcp/sse/message', res);
      const sessionId = transport.sessionId;
      this.sseContexts.set(sessionId, { transport, server: serverInstance, release });
      transport.onerror = (err) => {
        this.log(`sse transport error: ${err instanceof Error ? err.message : String(err)}`, 'ERR');
      };
      transport.onclose = () => {
        const ctx = this.sseContexts.get(sessionId);
        const releaseFn = ctx?.release;
        if (typeof releaseFn === 'function') {
          try { releaseFn(); } catch { /* ignore */ }
        }
        this.sseContexts.delete(sessionId);
        void this.closeServer(serverInstance);
      };
      try {
        await transport.start();
        await serverInstance.connect(transport);
      } catch (err) {
        this.sseContexts.delete(sessionId);
        if (release !== undefined) release();
        this.log(`sse connection failed: ${err instanceof Error ? err.message : String(err)}`, 'ERR');
        try { await transport.close(); } catch { /* ignore */ }
        await this.closeServer(serverInstance);
        if (!res.writableEnded && !res.writableFinished) {
          res.statusCode = 500;
          res.end('Failed to start SSE session');
        }
      }
      return;
    }
    if (req.method === 'POST' && path === '/mcp/sse/message') {
      const sessionId = parsedUrl.searchParams.get('sessionId');
      if (sessionId === null) {
        res.statusCode = 404;
        res.end('Unknown session');
        return;
      }
      const ctx = this.sseContexts.get(sessionId);
      if (ctx === undefined) {
        res.statusCode = 404;
        res.end('Unknown session');
        return;
      }
      try {
        await ctx.transport.handlePostMessage(req as http.IncomingMessage & { auth?: AuthInfo }, res);
      } catch (err) {
        this.log(`sse message failed: ${err instanceof Error ? err.message : String(err)}`, 'ERR');
        if (!res.headersSent) {
          res.statusCode = 500;
          res.end('Internal error');
        }
      }
      return;
    }
    res.statusCode = 404;
    res.end('Not Found');
  }

  private logCloseFailure(kind: 'server' | 'transport', err: unknown): void {
    const message = err instanceof Error ? err.message : String(err);
    this.log(`${kind} close failed: ${message}`, 'WRN');
  }

  private writeJsonError(res: http.ServerResponse, statusCode: number, code: number, message: string, id: unknown = null): void {
    if (res.headersSent) return;
    res.statusCode = statusCode;
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify({ jsonrpc: '2.0', error: { code, message }, id }));
  }
}
