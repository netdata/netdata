import crypto from 'node:crypto';
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import type { AgentRegistry } from '../agent-registry.js';
import type { OutputFormatId } from '../formats.js';
import type { AccountingEntry, AIAgentEvent, AIAgentEventCallbacks, AIAgentEventMeta, ConversationMessage, EmbedAuthConfig, EmbedAuthTierConfig, EmbedHeadendConfig, EmbedTierRateLimitConfig, FinalReportPayload, LogEntry, ProgressEvent } from '../types.js';
import type { Headend, HeadendClosedEvent, HeadendContext, HeadendDescription } from './types.js';
import type { Socket } from 'node:net';

import { AIAgent } from '../ai-agent.js';
import { resolvePeristenceConfig } from '../persistence.js';
import { getTelemetryLabels } from '../telemetry/index.js';
import { isPlainObject } from '../utils.js';

import { ConcurrencyLimiter } from './concurrency.js';
import { EmbedMetrics } from './embed-metrics.js';
import { appendTranscriptTurn, loadTranscript, writeTranscript, type EmbedTranscriptEntry } from './embed-transcripts.js';
import { HttpError, readJson, writeJson, writeSseEvent } from './http-utils.js';
import { createHeadendEventState, markHandoffSeen, shouldAcceptFinalReport, shouldStreamOutput, shouldStreamTurnStarted } from './shared-event-filter.js';

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

const CONTENT_TYPE_HEADER = 'Content-Type';
const EVENT_STREAM_CONTENT_TYPE = 'text/event-stream';
const JS_CONTENT_TYPE = 'application/javascript; charset=utf-8';
const METRICS_CONTENT_TYPE = 'text/plain; version=0.0.4; charset=utf-8';
const OUTPUT_FORMAT_VALUES: readonly OutputFormatId[] = ['markdown', 'markdown+mermaid', 'slack-block-kit', 'tty', 'pipe', 'json', 'sub-agent'] as const;

const isOutputFormatId = (value: string): value is OutputFormatId => OUTPUT_FORMAT_VALUES.includes(value as OutputFormatId);

const buildLog = (headendId: string, label: string, message: string, severity: LogEntry['severity'] = 'VRB', fatal = false): LogEntry => ({
  timestamp: Date.now(),
  severity,
  turn: 0,
  subturn: 0,
  direction: 'response',
  type: 'tool',
  remoteIdentifier: 'headend:embed',
  fatal,
  message: `${label}: ${message}`,
  headendId,
});

interface EmbedHistoryMessage {
  role: 'user' | 'assistant' | 'status';
  content: string;
}

interface EmbedChatRequest {
  message: string;
  clientId?: string;
  history?: EmbedHistoryMessage[];
  sessionId?: string;
  format?: string;
  agentId?: string;
}

interface EmbedHeadendOptions {
  port: number;
  concurrency?: number;
  config?: EmbedHeadendConfig;
}

interface AuthDecision {
  allowed: boolean;
  tier: 'netdata-properties' | 'agent-dashboards' | 'unknown';
  code?: string;
  message?: string;
}

interface RateLimitDecision {
  allowed: boolean;
  code?: string;
  message?: string;
}

interface RateLimiterEntry {
  count: number;
  windowStart: number;
}

class SimpleRateLimiter {
  private readonly max: number;
  private readonly windowMs = 60_000;
  private readonly entries = new Map<string, RateLimiterEntry>();

  public constructor(requestsPerMinute: number, burstSize = 0) {
    this.max = Math.max(1, Math.floor(requestsPerMinute) + Math.max(0, Math.floor(burstSize)));
  }

  public check(key: string, now: number): RateLimitDecision {
    const entry = this.entries.get(key);
    if (entry === undefined || now - entry.windowStart >= this.windowMs) {
      this.entries.set(key, { count: 1, windowStart: now });
      return { allowed: true };
    }
    if (entry.count >= this.max) {
      return { allowed: false, code: 'rate_limited', message: 'rate limit exceeded' };
    }
    entry.count += 1;
    this.entries.set(key, entry);
    return { allowed: true };
  }
}

interface TierLimiters {
  perOrigin?: SimpleRateLimiter;
  perGuid?: SimpleRateLimiter;
  perIp?: SimpleRateLimiter;
}

