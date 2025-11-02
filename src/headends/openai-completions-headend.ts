import { randomUUID } from 'node:crypto';
import http from 'node:http';
import path from 'node:path';
import { URL } from 'node:url';

import type { AgentMetadata, AgentRegistry } from '../agent-registry.js';
import type { AccountingEntry, AIAgentCallbacks, LLMAccountingEntry, LogDetailValue, LogEntry, ProgressEvent, ProgressMetrics } from '../types.js';
import type { Headend, HeadendClosedEvent, HeadendContext, HeadendDescription } from './types.js';

import { mergeCallbacksWithPersistence } from '../persistence.js';
import { getTelemetryLabels } from '../telemetry/index.js';

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

interface OpenAIChatRequestMessage {
  role: 'system' | 'user' | 'assistant';
  content: unknown;
  tool_calls?: unknown;
}

interface OpenAIChatRequest {
  model: string;
  messages: OpenAIChatRequestMessage[];
  stream?: boolean;
  format?: string;
  response_format?: { type?: string; json_schema?: unknown };
  payload?: Record<string, unknown>;
}

interface OpenAIChatResponse {
  id: string;
  object: 'chat.completion';
  created: number;
  model: string;
  choices: {
    index: number;
    message: { role: 'assistant'; content: string };
    finish_reason: 'stop' | 'error';
  }[];
  usage: {
    prompt_tokens: number;
    completion_tokens: number;
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
  remoteIdentifier: 'headend:openai-completions',
  fatal,
  message: `${label}: ${message}`,
  headendId,
  details,
});

const SYSTEM_PREFIX = 'System context:\n';
const HISTORY_PREFIX = 'Conversation so far:\n';
const USER_PREFIX = 'User request:\n';
const CHAT_COMPLETION_OBJECT = 'chat.completion';
const CHAT_CHUNK_OBJECT = 'chat.completion.chunk';
const CLIENT_CLOSED_ERROR = 'client_closed_request';

const collectUsage = (entries: AccountingEntry[]): { prompt: number; completion: number; total: number } => {
  const usage = entries
    .filter((entry): entry is LLMAccountingEntry => entry.type === 'llm')
    .reduce<{ prompt: number; completion: number }>((acc, entry) => {
      const tokens = entry.tokens;
      acc.prompt += tokens.inputTokens;
      acc.completion += tokens.outputTokens;
      return acc;
    }, { prompt: 0, completion: 0 });
  return { ...usage, total: usage.prompt + usage.completion };
};

interface FormatResolution {
  format: string;
  schema?: Record<string, unknown>;
}

export class OpenAICompletionsHeadend implements Headend {
  public readonly id: string;
  public readonly kind = 'openai-completions';
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
    this.id = `openai-completions:${String(options.port)}`;
    this.label = `OpenAI chat headend (port ${String(options.port)})`;
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
      if (req.method === 'POST' && pathname === '/v1/chat/completions') {
        await this.handleChatCompletion(req, res);
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
    const models = Array.from(this.modelIdMap.keys()).map((modelId) => ({
      id: modelId,
      object: 'model',
      created: Math.floor(Date.now() / 1000),
      owned_by: 'ai-agent',
    }));
    writeJson(res, 200, { object: 'list', data: models });
  }

