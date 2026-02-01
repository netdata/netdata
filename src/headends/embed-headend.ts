import crypto from 'node:crypto';
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import type { AgentRegistry } from '../agent-registry.js';
import type { OutputFormatId } from '../formats.js';
import type { AccountingEntry, AIAgentEvent, AIAgentEventCallbacks, AIAgentEventMeta, ConversationMessage, EmbedAuthConfig, EmbedAuthTierConfig, EmbedProfileConfig, EmbedTierRateLimitConfig, FinalReportPayload, LogEntry, ProgressEvent } from '../types.js';
import type { Headend, HeadendClosedEvent, HeadendContext, HeadendDescription } from './types.js';
import type { Socket } from 'node:net';

import { AIAgent } from '../ai-agent.js';
import { resolvePeristenceConfig } from '../persistence.js';
import { getTelemetryLabels } from '../telemetry/index.js';
import { createDeferred, globToRegex, isPlainObject } from '../utils.js';

import { ConcurrencyLimiter } from './concurrency.js';
import { EmbedMetrics } from './embed-metrics.js';
import { appendTranscriptTurn, loadTranscript, writeTranscript, type EmbedTranscriptEntry } from './embed-transcripts.js';
import { signalHeadendClosed } from './headend-close-utils.js';
import { logHeadendEntry } from './headend-log-utils.js';
import { HttpError, readJson, writeJson, writeSseEvent } from './http-utils.js';
import { createHeadendEventState, markHandoffSeen, shouldAcceptFinalReport, shouldStreamOutput, shouldStreamTurnStarted } from './shared-event-filter.js';

const CONTENT_TYPE_HEADER = 'Content-Type';
const CACHE_CONTROL_HEADER = 'Cache-Control';
const CACHE_CONTROL_PUBLIC = 'public, max-age=3600';
const EVENT_STREAM_CONTENT_TYPE = 'text/event-stream';
const JS_CONTENT_TYPE = 'application/javascript; charset=utf-8';
const CSS_CONTENT_TYPE = 'text/css; charset=utf-8';
const HTML_CONTENT_TYPE = 'text/html; charset=utf-8';
const METRICS_CONTENT_TYPE = 'text/plain; version=0.0.4; charset=utf-8';
const OUTPUT_FORMAT_VALUES: readonly OutputFormatId[] = ['markdown', 'markdown+mermaid', 'slack-block-kit', 'tty', 'pipe', 'json', 'sub-agent'] as const;

const isOutputFormatId = (value: string): value is OutputFormatId => OUTPUT_FORMAT_VALUES.includes(value as OutputFormatId);

