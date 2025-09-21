import crypto from 'node:crypto';
import http from 'node:http';
import { URL } from 'node:url';

import type { AgentRegistry } from '../agent-registry.js';
import type { AIAgentCallbacks, LogEntry } from '../types.js';
import type { Headend, HeadendClosedEvent, HeadendContext, HeadendDescription } from './types.js';

import { ConcurrencyLimiter } from './concurrency.js';
import { HttpError, writeJson } from './http-utils.js';

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

const buildLog = (
  message: string,
  severity: LogEntry['severity'] = 'VRB',
  fatal = false,
  direction: LogEntry['direction'] = 'response'
): LogEntry => ({
  timestamp: Date.now(),
  severity,
  turn: 0,
  subturn: 0,
  direction,
  type: 'tool',
  remoteIdentifier: 'headend:api',
  fatal,
  message,
});

interface RestHeadendOptions {
  port: number;
  concurrency?: number;
}

interface HealthResponse {
  status: 'ok';
}

interface AgentSuccessResponse {
  success: true;
  output: string;
  finalReport: unknown;
  error?: undefined;
}

interface AgentErrorResponse {
  success: false;
  output: string;
  finalReport: unknown;
  error: unknown;
}

export class RestHeadend implements Headend {
  public readonly id: string;
  public readonly kind = 'api';
  public readonly closed: Promise<HeadendClosedEvent>;

  private readonly registry: AgentRegistry;
  private readonly options: RestHeadendOptions;
  private readonly closeDeferred = createDeferred<HeadendClosedEvent>();
  private stopping = false;
  private closedSignaled = false;
  private server?: http.Server;
  private context?: HeadendContext;
  private readonly limiter: ConcurrencyLimiter;

  public constructor(registry: AgentRegistry, opts: RestHeadendOptions) {
    this.registry = registry;
    this.options = opts;
    this.id = `api:${String(opts.port)}`;
    this.closed = this.closeDeferred.promise;
    const limit = typeof opts.concurrency === 'number' && Number.isFinite(opts.concurrency) && opts.concurrency > 0
      ? Math.floor(opts.concurrency)
      : 4;
    this.limiter = new ConcurrencyLimiter(limit);
  }

  public describe(): HeadendDescription {
    return { id: this.id, kind: this.kind, label: `REST API ${String(this.options.port)}` };
  }

  public async start(context: HeadendContext): Promise<void> {
    if (this.server !== undefined) return;
    this.context = context;

    const server = http.createServer((req, res) => {
      void this.handleRequest(req, res);
    });
    this.server = server;

    server.on('error', (err: unknown) => {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`server error: ${message}`, 'ERR', true);
      if (!this.stopping) this.stopping = true;
      this.signalClosed({ reason: 'error', error: err instanceof Error ? err : new Error(message) });
    });

    server.on('close', () => {
      if (!this.stopping) this.stopping = true;
      this.signalClosed({ reason: 'stopped', graceful: true });
    });

