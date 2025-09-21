import { randomUUID } from 'node:crypto';
import http from 'node:http';

import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { SSEServerTransport } from '@modelcontextprotocol/sdk/server/sse.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';
import { WebSocketServer } from 'ws';
import { z } from 'zod';

import type { AgentRegistry } from '../agent-registry.js';
import type { AIAgentCallbacks, LogEntry } from '../types.js';
import type { Headend, HeadendClosedEvent, HeadendContext, HeadendDescription } from './types.js';
import type { AuthInfo } from '@modelcontextprotocol/sdk/server/auth/types.js';

import { resolveToolName } from '../schema-adapters.js';

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
  private stopping = false;
  private closedSignaled = false;
  private readonly limiter?: ConcurrencyLimiter;

  public constructor(options: McpHeadendOptions) {
    this.registry = options.registry;
    this.instructions = options.instructions;
    this.transportSpec = options.transport;
    this.id = this.computeId(options.transport);
    this.closed = this.closeDeferred.promise;
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
          try { await this.server.close(); } catch (err) { this.logCloseFailure('server', err); }
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

  private registerTools(server: McpServer): void {
    const metadataList = this.registry.list();
    const formatValues = ['markdown', 'markdown+mermaid', 'slack-block-kit', 'tty', 'pipe', 'json', 'sub-agent'] as const;
    const jsonSchemaRequired = 'payload.schema is required when format is "json"';

    metadataList.forEach((meta) => {
      const normalized = resolveToolName({
        id: meta.id,
        toolName: meta.toolName,
        description: meta.description,
        usage: meta.usage,
        input: meta.input,
        outputSchema: meta.outputSchema,
      });
      const description = meta.description ?? meta.usage ?? `Agent ${meta.id}`;
      const paramsShape = {
        format: z.enum(formatValues),
        payload: z.record(z.string(), z.unknown()).describe('Payload forwarded to the agent (must include prompt)').optional(),
      } as const;
      const paramsSchema = z.object(paramsShape);

      server.tool(normalized, description, paramsShape, async (rawArgs, extra) => {
        const parsed = paramsSchema.safeParse(rawArgs);
        if (!parsed.success) {
          const message = parsed.error.issues.map((issue) => issue.message).join('; ');
          throw new Error(message);
        }
        const { format, payload: payloadFromArgs } = parsed.data;

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
            const schemaCandidate = payloadFromArgs?.schema;
            const hasSchema = schemaCandidate !== undefined
              && schemaCandidate !== null
              && typeof schemaCandidate === 'object'
              && !Array.isArray(schemaCandidate);
            if (!hasSchema) {
              throw new Error(jsonSchemaRequired);
            }
          }

          const payload: Record<string, unknown> = payloadFromArgs !== undefined ? { ...payloadFromArgs } : {};
          payload.format = format;

          const session = await this.registry.spawnSession({
            agentId: meta.id,
            payload,
            callbacks,
            abortSignal,
            stopRef,
          });
          const result = await session.run();
          if (result.success) {
            const finalReport = result.finalReport;
            const structuredContent = finalReport?.content_json;
            const contentBlock = output.length > 0 ? [{ type: 'text' as const, text: output }] : [];
            return {
              content: contentBlock,
              ...(structuredContent !== undefined ? { structuredContent } : {}),
            };
          }
          const message = result.error ?? 'Agent execution failed';
          return {
            isError: true,
            content: [{ type: 'text' as const, text: message }],
          };
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          return {
            isError: true,
            content: [{ type: 'text' as const, text: message }],
          };
        }
      });
    });
  }

  private log(message: string, severity: LogEntry['severity'] = 'VRB', fatal = false): void {
    if (this.context === undefined) return;
    this.context.log({
      timestamp: Date.now(),
      severity,
      turn: 0,
      subturn: 0,
      direction: 'response',
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
      try { await ctx.server.close(); } catch (err) { this.logCloseFailure('server', err); }
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
      try { await ctx.server.close(); } catch (err) { this.logCloseFailure('server', err); }
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
          void serverInstance.close();
        };
        try {
          await transport.start();
          await serverInstance.connect(transport);
        } catch (err) {
          this.log(`ws connection failed: ${err instanceof Error ? err.message : String(err)}`, 'ERR');
          this.wsContexts.delete(sessionId);
          if (release !== undefined) release();
          await transport.close();
          await serverInstance.close();
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
      try { await ctx.server.close(); } catch (err) { this.logCloseFailure('server', err); }
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
    if (req.url !== '/mcp') {
      this.writeJsonError(res, 404, -32601, 'Not Found');
      return;
    }
    if (req.method !== 'POST') {
      res.statusCode = 405;
      res.setHeader('Allow', 'POST');
      res.end('Method Not Allowed');
      return;
    }

    const sessionHeader = this.extractSessionId(req.headers['mcp-session-id']);
    const limiter = this.limiter;
    const isExistingSession = typeof sessionHeader === 'string' && sessionHeader.length > 0;
    let release: (() => void) | undefined;
    let releaseAttached = false;
    let closeListener: (() => void) | undefined;
    const abortController = new AbortController();
    if (!isExistingSession && limiter !== undefined) {
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
        this.writeJsonError(res, 503, -32001, 'concurrency_unavailable');
        return;
      }
    }

    let parsedBody: unknown;
    try {
      const rawBody = await this.readRequestBody(req);
      parsedBody = rawBody.length > 0 ? JSON.parse(rawBody) : undefined;
    } catch (err) {
      if (closeListener !== undefined) req.removeListener('close', closeListener);
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
      if (!releaseAttached && release !== undefined) release();
      const message = err instanceof Error ? err.message : String(err);
      this.log(`failed to parse MCP request body: ${message}`, 'ERR', true);
      this.writeJsonError(res, 400, -32700, 'Invalid JSON');
      return;
    }

    try {
      if (isExistingSession) {
        const ctx = this.httpContexts.get(sessionHeader);
        if (ctx === undefined) {
          this.writeJsonError(res, 404, -32000, 'Unknown MCP session');
          // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
          if (!releaseAttached && release !== undefined) release();
          if (closeListener !== undefined) req.removeListener('close', closeListener);
          return;
        }
        await ctx.transport.handleRequest(req as http.IncomingMessage & { auth?: AuthInfo }, res, parsedBody);
        if (closeListener !== undefined) req.removeListener('close', closeListener);
        return;
      }

      const serverInstance = this.createServerInstance();
        const transport = new StreamableHTTPServerTransport({
          sessionIdGenerator: () => randomUUID(),
          enableJsonResponse: true,
          onsessioninitialized: (sessionId) => {
            this.httpContexts.set(sessionId, { transport, server: serverInstance, release });
            releaseAttached = true;
        },
        onsessionclosed: (sessionId) => {
          const ctx = this.httpContexts.get(sessionId);
          if (ctx !== undefined) {
            const releaseFn = ctx.release;
            if (typeof releaseFn === 'function') {
              releaseFn();
            }
            this.httpContexts.delete(sessionId);
            void ctx.server.close();
          }
        },
      });
      transport.onerror = (err) => { this.log(`transport error: ${err instanceof Error ? err.message : String(err)}`, 'ERR', true); };
      transport.onclose = () => {
        const sessionId = transport.sessionId;
        if (typeof sessionId === 'string') {
          const ctx = this.httpContexts.get(sessionId);
          const releaseFn = ctx?.release;
          if (typeof releaseFn === 'function') {
            try { releaseFn(); } catch { /* ignore */ }
          }
          this.httpContexts.delete(sessionId);
        }
        void serverInstance.close();
      };
      await serverInstance.connect(transport);
      await transport.handleRequest(req as http.IncomingMessage & { auth?: AuthInfo }, res, parsedBody);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`streamable http request failed: ${message}`, 'ERR', true);
      this.writeJsonError(res, 500, -32603, message);
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
      if (!releaseAttached && release !== undefined) release();
    }
    if (closeListener !== undefined) req.removeListener('close', closeListener);
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
        void serverInstance.close();
      };
      try {
        await transport.start();
        await serverInstance.connect(transport);
      } catch (err) {
        this.sseContexts.delete(sessionId);
        if (release !== undefined) release();
        this.log(`sse connection failed: ${err instanceof Error ? err.message : String(err)}`, 'ERR');
        try { await transport.close(); } catch { /* ignore */ }
        await serverInstance.close();
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
