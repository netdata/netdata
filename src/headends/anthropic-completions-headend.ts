import { randomUUID } from 'node:crypto';
import http from 'node:http';
import path from 'node:path';
import { URL } from 'node:url';

import type { AgentMetadata, AgentRegistry } from '../agent-registry.js';
import type { AccountingEntry, AIAgentCallbacks, LLMAccountingEntry, LogEntry, ProgressEvent, ProgressMetrics } from '../types.js';
import type { Headend, HeadendClosedEvent, HeadendContext, HeadendDescription } from './types.js';

import { ConcurrencyLimiter } from './concurrency.js';
import { HttpError, readJson, writeJson, writeSseChunk, writeSseDone } from './http-utils.js';
import { escapeMarkdown, formatMetricsLine, italicize } from './summary-utils.js';

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

interface AnthropicMessageContent {
  type?: string;
  text?: string;
}

interface AnthropicRequestMessage {
  role: 'user' | 'assistant';
  content: string | AnthropicMessageContent[];
}

interface AnthropicRequestBody {
  model: string;
  messages: AnthropicRequestMessage[];
  system?: string | string[];
  stream?: boolean;
  format?: string;
  payload?: Record<string, unknown>;
}

interface AnthropicResponseBody {
  id: string;
  type: 'message';
  role: 'assistant';
  model: string;
  content: { type: 'text'; text: string }[];
  stop_reason: 'end_turn' | 'error';
  usage: {
    input_tokens: number;
    output_tokens: number;
    total_tokens: number;
  };
}

const buildLog = (headendId: string, message: string, severity: LogEntry['severity'] = 'VRB', fatal = false): LogEntry => ({
  timestamp: Date.now(),
  severity,
  turn: 0,
  subturn: 0,
  direction: 'response',
  type: 'tool',
  remoteIdentifier: 'headend:anthropic-completions',
  fatal,
  message,
  headendId,
});

const SYSTEM_PREFIX = 'System context:\n';
const HISTORY_PREFIX = 'Conversation so far:\n';
const USER_PREFIX = 'User request:\n';

const collectUsage = (entries: AccountingEntry[]): { input: number; output: number; total: number } => {
  const usage = entries
    .filter((entry): entry is LLMAccountingEntry => entry.type === 'llm')
    .reduce<{ input: number; output: number }>((acc, entry) => {
      const tokens = entry.tokens;
      acc.input += tokens.inputTokens;
      acc.output += tokens.outputTokens;
      return acc;
    }, { input: 0, output: 0 });
  return { ...usage, total: usage.input + usage.output };
};

export class AnthropicCompletionsHeadend implements Headend {
  public readonly id: string;
  public readonly kind = 'anthropic-completions';
  public readonly closed: Promise<HeadendClosedEvent>;

  private readonly registry: AgentRegistry;
  private readonly options: { port: number; concurrency?: number };
  private readonly limiter: ConcurrencyLimiter;
  private readonly closeDeferred = createDeferred<HeadendClosedEvent>();
  private readonly modelIdMap = new Map<string, string>();
  private context?: HeadendContext;
  private server?: http.Server;
  private stopping = false;
  private closedSignaled = false;

  public constructor(registry: AgentRegistry, options: { port: number; concurrency?: number }) {
    this.registry = registry;
    this.options = options;
    this.id = `anthropic-completions:${String(options.port)}`;
    const limit = typeof options.concurrency === 'number' && Number.isFinite(options.concurrency) && options.concurrency > 0
      ? Math.floor(options.concurrency)
      : 10;
    this.limiter = new ConcurrencyLimiter(limit);
    this.closed = this.closeDeferred.promise;
    this.refreshModelMap();
  }

  public describe(): HeadendDescription {
    return { id: this.id, kind: this.kind, label: `Anthropic chat headend (port ${String(this.options.port)})` };
  }