// Test UI static file routes
const TEST_UI_FILES: Record<string, { file: string; contentType: string }> = {
  '/test-div.html': { file: 'test-div.html', contentType: HTML_CONTENT_TYPE },
  '/test-widget.html': { file: 'test-widget.html', contentType: HTML_CONTENT_TYPE },
  '/test.css': { file: 'test.css', contentType: CSS_CONTENT_TYPE },
  '/test.js': { file: 'test.js', contentType: JS_CONTENT_TYPE },
};

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
  profileName: string;
  port: number;
  concurrency?: number;
  config?: EmbedProfileConfig;
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
  private readonly config?: EmbedProfileConfig;
  private readonly metrics: EmbedMetrics;
  private readonly tierLimiters: Record<AuthDecision['tier'], TierLimiters>;
  private readonly globalLimiter?: SimpleRateLimiter;
  private server?: http.Server;
  private context?: HeadendContext;
  private stopping = false;
  private closedSignaled = false;
  private shutdownSignal?: AbortSignal;
  private shutdownListener?: () => void;
  private globalStopRef?: { stopping: boolean; reason?: 'stop' | 'abort' | 'shutdown' };
  private readonly sockets = new Set<Socket>();
  private cachedClientScript?: { etag: string; body: Buffer; mtimeMs: number };
  private readonly cachedTestFiles = new Map<string, { etag: string; body: Buffer; mtimeMs: number }>();
  private warnedGuidVerification = false;

  public constructor(registry: AgentRegistry, options: EmbedHeadendOptions) {
    this.registry = registry;
    this.options = options;
    this.config = options.config;
    this.id = `embed:${options.profileName}:${String(options.port)}`;
    this.label = `Embed headend ${options.profileName} (port ${String(options.port)})`;
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

    // Test UI files
    if (method === 'GET' && Object.hasOwn(TEST_UI_FILES, path)) {
      const testFile = TEST_UI_FILES[path];
      await this.handleTestFile(res, testFile.file, testFile.contentType);
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
    // Metrics state - accumulates within this HTTP request
    let reasoningChars = 0;
    let outputChars = 0;
    let documentsChars = 0;
    let toolsCount = 0;
    let llmCalls = 0;
    let metricsInterval: ReturnType<typeof setInterval> | undefined;
    let lastMetricsEmitTs = 0;
    let sseHeadersSent = false;
    let metricsEmitted = false;

    const cleanupMetrics = (): void => {
      if (metricsInterval !== undefined) {
        clearInterval(metricsInterval);
        metricsInterval = undefined;
      }
    };

    const emitMetrics = (isFinal: boolean): void => {
      if (!sseHeadersSent || res.writableEnded || res.destroyed) return;
      if (isFinal) {
        if (metricsEmitted) return;
        metricsEmitted = true;
      } else {
        const now = Date.now();
        if (now - lastMetricsEmitTs < 250) return;
        lastMetricsEmitTs = now;
      }
      writeSseEvent(res, 'metrics', {
        elapsed: Date.now() - sessionStartTs,
        reasoningChars,
        outputChars,
        documentsChars,
        tools: toolsCount,
        llmCalls,
        ...(isFinal ? { final: true } : {}),
      });
    };

    const emitFinalMetrics = (): void => {
      emitMetrics(true);
    };
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
      const stopRef = { stopping: this.globalStopRef?.stopping ?? false, reason: this.globalStopRef?.reason };
      const abortController = new AbortController();
      const onRequestClose = (): void => {
        abortController.abort();
        cleanupMetrics();
      };
      req.on('close', onRequestClose);
      sessionStartTs = Date.now();
      sessionActive = true;
      this.metrics.recordSessionStart();

      res.writeHead(200, {
        [CONTENT_TYPE_HEADER]: EVENT_STREAM_CONTENT_TYPE,
        'Cache-Control': 'no-cache',
        Connection: 'keep-alive',
      });
      sseHeadersSent = true;
      const flushHeaders = (res as http.ServerResponse & { flushHeaders?: () => void }).flushHeaders;
      if (typeof flushHeaders === 'function') flushHeaders.call(res);

      writeSseEvent(res, 'client', { clientId, isNew: isNewClient });

      metricsInterval = setInterval(() => {
        emitMetrics(false);
      }, 250);

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
        // Forward only essential progress events to the client
        // Agent started/finished events are tracked internally but not sent to UI
        switch (event.type) {
          case 'agent_started': {
            // Agent count no longer tracked - we track LLM calls instead
            return;
          }
          case 'agent_update': {
            // Only send "now" field from taskStatus - simplified progress updates
            if (event.taskStatus?.now === undefined || event.taskStatus.now.length === 0) {
              return;
            }
            const agentLabel = event.agentName ?? event.agentId;
            const nowMessage = event.taskStatus.now;
            statusEntries.push({ role: 'status', content: nowMessage });
            writeSseEvent(res, 'status', {
              eventType: 'agent_update',
              agent: agentLabel,
              agentPath: event.agentPath,
              now: nowMessage,
              timestamp: event.timestamp,
            });
            return;
          }
          case 'agent_finished':
          case 'agent_failed':
            // Don't emit these to client - tracked internally only
            return;
          case 'tool_started':
            toolsCount += 1;
            emitMetrics(false);
            return;
          case 'tool_finished':
            // Tool events are not forwarded to the UI
            return;
        }
      };

      const callbacks: AIAgentEventCallbacks = {
        onEvent: (event: AIAgentEvent, meta: AIAgentEventMeta) => {
          switch (event.type) {
            case 'output': {
              const chunk = event.text;
              if (chunk.length === 0) return;
              outputChars += chunk.length;
              emitMetrics(false);
              if (!shouldStreamOutput(event, meta)) return;
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
              llmCalls += 1;
              emitMetrics(false);
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
            case 'thinking': {
              const chunk = event.text;
              if (chunk.length === 0) return;
              reasoningChars += chunk.length;
              emitMetrics(false);
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
              const entry = event.entry;
              accounting.push(entry);
              if (entry.type === 'tool') {
                const charsIn = typeof entry.charactersIn === 'number' ? entry.charactersIn : 0;
                const charsOut = typeof entry.charactersOut === 'number' ? entry.charactersOut : 0;
                outputChars += charsIn;
                const isAgentTool = entry.mcpServer === 'agent' || entry.command.startsWith('agent__');
                if (!isAgentTool) {
                  documentsChars += charsOut;
                }
                emitMetrics(false);
              }
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
        outputMode: 'chat',
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
        outputChars += finalText.length;
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
      cleanupMetrics();
      emitFinalMetrics();
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
          cleanupMetrics();
          emitFinalMetrics();
          writeSseEvent(res, 'error', { code, message, recoverable: false });
          res.end();
        } else {
          writeJson(res, err instanceof HttpError ? err.statusCode : 500, { error: code, message });
        }
      }
    } finally {
      cleanupMetrics();
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
      res.setHeader(CACHE_CONTROL_HEADER, CACHE_CONTROL_PUBLIC);
      res.setHeader('ETag', cached.etag);
      res.end(cached.body);
      return;
    }
    // Read file and strip ES module artifacts that TypeScript adds
    // (browsers expect plain script, not ES module syntax)
    const rawContent = await fs.promises.readFile(filePath, 'utf-8');
    const browserSafeContent = rawContent
      .replace(/^export\s*\{\s*\}\s*;?\s*$/gm, '') // Remove empty export statements
      .replace(/\/\/# sourceMappingURL=.*$/gm, ''); // Remove source map references
    const body = Buffer.from(browserSafeContent, 'utf-8');
    const etag = crypto.createHash('sha256').update(body).digest('hex');
    this.cachedClientScript = { etag, body, mtimeMs: stat.mtimeMs };
    res.statusCode = 200;
    res.setHeader(CONTENT_TYPE_HEADER, JS_CONTENT_TYPE);
    res.setHeader(CACHE_CONTROL_HEADER, CACHE_CONTROL_PUBLIC);
    res.setHeader('ETag', etag);
    res.end(body);
  }

  private async handleTestFile(res: http.ServerResponse, fileName: string, contentType: string): Promise<void> {
    const fileUrl = new URL(`./embed-test/${fileName}`, import.meta.url);
    const filePath = fileURLToPath(fileUrl);
    try {
      const stat = await fs.promises.stat(filePath);
      const cached = this.cachedTestFiles.get(fileName);
      if (cached?.mtimeMs === stat.mtimeMs) {
        res.statusCode = 200;
        res.setHeader(CONTENT_TYPE_HEADER, contentType);
        res.setHeader(CACHE_CONTROL_HEADER, CACHE_CONTROL_PUBLIC);
        res.setHeader('ETag', cached.etag);
        res.end(cached.body);
        return;
      }
      const body = await fs.promises.readFile(filePath);
      const etag = crypto.createHash('sha256').update(body).digest('hex');
      this.cachedTestFiles.set(fileName, { etag, body, mtimeMs: stat.mtimeMs });
      res.statusCode = 200;
      res.setHeader(CONTENT_TYPE_HEADER, contentType);
      res.setHeader(CACHE_CONTROL_HEADER, CACHE_CONTROL_PUBLIC);
      res.setHeader('ETag', etag);
      res.end(body);
    } catch {
      writeJson(res, 404, { error: 'file_not_found' });
    }
  }

  private resolveAgentId(requested?: string): string | undefined {
    // Build effective allowed list: allowedAgents > [defaultAgent] > first registered
    const allowedAgents = this.getEffectiveAllowedAgents();
    if (allowedAgents.length === 0) return undefined;

    // If client requested a specific agent, check if it's allowed
    if (typeof requested === 'string' && requested.length > 0) {
      if (allowedAgents.includes(requested) && this.registry.has(requested)) {
        return requested;
      }
      // Requested agent not allowed - fall through to default
    }

    // Return first allowed agent that exists in registry
    // eslint-disable-next-line functional/no-loop-statements -- early return on first match
    for (const agentId of allowedAgents) {
      if (this.registry.has(agentId)) return agentId;
    }
    return undefined;
  }

  private getEffectiveAllowedAgents(): string[] {
    // Use allowedAgents from profile config
    if (Array.isArray(this.config?.allowedAgents) && this.config.allowedAgents.length > 0) {
      return this.config.allowedAgents;
    }
    // Fallback: first registered agent (for minimal config)
    const list = this.registry.list();
    return list.length > 0 ? [list[0].id] : [];
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
    logHeadendEntry(this.context, entry);
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
    this.closedSignaled = signalHeadendClosed(this.closedSignaled, this.closeDeferred, event);
  }
}