const buildTierLimiters = (config?: EmbedTierRateLimitConfig): TierLimiters => {
  if (config === undefined) return {};
  const perOrigin = typeof config.requestsPerMinute === 'number'
    ? new SimpleRateLimiter(config.requestsPerMinute)
    : undefined;
  const perGuid = typeof config.requestsPerMinutePerGuid === 'number'
    ? new SimpleRateLimiter(config.requestsPerMinutePerGuid)
    : undefined;
  const perIp = typeof config.requestsPerMinutePerIp === 'number'
    ? new SimpleRateLimiter(config.requestsPerMinutePerIp)
    : undefined;
  return { perOrigin, perGuid, perIp };
};

const normalizeOrigin = (origin: string): { origin: string; host: string } => {
  try {
    const parsed = new URL(origin);
    return { origin, host: parsed.host };
  } catch {
    return { origin, host: origin };
  }
};

const globToRegex = (pat: string): RegExp => {
  const escaped = pat.replace(/[.+^${}()|[\]\\]/g, '\\$&');
  const regex = `^${escaped.replace(/\*/g, '.*').replace(/\?/g, '.')}$`;
  return new RegExp(regex, 'i');
};

const matchOriginPatterns = (patterns: string[], origin: string): boolean => {
  if (patterns.length === 0) return false;
  const { origin: fullOrigin, host } = normalizeOrigin(origin);
  return patterns.some((pattern) => {
    const trimmed = pattern.trim();
    if (trimmed.length === 0) return false;
    const regex = globToRegex(trimmed);
    const candidate = trimmed.includes('://') ? fullOrigin : host;
    return regex.test(candidate);
  });
};

const getHeaderValue = (headers: http.IncomingHttpHeaders, name: string): string | undefined => {
  const value = headers[name.toLowerCase()];
  if (typeof value === 'string') return value;
  if (Array.isArray(value) && value.length > 0) return value[0];
  return undefined;
};

const isPromptHistoryEntry = (item: EmbedHistoryMessage): item is EmbedHistoryMessage & { role: 'user' | 'assistant' } => {
  return item.role === 'user' || item.role === 'assistant';
};

const parseHistoryMessages = (history: EmbedHistoryMessage[] | undefined): ConversationMessage[] => {
  if (!Array.isArray(history)) return [];
  return history
    .filter((item): item is EmbedHistoryMessage => isPlainObject(item))
    .filter(isPromptHistoryEntry)
    .map((item) => ({
      role: item.role,
      content: typeof item.content === 'string' ? item.content : '',
    }))
    .filter((item) => item.content.trim().length > 0);
};

const validateClientId = (value: string | undefined): string | undefined => {
  if (typeof value !== 'string') return undefined;
  const trimmed = value.trim();
  if (trimmed.length === 0) return undefined;
  const uuidPattern = /^[0-9a-fA-F-]{36}$/;
  return uuidPattern.test(trimmed) ? trimmed : undefined;
};

const collectUsage = (entries: AccountingEntry[]): { tokensIn: number; tokensOut: number; toolsRun: number } => {
  return entries.reduce(
    (acc, entry) => {
      if (entry.type === 'llm') {
        acc.tokensIn += entry.tokens.inputTokens;
        acc.tokensOut += entry.tokens.outputTokens;
      } else {
        acc.toolsRun += 1;
      }
      return acc;
    },
    { tokensIn: 0, tokensOut: 0, toolsRun: 0 }
  );
};

export class EmbedHeadend implements Headend {
  public readonly id: string;
  public readonly kind = 'embed';
  public readonly closed: Promise<HeadendClosedEvent>;

  private readonly registry: AgentRegistry;
  private readonly options: EmbedHeadendOptions;
  private readonly limiter: ConcurrencyLimiter;
  private readonly closeDeferred = createDeferred<HeadendClosedEvent>();
  private readonly label: string;
  private readonly config?: EmbedHeadendConfig;
  private readonly metrics: EmbedMetrics;
  private readonly tierLimiters: Record<AuthDecision['tier'], TierLimiters>;
  private readonly globalLimiter?: SimpleRateLimiter;
  private server?: http.Server;
  private context?: HeadendContext;
  private stopping = false;
  private closedSignaled = false;
  private shutdownSignal?: AbortSignal;
  private shutdownListener?: () => void;
  private globalStopRef?: { stopping: boolean };
  private readonly sockets = new Set<Socket>();
  private cachedClientScript?: { etag: string; body: Buffer; mtimeMs: number };
  private warnedGuidVerification = false;

