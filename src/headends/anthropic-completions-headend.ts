import { randomUUID } from 'node:crypto';
import http from 'node:http';
import path from 'node:path';
import { URL } from 'node:url';

import type { AgentMetadata, AgentRegistry } from '../agent-registry.js';
import type { AccountingEntry, AIAgentEvent, AIAgentEventCallbacks, AIAgentEventMeta, FinalReportPayload, LogDetailValue, LogEntry, ProgressEvent, ProgressMetrics } from '../types.js';
import type { Headend, HeadendClosedEvent, HeadendContext, HeadendDescription } from './types.js';
import type { Socket } from 'node:net';

import { AIAgent } from '../ai-agent.js';
import { getTelemetryLabels } from '../telemetry/index.js';
import { createDeferred, normalizeCallPath } from '../utils.js';

import { resolveCompletionsAgent } from './completions-agent-resolution.js';
import { buildCompletionsLogEntry } from './completions-log.js';
import { buildPromptSections } from './completions-prompt.js';
import { resolveFinalReportContent } from './completions-response.js';
import { collectLlmUsage } from './completions-usage.js';
import { ConcurrencyLimiter } from './concurrency.js';
import { signalHeadendClosed } from './headend-close-utils.js';
import { logHeadendEntry } from './headend-log-utils.js';
import { HttpError, readJson, writeJson, writeSseChunk, writeSseDone } from './http-utils.js';
import { buildHeadendModelId } from './model-id-utils.js';
import { refreshModelIdMap } from './model-map-utils.js';
import { renderReasoningMarkdown, type ReasoningTurnState } from './reasoning-markdown.js';
import { createHeadendEventState, markHandoffSeen, shouldAcceptFinalReport, shouldStreamMasterContent, shouldStreamOutput, shouldStreamTurnStarted } from './shared-event-filter.js';
import { handleHeadendShutdown } from './shutdown-utils.js';
import { closeSockets } from './socket-utils.js';
import { escapeMarkdown, formatMetricsLine, formatSummaryLine, italicize, resolveAgentHeadingLabel } from './summary-utils.js';

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

const REMOTE_IDENTIFIER = 'headend:anthropic-completions';

const buildLog = (
  headendId: string,
  label: string,
  message: string,
  severity: LogEntry['severity'] = 'VRB',
  fatal = false,
  details?: Record<string, LogDetailValue>,
): LogEntry => buildCompletionsLogEntry(
  REMOTE_IDENTIFIER,
  headendId,
  label,
  message,
  severity,
  fatal,
  details,
);