    await new Promise<void>((resolve, reject) => {
      const onError = (err: unknown) => {
        server.off('listening', onListening);
        reject(err instanceof Error ? err : new Error(String(err)));
      };
      const onListening = () => {
        server.off('error', onError);
        resolve();
      };
      server.once('error', onError);
      server.once('listening', onListening);
      server.listen(this.options.port);
    });
    this.log(`listening on port ${String(this.options.port)}`);
  }

  public async stop(): Promise<void> {
    if (this.server === undefined) {
      if (!this.stopping) {
        this.stopping = true;
        this.closeDeferred.resolve({ reason: 'stopped', graceful: true });
      }
      return;
    }
    this.stopping = true;
    await new Promise<void>((resolve) => {
      this.server?.close(() => {
        resolve();
      });
    });
    this.server = undefined;
    this.signalClosed({ reason: 'stopped', graceful: true });
  }

  private async handleRequest(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
    const requestId = crypto.randomUUID();
    try {
      const urlRaw = req.url ?? '/';
      const parsedUrl = new URL(urlRaw, `http://localhost:${String(this.options.port)}`);
      const pathname = parsedUrl.pathname.replace(/\/+/g, '/');
      const segments = pathname.split('/').filter((segment) => segment.length > 0);
      if (req.method !== 'GET') {
        writeJson(res, 405, { success: false, output: '', finalReport: undefined, error: 'method_not_allowed' });
        return;
      }
      if (segments.length === 1 && segments[0] === 'health') {
        const payload: HealthResponse = { status: 'ok' };
        writeJson(res, 200, payload);
        return;
      }
      if (segments.length === 2 && segments[0] === 'v1') {
        const agentId = segments[1];
        await this.handleAgentRequest(agentId, parsedUrl, req, res, requestId);
        return;
      }
      writeJson(res, 404, { success: false, output: '', finalReport: undefined, error: 'not_found' });
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`handler failure ${requestId}: ${message}`, 'ERR', true);
      if (err instanceof HttpError) {
        writeJson(res, err.statusCode, { success: false, output: '', finalReport: undefined, error: err.code });
        return;
      }
      writeJson(res, 500, { success: false, output: '', finalReport: undefined, error: 'internal_error' });
    }
  }

  private async handleAgentRequest(agentId: string, url: URL, req: http.IncomingMessage, res: http.ServerResponse, requestId: string): Promise<void> {
    this.log(`request ${requestId} agent=${agentId}`, 'VRB', false, 'request');
    if (!this.registry.has(agentId)) {
      writeJson(res, 404, { success: false, output: '', finalReport: undefined, error: `Agent '${agentId}' not registered` });
      return;
    }

    const prompt = url.searchParams.get('q');
    const formatParam = url.searchParams.get('format');
    if (prompt === null || prompt.trim().length === 0) {
      writeJson(res, 400, { success: false, output: '', finalReport: undefined, error: 'Query parameter q is required' });
      return;
    }

    const release = await this.limiter.acquire();
    const abortController = new AbortController();
    const stopRef = { stopping: false };
    const onAbort = () => {
      if (abortController.signal.aborted) return;
      stopRef.stopping = true;
      abortController.abort();
      this.log(`request ${requestId} agent=${agentId} aborted by client`, 'WRN');
    };
    req.on('aborted', onAbort);
    res.on('close', onAbort);

    const cleanup = () => {
      req.removeListener('aborted', onAbort);
      res.removeListener('close', onAbort);
    };

    let output = '';
    const callbacks: AIAgentCallbacks = {
      onOutput: (chunk) => {
        output += chunk;
      },
      onLog: (entry) => {
        this.logEntry(entry);
      },
    };

    try {
      const session = await this.registry.spawnSession({
        agentId,
        payload: { prompt, format: formatParam ?? undefined },
        callbacks,
        abortSignal: abortController.signal,
        stopRef,
      });
      const result = await session.run();
      if (abortController.signal.aborted || res.writableEnded || res.writableFinished) return;
      if (result.success) {
        const payload: AgentSuccessResponse = {
          success: true,
          output,
          finalReport: result.finalReport,
        };
        writeJson(res, 200, payload);
        this.log(`response ${requestId} status=ok`);
      } else {
        const payload: AgentErrorResponse = {
          success: false,
          output,
          finalReport: result.finalReport,
          error: result.error ?? 'session_failed',
        };
        writeJson(res, 500, payload);
        this.log(`response ${requestId} status=error`, 'WRN');
      }
    } catch (err: unknown) {
      if (abortController.signal.aborted || res.writableEnded || res.writableFinished) return;
      const message = err instanceof Error ? err.message : String(err);
      this.log(`session error ${requestId}: ${message}`, 'ERR', true);
      const payload: AgentErrorResponse = {
        success: false,
        output: '',
        finalReport: undefined,
        error: message,
      };
      writeJson(res, 500, payload);
    } finally {
      release();
      cleanup();
    }
  }

  private log(
    message: string,
    severity: LogEntry['severity'] = 'VRB',
    fatal = false,
    direction: LogEntry['direction'] = 'response'
  ): void {
    if (this.context === undefined) return;
    this.context.log(buildLog(message, severity, fatal, direction));
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
}