  public async start(context: HeadendContext): Promise<void> {
    if (this.server !== undefined) return;
    this.context = context;
    const server = http.createServer((req, res) => { void this.handleRequest(req, res); });
    this.server = server;
    server.on('error', (err) => {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`server error: ${message}`, 'ERR', true);
      if (!this.stopping) {
        this.stopping = true;
        this.signalClosed({ reason: 'error', error: err instanceof Error ? err : new Error(message) });
      }
    });
    server.on('close', () => {
      if (!this.stopping) {
        this.stopping = true;
        this.signalClosed({ reason: 'stopped', graceful: true });
      }
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
    this.log('listening');
  }

  public async stop(): Promise<void> {
    if (this.server === undefined) {
      if (!this.stopping) {
        this.stopping = true;
        this.signalClosed({ reason: 'stopped', graceful: true });
      }
      return;
    }
    this.stopping = true;
    await new Promise<void>((resolve) => { this.server?.close(() => { resolve(); }); });
    this.server = undefined;
    this.signalClosed({ reason: 'stopped', graceful: true });
  }

  private async handleRequest(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
    try {
      const url = new URL(req.url ?? '/', `http://localhost:${String(this.options.port)}`);
      const pathname = url.pathname.replace(/\/+/g, '/');
      if (req.method === 'GET' && pathname === '/health') {
        writeJson(res, 200, { status: 'ok' });
        return;
      }
      if (req.method === 'GET' && pathname === '/v1/models') {
        this.handleModels(res);
        return;
      }
      if (req.method === 'POST' && pathname === '/v1/messages') {
        await this.handleMessages(req, res);
        return;
      }
      writeJson(res, 404, { error: 'not_found' });
    } catch (err) {
      if (err instanceof HttpError) {
        writeJson(res, err.statusCode, { error: err.code });
        return;
      }
      const message = err instanceof Error ? err.message : String(err);
      this.log(`request failure: ${message}`, 'ERR');
      writeJson(res, 500, { error: 'internal_error' });
    }
  }

  private handleModels(res: http.ServerResponse): void {
    this.refreshModelMap();
    const models = Array.from(this.modelIdMap.entries()).map(([modelId, agentId]) => {
      const meta = this.registry.getMetadata(agentId);
      return {
        id: modelId,
        type: 'model',
        display_name: meta?.description ?? modelId,
      };
    });
    writeJson(res, 200, { data: models });
  }

  private async handleMessages(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
    const body = await readJson<AnthropicRequestBody>(req);
    if (typeof body.model !== 'string' || body.model.length === 0) {
      throw new HttpError(400, 'missing_model', 'model is required');
    }
    const agent = this.resolveAgent(body.model);
    if (agent === undefined) {
      throw new HttpError(404, 'unknown_model', `Model '${body.model}' not registered`);
    }
    if (!Array.isArray(body.messages) || body.messages.length === 0) {
      throw new HttpError(400, 'missing_messages', 'messages array is required');
    }
    const prompt = this.composePrompt(body.system, body.messages);
    const format = typeof body.format === 'string' && body.format.length > 0 ? body.format : (agent.expectedOutput?.format ?? 'markdown');
    const schema = format === 'json' ? (agent.outputSchema ?? this.extractSchema(body.payload)) : undefined;
    if (format === 'json' && schema === undefined) {
      throw new HttpError(400, 'missing_schema', 'JSON format requires schema');
    }
    const payload: Record<string, unknown> = { prompt, format };
    if (schema !== undefined) payload.schema = schema;
    const additionalPayload = body.payload !== undefined && typeof body.payload === 'object' && !Array.isArray(body.payload)
      ? body.payload
      : undefined;
    if (additionalPayload !== undefined) {
      Object.entries(additionalPayload).forEach(([key, value]) => {
        if (!(key in payload)) payload[key] = value;
      });
    }

    const abortController = new AbortController();
    const stopRef = { stopping: false };
    const onAbort = () => {
      if (abortController.signal.aborted) return;
      stopRef.stopping = true;
      abortController.abort();
      this.log(`anthropic request aborted model=${agent.id}`, 'WRN');
    };
    req.on('aborted', onAbort);
    res.on('close', onAbort);
    const cleanup = () => {
      req.removeListener('aborted', onAbort);
      res.removeListener('close', onAbort);
    };

    let release: (() => void) | undefined;
    try {
      release = await this.limiter.acquire({ signal: abortController.signal });
    } catch (err: unknown) {
      cleanup();
      if (abortController.signal.aborted) {
        return;
      }
      const message = err instanceof Error ? err.message : String(err);
      this.log(`concurrency acquire failed for model=${agent.id}: ${message}`, 'ERR');
      writeJson(res, 503, { error: 'concurrency_unavailable', message: 'Concurrency limit reached' });
      return;
    }

    const requestId = randomUUID();
    const streamed = body.stream === true;
    let output = '';
    const accounting: AccountingEntry[] = [];
    let masterSummary: { text: string; origin?: string } | undefined;
    let rootOriginTxnId: string | undefined;
    const resolveOriginFromAccounting = (): string | undefined => {
      const entry = accounting.find((item) => typeof item.originTxnId === 'string' && item.originTxnId.length > 0);
      return entry?.originTxnId;
    };
    const handleProgressEvent = (event: ProgressEvent): void => {
      if (event.type !== 'agent_finished' && event.type !== 'agent_failed') return;
      if (event.agentId !== agent.id) return;
      const metrics = 'metrics' in event ? (event as { metrics?: ProgressMetrics }).metrics : undefined;
      const metricsText = formatMetricsLine(metrics);
      const originCandidate = (event as { originTxnId?: string }).originTxnId;
      if (typeof originCandidate === 'string' && originCandidate.length > 0) {
        rootOriginTxnId = originCandidate;
      }
      const errorText = event.type === 'agent_failed' && typeof event.error === 'string' && event.error.length > 0
        ? italicize(event.error)
        : undefined;
      const summary = metricsText.length > 0
        ? metricsText
        : (event.type === 'agent_failed' ? (errorText !== undefined ? `failed: ${errorText}` : 'failed') : 'completed');
      const origin = originCandidate ?? rootOriginTxnId;
      if (typeof origin === 'string' && origin.length > 0) rootOriginTxnId = origin;
      masterSummary = { text: summary, origin };
    };
    const callbacks: AIAgentCallbacks = {
      onOutput: (chunk) => {
        output += chunk;
        if (streamed) {
          const event = {
            type: 'content_block_delta',
            content_block: { type: 'text', text_delta: chunk },
          };
          writeSseChunk(res, event);
        }
      },
      onProgress: (event) => { handleProgressEvent(event); },
      onLog: (entry) => { this.logEntry(entry); },
      onAccounting: (entry) => { accounting.push(entry); },
    };

    if (streamed) {
      res.writeHead(200, {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        Connection: 'keep-alive',
      });
      const startEvent = {
        type: 'message_start',
        message: {
          id: requestId,
          type: 'message',
          role: 'assistant',
          model: body.model,
          content: [] as unknown[],
        },
      };
      writeSseChunk(res, startEvent);
      const contentBlockStart = { type: 'content_block_start', content_block: { type: 'text' as const } };
      writeSseChunk(res, contentBlockStart);
    }

    try {
      const session = await this.registry.spawnSession({
        agentId: agent.id,
        payload,
        callbacks,
        abortSignal: abortController.signal,
        stopRef,
      });
      const result = await session.run();
      if (abortController.signal.aborted) {
        if (!res.writableEnded && !res.writableFinished) {
          if (streamed) { writeSseDone(res); } else { writeJson(res, 499, { error: 'client_closed_request' }); }
        }
        return;
      }
      const finalText = this.resolveContent(output, result.finalReport);
      const usage = collectUsage(accounting);
      if (streamed) {
        const contentStop = { type: 'content_block_stop' as const };
        const fallbackMetrics = masterSummary === undefined
          ? (() => {
            const agentIds = new Set<string>();
            let toolsRun = 0;
            let tokensCacheRead = 0;
            let tokensCacheWrite = 0;
            let costUsd = 0;
            let minTs: number | undefined;
            let maxTs: number | undefined;
            accounting.forEach((entry) => {
              if (typeof entry.agentId === 'string' && entry.agentId.length > 0) agentIds.add(entry.agentId);
              if (entry.type === 'tool') toolsRun += 1;
              if (entry.type === 'llm') {
                tokensCacheRead += entry.tokens.cacheReadInputTokens ?? entry.tokens.cachedTokens ?? 0;
                tokensCacheWrite += entry.tokens.cacheWriteInputTokens ?? 0;
                if (typeof entry.costUsd === 'number' && Number.isFinite(entry.costUsd)) {
                  costUsd += entry.costUsd;
                }
              }
              const ts = entry.timestamp;
              if (typeof ts === 'number' && Number.isFinite(ts)) {
                minTs = minTs === undefined ? ts : Math.min(minTs, ts);
                maxTs = maxTs === undefined ? ts : Math.max(maxTs, ts);
              }
            });
            const durationMs = (minTs !== undefined && maxTs !== undefined) ? Math.max(0, maxTs - minTs) : undefined;
            const agentsRun = agentIds.size > 0 ? agentIds.size : 1;
            const normalizedCost = Number(costUsd.toFixed(4));
            const metrics: ProgressMetrics = {
              durationMs,
              tokensIn: usage.input,
              tokensOut: usage.output,
              tokensCacheRead,
              tokensCacheWrite,
              toolsRun,
              agentsRun,
              costUsd: Number.isFinite(normalizedCost) ? normalizedCost : undefined,
            };
            return metrics;
          })()
          : undefined;
        const summaryText = masterSummary?.text
          ?? (() => {
            const metricsLine = fallbackMetrics !== undefined ? formatMetricsLine(fallbackMetrics) : '';
            if (metricsLine.length > 0) return metricsLine;
            if (usage.total > 0) {
              return `tokens →${String(usage.input)} ←${String(usage.output)}`;
            }
            return 'completed';
          })();
        const summaryOrigin = masterSummary?.origin
          ?? rootOriginTxnId
          ?? resolveOriginFromAccounting()
          ?? requestId;
        if (masterSummary === undefined) {
          masterSummary = { text: summaryText, origin: summaryOrigin };
          if (typeof summaryOrigin === 'string' && summaryOrigin.length > 0) rootOriginTxnId = summaryOrigin;
        }
        const originSuffix = typeof summaryOrigin === 'string' && summaryOrigin.length > 0
          ? ` | origin ${escapeMarkdown(summaryOrigin)}`
          : '';
        const summaryEvent = {
          type: 'content_block_delta' as const,
          content_block: { type: 'text' as const, text_delta: `\nTransaction summary: ${summaryText}${originSuffix}\n` },
        };
        writeSseChunk(res, summaryEvent);
        writeSseChunk(res, contentStop);
        const stopEvent = { type: 'message_stop' as const };
        writeSseChunk(res, stopEvent);
        writeSseDone(res);
      } else {
        const response: AnthropicResponseBody = {
          id: requestId,
          type: 'message',
          role: 'assistant',
          model: body.model,
          content: [{ type: 'text', text: finalText }],
          stop_reason: result.success ? 'end_turn' : 'error',
          usage: {
            input_tokens: usage.input,
            output_tokens: usage.output,
            total_tokens: usage.total,
          },
        };
        writeJson(res, result.success ? 200 : 500, response);
      }
      this.log(`anthropic completion model=${agent.id} ${result.success ? 'ok' : 'error'}`);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.log(`anthropic completion failed model=${agent.id}: ${message}`, 'ERR');
      if (streamed) {
        const errorEvent = { type: 'error', message };
        writeSseChunk(res, errorEvent);
        const contentStop = { type: 'content_block_stop' as const };
        writeSseChunk(res, contentStop);
        writeSseDone(res);
      } else {
        const status = err instanceof HttpError ? err.statusCode : 500;
        const code = err instanceof HttpError ? err.code : 'chat_failure';
        writeJson(res, status, { error: code, message });
      }
    } finally {
      cleanup();
      release();
    }
  }

  private resolveAgent(model: string): AgentMetadata | undefined {
    this.refreshModelMap();
    const direct = this.modelIdMap.get(model);
    if (direct !== undefined) {
      return this.registry.getMetadata(direct);
    }
    const resolved = this.registry.resolveAgentId(model);
    if (resolved !== undefined) {
      return this.registry.getMetadata(resolved);
    }
    return undefined;
  }

  private composePrompt(system: string | string[] | undefined, messages: AnthropicRequestMessage[]): string {
    const systemParts: string[] = [];
    if (typeof system === 'string' && system.length > 0) systemParts.push(system);
    if (Array.isArray(system)) {
      system.filter((s): s is string => typeof s === 'string' && s.length > 0).forEach((s) => systemParts.push(s));
    }
    const historyParts: string[] = [];
    let lastUser: string | undefined;
    messages.forEach((msg, idx) => {
      const text = this.stringifyContent(msg.content);
      if (msg.role === 'user') {
        if (idx === messages.length - 1) {
          lastUser = text;
        } else {
          historyParts.push(`User: ${text}`);
        }
      } else {
        historyParts.push(`Assistant: ${text}`);
      }
    });
    if (lastUser === undefined || lastUser.length === 0) {
      throw new HttpError(400, 'missing_user_prompt', 'Final user message is required');
    }
    const segments: string[] = [];
    if (systemParts.length > 0) segments.push(`${SYSTEM_PREFIX}${systemParts.join('\n')}`);
    if (historyParts.length > 0) segments.push(`${HISTORY_PREFIX}${historyParts.join('\n')}`);
    segments.push(`${USER_PREFIX}${lastUser}`);
    return segments.join('\n\n');
  }

  private stringifyContent(content: string | AnthropicMessageContent[]): string {
    if (typeof content === 'string') return content;
    return content
      .map((item) => (typeof item.text === 'string' ? item.text : ''))
      .filter((s) => s.length > 0)
      .join('\n');
  }

  private extractSchema(payload?: Record<string, unknown>): Record<string, unknown> | undefined {
    if (payload === undefined) return undefined;
    const val = payload.schema;
    if (val !== null && typeof val === 'object' && !Array.isArray(val)) {
      return val as Record<string, unknown>;
    }
    return undefined;
  }

  private resolveContent(output: string, finalReport: unknown): string {
    if (finalReport !== undefined && finalReport !== null && typeof finalReport === 'object') {
      const report = finalReport as { format?: string; content?: string; content_json?: Record<string, unknown> };
      if (report.format === 'json' && report.content_json !== undefined) {
        try { return JSON.stringify(report.content_json); } catch { /* ignore */ }
      }
      if (typeof report.content === 'string' && report.content.length > 0) return report.content;
    }
    return output;
  }

  private log(message: string, severity: LogEntry['severity'] = 'VRB', fatal = false): void {
    if (this.context === undefined) return;
    this.context.log(buildLog(this.id, message, severity, fatal));
  }

  private logEntry(entry: LogEntry): void {
    if (this.context === undefined) return;
    this.context.log(entry);
  }

  private refreshModelMap(): void {
    this.modelIdMap.clear();
    const seen = new Set<string>();
    this.registry.list().forEach((meta) => {
      const modelId = this.buildModelId(meta, seen);
      this.modelIdMap.set(modelId, meta.id);
    });
  }

  private buildModelId(meta: AgentMetadata, seen: Set<string>): string {
    const baseSources = [
      typeof meta.toolName === 'string' ? meta.toolName : undefined,
      typeof meta.promptPath === 'string' ? path.basename(meta.promptPath) : undefined,
      (() => {
        const parts = meta.id.split(/[/\\]/);
        return parts[parts.length - 1];
      })(),
    ];
    const primary = baseSources.find((value): value is string => typeof value === 'string' && value.length > 0) ?? 'agent';
    const root = primary.replace(/\.ai$/i, '') || 'agent';
    let candidate = root;
    let counter = 2;
    // eslint-disable-next-line functional/no-loop-statements
    while (seen.has(candidate)) {
      const suffix = String(counter);
      candidate = `${root}_${suffix}`;
      counter += 1;
    }
    seen.add(candidate);
    return candidate;
  }

  private signalClosed(event: HeadendClosedEvent): void {
    if (this.closedSignaled) return;
    this.closedSignaled = true;
    this.closeDeferred.resolve(event);
  }
}
