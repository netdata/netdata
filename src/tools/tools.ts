import { SpanKind } from '@opentelemetry/api';

import type { ResponseCache } from '../cache/response-cache.js';
import type { ToolCacheResolver } from '../cache/tool-cache-resolver.js';
import type { CacheEntryMetadata, CacheLookupResult } from '../cache/types.js';
import type { SessionProgressReporter } from '../session-progress-reporter.js';
import type { SessionTreeBuilder, SessionNode } from '../session-tree.js';
import type { ToolMetricsRecord } from '../telemetry/index.js';
import type { MCPTool, LogEntry, AccountingEntry, ProgressMetrics, LogDetailValue } from '../types.js';
import type { ToolExecuteOptions, ToolExecuteResult, ToolKind, ToolProvider, ToolExecutionContext } from './types.js';

import { recordToolMetrics, runWithSpan, addSpanAttributes, addSpanEvent, recordSpanError, recordQueueDepthMetrics, recordQueueWaitMetrics } from '../telemetry/index.js';
import { truncateToBytesWithInfo } from '../truncation.js';
import { appendCallPathSegment, formatToolRequestCompact, normalizeCallPath, sanitizeToolName, warn } from '../utils.js';

import { queueManager, type AcquireResult, QueueAbortError } from './queue-manager.js';

queueManager.setListeners({
  onDepthChange: (queue, info) => {
    recordQueueDepthMetrics({ queue, capacity: info.capacity, inUse: info.inUse, waiting: info.waiting });
  },
  onWaitComplete: (queue, waitMs) => {
    recordQueueWaitMetrics({ queue, waitMs });
  },
});

const toErrorMessage = (value: unknown): string => {
  if (value instanceof Error && typeof value.message === 'string') return value.message;
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') {
    return value.toString();
  }
  if (value === null) return 'null';
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value);
    } catch {
      return '[unserializable-error]';
    }
  }
  return 'unknown_error';
};

const isCachedToolResult = (value: unknown): value is { result: string } => {
  if (value === null || typeof value !== 'object') return false;
  const record = value as Record<string, unknown>;
  return typeof record.result === 'string';
};

const computeTruncationPercent = (originalBytes: number, finalBytes: number): number => {
  if (originalBytes <= 0) return 0;
  const pct = ((originalBytes - finalBytes) / originalBytes) * 100;
  return Number(Math.max(0, pct).toFixed(1));
};

export interface ToolBudgetReservation {
  ok: boolean;
  tokens?: number;
  reason?: string;
  availableTokens?: number;  // Available token budget (only set when ok=false)
}

export interface ToolBudgetCallbacks {
  reserveToolOutput: (output: string) => Promise<ToolBudgetReservation>;
  previewToolOutput?: (output: string) => Promise<ToolBudgetReservation>;
  canExecuteTool: () => boolean;
  countTokens: (text: string) => number;  // Tokenizer for accurate budget truncation
}

export interface ToolOutputHandleRequest {
  toolName: string;
  toolArgsJson: string;
  output: string;
  sizeLimitBytes?: number;
  budgetCallbacks?: ToolBudgetCallbacks;
  sourceTarget?: { provider: string; model: string };
  forceReason?: 'reserve_failed';
}

export interface ToolOutputHandleResult {
  handle: string;
  message: string;
  stats: { bytes: number; lines: number; tokens: number };
  reason: 'size_cap' | 'token_budget' | 'reserve_failed';
}

export interface ToolOutputHandler {
  maybeStore: (req: ToolOutputHandleRequest) => Promise<ToolOutputHandleResult | undefined>;
}

interface ToolCacheContext {
  getCache?: () => ResponseCache | undefined;
  resolver?: ToolCacheResolver;
}

export interface ManagedToolResult {
  ok: boolean;
  result: string;
  providerLabel: string;
  latency: number;
  charactersIn: number;
  charactersOut: number;
  tokens?: number;
  dropped?: boolean;
  reason?: string;
}

const TOOL_MANAGE_ERROR_EVENT = 'tool.manage.error';

export class ToolsOrchestrator {
  private readonly providers: ToolProvider[] = [];
  private readonly mapping = new Map<string, { provider: ToolProvider; kind: ToolKind; queueName?: string }>();
  // Minimal alias support to smooth tool naming mismatches across prompts/providers
  private readonly aliases = new Map<string, string>();
  private canceled = false;
  private readonly pendingQueueControllers = new Set<AbortController>();
  private readonly cacheProvider?: () => ResponseCache | undefined;
  private readonly toolCacheResolver?: ToolCacheResolver;
  private readonly sessionInfo: { agentId?: string; agentPath?: string; callPath?: string; txnId?: string; headendId?: string; telemetryLabels?: Record<string, string>; agentHash?: string };

  constructor(
    private readonly opts: { toolTimeout?: number; toolResponseMaxBytes?: number; traceTools?: boolean },
    private readonly opTree: SessionTreeBuilder,
    private readonly emitOpTree: (tree: SessionNode) => void,
    // Session-level logger (must never throw)
    private readonly log: (entry: LogEntry, opts?: { opId?: string }) => void,
    private readonly recordAccounting?: (entry: AccountingEntry) => void,
    sessionInfo?: { agentId?: string; agentPath?: string; callPath?: string; txnId?: string; headendId?: string; telemetryLabels?: Record<string, string>; agentHash?: string },
    private readonly progress?: SessionProgressReporter,
    private readonly budgetCallbacks?: ToolBudgetCallbacks,
    private readonly toolOutputHandler?: ToolOutputHandler,
    cacheContext?: ToolCacheContext,
  ) {
    const normalizedCallPath = sessionInfo?.callPath !== undefined ? normalizeCallPath(sessionInfo.callPath) : undefined;
    const normalizedAgentPath = sessionInfo?.agentPath !== undefined ? normalizeCallPath(sessionInfo.agentPath) : undefined;
    const normalizedAgentId = sessionInfo?.agentId !== undefined ? normalizeCallPath(sessionInfo.agentId) : undefined;
    const maybeValue = (value: string | undefined): string | undefined => (value !== undefined && value.length > 0 ? value : undefined);
    this.sessionInfo = {
      ...sessionInfo,
      agentId: maybeValue(normalizedAgentId) ?? sessionInfo?.agentId,
      agentPath: maybeValue(normalizedAgentPath) ?? maybeValue(normalizedAgentId) ?? sessionInfo?.agentPath,
      callPath: maybeValue(normalizedCallPath)
        ?? maybeValue(sessionInfo?.callPath)
        ?? maybeValue(normalizedAgentPath)
        ?? maybeValue(normalizedAgentId),
    };
    this.cacheProvider = cacheContext?.getCache;
    this.toolCacheResolver = cacheContext?.resolver;
  }

  register(provider: ToolProvider): void {
    this.providers.push(provider);
    // Populate mapping lazily on list to avoid stale entries
  }