  public constructor(registry: AgentRegistry, options: EmbedHeadendOptions) {
    this.registry = registry;
    this.options = options;
    this.config = options.config;
    this.id = `embed:${String(options.port)}`;
    this.label = `Embed headend (port ${String(options.port)})`;
    const limit = typeof options.concurrency === 'number' && Number.isFinite(options.concurrency) && options.concurrency > 0
      ? Math.floor(options.concurrency)
      : (typeof this.config?.concurrency === 'number' && this.config.concurrency > 0 ? Math.floor(this.config.concurrency) : 10);
    this.limiter = new ConcurrencyLimiter(limit);
    this.closed = this.closeDeferred.promise;
    this.metrics = new EmbedMetrics(this.config?.metrics);
    this.tierLimiters = {
      'netdata-properties': buildTierLimiters(this.config?.auth?.tiers?.netdataProperties?.rateLimit),
      'agent-dashboards': buildTierLimiters(this.config?.auth?.tiers?.agentDashboards?.rateLimit),
      unknown: buildTierLimiters(this.config?.auth?.tiers?.unknown?.rateLimit),
    };
    const globalRate = this.config?.rateLimit;
    if (globalRate?.enabled === true && typeof globalRate.requestsPerMinute === 'number') {
      this.globalLimiter = new SimpleRateLimiter(globalRate.requestsPerMinute, globalRate.burstSize ?? 0);
    }
  }

  public describe(): HeadendDescription {
    return { id: this.id, kind: this.kind, label: this.label };
  }

  public async start(context: HeadendContext): Promise<void> {
    if (this.server !== undefined) return;
    this.context = context;
    this.shutdownSignal = context.shutdownSignal;
    this.globalStopRef = context.stopRef;
    this.log('starting');
    const server = http.createServer((req, res) => { void this.handleRequest(req, res); });
    this.server = server;
    server.on('connection', (socket) => {
      this.sockets.add(socket);
      socket.on('close', () => { this.sockets.delete(socket); });
    });
    const handler = () => { this.handleShutdownSignal(); };
    context.shutdownSignal.addEventListener('abort', handler);
    this.shutdownListener = handler;
    server.on('error', (err) => {
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
      const onListening = () => {
        server.off('error', onError);
        resolve();
      };
      const onError = (err: unknown) => {
        server.off('listening', onListening);
        reject(err instanceof Error ? err : new Error(String(err)));
      };
      server.once('listening', onListening);
      server.once('error', onError);
      server.listen(this.options.port);
    });
    this.log('listening', 'VRB', false);
    this.log('started');
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
    this.closeActiveSockets(true);
    await new Promise<void>((resolve) => {
      this.server?.close(() => { resolve(); });
    });
    this.server = undefined;
    if (this.shutdownListener !== undefined && this.shutdownSignal !== undefined) {
      this.shutdownSignal.removeEventListener('abort', this.shutdownListener);
      this.shutdownListener = undefined;
    }
    this.signalClosed({ reason: 'stopped', graceful: true });
  }

  private async handleRequest(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
    const origin = getHeaderValue(req.headers, 'origin');
    const agentGuid = getHeaderValue(req.headers, 'x-netdata-agent-guid');
    const clientIp = this.resolveClientIp(req);
    const cors = this.resolveCors(origin, agentGuid);
    if (origin !== undefined && cors.allowed) {
      this.applyCorsHeaders(res, cors.allowOrigin);
    }
    const method = (req.method ?? 'GET').toUpperCase();
    if (method === 'OPTIONS') {
      if (!cors.allowed) {
        writeJson(res, 403, { error: 'cors_rejected' });
        return;
      }
      this.applyPreflightHeaders(res);
      res.statusCode = 204;
      res.end();
      return;
    }

    if (!cors.allowed) {
      writeJson(res, 403, { error: 'cors_rejected' });
      return;
    }

    const url = new URL(req.url ?? '/', `http://localhost:${String(this.options.port)}`);
    const path = url.pathname;

    if (method === 'GET' && path === '/health') {
      writeJson(res, 200, { status: 'ok' });
      return;
    }

    if (method === 'GET' && this.metrics.isEnabled() && path === this.metrics.getPath()) {
      res.statusCode = 200;
      res.setHeader(CONTENT_TYPE_HEADER, METRICS_CONTENT_TYPE);
      res.end(this.metrics.renderPrometheus());
      return;
    }

    if (method === 'GET' && path === '/ai-agent-public.js') {
      await this.handleClientScript(req, res);
      return;
    }

    if (method === 'POST' && path === '/v1/chat') {
      await this.handleChatRequest(req, res, origin, clientIp, agentGuid);
      return;
    }

    writeJson(res, 404, { error: 'not_found' });
  }

