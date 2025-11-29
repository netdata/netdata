import crypto from 'node:crypto';
import fs from 'node:fs';
// import os from 'node:os';
import path from 'node:path';

import { SpanStatusCode, type Attributes } from '@opentelemetry/api';

import type { OutputFormatId } from './formats.js';
import type { SessionNode } from './session-tree.js';
import type { AIAgentSessionConfig, AIAgentResult, ConversationMessage, LogEntry, AccountingEntry, Configuration, LLMAccountingEntry, MCPTool, ToolAccountingEntry, RestToolConfig, ProgressMetrics, SessionSnapshotPayload, AccountingFlushPayload, ReasoningLevel, ProviderReasoningMapping, ProviderReasoningValue, ToolChoiceMode, LogPayload, TurnRequestContextMetrics } from './types.js';

import { validateProviders, validateMCPServers, validatePrompts } from './config.js';
import { ContextGuard, type ContextGuardBlockedEntry, type ContextGuardEvaluation, type TargetContextConfig } from './context-guard.js';
import { FinalReportManager, FINAL_REPORT_SOURCE_TEXT_FALLBACK, FINAL_REPORT_SOURCE_TOOL_MESSAGE, type PendingFinalReportPayload } from './final-report-manager.js';
import { parseFrontmatter, parsePairs, extractBodyWithoutFrontmatter } from './frontmatter.js';
import { LLMClient } from './llm-client.js';
import { CONTEXT_FINAL_MESSAGE } from './llm-messages.js';
import { buildPromptVars, applyFormat, expandVars } from './prompt-builder.js';
import { SessionProgressReporter } from './session-progress-reporter.js';
import { SessionToolExecutor, type SessionContext } from './session-tool-executor.js';
import { SessionTreeBuilder } from './session-tree.js';
import { TurnRunner, type TurnRunnerContext, type TurnRunnerCallbacks } from './session-turn-runner.js';
import { SubAgentRegistry } from './subagent-registry.js';
import { recordContextGuardMetrics, runWithSpan, addSpanEvent } from './telemetry/index.js';
import { AgentProvider } from './tools/agent-provider.js';
import { InternalToolProvider } from './tools/internal-provider.js';
import { MCPProvider } from './tools/mcp-provider.js';
import { RestProvider } from './tools/rest-provider.js';
import { ToolsOrchestrator } from './tools/tools.js';
import { appendCallPathSegment, normalizeCallPath, sanitizeToolName, truncateUtf8WithNotice, warn } from './utils.js';
import { XmlToolTransport } from './xml-transport.js';

// Exit codes according to DESIGN.md
type ExitCode =
  // SUCCESS
  | 'EXIT-FINAL-ANSWER'
  | 'EXIT-MAX-TURNS-WITH-RESPONSE'
  | 'EXIT-USER-STOP'
  // LLM FAILURES
  | 'EXIT-NO-LLM-RESPONSE'
  | 'EXIT-EMPTY-RESPONSE'
  | 'EXIT-AUTH-FAILURE'
  | 'EXIT-QUOTA-EXCEEDED'
  | 'EXIT-MODEL-ERROR'
  // TOOL FAILURES
  | 'EXIT-TOOL-FAILURE'
  | 'EXIT-MCP-CONNECTION-LOST'
  | 'EXIT-TOOL-NOT-AVAILABLE'
  | 'EXIT-TOOL-TIMEOUT'
  // CONFIGURATION
  | 'EXIT-NO-PROVIDERS'
  | 'EXIT-INVALID-MODEL'
  | 'EXIT-MCP-INIT-FAILED'
  // TIMEOUT/LIMITS
  | 'EXIT-INACTIVITY-TIMEOUT'
  | 'EXIT-MAX-RETRIES'
  | 'EXIT-TOKEN-LIMIT'
  | 'EXIT-MAX-TURNS-NO-RESPONSE'
  // UNEXPECTED
  | 'EXIT-UNCAUGHT-EXCEPTION'
  | 'EXIT-SIGNAL-RECEIVED'
  | 'EXIT-UNKNOWN';

// PendingFinalReportSource type alias for internal use
type PendingFinalReportSource = typeof FINAL_REPORT_SOURCE_TEXT_FALLBACK | typeof FINAL_REPORT_SOURCE_TOOL_MESSAGE;

interface ToolSelection {
  availableTools: MCPTool[];
  allowedToolNames: Set<string>;
  toolsForTurn: MCPTool[];
}

// Immutable session class according to DESIGN.md

// TargetContextConfig, ContextGuardBlockedEntry, ContextGuardEvaluation moved to context-guard.ts


export class AIAgentSession {
  // Log identifiers (avoid duplicate string literals)
  private static readonly REMOTE_AGENT_TOOLS = 'agent:tools';
  private static readonly REMOTE_FINAL_TURN = 'agent:final-turn';
  private static readonly REMOTE_CONTEXT = 'agent:context';
  private static readonly REMOTE_ORCHESTRATOR = 'agent:orchestrator';
  private static readonly REMOTE_SANITIZER = 'agent:sanitizer';
  private static readonly FINAL_REPORT_TOOL = 'agent__final_report';
  private static readonly FINAL_REPORT_TOOL_ALIASES = new Set<string>(['agent__final_report', 'agent-final-report']);
  private static readonly FINAL_REPORT_SHORT = 'final_report';
  private static readonly TOOL_NO_OUTPUT = '(tool failed: context window budget exceeded)';
  private static readonly RETRY_ACTION_SKIP_PROVIDER = 'skip-provider';
  private static readonly CONTEXT_POST_SHRINK_WARN = 'Context guard post-shrink still over projected limit; continuing with forced final turn.';
  private static readonly CONTEXT_POST_SHRINK_TURN_WARN = 'Context guard post-shrink still over projected limit during turn execution; proceeding anyway.';
  readonly config: Configuration;
  readonly conversation: ConversationMessage[];
  readonly logs: LogEntry[];
  readonly accounting: AccountingEntry[];
  readonly success: boolean;
  readonly error?: string;
  private _currentTurn: number;
  get currentTurn(): number { return this._currentTurn; }
  
  private readonly llmClient: LLMClient;
  private readonly sessionConfig: AIAgentSessionConfig;
  private readonly abortSignal?: AbortSignal;
  private canceled = false;
  private readonly stopRef?: { stopping: boolean };
  private readonly toolFailureMessages = new Map<string, string>();
  private readonly toolFailureFallbacks: string[] = [];
  private readonly contextGuard: ContextGuard;
  private finalTurnEntryLogged = false;
  // Internal housekeeping notes
  private childConversations: { agentId?: string; toolName: string; promptPath: string; conversation: ConversationMessage[]; trace?: { originId?: string; parentId?: string; selfId?: string; callPath?: string } }[] = [];
  private readonly subAgents?: SubAgentRegistry;
  private readonly subAgentCount: number;
  private readonly agentPath: string;
  private readonly turnPathPrefix: string;
  private readonly subAgentToolNames: readonly string[];
  private readonly invokedSubAgents = new Set<string>();
  private readonly txnId: string;
  private readonly originTxnId?: string;
  private readonly parentTxnId?: string;
  private readonly callPath?: string;
  private readonly headendId?: string;
  private readonly telemetryLabels: Record<string, string>;
  // opTree-only tracking (canonical)
  private readonly toolsOrchestrator?: ToolsOrchestrator;
  private readonly opTree: SessionTreeBuilder;
  private readonly progressReporter: SessionProgressReporter;
  private readonly progressToolEnabled: boolean;
  private readonly expectedJsonSchema?: Record<string, unknown>;
  private readonly toolTransport: 'native' | 'xml' | 'xml-final';
  // XML transport encapsulates all XML-related state when toolTransport !== 'native'
  private readonly xmlTransport?: XmlToolTransport;
  private turnFailureReasons: string[] = [];
  // Per-turn planned subturns (tool call count) discovered when LLM yields toolCalls
  private plannedSubturns: Map<number, number> = new Map<number, number>();
  private resolvedFormat?: OutputFormatId;
  private resolvedFormatPromptValue?: string;
  private resolvedFormatParameterDescription?: string;
  private resolvedUserPrompt?: string;
  private resolvedSystemPrompt?: string;
  private masterLlmStartLogged = false;
  private sessionTitle?: { title: string; emoji?: string; ts: number };
  // Finalization state managed by FinalReportManager
  private readonly finalReportManager: FinalReportManager;
  private pendingToolSelection?: ToolSelection;
  private pendingRetryMessages: string[] = [];
  private trimmedToolCallIds = new Set<string>();

  // Counters for summary
  private llmAttempts = 0;
  private llmSyntheticFailures = 0;
  private centralSizeCapHits = 0;

  private readonly sessionExecutor: SessionToolExecutor;

  // Delegation accessors for FinalReportManager
  private get finalReport(): NonNullable<AIAgentResult['finalReport']> | undefined { return this.finalReportManager.getReport(); }
  private set finalReport(value: NonNullable<AIAgentResult['finalReport']> | undefined) {
    if (value === undefined) {
      this.finalReportManager.clear();
      return;
    }
    const payload: PendingFinalReportPayload = {
      status: value.status,
      format: value.format,
      content: value.content,
      content_json: value.content_json,
      metadata: value.metadata,
    };
    this.finalReportManager.commit(payload, 'synthetic');
  }
  private get finalReportSource() { return this.finalReportManager.getSource(); }
  private get pendingFinalReport() { return this.finalReportManager.getPending(); }
  private get finalReportAttempts() { return this.finalReportManager.finalReportAttempts; }
  private currentLlmOpId?: string;
  private systemTurnBegan = false;

  // Per-turn buffer of tool failures to synthesize messages when provider omits tool_error
  private pendingToolErrors: { id?: string; name: string; message: string; parameters?: Record<string, unknown> }[] = [];

  private encodePayloadForSnapshot(payload?: LogPayload): { format: string; encoding: 'base64'; value: string } | undefined {
    if (payload === undefined) return undefined;
    try {
      const encoded = Buffer.from(payload.body, 'utf8').toString('base64');
      return { format: payload.format, encoding: 'base64', value: encoded };
    } catch {
      const fallback = Buffer.from('[unavailable]', 'utf8').toString('base64');
      return { format: payload.format, encoding: 'base64', value: fallback };
    }
  }


  private resolveModelOverrides(
    provider: string,
    model: string
  ): { temperature?: number | null; topP?: number | null; topK?: number | null } {
    const providers = this.sessionConfig.config.providers;
    const providerConfig = Object.prototype.hasOwnProperty.call(providers, provider)
      ? providers[provider]
      : undefined;
    if (providerConfig === undefined) return {};
    const modelConfig = providerConfig.models?.[model];
    const overrides = modelConfig?.overrides;
    if (overrides === undefined) return {};
    const result: { temperature?: number | null; topP?: number | null; topK?: number | null } = {};

    const overrideTemperature = overrides.temperature;
    if (overrideTemperature !== undefined) {
      result.temperature = overrideTemperature ?? null;
    }

    const overrideTopPCamel = overrides.topP;
    if (overrideTopPCamel !== undefined) {
      result.topP = overrideTopPCamel ?? null;
    } else {
      const overrideTopPSnake = overrides.top_p;
      if (overrideTopPSnake !== undefined) {
        result.topP = overrideTopPSnake ?? null;
      }
    }

    const overrideTopKCamel = overrides.topK;
    if (overrideTopKCamel !== undefined) {
      result.topK = overrideTopKCamel ?? null;
    } else {
      const overrideTopKSnake = overrides.top_k;
      if (overrideTopKSnake !== undefined) {
        result.topK = overrideTopKSnake ?? null;
      }
    }

    return result;
  }