  // Warmup providers that require async initialization (e.g., MCP) and refresh tool mapping
  async warmup(): Promise<void> {
    if (this.canceled) return;
    await Promise.all(this.providers.map(async (p) => { try { await p.warmup(); } catch (e) { warn(`provider warmup failed: ${toErrorMessage(e)}`); } }));
    // Refresh mapping now that providers are warmed
    this.listTools();
  }

  private getSessionCallPath(): string {
    const { callPath, agentPath, agentId } = this.sessionInfo;
    const normalizedCallPath = normalizeCallPath(callPath);
    if (normalizedCallPath.length > 0) return normalizedCallPath;
    if (typeof callPath === 'string' && callPath.length > 0) return callPath;
    const normalizedAgentPath = normalizeCallPath(agentPath);
    if (normalizedAgentPath.length > 0) return normalizedAgentPath;
    if (typeof agentPath === 'string' && agentPath.length > 0) return agentPath;
    const normalizedAgentId = normalizeCallPath(agentId);
    if (normalizedAgentId.length > 0) return normalizedAgentId;
    if (typeof agentId === 'string' && agentId.length > 0) return agentId;
    return 'agent';
  }

  private getSessionAgentId(): string {
    const { agentId } = this.sessionInfo;
    if (typeof agentId === 'string' && agentId.length > 0) return agentId;
    return this.getSessionCallPath();
  }

  private getSessionAgentPath(): string {
    const { agentPath, agentId, callPath } = this.sessionInfo;
    const normalizedAgentPath = normalizeCallPath(agentPath);
    if (normalizedAgentPath.length > 0) return normalizedAgentPath;
    if (typeof agentPath === 'string' && agentPath.length > 0) return agentPath;
    const normalizedAgentId = normalizeCallPath(agentId);
    if (normalizedAgentId.length > 0) return normalizedAgentId;
    if (typeof agentId === 'string' && agentId.length > 0) return agentId;
    const normalizedCallPath = normalizeCallPath(callPath);
    if (normalizedCallPath.length > 0) return normalizedCallPath;
    if (typeof callPath === 'string' && callPath.length > 0) return callPath;
    return 'agent';
  }

  listTools(): MCPTool[] {
    if (this.canceled) return [];
    const tools = this.providers.flatMap((p) => p.listTools());
    // Refresh mapping every time we list
    this.mapping.clear();
    this.providers.forEach((p) => {
      p.listTools().forEach((t) => {
        const queueName = p.resolveQueueName(t.name) ?? t.queue ?? 'default';
        this.mapping.set(t.name, { provider: p, kind: p.kind, queueName });
        const sanitized = sanitizeToolName(t.name);
        if (!this.mapping.has(sanitized)) {
          this.mapping.set(sanitized, { provider: p, kind: p.kind, queueName });
        }
      });
    });
    return tools;
  }

  private resolveName(name: string): string {
    const sanitized = sanitizeToolName(name);
    if (this.mapping.has(sanitized)) return sanitized;
    const alias = this.aliases.get(sanitized);
    if (alias !== undefined && this.mapping.has(alias)) return alias;
    // Heuristics: try common prefixes for sub-agents and REST tools when the model omits them
    const tryAgent = `agent__${sanitized}`;
    if (this.mapping.has(tryAgent)) return tryAgent;
    const tryRest = `rest__${sanitized}`;
    if (this.mapping.has(tryRest)) return tryRest;
    return sanitized;
  }

  hasTool(name: string): boolean {
    if (this.canceled) return false;
    if (this.mapping.size === 0) this.listTools();
    const effective = this.resolveName(name);
    return this.mapping.has(effective);
  }

  async execute(name: string, parameters: Record<string, unknown>, opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    if (this.canceled) throw new Error('canceled');
    if (this.mapping.size === 0) this.listTools();
    const effective = this.resolveName(name);
    let entry = this.mapping.get(effective);
    if (entry === undefined) {
      // Ensure mapping is fully refreshed before failing (all providers/sub-agents loaded)
      this.listTools();
      const effective2 = this.resolveName(name);
      entry = this.mapping.get(effective2);
    }
    if (entry === undefined) {
      // Fail fast for sub-agent names that are not part of the static registry snapshot
      if (name.startsWith('agent__') || effective.startsWith('agent__')) {
        throw new Error(`unknown_subagent_tool: '${name}' is not registered in this session's agent registry`);
      }
      throw new Error(`Unknown tool: ${name}`);
    }
    return await entry.provider.execute(effective, parameters, opts);
  }

  // Management wrapper: applies timeout, size cap, logging, and accounting for all providers
  async executeWithManagement(
    name: string,
    parameters: Record<string, unknown>,
    ctx: ToolExecutionContext,
    opts?: ToolExecuteOptions
  ) {
    const attributes: Record<string, string> = {
      'ai.tool.requested_name': name,
      'ai.tool.call_path': this.getSessionCallPath(),
    };
    return await runWithSpan('agent.tool', { attributes, kind: SpanKind.INTERNAL }, () =>
      this.executeWithManagementInternal(name, parameters, ctx, opts)
    );
  }