const collectUsage = (entries: AccountingEntry[]): { input: number; output: number; total: number } => (
  collectLlmUsage(entries)
);

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
  private shutdownSignal?: AbortSignal;
  private globalStopRef?: { stopping: boolean };
  private shutdownListener?: () => void;
  private readonly sockets = new Set<Socket>();

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
    const shutdownSignal = context.shutdownSignal;
    this.shutdownSignal = shutdownSignal;
    this.globalStopRef = context.stopRef;
    this.log('starting');
    const server = http.createServer((req, res) => { void this.handleRequest(req, res); });
    this.server = server;
    server.on('connection', (socket: Socket) => {
      this.sockets.add(socket);
      socket.on('close', () => { this.sockets.delete(socket); });
    });
    const handler = () => { this.handleShutdownSignal(); };
    shutdownSignal.addEventListener('abort', handler);
    this.shutdownListener = handler;
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
    this.closeActiveSockets(true);
    await new Promise<void>((resolve) => { this.server?.close(() => { resolve(); }); });
    this.server = undefined;
    if (this.shutdownListener !== undefined && this.shutdownSignal !== undefined) {
      this.shutdownSignal.removeEventListener('abort', this.shutdownListener);
      this.shutdownListener = undefined;
    }
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
    let streamedChunks = 0;
    let reasoning = '';
    const accounting: AccountingEntry[] = [];
    let masterSummary: { text?: string; metrics?: ProgressMetrics; statusNote?: string } | undefined;
    let textBlockOpen = false;
    let thinkingBlockOpen = false;
    let lastSentType: 'thinking' | 'output' | undefined;
    let pendingOutputWhitespace = '';
    const agentHeadingLabel = escapeMarkdown(resolveAgentHeadingLabel(agent));
    const turns: ReasoningTurnState[] = [];
    let renderedReasoning = '';
    let transactionHeader: string | undefined;
    let turnCounter = 0;
    let expectingNewTurn = true;
    let rootCallPath: string | undefined;
    let summaryEmitted = false;
    let rootTxnId: string | undefined;

    // Mode machine for progress/model output separation (Solution 2)
    // - Start in 'progress' mode
    // - Thinking is buffered in progress mode, emitted immediately in model mode
    // - Progress events are always emitted immediately
    // - Switch to model mode after 10s timeout or newline in buffer
    let streamMode: 'progress' | 'model' = 'progress';
    let thinkingBuffer = '';
    let bufferTimer: ReturnType<typeof setTimeout> | undefined;
    const BUFFER_TIMEOUT_MS = 10000;
    const PROGRESS_TO_MODEL_SEP = '\n---\n\n';
    const MODEL_TO_PROGRESS_SEP = '\n\n---\n';

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
      lastSentType = 'thinking';
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
    const formatTurnReason = (event: Extract<AIAgentEvent, { type: 'turn_started' }>): string | undefined => {
      const parts: string[] = [];
      if (Array.isArray(event.retrySlugs)) {
        event.retrySlugs.forEach((slug) => {
          if (typeof slug === 'string' && slug.length > 0) parts.push(slug);
        });
      }
      if (typeof event.forcedFinalReason === 'string' && event.forcedFinalReason.length > 0) {
        parts.push(event.forcedFinalReason);
      }
      return parts.length > 0 ? parts.join(', ') : undefined;
    };
    const renderReasoning = (): string => renderReasoningMarkdown({
      header: transactionHeader,
      turns,
      summaryText: summaryEmitted && masterSummary?.text !== undefined ? masterSummary.text : undefined,
    });
    const flushReasoning = (): void => {
      if (!streamed || transactionHeader === undefined) return;
      const next = renderReasoning();
      if (next.length <= renderedReasoning.length) return;
      const delta = next.slice(renderedReasoning.length);
      emitReasoningDelta(delta);
      renderedReasoning = next;
    };
    const ensureSummary = (usageSnapshot: { input: number; output: number; total: number }): void => {
      const fallbackMetrics = masterSummary?.metrics ?? (() => {
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
          tokensIn: usageSnapshot.input,
          tokensOut: usageSnapshot.output,
          tokensCacheRead,
          tokensCacheWrite,
          toolsRun,
          agentsRun,
          costUsd: Number.isFinite(normalizedCost) ? normalizedCost : undefined,
        };
        return metrics;
      })();
      const statusNote = masterSummary?.statusNote;
      const summaryText = formatSummaryLine({
        agentLabel: agentHeadingLabel,
        metrics: fallbackMetrics,
        statusNote,
        usageSnapshot: { prompt: usageSnapshot.input, completion: usageSnapshot.output },
      });
      if (masterSummary === undefined) {
        masterSummary = { text: summaryText, metrics: fallbackMetrics, statusNote };
      } else {
        masterSummary.text = summaryText;
        masterSummary.metrics ??= fallbackMetrics;
      }
      summaryEmitted = true;
      flushReasoning();
    };
    const ensureTurn = (): ReasoningTurnState => {
      if (expectingNewTurn || turns.length === 0) {
        startNextTurn();
        expectingNewTurn = false;
      }
      return turns[turns.length - 1];
    };
    const flushThinkingBuffer = (): void => {
      if (bufferTimer !== undefined) {
        clearTimeout(bufferTimer);
        bufferTimer = undefined;
      }
      // Skip whitespace-only buffers to prevent mode switches on empty newlines from open-source models
      if (thinkingBuffer.length === 0 || thinkingBuffer.trim().length === 0) {
        thinkingBuffer = '';
        return;
      }
      const bufferedContent = thinkingBuffer;
      thinkingBuffer = '';
      // Switch mode BEFORE emitting
      streamMode = 'model';
      // Emit separator then buffer directly (no escaping, no modification)
      emitReasoningDelta(PROGRESS_TO_MODEL_SEP);
      emitReasoningDelta(bufferedContent);
    };
    const startNextTurn = (info?: { index: number; attempt?: number; reason?: string }): void => {
      // DO NOT flush thinking buffer here - it's independent of turns
      // DO NOT reset streamMode here - mode is controlled by thinking/progress flow

      turnCounter += 1;
      const turnIndex = info?.index ?? turnCounter;
      const turn: ReasoningTurnState = { index: turnIndex, updates: [], progressSeen: false };
      if (typeof info?.attempt === 'number' && Number.isFinite(info.attempt)) {
        turn.attempt = info.attempt;
      }
      if (typeof info?.reason === 'string' && info.reason.length > 0) {
        turn.reason = info.reason;
      }
      if (turnCounter > 1) {
        const summary = formatTotals();
        if (summary !== undefined) turn.summary = summary;
      }
      turns.push(turn);
      expectingNewTurn = false;
      flushReasoning();
    };
    const handleTurnStarted = (event: Extract<AIAgentEvent, { type: 'turn_started' }>): void => {
      const reason = formatTurnReason(event);
      if (turns.length > 0) {
        const lastTurn = turns[turns.length - 1];
        if (lastTurn.attempt === undefined && lastTurn.index === event.turn) {
          lastTurn.attempt = event.attempt;
          if (reason !== undefined) lastTurn.reason = reason;
          flushReasoning();
          expectingNewTurn = false;
          return;
        }
      }
      startNextTurn({ index: event.turn, attempt: event.attempt, reason });
      expectingNewTurn = false;
    };
    const appendThinkingChunk = (chunk: string): void => {
      if (chunk.length === 0) return;

      if (streamMode === 'model') {
        // In model mode: emit directly AS-IS (no escaping, no modification)
        emitReasoningDelta(chunk);
      } else {
        // In progress mode: buffer thinking
        thinkingBuffer += chunk;

        // Check for newline - if found and buffer has non-whitespace content, flush immediately
        if (thinkingBuffer.includes('\n') && thinkingBuffer.trim().length > 0) {
          flushThinkingBuffer();
          return;
        }

        // Start timer on first buffered chunk
        bufferTimer ??= setTimeout(() => {
          bufferTimer = undefined;
          flushThinkingBuffer();
        }, BUFFER_TIMEOUT_MS);
      }
    };
    const appendProgressLine = (line: string): void => {
      const trimmed = line.trim();
      if (trimmed.length === 0) return;

      // DO NOT flush thinking buffer - it's independent
      // If in model mode, emit separator and switch to progress
      if (streamMode === 'model') {
        emitReasoningDelta(MODEL_TO_PROGRESS_SEP);
        streamMode = 'progress';
      }

      const turn = ensureTurn();
      turn.updates.push(trimmed);
      flushReasoning();
    };
    const ensureHeader = (txnId?: string, callPath?: string): void => {
      if (rootTxnId === undefined && typeof txnId === 'string' && txnId.length > 0) {
        rootTxnId = txnId;
      }
      if (transactionHeader === undefined) {
        // Use txnId if available; otherwise just show agent label without raw file paths
        const headerId = rootTxnId ?? txnId;
        transactionHeader = headerId !== undefined
          ? `## ${agentHeadingLabel}: ${escapeMarkdown(headerId)}`
          : `## ${agentHeadingLabel}`;
        flushReasoning();
      }
      if (rootCallPath === undefined && typeof callPath === 'string' && callPath.length > 0) {
        rootCallPath = callPath;
      }
    };
    const handleProgressEvent = (event: ProgressEvent): void => {
      if (event.type === 'tool_started' || event.type === 'tool_finished') return;
      // Note: Don't filter by agentId here - subagent progress is allowed via callPathMatches below
      const callPathRaw = typeof event.callPath === 'string' && event.callPath.length > 0 ? event.callPath : event.agentId;
      const normalizedCallPath = normalizeCallPath(callPathRaw);
      const callPathForHeader = normalizedCallPath.length > 0 ? normalizedCallPath : callPathRaw;
      if (rootCallPath === undefined) {
        if (event.agentId === agent.id && normalizedCallPath.length > 0) {
          rootCallPath = normalizedCallPath;
        } else if (event.agentId === agent.id) {
          rootCallPath = agent.id;
        } else if (normalizedCallPath.length > 0) {
          const segments = normalizedCallPath.split(':');
          rootCallPath = segments.length > 1 ? segments.slice(0, -1).join(':') : normalizedCallPath;
        }
      }
      ensureHeader(event.txnId, callPathForHeader);
      const agentMatches = event.agentId === agent.id;
      const callPathMatches = (() => {
        if (rootCallPath === undefined) return agentMatches;
        if (normalizedCallPath.length === 0) return agentMatches;
        if (normalizedCallPath === rootCallPath) return true;
        if (normalizedCallPath.startsWith(`${rootCallPath}:`)) return true;
        return false;
      })();
      if (!agentMatches && !callPathMatches) return;
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
          const turn = ensureTurn();
          turn.progressSeen = true;
          return;
        }
        case 'agent_update': {
          appendProgressLine(`${prefix}: update ${italicize(event.message)}`);
          const turn = ensureTurn();
          turn.progressSeen = true;
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
          const turn = ensureTurn();
          turn.progressSeen = true;
          const summaryStatus = metrics === undefined ? 'completed' : undefined;
          const summaryText = formatSummaryLine({
            agentLabel: agentHeadingLabel,
            metrics,
            statusNote: summaryStatus,
          });
          masterSummary = { text: summaryText, metrics, statusNote: summaryStatus };
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
          const turn = ensureTurn();
          turn.progressSeen = true;
          const statusNote = typeof event.error === 'string' && event.error.length > 0
            ? `failed: ${event.error}`
            : 'failed';
          const summaryText = formatSummaryLine({
            agentLabel: agentHeadingLabel,
            metrics,
            statusNote,
          });
          masterSummary = { text: summaryText, metrics, statusNote };
          summaryEmitted = true;
          flushReasoning();
          return;
        }
        default: {
          appendProgressLine(`${prefix}: update ${italicize('progress event')}`);
          const turn = ensureTurn();
          turn.progressSeen = true;
        }
      }
    };
    const telemetryLabels = { ...getTelemetryLabels(), headend: this.id };

    const eventState = createHeadendEventState();
    let finalReportFromEvent: FinalReportPayload | undefined;

    const callbacks: AIAgentEventCallbacks = {
      onEvent: (event: AIAgentEvent, meta: AIAgentEventMeta) => {
        switch (event.type) {
          case 'output': {
            if (!shouldStreamOutput(event, meta)) return;
            const chunk = event.text;
            output += chunk;
            if (streamed) {
              // Buffer whitespace-only output if last sent was thinking (prevents closing thinking block)
              if (lastSentType === 'thinking' && chunk.trim().length === 0) {
                pendingOutputWhitespace += chunk;
                return;
              }
              // Prepend any buffered whitespace to real output
              const fullChunk = pendingOutputWhitespace + chunk;
              pendingOutputWhitespace = '';
              if (thinkingBlockOpen) closeThinkingBlock();
              openTextBlock();
              const deltaEvent = {
                type: 'content_block_delta',
                content_block: { type: 'text', text_delta: fullChunk },
              };
              writeSseChunk(res, deltaEvent);
              lastSentType = 'output';
              if (chunk.length > 0) {
                streamedChunks += 1;
              }
            }
            return;
          }
          case 'thinking': {
            if (!shouldStreamMasterContent(meta)) return;
            const chunk = event.text;
            if (chunk.length === 0) return;
            reasoning += chunk;
            // Don't call ensureHeader here - wait for progress event with txnId
            // Header will be created when thinking is flushed or progress event arrives
            if (expectingNewTurn) {
              startNextTurn();
              expectingNewTurn = false;
            }
            appendThinkingChunk(chunk);
            return;
          }
          case 'turn_started': {
            if (!shouldStreamTurnStarted(meta)) return;
            // Don't call ensureHeader here - wait for progress event with txnId
            handleTurnStarted(event);
            return;
          }
          case 'progress': {
            handleProgressEvent(event.event);
            return;
          }
          case 'status': {
            // avoid duplicate status handling (progress already includes agent_update)
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
            if (entry.type === 'llm' && meta.isMaster) {
              expectingNewTurn = true;
            }
            return;
          }
          case 'handoff': {
            markHandoffSeen(eventState, meta);
            return;
          }
          case 'final_report': {
            if (shouldAcceptFinalReport(eventState, meta)) {
              finalReportFromEvent = event.report;
            }
            return;
          }
          case 'snapshot':
          case 'accounting_flush':
          case 'op_tree': {
            return;
          }
          default: {
            return;
          }
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
      const result = await AIAgent.run(session);
      if (abortController.signal.aborted) {
        if (!res.writableEnded && !res.writableFinished) {
          if (streamed) { writeSseDone(res); } else { writeJson(res, 499, { error: 'client_closed_request' }); }
        }
        return;
      }
      const finalReport = finalReportFromEvent ?? result.finalReport;
      const finalText = this.resolveContent(output, finalReport);
      const usage = collectUsage(accounting);

      // Flush any remaining buffered thinking before final response
      flushThinkingBuffer();

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
        if (streamedChunks === 0 && finalText.length > 0) {
          openTextBlock();
          const textEvent = {
            type: 'content_block_delta' as const,
            content_block: { type: 'text' as const, text_delta: finalText },
          };
          writeSseChunk(res, textEvent);
        }
        ensureSummary(usage);
        const summaryText = masterSummary?.text;
        if (summaryText !== undefined && summaryText.length > 0) {
          openTextBlock();
          const summaryEvent = {
            type: 'content_block_delta' as const,
            content_block: { type: 'text' as const, text_delta: `\n${summaryText}\n` },
          };
          writeSseChunk(res, summaryEvent);
          closeTextBlock();
        }
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
        ensureSummary(usage);
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
      // Clear any pending buffer timer
      if (bufferTimer !== undefined) {
        clearTimeout(bufferTimer);
        bufferTimer = undefined;
      }
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
      // Clear any pending buffer timer to prevent post-teardown emissions
      if (bufferTimer !== undefined) {
        clearTimeout(bufferTimer);
        bufferTimer = undefined;
      }
      cleanup();
      release();
    }
  }

  private resolveAgent(model: string): AgentMetadata | undefined {
    return resolveCompletionsAgent(model, this.registry, this.modelIdMap, () => { this.refreshModelMap(); });
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
    return buildPromptSections({ systemParts, historyParts, lastUser });
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
    return resolveFinalReportContent(output, finalReport);
  }

  private log(message: string, severity: LogEntry['severity'] = 'VRB', fatal = false, details?: Record<string, LogDetailValue>): void {
    logHeadendEntry(this.context, buildLog(this.id, this.label, message, severity, fatal, details));
  }

  private handleShutdownSignal(): void {
    handleHeadendShutdown(this.globalStopRef, () => { this.closeActiveSockets(); });
  }

  private closeActiveSockets(force = false): void {
    closeSockets(this.sockets, force);
  }

  private logEntry(entry: LogEntry): void {
    logHeadendEntry(this.context, entry);
  }

  private refreshModelMap(): void {
    refreshModelIdMap(this.registry, this.modelIdMap, (meta, seen) => this.buildModelId(meta, seen));
  }

  private buildModelId(meta: AgentMetadata, seen: Set<string>): string {
    const sources = [
      typeof meta.toolName === 'string' ? meta.toolName : undefined,
      typeof meta.promptPath === 'string' ? path.basename(meta.promptPath) : undefined,
      (() => {
        const parts = meta.id.split(/[/\\]/);
        return parts[parts.length - 1];
      })(),
    ];
    return buildHeadendModelId(sources, seen, '_');
  }

  private signalClosed(event: HeadendClosedEvent): void {
    this.closedSignaled = signalHeadendClosed(this.closedSignaled, this.closeDeferred, event);
  }
}