  private getCallPathLabel(): string {
    if (typeof this.callPath === 'string' && this.callPath.length > 0) return this.callPath;
    return this.agentPath;
  }

  private getAgentIdLabel(): string {
    if (typeof this.sessionConfig.agentId === 'string' && this.sessionConfig.agentId.length > 0) {
      return this.sessionConfig.agentId;
    }
    return this.getCallPathLabel();
  }

  private getAgentDisplayName(): string {
    const label = this.getCallPathLabel();
    const parts = label.split(':').map((part) => part.trim()).filter((part) => part.length > 0);
    if (parts.length > 0) return parts[parts.length - 1];
    return label;
  }

  private getAgentPathLabel(): string {
    if (typeof this.agentPath === 'string' && this.agentPath.length > 0) return this.agentPath;
    return this.getCallPathLabel();
  }

  private composeTurnPath(turn: number, subturn: number): string {
    const segment = `${String(turn)}.${String(subturn)}`;
    return this.turnPathPrefix.length > 0
      ? `${this.turnPathPrefix}-${segment}`
      : segment;
  }

  private extractToolNameForCallPath(entry: LogEntry): string | undefined {
    if (entry.type !== 'tool') return undefined;
    const remote = entry.remoteIdentifier;
    if (typeof remote === 'string' && remote.length > 0) {
      const parts = remote.split(':');
      if (parts.length > 0) {
        const candidate = parts[parts.length - 1];
        if (candidate.length > 0) return candidate;
      }
    }
    const detailsTool = entry.details?.tool;
    if (typeof detailsTool === 'string' && detailsTool.length > 0) {
      const tokens = detailsTool.split('__');
      const tail = tokens[tokens.length - 1];
      if (tail.length > 0) return tail;
      return detailsTool;
    }
    return undefined;
  }

  private buildMetricsFromSession(session: SessionNode | undefined): ProgressMetrics | undefined {
    if (session === undefined) return undefined;
    const metrics: ProgressMetrics = {};
    if (typeof session.startedAt === 'number') {
      const endTs = typeof session.endedAt === 'number' ? session.endedAt : Date.now();
      metrics.durationMs = Math.max(0, endTs - session.startedAt);
    }
    const totals = session.totals;
    if (totals !== undefined) {
      if (typeof totals.tokensIn === 'number') metrics.tokensIn = totals.tokensIn;
      if (typeof totals.tokensOut === 'number') metrics.tokensOut = totals.tokensOut;
      if (typeof totals.tokensCacheRead === 'number') metrics.tokensCacheRead = totals.tokensCacheRead;
      if (typeof totals.tokensCacheWrite === 'number') metrics.tokensCacheWrite = totals.tokensCacheWrite;
      if (typeof totals.toolsRun === 'number') metrics.toolsRun = totals.toolsRun;
      if (typeof totals.costUsd === 'number') metrics.costUsd = totals.costUsd;
      if (typeof totals.agentsRun === 'number') metrics.agentsRun = totals.agentsRun;
    }
    const hasMetrics = Object.keys(metrics).length > 0;
    return hasMetrics ? metrics : undefined;
  }

  private emitAgentCompletion(success: boolean, error?: string): void {
    const sessionSnapshot = (() => {
      try { return this.opTree.getSession(); } catch { return undefined; }
    })();
    const metrics = this.buildMetricsFromSession(sessionSnapshot);
    const payload = {
      callPath: this.getCallPathLabel(),
      agentId: this.getAgentIdLabel(),
      agentPath: this.getAgentPathLabel(),
      agentName: this.getAgentDisplayName(),
      txnId: this.txnId,
      parentTxnId: this.parentTxnId,
      originTxnId: this.originTxnId,
      metrics,
      error,
    };
    if (success) {
      this.progressReporter.agentFinished(payload);
    } else {
      this.progressReporter.agentFailed(payload);
    }
  }

  // Find the last LLM op for a given turn to anchor agent-emitted logs (debug/FIN)
  private getLastLlmOpIdForTurn(turnIndex: number): string | undefined {
    try {
      const s = this.opTree.getSession();
      const turns = Array.isArray((s as { turns?: unknown }).turns) ? (s.turns as unknown as { index: number; ops?: unknown[] }[]) : [];
      const t = turns.find((x) => x.index === turnIndex);
      if (t === undefined) return undefined;
      const ops = Array.isArray(t.ops) ? t.ops : [];
      // eslint-disable-next-line functional/no-loop-statements
      for (let i = ops.length - 1; i >= 0; i--) {
        const o = ops[i] as { opId?: string; kind?: string } | undefined;
        if (o?.kind === 'llm' && typeof o.opId === 'string' && o.opId.length > 0) return o.opId;
      }
    } catch (e) {
      warn(`getLastLlmOpIdForTurn failed: ${e instanceof Error ? e.message : String(e)}`);
    }
    return undefined;
  }
  