  async executeWithManagementInternal(
    name: string,
    parameters: Record<string, unknown>,
    ctx: ToolExecutionContext,
    opts?: ToolExecuteOptions
  ): Promise<ManagedToolResult> {
    if (this.canceled) throw new Error('canceled');
    const bypass = opts?.bypassConcurrency === true;
    if (this.mapping.size === 0) { this.listTools(); }
    const effective = this.resolveName(name);
    const entry = this.mapping.get(effective);
    if (entry === undefined) throw new Error(`Unknown tool: ${name}`);

    const provider = entry.provider;
    const kind = entry.kind;
    let queueName: string | undefined;
    let queueResult: AcquireResult | undefined;
    let queueController: AbortController | undefined;
    let acquiredQueue = false;
    if (!bypass && this.shouldQueue(kind)) {
      queueName = entry.queueName ?? 'default';
      queueController = new AbortController();
      this.pendingQueueControllers.add(queueController);
      try {
        queueResult = await queueManager.acquire(queueName, { signal: queueController.signal, agentId: this.sessionInfo.agentId, toolName: effective });
        this.pendingQueueControllers.delete(queueController);
        queueController = undefined;
        acquiredQueue = true;
        if (queueResult.queued) {
          this.logQueued(queueName, queueResult, ctx, kind);
        }
      } catch (error) {
        if (queueController !== undefined) {
          this.pendingQueueControllers.delete(queueController);
          queueController = undefined;
        }
        if (error instanceof QueueAbortError) {
          this.logQueued(error.queueName, error.info, ctx, kind);
        }
        if (error instanceof DOMException && error.name === 'AbortError') {
          throw new Error('canceled');
        }
        throw error;
      }
    }

    try {
      const toolIdentity = (() => {
      try {
        return provider.resolveToolIdentity(effective);
      } catch {
        return { namespace: provider.namespace, tool: effective };
      }
    })();
    const composedToolName = `${toolIdentity.namespace}__${toolIdentity.tool}`;
    const logProviderLabel = (() => {
      try {
        return provider.resolveLogProvider(effective);
      } catch {
        return `${kind}:${toolIdentity.namespace}`;
      }
    })();
    const logProviderNamespace = toolIdentity.namespace;
    addSpanAttributes({
      'ai.tool.name': composedToolName,
      'ai.tool.provider': logProviderNamespace,
      'ai.tool.kind': kind,
    });
    // no spans; opTree is canonical
    // Begin hierarchical op (Option C).
    // Only treat sub-agents and tool_output as child 'session' ops.
    // Internal agent-scoped tools (provider.namespace === 'agent') remain regular 'tool' ops.
    const isSessionTool = kind === 'agent' && (provider.namespace === 'subagent' || provider.namespace === 'tool-output');
    const opKind = isSessionTool ? 'session' : 'tool';
    const opId = (() => {
      try { return this.opTree.beginOp(ctx.turn, opKind, { name: effective, provider: logProviderNamespace, kind }); } catch { return undefined; }
    })();
    const callPathLabel = this.getSessionCallPath();
    const agentIdLabel = this.getSessionAgentId();
    const agentPathLabel = this.getSessionAgentPath();
    const headendId = this.sessionInfo.headendId;
    const sessionTelemetryLabels: Record<string, string> | undefined = this.sessionInfo.telemetryLabels;
    const instrumentTool = opKind !== 'session' && provider.namespace !== 'agent';

    // For sub-agent session ops, attach a placeholder child session immediately so live views can show it
    try {
      if (opKind === 'session' && opId !== undefined) {
        const parentSession = this.opTree.getSession();
        const childName = effective.startsWith('agent__') ? effective.slice('agent__'.length) : effective;
        const baseAgentPath = (() => {
          const sessionAgentPath = this.sessionInfo.agentPath;
          if (typeof sessionAgentPath === 'string' && sessionAgentPath.length > 0) return sessionAgentPath;
          const sessionAgentId = this.sessionInfo.agentId;
          if (typeof sessionAgentId === 'string' && sessionAgentId.length > 0) return sessionAgentId;
          if (typeof parentSession.agentId === 'string' && parentSession.agentId.length > 0) return parentSession.agentId;
          if (typeof parentSession.callPath === 'string' && parentSession.callPath.length > 0) return parentSession.callPath;
          return 'agent';
        })();
        const childCallPath = appendCallPathSegment(baseAgentPath, childName);
        const stub: SessionNode = {
          id: `${Date.now().toString(36)}-stub`,
          traceId: parentSession.traceId,
          agentId: childName,
          callPath: childCallPath,
          sessionTitle: '',
          startedAt: Date.now(),
          turns: [],
        };
        this.opTree.attachChildSession(opId, stub);
        this.emitOpTree(this.opTree.getSession());
      }
    } catch (e) { warn(`unknown tool '${name}' after refresh: ${toErrorMessage(e)}`); }
    const requestMsg = formatToolRequestCompact(effective, parameters);
    // Log request (compact)
    const remoteIdentifier = `${kind}:${toolIdentity.namespace}:${toolIdentity.tool}`;
    const traceIdentifier = `trace:${kind}:${toolIdentity.namespace}`;
    const requestDetails: Record<string, LogDetailValue> = {
      tool: composedToolName,
      tool_namespace: toolIdentity.namespace,
      provider: logProviderLabel,
      tool_kind: kind,
      request_preview: requestMsg,
    };
    const reqLog: LogEntry = {
      timestamp: Date.now(),
      severity: 'VRB',
      turn: ctx.turn,
      subturn: ctx.subturn,
      direction: 'request',
      type: 'tool',
      toolKind: kind,
      remoteIdentifier,
      fatal: false,
      message: 'started',
      details: requestDetails,
    };
    // Single emission point via session logger
    this.log(reqLog, { opId });

    if (instrumentTool) {
      this.progress?.toolStarted({
        callPath: callPathLabel,
        agentId: agentIdLabel,
        agentPath: agentPathLabel,
        tool: { name: composedToolName, provider: logProviderNamespace },
      });
    }
    const normalizeParameters = (toolName: string, a: Record<string, unknown>): Record<string, unknown> => {
      if (toolName === 'github__search_code') {
        const getStr = (obj: unknown, k: string): string | undefined => {
          if (obj !== null && typeof obj === 'object') {
            const v = (obj as Record<string, unknown>)[k];
            if (typeof v === 'string') {
              const t = v.trim();
              return t.length > 0 ? t : undefined;
            }
          }
          return undefined;
        };
        const existing = getStr(a, 'q');
        if (typeof existing === 'string') return a;
        const parts: string[] = [];
        const query = getStr(a, 'query');
        const repo = getStr(a, 'repo');
        const path = getStr(a, 'path');
        const languageRaw = getStr(a, 'language');
        if (typeof query === 'string' && query.length > 0) parts.push(query);
        if (typeof repo === 'string' && repo.length > 0) parts.push(`repo:${repo}`);
        if (typeof path === 'string' && path.length > 0) parts.push(`path:${path}`);
        if (typeof languageRaw === 'string' && languageRaw.length > 0) {
          const norm = languageRaw.replace(/\s+OR\s+/gi, ',').replace(/\|/g, ',');
          const toks = norm.split(/[\,\s]+/).map((t) => t.trim()).filter((t) => t.length > 0);
          const langSet = new Set<string>();
          const extSet = new Set<string>();
          const mapSyn = (s: string): string => (s === 'js' ? 'javascript' : s === 'ts' ? 'typescript' : s);
          toks.forEach((t) => {
            const low = mapSyn(t.toLowerCase());
            if (low === 'jsx') { extSet.add('jsx'); return; }
            if (low === 'tsx') { extSet.add('tsx'); return; }
            langSet.add(low);
          });
          langSet.forEach((l) => { parts.push(`language:${l}`); });
          extSet.forEach((e) => { parts.push(`extension:${e}`); });
        }
        const q = parts.join(' ').trim();
        if (q.length > 0) return { ...a, q };
        return a;
      }
      return a;
    };
    const preparedParameters = normalizeParameters(effective, parameters);
    const isBatchTool = toolIdentity.namespace === 'agent' && toolIdentity.tool === 'batch';
    const batchToolSummary = (() => {
      if (!isBatchTool) return undefined;
      const calls = (preparedParameters as { calls?: unknown }).calls;
      if (!Array.isArray(calls)) return undefined;
      const names = calls
        .map((entry) => {
          if (entry !== null && typeof entry === 'object') {
            const toolName = (entry as Record<string, unknown>).tool;
            if (typeof toolName === 'string' && toolName.length > 0) return toolName;
          }
          return undefined;
        })
        .filter((value): value is string => typeof value === 'string');
      if (names.length === 0) return undefined;
      return names.join(', ');
    })();

    try {
      if (opId !== undefined) {
        const paramSize = (() => { try { return JSON.stringify(parameters).length; } catch { return undefined; } })();
        this.opTree.setRequest(opId, { kind: 'tool', payload: parameters, size: paramSize });
      }
    } catch (e) { warn(`setRequest failed: ${toErrorMessage(e)}`); }
    // Optional full request trace (parameters JSON)
    if (this.opts.traceTools === true) {
      const fullParams = (() => { try { return JSON.stringify(parameters, null, 2); } catch { return '[unserializable-parameters]'; } })();
      const traceReq: LogEntry = {
        timestamp: Date.now(),
        severity: 'TRC',
        turn: ctx.turn,
        subturn: ctx.subturn,
        direction: 'request',
        type: 'tool',
        toolKind: kind,
        remoteIdentifier: traceIdentifier,
        fatal: false,
        message: `REQUEST ${effective}\n${fullParams}`,
      };
      this.log(traceReq, { opId });
    }
    const start = Date.now();
    const serializedParameters = (() => {
      try { return JSON.stringify(parameters); } catch { return undefined; }
    })();
    const charactersIn = serializedParameters !== undefined ? serializedParameters.length : 0;
    const inputBytes = serializedParameters !== undefined ? Buffer.byteLength(serializedParameters, 'utf8') : 0;

    const STATUS_FAILED = 'failed' as const;
    const STATUS_ERROR = 'error' as const;
    const isInternalAgentTool = kind === 'agent' && provider.namespace === 'agent';
    const budgetCallbacks = this.budgetCallbacks;
    if (budgetCallbacks !== undefined && !isInternalAgentTool && !budgetCallbacks.canExecuteTool()) {
      const latency = Date.now() - start;
      const failureReason = 'budget_exceeded_prior_tool';
      const failureStub = '(tool failed: context window budget exceeded; previous tool overflowed. Session will conclude after this turn.)';
      const finalBytesForSkip = Buffer.byteLength(failureStub, 'utf8');
      const dropDetails: Record<string, LogDetailValue> = {
        tool: composedToolName,
        tool_namespace: toolIdentity.namespace,
        provider: logProviderLabel,
        tool_kind: kind,
        input_bytes: inputBytes,
        final_bytes: finalBytesForSkip,
        original_bytes_unknown: true,
        truncated_pct: 0,
        reason: failureReason,
        dropped: true,
        skipped_execution: true,
        latency_ms: latency,
      };
      if (batchToolSummary !== undefined) dropDetails.batch_tools = batchToolSummary;
      const dropLog: LogEntry = {
        timestamp: Date.now(),
        severity: 'WRN',
        turn: ctx.turn,
        subturn: ctx.subturn,
        direction: 'response',
        type: 'tool',
        toolKind: kind,
        remoteIdentifier,
        fatal: false,
        message: `Tool '${composedToolName}' skipped execution: token budget exceeded by previous tool.`,
        details: dropDetails,
      };
      this.log(dropLog, { opId });
      if (instrumentTool) {
        const failureMetrics: ProgressMetrics = { latencyMs: latency, charactersIn, charactersOut: 0 };
        this.progress?.toolFinished({
          callPath: callPathLabel,
          agentId: agentIdLabel,
          agentPath: agentPathLabel,
          tool: { name: composedToolName, provider: logProviderNamespace },
          metrics: failureMetrics,
          error: failureReason,
          status: STATUS_FAILED,
        });
      }
      const accDrop: AccountingEntry = {
        type: 'tool',
        timestamp: start,
        status: STATUS_FAILED,
        latency,
        mcpServer: logProviderNamespace,
        command: name,
        charactersIn,
        charactersOut: 0,
        error: failureReason,
      };
      try { if (opId !== undefined) this.opTree.appendAccounting(opId, accDrop); } catch (e) { warn(`tools accounting append failed: ${toErrorMessage(e)}`); }
      const errorMetricsDrop = {
        agentId: agentIdLabel,
        callPath: callPathLabel,
        headendId,
        toolName: composedToolName,
        toolKind: kind,
        provider: kind === 'mcp' ? `${kind}:${logProviderNamespace}` : logProviderLabel,
        status: STATUS_ERROR,
        errorType: failureReason,
        latencyMs: latency,
        inputBytes,
        outputBytes: 0,
        ...(sessionTelemetryLabels !== undefined ? { customLabels: sessionTelemetryLabels } : {}),
      } satisfies ToolMetricsRecord;
      recordToolMetrics(errorMetricsDrop);
      addSpanAttributes({ 'ai.tool.status': STATUS_ERROR, 'ai.tool.latency_ms': latency });
      addSpanEvent(TOOL_MANAGE_ERROR_EVENT, { 'ai.tool.name': composedToolName, 'ai.tool.error': failureReason });
      recordSpanError(failureReason);
      try {
        if (opId !== undefined) {
          this.opTree.endOp(opId, STATUS_FAILED, { latency, error: failureReason });
        }
      } catch (e) { warn(`tools endOp failed: ${toErrorMessage(e)}`); }
      return {
        ok: false,
        result: failureStub,
        providerLabel: logProviderNamespace,
        latency,
        charactersIn,
        charactersOut: failureStub.length,
        tokens: 0,
        dropped: true,
        reason: failureReason,
      };
    }

    let exec: ToolExecuteResult | undefined;
    let errorMessage: string | undefined;
    let cacheHitEntry: CacheLookupResult | undefined;
    const cache = this.cacheProvider?.();
    const cacheTtlMs = (kind === 'mcp' || kind === 'rest')
      ? this.toolCacheResolver?.resolveTtlMs({ kind, namespace: toolIdentity.namespace, tool: toolIdentity.tool })
      : undefined;
    const cacheKeyPayload = (cache !== undefined && typeof cacheTtlMs === 'number' && cacheTtlMs > 0)
      ? {
          v: 1,
          kind: 'tool',
          tool: { kind, namespace: toolIdentity.namespace, name: toolIdentity.tool },
          parameters: preparedParameters,
        }
      : undefined;

    if (cache !== undefined && cacheKeyPayload !== undefined && typeof cacheTtlMs === 'number' && cacheTtlMs > 0) {
      const hit = await cache.get(cacheKeyPayload, cacheTtlMs, Date.now());
      if (hit !== undefined && isCachedToolResult(hit.value)) {
        cacheHitEntry = hit.entry;
        exec = { ok: true, result: hit.value.result, latencyMs: 0, kind, namespace: toolIdentity.namespace };
        const cacheDetails: Record<string, LogDetailValue> = {
          cache_kind: 'tool',
          cache_key: cacheHitEntry.keyHash,
          cache_age_ms: cacheHitEntry.ageMs,
          cache_current_agent: agentIdLabel,
          cache_stored_agent: cacheHitEntry.metadata.agentName ?? '',
          cache_current_tool: toolIdentity.tool,
          cache_stored_tool: cacheHitEntry.metadata.toolName ?? '',
          cache_current_tool_namespace: toolIdentity.namespace,
          cache_stored_tool_namespace: cacheHitEntry.metadata.toolNamespace ?? '',
        };
        if (this.sessionInfo.agentHash !== undefined) {
          cacheDetails.cache_current_agent_hash = this.sessionInfo.agentHash;
        }
        if (cacheHitEntry.metadata.agentHash !== undefined) {
          cacheDetails.cache_stored_agent_hash = cacheHitEntry.metadata.agentHash;
        }
        const cacheLog: LogEntry = {
          timestamp: Date.now(),
          severity: 'VRB',
          turn: ctx.turn,
          subturn: ctx.subturn,
          direction: 'response',
          type: 'tool',
          toolKind: kind,
          remoteIdentifier,
          fatal: false,
          message: `cache hit: ${composedToolName}`,
          details: cacheDetails,
        };
        this.log(cacheLog, { opId });
      }
    }

    const withTimeout = async <T>(p: Promise<T>, timeoutMs: number | undefined, onTimeout?: () => void | Promise<void>): Promise<T> => {
      if (typeof timeoutMs !== 'number' || timeoutMs <= 0) return await p;
      return await new Promise<T>((resolve, reject) => {
        let settled = false;
        let timer: ReturnType<typeof setTimeout>;
        const finalize = (action: () => void): void => {
          if (settled) return;
          settled = true;
          clearTimeout(timer);
          action();
        };
        const rejectWith = (error: unknown): void => {
          const normalized = error instanceof Error ? error : new Error(toErrorMessage(error));
          finalize(() => { reject(normalized); });
        };
        timer = setTimeout(() => {
          if (settled) return;
          if (onTimeout === undefined) {
            rejectWith(new Error('Tool execution timed out'));
            return;
          }
          try {
            Promise.resolve(onTimeout())
              .then(() => {
                rejectWith(new Error('Tool execution timed out'));
              })
              .catch((err: unknown) => {
                warn(`provider cancelTool failed: ${toErrorMessage(err)}`);
                rejectWith(err);
              });
          } catch (err: unknown) {
            warn(`provider cancelTool threw: ${toErrorMessage(err)}`);
            rejectWith(err);
          }
        }, timeoutMs);

        p.then((value) => {
          finalize(() => { resolve(value); });
        }).catch((error: unknown) => {
          rejectWith(error);
        });
      });
    };

    if (exec === undefined) {
      try {
        // Do not apply parent-level withTimeout to sub-agents; they manage their own timing
        const isSessionToolExec = isSessionTool;
        if (isSessionToolExec) {
          const parentOpPath = (() => { try { return (opId !== undefined) ? this.opTree.getOpPath(opId) : undefined; } catch { return undefined; } })();
          const onChildOpTree = (tree: SessionNode) => {
            try {
              if (opId !== undefined) {
                this.opTree.attachChildSession(opId, tree);
                this.emitOpTree(this.opTree.getSession());
              }
            } catch (e) { warn(`onChildOpTree snapshot failed: ${toErrorMessage(e)}`); }
          };
          exec = await provider.execute(effective, preparedParameters, { ...opts, timeoutMs: undefined, trace: this.opts.traceTools, onChildOpTree, parentOpPath, parentContext: ctx });
        } else {
          const providerTimeout = opts?.timeoutMs ?? this.opts.toolTimeout;
          const execPromise = provider.execute(
            effective,
            preparedParameters,
            { ...opts, timeoutMs: providerTimeout, trace: this.opts.traceTools, parentContext: ctx }
          );
          if (opts?.disableGlobalTimeout === true) {
            exec = await execPromise;
          } else {
            const timeoutCallback = (): Promise<void> => provider.cancelTool(effective, { reason: 'timeout', context: ctx });
            exec = await withTimeout(execPromise, this.opts.toolTimeout, timeoutCallback);
          }
        }
      } catch (e) {
        errorMessage = toErrorMessage(e);
      }
    }

    const latency = Date.now() - start;

    const isFailed = (() => {
      if (exec === undefined) return true;
      return !exec.ok;
    })();
    if (isFailed) {
      const errorMessageDetail = (() => {
        if (typeof exec?.error === 'string' && exec.error.length > 0) return exec.error;
        if (typeof errorMessage === 'string' && errorMessage.length > 0) return errorMessage;
        return 'execution_failed';
      })();
      const providerFieldFailed = kind === 'mcp'
        ? `${kind}:${exec?.namespace ?? logProviderNamespace}`
        : logProviderLabel;
      const failureDetails: Record<string, LogDetailValue> = {
        tool: composedToolName,
        tool_namespace: toolIdentity.namespace,
        provider: providerFieldFailed,
        tool_kind: kind,
        latency_ms: latency,
        input_bytes: inputBytes,
        output_bytes: 0,
        error_message: errorMessageDetail,
      };
      if (batchToolSummary !== undefined) {
        failureDetails.batch_tools = batchToolSummary;
      }
      const msg = errorMessageDetail;
      if (this.opts.traceTools === true) {
        const traceErr: LogEntry = {
          timestamp: Date.now(),
          severity: 'TRC',
          turn: ctx.turn,
          subturn: ctx.subturn,
          direction: 'response',
          type: 'tool',
          toolKind: kind,
          remoteIdentifier: traceIdentifier,
          fatal: false,
          message: `ERROR ${effective}\n${msg}`,
        };
      this.log(traceErr, { opId });
      }
      const errMessage = batchToolSummary !== undefined
        ? `error ${composedToolName}: ${msg} (calls: ${batchToolSummary})`
        : `error ${composedToolName}: ${msg}`;
      const errLog: LogEntry = {
        timestamp: Date.now(),
        severity: 'WRN',
        turn: ctx.turn,
        subturn: ctx.subturn,
        direction: 'response',
        type: 'tool',
        toolKind: kind,
        remoteIdentifier,
        fatal: false,
        message: errMessage,
        details: failureDetails,
      };
      this.log(errLog, { opId });
      if (instrumentTool) {
        const failureMetrics: ProgressMetrics = {
          latencyMs: latency,
          charactersIn,
          charactersOut: 0,
        };
        this.progress?.toolFinished({
          callPath: callPathLabel,
          agentId: agentIdLabel,
          agentPath: agentPathLabel,
          tool: { name: composedToolName, provider: logProviderNamespace },
          metrics: failureMetrics,
          error: msg,
          status: 'failed',
        });
      }
      const acc: AccountingEntry = {
        type: 'tool', timestamp: start, status: 'failed', latency,
        mcpServer: kind === 'mcp' ? (exec?.namespace ?? logProviderNamespace) : logProviderNamespace,
        command: name,
        charactersIn,
        charactersOut: 0,
        error: msg,
      };
      try { if (opId !== undefined) this.opTree.appendAccounting(opId, acc); } catch (e) { warn(`tools accounting append failed: ${toErrorMessage(e)}`); }
      const errorMetrics = {
        agentId: agentIdLabel,
        callPath: callPathLabel,
        headendId,
        toolName: composedToolName,
        toolKind: kind,
        provider: kind === 'mcp' ? `${kind}:${logProviderNamespace}` : logProviderLabel,
        status: 'error',
        errorType: msg,
        latencyMs: latency,
        inputBytes,
        outputBytes: 0,
        ...(sessionTelemetryLabels !== undefined ? { customLabels: sessionTelemetryLabels } : {}),
      } satisfies ToolMetricsRecord;
      recordToolMetrics(errorMetrics);
      addSpanAttributes({ 'ai.tool.status': 'error', 'ai.tool.latency_ms': latency });
      addSpanEvent(TOOL_MANAGE_ERROR_EVENT, { 'ai.tool.name': composedToolName, 'ai.tool.error': msg });
      recordSpanError(msg);
      // Accounting is recorded in opTree op context only via attach; keep arrays via SessionManager callbacks
    try {
        if (opId !== undefined) {
          this.opTree.endOp(opId, 'failed', { latency, error: msg });
        }
      } catch (e) { warn(`tools endOp failed: ${toErrorMessage(e)}`); }
      throw new Error(msg);
    }

    const safeExec = (() => { if (exec === undefined) { throw new Error('unexpected_undefined_execution_result'); } return exec; })();
    const raw = typeof safeExec.result === 'string' ? safeExec.result : '';
    const rawBytes = Buffer.byteLength(raw, 'utf8');
    // Optional full response trace (raw, before truncation)
    if (this.opts.traceTools === true) {
      // Prefer provider-supplied raw payload when available
      const rawPayload = (() => {
        const extra = safeExec.extras;
        if (extra !== undefined && typeof extra === 'object') {
          const rp = (extra as { rawResponse?: unknown }).rawResponse;
          if (typeof rp === 'string' && rp.length > 0) return rp;
        }
        return raw;
      })();
      const traceRes: LogEntry = {
        timestamp: Date.now(),
        severity: 'TRC',
        turn: ctx.turn,
        subturn: ctx.subturn,
        direction: 'response',
        type: 'tool',
        toolKind: kind,
        remoteIdentifier: traceIdentifier,
        fatal: false,
        message: `RESPONSE ${effective}\n${rawPayload}`,
      };
      this.log(traceRes, { opId });
    }
    const limit = this.opts.toolResponseMaxBytes;
    const providerLabel = kind === 'mcp' ? safeExec.namespace : logProviderNamespace; // namespace label
    const providerField = kind === 'mcp'
      ? `${kind}:${providerLabel}`
      : logProviderLabel;
    const toolOutputHandler = this.toolOutputHandler;
    const isToolOutputTool = toolIdentity.tool === 'tool_output' || effective === 'tool_output';
    const canHandleToolOutput = toolOutputHandler !== undefined && !isToolOutputTool && !isInternalAgentTool;
    const toolArgsJson = (() => {
      try {
        return JSON.stringify(preparedParameters);
      } catch {
        return '{}';
      }
    })();
    let result = raw;
    let storedHandle: ToolOutputHandleResult | undefined;
    let resultTruncated = false;
    const logToolOutputHandle = (stored: ToolOutputHandleResult): void => {
      const warnDetails: Record<string, LogDetailValue> = {
        tool: composedToolName,
        tool_namespace: toolIdentity.namespace,
        provider: providerField,
        tool_kind: kind,
        handle: stored.handle,
        reason: stored.reason,
        bytes: stored.stats.bytes,
        lines: stored.stats.lines,
        tokens: stored.stats.tokens,
        tool_output: true,
      };
      if (batchToolSummary !== undefined) {
        warnDetails.batch_tools = batchToolSummary;
      }
      const warnLog: LogEntry = {
        timestamp: Date.now(),
        severity: 'WRN',
        turn: ctx.turn,
        subturn: ctx.subturn,
        direction: 'response',
        type: 'tool',
        toolKind: kind,
        remoteIdentifier,
        fatal: false,
        message: `Tool '${composedToolName}' output stored for tool_output (${stored.reason}).`,
        details: warnDetails,
      };
      this.log(warnLog, { opId });
    };
    if (canHandleToolOutput) {
      try {
        storedHandle = await toolOutputHandler.maybeStore({
          toolName: effective,
          toolArgsJson,
          output: raw,
          sizeLimitBytes: limit,
          budgetCallbacks,
          sourceTarget: opts?.sourceTarget,
        });
      } catch (e) {
        const msg = toErrorMessage(e);
        const errLog: LogEntry = {
          timestamp: Date.now(),
          severity: 'ERR',
          turn: ctx.turn,
          subturn: ctx.subturn,
          direction: 'response',
          type: 'tool',
          toolKind: kind,
          remoteIdentifier,
          fatal: false,
          message: `tool_output store failed: ${msg}`,
          details: {
            tool: composedToolName,
            provider: providerField,
          },
        };
        this.log(errLog, { opId });
      }
      if (storedHandle !== undefined) {
        result = storedHandle.message;
        logToolOutputHandle(storedHandle);
      }
    }

    if (!canHandleToolOutput && !isBatchTool && typeof limit === 'number' && limit > 0) {
      const originalBytes = Buffer.byteLength(result, 'utf8');
      if (originalBytes > limit) {
        const truncated = truncateToBytesWithInfo(result, limit);
        const fallback = truncated?.value ?? Buffer.from(result, 'utf8').subarray(0, limit).toString('utf8');
        const finalValue = fallback.length > 0 ? fallback : ' ';
        const finalBytes = Buffer.byteLength(finalValue, 'utf8');
        const truncatedPct = computeTruncationPercent(originalBytes, finalBytes);
        result = finalValue;
        resultTruncated = true;
        const warnDetails: Record<string, LogDetailValue> = {
          tool: composedToolName,
          tool_namespace: toolIdentity.namespace,
          provider: providerField,
          tool_kind: kind,
          original_bytes: originalBytes,
          final_bytes: finalBytes,
          truncated_pct: truncatedPct,
          reason: 'size_cap_truncated',
          truncated: true,
        };
        if (batchToolSummary !== undefined) warnDetails.batch_tools = batchToolSummary;
        const warnLog: LogEntry = {
          timestamp: Date.now(),
          severity: 'WRN',
          turn: ctx.turn,
          subturn: ctx.subturn,
          direction: 'response',
          type: 'tool',
          toolKind: kind,
          remoteIdentifier,
          fatal: false,
          message: `Tool '${composedToolName}' output truncated to toolResponseMaxBytes (${String(limit)} bytes).`,
          details: warnDetails,
        };
        this.log(warnLog, { opId });
      }
    }

    // Ensure non-empty result for downstream providers that expect a non-empty tool output
    if (result.length === 0) result = ' ';
    let resultBytes = Buffer.byteLength(result, 'utf8');
    let reservation: ToolBudgetReservation = (!isInternalAgentTool && budgetCallbacks !== undefined)
      ? await budgetCallbacks.reserveToolOutput(result)
      : { ok: true };

    if (!reservation.ok && canHandleToolOutput && storedHandle === undefined && budgetCallbacks !== undefined) {
      try {
        const forcedHandle = await toolOutputHandler.maybeStore({
          toolName: effective,
          toolArgsJson,
          output: raw,
          sizeLimitBytes: limit,
          budgetCallbacks,
          sourceTarget: opts?.sourceTarget,
          forceReason: 'reserve_failed',
        });
        if (forcedHandle !== undefined) {
          storedHandle = forcedHandle;
          result = forcedHandle.message;
          if (result.length === 0) result = ' ';
          resultBytes = Buffer.byteLength(result, 'utf8');
          logToolOutputHandle(forcedHandle);
          reservation = await budgetCallbacks.reserveToolOutput(result);
        }
      } catch (e) {
        warn(`tool_output reserve fallback failed: ${toErrorMessage(e)}`);
      }
    }

    if (!reservation.ok) {
      const failureReason = reservation.reason ?? 'token_budget_exceeded';
      const failureStub = '(tool failed: context window budget exceeded)';
      const dropResult = storedHandle !== undefined ? result : failureStub;
      const originalBytesForDrop = resultBytes;
      const finalBytesForDrop = Buffer.byteLength(dropResult, 'utf8');
      const truncatedPct = computeTruncationPercent(originalBytesForDrop, finalBytesForDrop);
      const dropDetails: Record<string, LogDetailValue> = {
        tool: composedToolName,
        tool_namespace: toolIdentity.namespace,
        provider: providerField,
        tool_kind: kind,
        original_bytes: originalBytesForDrop,
        final_bytes: finalBytesForDrop,
        truncated_pct: truncatedPct,
        raw_bytes: rawBytes,
        tokens_estimated: reservation.tokens ?? 0,
        reason: failureReason,
        truncated: result !== raw,
        dropped: true,
        latency_ms: latency,
      };
      if (batchToolSummary !== undefined) dropDetails.batch_tools = batchToolSummary;
      const dropLog: LogEntry = {
        timestamp: Date.now(),
        severity: 'WRN',
        turn: ctx.turn,
        subturn: ctx.subturn,
        direction: 'response',
        type: 'tool',
        toolKind: kind,
        remoteIdentifier,
        fatal: false,
        message: `Tool '${composedToolName}' output dropped after execution: token budget exceeded.`,
        details: dropDetails,
      };
      this.log(dropLog, { opId });
      if (instrumentTool) {
        const failureMetrics: ProgressMetrics = {
          latencyMs: latency,
          charactersIn,
          charactersOut: dropResult.length,
        };
        this.progress?.toolFinished({
          callPath: callPathLabel,
          agentId: agentIdLabel,
          agentPath: agentPathLabel,
          tool: { name: composedToolName, provider: logProviderNamespace },
          metrics: failureMetrics,
          error: failureReason,
          status: STATUS_FAILED,
        });
      }
      const accDrop: AccountingEntry = {
        type: 'tool',
        timestamp: start,
        status: STATUS_FAILED,
        latency,
        mcpServer: providerLabel,
        command: name,
        charactersIn,
        charactersOut: dropResult.length,
        error: failureReason,
      };
      try { if (opId !== undefined) this.opTree.appendAccounting(opId, accDrop); } catch (e) { warn(`tools accounting append failed: ${toErrorMessage(e)}`); }
      const errorMetricsDrop = {
        agentId: agentIdLabel,
        callPath: callPathLabel,
        headendId,
        toolName: composedToolName,
        toolKind: kind,
        provider: kind === 'mcp' ? `${kind}:${logProviderNamespace}` : logProviderLabel,
        status: STATUS_ERROR,
        errorType: failureReason,
        latencyMs: latency,
        inputBytes,
        outputBytes: Buffer.byteLength(dropResult, 'utf8'),
        ...(sessionTelemetryLabels !== undefined ? { customLabels: sessionTelemetryLabels } : {}),
      } satisfies ToolMetricsRecord;
      recordToolMetrics(errorMetricsDrop);
      addSpanAttributes({ 'ai.tool.status': STATUS_ERROR, 'ai.tool.latency_ms': latency });
      addSpanEvent(TOOL_MANAGE_ERROR_EVENT, { 'ai.tool.name': composedToolName, 'ai.tool.error': failureReason });
      recordSpanError(failureReason);
      try {
        if (opId !== undefined) {
          this.opTree.endOp(opId, STATUS_FAILED, { latency, error: failureReason });
        }
      } catch (e) { warn(`tools endOp failed: ${toErrorMessage(e)}`); }
      return {
        ok: false,
        result: dropResult,
        providerLabel,
        latency,
        charactersIn,
        charactersOut: dropResult.length,
        tokens: reservation.tokens,
        dropped: true,
        reason: failureReason,
      };
    }

    const responseDetails: Record<string, LogDetailValue> = {
      tool: composedToolName,
      tool_namespace: toolIdentity.namespace,
      provider: providerField,
      tool_kind: kind,
      result_chars: result.length,
      result_bytes: resultBytes,
      latency_ms: latency,
    };
    if (batchToolSummary !== undefined) {
      responseDetails.batch_tools = batchToolSummary;
    }
    if (reservation.tokens !== undefined) {
      responseDetails.tokens_estimated = reservation.tokens;
    }
    responseDetails.dropped = false;
    responseDetails.truncated = storedHandle !== undefined;
    if (storedHandle !== undefined) {
      responseDetails.tool_output_handle = storedHandle.handle;
      responseDetails.tool_output_reason = storedHandle.reason;
    }
    const resPreview = (() => {
      try {
        const jsonified = JSON.stringify(raw);
        if (jsonified.length > 100) return `${jsonified.slice(0, 100)}…`;
        return jsonified;
      } catch {
        const normalized = raw.replace(/\s+/g, ' ').trim();
        if (normalized.length > 100) return `${normalized.slice(0, 100)}…`;
        if (normalized.length === 0) return '<empty>';
        return normalized;
      }
    })();
    responseDetails.preview = resPreview;
    const resLog: LogEntry = {
      timestamp: Date.now(),
      severity: 'VRB',
      turn: ctx.turn,
      subturn: ctx.subturn,
      direction: 'response',
      type: 'tool',
      toolKind: kind,
      remoteIdentifier,
      fatal: false,
      message: `ok preview: ${resPreview}`,
      details: responseDetails,
    };
    this.log(resLog, { opId });
    if (instrumentTool) {
      const successMetrics: ProgressMetrics = {
        latencyMs: latency,
        charactersIn,
        charactersOut: result.length,
      };
      this.progress?.toolFinished({
        callPath: callPathLabel,
        agentId: agentIdLabel,
        agentPath: agentPathLabel,
        tool: { name: composedToolName, provider: logProviderNamespace },
        metrics: successMetrics,
        status: 'ok',
      });
    }
    try {
      if (opId !== undefined) {
        this.opTree.setResponse(opId, { payload: result, size: resultBytes, truncated: storedHandle !== undefined || resultTruncated });
      }
    } catch (e) { warn(`tools setResponse failed: ${toErrorMessage(e)}`); }

    const accOk: AccountingEntry = {
      type: 'tool',
      timestamp: start,
      status: 'ok',
      latency,
      mcpServer: providerLabel,
      command: name,
      charactersIn,
      charactersOut: result.length,
    };
    try { this.recordAccounting?.(accOk); } catch (e) { warn(`tools accounting dispatch failed: ${toErrorMessage(e)}`); }
    try { if (opId !== undefined) this.opTree.appendAccounting(opId, accOk); } catch (e) { warn(`tools accounting append failed: ${toErrorMessage(e)}`); }
    // accounting will be attached under op
    // Optional: child accounting (from sub-agents)
    // Do NOT forward child accounting to the parent execution tree here.
    // Sub-agents already emit their own accounting events (wired to the same SessionManager),
    // and their full accounting is attached to the opTree via attachChildSession above.
    // Re-injecting entries here would double-count tokens/cost in Slack progress at finish.
    // end via opTree only
    try {
      if (opId !== undefined) {
        // Attach child session tree when provider is 'agent'
        try {
          if (opKind === 'session') {
            const maybe = (safeExec.extras as { childOpTree?: SessionNode } | undefined)?.childOpTree;
            if (maybe !== undefined) {
              this.opTree.attachChildSession(opId, maybe);
            }
          }
        } catch (e) { warn(`tools child attach failed: ${toErrorMessage(e)}`); }
        this.opTree.endOp(opId, 'ok', { latency, size: result.length });
        try {
          this.emitOpTree(this.opTree.getSession());
        } catch (e) { warn(`tools opTree emit failed: ${toErrorMessage(e)}`); }
      }
    } catch (e) { warn(`tools finalize failed: ${toErrorMessage(e)}`); }
    const successMetrics = {
      agentId: agentIdLabel,
      callPath: callPathLabel,
      headendId,
      toolName: composedToolName,
      toolKind: kind,
      provider: kind === 'mcp' ? `${kind}:${logProviderNamespace}` : logProviderLabel,
      status: 'success',
      latencyMs: latency,
      inputBytes,
      outputBytes: resultBytes,
      ...(sessionTelemetryLabels !== undefined ? { customLabels: sessionTelemetryLabels } : {}),
    } satisfies ToolMetricsRecord;
    recordToolMetrics(successMetrics);
    addSpanAttributes({
      'ai.tool.status': 'success',
      'ai.tool.latency_ms': latency,
      'ai.tool.input_bytes': charactersIn,
      'ai.tool.output_bytes': result.length,
      'ai.tool.provider_label': providerLabel,
    });
    if (
      cache !== undefined
      && cacheKeyPayload !== undefined
      && cacheHitEntry === undefined
      && typeof cacheTtlMs === 'number'
      && cacheTtlMs > 0
      && storedHandle === undefined
      && !isToolOutputTool
    ) {
      const metadata: CacheEntryMetadata = {
        kind: 'tool',
        agentName: agentIdLabel,
        toolName: toolIdentity.tool,
        toolNamespace: toolIdentity.namespace,
        agentHash: this.sessionInfo.agentHash,
      };
      try {
        await cache.set(cacheKeyPayload, cacheTtlMs, { result }, metadata, Date.now());
      } catch (e) {
        warn(`tool cache write failed: ${toErrorMessage(e)}`);
      }
    }
    return {
      ok: true,
      result,
      providerLabel,
      latency,
      charactersIn,
      charactersOut: result.length,
      tokens: reservation.tokens,
      dropped: false,
    };
    } finally {
      if (acquiredQueue && queueName !== undefined) {
        queueManager.release(queueName);
      }
      if (queueController !== undefined) {
        this.pendingQueueControllers.delete(queueController);
        try { queueController.abort(); } catch (e) { warn(`queue abort failed: ${toErrorMessage(e)}`); }
      }
    }
  }