  private async handleChatRequest(req: http.IncomingMessage, res: http.ServerResponse, origin: string | undefined, clientIp: string, agentGuid: string | undefined): Promise<void> {
    const limiterRelease = await this.limiter.acquire({ signal: this.shutdownSignal }).catch(() => undefined);
    if (limiterRelease === undefined) {
      writeJson(res, 503, { error: 'concurrency_limit' });
      return;
    }

    const release = limiterRelease;
    const now = Date.now();
    let sessionActive = false;
    let sessionStartTs = now;
    try {
      if (this.globalStopRef?.stopping ?? false) {
        writeJson(res, 503, { error: 'server_stopping' });
        return;
      }
      const authHeader = getHeaderValue(req.headers, 'authorization');
      const authDecision = this.authorizeRequest(origin, agentGuid, authHeader);
      if (!authDecision.allowed) {
        this.metrics.recordRequest('unknown', origin ?? 'unknown', 'denied');
        this.metrics.recordError(authDecision.code ?? 'auth_denied');
        writeJson(res, 403, { error: authDecision.code ?? 'auth_denied', message: authDecision.message });
        return;
      }
      const rateDecision = this.checkRateLimit(authDecision.tier, origin, clientIp, agentGuid, now);
      if (!rateDecision.allowed) {
        this.metrics.recordRequest('unknown', origin ?? 'unknown', 'rate_limited');
        this.metrics.recordError(rateDecision.code ?? 'rate_limited');
        writeJson(res, 429, { error: rateDecision.code ?? 'rate_limited', message: rateDecision.message });
        return;
      }

      const body = await readJson<EmbedChatRequest>(req);
      const message = typeof body.message === 'string' ? body.message.trim() : '';
      if (message.length === 0) {
        throw new HttpError(400, 'invalid_message', 'message is required');
      }

      const requestedClientId = validateClientId(body.clientId);
      const clientId = requestedClientId ?? crypto.randomUUID();
      const isNewClient = requestedClientId === undefined;

      const agentId = this.resolveAgentId(body.agentId);
      if (agentId === undefined) {
        throw new HttpError(404, 'agent_not_found', 'agent not found');
      }

      const format = typeof body.format === 'string' && isOutputFormatId(body.format) ? body.format : undefined;
      const history = parseHistoryMessages(body.history);

      const telemetryLabels = { ...getTelemetryLabels(), headend: this.id };
      const stopRef = { stopping: this.globalStopRef?.stopping ?? false };
      const abortController = new AbortController();
      req.on('close', () => { abortController.abort(); });
      sessionStartTs = Date.now();
      sessionActive = true;
      this.metrics.recordSessionStart();

      res.writeHead(200, {
        [CONTENT_TYPE_HEADER]: EVENT_STREAM_CONTENT_TYPE,
        'Cache-Control': 'no-cache',
        Connection: 'keep-alive',
      });
      const flushHeaders = (res as http.ServerResponse & { flushHeaders?: () => void }).flushHeaders;
      if (typeof flushHeaders === 'function') flushHeaders.call(res);

      writeSseEvent(res, 'client', { clientId, isNew: isNewClient });

      let sessionId: string | undefined;
      let currentTurn: number | undefined;
      let lastMetaTurn: number | undefined;
      let reportIndex = 0;
      let reportBuffer = '';
      const statusEntries: EmbedTranscriptEntry[] = [];
      const accounting: AccountingEntry[] = [];
      const eventState = createHeadendEventState();
      let finalReportFromEvent: FinalReportPayload | undefined;

      const maybeEmitMeta = (): void => {
        if (sessionId === undefined || currentTurn === undefined) return;
        if (lastMetaTurn === currentTurn) return;
        lastMetaTurn = currentTurn;
        writeSseEvent(res, 'meta', { sessionId, turn: currentTurn, agentId });
      };

      const handleStatusEvent = (event: ProgressEvent): void => {
        if (event.type !== 'agent_update') return;
        if (event.taskStatus === undefined) return;
        const agentLabel = event.agentName ?? event.agentId;
        const statusPayload = {
          agent: agentLabel,
          agentPath: event.agentPath,
          status: event.taskStatus.status,
          message: event.message,
          done: event.taskStatus.done,
          pending: event.taskStatus.pending,
          now: event.taskStatus.now,
          timestamp: event.timestamp,
        };
        statusEntries.push({ role: 'status', content: event.message });
        writeSseEvent(res, 'status', statusPayload);
      };

      const callbacks: AIAgentEventCallbacks = {
        onEvent: (event: AIAgentEvent, meta: AIAgentEventMeta) => {
          switch (event.type) {
            case 'output': {
              if (!shouldStreamOutput(event, meta)) return;
              const chunk = event.text;
              if (chunk.length === 0) return;
              if (meta.sessionId !== undefined && sessionId === undefined) {
                sessionId = meta.sessionId;
                maybeEmitMeta();
              }
              reportBuffer += chunk;
              reportIndex += 1;
              this.metrics.recordReportChunk();
              writeSseEvent(res, 'report', { chunk, index: reportIndex });
              return;
            }
            case 'turn_started': {
              if (!shouldStreamTurnStarted(meta)) return;
              currentTurn = event.turn;
              maybeEmitMeta();
              return;
            }
            case 'progress': {
              const progressEvent = event.event;
              if (
                progressEvent.type === 'agent_update'
                || progressEvent.type === 'agent_started'
                || progressEvent.type === 'agent_finished'
                || progressEvent.type === 'agent_failed'
              ) {
                const txnId = typeof progressEvent.txnId === 'string' ? progressEvent.txnId : undefined;
                if (txnId !== undefined && sessionId === undefined) {
                  sessionId = txnId;
                  maybeEmitMeta();
                }
              }
              handleStatusEvent(progressEvent);
              return;
            }
            case 'status': {
              // avoid duplicate status handling (progress already includes agent_update)
              return;
            }
            case 'handoff': {
              markHandoffSeen(eventState, meta);
              return;
            }
            case 'final_report': {
              if (!shouldAcceptFinalReport(eventState, meta)) return;
              finalReportFromEvent = event.report;
              return;
            }
            case 'log': {
              const entry = event.entry;
              entry.headendId = this.id;
              this.logEntry(entry);
              return;
            }
            case 'accounting': {
              accounting.push(event.entry);
              return;
            }
            default: {
              return;
            }
          }
        },
      };

      const session = await this.registry.spawnSession({
        agentId,
        userPrompt: message,
        format,
        history,
        callbacks,
        abortSignal: abortController.signal,
        stopRef,
        headendId: this.id,
        telemetryLabels,
        wantsProgressUpdates: true,
        renderTarget: 'embed',
        stream: true,
      });

      const result = await AIAgent.run(session);
      const durationMs = Date.now() - sessionStartTs;
      this.metrics.recordSessionEnd(durationMs);
      sessionActive = false;

      const finalReport = finalReportFromEvent ?? result.finalReport;
      const finalText = (() => {
        if (finalReport === undefined) return '';
        if (finalReport.format === 'json' && finalReport.content_json !== undefined) {
          try { return JSON.stringify(finalReport.content_json); } catch { return ''; }
        }
        return typeof finalReport.content === 'string' ? finalReport.content : '';
      })();

      if (reportBuffer.length === 0 && finalText.length > 0) {
        reportBuffer = finalText;
        reportIndex += 1;
        this.metrics.recordReportChunk();
        writeSseEvent(res, 'report', { chunk: finalText, index: reportIndex });
      }

      const usage = collectUsage(accounting);
      const reportLength = reportBuffer.length > 0 ? reportBuffer.length : finalText.length;
      const success = result.success;
      const status = success ? 'ok' : 'failed';
      this.metrics.recordRequest(agentId, origin ?? 'unknown', status);
      if (!success) {
        this.metrics.recordError('session_failed');
      }

      const donePayload = {
        success,
        metrics: {
          durationMs,
          tokensIn: usage.tokensIn,
          tokensOut: usage.tokensOut,
          toolsRun: usage.toolsRun,
        },
        reportLength,
      };
      writeSseEvent(res, 'done', donePayload);
      res.end();

      await this.persistTranscript({
        clientId,
        agentId,
        message,
        statusEntries,
        report: reportBuffer.length > 0 ? reportBuffer : finalText,
      });
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err));
      const code = err instanceof HttpError ? err.code : 'server_error';
      const message = error.message;
      this.metrics.recordError(code);
      if (!res.writableEnded && !res.writableFinished) {
        if (res.getHeader(CONTENT_TYPE_HEADER) === EVENT_STREAM_CONTENT_TYPE) {
          writeSseEvent(res, 'error', { code, message, recoverable: false });
          res.end();
        } else {
          writeJson(res, err instanceof HttpError ? err.statusCode : 500, { error: code, message });
        }
      }
    } finally {
      if (sessionActive) {
        const durationMs = Date.now() - sessionStartTs;
        this.metrics.recordSessionEnd(durationMs);
      }
      release();
    }
  }

  private async handleClientScript(_req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
    const fileUrl = new URL('./embed-public-client.js', import.meta.url);
    const filePath = fileURLToPath(fileUrl);
    const stat = await fs.promises.stat(filePath);
    const cached = this.cachedClientScript;
    if (cached?.mtimeMs === stat.mtimeMs) {
      res.statusCode = 200;
      res.setHeader(CONTENT_TYPE_HEADER, JS_CONTENT_TYPE);
      res.setHeader('Cache-Control', 'public, max-age=3600');
      res.setHeader('ETag', cached.etag);
      res.end(cached.body);
      return;
    }
    const body = await fs.promises.readFile(filePath);
    const etag = crypto.createHash('sha256').update(body).digest('hex');
    this.cachedClientScript = { etag, body, mtimeMs: stat.mtimeMs };
    res.statusCode = 200;
    res.setHeader(CONTENT_TYPE_HEADER, JS_CONTENT_TYPE);
    res.setHeader('Cache-Control', 'public, max-age=3600');
    res.setHeader('ETag', etag);
    res.end(body);
  }

  private resolveAgentId(requested?: string): string | undefined {
    if (typeof requested === 'string' && requested.length > 0 && this.registry.has(requested)) {
      return requested;
    }
    if (typeof this.config?.defaultAgent === 'string' && this.config.defaultAgent.length > 0 && this.registry.has(this.config.defaultAgent)) {
      return this.config.defaultAgent;
    }
    const list = this.registry.list();
    return list.length > 0 ? list[0].id : undefined;
  }

  private resolveClientIp(req: http.IncomingMessage): string {
    const forwarded = getHeaderValue(req.headers, 'x-forwarded-for');
    if (forwarded !== undefined) {
      const first = forwarded.split(',')[0].trim();
      if (first.length > 0) return first;
    }
    return req.socket.remoteAddress ?? 'unknown';
  }

  private resolveCors(origin: string | undefined, agentGuid: string | undefined): { allowed: boolean; allowOrigin?: string } {
    if (origin === undefined) return { allowed: true };
    const corsOrigins = this.config?.corsOrigins;
    const tierOrigins = this.config?.auth?.tiers?.netdataProperties?.origins;
    const allowedList = Array.isArray(corsOrigins) && corsOrigins.length > 0
      ? corsOrigins
      : (Array.isArray(tierOrigins) ? tierOrigins : undefined);
    if (Array.isArray(allowedList) && allowedList.length > 0) {
      const allowed = matchOriginPatterns(allowedList, origin);
      return allowed ? { allowed: true, allowOrigin: origin } : { allowed: false };
    }
    if (agentGuid !== undefined && agentGuid.length > 0) {
      return { allowed: true, allowOrigin: origin };
    }
    return { allowed: true, allowOrigin: origin };
  }

  private applyCorsHeaders(res: http.ServerResponse, allowOrigin?: string): void {
    if (allowOrigin !== undefined) {
      res.setHeader('Access-Control-Allow-Origin', allowOrigin);
      res.setHeader('Vary', 'Origin');
    }
    res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
    res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-Netdata-Agent-GUID');
  }

  private applyPreflightHeaders(res: http.ServerResponse): void {
    res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
    res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-Netdata-Agent-GUID');
    res.setHeader('Access-Control-Max-Age', '86400');
  }

  private authorizeRequest(origin: string | undefined, agentGuid: string | undefined, authHeader: string | undefined): AuthDecision {
    const auth = this.config?.auth;
    if (auth === undefined) return { allowed: true, tier: 'unknown' };
    if (!this.checkSignedToken(auth, authHeader)) {
      return { allowed: false, tier: 'unknown', code: 'invalid_token', message: 'invalid token' };
    }
    const netdataTier = auth.tiers?.netdataProperties;
    if (origin !== undefined && this.matchTierOrigins(netdataTier, origin)) {
      return { allowed: true, tier: 'netdata-properties' };
    }
    const agentTier = auth.tiers?.agentDashboards;
    if (agentTier?.verifyGuidInCloud === true && !this.warnedGuidVerification) {
      this.warnedGuidVerification = true;
      this.log('verifyGuidInCloud is enabled but no registry is configured; treating as disabled', 'WRN');
    }
    if (agentGuid !== undefined && agentGuid.length > 0) {
      return { allowed: true, tier: 'agent-dashboards' };
    }
    if (agentTier?.requireGuid === true) {
      return { allowed: false, tier: 'unknown', code: 'missing_guid', message: 'missing agent guid' };
    }
    const unknownTier = auth.tiers?.unknown;
    if (unknownTier?.allow === false) {
      return { allowed: false, tier: 'unknown', code: 'origin_not_allowed', message: 'origin not allowed' };
    }
    return { allowed: true, tier: 'unknown' };
  }

  private checkSignedToken(auth: EmbedAuthConfig, authHeader: string | undefined): boolean {
    const tokens = auth.signedTokens;
    if (tokens?.enabled !== true) return true;
    const secret = typeof tokens.secret === 'string' ? tokens.secret.trim() : '';
    if (secret.length === 0) return false;
    if (authHeader !== undefined && authHeader.length > 0) {
      const token = authHeader.startsWith('Bearer ') ? authHeader.slice('Bearer '.length).trim() : authHeader.trim();
      return token === secret;
    }
    return false;
  }

  private matchTierOrigins(tier: EmbedAuthTierConfig | undefined, origin: string): boolean {
    if (tier?.origins === undefined) return false;
    return matchOriginPatterns(tier.origins, origin);
  }

  private checkRateLimit(tier: AuthDecision['tier'], origin: string | undefined, clientIp: string, agentGuid: string | undefined, now: number): RateLimitDecision {
    if (this.globalLimiter !== undefined) {
      const key = origin ?? clientIp;
      const decision = this.globalLimiter.check(key, now);
      if (!decision.allowed) return decision;
    }
    const tierLimiter = this.tierLimiters[tier];
    if (tier === 'netdata-properties' && tierLimiter.perOrigin !== undefined) {
      const key = origin ?? clientIp;
      return tierLimiter.perOrigin.check(key, now);
    }
    if (tier === 'agent-dashboards' && tierLimiter.perGuid !== undefined) {
      const key = agentGuid ?? clientIp;
      return tierLimiter.perGuid.check(key, now);
    }
    if (tierLimiter.perIp !== undefined) {
      return tierLimiter.perIp.check(clientIp, now);
    }
    return { allowed: true };
  }

  private async persistTranscript(args: { clientId: string; agentId: string; message: string; statusEntries: EmbedTranscriptEntry[]; report: string }): Promise<void> {
    const persistence = resolvePeristenceConfig(this.registry.getPersistence(args.agentId));
    const sessionsDir = persistence.sessionsDir;
    if (typeof sessionsDir !== 'string' || sessionsDir.length === 0) return;
    const filePath = path.join(sessionsDir, 'embed-conversations', `${args.clientId}.json.gz`);
    const transcript = await loadTranscript(filePath, args.clientId);
    const entries: EmbedTranscriptEntry[] = [
      { role: 'user', content: args.message },
      ...args.statusEntries,
      { role: 'assistant', content: args.report },
    ];
    const updated = appendTranscriptTurn(transcript, entries, new Date());
    await writeTranscript(filePath, updated);
  }

  private log(message: string, severity: LogEntry['severity'] = 'VRB', fatal = false): void {
    this.context?.log(buildLog(this.id, this.label, message, severity, fatal));
  }

  private logEntry(entry: LogEntry): void {
    this.context?.log(entry);
  }

  private closeActiveSockets(force: boolean): void {
    this.sockets.forEach((socket) => {
      try {
        if (force) {
          socket.destroy();
        } else {
          socket.end();
        }
      } catch { /* ignore */ }
    });
    this.sockets.clear();
  }

  private handleShutdownSignal(): void {
    if (this.stopping) return;
    this.stopping = true;
    if (this.globalStopRef !== undefined) {
      this.globalStopRef.stopping = true;
    }
    void this.stop();
  }

  private signalClosed(event: HeadendClosedEvent): void {
    if (this.closedSignaled) return;
    this.closedSignaled = true;
    this.closeDeferred.resolve(event);
  }
}
