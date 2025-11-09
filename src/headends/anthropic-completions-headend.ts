import { randomUUID } from 'node:crypto';
import http from 'node:http';
import path from 'node:path';
import { URL } from 'node:url';

import type { AgentMetadata, AgentRegistry } from '../agent-registry.js';
import type { AccountingEntry, AIAgentCallbacks, LLMAccountingEntry, LogDetailValue, LogEntry, ProgressEvent, ProgressMetrics } from '../types.js';
import type { Headend, HeadendClosedEvent, HeadendContext, HeadendDescription } from './types.js';

import { getTelemetryLabels } from '../telemetry/index.js';
import { normalizeCallPath } from '../utils.js';

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

const buildLog = (
  headendId: string,
  label: string,
  message: string,
  severity: LogEntry['severity'] = 'VRB',
  fatal = false,
  details?: Record<string, LogDetailValue>,
): LogEntry => ({
  timestamp: Date.now(),
  severity,
  turn: 0,
  subturn: 0,
  direction: 'response',
  type: 'tool',
  remoteIdentifier: 'headend:anthropic-completions',
  fatal,
  message: `${label}: ${message}`,
  headendId,
  details,
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
  private readonly label: string;
  private context?: HeadendContext;
  private server?: http.Server;
  private stopping = false;
  private closedSignaled = false;

  public constructor(registry: AgentRegistry, options: { port: number; concurrency?: number }) {
    this.registry = registry;
    this.options = options;
    this.id = `anthropic-completions:${String(options.port)}`;
    this.label = `Anthropic chat headend (port ${String(options.port)})`;
    const limit = typeof options.concurrency === 'number' && Number.isFinite(options.concurrency) && options.concurrency > 0
      ? Math.floor(options.concurrency)
      : 10;
    this.limiter = new ConcurrencyLimiter(limit);
    this.closed = this.closeDeferred.promise;
    this.refreshModelMap();
  }

  public describe(): HeadendDescription {
    return { id: this.id, kind: this.kind, label: this.label };
  }

  public async start(context: HeadendContext): Promise<void> {
    if (this.server !== undefined) return;
    this.context = context;
    this.log('starting');
    const server = http.createServer((req, res) => { void this.handleRequest(req, res); });
    this.server = server;
    server.on('error', (err) => {
      const message = err instanceof Error ? err.message : String(err);
      this.log('server error', 'ERR', true, { error: message });
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
    this.log('listening', 'VRB', false, { port: this.options.port, concurrency_limit: this.limiter.maxConcurrency });
    this.log('started');
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
      this.log('request failed', 'ERR', false, { error: message });
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
      this.log('anthropic request aborted', 'WRN', false, { model: agent.id });
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
      this.log('concurrency acquisition failed', 'ERR', false, { model: agent.id, error: message });
      writeJson(res, 503, { error: 'concurrency_unavailable', message: 'Concurrency limit reached' });
      return;
    }

    const requestId = randomUUID();
    const streamed = body.stream === true;
    let output = '';
    let reasoning = '';
    const accounting: AccountingEntry[] = [];
    let masterSummary: { text: string; origin?: string } | undefined;
    let rootOriginTxnId: string | undefined;
    let textBlockOpen = false;
    let thinkingBlockOpen = false;
    const agentHeadingLabel = `**${escapeMarkdown(agent.toolName ?? agent.id)}**`;
    interface TurnRenderState { index: number; summary?: string; thinking?: string; updates: string[] }
    const turns: TurnRenderState[] = [];
    let renderedReasoning = '';
    let transactionHeader: string | undefined;
    let turnCounter = 0;
    let expectingNewTurn = true;
    let rootCallPath: string | undefined;
    let summaryEmitted = false;
    let rootTxnId: string | undefined;

    const openTextBlock = (): void => {
      if (!streamed || textBlockOpen) return;
      const start = { type: 'content_block_start', content_block: { type: 'text' as const } };
      writeSseChunk(res, start);
      textBlockOpen = true;
    };

    const closeTextBlock = (): void => {
      if (!streamed || !textBlockOpen) return;
      const stop = { type: 'content_block_stop' as const };
      writeSseChunk(res, stop);
      textBlockOpen = false;
    };

    const openThinkingBlock = (): void => {
      if (!streamed || thinkingBlockOpen) return;
      const start = { type: 'content_block_start', content_block: { type: 'thinking' as const } };
      writeSseChunk(res, start);
      thinkingBlockOpen = true;
    };

    const closeThinkingBlock = (): void => {
      if (!streamed || !thinkingBlockOpen) return;
      const stop = { type: 'content_block_stop' as const };
      writeSseChunk(res, stop);
      thinkingBlockOpen = false;
    };
    const emitReasoningDelta = (text: string): void => {
      if (!streamed || text.length === 0) return;
      if (textBlockOpen) closeTextBlock();
      openThinkingBlock();
      const event = {
        type: 'content_block_delta' as const,
        content_block: { type: 'thinking' as const, thinking_delta: text },
      };
      writeSseChunk(res, event);
    };
    const computeTotals = (): { tokensIn: number; tokensOut: number; tokensCacheRead: number; tokensCacheWrite: number; tools: number; costUsd: number } => {
      let tokensIn = 0;
      let tokensOut = 0;
      let tokensCacheRead = 0;
      let tokensCacheWrite = 0;
      let costUsd = 0;
      let tools = 0;
      accounting.forEach((entry) => {
        if (entry.type === 'llm') {
          tokensIn += entry.tokens.inputTokens;
          tokensOut += entry.tokens.outputTokens;
          tokensCacheRead += entry.tokens.cacheReadInputTokens ?? entry.tokens.cachedTokens ?? 0;
          tokensCacheWrite += entry.tokens.cacheWriteInputTokens ?? 0;
          if (typeof entry.costUsd === 'number' && Number.isFinite(entry.costUsd)) costUsd += entry.costUsd;
          return;
        }
        tools += 1;
      });
      return { tokensIn, tokensOut, tokensCacheRead, tokensCacheWrite, tools, costUsd };
    };
    const formatTotals = (): string | undefined => {
      const totals = computeTotals();
      const parts: string[] = [];
      if (totals.tokensIn > 0 || totals.tokensOut > 0 || totals.tokensCacheRead > 0 || totals.tokensCacheWrite > 0) {
        const segments: string[] = [];
        if (totals.tokensIn > 0) segments.push(`→${String(totals.tokensIn)}`);
        if (totals.tokensOut > 0) segments.push(`←${String(totals.tokensOut)}`);
        if (totals.tokensCacheRead > 0) segments.push(`c→${String(totals.tokensCacheRead)}`);
        if (totals.tokensCacheWrite > 0) segments.push(`c←${String(totals.tokensCacheWrite)}`);
        parts.push(`tokens ${segments.join(' ')}`);
      }
      if (totals.tools > 0) parts.push(`tools ${String(totals.tools)}`);
      if (totals.costUsd > 0) parts.push(`cost $${totals.costUsd.toFixed(4)}`);
      return parts.length > 0 ? parts.join(', ') : undefined;
    };
    const renderReasoning = (): string => {
      if (transactionHeader === undefined) return '';
      const lines: string[] = [transactionHeader];
      turns.forEach((turn) => {
        const headingParts = [`${String(turn.index)}. Turn ${String(turn.index)}`];
        if (turn.summary !== undefined && turn.summary.length > 0) headingParts.push(`(${turn.summary})`);
        lines.push(headingParts.join(' '));
        if (turn.thinking !== undefined && turn.thinking.trim().length > 0) {
          lines.push(`  - thinking: _${escapeMarkdown(turn.thinking.trim())}_`);
        }
        turn.updates.forEach((line) => {
          lines.push(`  - ${line}`);
        });
      });
      if (summaryEmitted && masterSummary !== undefined) {
        const originSuffix = masterSummary.origin !== undefined && masterSummary.origin.length > 0
          ? ` (origin ${escapeMarkdown(masterSummary.origin)})`
          : '';
        lines.push(`Transaction summary: ${masterSummary.text}${originSuffix}`);
      }
      return `${lines.join('\n')}\n`;
    };
    const flushReasoning = (): void => {
      if (!streamed || transactionHeader === undefined) return;
      const next = renderReasoning();
      if (next.length <= renderedReasoning.length) return;
      const delta = next.slice(renderedReasoning.length);
      emitReasoningDelta(delta);
      renderedReasoning = next;
    };
    const ensureTurn = (): TurnRenderState => {
      if (expectingNewTurn || turns.length === 0) {
        startNextTurn();
        expectingNewTurn = false;
      }
      return turns[turns.length - 1];
    };
    const startNextTurn = (): void => {
      turnCounter += 1;
      const turn: TurnRenderState = { index: turnCounter, updates: [] };
      if (turnCounter > 1) {
        const summary = formatTotals();
        if (summary !== undefined) turn.summary = summary;
      }
      turns.push(turn);
      expectingNewTurn = false;
      flushReasoning();
    };
    const ensureTurnIndex = (target: number): void => {
      if (!Number.isFinite(target) || target <= 0) return;
      // eslint-disable-next-line functional/no-loop-statements -- deterministic numbering
      while (turnCounter < target) {
        startNextTurn();
      }
      expectingNewTurn = false;
    };
    const appendThinkingChunk = (chunk: string): void => {
      if (chunk.length === 0) return;
      const turn = ensureTurn();
      turn.thinking = (turn.thinking ?? '') + chunk;
      flushReasoning();
    };
    const appendProgressLine = (line: string): void => {
      const trimmed = line.trim();
      if (trimmed.length === 0) return;
      const turn = ensureTurn();
      turn.updates.push(trimmed);
      flushReasoning();
    };
    const ensureHeader = (txnId?: string, callPath?: string): void => {
      if (rootTxnId === undefined && typeof txnId === 'string' && txnId.length > 0) {
        rootTxnId = txnId;
      }
      if (transactionHeader === undefined) {
        const headerId = rootTxnId ?? txnId ?? callPath ?? requestId;
        const headerLabel = `**${escapeMarkdown(headerId)}**`;
        transactionHeader = `Started ${agentHeadingLabel} transaction ${headerLabel}:`;
        flushReasoning();
      }
      if (rootCallPath === undefined && typeof callPath === 'string' && callPath.length > 0) {
        rootCallPath = callPath;
      }
    };
    const resolveOriginFromAccounting = (): string | undefined => {
      const entry = accounting.find((item) => typeof item.originTxnId === 'string' && item.originTxnId.length > 0);
      return entry?.originTxnId;
    };
    const handleProgressEvent = (event: ProgressEvent): void => {
      if (event.type === 'tool_started' || event.type === 'tool_finished') return;
      if (event.agentId !== agent.id) return;
      const callPathRaw = typeof event.callPath === 'string' && event.callPath.length > 0 ? event.callPath : event.agentId;
      const normalizedCallPath = normalizeCallPath(callPathRaw);
      const callPathForHeader = normalizedCallPath.length > 0 ? normalizedCallPath : callPathRaw;
      ensureHeader(event.txnId, callPathForHeader);
      const agentPathRaw = (event as { agentPath?: string }).agentPath;
      const normalizedAgentPath = normalizeCallPath(agentPathRaw);
      const displayCallPath = (() => {
        if (normalizedAgentPath.length > 0) return normalizedAgentPath;
        if (typeof agentPathRaw === 'string' && agentPathRaw.length > 0) return agentPathRaw;
        if (callPathForHeader.length > 0) return callPathForHeader;
        const fallback = normalizeCallPath(event.agentId);
        return fallback.length > 0 ? fallback : event.agentId;
      })();
      const prefix = `**${escapeMarkdown(displayCallPath)}**`;
      if (expectingNewTurn || turns.length === 0) {
        startNextTurn();
        expectingNewTurn = false;
      }
      switch (event.type) {
        case 'agent_started': {
          const reason = typeof event.reason === 'string' && event.reason.length > 0 ? italicize(event.reason) : undefined;
          appendProgressLine(reason !== undefined ? `${prefix}: started ${reason}` : `${prefix}: started`);
          return;
        }
        case 'agent_update': {
          appendProgressLine(`${prefix}: update ${italicize(event.message)}`);
          return;
        }
        case 'agent_finished': {
          const metrics = 'metrics' in event ? (event as { metrics?: ProgressMetrics }).metrics : undefined;
          const metricsText = formatMetricsLine(metrics);
          const isRoot = callPathForHeader === normalizedAgentPath || (normalizedAgentPath.length === 0 && callPathForHeader === event.agentId);
          const finishedLine = (() => {
            if (isRoot) return `${prefix}: finished`;
            return metricsText.length > 0 ? `${prefix}: finished ${metricsText}` : `${prefix}: finished`;
          })();
          appendProgressLine(finishedLine);
          const originCandidate = (event as { originTxnId?: string }).originTxnId;
          if (typeof originCandidate === 'string' && originCandidate.length > 0) {
            rootOriginTxnId = originCandidate;
          }
          const summary = metricsText.length > 0 ? metricsText : 'completed';
          const origin = originCandidate ?? rootOriginTxnId;
          if (typeof origin === 'string' && origin.length > 0) rootOriginTxnId = origin;
          masterSummary = { text: summary, origin };
          summaryEmitted = true;
          flushReasoning();
          return;
        }
        case 'agent_failed': {
          const metrics = 'metrics' in event ? (event as { metrics?: ProgressMetrics }).metrics : undefined;
          const metricsText = formatMetricsLine(metrics);
          const errorText = typeof event.error === 'string' && event.error.length > 0 ? italicize(event.error) : undefined;
          let line = `${prefix}: failed`;
          if (errorText !== undefined && metricsText.length > 0) {
            line += `: ${errorText}, ${metricsText}`;
          } else if (errorText !== undefined) {
            line += `: ${errorText}`;
          } else if (metricsText.length > 0) {
            line += `: ${metricsText}`;
          }
          appendProgressLine(line);
          const originCandidate = (event as { originTxnId?: string }).originTxnId;
          if (typeof originCandidate === 'string' && originCandidate.length > 0) {
            rootOriginTxnId = originCandidate;
          }
          const summary = metricsText.length > 0
            ? metricsText
            : (errorText !== undefined ? `failed: ${errorText}` : 'failed');
          const origin = originCandidate ?? rootOriginTxnId;
          if (typeof origin === 'string' && origin.length > 0) rootOriginTxnId = origin;
          masterSummary = { text: summary, origin };
          summaryEmitted = true;
          flushReasoning();
          return;
        }
        default:
          appendProgressLine(`${prefix}: update ${italicize('progress event')}`);
      }
    };
    const telemetryLabels = { ...getTelemetryLabels(), headend: this.id };

    const callbacks: AIAgentCallbacks = {
      onOutput: (chunk) => {
        output += chunk;
        if (streamed) {
          if (thinkingBlockOpen) closeThinkingBlock();
          openTextBlock();
          const event = {
            type: 'content_block_delta',
            content_block: { type: 'text', text_delta: chunk },
          };
          writeSseChunk(res, event);
        }
      },
      onThinking: (chunk) => {
        if (chunk.length === 0) return;
        reasoning += chunk;
        ensureHeader(undefined, rootCallPath ?? agent.id);
        if (expectingNewTurn) {
          startNextTurn();
          expectingNewTurn = false;
        }
        appendThinkingChunk(chunk);
      },
      onTurnStarted: (turnIndex) => {
        ensureHeader(undefined, rootCallPath ?? agent.id);
        ensureTurnIndex(turnIndex);
      },
      onProgress: (event) => { handleProgressEvent(event); },
      onLog: (entry) => {
        entry.headendId = this.id;
        this.logEntry(entry);
      },
      onAccounting: (entry) => {
        accounting.push(entry);
        if (entry.type === 'llm' && (entry.agentId === undefined || entry.agentId === agent.id)) {
          expectingNewTurn = true;
        }
      },
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
    }

    try {
      const session = await this.registry.spawnSession({
        agentId: agent.id,
        payload,
        callbacks,
        abortSignal: abortController.signal,
        stopRef,
        headendId: this.id,
        telemetryLabels,
        wantsProgressUpdates: true,
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
        if (turns.length === 0) {
          startNextTurn();
          expectingNewTurn = false;
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- guard preserves existing turn numbering when additional turns aren't scheduled
        } else if (expectingNewTurn) {
          startNextTurn();
          expectingNewTurn = false;
        }
        flushReasoning();
        closeThinkingBlock();
        if (finalText.length > 0) {
          openTextBlock();
          const textEvent = {
            type: 'content_block_delta' as const,
            content_block: { type: 'text' as const, text_delta: finalText },
          };
          writeSseChunk(res, textEvent);
        }
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
          summaryEmitted = true;
          flushReasoning();
        }
        const originSuffix = typeof summaryOrigin === 'string' && summaryOrigin.length > 0
          ? ` | origin ${escapeMarkdown(summaryOrigin)}`
          : '';
        openTextBlock();
        const summaryEvent = {
          type: 'content_block_delta' as const,
          content_block: { type: 'text' as const, text_delta: `\nTransaction summary: ${summaryText}${originSuffix}\n` },
        };
        writeSseChunk(res, summaryEvent);
        closeTextBlock();
        const stopEvent = { type: 'message_stop' as const };
        writeSseChunk(res, stopEvent);
        writeSseDone(res);
      } else {
        const trimmedReasoning = reasoning.trim();
        ensureHeader(undefined, rootCallPath ?? agent.id);
        if (turns.length === 0) {
          startNextTurn();
          expectingNewTurn = false;
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- guard preserves existing turn numbering when additional turns aren't scheduled
        } else if (expectingNewTurn) {
          startNextTurn();
          expectingNewTurn = false;
        }
        const rendered = renderReasoning().trim();
        const contentBlocks = (() => {
          const blocks: { type: 'text' | 'thinking'; text?: string; thinking?: string }[] = [];
          if (rendered.length > 0) {
            blocks.push({ type: 'thinking', thinking: rendered });
          } else if (trimmedReasoning.length > 0) {
            blocks.push({ type: 'thinking', thinking: trimmedReasoning });
          }
          blocks.push({ type: 'text', text: finalText });
          return blocks;
        })();
        const response: AnthropicResponseBody = {
          id: requestId,
          type: 'message',
          role: 'assistant',
          model: body.model,
          content: contentBlocks as unknown as AnthropicResponseBody['content'],
          stop_reason: result.success ? 'end_turn' : 'error',
          usage: {
            input_tokens: usage.input,
            output_tokens: usage.output,
            total_tokens: usage.total,
          },
        };
        writeJson(res, result.success ? 200 : 500, response);
      }
      this.log('anthropic completion finished', 'VRB', false, { model: agent.id, status: result.success ? 'ok' : 'error' });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.log('anthropic completion failed', 'ERR', false, { model: agent.id, error: message });
      if (streamed) {
        closeThinkingBlock();
        closeTextBlock();
        const errorEvent = { type: 'error', message };
        writeSseChunk(res, errorEvent);
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

  private log(message: string, severity: LogEntry['severity'] = 'VRB', fatal = false, details?: Record<string, LogDetailValue>): void {
    if (this.context === undefined) return;
    this.context.log(buildLog(this.id, this.label, message, severity, fatal, details));
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