  // Aggregate instructions across providers
  // Order: internal (final report + internal tools) first, then external tool providers
  getCombinedInstructions(): string {
    const external: string[] = [];
    const internal: string[] = [];
    // eslint-disable-next-line functional/no-loop-statements
    for (const provider of this.providers) {
      try {
        const instr = provider.getInstructions();
        if (typeof instr !== 'string') continue;
        const trimmed = instr.trim();
        if (trimmed.length === 0) continue;
        if (provider.kind === 'agent') {
          internal.push(trimmed);
        } else {
          external.push(trimmed);
        }
      } catch (e) { warn(`getCombinedInstructions failed: ${toErrorMessage(e)}`); }
    }
    if (external.length === 0 && internal.length === 0) {
      return '';
    }
    const sections: string[] = [];
    // Internal instructions first (final report format + internal tools like task_status)
    if (internal.length > 0) {
      sections.push(internal.join('\n\n'));
    }
    // External tool providers last
    if (external.length > 0) {
      sections.push('## Available Tool Providers');
      sections.push(external.join('\n\n'));
    }
    return sections.join('\n\n');
  }

  async cleanup(): Promise<void> {
    this.canceled = true;
    this.abortAllQueueControllers();
    // eslint-disable-next-line functional/no-loop-statements
    for (const p of this.providers) {
      const maybe = p as unknown as { cleanup?: () => Promise<void> };
      if (typeof maybe.cleanup === 'function') {
        try { await maybe.cleanup(); } catch (e) { warn(`provider cleanup failed: ${toErrorMessage(e)}`); }
      }
    }
  }