  private async handleChatCompletion(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
    const body = await readJson<OpenAIChatRequest>(req);
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
    const prompt = this.buildPrompt(body.messages);
    const formatInfo = this.resolveFormat(body, agent);
    const additionalPayload = body.payload !== undefined && typeof body.payload === 'object' && !Array.isArray(body.payload)
      ? body.payload
      : {};
    const agentPayload: Record<string, unknown> = { ...additionalPayload, prompt, format: formatInfo.format };
    if (formatInfo.schema !== undefined) agentPayload.schema = formatInfo.schema;

    const abortController = new AbortController();
    const stopRef = { stopping: false };
    const onAbort = () => {
      if (abortController.signal.aborted) return;
      stopRef.stopping = true;
      abortController.abort();
      this.log('chat completion aborted', 'WRN', false, { model: agent.id });
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

    const created = Math.floor(Date.now() / 1000);
    const responseId = randomUUID();
    const streamed = body.stream === true;
    this.log('chat completion request received', 'VRB', false, { stream: streamed });
    let output = '';
    let streamedChunks = 0;
    let assistantRoleSent = false;
    let transactionHeaderSent = false;
    let rootTxnId: string | undefined;
    let rootOriginTxnId: string | undefined;
    let summaryEmitted = false;
    const accounting: AccountingEntry[] = [];
    const agentHeadingLabel = `**${escapeMarkdown(agent.toolName ?? agent.id)}**`;
    interface TurnRenderState { index: number; summary?: string; thinking?: string; updates: string[] }
    let rootCallPath: string | undefined;
    let transactionHeader: string | undefined;
    let renderedReasoning = '';
    const turns: TurnRenderState[] = [];
    let turnCounter = 0;
    let expectingNewTurn = true;
    let masterSummary: { text: string; origin?: string } | undefined;
    const emitAssistantRole = (): void => {
      if (!streamed || assistantRoleSent) return;
      assistantRoleSent = true;
      const roleChunk: Record<string, unknown> = {
        id: responseId,
        object: CHAT_CHUNK_OBJECT,
        created,
        model: body.model,
        choices: [
          {
            index: 0,
            delta: { role: 'assistant' },
            finish_reason: null,
          },
        ],
      };
      writeSseChunk(res, roleChunk);
    };
    const emitReasoning = (text: string): void => {
      if (!streamed || text.length === 0) return;
      emitAssistantRole();
      const chunk: Record<string, unknown> = {
        id: responseId,
        object: CHAT_CHUNK_OBJECT,
        created,
        model: body.model,
        choices: [
          {
            index: 0,
            delta: { reasoning_content: text },
            finish_reason: null,
          },
        ],
      };
      writeSseChunk(res, chunk);
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
    const resolveOriginFromAccounting = (): string | undefined => {
      const entry = accounting.find((item) => typeof item.originTxnId === 'string' && item.originTxnId.length > 0);
      return entry?.originTxnId;
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
      if (totals.costUsd > 0) parts.push(`cost $**${totals.costUsd.toFixed(4)}**`);
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
          ? ` | origin ${escapeMarkdown(masterSummary.origin)}`
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
      emitReasoning(delta);
      renderedReasoning = next;
    };
    const ensureSummary = (usageSnapshot: { prompt: number; completion: number; total: number }): { text: string; origin: string } => {
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
            tokensIn: usageSnapshot.prompt,
            tokensOut: usageSnapshot.completion,
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
          if (usageSnapshot.total > 0) {
            const tokenSegments = [`→${String(usageSnapshot.prompt)}`, `←${String(usageSnapshot.completion)}`];
            return `tokens ${tokenSegments.join(' ')}`;
          }
          return 'completed';
        })();
      const summaryOrigin = masterSummary?.origin
        ?? rootOriginTxnId
        ?? resolveOriginFromAccounting()
        ?? rootTxnId
        ?? agent.id;
      if (masterSummary === undefined) {
        masterSummary = { text: summaryText, origin: summaryOrigin };
        if (typeof summaryOrigin === 'string' && summaryOrigin.length > 0) {
          rootOriginTxnId = summaryOrigin;
        }
        summaryEmitted = true;
      }
      return { text: summaryText, origin: summaryOrigin };
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
    const appendThinkingChunk = (chunk: string): void => {
      if (chunk.length === 0) return;
      const turn = ensureTurn();
      turn.thinking = (turn.thinking ?? '') + chunk;
      flushReasoning();
    };
    const appendProgressLine = (line: string): void => {
      if (line.length === 0) return;
      const turn = ensureTurn();
      turn.updates.push(line);
      flushReasoning();
    };
    const ensureHeader = (txnId?: string, callPath?: string, agentIdParam?: string): void => {
      if (rootTxnId === undefined && typeof txnId === 'string' && txnId.length > 0) {
        rootTxnId = txnId;
      }
      if (!transactionHeaderSent) {
        const headerId = rootTxnId ?? txnId ?? agentIdParam ?? callPath ?? 'transaction';
        const headerLabel = `**${escapeMarkdown(headerId)}**`;
        transactionHeader = `Started ${agentHeadingLabel} transaction ${headerLabel}:`;
        transactionHeaderSent = true;
        flushReasoning();
      }
      if (rootCallPath === undefined && typeof callPath === 'string' && callPath.length > 0) {
        rootCallPath = callPath;
      }
    };
    const handleProgressEvent = (event: ProgressEvent): void => {
      if (event.type === 'tool_started' || event.type === 'tool_finished') return;
      const callPath = typeof event.callPath === 'string' && event.callPath.length > 0 ? event.callPath : undefined;
      if (rootCallPath === undefined) {
        if (event.agentId === agent.id && callPath !== undefined) {
          rootCallPath = callPath;
        } else if (event.agentId === agent.id) {
          rootCallPath = agent.id;
        } else if (callPath !== undefined) {
          const segments = callPath.split(':');
          rootCallPath = segments.length > 1 ? segments.slice(0, -1).join(':') : callPath;
        }
      }
      const txnId = 'txnId' in event ? event.txnId : undefined;
      ensureHeader(txnId, callPath, event.agentId);
      const agentMatches = event.agentId === agent.id;
      const callPathMatches = (() => {
        if (rootCallPath === undefined) return agentMatches;
        if (callPath === undefined) return agentMatches;
        if (callPath === rootCallPath) return true;
        if (callPath.startsWith(`${rootCallPath}:`)) return true;
        return false;
      })();
      if (!agentMatches && !callPathMatches) return;
      const eventMetrics = 'metrics' in event ? (event as { metrics?: ProgressMetrics }).metrics : undefined;
      const metricsText = formatMetricsLine(eventMetrics);
      const originCandidate = (event as { originTxnId?: string }).originTxnId;
      if (typeof originCandidate === 'string' && originCandidate.length > 0) {
        rootOriginTxnId = originCandidate;
      }
      const displayCallPath = callPath ?? event.agentId;

      if (expectingNewTurn || turns.length === 0) {
        startNextTurn();
      }
      switch (event.type) {
        case 'agent_started': {
          const isRoot = agentMatches && (callPath === undefined || callPath === rootCallPath);
          if (isRoot) return;
          const reason = typeof event.reason === 'string' && event.reason.length > 0 ? italicize(event.reason) : undefined;
          const prefix = `[${escapeMarkdown(displayCallPath)}]`;
          const line = reason !== undefined ? `${prefix}: started ${reason}` : `${prefix}: started`;
          appendProgressLine(line);
          return;
        }
        case 'agent_update': {
          const prefix = `[${escapeMarkdown(displayCallPath)}]`;
          const updateLine = `${prefix}: update ${italicize(event.message)}`;
          appendProgressLine(updateLine);
          return;
        }
        case 'agent_finished': {
          const prefix = `[${escapeMarkdown(displayCallPath)}]`;
          const line = metricsText.length > 0 ? `${prefix}: finished ${metricsText}` : `${prefix}: finished`;
          appendProgressLine(line);
          if (agentMatches && (callPath === undefined || callPath === rootCallPath)) {
            const summary = metricsText.length > 0 ? metricsText : 'completed';
            const origin = (event as { originTxnId?: string }).originTxnId ?? rootOriginTxnId;
            if (origin !== undefined && origin.length > 0) rootOriginTxnId = origin;
            masterSummary = { text: summary, origin };
            summaryEmitted = true;
            flushReasoning();
          }
          return;
        }
        case 'agent_failed': {
          const errorText = typeof event.error === 'string' && event.error.length > 0 ? italicize(event.error) : undefined;
          const prefix = `[${escapeMarkdown(displayCallPath)}]`;
          let line = `${prefix}: failed`;
          if (errorText !== undefined && metricsText.length > 0) {
            line += `: ${errorText}, ${metricsText}`;
          } else if (errorText !== undefined) {
            line += `: ${errorText}`;
          } else if (metricsText.length > 0) {
            line += `: ${metricsText}`;
          }
          appendProgressLine(line);
          if (agentMatches && (callPath === undefined || callPath === rootCallPath)) {
            const summary = metricsText.length > 0
              ? metricsText
              : (errorText !== undefined ? `failed: ${errorText}` : 'failed');
            const origin = (event as { originTxnId?: string }).originTxnId ?? rootOriginTxnId;
            if (origin !== undefined && origin.length > 0) rootOriginTxnId = origin;
            masterSummary = { text: summary, origin };
            summaryEmitted = true;
            flushReasoning();
          }
          return;
        }
        default:
          appendProgressLine(`[${escapeMarkdown(displayCallPath)}]: update ${italicize('progress event')}`);
      }
    };
    const telemetryLabels = { ...getTelemetryLabels(), headend: this.id };

    const baseCallbacks: AIAgentCallbacks = {
      onOutput: (chunk) => {
        output += chunk;
        if (streamed) {
          emitAssistantRole();
          const chunkPayload = {
            id: responseId,
            object: CHAT_CHUNK_OBJECT,
            created,
            model: body.model,
            choices: [
              {
                index: 0,
                delta: { content: chunk },
                finish_reason: null,
              },
            ],
          };
          writeSseChunk(res, chunkPayload);
          if (chunk.length > 0) {
            streamedChunks += 1;
          }
        }
      },
      onThinking: (chunk) => {
        if (chunk.length === 0) return;
        ensureHeader(undefined, rootCallPath, agent.id);
        if (expectingNewTurn) {
          startNextTurn();
          expectingNewTurn = false;
        }
        appendThinkingChunk(chunk);
      },
      onProgress: (event) => {
        handleProgressEvent(event);
      },
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
    const callbacks = mergeCallbacksWithPersistence(baseCallbacks, this.registry.getPersistence(agent.id)) ?? baseCallbacks;

    if (streamed) {
      res.writeHead(200, {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        Connection: 'keep-alive',
      });
      const flushHeaders = (res as http.ServerResponse & { flushHeaders?: () => void }).flushHeaders;
      if (typeof flushHeaders === 'function') flushHeaders.call(res);
    }

    try {
      const session = await this.registry.spawnSession({
        agentId: agent.id,
        payload: agentPayload,
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
          if (streamed) { writeSseDone(res); } else { writeJson(res, 499, { error: CLIENT_CLOSED_ERROR }); }
        }
        return;
      }
      const finalText = this.resolveContent(output, result.finalReport);
      const fallbackError = (!result.success && typeof result.error === 'string' && result.error.length > 0)
        ? result.error
        : undefined;
      const effectiveText = result.success
        ? finalText
        : (finalText.length > 0
          ? finalText
          : (fallbackError ?? 'Agent session failed without details.'));
      const usageSnapshot = collectUsage(accounting);
      if (streamed) {
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- guard avoids duplicating turn entries when a new LLM turn begins
        if (expectingNewTurn || turns.length === 0) {
          startNextTurn();
          expectingNewTurn = false;
        }
        if (streamedChunks === 0 && effectiveText.length > 0) {
          emitAssistantRole();
          const contentChunk = {
            id: responseId,
            object: CHAT_CHUNK_OBJECT,
            created,
            model: body.model,
            choices: [
              {
                index: 0,
                delta: { content: effectiveText },
                finish_reason: null,
              },
            ],
          };
          writeSseChunk(res, contentChunk);
        }
        ensureSummary(usageSnapshot);
        flushReasoning();
        const finalChunk: Record<string, unknown> = {
          id: responseId,
          object: CHAT_CHUNK_OBJECT,
          created,
          model: body.model,
          choices: [
            {
              index: 0,
              delta: {},
              finish_reason: result.success ? 'stop' : 'error',
            },
          ],
          usage: {
            prompt_tokens: usageSnapshot.prompt,
            completion_tokens: usageSnapshot.completion,
            total_tokens: usageSnapshot.total,
          },
        };
        emitAssistantRole();
        writeSseChunk(res, finalChunk);
        writeSseDone(res);
      } else {
        const usageSnapshot = collectUsage(accounting);
        ensureSummary(usageSnapshot);
        const reasoningText = renderReasoning().trim();
        const combinedContent = reasoningText.length > 0
          ? `${reasoningText}${reasoningText.endsWith('\n') ? '' : '\n'}${effectiveText}`
          : effectiveText;
        flushReasoning();
        const response: OpenAIChatResponse = {
          id: responseId,
          object: CHAT_COMPLETION_OBJECT,
          created,
          model: body.model,
          choices: [
            {
              index: 0,
              message: { role: 'assistant', content: combinedContent },
              finish_reason: result.success ? 'stop' : 'error',
            },
          ],
          usage: {
            prompt_tokens: usageSnapshot.prompt,
            completion_tokens: usageSnapshot.completion,
            total_tokens: usageSnapshot.total,
          },
        };
        writeJson(res, result.success ? 200 : 500, response);
      }
      this.log('chat completion finished', 'VRB', false, { model: agent.id, status: result.success ? 'ok' : 'error' });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.log('chat completion failed', 'ERR', false, { model: agent.id, error: message });
      if (streamed) {
        emitAssistantRole();
        const chunk = {
          id: responseId,
          object: CHAT_CHUNK_OBJECT,
          created,
          model: body.model,
          choices: [
            {
              index: 0,
              delta: { content: message },
              finish_reason: 'error' as const,
            },
          ],
        };
        writeSseChunk(res, chunk);
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

  private buildPrompt(messages: OpenAIChatRequestMessage[]): string {
    const systemParts: string[] = [];
    const historyParts: string[] = [];
    let lastUser: string | undefined;
    messages.forEach((msg, idx) => {
      if (Object.prototype.hasOwnProperty.call(msg, 'tool_calls')) {
        throw new HttpError(400, 'tool_calls_unsupported', 'tool calls are not supported');
      }
      const text = this.stringifyContent(msg.content);
      if (msg.role === 'system') {
        if (text.length > 0) systemParts.push(text);
      } else if (msg.role === 'user') {
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
      throw new HttpError(400, 'missing_user_prompt', 'A user message is required as the final turn');
    }
    const sections: string[] = [];
    if (systemParts.length > 0) sections.push(`${SYSTEM_PREFIX}${systemParts.join('\n')}`);
    if (historyParts.length > 0) sections.push(`${HISTORY_PREFIX}${historyParts.join('\n')}`);
    sections.push(`${USER_PREFIX}${lastUser}`);
    return sections.join('\n\n');
  }

  private resolveFormat(body: OpenAIChatRequest, agent: AgentMetadata): FormatResolution {
    const explicit = typeof body.format === 'string' && body.format.length > 0 ? body.format : undefined;
    const responseFormat = body.response_format;
    if (explicit !== undefined) {
      const schema = explicit === 'json' ? this.pickSchema(body, agent) : undefined;
      return { format: explicit, schema };
    }
    if (responseFormat?.type === 'json_object') {
      const schema = this.pickSchema(body, agent) ?? this.asObject(responseFormat.json_schema);
      if (schema === undefined) {
        throw new HttpError(400, 'missing_schema', 'JSON response_format requires json_schema');
      }
      return { format: 'json', schema };
    }
    if (agent.outputSchema !== undefined && agent.expectedOutput?.format === 'json') {
      return { format: 'json', schema: agent.outputSchema };
    }
    const fallback = agent.expectedOutput?.format ?? 'markdown';
    const schema = fallback === 'json' ? this.pickSchema(body, agent) : undefined;
    if (fallback === 'json' && schema === undefined) {
      throw new HttpError(400, 'missing_schema', 'Agent expects JSON output but no schema provided');
    }
    return { format: fallback, schema };
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
    ].filter((value): value is string => typeof value === 'string' && value.length > 0);
    let base = baseSources[0] ?? 'agent';
    base = base.replace(/\.ai$/i, '');
    if (base.length === 0) base = 'agent';
    let candidate = base;
    let counter = 2;
    // eslint-disable-next-line functional/no-loop-statements
    while (seen.has(candidate)) {
      const suffix = String(counter);
      candidate = `${base}-${suffix}`;
      counter += 1;
    }
    seen.add(candidate);
    return candidate;
  }

  private pickSchema(body: OpenAIChatRequest, agent: AgentMetadata): Record<string, unknown> | undefined {
    if (agent.outputSchema !== undefined) return agent.outputSchema;
    const responseFormat = body.response_format;
    if (responseFormat?.json_schema !== undefined) {
      const obj = this.asObject(responseFormat.json_schema);
      if (obj !== undefined) return obj;
    }
    if (body.payload !== undefined) {
      const schemaCandidate = body.payload.schema;
      const obj = this.asObject(schemaCandidate);
      if (obj !== undefined) return obj;
    }
    return undefined;
  }

  private asObject(value: unknown): Record<string, unknown> | undefined {
    if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
      return value as Record<string, unknown>;
    }
    return undefined;
  }

  private stringifyContent(content: unknown): string {
    if (typeof content === 'string') return content;
    if (Array.isArray(content)) {
      return content
        .map((item) => {
          if (typeof item === 'string') return item;
          if (item !== null && typeof item === 'object') {
            const text = (item as { text?: unknown }).text;
            if (typeof text === 'string') return text;
          }
          return '';
        })
        .filter((s) => s.length > 0)
        .join('\n');
    }
    return '';
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

  private signalClosed(event: HeadendClosedEvent): void {
    if (this.closedSignaled) return;
    this.closedSignaled = true;
    this.closeDeferred.resolve(event);
  }
}