  private async persistSessionSnapshot(reason?: string): Promise<void> {
    const sink = this.sessionConfig.callbacks?.onSessionSnapshot;
    if (sink === undefined) {
      return;
    }
    try {
      const payload: SessionSnapshotPayload = {
        reason,
        sessionId: this.txnId,
        originId: this.originTxnId ?? this.txnId,
        timestamp: Date.now(),
        snapshot: { version: 1, opTree: this.opTree.getSession() },
      };
      await sink(payload);
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e);
      warn(`persistSessionSnapshot(${reason ?? 'unspecified'}) failed: ${message}`);
    }
  }

  private async flushAccounting(entries: AccountingEntry[]): Promise<void> {
    if (entries.length === 0) {
      return;
    }
    const sink = this.sessionConfig.callbacks?.onAccountingFlush;
    if (sink === undefined) {
      return;
    }
    try {
      const payload: AccountingFlushPayload = {
        sessionId: this.txnId,
        originId: this.originTxnId ?? this.txnId,
        timestamp: Date.now(),
        entries: entries.map((entry) => ({ ...entry })),
      };
      await sink(payload);
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e);
      warn(`flushAccounting failed: ${message}`);
    }
  }

  private constructor(
    config: Configuration,
    conversation: ConversationMessage[],
    logs: LogEntry[],
    accounting: AccountingEntry[],
    success: boolean,
    error: string | undefined,
    currentTurn: number,
    llmClient: LLMClient,
    sessionConfig: AIAgentSessionConfig
  ) {
    this.config = config;
    this.conversation = [...conversation]; // Defensive copy
    this.logs = [...logs]; // Defensive copy
    this.accounting = [...accounting]; // Defensive copy
    this.success = success;
    this.error = error;
    this._currentTurn = currentTurn;
    this.llmClient = llmClient;
    this.sessionConfig = sessionConfig;
    // Initialize FinalReportManager (format resolved later in run())
    this.finalReportManager = new FinalReportManager({
      finalReportToolName: AIAgentSession.FINAL_REPORT_TOOL,
    });
    this.abortSignal = sessionConfig.abortSignal;
    this.stopRef = sessionConfig.stopRef;
    this.headendId = sessionConfig.headendId;
    const telemetryLabels = { ...(sessionConfig.telemetryLabels ?? {}) };
    if (this.headendId !== undefined && !Object.prototype.hasOwnProperty.call(telemetryLabels, 'headend')) {
      telemetryLabels.headend = this.headendId;
    }
    this.telemetryLabels = telemetryLabels;
    // Build context guard with target configurations
    const sessionTargets = Array.isArray(this.sessionConfig.targets) ? this.sessionConfig.targets : [];
    const defaultBuffer = this.config.defaults?.contextWindowBufferTokens ?? ContextGuard.DEFAULT_CONTEXT_BUFFER_TOKENS;
    const defaultContextWindow = ContextGuard.DEFAULT_CONTEXT_WINDOW_TOKENS;
    const targetContextConfigs: TargetContextConfig[] = sessionTargets.map((target) => {
      const providerConfig = this.config.providers[target.provider];
      const modelConfig = providerConfig.models?.[target.model];
      // Session override (from CLI --override contextWindow=X) takes highest priority
      const contextWindow = this.sessionConfig.contextWindow
        ?? modelConfig?.contextWindow
        ?? providerConfig.contextWindow
        ?? defaultContextWindow;
      const tokenizerId = modelConfig?.tokenizer ?? providerConfig.tokenizer;
      const bufferTokens = modelConfig?.contextWindowBufferTokens
        ?? providerConfig.contextWindowBufferTokens
        ?? defaultBuffer;
      return {
        provider: target.provider,
        model: target.model,
        contextWindow,
        tokenizerId,
        bufferTokens,
      } satisfies TargetContextConfig;
    });
    this.contextGuard = new ContextGuard({
      targets: targetContextConfigs,
      defaultContextWindow,
      defaultBufferTokens: defaultBuffer,
      maxOutputTokens: sessionConfig.maxOutputTokens,
      callbacks: {
        onForcedFinalTurn: (blocked, trigger) => {
          // Handle side effects when tool preflight forces final turn
          this.handleContextGuardForcedFinalTurn(blocked, trigger);
        },
      },
    });
    try {
    if (this.abortSignal !== undefined) {
      if (this.abortSignal.aborted) {
        this.canceled = true;
        try { this.toolsOrchestrator?.cancel(); } catch (e) { warn(`toolsOrchestrator.cancel failed: ${e instanceof Error ? e.message : String(e)}`); }
      } else {
        this.abortSignal.addEventListener('abort', () => { this.canceled = true; try { this.toolsOrchestrator?.cancel(); } catch (e) { warn(`toolsOrchestrator.cancel failed: ${e instanceof Error ? e.message : String(e)}`); } }, { once: true });
      }
    }
    } catch (e) { warn(`abortSignal wiring failed: ${e instanceof Error ? e.message : String(e)}`); }
    // Initialize sub-agents registry if provided
    const childSubAgents = Array.isArray(sessionConfig.subAgents) ? sessionConfig.subAgents : [];
    this.subAgentCount = childSubAgents.length;
    if (this.subAgentCount > 0) {
      const reg = new SubAgentRegistry(
        childSubAgents,
        Array.isArray(sessionConfig.ancestors) ? sessionConfig.ancestors : []
      );
      this.subAgents = reg;
      this.subAgentToolNames = reg.listToolNames();
    } else {
    this.subAgentToolNames = [];
    }
    // REST tools handled by RestProvider; no local registry here
    // Tracing context
    this.txnId = sessionConfig.trace?.selfId ?? crypto.randomUUID();
    this.originTxnId = sessionConfig.trace?.originId ?? this.txnId;
    this.parentTxnId = sessionConfig.trace?.parentId;
    const initialAgentPath = sessionConfig.agentPath
      ?? sessionConfig.trace?.agentPath
      ?? (sessionConfig.agentId ?? 'agent');
    this.agentPath = normalizeCallPath(initialAgentPath);
    this.turnPathPrefix = sessionConfig.turnPathPrefix
      ?? sessionConfig.trace?.turnPath
      ?? '';
    this.callPath = normalizeCallPath(sessionConfig.trace?.callPath ?? this.agentPath);

    // Hierarchical operation tree (Option C)
    this.opTree = new SessionTreeBuilder({ traceId: this.txnId, agentId: sessionConfig.agentId, callPath: this.callPath, sessionTitle: sessionConfig.initialTitle ?? '' });

    this.progressReporter = new SessionProgressReporter((event) => {
      try {
        this.sessionConfig.callbacks?.onProgress?.(event);
      } catch (e) {
        warn(`onProgress callback failed: ${e instanceof Error ? e.message : String(e)}`);
      }
    });
    const toolBudgetCallbacks = this.contextGuard.createToolBudgetCallbacks();
    // Begin system preflight turn (turn 0) and log init
    try {
      if (!this.systemTurnBegan) {
        this.opTree.beginTurn(0, { system: true, label: 'init' });
        this.systemTurnBegan = true;
      }
      const sysInitOp = this.opTree.beginOp(0, 'system', { label: 'init' });
      this.log({ timestamp: Date.now(), severity: 'VRB', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'agent:init', fatal: false, message: 'session initialized' }, { opId: sysInitOp });
      this.opTree.endOp(sysInitOp, 'ok');
      this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
    } catch (e) { warn(`system init logging failed: ${e instanceof Error ? e.message : String(e)}`); }

    // Tools orchestrator (MCP + REST + Internal + Subagents)
    const orch = new ToolsOrchestrator({
      toolTimeout: sessionConfig.toolTimeout,
      toolResponseMaxBytes: sessionConfig.toolResponseMaxBytes,
      traceTools: sessionConfig.traceMCP === true,
    },
    this.opTree,
    (tree: SessionNode) => { try { this.sessionConfig.callbacks?.onOpTree?.(tree); } catch (e) { warn(`onOpTree callback failed: ${e instanceof Error ? e.message : String(e)}`); } },
    (entry, opts) => { this.log(entry, opts); },
    sessionConfig.callbacks?.onAccounting,
    {
      agentId: sessionConfig.agentId,
      agentPath: this.agentPath,
      callPath: this.getCallPathLabel(),
      txnId: this.txnId,
      headendId: this.headendId,
      telemetryLabels: this.telemetryLabels,
    },
    this.progressReporter,
    toolBudgetCallbacks);
    const providerRequestTimeout = (() => {
      const raw = sessionConfig.toolTimeout;
      if (typeof raw !== 'number' || !Number.isFinite(raw) || raw <= 0) return undefined;
      const scaled = Math.trunc(raw * 1.5);
      const buffered = raw + 1000;
      return Math.max(scaled, buffered);
    })();
    orch.register(new MCPProvider('mcp', sessionConfig.config.mcpServers, {
      trace: sessionConfig.traceMCP,
      verbose: sessionConfig.verbose,
      requestTimeoutMs: providerRequestTimeout,
      onLog: (e) => { this.log(e); },
      initConcurrency: sessionConfig.mcpInitConcurrency
    }));
    // Build selected REST tools by frontmatter selection
    if (this.sessionConfig.config.restTools !== undefined) {
      const selected: Record<string, RestToolConfig> = {};
      const available = this.sessionConfig.config.restTools;
      const list = Array.isArray(this.sessionConfig.tools) ? this.sessionConfig.tools : [];
      list.forEach((name) => { if (Object.prototype.hasOwnProperty.call(available, name)) selected[name] = available[name]; });
      if (Object.keys(selected).length > 0) orch.register(new RestProvider('rest', selected));
    }
    // Internal tools provider
    {
      const resolvedMaxToolCallsPerTurn = Math.max(1, this.sessionConfig.maxToolCallsPerTurn ?? 10);
      const declaredTools = Array.isArray(this.sessionConfig.tools) ? this.sessionConfig.tools : [];
      const hasNonInternalDeclaredTools = declaredTools.some((toolName) => {
        if (typeof toolName !== 'string') return false;
        const normalized = toolName.trim();
        if (normalized.length === 0) return false;
        if (normalized === 'batch') return false;
        if (normalized === 'agent__progress_report') return false;
        if (normalized === AIAgentSession.FINAL_REPORT_TOOL) return false;
        return true;
      });
      const hasSubAgentsConfigured = Array.isArray(this.sessionConfig.subAgents) && this.sessionConfig.subAgents.length > 0;
      const wantsProgressUpdates = this.sessionConfig.headendWantsProgressUpdates !== false;
    const enableProgressTool = wantsProgressUpdates && (hasNonInternalDeclaredTools || hasSubAgentsConfigured);
    this.progressToolEnabled = enableProgressTool;
    this.toolTransport = this.sessionConfig.toolingTransport ?? 'xml-final';
    // Initialize XML transport when not using native tool calling
    if (this.toolTransport !== 'native') {
      this.xmlTransport = new XmlToolTransport(this.toolTransport);
    }
      const enableBatch = declaredTools.includes('batch');
      const eo = this.sessionConfig.expectedOutput;
      const expectedJsonSchema = (eo?.format === 'json') ? eo.schema : undefined;
      this.expectedJsonSchema = expectedJsonSchema;
      const internalProvider = new InternalToolProvider('agent', {
        enableBatch,
        outputFormat: this.sessionConfig.outputFormat,
        expectedOutputFormat: eo?.format === 'json' ? 'json' : undefined,
        expectedJsonSchema,
        disableProgressTool: !enableProgressTool,
        maxToolCallsPerTurn: resolvedMaxToolCallsPerTurn,
        logError: (message: string) => {
          const entry: LogEntry = {
            timestamp: Date.now(),
            severity: 'ERR',
            turn: this.currentTurn,
            subturn: 0,
            direction: 'response',
            type: 'tool',
            remoteIdentifier: 'agent:batch',
            fatal: false,
            message,
          };
          this.log(entry);
        },
        updateStatus: (text: string) => {
          const t = text.trim();
          if (t.length > 0) {
            this.opTree.setLatestStatus(t);
            try {
              this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
            } catch (e) {
              warn(`onOpTree callback failed: ${e instanceof Error ? e.message : String(e)}`);
            }
            const ts = Date.now();
            const entry: LogEntry = {
              timestamp: ts,
              severity: 'VRB',
              turn: this.currentTurn,
              subturn: 0,
              direction: 'response',
              type: 'tool',
              remoteIdentifier: 'agent:progress',
              fatal: false,
              bold: true,
              message: `[PROGRESS UPDATE] ${t}`,
            };
            this.log(entry);
            this.progressReporter.agentUpdate({
              callPath: this.getCallPathLabel(),
              agentId: this.getAgentIdLabel(),
              agentPath: this.getAgentPathLabel(),
              agentName: this.getAgentDisplayName(),
              txnId: this.txnId,
              parentTxnId: this.parentTxnId,
              originTxnId: this.originTxnId,
              message: t,
            });
          }
        },
        setTitle: (title: string, emoji?: string) => {
          const clean = title.trim();
          if (clean.length === 0) return;
          this.sessionTitle = { title: clean, emoji, ts: Date.now() };
          this.opTree.setSessionTitle(clean);
          const entry: LogEntry = {
            timestamp: Date.now(),
            severity: 'VRB',
            turn: this.currentTurn,
            subturn: 0,
            direction: 'response',
            type: 'tool',
            remoteIdentifier: 'agent:title',
            fatal: false,
            bold: true,
            message: (typeof emoji === 'string' && emoji.length > 0) ? `${emoji} ${clean}` : clean,
          };
          this.log(entry);
        },
        setFinalReport: (p) => {
          const normalizedStatus: 'success' | 'failure' | 'partial' = p.status;
          this.commitFinalReport({
            status: normalizedStatus,
            format: p.format as 'json'|'markdown'|'markdown+mermaid'|'slack-block-kit'|'tty'|'pipe'|'sub-agent'|'text',
            content: p.content,
            content_json: p.content_json,
            metadata: p.metadata,
          }, 'tool-call');
        },
        orchestrator: orch,
        getCurrentTurn: () => this.currentTurn,
        toolTimeoutMs: sessionConfig.toolTimeout
      });
      const formatInfo = internalProvider.getFormatInfo();
      this.resolvedFormat = formatInfo.formatId;
      this.resolvedFormatPromptValue = formatInfo.promptValue;
      this.resolvedFormatParameterDescription = formatInfo.parameterDescription;
      this.finalReportManager.setResolvedFormat(
        formatInfo.formatId,
        formatInfo.promptValue,
        formatInfo.parameterDescription
      );
      orch.register(internalProvider);
    }
    if (this.subAgents !== undefined) {
      const subAgents = this.subAgents;
      const execFn = async (
        name: string,
        parameters: Record<string, unknown>,
        opts?: { onChildOpTree?: (tree: SessionNode) => void; parentOpPath?: string; parentContext?: { turn: number; subturn: number } }
      ) => {
        const normalizedChildName = name.startsWith('agent__') ? name.slice('agent__'.length) : name;
        const parentTurnPath = opts?.parentContext !== undefined
          ? this.composeTurnPath(opts.parentContext.turn, opts.parentContext.subturn)
          : this.turnPathPrefix;
        const childAgentPath = appendCallPathSegment(this.agentPath, normalizedChildName);
        const exec = await subAgents.execute(name, parameters, {
          config: this.sessionConfig.config,
          callbacks: this.sessionConfig.callbacks,
          targets: this.sessionConfig.targets,
          stream: this.sessionConfig.stream,
          traceLLM: this.sessionConfig.traceLLM,
          traceMCP: this.sessionConfig.traceMCP,
          traceSdk: this.sessionConfig.traceSdk,
          verbose: this.sessionConfig.verbose,
          temperature: this.sessionConfig.temperature,
          topP: this.sessionConfig.topP,
          llmTimeout: this.sessionConfig.llmTimeout,
          toolTimeout: this.sessionConfig.toolTimeout,
          maxRetries: this.sessionConfig.maxRetries,
          maxTurns: this.sessionConfig.maxTurns,
          toolResponseMaxBytes: this.sessionConfig.toolResponseMaxBytes,
          // propagate control signals so children can stop/abort
          abortSignal: this.abortSignal,
          stopRef: this.stopRef,
          trace: { originId: this.originTxnId, parentId: this.txnId, callPath: childAgentPath, agentPath: childAgentPath, turnPath: parentTurnPath },
          agentPath: childAgentPath,
          turnPathPrefix: parentTurnPath,
          onChildOpTree: opts?.onChildOpTree
        }, { ...opts, parentTurnPath });
        // Keep child conversation list (may be reported in results for compatibility)
        this.childConversations.push({ agentId: exec.child.toolName, toolName: exec.child.toolName, promptPath: exec.child.promptPath, conversation: exec.conversation, trace: exec.trace });
        await this.persistSessionSnapshot('subagent_finish');
        return { result: exec.result, childAccounting: exec.accounting, childOpTree: exec.opTree };
      };
      // Register AgentProvider synchronously so sub-agent tools are known before first turn
      orch.register(new AgentProvider('subagent', subAgents, execFn));
    }
    // Populate mapping now (before warmup) so hasTool() sees all registered providers
    void orch.listTools();
    this.toolsOrchestrator = orch;

    const sessionContext: SessionContext = {
      agentId: sessionConfig.agentId,
      callPath: this.callPath,
      txnId: this.txnId,
      parentTxnId: this.parentTxnId,
      originTxnId: this.originTxnId,
      toolTimeout: sessionConfig.toolTimeout,
      maxToolCallsPerTurn: sessionConfig.maxToolCallsPerTurn,
      toolResponseMaxBytes: sessionConfig.toolResponseMaxBytes,
      stopRef: this.stopRef,
      isCanceled: () => this.canceled,
      progressToolEnabled: this.progressToolEnabled,
      finalReportToolName: AIAgentSession.FINAL_REPORT_TOOL,
    };

    this.sessionExecutor = new SessionToolExecutor(
      this.toolsOrchestrator,
      this.contextGuard,
      this.finalReportManager,
      this.xmlTransport,
      (entry) => { this.log(entry); },
      (entry) => {
        this.accounting.push(entry);
        try { this.sessionConfig.callbacks?.onAccounting?.(entry); } catch (e) { warn(`onAccounting callback failed: ${e instanceof Error ? e.message : String(e)}`); }
      },
      (result, context) => this.applyToolResponseCap(result, this.sessionConfig.toolResponseMaxBytes, this.logs, context),
      sessionContext,
      this.subAgents !== undefined ? {
        hasTool: (name) => this.subAgents?.hasTool(name) ?? false,
        addInvoked: (name) => this.invokedSubAgents.add(name)
      } : undefined
    );

    // Apply an initial session title without consuming a tool turn (side-channel)
    try {
      const t = typeof sessionConfig.initialTitle === 'string' ? sessionConfig.initialTitle.trim() : '';
      if (t.length > 0) {
        this.sessionTitle = { title: t, ts: Date.now() };
        const entry: LogEntry = {
          timestamp: Date.now(),
          severity: 'VRB',
          turn: this.currentTurn,
          subturn: 0,
          direction: 'response',
          type: 'tool',
          remoteIdentifier: 'agent:title',
          fatal: false,
          bold: true,
          message: t,
        };
        this.log(entry);
      }
    } catch (e) { warn(`initialTitle handling failed: ${e instanceof Error ? e.message : String(e)}`); }
  }

  // Static factory method for creating new sessions
  static create(sessionConfig: AIAgentSessionConfig): AIAgentSession {
    // Validate configuration
    validateProviders(sessionConfig.config, sessionConfig.targets.map((t) => t.provider));
    validateMCPServers(sessionConfig.config, sessionConfig.tools);
    validatePrompts(sessionConfig.systemPrompt, sessionConfig.userPrompt);

    // Generate a unique span id (self) for this session and enrich trace context
    const sessionTxnId = crypto.randomUUID();
    const inferredAgentPath = sessionConfig.agentPath
      ?? sessionConfig.trace?.agentPath
      ?? sessionConfig.agentId
      ?? 'agent';
    const inferredTurnPathPrefix = sessionConfig.turnPathPrefix
      ?? sessionConfig.trace?.turnPath
      ?? '';
    const enrichedSessionConfig: AIAgentSessionConfig = {
      ...sessionConfig,
      agentPath: inferredAgentPath,
      turnPathPrefix: inferredTurnPathPrefix,
      trace: {
        selfId: sessionTxnId,
        originId: sessionConfig.trace?.originId ?? sessionTxnId,
        parentId: sessionConfig.trace?.parentId,
        callPath: sessionConfig.trace?.callPath ?? inferredAgentPath,
        agentPath: sessionConfig.trace?.agentPath ?? inferredAgentPath,
        turnPath: sessionConfig.trace?.turnPath ?? inferredTurnPathPrefix,
      },
    };

    // External log relay placeholder; bound after session instantiation to tree
    let externalLogRelay: (entry: LogEntry) => void = (e: LogEntry) => { void e; };
    // Wrap onLog to inject trace fields and route only through the execution tree (which fans out to callbacks)
    const wrapLog = (_fn?: (entry: LogEntry) => void) => (entry: LogEntry): void => {
      const enriched: LogEntry = {
        ...entry,
        agentId: enrichedSessionConfig.agentId,
        agentPath: enrichedSessionConfig.agentPath,
        callPath: enrichedSessionConfig.trace?.callPath ?? enrichedSessionConfig.agentPath,
        txnId: enrichedSessionConfig.trace?.selfId,
        parentTxnId: enrichedSessionConfig.trace?.parentId,
        originTxnId: enrichedSessionConfig.trace?.originId,
      };
      try { externalLogRelay(enriched); } catch (e) { warn(`external log relay failed: ${e instanceof Error ? e.message : String(e)}`); }
    };

    // Create session-owned LLM client
    const llmClient = new LLMClient(enrichedSessionConfig.config.providers, {
      traceLLM: enrichedSessionConfig.traceLLM,
      traceSDK: enrichedSessionConfig.traceSdk,
      onLog: wrapLog(enrichedSessionConfig.callbacks?.onLog),
      pricing: enrichedSessionConfig.config.pricing,
    });

    const sess = new AIAgentSession(
      enrichedSessionConfig.config,
      [], // empty conversation
      [], // empty logs  
      [], // empty accounting
      false, // not successful yet
      undefined, // no error yet
      0, // start at turn 0
      llmClient,
      enrichedSessionConfig
    );
    // Bind external log relay to append into opTree and relay to callbacks
    externalLogRelay = (e: LogEntry) => { try { sess.recordExternalLog(e); } catch (err) { warn(`recordExternalLog failed: ${err instanceof Error ? err.message : String(err)}`); } };
    return sess;
  }

  // Helper method to log exit with proper format
  private logExit(
    exitCode: ExitCode,
    reason: string,
    turn: number,
    options?: { fatal?: boolean; severity?: LogEntry['severity'] }
  ): void {
    const fatal = options?.fatal ?? true;
    const severity: LogEntry['severity'] = options?.severity ?? (fatal ? 'ERR' : 'VRB');
    const logEntry: LogEntry = {
      timestamp: Date.now(),
      severity,
      turn,
      subturn: 0,
      direction: 'response',
      type: 'llm',
      remoteIdentifier: `agent:${exitCode}`,
      fatal,
      message: `${exitCode}: ${reason} (fatal=${fatal ? 'true' : 'false'})`,
      agentId: this.sessionConfig.agentId,
      callPath: this.callPath,
      txnId: this.txnId,
      parentTxnId: this.parentTxnId,
      originTxnId: this.originTxnId
    };
    this.log(logEntry);
  }

  // Centralized helper to ensure all logs carry trace fields
  // Public unified logger used across the codebase
  private log(entry: LogEntry, opts?: { opId?: string }): void {
    this.addLog(this.logs, entry, opts);
  }

  private addLog(logs: LogEntry[], entry: LogEntry, opts?: { opId?: string }): void {
    const turnNum = typeof entry.turn === 'number' ? entry.turn : 0;
    const planned = this.plannedSubturns.get(turnNum);
    const enriched: LogEntry = {
      agentId: this.sessionConfig.agentId,
      agentPath: this.agentPath,
      callPath: this.agentPath,
      txnId: this.txnId,
      parentTxnId: this.parentTxnId,
      originTxnId: this.originTxnId,
      ...(this.sessionConfig.maxTurns !== undefined ? { 'max_turns': this.sessionConfig.maxTurns } : {}),
      ...(planned !== undefined ? { 'max_subturns': planned } : {}),
      ...entry,
    };
    const turnPath = this.composeTurnPath(enriched.turn, enriched.subturn);
    enriched.turnPath = turnPath;

    if (enriched.type === 'tool') {
      const toolName = this.extractToolNameForCallPath(enriched);
      if (toolName !== undefined && toolName.length > 0) {
        enriched.callPath = appendCallPathSegment(this.agentPath, toolName);
      } else {
        enriched.callPath = normalizeCallPath(this.agentPath);
      }
    } else {
      enriched.callPath = normalizeCallPath(this.agentPath);
    }
    const label = this.getCallPathLabel();
    const rawMessage = typeof enriched.message === 'string' ? enriched.message.trim() : '';
    if (rawMessage.length === 0) {
      enriched.message = `${label}: (message missing)`;
    } else {
      enriched.message = rawMessage;
    }
    if (enriched.headendId === undefined && this.headendId !== undefined) {
      enriched.headendId = this.headendId;
    }
    if ((enriched.severity === 'WRN' || enriched.severity === 'ERR') && enriched.stack === undefined) {
      enriched.stack = this.captureStackTrace(2);
    }
    logs.push(enriched);
    let appendedOpId: string | undefined;
    // Anchor log to opTree when an opId is known, else best-effort for LLM logs
    try {
      const explicitOp = opts?.opId;
      if (typeof explicitOp === 'string' && explicitOp.length > 0) {
        this.opTree.appendLog(explicitOp, enriched);
        appendedOpId = explicitOp;
        this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
      } else if (enriched.type === 'llm') {
        const active = this.currentLlmOpId;
        const targetOp = (typeof active === 'string' && active.length > 0)
          ? active
          : this.getLastLlmOpIdForTurn(typeof enriched.turn === 'number' ? enriched.turn : this.currentTurn);
        if (typeof targetOp === 'string' && targetOp.length > 0) {
          this.opTree.appendLog(targetOp, enriched);
          appendedOpId = targetOp;
          this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
        }
      }
    } catch (e) { warn(`addLog opTree anchor failed: ${e instanceof Error ? e.message : String(e)}`); }
    if (appendedOpId !== undefined) {
      try {
        const updated = this.applyLogPayloadToOp(appendedOpId, enriched);
        if (updated) {
          this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
        }
      } catch (e) { warn(`opTree payload update failed: ${e instanceof Error ? e.message : String(e)}`); }
    }
    // Single place try/catch for external sinks
    try { this.sessionConfig.callbacks?.onLog?.(enriched); } catch (e) { warn(`onLog callback failed: ${e instanceof Error ? e.message : String(e)}`); }
  }

  private captureStackTrace(skip = 0): string | undefined {
    const err = new Error();
    if (typeof err.stack !== 'string') return undefined;
    const lines = err.stack.split('\n');
    const start = Math.min(lines.length, 1 + skip);
    const pruned = lines.slice(start).join('\n');
    return pruned;
  }

  // Relay for logs originating outside AIAgentSession (LLM/MCP internals)
  recordExternalLog(entry: LogEntry): void {
    // Delegate to unified logger with the active LLM opId when present
    let opId: string | undefined;
    if (entry.type === 'llm') {
      const cid = this.currentLlmOpId;
      if (typeof cid === 'string' && cid.length > 0) opId = cid;
    }
    this.log(entry, (typeof opId === 'string' && opId.length > 0) ? { opId } : undefined);
  }

  // Optional: expose a snapshot for progress UIs or web monitoring
  getExecutionSnapshot(): { logs: number; accounting: number } { return { logs: this.logs.length, accounting: this.accounting.length }; }

  // Enforce a hard timeout around a promise (centralized tool timeout)
  private async withTimeout<T>(promise: Promise<T>, timeoutMs?: number): Promise<T> {
    if (typeof timeoutMs !== 'number' || timeoutMs <= 0) return await promise;
    let timer: ReturnType<typeof setTimeout> | undefined;
    try {
      return await Promise.race([
        promise,
        new Promise<T>((_resolve, reject) => {
          timer = setTimeout(() => { reject(new Error('Tool execution timed out')); }, timeoutMs);
        })
      ]);
    } finally {
      try { if (timer !== undefined) clearTimeout(timer); } catch { /* noop */ }
    }
  }

  // Apply centralized response size cap (in bytes) to any tool result
  private applyToolResponseCap(
    result: string,
    limitBytes: number | undefined,
    logs: LogEntry[],
    context?: { server?: string; tool?: string; turn?: number; subturn?: number }
  ): string {
    if (typeof limitBytes !== 'number' || limitBytes <= 0) return result;
    const sizeBytes = Buffer.byteLength(result, 'utf8');
    if (sizeBytes <= limitBytes) return result;
    try {
      const srv = context !== undefined ? context.server : undefined;
      const tl = context !== undefined ? context.tool : undefined;
      const rid = (srv !== undefined && tl !== undefined)
        ? `${srv}:${tl}`
        : 'agent:tool';
      const warn: LogEntry = {
        timestamp: Date.now(),
        severity: 'WRN',
        turn: context?.turn ?? this.currentTurn,
        subturn: context?.subturn ?? 0,
        direction: 'response',
        type: 'tool',
        remoteIdentifier: rid,
        fatal: false,
        message: `response exceeded max size: ${String(sizeBytes)} bytes > limit ${String(limitBytes)} bytes (truncated)`
      };
      this.log(warn);
    } catch { /* ignore logging errors */ }
    this.centralSizeCapHits += 1;
    return truncateUtf8WithNotice(result, limitBytes, sizeBytes);
  }

  private applyLogPayloadToOp(opId: string, log: LogEntry): boolean {
    let updated = false;
    const kind: 'llm' | 'tool' = log.type === 'tool' ? 'tool' : 'llm';
    const requestPayload = log.llmRequestPayload ?? (kind === 'tool' ? log.toolRequestPayload : undefined);
    const responsePayload = log.llmResponsePayload ?? (kind === 'tool' ? log.toolResponsePayload : undefined);
    const encodedRequest = this.encodePayloadForSnapshot(requestPayload);
    if (encodedRequest !== undefined) {
      this.opTree.setRequest(opId, { kind, payload: { raw: encodedRequest } });
      updated = true;
    }
    const encodedResponse = this.encodePayloadForSnapshot(responsePayload);
    if (encodedResponse !== undefined) {
      this.opTree.setResponse(opId, { payload: { raw: encodedResponse } });
      updated = true;
    }
    return updated;
  }

  // Main execution method - returns immutable result
  async run(): Promise<AIAgentResult> {
    const sessionSpanAttributes: Record<string, string> = {};
    if (typeof this.sessionConfig.agentId === 'string' && this.sessionConfig.agentId.length > 0) {
      sessionSpanAttributes['ai.agent.id'] = this.sessionConfig.agentId;
    }
    if (typeof this.callPath === 'string' && this.callPath.length > 0) {
      sessionSpanAttributes['ai.agent.call_path'] = this.callPath;
    } else if (typeof this.sessionConfig.agentId === 'string' && this.sessionConfig.agentId.length > 0) {
      sessionSpanAttributes['ai.agent.call_path'] = this.sessionConfig.agentId;
    }
    if (typeof this.txnId === 'string' && this.txnId.length > 0) {
      sessionSpanAttributes['ai.session.txn_id'] = this.txnId;
    }
    if (typeof this.parentTxnId === 'string' && this.parentTxnId.length > 0) {
      sessionSpanAttributes['ai.session.parent_txn_id'] = this.parentTxnId;
    }
    if (typeof this.originTxnId === 'string' && this.originTxnId.length > 0) {
      sessionSpanAttributes['ai.session.origin_txn_id'] = this.originTxnId;
    }
    if (typeof this.headendId === 'string' && this.headendId.length > 0) {
      sessionSpanAttributes['ai.session.headend_id'] = this.headendId;
    }

    return await runWithSpan('agent.session', { attributes: sessionSpanAttributes }, async (span) => {
      let currentConversation = [...this.conversation];
      let currentLogs = [...this.logs];
      let currentAccounting = [...this.accounting];
      let currentTurn = this.currentTurn;

      this.progressReporter.agentStarted({
        callPath: this.getCallPathLabel(),
        agentId: this.getAgentIdLabel(),
        agentPath: this.getAgentPathLabel(),
        agentName: this.getAgentDisplayName(),
        txnId: this.txnId,
        parentTxnId: this.parentTxnId,
        originTxnId: this.originTxnId,
        reason: this.sessionTitle?.title ?? this.sessionConfig.initialTitle,
      });

      try {
      // Start execution session in the tree
      // opTree is canonical; no separate session
      // Warmup providers (ensures MCP tools/instructions are available) and refresh mapping
      try { await this.toolsOrchestrator?.warmup(); } catch (e) { warn(`tools warmup failed: ${e instanceof Error ? e.message : String(e)}`); }

      // Verbose: emit settings summary at start (without prompts)
      if (this.sessionConfig.verbose === true) {
        const summarizeConfig = () => {
          const prov = Object.entries(this.config.providers).map(([name, p]) => ({
            name,
            type: p.type,
            baseUrl: p.baseUrl,
            hasApiKey: typeof p.apiKey === 'string' && p.apiKey.length > 0,
            headerKeys: p.headers !== undefined ? Object.keys(p.headers) : [],
          }));
          const srvs = Object.entries(this.config.mcpServers).map(([name, s]) => ({
            name,
            type: s.type,
            url: s.url,
            command: s.command,
            argsCount: Array.isArray(s.args) ? s.args.length : 0,
            headerKeys: s.headers !== undefined ? Object.keys(s.headers) : [],
            envKeys: s.env !== undefined ? Object.keys(s.env) : [],
            enabled: s.enabled !== false,
          }));
          return { providers: prov, mcpServers: srvs, billingFile: this.config.persistence?.billingFile ?? this.config.accounting?.file };
        };
        const summarizeSession = () => ({
          targets: this.contextGuard.getTargets().map((t) => ({
            provider: t.provider,
            model: t.model,
            contextWindow: t.contextWindow,
            bufferTokens: t.bufferTokens,
          })),
          tools: this.sessionConfig.tools,
          expectedOutput: this.sessionConfig.expectedOutput?.format,
          temperature: this.sessionConfig.temperature,
          topP: this.sessionConfig.topP,
          llmTimeout: this.sessionConfig.llmTimeout,
          toolTimeout: this.sessionConfig.toolTimeout,
          maxRetries: this.sessionConfig.maxRetries,
          maxTurns: this.sessionConfig.maxTurns,
          stream: this.sessionConfig.stream,
          traceLLM: this.sessionConfig.traceLLM,
          traceMCP: this.sessionConfig.traceMCP,
          traceSdk: this.sessionConfig.traceSdk,
          verbose: this.sessionConfig.verbose,
          toolResponseMaxBytes: this.sessionConfig.toolResponseMaxBytes,
          mcpInitConcurrency: this.sessionConfig.mcpInitConcurrency,
          conversationHistoryLength: Array.isArray(this.sessionConfig.conversationHistory) ? this.sessionConfig.conversationHistory.length : 0,
        });
        const settings = { config: summarizeConfig(), session: summarizeSession() };
        const entry: LogEntry = {
          timestamp: Date.now(),
          severity: 'VRB',
          turn: currentTurn,
          subturn: 0,
          direction: 'request',
          type: 'llm',
          remoteIdentifier: 'agent:settings',
          fatal: false,
          message: JSON.stringify(settings),
        };
        this.log(entry);

      }
      // MCP servers initialize lazily via provider; no explicit init here

      // Startup banner: list resolved MCP tools, REST tools, and sub-agent tools (exposed names)
      try {
        const allTools = this.toolsOrchestrator?.listTools() ?? [];
        const names = allTools.map((t) => t.name);
        const restToolNames = names.filter((n) => n.startsWith('rest__'));
        const subAgentTools = names.filter((n) => n.startsWith('agent__'));
        const mcpToolNames = names.filter((n) => !n.startsWith('rest__') && !n.startsWith('agent__'));
        const banner: LogEntry = {
          timestamp: Date.now(),
          severity: 'VRB',
          turn: 0,
          subturn: 0,
          direction: 'response',
          type: 'llm',
          remoteIdentifier: AIAgentSession.REMOTE_AGENT_TOOLS,
          fatal: false,
          message: `tools: mcp=${String(mcpToolNames.length)} [${mcpToolNames.join(', ')}]; rest=${String(restToolNames.length)} [${restToolNames.join(', ')}]; subagents=${String(subAgentTools.length)} [${subAgentTools.join(', ')}]`
        };
        this.log(banner);
      } catch (e) { warn(`tools banner emit failed: ${e instanceof Error ? e.message : String(e)}`); }

      // One-time pricing coverage warning for master + loaded sub-agents
      try {
        const pricing = this.sessionConfig.config.pricing ?? {};
        const pairs: { provider: string; model: string }[] = [];
        // master targets
        pairs.push(...this.sessionConfig.targets);
        // sub-agent targets (declared), fallback to master targets when none declared
        const subPaths = this.subAgents?.getPromptPaths() ?? [];
        subPaths.forEach((p) => {
          try {
            const raw = fs.readFileSync(p, 'utf-8');
            const fm = parseFrontmatter(raw, { baseDir: path.dirname(p) });
            const models = parsePairs((fm?.options as { models?: unknown } | undefined)?.models);
            if (models.length > 0) pairs.push(...models); else pairs.push(...this.sessionConfig.targets);
          } catch (e) { warn(`sub-agent options parse failed: ${e instanceof Error ? e.message : String(e)}`); }
        });
        // Deduplicate and find missing pricing entries
        const uniq = new Set(pairs.map((x) => `${x.provider}/${x.model}`));
        const missing: string[] = [];
        uniq.forEach((kv) => {
          const [prov, model] = kv.split('/', 2);
          const provMap = (pricing as Partial<Record<string, Record<string, unknown>>>)[prov];
          const has = provMap !== undefined && Object.prototype.hasOwnProperty.call(provMap, model);
          if (!has) missing.push(kv);
        });
        if (missing.length > 0) {
          const warn: LogEntry = {
            timestamp: Date.now(),
            severity: 'WRN',
            turn: 0,
            subturn: 0,
            direction: 'response',
            type: 'llm',
            remoteIdentifier: 'agent:pricing',
            fatal: false,
            message: `Missing pricing entries for ${String(missing.length)} model(s): ${missing.join(', ')}`
          };
          this.log(warn);
        }
      } catch (e) { warn(`pricing coverage check failed: ${e instanceof Error ? e.message : String(e)}`); }

      // Apply ${FORMAT} replacement first, then expand variables
      // Safety: strip any shebang/frontmatter from system prompt to avoid leaking YAML to the LLM
      const sysBody = extractBodyWithoutFrontmatter(this.sessionConfig.systemPrompt);
      // Inject plain format description only where ${FORMAT} or {{FORMAT}} exists; do not prepend extra text
      const fmtDesc = this.resolvedFormatPromptValue ?? '';
      const withFormat = applyFormat(sysBody, fmtDesc);
      const vars = { ...buildPromptVars(), MAX_TURNS: String(this.sessionConfig.maxTurns ?? 10) };
      const systemExpanded = expandVars(withFormat, vars);
      const userExpanded = expandVars(this.sessionConfig.userPrompt, vars);
      this.resolvedUserPrompt = userExpanded;

      // Startup VRB for sub-agents only (avoid duplicate master logs)
      try {
        if (this.parentTxnId !== undefined) {
          const ctx: string = (typeof this.callPath === 'string' && this.callPath.length > 0)
            ? this.callPath
            : (this.sessionConfig.agentId ?? 'agent');
          const entry: LogEntry = {
            timestamp: Date.now(),
            severity: 'VRB',
            turn: 0,
            subturn: 0,
            direction: 'request',
            type: 'llm',
            remoteIdentifier: 'agent:start',
            fatal: false,
            bold: true,
            message: `${ctx}: ${userExpanded}`,
          };
          this.log(entry);
        }
          } catch (e) { warn(`startup verbose log failed: ${e instanceof Error ? e.message : String(e)}`); }

      // Build enhanced system prompt with tool instructions
      const toolInstructions = this.toolTransport === 'native' ? (this.toolsOrchestrator?.getCombinedInstructions() ?? '') : '';
      const enhancedSystemPrompt = this.enhanceSystemPrompt(systemExpanded, toolInstructions);
      this.resolvedSystemPrompt = enhancedSystemPrompt;

      // Initialize conversation
      if (this.sessionConfig.conversationHistory !== undefined && this.sessionConfig.conversationHistory.length > 0) {
        const history = [...this.sessionConfig.conversationHistory];
        if (history[0].role === 'system') {
          history[0] = { role: 'system', content: enhancedSystemPrompt };
        } else {
          history.unshift({ role: 'system', content: enhancedSystemPrompt });
        }
        currentConversation.push(...history);
      } else {
        currentConversation.push({ role: 'system', content: enhancedSystemPrompt });
      }
      currentConversation.push({ role: 'user', content: userExpanded });

      // Main agent loop with retry logic - delegated to TurnRunner
      const turnRunnerContext: TurnRunnerContext = {
        sessionConfig: this.sessionConfig,
        config: this.config,
        agentId: this.sessionConfig.agentId,
        callPath: this.callPath,
        agentPath: this.agentPath,
        txnId: this.txnId,
        parentTxnId: this.parentTxnId,
        originTxnId: this.originTxnId,
        headendId: this.headendId,
        telemetryLabels: this.telemetryLabels,
        llmClient: this.llmClient,
        toolsOrchestrator: this.toolsOrchestrator,
        contextGuard: this.contextGuard,
        finalReportManager: this.finalReportManager,
        opTree: this.opTree,
        progressReporter: this.progressReporter,
        sessionExecutor: this.sessionExecutor,
        xmlTransport: this.xmlTransport,
        subAgents: this.subAgents,
        resolvedFormat: this.resolvedFormat,
        resolvedFormatPromptValue: this.resolvedFormatPromptValue,
        resolvedFormatParameterDescription: this.resolvedFormatParameterDescription,
        resolvedUserPrompt: this.resolvedUserPrompt,
        resolvedSystemPrompt: this.resolvedSystemPrompt,
        expectedJsonSchema: this.expectedJsonSchema,
        progressToolEnabled: this.progressToolEnabled,
        toolTransport: this.toolTransport,
        abortSignal: this.sessionConfig.abortSignal,
        stopRef: this.sessionConfig.stopRef,
        isCanceled: () => this.canceled,
      };

      const turnRunnerCallbacks: TurnRunnerCallbacks = {
        log: (entry, opts) => { this.log(entry, opts); },
        onAccounting: (entry) => { try { this.sessionConfig.callbacks?.onAccounting?.(entry); } catch (e) { warn(`onAccounting callback failed: ${e instanceof Error ? e.message : String(e)}`); } },
        onOpTree: (tree) => { try { this.sessionConfig.callbacks?.onOpTree?.(tree); } catch (e) { warn(`onOpTree callback failed: ${e instanceof Error ? e.message : String(e)}`); } },
        onTurnStarted: (turn) => { try { this.sessionConfig.callbacks?.onTurnStarted?.(turn); } catch (e) { warn(`onTurnStarted callback failed: ${e instanceof Error ? e.message : String(e)}`); } },
        onOutput: (chunk) => { try { this.sessionConfig.callbacks?.onOutput?.(chunk); } catch (e) { warn(`onOutput callback failed: ${e instanceof Error ? e.message : String(e)}`); } },
        onThinking: (chunk) => { try { this.sessionConfig.callbacks?.onThinking?.(chunk); } catch (e) { warn(`onThinking callback failed: ${e instanceof Error ? e.message : String(e)}`); } },
        setCurrentTurn: (turn) => { this._currentTurn = turn; },
        setMasterLlmStartLogged: () => { this.masterLlmStartLogged = true; },
        isMasterLlmStartLogged: () => this.masterLlmStartLogged,
        setSystemTurnBegan: () => { this.systemTurnBegan = true; },
        isSystemTurnBegan: () => this.systemTurnBegan,
        setCurrentLlmOpId: (opId) => { this.currentLlmOpId = opId; },
        getCurrentLlmOpId: () => this.currentLlmOpId,
        applyToolResponseCap: (result, limitBytes, logs, context) => this.applyToolResponseCap(result, limitBytes, logs, context),
      };

      const turnRunner = new TurnRunner(turnRunnerContext, turnRunnerCallbacks);
      const result = await turnRunner.execute(
        currentConversation,
        currentLogs,
        currentAccounting,
        currentTurn,
        this.childConversations,
        this.plannedSubturns
      );

      // System finalization log - must be added before merging logs
      try {
        if (!this.systemTurnBegan) { this.opTree.beginTurn(0, { system: true, label: 'init' }); this.systemTurnBegan = true; }
        const finOp = this.opTree.beginOp(0, 'system', { label: 'fin' });
        this.log({ timestamp: Date.now(), severity: 'VRB', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'agent:fin', fatal: false, message: 'session finalized' }, { opId: finOp });
        this.opTree.endOp(finOp, 'ok');
        // End system turn
        this.opTree.endTurn(0);
        this.opTree.endSession(result.success, result.error);
      } catch (e) { warn(`endSession failed: ${e instanceof Error ? e.message : String(e)}`); }

      // Derive arrays from opTree for canonical output
      const flat = (() => { try { return this.opTree.flatten(); } catch { return { logs: this.logs, accounting: this.accounting }; } })();
      const mergedLogs = (() => {
        const combined = [...flat.logs];
        const seen = new Set<string>();
        combined.forEach((entry) => {
          const key = `${String(entry.timestamp)}:${entry.remoteIdentifier}:${entry.message}`;
          seen.add(key);
        });
        this.logs.forEach((entry) => {
          const key = `${String(entry.timestamp)}:${entry.remoteIdentifier}:${entry.message}`;
          if (!seen.has(key)) {
            combined.push(entry);
            seen.add(key);
          }
        });
        return combined;
      })();
      const resultShape = {
        success: result.success,
        error: result.error,
        conversation: result.conversation,
        logs: mergedLogs,
        accounting: flat.accounting,
        finalReport: result.finalReport,
        childConversations: result.childConversations,
        // Provide ASCII tree for downstream consumers (CLI may choose to print)
        treeAscii: undefined,
        opTreeAscii: (() => { try { return this.opTree.renderAscii(); } catch { return undefined; } })(),
        opTree: (() => { try { return this.opTree.getSession(); } catch { return undefined; } })(),
      } as AIAgentResult;
      this.emitAgentCompletion(result.success, result.error);
      // Phase B/C: persist final session and accounting ledger
      try {
        await this.persistSessionSnapshot('final');
        await this.flushAccounting(resultShape.accounting);
      } catch (e) {
        warn(`final persistence failed: ${e instanceof Error ? e.message : String(e)}`);
      }
      span.setAttributes({
        'ai.agent.success': resultShape.success,
        'ai.agent.turn_count': currentTurn,
      });
      if (typeof resultShape.finalReport?.status === 'string' && resultShape.finalReport.status.length > 0) {
        span.setAttribute('ai.agent.final_report.status', resultShape.finalReport.status);
      }
      if (!resultShape.success && typeof resultShape.error === 'string' && resultShape.error.length > 0) {
        span.setStatus({ code: SpanStatusCode.ERROR, message: resultShape.error });
      }
      return resultShape;

      } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      const logEntry: LogEntry = {
        timestamp: Date.now(),
        severity: 'ERR',
        turn: currentTurn,
        subturn: 0,
        direction: 'response',
        type: 'llm',
        remoteIdentifier: 'agent:error',
        fatal: true,
        message: `AI Agent failed: ${message}`
      };
      this.log(logEntry);
      
      // Log exit for uncaught exception
      this.logExit('EXIT-UNCAUGHT-EXCEPTION', `Uncaught exception: ${message}`, currentTurn);

      // Emit FIN summary even on failure
      this.emitFinalSummary(currentLogs, currentAccounting);
      const flatFail = (() => { try { return this.opTree.flatten(); } catch { return { logs: this.logs, accounting: this.accounting }; } })();
      const mergedFailLogs = (() => {
        const combined = [...flatFail.logs];
        const seen = new Set<string>();
        combined.forEach((entry) => {
          const key = `${String(entry.timestamp)}:${entry.remoteIdentifier}:${entry.message}`;
          seen.add(key);
        });
        this.logs.forEach((entry) => {
          const key = `${String(entry.timestamp)}:${entry.remoteIdentifier}:${entry.message}`;
          if (!seen.has(key)) {
            combined.push(entry);
            seen.add(key);
          }
        });
        return combined;
      })();
      const failShape = {
        success: false,
        error: message,
        conversation: currentConversation,
        logs: mergedFailLogs,
        accounting: flatFail.accounting
      } as AIAgentResult;
      try {
        if (!this.systemTurnBegan) { this.opTree.beginTurn(0, { system: true, label: 'init' }); this.systemTurnBegan = true; }
        const finOp = this.opTree.beginOp(0, 'system', { label: 'fin' });
        this.log({ timestamp: Date.now(), severity: 'VRB', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'agent:fin', fatal: false, message: `session finalization (error)` }, { opId: finOp });
        this.opTree.endOp(finOp, 'ok');
        this.opTree.endTurn(0);
        this.opTree.endSession(false, message);
      } catch (e) { warn(`endSession failed: ${e instanceof Error ? e.message : String(e)}`); }
      this.emitAgentCompletion(false, message);
      if (error instanceof Error) {
        span.recordException(error);
      } else {
        span.recordException(new Error(message));
      }
      span.setStatus({ code: SpanStatusCode.ERROR, message });
      return failShape;
      } finally {
        await this.toolsOrchestrator?.cleanup();
      }
    });
  }


  private finalizeCanceledSession(
    conversation: ConversationMessage[],
    logs: LogEntry[],
    accounting: AccountingEntry[]
  ): AIAgentResult {
    const errMsg = 'canceled';
    this.emitFinalSummary(logs, accounting);
    try {
      if (!this.systemTurnBegan) { this.opTree.beginTurn(0, { system: true, label: 'init' }); this.systemTurnBegan = true; }
      const finOp = this.opTree.beginOp(0, 'system', { label: 'fin' });
      this.log({ timestamp: Date.now(), severity: 'ERR', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'agent:fin', fatal: false, message: 'session finalized after uncaught error' }, { opId: finOp });
      this.opTree.endOp(finOp, 'ok');
      this.opTree.endTurn(0);
      this.opTree.endSession(false, errMsg);
    } catch (e) { warn(`endSession failed: ${e instanceof Error ? e.message : String(e)}`); }
    return { success: false, error: errMsg, conversation, logs: this.logs, accounting };
  }

  private finalizeGracefulStopSession(
    conversation: ConversationMessage[],
    logs: LogEntry[],
    accounting: AccountingEntry[]
  ): AIAgentResult {
    this.emitFinalSummary(logs, accounting);
    try {
      if (!this.systemTurnBegan) { this.opTree.beginTurn(0, { system: true, label: 'init' }); this.systemTurnBegan = true; }
      const finOp = this.opTree.beginOp(0, 'system', { label: 'fin' });
      this.log({ timestamp: Date.now(), severity: 'VRB', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'agent:fin', fatal: false, message: 'session finalized' }, { opId: finOp });
      this.opTree.endOp(finOp, 'ok');
      this.opTree.endTurn(0);
      this.opTree.endSession(true);
    } catch (e) { warn(`endSession failed: ${e instanceof Error ? e.message : String(e)}`); }
    return { success: true, conversation, logs: this.logs, accounting } as AIAgentResult;
  }

  private async sleepWithAbort(
    ms: number
  ): Promise<'completed' | 'aborted_cancel' | 'aborted_stop'> {
    if (ms <= 0) return 'completed';
    if (this.canceled) return 'aborted_cancel';
    if (this.stopRef?.stopping === true) return 'aborted_stop';

    return await new Promise<'completed' | 'aborted_cancel' | 'aborted_stop'>((resolve) => {
      let settled = false;
      let timer: ReturnType<typeof setTimeout> | undefined;
      let stopInterval: ReturnType<typeof setInterval> | undefined;
      let abortHandler: (() => void) | undefined;

      const cleanup = () => {
        if (timer !== undefined) clearTimeout(timer);
        if (stopInterval !== undefined) clearInterval(stopInterval);
        if (abortHandler !== undefined && this.abortSignal !== undefined) {
          try { this.abortSignal.removeEventListener('abort', abortHandler); } catch (e) { warn(`abort cleanup failed: ${e instanceof Error ? e.message : String(e)}`); }
        }
      };

      const finish = (result: 'completed' | 'aborted_cancel' | 'aborted_stop') => {
        if (settled) return;
        settled = true;
        cleanup();
        resolve(result);
      };

      timer = setTimeout(() => {
        finish('completed');
      }, ms);

      if (this.abortSignal !== undefined) {
        abortHandler = () => {
          this.canceled = true;
          finish('aborted_cancel');
        };
        try { this.abortSignal.addEventListener('abort', abortHandler, { once: true }); } catch (e) { warn(`abortSignal listener failed: ${e instanceof Error ? e.message : String(e)}`); }
      }

      if (this.stopRef !== undefined) {
        stopInterval = setInterval(() => {
          if (this.stopRef?.stopping === true) {
            finish('aborted_stop');
          }
        }, 100);
      }
    });
  }

  // Compose and emit FIN summary log
  private emitFinalSummary(logs: LogEntry[], accounting: AccountingEntry[]): void {
    try {
      // Small helpers (none needed for condensed FIN logs)

      // Token totals and latency
      const llmEntries = accounting.filter((e): e is LLMAccountingEntry => e.type === 'llm');
      const tokIn = llmEntries.reduce((s, e) => s + e.tokens.inputTokens, 0);
      const tokOut = llmEntries.reduce((s, e) => s + e.tokens.outputTokens, 0);
      const tokCacheRead = llmEntries.reduce((s, e) => s + (e.tokens.cacheReadInputTokens ?? e.tokens.cachedTokens ?? 0), 0);
      const tokCacheWrite = llmEntries.reduce((s, e) => s + (e.tokens.cacheWriteInputTokens ?? 0), 0);
      // Total should include all four components: input + output + cacheR + cacheW
      const tokTotal = tokIn + tokOut + tokCacheRead + tokCacheWrite;
      const llmLatencies = llmEntries.map((e) => e.latency);
      const llmLatencySum = llmLatencies.reduce((s, v) => s + v, 0);
      const llmLatencyAvg = llmEntries.length > 0 ? Math.round(llmLatencySum / llmEntries.length) : 0;
      const totalCost = llmEntries.reduce((s, e) => s + (e.costUsd ?? 0), 0);
      const totalUpstreamCost = llmEntries.reduce((s, e) => s + (e.upstreamInferenceCostUsd ?? 0), 0);

      // Requests/failures
      const llmRequests = llmEntries.length;
      const llmFailures = llmEntries.filter((e) => e.status === 'failed').length;

      // Providers/models by configured/actual:model with ok/failed
      interface PairStats { total: number; ok: number; failed: number }
      const pairStats = new Map<string, PairStats>();
      llmEntries.forEach((e) => {
        const key = (e.actualProvider !== undefined && e.actualProvider.length > 0 && e.actualProvider !== e.provider)
        ? `${e.provider}/${e.actualProvider}:${e.model}`
        : `${e.provider}:${e.model}`;
        const curr = pairStats.get(key) ?? { total: 0, ok: 0, failed: 0 };
        curr.total += 1;
        if (e.status === 'failed') curr.failed += 1; else curr.ok += 1;
        pairStats.set(key, curr);
      });
      const pairsStr = [...pairStats.entries()]
        .map(([key, s]) => `${String(s.total)}x [${String(s.ok)}+${String(s.failed)}] ${key}`)
        .join(', ');

      const stopReasonStats = new Map<string, number>();
      llmEntries.forEach((entry) => {
        const reason = entry.stopReason;
        if (typeof reason === 'string' && reason.length > 0) {
          stopReasonStats.set(reason, (stopReasonStats.get(reason) ?? 0) + 1);
        }
      });
      const stopReasonStr = [...stopReasonStats.entries()]
        .map(([reason, count]) => `${reason}(${String(count)})`)
        .join(', ');

      let msg = `requests=${String(llmRequests)} failed=${String(llmFailures)}, tokens prompt=${String(tokIn)} output=${String(tokOut)} cacheR=${String(tokCacheRead)} cacheW=${String(tokCacheWrite)} total=${String(tokTotal)}, cost total=$${totalCost.toFixed(5)} upstream=$${totalUpstreamCost.toFixed(5)}, latency sum=${String(llmLatencySum)}ms avg=${String(llmLatencyAvg)}ms, providers/models: ${pairsStr.length > 0 ? pairsStr : 'none'}`;
      if (stopReasonStr.length > 0) {
        msg += `, stop reasons: ${stopReasonStr}`;
      }
      const fin: LogEntry = {
        timestamp: Date.now(),
        severity: 'FIN',
        turn: this.currentTurn,
        subturn: 0,
        direction: 'response',
        type: 'llm',
        remoteIdentifier: 'summary',
        fatal: false,
        message: msg,
      };
      this.log(fin);

      // MCP summary single line
      const toolEntries = accounting.filter((e): e is ToolAccountingEntry => e.type === 'tool');
      const totalToolCharsIn = toolEntries.reduce((s, e) => s + e.charactersIn, 0);
      const totalToolCharsOut = toolEntries.reduce((s, e) => s + e.charactersOut, 0);
      const mcpRequests = toolEntries.length;
      const mcpFailures = toolEntries.filter((e) => e.status === 'failed').length;
      interface ToolStats { total: number; ok: number; failed: number }
      const byToolStats = new Map<string, ToolStats>();
      toolEntries.forEach((e) => {
        const key = `${e.mcpServer}:${e.command}`;
        const curr = byToolStats.get(key) ?? { total: 0, ok: 0, failed: 0 };
        curr.total += 1;
        if (e.status === 'failed') curr.failed += 1; else curr.ok += 1;
        byToolStats.set(key, curr);
      });
      const toolPairsStr = [...byToolStats.entries()]
        .map(([k, s]) => `${String(s.total)}x [${String(s.ok)}+${String(s.failed)}] ${k}`)
        .join(', ');
      const sizeCaps = this.centralSizeCapHits;
      const finMcp: LogEntry = {
        timestamp: Date.now(),
        severity: 'FIN',
        turn: this.currentTurn,
        subturn: 0,
        direction: 'response',
        type: 'tool',
        remoteIdentifier: 'summary',
        fatal: false,
        message: `requests=${String(mcpRequests)}, failed=${String(mcpFailures)}, capped=${String(sizeCaps)}, bytes in=${String(totalToolCharsIn)} out=${String(totalToolCharsOut)}, providers/tools: ${toolPairsStr.length > 0 ? toolPairsStr : 'none'}`,
      };
      this.log(finMcp);
    } catch { /* swallow summary errors */ }
  }
  private pushSystemRetryMessage(conversation: ConversationMessage[], message: string): void {
    const trimmed = message.trim();
    if (trimmed.length === 0) return;
    if (this.pendingRetryMessages.includes(trimmed)) return;
    this.pendingRetryMessages.push(trimmed);
  }

  private reportContextGuardEvent(params: {
    provider: string;
    model: string;
    trigger: 'tool_preflight' | 'turn_preflight';
    outcome: 'skipped_provider' | 'forced_final';
    limitTokens?: number;
    projectedTokens: number;
    remainingTokens?: number;
  }): void {
    const { provider, model, trigger, outcome, limitTokens, projectedTokens } = params;
    const directRemaining = params.remainingTokens;
    const hasDirectRemaining = typeof directRemaining === 'number' && Number.isFinite(directRemaining);
    const computedRemaining = hasDirectRemaining
      ? Math.max(0, directRemaining)
      : ((typeof limitTokens === 'number' && Number.isFinite(limitTokens))
        ? Math.max(0, limitTokens - projectedTokens)
        : undefined);

    recordContextGuardMetrics({
      agentId: this.sessionConfig.agentId,
      callPath: this.callPath,
      headendId: this.headendId,
      provider,
      model,
      trigger,
      outcome,
      limitTokens,
      projectedTokens,
      remainingTokens: computedRemaining,
    });

    const attributes: Attributes = {
      'ai.context.trigger': trigger,
      'ai.context.outcome': outcome,
      'ai.context.provider': provider,
      'ai.context.model': model,
      'ai.context.projected_tokens': projectedTokens,
    } satisfies Attributes;
    if (typeof limitTokens === 'number' && Number.isFinite(limitTokens)) {
      attributes['ai.context.limit_tokens'] = limitTokens;
    }
    if (computedRemaining !== undefined) {
      attributes['ai.context.remaining_tokens'] = computedRemaining;
    }
    addSpanEvent('context.guard', attributes);
  }

  private mergePendingRetryMessages(conversation: ConversationMessage[]): ConversationMessage[] {
    const cleaned = conversation.filter((msg) => msg.metadata?.retryMessage === undefined);
    if (cleaned.length !== conversation.length) {
      conversation.splice(0, conversation.length, ...cleaned);
    }
    if (this.pendingRetryMessages.length === 0) {
      return conversation;
    }
    const retryMessages = this.pendingRetryMessages.map((text) => ({ role: 'user' as const, content: text }));
    return [...conversation, ...retryMessages];
  }

  // Context guard delegation methods
  private estimateTokensForCounters(messages: ConversationMessage[]): number {
    return this.contextGuard.estimateTokens(messages);
  }

  private estimateToolSchemaTokens(tools: readonly MCPTool[]): number {
    return this.contextGuard.estimateToolSchemaTokens(tools);
  }

  private computeMaxOutputTokens(contextWindow: number): number {
    return this.contextGuard.computeMaxOutputTokens(contextWindow);
  }

  // Token counter accessors delegating to contextGuard
  private get currentCtxTokens(): number { return this.contextGuard.getCurrentTokens(); }
  private set currentCtxTokens(value: number) { this.contextGuard.setCurrentTokens(value); }
  private get pendingCtxTokens(): number { return this.contextGuard.getPendingTokens(); }
  private set pendingCtxTokens(value: number) { this.contextGuard.setPendingTokens(value); }
  private get newCtxTokens(): number { return this.contextGuard.getNewTokens(); }
  private set newCtxTokens(value: number) { this.contextGuard.setNewTokens(value); }
  private get schemaCtxTokens(): number { return this.contextGuard.getSchemaTokens(); }
  private set schemaCtxTokens(value: number) { this.contextGuard.setSchemaTokens(value); }
  private get forcedFinalTurnReason(): 'context' | undefined { return this.contextGuard.getForcedFinalReason(); }
  private get contextLimitWarningLogged(): boolean { return this.contextGuard.hasLoggedContextWarning(); }
  private set contextLimitWarningLogged(value: boolean) { if (value) this.contextGuard.markContextWarningLogged(); }

  private evaluateContextGuard(extraTokens = 0): ContextGuardEvaluation {
    return this.contextGuard.evaluate(extraTokens);
  }

  private buildContextMetrics(provider: string, model: string): TurnRequestContextMetrics {
    return this.contextGuard.buildMetrics(provider, model);
  }


  /**
   * Callback handler for ContextGuard when tool preflight forces final turn.
   * Handles AIAgentSession-specific side effects (telemetry, logging, system message).
   */
  private handleContextGuardForcedFinalTurn(blocked: ContextGuardBlockedEntry[], trigger: 'tool_preflight' | 'turn_preflight'): void {
    // Report telemetry for each blocked provider/model
    if (blocked.length > 0) {
      blocked.forEach((entry) => {
        const remainingTokens = Number.isFinite(entry.limit)
          ? Math.max(0, entry.limit - entry.projected)
          : undefined;
        this.reportContextGuardEvent({
          provider: entry.provider,
          model: entry.model,
          trigger,
          outcome: 'forced_final',
          limitTokens: entry.limit,
          projectedTokens: entry.projected,
          remainingTokens,
        });
      });
    }

    // AIAgentSession-specific side effects
    this.logEnteringFinalTurn('context', this.currentTurn);
    this.pushSystemRetryMessage(this.conversation, CONTEXT_FINAL_MESSAGE);

    // Log warning if not already logged
    if (this.contextLimitWarningLogged) {
      return;
    }
    const first = blocked[0] ?? {
      provider: 'unknown',
      model: 'unknown',
      contextWindow: ContextGuard.DEFAULT_CONTEXT_WINDOW_TOKENS,
      bufferTokens: ContextGuard.DEFAULT_CONTEXT_BUFFER_TOKENS,
      maxOutputTokens: this.computeMaxOutputTokens(ContextGuard.DEFAULT_CONTEXT_WINDOW_TOKENS),
      limit: 0,
      projected: 0,
    };
    const remainingTokens = Number.isFinite(first.limit)
      ? Math.max(0, first.limit - first.projected)
      : undefined;
    const message = `Context budget exceeded (${first.provider}:${first.model} - projected ${String(first.projected)} tokens vs limit ${String(first.limit)}). Forcing final turn.`;
    const warnEntry: LogEntry = {
      timestamp: Date.now(),
      severity: 'WRN',
      turn: this.currentTurn,
      subturn: 0,
      direction: 'response',
      type: 'llm',
      remoteIdentifier: AIAgentSession.REMOTE_CONTEXT,
      fatal: false,
      message,
      details: {
        projected_tokens: first.projected,
        limit_tokens: first.limit,
        ...(remainingTokens !== undefined ? { remaining_tokens: remainingTokens } : {}),
      },
    };
    if (process.env.CONTEXT_DEBUG === 'true') {
      console.log('context-guard/log-entry', warnEntry);
    }
    this.log(warnEntry);
    this.contextLimitWarningLogged = true;
  }


  private logEnteringFinalTurn(reason: 'context' | 'max_turns', turn: number): void {
    if (this.finalTurnEntryLogged) return;
    const message = reason === 'context'
      ? `Context guard enforced: restricting tools to \`${AIAgentSession.FINAL_REPORT_TOOL}\` and injecting finalization instruction.`
      : `Final turn (${String(turn)}) detected: restricting tools to \`${AIAgentSession.FINAL_REPORT_TOOL}\`.`;
    const warnEntry: LogEntry = {
      timestamp: Date.now(),
      severity: 'WRN',
      turn,
      subturn: 0,
      direction: 'request',
      type: 'llm',
      remoteIdentifier: AIAgentSession.REMOTE_FINAL_TURN,
      fatal: false,
      message,
    };
    this.log(warnEntry);
    this.finalTurnEntryLogged = true;
  }

  private logFinalReportAccepted(source: 'tool-call' | PendingFinalReportSource | 'synthetic', turn: number): void {
    const entry: LogEntry = {
      timestamp: Date.now(),
      severity: source === 'tool-call' ? 'VRB' : (source === 'synthetic' ? 'ERR' : 'WRN'),
      turn,
      subturn: 0,
      direction: 'response',
      type: 'llm',
      remoteIdentifier: 'agent:final-report-accepted',
      fatal: false,
      message: source === 'tool-call'
        ? 'Final report accepted from tool call.'
        : source === FINAL_REPORT_SOURCE_TEXT_FALLBACK
          ? 'Final report accepted from text extraction fallback.'
          : source === FINAL_REPORT_SOURCE_TOOL_MESSAGE
            ? 'Final report accepted from tool-message fallback.'
            : 'Synthetic final report generated.',
      details: { source }
    };
    this.log(entry);
  }

  private logFallbackAcceptance(source: PendingFinalReportSource, turn: number): void {
    const entry: LogEntry = {
      timestamp: Date.now(),
      severity: 'WRN',
      turn,
      subturn: 0,
      direction: 'response',
      type: 'llm',
      remoteIdentifier: 'agent:fallback-report',
      fatal: false,
      message: source === FINAL_REPORT_SOURCE_TEXT_FALLBACK
        ? 'Accepting final report from text extraction as last resort (final turn, no valid tool call).'
        : 'Accepting final report from tool-message fallback as last resort (final turn, no valid tool call).'
    };
    this.log(entry);
  }



  private commitFinalReport(payload: PendingFinalReportPayload, source: 'tool-call' | PendingFinalReportSource | 'synthetic'): void {
    this.finalReportManager.commit(payload, source);
  }


  private filterToolsForProvider(tools: MCPTool[], provider: string): { tools: MCPTool[]; allowedNames: Set<string> } {
    void provider;
    const allowedNames = new Set<string>();
    const filtered = tools.map((tool) => {
      const sanitized = sanitizeToolName(tool.name);
      allowedNames.add(sanitized);
      return tool;
    });

    if (!allowedNames.has(AIAgentSession.FINAL_REPORT_TOOL)) {
      const fallback = tools.find((tool) => sanitizeToolName(tool.name) === AIAgentSession.FINAL_REPORT_TOOL);
      if (fallback !== undefined) allowedNames.add(AIAgentSession.FINAL_REPORT_TOOL);
    }

    return { tools: filtered, allowedNames };
  }


  private resolveReasoningMapping(provider: string, model: string): ProviderReasoningMapping | null | undefined {
    const providerConfig = this.sessionConfig.config.providers[provider] as (Configuration['providers'][string] | undefined);
    if (providerConfig === undefined) return undefined;
    const modelReasoning = providerConfig.models?.[model]?.reasoning;
    if (modelReasoning !== undefined) return modelReasoning;
    if (providerConfig.reasoning !== undefined) return providerConfig.reasoning;
    return undefined;
  }

  private resolveToolChoice(provider: string, model: string, toolsCount: number): ToolChoiceMode | undefined {
    const transport = this.sessionConfig.toolingTransport ?? 'xml-final';
    if (transport !== 'native') {
      if (toolsCount === 0) return undefined;
      return 'auto';
    }
    const providerConfig = this.sessionConfig.config.providers[provider] as (Configuration['providers'][string] | undefined);
    if (providerConfig === undefined) {
      return undefined;
    }
    const modelChoice = providerConfig.models?.[model]?.toolChoice;
    if (modelChoice !== undefined) {
      return modelChoice;
    }
    return providerConfig.toolChoice;
  }

  private resolveReasoningValue(provider: string, model: string, level: ReasoningLevel, maxOutputTokens: number | undefined): ProviderReasoningValue | null | undefined {
    const mapping = this.resolveReasoningMapping(provider, model);
    return this.llmClient.resolveReasoningValue(provider, { level, mapping, maxOutputTokens });
  }

  private addTurnFailure(reason: string): void {
    const trimmed = reason.trim();
    if (trimmed.length === 0) return;
    this.turnFailureReasons.push(trimmed);
  }

  private enhanceSystemPrompt(systemPrompt: string, toolsInstructions: string): string {
    const blocks: string[] = [systemPrompt];
    if (toolsInstructions.trim().length > 0) blocks.push(`## TOOLS' INSTRUCTIONS\n\n${toolsInstructions}`);
    return blocks.join('\n\n');
  }


  // Immutable retry method - returns new session
  retry(): AIAgentSession {
    // Create new session with same configuration but advanced state
    return new AIAgentSession(
      this.config,
      this.conversation,
      this.logs,
      this.accounting,
      false, // reset success
      undefined, // reset error
      this.currentTurn,
      this.llmClient, // reuse LLM client
      this.sessionConfig
    );
  }

}

// Export session class as main interface
export { AIAgentSession as AIAgent };