  // Explicit cancel entrypoint to abort queue and tear down providers
  cancel(): void {
    this.canceled = true;
    this.abortAllQueueControllers();
    // Best-effort async cleanup; do not await here to avoid blocking callers
    void this.cleanup();
  }

  private shouldQueue(kind: ToolKind): boolean {
    return kind === 'mcp' || kind === 'rest';
  }

  private logQueued(queueName: string, result: AcquireResult, ctx: ToolExecutionContext, kind: ToolKind): void {
    const entry: LogEntry = {
      timestamp: Date.now(),
      severity: 'VRB',
      turn: ctx.turn,
      subturn: ctx.subturn,
      direction: 'request',
      type: 'tool',
      toolKind: kind,
      remoteIdentifier: `queue:${queueName}`,
      fatal: false,
      message: 'queued',
      details: {
        queue: queueName,
        wait_ms: result.waitMs,
        queued_depth: result.queuedDepth,
        queue_capacity: result.capacity,
      },
    };
    this.log(entry);
  }

  private abortAllQueueControllers(): void {
    this.pendingQueueControllers.forEach((controller) => {
      try { controller.abort(); } catch (e) { warn(`queue controller abort failed: ${toErrorMessage(e)}`); }
    });
    this.pendingQueueControllers.clear();
  }
}
