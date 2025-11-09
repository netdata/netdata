import crypto from 'node:crypto';
import fs from 'node:fs';
// import os from 'node:os';
import path from 'node:path';

import { SpanKind, SpanStatusCode, type Attributes } from '@opentelemetry/api';
import Ajv from 'ajv';
import { Mutex } from 'async-mutex';

import type { OutputFormatId } from './formats.js';
import type { SessionNode } from './session-tree.js';
import type { AIAgentSessionConfig, AIAgentResult, ConversationMessage, LogEntry, AccountingEntry, Configuration, TurnRequest, LLMAccountingEntry, MCPTool, ToolAccountingEntry, RestToolConfig, ProgressMetrics, ToolCall, SessionSnapshotPayload, AccountingFlushPayload, ReasoningLevel, ProviderReasoningMapping, ProviderReasoningValue, TurnRetryDirective, TurnStatus, LogDetailValue, ToolChoiceMode, LogPayload, TurnRequestContextMetrics } from './types.js';
import type { Ajv as AjvClass, ErrorObject, Options as AjvOptions } from 'ajv';

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


const FINAL_REPORT_FORMAT_VALUES = ['json', 'markdown', 'markdown+mermaid', 'slack-block-kit', 'tty', 'pipe', 'sub-agent', 'text'] as const satisfies readonly NonNullable<AIAgentResult['finalReport']>['format'][];

// empty line between import groups enforced by linter
import { validateProviders, validateMCPServers, validatePrompts } from './config.js';
import { parseFrontmatter, parsePairs, extractBodyWithoutFrontmatter } from './frontmatter.js';
import { LLMClient } from './llm-client.js';
import { buildPromptVars, applyFormat, expandVars } from './prompt-builder.js';
import { SessionProgressReporter } from './session-progress-reporter.js';
import { SessionTreeBuilder } from './session-tree.js';
import { SubAgentRegistry } from './subagent-registry.js';
import { recordContextGuardMetrics, recordLlmMetrics, runWithSpan, addSpanAttributes, addSpanEvent } from './telemetry/index.js';
import { estimateMessagesTokens, resolveTokenizer } from './tokenizer-registry.js';
import { AgentProvider } from './tools/agent-provider.js';
import { InternalToolProvider } from './tools/internal-provider.js';
import { MCPProvider } from './tools/mcp-provider.js';
import { RestProvider } from './tools/rest-provider.js';
import { ToolsOrchestrator, type ManagedToolResult, type ToolBudgetCallbacks } from './tools/tools.js';
import { appendCallPathSegment, clampToolName, formatToolRequestCompact, normalizeCallPath, sanitizeToolName, truncateUtf8WithNotice, warn } from './utils.js';

// Immutable session class according to DESIGN.md
type AjvInstance = AjvClass;
type AjvErrorObject = ErrorObject<string, Record<string, unknown>>;
type AjvConstructor = new (options?: AjvOptions) => AjvInstance;
const AjvCtor: AjvConstructor = Ajv as unknown as AjvConstructor;

const estimateMessagesBytes = (messages: readonly ConversationMessage[] | undefined): number => {
  if (messages === undefined || messages.length === 0) return 0;
  return messages.reduce((total, message) => total + safeJsonByteLength(message), 0);
};

const safeJsonByteLength = (value: unknown): number => {
  try {
    return Buffer.byteLength(JSON.stringify(value), 'utf8');
  } catch {
    if (typeof value === 'string') return Buffer.byteLength(value, 'utf8');
    if (value === null || value === undefined) return 0;
    if (typeof value === 'object') {
      let total = 0;
      Object.values(value as Record<string, unknown>).forEach((nested) => {
        total += safeJsonByteLength(nested);
      });
      return total;
    }
    return 0;
  }
};

interface TargetContextConfig {
  provider: string;
  model: string;
  contextWindow: number;
  tokenizerId?: string;
  bufferTokens: number;
}

interface ContextGuardBlockedEntry {
  provider: string;
  model: string;
  contextWindow: number;
  bufferTokens: number;
  maxOutputTokens: number;
  limit: number;
  projected: number;
}

interface ContextGuardEvaluation {
  blocked: ContextGuardBlockedEntry[];
  projectedTokens: number;
}


export class AIAgentSession {
  // Log identifiers (avoid duplicate string literals)
  private static readonly REMOTE_AGENT_TOOLS = 'agent:tools';
  private static readonly REMOTE_FINAL_TURN = 'agent:final-turn';
  private static readonly REMOTE_CONTEXT = 'agent:context';
  private static readonly FINAL_REPORT_TOOL = 'agent__final_report';
  private static readonly FINAL_REPORT_SHORT = 'final_report';
  private static readonly MAX_TURNS_FINAL_MESSAGE = 'Maximum number of turns/steps reached. You must provide your final report now, by calling the `agent__final_report` tool. Do not attempt to call any other tool. Read carefully the instructions on how to call the `agent__final_report` tool and call it now.';
  private static readonly CONTEXT_FINAL_MESSAGE = 'The conversation is at the context window limit. You must call `agent__final_report` immediately to deliver your final answer using the information already gathered. Do not call any other tools.';
  private static readonly TOOL_NO_OUTPUT = '(no output)';
  private static readonly RETRY_ACTION_SKIP_PROVIDER = 'skip-provider';
  private static readonly DEFAULT_CONTEXT_WINDOW_TOKENS = 131072;
  private static readonly DEFAULT_CONTEXT_BUFFER_TOKENS = 256;
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
  private ajv?: AjvInstance;
  private readonly toolFailureMessages = new Map<string, string>();
  private readonly toolFailureFallbacks: string[] = [];
  private readonly targetContextConfigs: TargetContextConfig[];
  private currentCtxTokens = 0;
  private pendingCtxTokens = 0;
  private newCtxTokens = 0;
  private schemaCtxTokens = 0;
  private toolBudgetExceeded = false;
  private readonly toolBudgetMutex = new Mutex();
  private readonly toolBudgetCallbacks: ToolBudgetCallbacks;
  private forcedFinalTurnReason?: 'context';
  private contextLimitWarningLogged = false;
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
  // Per-turn planned subturns (tool call count) discovered when LLM yields toolCalls
  private plannedSubturns: Map<number, number> = new Map<number, number>();
  private resolvedFormat?: OutputFormatId;
  private resolvedFormatPromptValue?: string;
  private resolvedFormatParameterDescription?: string;
  private resolvedUserPrompt?: string;
  private resolvedSystemPrompt?: string;
  private masterLlmStartLogged = false;
  private sessionTitle?: { title: string; emoji?: string; ts: number };
  // Finalization state captured via agent__final_report
  private finalReport?: {
    status: 'success' | 'failure' | 'partial';
    // Allow all known output formats plus legacy 'text'
    format: 'json' | 'markdown' | 'markdown+mermaid' | 'slack-block-kit' | 'tty' | 'pipe' | 'sub-agent' | 'text';
    content?: string;
    content_json?: Record<string, unknown>;
    metadata?: Record<string, unknown>;
    ts: number;
  };
  private pendingRetryMessages: string[] = [];

  // Counters for summary
  private llmAttempts = 0;
  private llmSyntheticFailures = 0;
  private centralSizeCapHits = 0;
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
  ): { temperature?: number | null; topP?: number | null } {
    const providers = this.sessionConfig.config.providers;
    const providerConfig = Object.prototype.hasOwnProperty.call(providers, provider)
      ? providers[provider]
      : undefined;
    if (providerConfig === undefined) return {};
    const modelConfig = providerConfig.models?.[model];
    const overrides = modelConfig?.overrides;
    if (overrides === undefined) return {};
    const result: { temperature?: number | null; topP?: number | null } = {};

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
    this.abortSignal = sessionConfig.abortSignal;
    this.stopRef = sessionConfig.stopRef;
    this.headendId = sessionConfig.headendId;
    const telemetryLabels = { ...(sessionConfig.telemetryLabels ?? {}) };
    if (this.headendId !== undefined && !Object.prototype.hasOwnProperty.call(telemetryLabels, 'headend')) {
      telemetryLabels.headend = this.headendId;
    }
    this.telemetryLabels = telemetryLabels;
    const sessionTargets = Array.isArray(this.sessionConfig.targets) ? this.sessionConfig.targets : [];
    const defaultBuffer = this.config.defaults?.contextWindowBufferTokens ?? AIAgentSession.DEFAULT_CONTEXT_BUFFER_TOKENS;
    const defaultContextWindow = AIAgentSession.DEFAULT_CONTEXT_WINDOW_TOKENS;
    this.targetContextConfigs = sessionTargets.map((target) => {
      const providerConfig = this.config.providers[target.provider];
      const modelConfig = providerConfig.models?.[target.model];
      const contextWindow = modelConfig?.contextWindow
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
    this.currentCtxTokens = this.estimateTokensForCounters(this.conversation);
    if (process.env.CONTEXT_DEBUG === 'true') {
      console.log('context-guard/init-counters', {
        currentCtxTokens: this.currentCtxTokens,
        pendingCtxTokens: this.pendingCtxTokens,
        newCtxTokens: this.newCtxTokens,
        schemaCtxTokens: this.schemaCtxTokens,
        conversationMessages: this.conversation.length,
      });
    }
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
    this.toolBudgetCallbacks = {
      reserveToolOutput: async (output: string) => await this.toolBudgetMutex.runExclusive(() => {
        const tokens = this.estimateTokensForCounters([{ role: 'tool', content: output }]);
        const guard = this.evaluateContextGuard(tokens);
        if (guard.blocked.length > 0) {
          if (!this.toolBudgetExceeded) {
            this.toolBudgetExceeded = true;
            this.enforceContextFinalTurn(guard.blocked, 'tool_preflight');
          }
          return { ok: false as const, tokens, reason: 'token_budget_exceeded' };
        }
        return { ok: true as const, tokens };
      }),
      canExecuteTool: () => !this.toolBudgetExceeded,
    } satisfies ToolBudgetCallbacks;
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
    this.toolBudgetCallbacks);
    orch.register(new MCPProvider('mcp', sessionConfig.config.mcpServers, {
      trace: sessionConfig.traceMCP,
      verbose: sessionConfig.verbose,
      requestTimeoutMs: sessionConfig.toolTimeout,
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
      const enableBatch = declaredTools.includes('batch');
      const eo = this.sessionConfig.expectedOutput;
      const expectedJsonSchema = (eo?.format === 'json') ? eo.schema : undefined;
      const internalProvider = new InternalToolProvider('agent', {
        enableBatch,
        outputFormat: this.sessionConfig.outputFormat,
        expectedOutputFormat: eo?.format === 'json' ? 'json' : undefined,
        expectedJsonSchema,
        disableProgressTool: !enableProgressTool,
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
          this.finalReport = {
            status: normalizedStatus,
            format: p.format as 'json'|'markdown'|'markdown+mermaid'|'slack-block-kit'|'tty'|'pipe'|'sub-agent'|'text',
            content: p.content,
            content_json: p.content_json,
            metadata: p.metadata,
            ts: Date.now()
          };
        },
        orchestrator: orch,
        getCurrentTurn: () => this.currentTurn,
        toolTimeoutMs: sessionConfig.toolTimeout
      });
      const formatInfo = internalProvider.getFormatInfo();
      this.resolvedFormat = formatInfo.formatId;
      this.resolvedFormatPromptValue = formatInfo.promptValue;
      this.resolvedFormatParameterDescription = formatInfo.parameterDescription;
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
    turn: number
  ): void {
    const logEntry: LogEntry = {
      timestamp: Date.now(),
      severity: 'VRB',
      turn,
      subturn: 0,
      direction: 'response',
      type: 'llm',
      remoteIdentifier: `agent:${exitCode}`,
      fatal: true,
      message: `${exitCode}: ${reason} (fatal=true)`,
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
          targets: this.sessionConfig.targets,
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
      const toolInstructions = this.toolsOrchestrator?.getCombinedInstructions() ?? '';
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

      // Main agent loop with retry logic
      const result = await this.executeAgentLoop(
        currentConversation,
        currentLogs,
        currentAccounting,
        currentTurn
      );

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
        // Provide ASCII tree for downstream consumers (CLI may choose to print)
        treeAscii: undefined,
        opTreeAscii: (() => { try { return this.opTree.renderAscii(); } catch { return undefined; } })(),
        opTree: (() => { try { return this.opTree.getSession(); } catch { return undefined; } })(),
      } as AIAgentResult;

      try {
        // System finalization log
        if (!this.systemTurnBegan) { this.opTree.beginTurn(0, { system: true, label: 'init' }); this.systemTurnBegan = true; }
        const finOp = this.opTree.beginOp(0, 'system', { label: 'fin' });
        this.log({ timestamp: Date.now(), severity: 'VRB', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'agent:fin', fatal: false, message: 'session finalized' }, { opId: finOp });
        this.opTree.endOp(finOp, 'ok');
        // End system turn
        this.opTree.endTurn(0);
        this.opTree.endSession(result.success, result.error);
      } catch (e) { warn(`endSession failed: ${e instanceof Error ? e.message : String(e)}`); }
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

  private async executeAgentLoop(
    initialConversation: ConversationMessage[],
    initialLogs: LogEntry[],
    initialAccounting: AccountingEntry[],
    startTurn: number
  ): Promise<AIAgentResult> {
    let conversation = [...initialConversation];
    let logs = [...initialLogs];
    let accounting = [...initialAccounting];
    let currentTurn = startTurn;
    let finalReportToolFailed = false;

    this.currentCtxTokens = this.estimateTokensForCounters(conversation);
    this.pendingCtxTokens = 0;
    this.newCtxTokens = 0;
    if (process.env.CONTEXT_DEBUG === 'true') {
      console.log('context-guard/loop-init', {
        currentCtxTokens: this.currentCtxTokens,
        pendingCtxTokens: this.pendingCtxTokens,
        newCtxTokens: this.newCtxTokens,
        schemaCtxTokens: this.schemaCtxTokens,
        turn: currentTurn,
      });
    }

    // Track the last turn where we showed thinking header
    let lastShownThinkingHeaderTurn = -1;

    const finalTurnRetryMessage = 'Final turn did not produce final_report; retrying with next provider/model in this turn.';
    let maxTurns = this.sessionConfig.maxTurns ?? 10;
    const maxRetries: number = this.sessionConfig.maxRetries ?? 3; // GLOBAL attempts cap per turn

    const pairs = this.sessionConfig.targets;

    // Turn loop - necessary for control flow with early termination
    // Turn 0 is initialization; action turns are 1..maxTurns
    // eslint-disable-next-line functional/no-loop-statements
    for (currentTurn = 1; currentTurn <= maxTurns; currentTurn++) {
      if (this.canceled) return this.finalizeCanceledSession(conversation, logs, accounting);
      if (this.stopRef?.stopping === true) {
        // Graceful stop: do not start further turns
        return this.finalizeGracefulStopSession(conversation, logs, accounting);
      }
      this._currentTurn = currentTurn;
      try { this.sessionConfig.callbacks?.onTurnStarted?.(currentTurn); } catch (e) { warn(`onTurnStarted callback failed: ${e instanceof Error ? e.message : String(e)}`); }
      try {
        // Capture effective prompts for this turn (post expansion/enhancement)
        const turnAttrs = {
          prompts: {
            system: (() => { try { return this.resolvedSystemPrompt ?? ''; } catch { return ''; } })(),
            user: (() => { try { return this.resolvedUserPrompt ?? ''; } catch { return ''; } })(),
          }
        } as Record<string, unknown>;
        this.opTree.beginTurn(currentTurn, turnAttrs);
        this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
      } catch (e) { warn(`beginTurn/onOpTree failed: ${e instanceof Error ? e.message : String(e)}`); }
      // Orchestrator receives ctx turn/subturn per call; no MCP client turn management
      this.llmClient.setTurn(currentTurn, 0);
      let lastError: string | undefined;
      let turnSuccessful = false;
      let finalTurnWarnLogged = false;
      let finalTurnRetryWarnLogged = false;

      // Global attempts across all provider/model pairs for this turn
      let attempts = 0;
      let pairCursor = 0;
      let rateLimitedInCycle = 0;
      let maxRateLimitWaitMs = 0;
      // eslint-disable-next-line functional/no-loop-statements
      while (attempts < maxRetries && !turnSuccessful) {
        // Emit the same startup verbose line also when the master LLM runs (once per session)
        if (!this.masterLlmStartLogged && this.parentTxnId === undefined) {
          try {
            const ctx: string = (typeof this.callPath === 'string' && this.callPath.length > 0) ? this.callPath : (this.sessionConfig.agentId ?? 'agent');
            const promptText: string = typeof this.resolvedUserPrompt === 'string' ? this.resolvedUserPrompt : '';
            const entry: LogEntry = {
              timestamp: Date.now(),
              severity: 'VRB',
              turn: currentTurn,
              subturn: 0,
              direction: 'request',
              type: 'llm',
              remoteIdentifier: 'agent:start',
              fatal: false,
              bold: true,
              message: `${ctx}: ${promptText}`,
            };
            this.log(entry);
          } catch (e) { warn(`verbose log failed: ${e instanceof Error ? e.message : String(e)}`); }
          this.masterLlmStartLogged = true;
        }
        const pair = pairs[pairCursor % pairs.length];
        pairCursor += 1;
        const { provider, model } = pair;
        let cycleIndex = 0;
        let cycleComplete = false;

        const providerContextStatus = this.evaluateContextForProvider(provider, model);
        if (providerContextStatus === 'final') {
          const evaluation = this.evaluateContextGuard(this.schemaCtxTokens);
          this.enforceContextFinalTurn(evaluation.blocked, 'turn_preflight');
          this.schemaCtxTokens = this.computeForcedFinalSchemaTokens(provider);
          const postEnforce = this.evaluateContextGuard(this.schemaCtxTokens);
          if (postEnforce.blocked.length > 0) {
            const [firstBlocked] = postEnforce.blocked;
            const warnEntry: LogEntry = {
              timestamp: Date.now(),
              severity: 'WRN',
              turn: currentTurn,
              subturn: 0,
              direction: 'response',
              type: 'llm',
              remoteIdentifier: AIAgentSession.REMOTE_CONTEXT,
              fatal: false,
              message: AIAgentSession.CONTEXT_POST_SHRINK_WARN,
              details: {
                projected_tokens: postEnforce.projectedTokens,
                limit_tokens: firstBlocked.limit,
                provider: firstBlocked.provider,
                model: firstBlocked.model,
              },
            };
            this.log(warnEntry);
          }
        }
        if (providerContextStatus === 'skip') {
          const evaluation = this.evaluateContextGuard(this.schemaCtxTokens);
          const blocked = evaluation.blocked.find((entry) => entry.provider === provider && entry.model === model);
          if (blocked !== undefined) {
            const limitTokens = blocked.limit;
            const remainingTokens = Number.isFinite(blocked.limit)
              ? Math.max(0, blocked.limit - blocked.projected)
              : undefined;
            this.reportContextGuardEvent({
              provider,
              model,
              trigger: 'turn_preflight',
              outcome: 'skipped_provider',
              limitTokens: blocked.limit,
              projectedTokens: blocked.projected,
              remainingTokens,
            });
            const warnEntry: LogEntry = {
              timestamp: Date.now(),
              severity: 'WRN',
              turn: currentTurn,
              subturn: 0,
              direction: 'response',
              type: 'llm',
              remoteIdentifier: AIAgentSession.REMOTE_CONTEXT,
              fatal: false,
              message: `Skipping provider ${provider}:${model} due to projected context size ${String(blocked.projected)} tokens exceeding limit ${String(blocked.limit)}.`,
              details: {
                projected_tokens: blocked.projected,
                limit_tokens: limitTokens,
                ...(remainingTokens !== undefined ? { remaining_tokens: remainingTokens } : {}),
              },
            };
            this.log(warnEntry);
          }
          continue;
        }

          try {
            // Build per-attempt conversation with optional guidance injection
            let attemptConversation = [...conversation];
            // On the last allowed attempt within this turn, nudge the model to use tools (not progress_report)
            if ((attempts === maxRetries - 1) && currentTurn < (maxTurns - 1)) {
              const excludeProgress = this.progressToolEnabled ? ' (excluding `agent__progress_report`)' : '';
              attemptConversation.push({
                role: 'user',
                content: `Reminder: do not end with plain text. Use an available tool${excludeProgress} to make progress. When ready to conclude, call the tool \`${AIAgentSession.FINAL_REPORT_TOOL}\` to provide the final answer.`
              });
            }
            // Force the final-report instruction onto the conversation once we enter the last turn.

            const forcedFinalTurn = this.forcedFinalTurnReason !== undefined;
            const isFinalTurn = forcedFinalTurn || currentTurn === maxTurns;
            if (isFinalTurn) {
              const finalInstruction = this.forcedFinalTurnReason === 'context'
                ? AIAgentSession.CONTEXT_FINAL_MESSAGE
                : AIAgentSession.MAX_TURNS_FINAL_MESSAGE;
              this.pushSystemRetryMessage(conversation, finalInstruction);
              attemptConversation.push({
                role: 'user',
                content: finalInstruction,
                metadata: { retryMessage: 'final-turn-instruction' },
              });
            }
            if (isFinalTurn && !finalTurnWarnLogged) {
              const warn: LogEntry = {
                timestamp: Date.now(),
                severity: 'WRN',
                turn: currentTurn,
                subturn: 0,
                direction: 'request',
                type: 'llm',
                remoteIdentifier: AIAgentSession.REMOTE_FINAL_TURN,
                fatal: false,
                message: forcedFinalTurn
                  ? `Context guard enforced: restricting tools to \`${AIAgentSession.FINAL_REPORT_TOOL}\` and injecting finalization instruction.`
                  : `Final turn detected: restricting tools to \`${AIAgentSession.FINAL_REPORT_TOOL}\` and injecting finalization instruction.`
              };
              this.log(warn);
              finalTurnWarnLogged = true;
            }

            this.llmAttempts++;
            attempts += 1;
            cycleIndex = pairs.length > 0 ? (attempts - 1) % pairs.length : 0;
            cycleComplete = pairs.length > 0 ? (cycleIndex === pairs.length - 1) : false;
            // Begin hierarchical LLM operation (Option C)
            const llmOpId = (() => { try { return this.opTree.beginOp(currentTurn, 'llm', { provider, model, isFinalTurn }); } catch { return undefined; } })();
            this.currentLlmOpId = llmOpId;
            try {
              // Record LLM request summary
              const msgBytes = (() => { try { return new TextEncoder().encode(JSON.stringify(attemptConversation)).length; } catch { return undefined; } })();
              if (llmOpId !== undefined) this.opTree.setRequest(llmOpId, { kind: 'llm', payload: { messages: attemptConversation.length, bytes: msgBytes, isFinalTurn }, size: msgBytes });
            } catch (e) { warn(`final turn warning log failed: ${e instanceof Error ? e.message : String(e)}`); }
            const sessionReasoningLevel = this.sessionConfig.reasoning;
            const sessionReasoningValue = sessionReasoningLevel !== undefined
              ? this.resolveReasoningValue(provider, model, sessionReasoningLevel, this.sessionConfig.maxOutputTokens)
              : undefined;

            if (this.newCtxTokens > 0) {
              this.pendingCtxTokens += this.newCtxTokens;
              this.newCtxTokens = 0;
            }
            const guardEvaluation = this.evaluateContextGuard(this.schemaCtxTokens);
            if (guardEvaluation.blocked.length > 0) {
              this.enforceContextFinalTurn(guardEvaluation.blocked, 'turn_preflight');
              this.schemaCtxTokens = this.computeForcedFinalSchemaTokens(provider);
              const postEnforceGuard = this.evaluateContextGuard(this.schemaCtxTokens);
              if (postEnforceGuard.blocked.length > 0) {
                const [firstBlocked] = postEnforceGuard.blocked;
                const warnEntry: LogEntry = {
                  timestamp: Date.now(),
                  severity: 'WRN',
                  turn: currentTurn,
                  subturn: 0,
                  direction: 'response',
                  type: 'llm',
                  remoteIdentifier: AIAgentSession.REMOTE_CONTEXT,
                  fatal: false,
                  message: AIAgentSession.CONTEXT_POST_SHRINK_TURN_WARN,
                  details: {
                    projected_tokens: postEnforceGuard.projectedTokens,
                    limit_tokens: firstBlocked.limit,
                    provider: firstBlocked.provider,
                    model: firstBlocked.model,
                  },
                };
                this.log(warnEntry);
              }
            }

            const turnResult = await this.executeSingleTurn(
              attemptConversation,
              provider,
              model,
              isFinalTurn, // isFinalTurn
              currentTurn,
              logs,
              accounting,
              lastShownThinkingHeaderTurn,
              attempts,
              maxRetries,
              sessionReasoningLevel,
              sessionReasoningValue ?? undefined
            );
            const { messages: sanitizedMessages, dropped: droppedInvalidToolCalls } = this.sanitizeTurnMessages(
              turnResult.messages,
              { turn: currentTurn, provider, model }
            );

            sanitizedMessages.forEach((message) => {
              if (message.role !== 'tool') return;
              const callId = (message as { toolCallId?: string }).toolCallId;
              if (typeof callId !== 'string' || callId.length === 0) return;
              const override = this.toolFailureMessages.get(callId);
              if (override === undefined) return;
              message.content = override;
              this.toolFailureMessages.delete(callId);
            });

            if (this.toolFailureFallbacks.length > 0) {
              sanitizedMessages.forEach((message) => {
                if (this.toolFailureFallbacks.length === 0) return;
                if (message.role !== 'tool') return;
                const content = typeof message.content === 'string' ? message.content.trim() : '';
                if (content.length > 0 && content !== AIAgentSession.TOOL_NO_OUTPUT) return;
                const fallback = this.toolFailureFallbacks.shift();
                if (fallback === undefined) return;
                message.content = fallback;
              });
              this.toolFailureFallbacks.length = 0;
            }

            let reasoningHeaderEmitted = false;
            if (!turnResult.shownThinking) {
              const reasoningChunks: string[] = [];
              sanitizedMessages.forEach((message) => {
                if (message.role !== 'assistant') {
                  return;
                }
                const segments = Array.isArray(message.reasoning) ? message.reasoning : [];
                segments.forEach((segment) => {
                  const text = (segment as { text?: unknown }).text;
                  if (typeof text === 'string' && text.trim().length > 0) {
                    reasoningChunks.push(text);
                  }
                });
              });
              if (reasoningChunks.length > 0) {
                reasoningHeaderEmitted = true;
                if (lastShownThinkingHeaderTurn !== currentTurn) {
                  const thinkingHeader: LogEntry = {
                    timestamp: Date.now(),
                    severity: 'THK',
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response',
                    type: 'llm',
                    remoteIdentifier: 'thinking',
                    fatal: false,
                    message: 'reasoning output stream',
                  };
                  this.log(thinkingHeader);
                }
                if (this.parentTxnId === undefined) {
                  this.emitReasoningChunks(reasoningChunks);
                }
              }
            }

            // Update tracking if thinking was shown
            if (turnResult.shownThinking || reasoningHeaderEmitted) {
              lastShownThinkingHeaderTurn = currentTurn;
            }

            // Emit WRN for unknown tool calls that the AI SDK could not execute (name not in ToolSet)
            try {
              // Consider only internal tool set as always known
              // Internal tools always available; include optional batch tool if enabled for this session
              const internal = new Set<string>([AIAgentSession.FINAL_REPORT_TOOL]);
              if (this.progressToolEnabled) internal.add('agent__progress_report');
              if (this.sessionConfig.tools.includes('batch')) internal.add('agent__batch');
              const normalizeTool = (n: string) => n.replace(/^<\|[^|]+\|>/, '').trim();
              const assistantMessages = sanitizedMessages.filter((m) => m.role === 'assistant');
              const assistantMsg = assistantMessages.length > 0 ? assistantMessages[assistantMessages.length - 1] : undefined;
              if (assistantMsg?.toolCalls !== undefined && assistantMsg.toolCalls.length > 0) {
                (assistantMsg.toolCalls).forEach((tc) => {
                  const n = normalizeTool(tc.name);
                  const known = (this.toolsOrchestrator?.hasTool(n) ?? false) || internal.has(n);
                  if (!known) {
                    const req = formatToolRequestCompact(n, tc.parameters);
                    const warn: LogEntry = {
                      timestamp: Date.now(),
                      severity: 'WRN',
                      turn: currentTurn,
                      subturn: 0,
                      direction: 'response',
                      type: 'llm',
                      remoteIdentifier: 'assistant:tool',
                      fatal: false,
                      message: `Unknown tool requested (not executed): ${req}`
                    };
                    this.log(warn);
                  }
                });
              }
            } catch (e) { warn(`unknown tool warning failed: ${e instanceof Error ? e.message : String(e)}`); }

            // Record accounting for every attempt (include failed attempts with zeroed tokens if absent)
            {
              const tokens = turnResult.tokens ?? { inputTokens: 0, outputTokens: 0, cachedTokens: 0, totalTokens: 0 };
              // Normalize totalTokens to include cache read/write for accounting consistency
              try {
                const r = tokens.cacheReadInputTokens ?? tokens.cachedTokens ?? 0;
                const w = tokens.cacheWriteInputTokens ?? 0;
                const totalWithCache = tokens.inputTokens + tokens.outputTokens + r + w;
                if (Number.isFinite(totalWithCache)) tokens.totalTokens = totalWithCache;
              } catch { /* keep provider totalTokens if something goes wrong */ }
              // Compute cost from pricing table when available (prefer provider-reported when using routers like OpenRouter)
              const metadata = turnResult.providerMetadata;
              const computeCost = (): { costUsd?: number } => {
                try {
                  const pricing = (this.sessionConfig.config.pricing ?? {}) as Partial<Record<string, Partial<Record<string, { unit?: 'per_1k'|'per_1m'; currency?: 'USD'; prompt?: number; completion?: number; cacheRead?: number; cacheWrite?: number }>>>>;
                  const effectiveProvider = metadata?.actualProvider ?? provider;
                  const effectiveModel = metadata?.actualModel ?? model;
                  const provTable = pricing[effectiveProvider];
                  const modelTable = provTable !== undefined ? provTable[effectiveModel] : undefined;
                  if (modelTable === undefined) return {};
                  const denom = modelTable.unit === 'per_1k' ? 1000 : 1_000_000;
                  const pIn = modelTable.prompt ?? 0;
                  const pOut = modelTable.completion ?? 0;
                  const pRead = modelTable.cacheRead ?? 0;
                  const pWrite = modelTable.cacheWrite ?? 0;
                  const r = tokens.cacheReadInputTokens ?? tokens.cachedTokens ?? 0;
                  const w = tokens.cacheWriteInputTokens ?? 0;
                  const cost = (pIn * tokens.inputTokens + pOut * tokens.outputTokens + pRead * r + pWrite * w) / denom;
                  return { costUsd: Number.isFinite(cost) ? cost : undefined };
                } catch { return {}; }
              };
              const computed = computeCost();
              const accountingEntry: AccountingEntry = {
                type: 'llm',
                timestamp: Date.now(),
                status: turnResult.status.type === 'success' ? 'ok' : 'failed',
                latency: turnResult.latencyMs,
                provider,
                model,
                actualProvider: metadata?.actualProvider,
                actualModel: metadata?.actualModel,
                costUsd: metadata?.reportedCostUsd ?? computed.costUsd,
                upstreamInferenceCostUsd: metadata?.upstreamCostUsd,
                stopReason: turnResult.stopReason,
                tokens,
                error: turnResult.status.type !== 'success' ?
                  ('message' in turnResult.status ? turnResult.status.message : turnResult.status.type) : undefined,
                agentId: this.sessionConfig.agentId,
                callPath: this.callPath,
                txnId: this.txnId,
                parentTxnId: this.parentTxnId,
                originTxnId: this.originTxnId
              };
              accounting.push(accountingEntry);
              try { if (llmOpId !== undefined) this.opTree.appendAccounting(llmOpId, accountingEntry); } catch (e) { warn(`appendAccounting failed: ${e instanceof Error ? e.message : String(e)}`); }
              try { this.sessionConfig.callbacks?.onAccounting?.(accountingEntry); } catch (e) { warn(`onAccounting callback failed: ${e instanceof Error ? e.message : String(e)}`); }
            }
            // Close hierarchical LLM op and attach response summary
            try {
              if (llmOpId !== undefined) {
                const lastAssistant = [...sanitizedMessages].filter((m) => m.role === 'assistant').pop();
                const respText = typeof lastAssistant?.content === 'string' ? lastAssistant.content : (turnResult.response ?? '');
                const sz = respText.length > 0 ? Buffer.byteLength(respText, 'utf8') : 0;
                this.opTree.setResponse(llmOpId, { payload: { textPreview: respText.slice(0, 4096) }, size: sz, truncated: respText.length > 4096 });
                this.opTree.endOp(llmOpId, (turnResult.status.type === 'success') ? 'ok' : 'failed', { latency: turnResult.latencyMs });
                this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
              }
            } catch (e) { warn(`finalize LLM op failed: ${e instanceof Error ? e.message : String(e)}`); }
            this.currentLlmOpId = undefined;

      // Handle turn result based on status
            if (turnResult.status.type === 'success') {
              const assistantMessagesSanitized = sanitizedMessages.filter((m) => m.role === 'assistant');
              const lastAssistantSanitized = assistantMessagesSanitized.length > 0 ? assistantMessagesSanitized[assistantMessagesSanitized.length - 1] : undefined;
              const assistantForAdoption = lastAssistantSanitized;
              if (assistantForAdoption !== undefined && Array.isArray(assistantForAdoption.toolCalls)) {
                const expandedToolCalls = this.expandNestedLLMToolCalls(assistantForAdoption.toolCalls);
                if (expandedToolCalls !== undefined) {
                  assistantForAdoption.toolCalls = expandedToolCalls;
                }
              }
              let sanitizedHasToolCalls = assistantForAdoption !== undefined && Array.isArray(assistantForAdoption.toolCalls) && assistantForAdoption.toolCalls.length > 0;
              const textContent = assistantForAdoption !== undefined && typeof assistantForAdoption.content === 'string' ? assistantForAdoption.content : undefined;
              const sanitizedHasText = textContent !== undefined && textContent.trim().length > 0;

              const toolFailureDetected = sanitizedMessages.some((msg) => msg.role === 'tool' && typeof msg.content === 'string' && msg.content.trim().toLowerCase().startsWith('(tool failed:'));
              let failureToolNameNormalized: string | undefined;
              if (toolFailureDetected) {
                const failureToolMessage = sanitizedMessages
                  .filter((msg) => msg.role === 'tool' && typeof msg.content === 'string')
                  .find((msg) => {
                    const trimmed = msg.content.trim().toLowerCase();
                    return trimmed.startsWith('(tool failed:');
                  });
                if (failureToolMessage !== undefined) {
                  const failureCallId = (failureToolMessage as { toolCallId?: string }).toolCallId;
                  if (typeof failureCallId === 'string') {
                    const override = this.toolFailureMessages.get(failureCallId);
                    if (typeof override === 'string' && override.length > 0) {
                      failureToolMessage.content = override;
                    }
                    const owningAssistant = sanitizedMessages.find(
                      (msg): msg is ConversationMessage & { toolCalls: ToolCall[] } =>
                        msg.role === 'assistant' && Array.isArray((msg as { toolCalls?: unknown }).toolCalls)
                    );
                    if (owningAssistant !== undefined) {
                      const relatedCall = owningAssistant.toolCalls.find((tc) => tc.id === failureCallId);
                      if (relatedCall !== undefined) {
                        failureToolNameNormalized = sanitizeToolName(relatedCall.name);
                        if (failureToolNameNormalized === AIAgentSession.FINAL_REPORT_TOOL) {
                          finalReportToolFailed = true;
                        }
                      }
                    }
                    this.toolFailureMessages.delete(failureCallId);
                  }
                }
              }

               
              if (this.finalReport === undefined && !sanitizedHasToolCalls && sanitizedHasText) {
                if (this.tryAdoptFinalReportFromText(assistantForAdoption, textContent)) {
                  sanitizedHasToolCalls = true;
                }
              }

              if (turnResult.status.finalAnswer && this.finalReport === undefined) {
        const adoptFromToolCall = () => {
          if (toolFailureDetected) return false;
          const assistantWithCall = sanitizedMessages.find((msg): msg is ConversationMessage & { toolCalls: ToolCall[] } =>
            msg.role === 'assistant' && Array.isArray((msg as { toolCalls?: unknown }).toolCalls) && (msg as { toolCalls?: unknown }).toolCalls !== undefined
          );
          if (assistantWithCall === undefined) return false;
          const finalReportCall = assistantWithCall.toolCalls.find((call) => sanitizeToolName(call.name) === AIAgentSession.FINAL_REPORT_TOOL);
          if (finalReportCall === undefined) return false;
          const params = finalReportCall.parameters;
          const statusValue = (params as { status?: unknown }).status;
          const status = typeof statusValue === 'string' && statusValue.trim().length > 0 ? statusValue.trim() : undefined;
          const formatParamRaw = (params as { report_format?: unknown }).report_format;
          const formatParam = typeof formatParamRaw === 'string' && formatParamRaw.trim().length > 0 ? formatParamRaw.trim() : undefined;
          const contentValue = (params as { report_content?: unknown }).report_content;
          const contentParam = typeof contentValue === 'string' && contentValue.trim().length > 0 ? contentValue.trim() : undefined;
          if (status === undefined || contentParam === undefined) return false;
          const metadataCandidate = (params as { metadata?: unknown }).metadata;
          const metadata = (metadataCandidate !== null && typeof metadataCandidate === 'object' && !Array.isArray(metadataCandidate)) ? metadataCandidate as Record<string, unknown> : undefined;
          const contentJsonCandidate = (params as { content_json?: unknown }).content_json;
          const contentJson = (contentJsonCandidate !== null && typeof contentJsonCandidate === 'object' && !Array.isArray(contentJsonCandidate)) ? contentJsonCandidate as Record<string, unknown> : undefined;
          const normalizedStatus = status === 'success' || status === 'failure' || status === 'partial' ? status : undefined;
          if (normalizedStatus === undefined) return false;
          const formatCandidate = formatParam ?? this.resolvedFormat ?? 'text';
          const finalFormat = FINAL_REPORT_FORMAT_VALUES.find((value) => value === formatCandidate) ?? 'text';
          this.finalReport = {
            status: normalizedStatus,
            format: finalFormat,
            content: contentParam,
            content_json: contentJson,
            metadata,
            ts: Date.now(),
          };
          sanitizedHasToolCalls = true;
          return true;
        };
        const adoptFromToolMessage = () => {
          if (toolFailureDetected) return false;
          const toolMessage = sanitizedMessages.find((msg) => msg.role === 'tool' && typeof msg.content === 'string' && msg.content.trim().length > 0);
          if (toolMessage === undefined || typeof toolMessage.content !== 'string') return false;
          const normalizedContent = toolMessage.content.trim();
          if (normalizedContent.length === 0) return false;
          const finalFormat = this.resolvedFormat ?? 'text';
          this.finalReport = {
            status: 'success',
            format: (FINAL_REPORT_FORMAT_VALUES.find((value) => value === finalFormat) ?? 'text'),
            content: normalizedContent,
            ts: Date.now(),
          };
          sanitizedHasToolCalls = true;
          return true;
        };
        if (!adoptFromToolCall()) {
          if (!toolFailureDetected) {
            adoptFromToolMessage();
          }
        }
      }


      if (droppedInvalidToolCalls > 0 && !sanitizedHasToolCalls) {
                const warnEntry: LogEntry = {
                  timestamp: Date.now(),
                  severity: 'WRN',
                  turn: currentTurn,
                  subturn: 0,
                  direction: 'response',
                  type: 'llm',
                  remoteIdentifier: `${provider}:${model}`,
                  fatal: false,
                  message: 'Invalid tool call dropped due to malformed payload. Requesting retry without updating conversation history.',
                };
                this.log(warnEntry);
                this.pushSystemRetryMessage(conversation, `System notice: the previous response had invalid tool-call arguments. Provide a JSON object when invoking tools and retry the required call.`);
                lastError = 'invalid_response: malformed_tool_call';
                if (cycleComplete) {
                  rateLimitedInCycle = 0;
                  maxRateLimitWaitMs = 0;
                }
                continue;
              }
              // Synthetic error: success with content but no tools and no final_report → retry this turn
              if (this.finalReport === undefined) {
                if (!turnResult.status.finalAnswer && !sanitizedHasToolCalls && sanitizedHasText) {
                const warnEntry: LogEntry = {
                  timestamp: Date.now(),
                  severity: 'WRN',
                  turn: currentTurn,
                  subturn: 0,
                  direction: 'response',
                  type: 'llm',
                  remoteIdentifier: `${provider}:${model}`,
                  fatal: false,
                  message: 'Synthetic retry: assistant returned content without tool calls and without final_report.'
                };
                this.log(warnEntry);
                if (isFinalTurn && !finalTurnRetryWarnLogged) {
                  const agentWarn: LogEntry = {
                    timestamp: Date.now(),
                    severity: 'WRN',
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response',
                    type: 'llm',
                    remoteIdentifier: AIAgentSession.REMOTE_FINAL_TURN,
                    fatal: false,
                    message: finalTurnRetryMessage
                  };
                  this.log(agentWarn);
                  finalTurnRetryWarnLogged = true;
                }
                lastError = 'invalid_response: content_without_tools_or_final';
                if (cycleComplete) {
                  rateLimitedInCycle = 0;
                  maxRateLimitWaitMs = 0;
                }
                this.pushSystemRetryMessage(conversation, this.buildFinalReportReminder());
                this.pushSystemRetryMessage(conversation, 'System notice: plain text responses are ignored. Use the tool with JSON arguments to deliver the final report.');
                // do not append these messages; try next provider/model in same turn
                continue;
              }
            }

        // (removed misplaced orchestrator path; correct handling is below in toolExecutor)

        // Add new messages to conversation
        conversation.push(...sanitizedMessages);
        // Deterministic finalization: if final_report has been set, finish now
        if (this.finalReport !== undefined) {
          const fr = this.finalReport;
          // Validate final JSON against schema if provided (no output to onOutput here)
          if (fr.format === 'json') {
            // Validate JSON against frontmatter schema if available
            const schema = this.sessionConfig.expectedOutput?.schema;
            if (schema !== undefined && fr.content_json !== undefined) {
              try {
                this.ajv = this.ajv ?? new AjvCtor({ allErrors: true, strict: false });
                const validate = this.ajv.compile(schema);
                const valid = validate(fr.content_json);
                if (!valid) {
                  const ajvErrors = Array.isArray(validate.errors) ? (validate.errors as AjvErrorObject[]) : [];
                  const errs = ajvErrors.map((error) => {
                    const path = typeof error.instancePath === 'string' && error.instancePath.length > 0
                      ? error.instancePath
                      : (typeof error.schemaPath === 'string' ? error.schemaPath : '');
                    const msg = typeof error.message === 'string' ? error.message : '';
                    return `${path} ${msg}`.trim();
                  }).join('; ');
                  const payloadPreview = (() => {
                    try {
                      const raw = JSON.stringify(fr.content_json);
                      return typeof raw === 'string'
                        ? (raw.length > 200 ? `${raw.slice(0, 200)}…` : raw)
                        : undefined;
                    } catch {
                      return undefined;
                    }
                  })();
                  const warn: LogEntry = {
                    timestamp: Date.now(),
                    severity: 'WRN',
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response',
                    type: 'llm',
                    remoteIdentifier: 'agent:ajv',
                    fatal: false,
                    message: `final_report JSON does not match schema: ${errs}${payloadPreview !== undefined ? `; payload preview=${payloadPreview}` : ''}`
                  };
                  this.log(warn);
                }
              } catch (e) {
                const warn: LogEntry = {
                  timestamp: Date.now(),
                  severity: 'WRN',
                  turn: currentTurn,
                  subturn: 0,
                  direction: 'response',
                  type: 'llm',
                  remoteIdentifier: 'agent:ajv',
                  fatal: false,
                  message: `AJV validation failed: ${e instanceof Error ? e.message : String(e)}`
                };
                this.log(warn);
              }
            }
          } else {
            // For markdown/text, no onOutput; CLI will print via formatter
          }

          const finalOutput = (() => {
            if (fr.format === 'json' && fr.content_json !== undefined) {
              try { return JSON.stringify(fr.content_json); } catch { return undefined; }
            }
            if (typeof fr.content === 'string' && fr.content.length > 0) return fr.content;
            return undefined;
          })();
          if (this.sessionConfig.renderTarget !== 'sub-agent' && finalOutput !== undefined && this.sessionConfig.callbacks?.onOutput !== undefined) {
            this.sessionConfig.callbacks.onOutput(finalOutput);
            if (!finalOutput.endsWith('\n')) {
              this.sessionConfig.callbacks.onOutput('\n');
            }
          }

          // Log successful exit
          const exitCode = this.forcedFinalTurnReason === 'context' ? 'EXIT-TOKEN-LIMIT' : 'EXIT-FINAL-ANSWER';
          const exitMessage = this.forcedFinalTurnReason === 'context'
            ? `Final report received after context guard triggered (${AIAgentSession.FINAL_REPORT_TOOL}), session complete`
            : `Final report received (${AIAgentSession.FINAL_REPORT_TOOL}), session complete`;
          this.logExit(exitCode, exitMessage, currentTurn);

          // Emit FIN summary log entry
          this.emitFinalSummary(logs, accounting);

          return {
            success: true,
            conversation,
            logs,
            accounting,
            finalReport: fr,
            childConversations: this.childConversations
          };
        }
              
              const hasToolResults = sanitizedMessages.some((m: ConversationMessage) => m.role === 'tool');
              const hasContent = turnResult.hasContent ?? (() => {
                const lastAssistant = [...sanitizedMessages].filter((m) => m.role === 'assistant').pop();
                return (lastAssistant?.content.trim().length ?? 0) > 0;
              })();
              const hasReasoning = turnResult.hasReasoning ?? false;
              const tokens = turnResult.tokens;
              const inputTokens = tokens?.inputTokens ?? 0;
              const outputTokens = tokens?.outputTokens ?? 0;
              const cacheRead = tokens?.cacheReadInputTokens
                ?? turnResult.providerMetadata?.cacheReadInputTokens
                ?? tokens?.cachedTokens
                ?? 0;
              const cacheWrite = tokens?.cacheWriteInputTokens
                ?? turnResult.providerMetadata?.cacheWriteInputTokens
                ?? 0;
              const cachedTokens = tokens?.cachedTokens ?? cacheRead;
              const latencyMs = turnResult.latencyMs;
              const costInfo = this.llmClient.getLastCostInfo();
              const reportedCost = turnResult.providerMetadata?.reportedCostUsd;
              const effectiveCost = turnResult.providerMetadata?.effectiveCostUsd ?? reportedCost ?? costInfo?.costUsd;
              const upstreamCost = turnResult.providerMetadata?.upstreamCostUsd ?? costInfo?.upstreamInferenceCostUsd;
              const responseBytes = (() => {
                if (typeof turnResult.responseBytes === 'number') return turnResult.responseBytes;
                if (typeof turnResult.response === 'string') return Buffer.byteLength(turnResult.response, 'utf8');
                return 0;
              })();
              const reasoningStatus = turnResult.providerMetadata?.reasoningState
                ?? (turnResult.hasReasoning === true ? 'present' : 'unset');
              const metricsParts: string[] = [
                `input ${String(inputTokens)}`,
                `output ${String(outputTokens)}`,
                `cacheR ${String(cacheRead)}`,
                `cacheW ${String(cacheWrite)}`,
                `cached ${String(cachedTokens)}`,
                `${String(latencyMs)}ms`,
              ];
              const totalCtx = inputTokens + outputTokens + cacheRead;
              this.currentCtxTokens = totalCtx;
              this.pendingCtxTokens = 0;
              this.schemaCtxTokens = 0;
              if (totalCtx > 0) {
                metricsParts.push(`ctx ${String(totalCtx)}`);
              }
              if (typeof turnResult.stopReason === 'string' && turnResult.stopReason.length > 0) {
                metricsParts.push(`stop=${turnResult.stopReason}`);
              }
              if (typeof effectiveCost === 'number') {
                metricsParts.push(`cost $${effectiveCost.toFixed(6)}`);
              }
              if (typeof upstreamCost === 'number' && upstreamCost > 0) {
                metricsParts.push(`upstream $${upstreamCost.toFixed(6)}`);
              }
              metricsParts.push(`${String(responseBytes)} bytes`);
              metricsParts.push(`reasoning=${reasoningStatus}`);

              const turnSummary = `hasToolCalls=${String(sanitizedHasToolCalls)}, hasToolResults=${String(hasToolResults)}, finalAnswer=${String(turnResult.status.finalAnswer)}, hasReasoning=${String(hasReasoning)}, hasContent=${String(hasContent)}, response length=${String(turnResult.response?.length ?? 0)}, messages=${String(sanitizedMessages.length)}, stopReason=${turnResult.stopReason ?? 'none'}`;
              const remoteId = this.composeRemoteIdentifier(provider, model, turnResult.providerMetadata);
              const combinedMessage = `Turn result: ${turnSummary}`;
              const combinedDetails: Record<string, LogDetailValue> = {
                input_tokens: inputTokens,
                output_tokens: outputTokens,
                cache_read_tokens: cacheRead,
                cache_write_tokens: cacheWrite,
                cached_tokens: cachedTokens,
                latency_ms: latencyMs,
                response_bytes: responseBytes,
                reasoning: reasoningStatus,
                ctx_tokens: totalCtx,
                has_tool_calls: sanitizedHasToolCalls,
                has_tool_results: hasToolResults,
                final_answer: turnResult.status.finalAnswer,
                has_reasoning: hasReasoning,
                has_content: hasContent,
                response_length: turnResult.response?.length ?? 0,
                messages: sanitizedMessages.length,
              };
              if (turnResult.stopReason !== undefined) {
                combinedDetails.stop_reason = turnResult.stopReason;
              }
              if (typeof effectiveCost === 'number') {
                combinedDetails.cost_usd = effectiveCost;
              }
              if (typeof upstreamCost === 'number' && upstreamCost > 0) {
                combinedDetails.upstream_cost_usd = upstreamCost;
              }
              const combinedEntry: LogEntry = {
                timestamp: Date.now(),
                severity: 'VRB',
                turn: currentTurn,
                subturn: 0,
                direction: 'response',
                type: 'llm',
                remoteIdentifier: remoteId,
                fatal: false,
                message: combinedMessage,
                details: combinedDetails,
              };
              this.log(combinedEntry);

              // Additional verbose diagnostics (only when explicitly requested)
              if (this.sessionConfig.verbose === true && process.env.DEBUG === 'true') {
                // eslint-disable-next-line functional/no-loop-statements
                for (const msg of sanitizedMessages) {
                  warn(`[DEBUG] Message role=${msg.role}, content length=${String(msg.content.length)}, hasToolCalls=${String(msg.toolCalls !== undefined)}`);
                  if (msg.content.length > 0) {
                    warn(`[DEBUG] Content: ${msg.content}`);
                  }
                }
              }
              // Capture planned subturns for this turn (enriches subsequent logs)
              try {
                const lastAssistant = [...sanitizedMessages].filter((m: ConversationMessage) => m.role === 'assistant').pop();
                if (lastAssistant !== undefined && Array.isArray(lastAssistant.toolCalls)) {
                  const toolCalls = lastAssistant.toolCalls;
                  // Count only MCP tools (exclude internal agent__* and sub-agent tools)
                  const count = toolCalls.reduce((acc, tc) => {
                    const name = (typeof tc.name === 'string' ? tc.name : '').trim();
                    if (name.length === 0) return acc;
                    if (name === 'agent__progress_report' || name === AIAgentSession.FINAL_REPORT_TOOL || name === 'agent__batch') return acc;
                    // Count only known tools (non-internal) present in orchestrator
                    const isKnown = this.toolsOrchestrator?.hasTool(name) ?? false;
                    return isKnown ? acc + 1 : acc;
                  }, 0);
                  this.plannedSubturns.set(currentTurn, count);
                }
              } catch (e) { warn(`LLM accounting callback failed: ${e instanceof Error ? e.message : String(e)}`); }

              // Output response text if we have any (even with tool calls)
              // Only in non-streaming mode (streaming already called onOutput per chunk)
              if (this.sessionConfig.stream !== true && turnResult.response !== undefined && turnResult.response.length > 0) {
                this.sessionConfig.callbacks?.onThinking?.(turnResult.response);
                if (!turnResult.response.endsWith('\n')) {
                  this.sessionConfig.callbacks?.onThinking?.('\n');
                }
              }

              const retryFlags = turnResult as { incompleteFinalReportDetected?: boolean; finalReportAttempted?: boolean };
              if ((retryFlags.incompleteFinalReportDetected === true || retryFlags.finalReportAttempted === true) && currentTurn < maxTurns) {
                const previousMaxTurns = maxTurns;
                maxTurns = currentTurn + 1;
                const adjustLog: LogEntry = {
                  timestamp: Date.now(),
                  severity: 'WRN',
                  turn: currentTurn,
                  subturn: 0,
                  direction: 'response',
                  type: 'llm',
                  remoteIdentifier: 'agent:orchestrator',
                  fatal: false,
                  message: `Final report retry detected at turn ${String(currentTurn)}, adjusting max_turns from ${String(previousMaxTurns)} to ${String(maxTurns)}`
                };
                this.log(adjustLog);
              }

              // Check for reasoning-only responses (empty response but not final answer)
              if (!turnResult.status.finalAnswer && !turnResult.status.hasToolCalls &&
                  (turnResult.response === undefined || turnResult.response.trim().length === 0)) {
                if (turnResult.hasReasoning === true) {
                  // Reasoning-only response: allow conversation to continue without penalising the provider.
                  // No retry metadata or synthetic failure counters are updated; the reasoning will be appended
                  // to the conversation below so the model can build upon it in the next turn.
                } else {
                  // Log warning and retry this turn on another provider/model
                  const metadata = turnResult.providerMetadata;
                  const remoteId = this.composeRemoteIdentifier(provider, model, metadata);
                  const warnEntry: LogEntry = {
                    timestamp: Date.now(),
                    severity: 'WRN',
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response',
                    type: 'llm',
                    remoteIdentifier: remoteId,
                    fatal: false,
                    message: 'Empty response without tools; retrying with next provider/model in this turn.'
                  };
                  this.log(warnEntry);
                  this.llmSyntheticFailures++;
                  lastError = 'invalid_response: empty_without_tools';
                  if (cycleComplete) {
                    rateLimitedInCycle = 0;
                    maxRateLimitWaitMs = 0;
                  }
                  this.pushSystemRetryMessage(conversation, `System notice: the previous response was empty and no tools were invoked. Provide a tool call such as ${AIAgentSession.FINAL_REPORT_TOOL} with valid arguments.`);
                  // do not mark turnSuccessful; continue retry loop
                  continue;
                }
              }
              
              if (turnResult.status.finalAnswer) {
                const statusCandidate = this.getFinalReportStatus();
                const finalReportStatus: 'success' | 'failure' | 'partial' | 'missing' =
                  statusCandidate === 'success' || statusCandidate === 'failure' || statusCandidate === 'partial'
                    ? statusCandidate
                    : 'missing';
                const finalReportSucceeded = finalReportStatus === 'success';

                if (finalReportSucceeded) {
                  // Final report succeeded, we're done
                  turnSuccessful = true;
                  break;
                }

                const warnEntry: LogEntry = {
                  timestamp: Date.now(),
                  severity: 'WRN',
                  turn: currentTurn,
                  subturn: 0,
                  direction: 'response',
                  type: 'llm',
                  remoteIdentifier: 'agent:final-report',
                  fatal: false,
                  message: `Final report returned status='${finalReportStatus}', retrying.`,
                };
                this.log(warnEntry);
                this.llmSyntheticFailures++;
                lastError = `final_report_status_${finalReportStatus}`;
                if (cycleComplete) {
                  rateLimitedInCycle = 0;
                  maxRateLimitWaitMs = 0;
                }
                this.finalReport = undefined;
                this.pushSystemRetryMessage(conversation, this.buildFinalReportReminder());
                continue;
              } else {
                // If this is the final turn and we still don't have a final answer,
                // retry within this turn across provider/model pairs up to maxRetries.
                if (isFinalTurn) {
                  const metadata = turnResult.providerMetadata;
                  const remoteId = this.composeRemoteIdentifier(provider, model, metadata);
                  const warnEntry: LogEntry = {
                    timestamp: Date.now(),
                    severity: 'WRN',
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response',
                    type: 'llm',
                    remoteIdentifier: remoteId,
                    fatal: false,
                    message: finalTurnRetryMessage
                  };
                  this.log(warnEntry);
                  if (!finalTurnRetryWarnLogged) {
                    const agentWarn: LogEntry = {
                      timestamp: Date.now(),
                      severity: 'WRN',
                      turn: currentTurn,
                      subturn: 0,
                      direction: 'response',
                      type: 'llm',
                      remoteIdentifier: AIAgentSession.REMOTE_FINAL_TURN,
                      fatal: false,
                      message: finalTurnRetryMessage
                    };
                    this.log(agentWarn);
                    finalTurnRetryWarnLogged = true;
                  }
                  // Continue attempts loop (do not mark successful)
                  if (cycleComplete) {
                    rateLimitedInCycle = 0;
                    maxRateLimitWaitMs = 0;
                  }
                  this.pushSystemRetryMessage(conversation, 'System notice: this is the final turn. Provide the final report now.');
                  this.pushSystemRetryMessage(conversation, this.buildFinalReportReminder());
                  continue;
              }
              // Non-final turns: proceed to next turn; tools already executed if present
              turnSuccessful = true;
              break;
            }
          } else {
            // Handle various error types
            lastError = 'message' in turnResult.status ? turnResult.status.message : turnResult.status.type;

            const remoteId = this.composeRemoteIdentifier(provider, model, turnResult.providerMetadata);
            const retryDirective = turnResult.retry ?? this.buildFallbackRetryDirective({
              status: turnResult.status,
              remoteId,
              attempt: attempts,
            });

            if (turnResult.status.type === 'auth_error' || turnResult.status.type === 'quota_exceeded') {
              const directive = retryDirective ?? this.buildFallbackRetryDirective({
                status: turnResult.status,
                remoteId,
                attempt: attempts,
              });
              if (directive?.logMessage !== undefined) {
                const warnEntry: LogEntry = {
                  timestamp: Date.now(),
                  severity: 'WRN',
                  turn: currentTurn,
                  subturn: 0,
                  direction: 'response',
                  type: 'llm',
                  remoteIdentifier: remoteId,
                  fatal: false,
                  message: directive.logMessage,
                };
                this.log(warnEntry);
              }
              if (directive?.systemMessage !== undefined) {
                this.pushSystemRetryMessage(conversation, directive.systemMessage);
              }
              break;
            }

            if (turnResult.status.type === 'rate_limit') {
              const directive = retryDirective ?? this.buildFallbackRetryDirective({
                status: turnResult.status,
                remoteId,
                attempt: attempts,
              });
              const action = directive?.action ?? 'retry';
              if (action === AIAgentSession.RETRY_ACTION_SKIP_PROVIDER) {
                if (directive?.logMessage !== undefined) {
                  const warnEntry: LogEntry = {
                    timestamp: Date.now(),
                    severity: 'WRN',
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response',
                    type: 'llm',
                    remoteIdentifier: remoteId,
                    fatal: false,
                    message: directive.logMessage,
                  };
                  this.log(warnEntry);
                }
                if (directive?.systemMessage !== undefined) {
                  this.pushSystemRetryMessage(conversation, directive.systemMessage);
                }
                break;
              }

              rateLimitedInCycle += 1;
              const RATE_LIMIT_MIN_WAIT_MS = 1_000;
              const RATE_LIMIT_MAX_WAIT_MS = 60_000;
              const fallbackWait = Math.min(Math.max(attempts * 1_000, RATE_LIMIT_MIN_WAIT_MS), RATE_LIMIT_MAX_WAIT_MS);
              const recommendedWait = directive?.backoffMs;
              const waitCandidate = typeof recommendedWait === 'number' && Number.isFinite(recommendedWait)
                ? recommendedWait
                : fallbackWait;
              const effectiveWaitMs = Math.min(
                Math.max(Math.round(waitCandidate), RATE_LIMIT_MIN_WAIT_MS),
                RATE_LIMIT_MAX_WAIT_MS
              );
              maxRateLimitWaitMs = Math.max(maxRateLimitWaitMs, effectiveWaitMs);
              const sources = directive?.sources ?? turnResult.status.sources;
              const sourceDetails = Array.isArray(sources) && sources.length > 0
                ? ` Sources: ${sources.join(' | ')}`
                : '';
              const logMessage = directive?.logMessage
                ?? `Rate limited; suggested wait ${String(effectiveWaitMs)}ms before retry.${sourceDetails}`;
              const warnEntry: LogEntry = {
                timestamp: Date.now(),
                severity: 'WRN',
                turn: currentTurn,
                subturn: 0,
                direction: 'response',
                type: 'llm',
                remoteIdentifier: remoteId,
                fatal: false,
                message: logMessage,
              };
              this.log(warnEntry);
              const waitText = effectiveWaitMs > 0 ? `${String(effectiveWaitMs)}ms` : 'a short delay';
              const retryNotice = directive?.systemMessage
                ?? `System notice: upstream provider ${remoteId} rate-limited the previous request. Retrying after ${waitText}; no changes required unless rate limits persist.`;
              this.pushSystemRetryMessage(conversation, retryNotice);

              if (cycleComplete) {
                const allRateLimited = rateLimitedInCycle >= pairs.length;
                if (allRateLimited && attempts < maxRetries && maxRateLimitWaitMs > 0) {
                  const waitLog: LogEntry = {
                    timestamp: Date.now(),
                    severity: 'WRN',
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response',
                    type: 'llm',
                    remoteIdentifier: 'agent:retry',
                    fatal: false,
                    message: `All ${String(pairs.length)} providers rate-limited; backing off for ${String(maxRateLimitWaitMs)}ms before retry.`
                  };
                  this.log(waitLog);
                  const sleepResult = await this.sleepWithAbort(maxRateLimitWaitMs);
                  if (sleepResult === 'aborted_cancel') {
                    return this.finalizeCanceledSession(conversation, logs, accounting);
                  }
                  if (sleepResult === 'aborted_stop') {
                    return this.finalizeGracefulStopSession(conversation, logs, accounting);
                  }
                }
                rateLimitedInCycle = 0;
                maxRateLimitWaitMs = 0;
              }

              continue;
            }

            if (retryDirective !== undefined && retryDirective.action === AIAgentSession.RETRY_ACTION_SKIP_PROVIDER) {
              if (retryDirective.logMessage !== undefined) {
                const warnEntry: LogEntry = {
                  timestamp: Date.now(),
                  severity: 'WRN',
                  turn: currentTurn,
                  subturn: 0,
                  direction: 'response',
                  type: 'llm',
                  remoteIdentifier: remoteId,
                  fatal: false,
                  message: retryDirective.logMessage,
                };
                this.log(warnEntry);
              }
              if (retryDirective.systemMessage !== undefined) {
                this.pushSystemRetryMessage(conversation, retryDirective.systemMessage);
              }
              break;
            }

            if (retryDirective?.logMessage !== undefined) {
              const warnEntry: LogEntry = {
                timestamp: Date.now(),
                severity: 'WRN',
                turn: currentTurn,
                subturn: 0,
                direction: 'response',
                type: 'llm',
                remoteIdentifier: remoteId,
                fatal: false,
                message: retryDirective.logMessage,
              };
              this.log(warnEntry);
            }
            if (retryDirective?.systemMessage !== undefined) {
              this.pushSystemRetryMessage(conversation, retryDirective.systemMessage);
            }

            if (cycleComplete) {
              rateLimitedInCycle = 0;
              maxRateLimitWaitMs = 0;
            }
            const retryableStatus = turnResult.status;
            const rawDetail = 'message' in retryableStatus && typeof retryableStatus.message === 'string'
              ? retryableStatus.message.trim()
              : '';
            const detail = rawDetail.length > 0 ? rawDetail : undefined;
            const detailSuffixParen = detail !== undefined ? ` (${detail})` : '';
            const detailSuffixColon = detail !== undefined ? `: ${detail}` : '';
            const retryNotice = (() => {
              switch (retryableStatus.type) {
                case 'invalid_response':
                  return retryDirective?.systemMessage ?? `System notice: upstream rejected the previous response as invalid${detailSuffixParen}. Provide a valid JSON tool call and retry.`;
                case 'model_error':
                  return retryDirective?.systemMessage ?? `System notice: model error reported by ${remoteId}${detailSuffixColon}. Verify tool arguments before retrying.`;
                case 'network_error':
                  return retryDirective?.systemMessage ?? `System notice: network error from ${remoteId}${detailSuffixColon}. Retrying automatically; the model may proceed once connectivity stabilizes.`;
                case 'timeout':
                  return retryDirective?.systemMessage ?? `System notice: upstream timeout${detailSuffixParen}. Provide a shorter response or reduce tool usage before retrying.`;
                default:
                  return undefined;
              }
            })();
            if (retryNotice !== undefined) {
              this.pushSystemRetryMessage(conversation, retryNotice);
            }
            continue;
          }
        } catch (error: unknown) {
          const errorMessage: string = error instanceof Error ? error.message : String(error);
          lastError = errorMessage;
          // Ensure accounting entry even if an unexpected error occurred before turnResult
            const accountingEntry: AccountingEntry = {
              type: 'llm',
              timestamp: Date.now(),
              status: 'failed',
              latency: 0,
              provider,
              model,
              tokens: { inputTokens: 0, outputTokens: 0, cachedTokens: 0, totalTokens: 0 },
              error: lastError,
              agentId: this.sessionConfig.agentId, callPath: this.callPath, txnId: this.txnId, parentTxnId: this.parentTxnId, originTxnId: this.originTxnId
            };
            accounting.push(accountingEntry);
            try { if (typeof this.currentLlmOpId === 'string') this.opTree.appendAccounting(this.currentLlmOpId, accountingEntry); } catch (e) { warn(`appendAccounting failed: ${e instanceof Error ? e.message : String(e)}`); }
            try { this.sessionConfig.callbacks?.onAccounting?.(accountingEntry); } catch (e) { warn(`onAccounting callback failed: ${e instanceof Error ? e.message : String(e)}`); }
            if (cycleComplete) {
              rateLimitedInCycle = 0;
              maxRateLimitWaitMs = 0;
            }
            this.pushSystemRetryMessage(conversation, `System notice: an internal error prevented tool execution. Retry the required tool call with valid JSON arguments.`);
            continue;
        }
      }

      if (!turnSuccessful) {
        if (this.forcedFinalTurnReason === 'context') {
          const fallbackFormatCandidate = this.resolvedFormat ?? 'text';
          const fallbackFormat = FINAL_REPORT_FORMAT_VALUES.find((value) => value === fallbackFormatCandidate) ?? 'text';
          const detailMessage = 'Context window budget exhausted before another model call could be attempted. Providing a synthesized final report with known limitations.';
          const fallbackReport: NonNullable<AIAgentResult['finalReport']> = {
            status: 'failure',
            format: fallbackFormat,
            content: detailMessage,
            ts: Date.now(),
          };
          this.finalReport = fallbackReport;
          this.logExit('EXIT-TOKEN-LIMIT', 'Context window reached; synthesized final report without additional model turn', currentTurn);
          this.emitFinalSummary(logs, accounting);
          return {
            success: true,
            conversation,
            logs,
            accounting,
            finalReport: fallbackReport,
            childConversations: this.childConversations,
          };
        }
        if (currentTurn === maxTurns && this.finalReport === undefined && !finalReportToolFailed) {
          const attemptLabel = `${String(maxRetries)} attempt${maxRetries === 1 ? '' : 's'}`;
          const baseSummary = `The model did not produce a final report after ${attemptLabel} on the final turn.`;
          const detailedSummary = lastError !== undefined ? `${baseSummary} Last error: ${lastError}.` : baseSummary;
          const fallbackFormatCandidate = this.resolvedFormat ?? 'text';
          const fallbackFormat = FINAL_REPORT_FORMAT_VALUES.find((value) => value === fallbackFormatCandidate) ?? 'text';
          const metadata = lastError !== undefined ? { lastError } : undefined;
          const fallbackReport: NonNullable<AIAgentResult['finalReport']> = {
            status: 'failure',
            format: fallbackFormat,
            content: detailedSummary,
            ...(metadata !== undefined ? { metadata } : {}),
            ts: Date.now(),
          };
          this.finalReport = fallbackReport;
          this.logExit('EXIT-FINAL-ANSWER', 'Synthesized failure final_report after retries', currentTurn);
          this.emitFinalSummary(logs, accounting);
          return {
            success: true,
            conversation,
            logs,
            accounting,
            finalReport: fallbackReport,
            childConversations: this.childConversations,
          };
        }

        // All attempts failed for this turn
        const exitCode = (lastError?.includes('auth') === true) ? 'EXIT-AUTH-FAILURE' :
                        (lastError?.includes('quota') === true) ? 'EXIT-QUOTA-EXCEEDED' :
                        (lastError?.includes('timeout') === true) ? 'EXIT-INACTIVITY-TIMEOUT' :
                        'EXIT-NO-LLM-RESPONSE';
        
        const reason = `No LLM response after ${String(maxRetries)} retries across ${String(pairs.length)} provider/model pairs: ${lastError ?? 'All targets failed'}`;
        
        this.logExit(exitCode, reason, currentTurn);
        // Emit FIN summary even on failure
        this.emitFinalSummary(logs, accounting);
        return {
          success: false,
          error: lastError ?? 'All provider/model targets failed',
          conversation,
          logs,
          accounting,
          childConversations: this.childConversations
        };
      }
      try {
        // no-op; placeholder to pair beginTurn with an endTurn finally
      } finally {
        try {
          // Optionally capture assistant content for the turn
          const lastAssistant = (() => {
            try { return [...conversation].filter((m) => m.role === 'assistant').pop(); } catch { return undefined; }
          })();
          const assistantText = typeof lastAssistant?.content === 'string' ? lastAssistant.content : undefined;
          const attrs = (typeof assistantText === 'string' && assistantText.length > 0) ? { assistant: { content: assistantText } } : {};
          this.opTree.endTurn(currentTurn, attrs);
          this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
        } catch (e) { warn(`endTurn/onOpTree failed: ${e instanceof Error ? e.message : String(e)}`); }
      }
    }

    // Max turns exceeded
    this.logExit(
      'EXIT-MAX-TURNS-NO-RESPONSE',
      `Max turns (${String(maxTurns)}) reached without final response`,
      currentTurn
    );
    // Emit FIN summary even on failure
    this.emitFinalSummary(logs, accounting);
    
    return {
      success: false,
      error: 'Max tool turns exceeded',
      conversation,
      logs,
      accounting,
      childConversations: this.childConversations
    };
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
      this.log({ timestamp: Date.now(), severity: 'VRB', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'agent:fin', fatal: false, message: 'session finalized after uncaught error' }, { opId: finOp });
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
  
  private parseJsonRecord(raw: string): Record<string, unknown> | undefined {
    const trimmed = raw.trim();
    if (trimmed.length === 0) return undefined;
    const tryParse = (value: string): Record<string, unknown> | undefined => {
      try {
        const parsed: unknown = JSON.parse(value);
        return (parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed))
          ? (parsed as Record<string, unknown>)
          : undefined;
      } catch {
        return undefined;
      }
    };
    const direct = tryParse(trimmed);
    if (direct !== undefined) return direct;
    const repaired = trimmed.replace(/\\x([0-9a-fA-F]{2})/g, (_match, hex) => '\\u00' + String(hex).toUpperCase());
    if (repaired === trimmed) return undefined;
    return tryParse(repaired);
  }

  private extractJsonRecordFromText(text: string): Record<string, unknown> | undefined {
    const length = text.length;
    let index = 0;
    // eslint-disable-next-line functional/no-loop-statements
    while (index < length) {
      const startIdx = text.indexOf('{', index);
      if (startIdx === -1) break;
      let depth = 0;
      let inString = false;
      let escape = false;
      // eslint-disable-next-line functional/no-loop-statements
      for (let cursor = startIdx; cursor < length; cursor += 1) {
        const ch = text[cursor];
        if (inString) {
          if (escape) {
            escape = false;
          } else if (ch === '\\') {
            escape = true;
          } else if (ch === '"') {
            inString = false;
          }
          continue;
        }
        if (ch === '"') {
          inString = true;
          continue;
        }
        if (ch === '{') {
          depth += 1;
          continue;
        }
        if (ch === '}') {
          depth -= 1;
          if (depth === 0) {
            const candidate = text.slice(startIdx, cursor + 1);
            const parsed = this.parseJsonRecord(candidate);
            if (parsed !== undefined) {
              return parsed;
            }
            break;
          }
          continue;
        }
      }
      index = startIdx + 1;
    }
    return undefined;
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

  private buildFinalReportReminder(): string {
    const format = this.resolvedFormat ?? 'text';
    const formatDescription = this.resolvedFormatParameterDescription ?? 'the required format';
    const contentField = (() => {
      if (format === 'json') return 'content_json';
      if (format === 'slack-block-kit') return 'messages';
      return 'report_content';
    })();
    const contentGuidance = (() => {
      if (contentField === 'content_json') {
        return 'include a `content_json` object that matches the expected schema';
      }
      if (contentField === 'messages') {
        return 'include a `messages` array populated with the final Slack Block Kit blocks';
      }
      return 'include `report_content` containing the full final answer';
    })();
    return `System notice: call ${AIAgentSession.FINAL_REPORT_TOOL} with report_format="${format}" (${formatDescription}), set status to success, failure, or partial as appropriate, and ${contentGuidance}.`;
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

  private getContextTargets(): TargetContextConfig[] {
    if (this.targetContextConfigs.length > 0) {
      return this.targetContextConfigs;
    }
    const defaultBuffer = this.config.defaults?.contextWindowBufferTokens ?? AIAgentSession.DEFAULT_CONTEXT_BUFFER_TOKENS;
    return [{
      provider: 'default',
      model: 'default',
      contextWindow: AIAgentSession.DEFAULT_CONTEXT_WINDOW_TOKENS,
      tokenizerId: undefined,
      bufferTokens: defaultBuffer,
    }];
  }

  private estimateTokensForCounters(messages: ConversationMessage[]): number {
    if (messages.length === 0) {
      return 0;
    }
    let maxTokens = 0;
    const targets = this.getContextTargets();
    // eslint-disable-next-line functional/no-loop-statements
    for (const target of targets) {
      const tokenizer = resolveTokenizer(target.tokenizerId);
      const tokens = estimateMessagesTokens(tokenizer, messages);
      if (tokens > maxTokens) {
        maxTokens = tokens;
      }
    }
    const approxTokens = Math.ceil(estimateMessagesBytes(messages) / 4);
    return Math.max(maxTokens, approxTokens);
  }

  private estimateToolSchemaTokens(tools: readonly MCPTool[]): number {
    if (tools.length === 0) {
      return 0;
    }
    const serialized = tools.map((tool) => {
      const payload = {
        name: tool.name,
        description: tool.description,
        instructions: tool.instructions ?? '',
        inputSchema: tool.inputSchema,
      };
      return {
        role: 'system' as const,
        content: JSON.stringify(payload),
      } satisfies ConversationMessage;
    });
    const estimated = this.estimateTokensForCounters(serialized);
    if (process.env.CONTEXT_DEBUG === 'true') {
      console.log('context-guard/schema-estimate', { toolCount: tools.length, estimated });
    }
    return estimated;
  }

  private computeMaxOutputTokens(contextWindow: number): number {
    const configured = this.sessionConfig.maxOutputTokens;
    return configured ?? Math.max(0, Math.floor(contextWindow / 4));
  }

  private evaluateContextGuard(extraTokens = 0): ContextGuardEvaluation {
    const projectedTokens = this.currentCtxTokens + this.pendingCtxTokens + this.newCtxTokens + extraTokens;
    const blocked: ContextGuardBlockedEntry[] = [];
    const targets = this.getContextTargets();
    // eslint-disable-next-line functional/no-loop-statements
    for (const target of targets) {
      const contextWindow = target.contextWindow;
      const bufferTokens = target.bufferTokens;
      const maxOutputTokens = this.computeMaxOutputTokens(contextWindow);
      const limit = Math.max(0, contextWindow - bufferTokens - maxOutputTokens);
      if (limit > 0 && projectedTokens > limit) {
        blocked.push({
          provider: target.provider,
          model: target.model,
          contextWindow,
          bufferTokens,
          maxOutputTokens,
          limit,
          projected: projectedTokens,
        });
      }
    }
    return { blocked, projectedTokens };
  }

  private buildContextMetrics(provider: string, model: string): TurnRequestContextMetrics {
    const targets = this.getContextTargets();
    const target = targets.find((cfg) => cfg.provider === provider && cfg.model === model) ?? targets[0];
    const contextWindow = target.contextWindow;
    const bufferTokens = target.bufferTokens;
    const maxOutputTokens = this.computeMaxOutputTokens(contextWindow);
    const ctxTokens = this.currentCtxTokens;
    const newTokens = this.pendingCtxTokens;
    const schemaTokens = this.schemaCtxTokens;
    const expectedTokens = ctxTokens + newTokens + schemaTokens;
    const expectedPct = contextWindow > 0
      ? Math.round(((expectedTokens + bufferTokens + maxOutputTokens) / contextWindow) * 100)
      : undefined;
    if (process.env.CONTEXT_DEBUG === 'true') {
      console.log('context-guard/build-metrics', {
        provider,
        model,
        ctxTokens,
        pendingCtxTokens: this.pendingCtxTokens,
        schemaCtxTokens: this.schemaCtxTokens,
        newTokens,
        schemaTokens,
        expectedTokens,
      });
    }
    return {
      ctxTokens,
      newTokens,
      schemaTokens,
      expectedTokens,
      expectedPct,
      contextWindow,
      bufferTokens,
      maxOutputTokens,
    };
  }

  private evaluateContextForProvider(provider: string, model: string): 'ok' | 'skip' | 'final' {
    const targets = this.getContextTargets();
    const evaluation = this.evaluateContextGuard(this.schemaCtxTokens);
    if (process.env.CONTEXT_DEBUG === 'true') {
      console.log('context-guard/provider-eval', {
        provider,
        model,
        blocked: evaluation.blocked.map((entry) => ({
          provider: entry.provider,
          model: entry.model,
          limit: entry.limit,
          projected: entry.projected,
        })),
      });
    }
    if (evaluation.blocked.length === 0) {
      return 'ok';
    }
    const blockedKeys = new Set(evaluation.blocked.map((entry) => `${entry.provider}:${entry.model}`));
    const currentKey = `${provider}:${model}`;
    if (blockedKeys.size >= targets.length) {
      return 'final';
    }
    if (blockedKeys.has(currentKey)) {
      return 'skip';
    }
    if (targets.length === 1 && targets[0].provider === 'default') {
      return 'final';
    }
    return 'ok';
  }

  private enforceContextFinalTurn(blocked: ContextGuardBlockedEntry[], trigger: 'tool_preflight' | 'turn_preflight'): void {
    // Emit metrics for each blocked provider/model combination
    if (process.env.CONTEXT_DEBUG === 'true') {
      console.log('context-guard/enforce', {
        trigger,
        blocked: blocked.map((entry) => ({
          provider: entry.provider,
          model: entry.model,
          limit: entry.limit,
          projected: entry.projected,
        })),
      });
    }
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
    if (this.forcedFinalTurnReason === undefined) {
      this.forcedFinalTurnReason = 'context';
      this.contextLimitWarningLogged = false;
    }
    this.pushSystemRetryMessage(this.conversation, AIAgentSession.CONTEXT_FINAL_MESSAGE);
    if (this.contextLimitWarningLogged) {
      return;
    }
    const first = blocked[0] ?? {
      provider: 'unknown',
      model: 'unknown',
      contextWindow: AIAgentSession.DEFAULT_CONTEXT_WINDOW_TOKENS,
      bufferTokens: AIAgentSession.DEFAULT_CONTEXT_BUFFER_TOKENS,
      maxOutputTokens: this.computeMaxOutputTokens(AIAgentSession.DEFAULT_CONTEXT_WINDOW_TOKENS),
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


  private getFinalReportStatus(): 'success' | 'failure' | 'partial' | undefined {
    const status = this.finalReport?.status;
    if (status === 'success' || status === 'failure' || status === 'partial') {
      return status;
    }
    return undefined;
  }

  private composeRemoteIdentifier(provider: string, model: string, metadata?: { actualProvider?: string; actualModel?: string }): string {
    const providerSegment = metadata?.actualProvider !== undefined && metadata.actualProvider.length > 0 && metadata.actualProvider !== provider
      ? `${provider}/${metadata.actualProvider}`
      : provider;
    const modelSegment = metadata?.actualModel ?? model;
    return `${providerSegment}:${modelSegment}`;
  }

  private debugLogRawConversation(provider: string, model: string, payload: ConversationMessage[]): void {
    if (process.env.DEBUG !== 'true') {
      return;
    }
    try {
      warn(`[DEBUG] ${provider}:${model} raw_messages: ${JSON.stringify(payload, null, 2)}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      warn(`[DEBUG] ${provider}:${model} raw_messages: <unserializable: ${message}>`);
    }
  }

  private buildFallbackRetryDirective(input: { status: TurnStatus; remoteId: string; attempt: number }): TurnRetryDirective | undefined {
    const { status, remoteId, attempt } = input;
    if (status.type === 'rate_limit') {
      const wait = typeof status.retryAfterMs === 'number' && Number.isFinite(status.retryAfterMs)
        ? status.retryAfterMs
        : undefined;
      return {
        action: 'retry',
        backoffMs: wait,
        logMessage: `Rate limited; backing off ${wait !== undefined ? `${String(wait)}ms` : 'briefly'} before retry.`,
        systemMessage: `System notice: upstream provider ${remoteId} rate-limited the previous request. Retrying shortly; no changes required unless rate limits persist.`,
        sources: status.sources,
      };
    }
    if (status.type === 'auth_error') {
      return {
        action: AIAgentSession.RETRY_ACTION_SKIP_PROVIDER,
        logMessage: `Authentication error: ${status.message}. Skipping provider ${remoteId}.`,
        systemMessage: `System notice: authentication failed for ${remoteId}. This provider will be skipped; verify credentials before retrying later.`,
      };
    }
    if (status.type === 'quota_exceeded') {
      return {
        action: AIAgentSession.RETRY_ACTION_SKIP_PROVIDER,
        logMessage: `Quota exceeded for ${remoteId}; moving to next provider.`,
        systemMessage: `System notice: ${remoteId} exhausted its quota. Switching to the next configured provider.`,
      };
    }
    if (status.type === 'timeout' || status.type === 'network_error') {
      // Provide a gentle retry hint so the loop still emits guidance when providers omit details.
      const reason = status.message;
      const prettyType = status.type.replace('_', ' ');
      return {
        action: 'retry',
        backoffMs: Math.min(Math.max((attempt + 1) * 1_000, 1_000), 30_000),
        logMessage: `Transient ${prettyType} encountered on ${remoteId}: ${reason}; retrying shortly.`,
        systemMessage: `System notice: ${remoteId} experienced a transient ${prettyType} (${reason}). Retrying shortly.`,
      };
    }
    return undefined;
  }

  private tryAdoptFinalReportFromText(
    assistantMessage: ConversationMessage | undefined,
    text: string
  ): boolean {
    if (typeof text !== 'string') return false;
    const trimmedText = text.trim();
    if (trimmedText.length === 0) return false;
    const json = this.extractJsonRecordFromText(trimmedText);
     
    if (json === undefined) return false;
    const isRecord = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value);
    const pickString = (value: unknown): string | undefined => (typeof value === 'string' ? value : undefined);
    const statusRaw = json.status;
    const formatRaw = pickString(json.report_format) ?? pickString(json.format);
    const contentJsonRaw = json.content_json;
    const contentJson = isRecord(contentJsonRaw) ? contentJsonRaw : undefined;
    const contentCandidate = pickString(json.report_content) ?? pickString(json.content);
    if (typeof statusRaw !== 'string') return false;
    const formatCandidate = (formatRaw ?? '').trim();
    const formatMatch = FINAL_REPORT_FORMAT_VALUES.find((value) => value === formatCandidate);
    if (formatMatch === undefined) return false;
    const finalFormat = formatMatch;
    const normalizedStatus = statusRaw === 'success' || statusRaw === 'failure' || statusRaw === 'partial'
      ? statusRaw
      : undefined;
    if (normalizedStatus === undefined) return false;
    const finalStatus: 'success' | 'failure' | 'partial' = normalizedStatus;
    let finalContent: string | undefined = contentCandidate;
    if ((finalContent === undefined || finalContent.trim().length === 0) && contentJson !== undefined) {
      try {
        finalContent = JSON.stringify(contentJson);
      } catch {
        finalContent = undefined;
      }
    }
    if (finalContent === undefined) return false;
    if (finalContent.trim().length === 0) return false;
    const metadata = isRecord(json.metadata) ? json.metadata : undefined;
    const parameters: Record<string, unknown> = {
      status: finalStatus,
      report_format: finalFormat,
      report_content: finalContent,
    };
    if (contentJson !== undefined) parameters.content_json = contentJson;
    if (metadata !== undefined) parameters.metadata = metadata;
    if (assistantMessage !== undefined) {
      const syntheticCall: ToolCall = {
        id: 'synthetic-final-report-' + Date.now().toString(),
        name: AIAgentSession.FINAL_REPORT_TOOL,
        parameters,
      };
      assistantMessage.toolCalls = [syntheticCall];
    }
    const finalReportPayload: NonNullable<AIAgentResult['finalReport']> = {
      status: finalStatus,
      format: finalFormat,
      ts: Date.now(),
    };
    finalReportPayload.content = finalContent;
    if (contentJson !== undefined) {
      finalReportPayload.content_json = contentJson;
    }
    if (metadata !== undefined) {
      finalReportPayload.metadata = metadata;
    }
    this.finalReport = finalReportPayload;
    return true;
  }

  private sanitizeTurnMessages(
    messages: ConversationMessage[],
    context: { turn: number; provider: string; model: string }
  ): { messages: ConversationMessage[]; dropped: number } {
    let dropped = 0;
    const isRecord = (value: unknown): value is Record<string, unknown> =>
      value !== null && typeof value === 'object' && !Array.isArray(value);
    const sanitized = messages.map((msg, msgIndex) => {
      const rawToolCalls = (msg as { toolCalls?: unknown }).toolCalls;
      if (msg.role !== 'assistant' || !Array.isArray(rawToolCalls) || rawToolCalls.length === 0) {
        return msg;
      }
      const validCalls: ToolCall[] = [];
      const droppedReasons: string[] = [];
      rawToolCalls.forEach((tcRaw, callIndex) => {
        const callIndexStr = callIndex.toString();
        if (!isRecord(tcRaw)) {
          droppedReasons.push(`call ${callIndexStr}: invalid structure`);
          return;
        }
        const rawIdValue = tcRaw.id;
        const rawId = typeof rawIdValue === 'number' && Number.isFinite(rawIdValue)
          ? rawIdValue.toString()
          : (typeof rawIdValue === 'string' ? rawIdValue.trim() : '');
        if (rawId.length === 0) {
          droppedReasons.push(`call ${callIndexStr}: missing id`);
          return;
        }
        const rawNameValue = tcRaw.name;
        const rawName = typeof rawNameValue === 'string' ? sanitizeToolName(rawNameValue) : '';
        if (rawName.length === 0) {
          droppedReasons.push(`call ${callIndexStr}: missing name`);
          return;
        }
        let paramsCandidate: unknown = tcRaw.parameters;
        if (!isRecord(paramsCandidate) && typeof paramsCandidate === 'string') {
          const parsed = this.parseJsonRecord(paramsCandidate);
          if (parsed !== undefined) {
            paramsCandidate = parsed;
          }
        }
        if (!isRecord(paramsCandidate)) {
          droppedReasons.push(`call ${callIndexStr}: parameters not object`);
          return;
        }
        validCalls.push({
          id: rawId,
          name: rawName,
          parameters: paramsCandidate,
        });
      });

      if (droppedReasons.length > 0) {
        dropped += droppedReasons.length;
        const countStr = droppedReasons.length.toString();
        const msgIndexStr = msgIndex.toString();
        const warnEntry: LogEntry = {
          timestamp: Date.now(),
          severity: 'WRN',
          turn: context.turn,
          subturn: 0,
          direction: 'response',
          type: 'llm',
          remoteIdentifier: 'agent:sanitizer',
          fatal: false,
          message: `Dropped ${countStr} invalid tool call(s) from assistant message index ${msgIndexStr} (provider=${context.provider}:${context.model}): ${droppedReasons.join(', ')}`,
        };
        this.log(warnEntry);
      }

      if (validCalls.length === rawToolCalls.length) {
        return { ...msg, toolCalls: validCalls };
      }

      if (validCalls.length === 0) {
        const sanitizedMsg: ConversationMessage = { ...msg, toolCalls: undefined };
        return sanitizedMsg;
      }

      return { ...msg, toolCalls: validCalls };
    });
    return { messages: sanitized, dropped };
  }

  private emitReasoningChunks(chunks: string[]): void {
    if (this.parentTxnId !== undefined) {
      return;
    }
    chunks.forEach((chunk) => {
      if (typeof chunk !== 'string') {
        return;
      }
      const trimmed = chunk.trim();
      if (trimmed.length === 0) {
        return;
      }
      try {
        this.sessionConfig.callbacks?.onThinking?.(chunk);
      } catch (e) {
        warn(`onThinking callback failed: ${e instanceof Error ? e.message : String(e)}`);
      }
      try {
        this.opTree.setLatestStatus(trimmed);
      } catch (e) {
        warn(`opTree.setLatestStatus failed: ${e instanceof Error ? e.message : String(e)}`);
      }
      try {
        if (typeof this.currentLlmOpId === 'string') {
          this.opTree.appendReasoningChunk(this.currentLlmOpId, chunk);
        }
      } catch (e) {
        warn(`appendReasoningChunk failed: ${e instanceof Error ? e.message : String(e)}`);
      }
    });
  }

  private async executeSingleTurn(
    conversation: ConversationMessage[],
    provider: string,
    model: string,
    isFinalTurn: boolean,
    currentTurn: number,
    logs: LogEntry[],
    accounting: AccountingEntry[],
    lastShownThinkingHeaderTurn: number,
    attempt: number,
    maxAttempts: number,
    reasoningLevel?: ReasoningLevel,
    reasoningValue?: ProviderReasoningValue | null
  ) {
    const attributes: Record<string, string | number | boolean> = {
      'ai.llm.provider': provider,
      'ai.llm.model': model,
      'ai.llm.is_final_turn': isFinalTurn,
    };
    if (typeof reasoningLevel === 'string' && reasoningLevel.length > 0) {
      attributes['ai.llm.reasoning.level'] = reasoningLevel;
    }
    if (typeof reasoningValue === 'number') {
      attributes['ai.llm.reasoning.value'] = reasoningValue;
    }
    return await runWithSpan('agent.llm.turn', { attributes, kind: SpanKind.CLIENT }, () =>
      this.executeSingleTurnInternal(
        conversation,
        provider,
        model,
        isFinalTurn,
        currentTurn,
        logs,
        accounting,
        lastShownThinkingHeaderTurn,
        attempt,
        maxAttempts,
        reasoningLevel,
        reasoningValue,
      )
    );
  }

  private async executeSingleTurnInternal(
    conversation: ConversationMessage[],
    provider: string,
    model: string,
    isFinalTurn: boolean,
    currentTurn: number,
    logs: LogEntry[],
    accounting: AccountingEntry[],
    lastShownThinkingHeaderTurn: number,
    attempt: number,
    maxAttempts: number,
    reasoningLevel?: ReasoningLevel,
    reasoningValue?: ProviderReasoningValue | null
  ) {
    // no spans; opTree is canonical
    // Expose provider tools (which includes internal tools from InternalToolProvider)
    const allTools = [
      ...(this.toolsOrchestrator?.listTools() ?? [])
    ];

    const filteredSelection = this.filterToolsForProvider(allTools, provider);
    const availableTools = filteredSelection.tools;
    const allowedToolNames = filteredSelection.allowedNames;
    const toolsForTurn = isFinalTurn
      ? (() => {
          const filtered = availableTools.filter((tool) => sanitizeToolName(tool.name) === AIAgentSession.FINAL_REPORT_TOOL);
          return filtered.length > 0 ? filtered : availableTools;
        })()
      : availableTools;
    let disableReasoningForTurn = false;

    this.schemaCtxTokens = this.estimateToolSchemaTokens(toolsForTurn);
    if (process.env.CONTEXT_DEBUG === 'true') {
      console.log('context-guard/schema-tokens', {
        turn: currentTurn,
        schemaCtxTokens: this.schemaCtxTokens,
        toolsForTurn: toolsForTurn.map((tool) => sanitizeToolName(tool.name)),
      });
    }

    const reasoningGuard = this.llmClient.shouldDisableReasoning(provider, {
      conversation,
      currentTurn,
      expectSignature: true,
    });
    if (reasoningGuard.disable) {
      conversation = reasoningGuard.normalized;
      disableReasoningForTurn = true;
    }

    addSpanAttributes({ 'ai.llm.reasoning.disabled': disableReasoningForTurn });

    // Track if we've shown thinking for this turn
    let shownThinking = false;
    this.toolFailureMessages.clear();
    this.toolFailureFallbacks.length = 0;
    
    // Create tool executor function that delegates to MCP client with accounting
    let subturnCounter = 0;
    let incompleteFinalReportDetected = false;
    const maxToolCallsPerTurn = Math.max(1, this.sessionConfig.maxToolCallsPerTurn ?? 10);
    const toolExecutor = async (toolName: string, parameters: Record<string, unknown>, options?: { toolCallId?: string }): Promise<string> => {
      if (this.stopRef?.stopping === true) throw new Error('stop_requested');
      if (this.canceled) throw new Error('canceled');
      const normalizedToolName = sanitizeToolName(toolName);
      const isFinalReportTool = normalizedToolName === AIAgentSession.FINAL_REPORT_TOOL
        || normalizedToolName === AIAgentSession.FINAL_REPORT_SHORT;
      if (isFinalReportTool) {
        const nestedCalls = this.parseNestedCallsFromFinalReport(parameters);
        if (nestedCalls !== undefined) {
          let nestedResult = AIAgentSession.TOOL_NO_OUTPUT;
          // eslint-disable-next-line functional/no-loop-statements
          for (const nestedCall of nestedCalls) {
            nestedResult = await toolExecutor(nestedCall.name, nestedCall.parameters);
          }
          return nestedResult;
        }
      }
      // Advance subturn for each tool call within this turn
      subturnCounter += 1;
      if (subturnCounter > maxToolCallsPerTurn) {
        const msg = `Tool calls per turn exceeded: limit=${String(maxToolCallsPerTurn)}. Switch strategy: avoid further tool calls this turn; either summarize progress or call ${AIAgentSession.FINAL_REPORT_TOOL} to conclude.`;
        const warn: LogEntry = { timestamp: Date.now(), severity: 'ERR', turn: currentTurn, subturn: subturnCounter, direction: 'response', type: 'tool', remoteIdentifier: 'agent:limits', fatal: false, message: msg };
        this.log(warn);
        throw new Error('tool_calls_per_turn_limit_exceeded');
      }
      // Orchestrator receives ctx turn/subturn per call

      // (no local preview helpers needed)

      // Normalize tool name to strip router/provider wrappers like <|constrain|>
      const effectiveToolName = normalizedToolName;
      addSpanEvent('tool.call.requested', { 'ai.tool.name': effectiveToolName });
      const startTime = Date.now();
      try {
        if (!allowedToolNames.has(effectiveToolName)) {
      const blocked: LogEntry = {
        timestamp: Date.now(),
        severity: 'WRN',
        turn: currentTurn,
        subturn: subturnCounter,
        direction: 'response',
        type: 'tool',
        remoteIdentifier: AIAgentSession.REMOTE_AGENT_TOOLS,
        fatal: false,
        message: `Tool '${effectiveToolName}' is not permitted for provider '${provider}'`
      };
          this.log(blocked);
          throw new Error('tool_not_permitted');
        }
        // Internal tools are handled by InternalToolProvider via orchestrator

        // Sub-agent execution is handled by the orchestrator (AgentProvider)

        // Orchestrator-managed execution for MCP + REST
        if (this.toolsOrchestrator?.hasTool(effectiveToolName) === true) {
          const isBatchTool = effectiveToolName === 'agent__batch';
          const isSubAgentTool = this.subAgents?.hasTool(effectiveToolName) === true;
          const managed = await this.toolsOrchestrator.executeWithManagement(
            effectiveToolName,
            parameters,
            { turn: currentTurn, subturn: subturnCounter },
            {
              timeoutMs: this.sessionConfig.toolTimeout,
              bypassConcurrency: isBatchTool,
              disableGlobalTimeout: isBatchTool
            }
          );
          if (isSubAgentTool) {
            this.invokedSubAgents.add(effectiveToolName);
          }
          const providerLabel = typeof managed.providerLabel === 'string' && managed.providerLabel.length > 0
            ? managed.providerLabel
            : AIAgentSession.REMOTE_AGENT_TOOLS;
          const toolOutput = managed.result.length > 0 ? managed.result : AIAgentSession.TOOL_NO_OUTPUT;
          const callId = typeof options?.toolCallId === 'string' && options.toolCallId.length > 0
            ? options.toolCallId
            : undefined;
          if (managed.dropped === true) {
            const failureReason = managed.reason ?? 'context_budget_exceeded';
            const failureTokens = this.estimateTokensForCounters([{ role: 'tool', content: toolOutput }]);
            this.newCtxTokens += failureTokens;
            if (callId !== undefined) {
              this.toolFailureMessages.set(callId, toolOutput);
            } else {
              this.toolFailureFallbacks.push(toolOutput);
            }
            const failureEntry: AccountingEntry = {
              type: 'tool',
              timestamp: startTime,
              status: 'failed',
              latency: managed.latency,
              mcpServer: providerLabel,
              command: effectiveToolName,
              charactersIn: managed.charactersIn,
              charactersOut: managed.charactersOut,
              error: failureReason,
              agentId: this.sessionConfig.agentId,
              callPath: this.callPath,
              txnId: this.txnId,
              parentTxnId: this.parentTxnId,
              originTxnId: this.originTxnId,
              details: {
                reason: failureReason,
                original_tokens: managed.tokens ?? 0,
                replacement_tokens: failureTokens,
              },
            };
            accounting.push(failureEntry);
            try { this.sessionConfig.callbacks?.onAccounting?.(failureEntry); } catch (e) { warn(`tool accounting callback failed: ${e instanceof Error ? e.message : String(e)}`); }
            addSpanEvent('tool.call.failure', {
              'ai.tool.name': effectiveToolName,
              'ai.tool.latency_ms': managed.latency,
              'ai.tool.failure.reason': failureReason,
            });
            return toolOutput;
          }

          const managedTokens = typeof managed.tokens === 'number' ? managed.tokens : undefined;
          let toolTokens: number;
          if (managedTokens === undefined) {
            toolTokens = this.estimateTokensForCounters([{ role: 'tool', content: toolOutput }]);
            const guardEvaluation = this.evaluateContextGuard(toolTokens);
            if (process.env.CONTEXT_DEBUG === 'true') {
              const approxTokens = Math.ceil(estimateMessagesBytes([{ role: 'tool', content: toolOutput }]) / 4);
              let limitTokens: number | undefined;
              let contextWindow: number | undefined;
              if (guardEvaluation.blocked.length > 0) {
                const firstBlocked = guardEvaluation.blocked[0];
                limitTokens = firstBlocked.limit;
                contextWindow = firstBlocked.contextWindow;
              }
              console.log('context-guard/tool-eval', {
                toolTokens,
                approxTokens,
                contentLength: toolOutput.length,
                currentCtx: this.currentCtxTokens,
                pendingCtx: this.pendingCtxTokens,
                newCtx: this.newCtxTokens,
                projectedTokens: guardEvaluation.projectedTokens,
                limitTokens,
                contextWindow,
              });
            }
            if (guardEvaluation.blocked.length > 0) {
              const blockedEntries = guardEvaluation.blocked.length > 0
                ? guardEvaluation.blocked
                : [{
                    provider: 'unknown',
                    model: 'unknown',
                    contextWindow: AIAgentSession.DEFAULT_CONTEXT_WINDOW_TOKENS,
                    bufferTokens: AIAgentSession.DEFAULT_CONTEXT_BUFFER_TOKENS,
                    maxOutputTokens: this.computeMaxOutputTokens(AIAgentSession.DEFAULT_CONTEXT_WINDOW_TOKENS),
                    limit: 0,
                    projected: guardEvaluation.projectedTokens,
                  }];
              const first = blockedEntries[0];
              const remainingTokens = first.limit > first.projected ? first.limit - first.projected : 0;
              const warnEntry: LogEntry = {
                timestamp: Date.now(),
                severity: 'WRN',
                turn: currentTurn,
                subturn: subturnCounter,
                direction: 'response',
                type: 'tool',
                remoteIdentifier: providerLabel,
                fatal: false,
                message: `Tool '${effectiveToolName}' output discarded: context window budget exceeded (${String(toolTokens)} tokens, limit ${String(first.limit)}).`,
                details: {
                  tool: effectiveToolName,
                  provider: providerLabel,
                  tokens_estimated: toolTokens,
                  projected_tokens: guardEvaluation.projectedTokens,
                  limit_tokens: first.limit,
                  remaining_tokens: remainingTokens,
                },
              };
              this.log(warnEntry);
              const renderedFailure = '(tool failed: context window budget exceeded)';
              const failureTokens = this.estimateTokensForCounters([{ role: 'tool', content: renderedFailure }]);
              this.newCtxTokens += failureTokens;
              if (callId !== undefined) {
                this.toolFailureMessages.set(callId, renderedFailure);
              } else {
                this.toolFailureFallbacks.push(renderedFailure);
              }
              const failureEntry: AccountingEntry = {
                type: 'tool',
                timestamp: startTime,
                status: 'failed',
                latency: managed.latency,
                mcpServer: providerLabel,
                command: effectiveToolName,
                charactersIn: managed.charactersIn,
                charactersOut: managed.charactersOut,
                error: 'context_budget_exceeded',
                agentId: this.sessionConfig.agentId,
                callPath: this.callPath,
                txnId: this.txnId,
                parentTxnId: this.parentTxnId,
                originTxnId: this.originTxnId,
                details: {
                  projected_tokens: first.projected,
                  limit_tokens: first.limit,
                  remaining_tokens: remainingTokens,
                  original_tokens: toolTokens,
                  replacement_tokens: failureTokens,
                },
              };
              accounting.push(failureEntry);
              try { this.sessionConfig.callbacks?.onAccounting?.(failureEntry); } catch (e) { warn(`tool accounting callback failed: ${e instanceof Error ? e.message : String(e)}`); }
              addSpanEvent('tool.call.failure', {
                'ai.tool.name': effectiveToolName,
                'ai.tool.latency_ms': managed.latency,
                'ai.tool.failure.reason': 'context_budget_exceeded',
              });
              this.enforceContextFinalTurn(blockedEntries, 'tool_preflight');
              return renderedFailure;
            }
          } else {
            toolTokens = managedTokens;
            if (process.env.CONTEXT_DEBUG === 'true') {
              const approxTokens = Math.ceil(estimateMessagesBytes([{ role: 'tool', content: toolOutput }]) / 4);
              console.log('context-guard/tool-eval', {
                toolTokens,
                approxTokens,
                contentLength: toolOutput.length,
                currentCtx: this.currentCtxTokens,
                pendingCtx: this.pendingCtxTokens,
                newCtx: this.newCtxTokens,
                projectedTokens: this.currentCtxTokens + this.pendingCtxTokens + this.newCtxTokens,
                limitTokens: undefined,
                contextWindow: undefined,
              });
            }
          }

          this.newCtxTokens += toolTokens;
          const successEntry: AccountingEntry = {
            type: 'tool',
            timestamp: startTime,
            status: 'ok',
            latency: managed.latency,
            mcpServer: providerLabel,
            command: effectiveToolName,
            charactersIn: managed.charactersIn,
            charactersOut: managed.charactersOut,
            agentId: this.sessionConfig.agentId,
            callPath: this.callPath,
            txnId: this.txnId,
            parentTxnId: this.parentTxnId,
            originTxnId: this.originTxnId,
            details: { tokens: toolTokens },
          };
          accounting.push(successEntry);
          try { this.sessionConfig.callbacks?.onAccounting?.(successEntry); } catch (e) { warn(`tool accounting callback failed: ${e instanceof Error ? e.message : String(e)}`); }
          if (managedTokens === undefined) {
            const toolLog: LogEntry = {
              timestamp: Date.now(),
              severity: 'VRB',
              turn: currentTurn,
              subturn: subturnCounter,
              direction: 'response',
              type: 'tool',
              remoteIdentifier: providerLabel,
              fatal: false,
              message: `Tool '${effectiveToolName}' completed (${String(managed.charactersOut)} bytes, ${String(toolTokens)} tokens).`,
              details: {
                latency_ms: managed.latency,
                characters_in: managed.charactersIn,
                characters_out: managed.charactersOut,
                tokens: toolTokens,
              },
            };
            this.log(toolLog);
          }
          addSpanEvent('tool.call.success', {
            'ai.tool.name': effectiveToolName,
            'ai.tool.latency_ms': managed.latency,
            'ai.tool.tokens': toolTokens,
          });
          return toolOutput;
        }

        // Unknown tool after all paths
        {
          const req = formatToolRequestCompact(toolName, parameters);
          const warn: LogEntry = { timestamp: Date.now(), severity: 'WRN', turn: currentTurn, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'assistant:tool', fatal: false, message: `Unknown tool requested: ${req}` };
          this.log(warn);
          addSpanEvent('tool.call.unknown', { 'ai.tool.name': effectiveToolName });
          throw new Error(`No server found for tool: ${effectiveToolName}`);
        }
      } catch (error) {
        const latency = Date.now() - startTime;
        const errorMsg = error instanceof Error ? error.message : String(error);
        addSpanEvent('tool.call.error', { 'ai.tool.name': effectiveToolName, 'ai.tool.error': errorMsg });

        // Add failed tool accounting
        const accountingEntry: AccountingEntry = {
          type: 'tool',
          timestamp: startTime,
          status: 'failed',
          latency,
          mcpServer: 'unknown', // Can't get server name if the call failed early
          command: toolName,
          charactersIn: JSON.stringify(parameters).length,
          charactersOut: 0,
          error: errorMsg,
          agentId: this.sessionConfig.agentId, callPath: this.callPath, txnId: this.txnId, parentTxnId: this.parentTxnId, originTxnId: this.originTxnId
        };
        accounting.push(accountingEntry);
        try { this.sessionConfig.callbacks?.onAccounting?.(accountingEntry); } catch (e) { warn(`tool accounting callback failed: ${e instanceof Error ? e.message : String(e)}`); }

        // Check if this is an incomplete final_report error
        if (toolName === AIAgentSession.FINAL_REPORT_TOOL) {
          incompleteFinalReportDetected = true;
        }

        // Return error message instead of throwing - ensures LLM always gets valid tool output
        const limitMessage = `execution not allowed because the per-turn tool limit (${String(maxToolCallsPerTurn)}) was reached; retry this tool on the next turn if available.`;
        const failureDetail = errorMsg === 'tool_calls_per_turn_limit_exceeded' ? limitMessage : errorMsg;
        const renderedFailure = `(tool failed: ${failureDetail})`;
        if (typeof options?.toolCallId === 'string' && options.toolCallId.length > 0) {
          this.toolFailureMessages.set(options.toolCallId, renderedFailure);
        } else {
          this.toolFailureFallbacks.push(renderedFailure);
        }
        return renderedFailure;
      }
    };
    const modelOverrides = this.resolveModelOverrides(provider, model);
    let effectiveTemperature = this.sessionConfig.temperature;
    if (modelOverrides.temperature !== undefined) {
      if (modelOverrides.temperature === null) effectiveTemperature = undefined;
      else effectiveTemperature = modelOverrides.temperature;
    }
    let effectiveTopP = this.sessionConfig.topP;
    if (modelOverrides.topP !== undefined) {
      if (modelOverrides.topP === null) effectiveTopP = undefined;
      else effectiveTopP = modelOverrides.topP;
    }

    const fallbackReasoningLevel = this.sessionConfig.reasoning;
    let effectiveReasoningLevel = reasoningLevel ?? fallbackReasoningLevel;
    const targetMaxOutputTokens = this.sessionConfig.maxOutputTokens;
    let effectiveReasoningValue = reasoningValue ?? (
      effectiveReasoningLevel !== undefined
        ? this.resolveReasoningValue(provider, model, effectiveReasoningLevel, targetMaxOutputTokens)
        : undefined
    );
    const reasoningActive = effectiveReasoningLevel !== undefined || (effectiveReasoningValue !== undefined && effectiveReasoningValue !== null);
    if (disableReasoningForTurn) {
      if (reasoningActive) {
        const warnEntry: LogEntry = {
          timestamp: Date.now(),
          severity: 'WRN',
          turn: currentTurn,
          subturn: 0,
          direction: 'response',
          type: 'llm',
          remoteIdentifier: 'agent:reasoning',
          fatal: false,
          message: 'Anthropic reasoning disabled for this turn: previous assistant tool call lacked signature metadata.',
        };
        this.log(warnEntry);
      }
      effectiveReasoningLevel = undefined;
      effectiveReasoningValue = null;
    }
    addSpanAttributes({
      'ai.llm.reasoning.level_effective': effectiveReasoningLevel ?? 'none',
      'ai.llm.reasoning.value_effective': effectiveReasoningValue ?? 'none',
    });
    const sendReasoning = disableReasoningForTurn ? false : undefined;
    const sessionStreamFlag = this.sessionConfig.stream === true;
    const autoEnableReasoningStream = this.llmClient.shouldAutoEnableReasoningStream(
      provider,
      effectiveReasoningLevel,
      {
        maxOutputTokens: targetMaxOutputTokens,
        reasoningActive,
        streamRequested: sessionStreamFlag,
      }
    );
    let effectiveStream = sessionStreamFlag;
    if (!effectiveStream && autoEnableReasoningStream) {
      effectiveStream = true;
      const warnEntry: LogEntry = {
        timestamp: Date.now(),
        severity: 'WRN',
        turn: currentTurn,
        subturn: 0,
        direction: 'response',
        type: 'llm',
        remoteIdentifier: 'agent:reasoning',
        fatal: false,
        message: `Provider '${provider}' requires streaming for the current reasoning request; overriding stream=true.`,
      };
      this.log(warnEntry);
    }
    const requestMessages: ConversationMessage[] = this.mergePendingRetryMessages(conversation);
    const requestMessagesBytes = estimateMessagesBytes(requestMessages);
    this.pendingRetryMessages = [];
    const metricsForRequest = this.buildContextMetrics(provider, model);
    if (process.env.CONTEXT_DEBUG === 'true') {
      console.log('context-guard/request-metrics', { provider, model, metricsForRequest });
    }

    const turnMetadata: TurnRequest['turnMetadata'] = {
      attempt,
      maxAttempts,
      turn: currentTurn,
      isFinalTurn,
    };
    if (effectiveReasoningLevel !== undefined) {
      turnMetadata.reasoningLevel = effectiveReasoningLevel;
    }
    if (effectiveReasoningValue !== undefined) {
      turnMetadata.reasoningValue = effectiveReasoningValue;
    }

    const request: TurnRequest = {
      messages: requestMessages,
      provider,
      model,
      tools: toolsForTurn,
      toolExecutor,
      temperature: effectiveTemperature,
      topP: effectiveTopP,
      maxOutputTokens: targetMaxOutputTokens,
      repeatPenalty: this.sessionConfig.repeatPenalty,
      stream: effectiveStream,
      isFinalTurn,
      llmTimeout: this.sessionConfig.llmTimeout,
      toolChoice: this.resolveToolChoice(provider, model),
      abortSignal: this.abortSignal,
      sendReasoning,
      onChunk: (chunk: string, type: 'content' | 'thinking') => {
        const isRootSession = this.parentTxnId === undefined;
        if (type === 'content' && this.sessionConfig.callbacks?.onOutput !== undefined) {
          this.sessionConfig.callbacks.onOutput(chunk);
        } else if (type === 'thinking') {
          const trimmedThinking = chunk.trim();
          if (isRootSession && trimmedThinking.length > 0) {
            this.opTree.setLatestStatus(trimmedThinking);
          }
          if (lastShownThinkingHeaderTurn !== currentTurn) {
            const thinkingHeader: LogEntry = {
              timestamp: Date.now(),
              severity: 'THK',
              turn: currentTurn,
              subturn: 0,
              direction: 'response',
              type: 'llm',
              remoteIdentifier: 'thinking',
              fatal: false,
              message: 'reasoning output stream'
            };
            this.log(thinkingHeader);
            shownThinking = true;
            lastShownThinkingHeaderTurn = currentTurn;
          }
          if (isRootSession) {
            this.sessionConfig.callbacks?.onThinking?.(chunk);
          }
          try {
            if (typeof this.currentLlmOpId === 'string') this.opTree.appendReasoningChunk(this.currentLlmOpId, chunk);
          } catch (e) { warn(`appendReasoningChunk failed: ${e instanceof Error ? e.message : String(e)}`); }
        }
      },
      reasoningLevel: effectiveReasoningLevel,
      reasoningValue: effectiveReasoningValue,
      turnMetadata,
      caching: this.sessionConfig.caching,
      contextMetrics: metricsForRequest,
    };

    this.debugLogRawConversation(provider, model, conversation);

    try {
      const result = await this.llmClient.executeTurn(request);
      const tokens = result.tokens;
      const promptTokens = tokens?.inputTokens ?? 0;
      const completionTokens = tokens?.outputTokens ?? 0;
      const cacheReadTokens = tokens?.cacheReadInputTokens ?? tokens?.cachedTokens ?? 0;
      const cacheWriteTokens = tokens?.cacheWriteInputTokens ?? 0;
      const responseBytes = typeof result.response === 'string' && result.response.length > 0
        ? Buffer.byteLength(result.response, 'utf8')
        : estimateMessagesBytes(result.messages);
      const costInfo = this.llmClient.getLastCostInfo();
      const attempts = request.turnMetadata?.attempt ?? 1;
      const retries = attempts > 1 ? attempts - 1 : 0;
      recordLlmMetrics({
        agentId: this.sessionConfig.agentId,
        callPath: this.callPath,
        headendId: this.headendId,
        provider,
        model,
        status: result.status.type === 'success' ? 'success' : 'error',
        errorType: result.status.type === 'success' ? undefined : result.status.type,
        latencyMs: result.latencyMs,
        promptTokens,
        completionTokens,
        cacheReadTokens,
        cacheWriteTokens,
        requestBytes: requestMessagesBytes,
        responseBytes,
        retries,
        costUsd: costInfo?.costUsd,
        reasoningLevel: effectiveReasoningLevel ?? this.sessionConfig.reasoning,
        customLabels: this.telemetryLabels,
      });
      addSpanAttributes({
        'ai.llm.latency_ms': result.latencyMs,
        'ai.llm.prompt_tokens': promptTokens,
        'ai.llm.completion_tokens': completionTokens,
        'ai.llm.cache_read_tokens': cacheReadTokens,
        'ai.llm.cache_write_tokens': cacheWriteTokens,
        'ai.llm.retry_count': retries,
        'ai.llm.request_bytes': requestMessagesBytes,
        'ai.llm.response_bytes': responseBytes,
      });
      if (typeof result.stopReason === 'string' && result.stopReason.length > 0) {
        addSpanAttributes({ 'ai.llm.stop_reason': result.stopReason });
      }
      if (result.status.type !== 'success') {
        addSpanAttributes({ 'ai.llm.status': result.status.type });
      }
      return { ...result, shownThinking, incompleteFinalReportDetected };
    } catch (e) {
      // const msg = e instanceof Error ? e.message : String(e);
      throw e;
    }
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

  private computeForcedFinalSchemaTokens(provider: string): number {
    const allTools = [...(this.toolsOrchestrator?.listTools() ?? [])];
    if (allTools.length === 0) {
      return this.schemaCtxTokens;
    }
    const filtered = this.filterToolsForProvider(allTools, provider).tools;
    if (filtered.length === 0) {
      return this.schemaCtxTokens;
    }
    const finalOnly = filtered.filter((tool) => sanitizeToolName(tool.name) === AIAgentSession.FINAL_REPORT_TOOL);
    const targetTools = finalOnly.length > 0 ? finalOnly : filtered;
    return this.estimateToolSchemaTokens(targetTools);
  }

  private resolveReasoningMapping(provider: string, model: string): ProviderReasoningMapping | null | undefined {
    const providerConfig = this.sessionConfig.config.providers[provider] as (Configuration['providers'][string] | undefined);
    if (providerConfig === undefined) return undefined;
    const modelReasoning = providerConfig.models?.[model]?.reasoning;
    if (modelReasoning !== undefined) return modelReasoning;
    if (providerConfig.reasoning !== undefined) return providerConfig.reasoning;
    return undefined;
  }

  private resolveToolChoice(provider: string, model: string): ToolChoiceMode | undefined {
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

  private enhanceSystemPrompt(systemPrompt: string, toolsInstructions: string): string {
    const blocks: string[] = [systemPrompt];
    if (toolsInstructions.trim().length > 0) blocks.push(`## TOOLS' INSTRUCTIONS\n\n${toolsInstructions}`);
    return blocks.join('\n\n');
  }

  // Extracted batch execution for readability
  private async executeBatchCalls(
    parameters: Record<string, unknown>,
    currentTurn: number,
    startTime: number,
    logs: LogEntry[],
    accounting: AccountingEntry[],
    toolName: string
  ): Promise<string> {
    interface BatchCall { id: string; tool: string; parameters: Record<string, unknown> }
    interface BatchInput { calls: BatchCall[] }

    const isPlainObject = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object' && !Array.isArray(v);
    const asString = (v: unknown): string | undefined => typeof v === 'string' ? v : undefined;
    const asId = (v: unknown): string | undefined => (typeof v === 'string') ? v : (typeof v === 'number' && Number.isFinite(v) ? String(v) : undefined);

    // Shared constants for batch logging
    const BATCH_REMOTE_ID = 'agent:batch' as const;
    const RESP_DIR = 'response' as const;
    const LLM_TYPE = 'llm' as const;

    const bi: BatchInput | undefined = (() => {
      if (!isPlainObject(parameters)) return undefined;
      const p: Record<string, unknown> = parameters;
      const callsRaw = p.calls;
      if (!Array.isArray(callsRaw)) return undefined;
      const calls: BatchCall[] = callsRaw.map((cUnknown) => {
        const c = isPlainObject(cUnknown) ? cUnknown : {};
        const id = asId(c.id) ?? '';
        const tool = asString(c.tool) ?? '';
        const parameters = (isPlainObject(c.parameters) ? c.parameters : {});
        return { id, tool, parameters };
      });
      return { calls };
    })();

    if (bi === undefined || bi.calls.some((c) => c.id.length === 0 || c.tool.length === 0)) {
      // Warn once for invalid batch input
      try {
        const warn: LogEntry = { timestamp: Date.now(), severity: 'WRN', turn: currentTurn, subturn: 0, direction: RESP_DIR, type: LLM_TYPE, remoteIdentifier: BATCH_REMOTE_ID, fatal: false, message: 'Invalid batch input: each call requires id, tool, parameters' };
        this.log(warn);
      } catch (e) { warn(`LLM error log failed: ${e instanceof Error ? e.message : String(e)}`); }
      const latency = Date.now() - startTime;
      const accountingEntry: AccountingEntry = {
        type: 'tool', timestamp: startTime, status: 'failed', latency,
        mcpServer: 'agent', command: toolName,
        charactersIn: JSON.stringify(parameters).length, charactersOut: 0,
        error: 'invalid_batch_input',
        agentId: this.sessionConfig.agentId, callPath: this.callPath, txnId: this.txnId, parentTxnId: this.parentTxnId, originTxnId: this.originTxnId
      };
      accounting.push(accountingEntry);
      this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
      throw new Error('invalid_batch_input: calls[] requires id, tool, parameters');
    }

    // Batch execution: orchestrator-only inner calls; record a span for the batch
    // no spans
    const results = await Promise.all(bi.calls.map(async (c) => {
      const t0 = Date.now();
      // Allow progress_report in batch, but not final_report or nested batch
      if (c.tool === AIAgentSession.FINAL_REPORT_TOOL || c.tool === 'agent__batch') {
        return { id: c.id, tool: c.tool, ok: false, elapsedMs: 0, error: { code: 'INTERNAL_NOT_ALLOWED', message: 'Internal tools are not allowed in batch' } };
      }
      // Handle progress_report directly in batch
      if (c.tool === 'agent__progress_report') {
        if (!this.progressToolEnabled) {
          return { id: c.id, tool: c.tool, ok: false, elapsedMs: 0, error: { code: 'PROGRESS_DISABLED', message: 'agent__progress_report is disabled for this session' } };
        }
        const progress = typeof (c.parameters.progress) === 'string' ? c.parameters.progress : '';
        if (progress.trim().length > 0) {
          this.opTree.setLatestStatus(progress);
          // Trigger callback to notify listeners (like Slack progress updates)
          try {
            this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
          } catch (e) {
            warn(`onOpTree callback failed: ${e instanceof Error ? e.message : String(e)}`);
          }
          this.progressReporter.agentUpdate({
            callPath: this.getCallPathLabel(),
            agentId: this.getAgentIdLabel(),
            agentPath: this.getAgentPathLabel(),
            agentName: this.getAgentDisplayName(),
            txnId: this.txnId,
            parentTxnId: this.parentTxnId,
            originTxnId: this.originTxnId,
            message: progress,
          });
        }
        return { id: c.id, tool: c.tool, ok: true, elapsedMs: 0, output: JSON.stringify({ ok: true }) };
      }
      try {
        if (!(this.toolsOrchestrator?.hasTool(c.tool) ?? false)) {
          return { id: c.id, tool: c.tool, ok: false, elapsedMs: 0, error: { code: 'UNKNOWN_TOOL', message: `Unknown tool: ${c.tool}` } };
        }
        const orchestrator = (this.toolsOrchestrator as unknown) as { executeWithManagement: (t: string, a: Record<string, unknown>, ctx: { turn: number; subturn: number }, opts?: { timeoutMs?: number }) => Promise<ManagedToolResult> };
        const managed = await orchestrator.executeWithManagement(c.tool, c.parameters, { turn: currentTurn, subturn: 0 }, { timeoutMs: this.sessionConfig.toolTimeout });
        return { id: c.id, tool: c.tool, ok: true, elapsedMs: managed.latency, output: managed.result };
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        return { id: c.id, tool: c.tool, ok: false, elapsedMs: Date.now() - t0, error: { code: 'EXECUTION_ERROR', message: msg } };
      }
    }));
    const payload = this.applyToolResponseCap(JSON.stringify({ results }), this.sessionConfig.toolResponseMaxBytes, logs, { server: 'agent', tool: toolName, turn: currentTurn, subturn: 0 });
    // end via opTree only
    return payload;
  }

  private expandNestedLLMToolCalls(toolCalls: ToolCall[]): ToolCall[] | undefined {
    let mutated = false;
    const expanded: ToolCall[] = [];
    // eslint-disable-next-line functional/no-loop-statements
    for (const call of toolCalls) {
      const normalizedName = sanitizeToolName(call.name);
      if (normalizedName === AIAgentSession.FINAL_REPORT_TOOL) {
        const nested = this.parseNestedCallsFromFinalReport(call.parameters);
        if (nested !== undefined) {
          expanded.push(...nested);
          mutated = true;
          continue;
        }
      }
      expanded.push(call);
    }
    return mutated ? expanded : undefined;
  }

  private parseNestedCallsFromFinalReport(parameters: Record<string, unknown>): ToolCall[] | undefined {
    const rawCalls = (parameters as { calls?: unknown }).calls;
    if (!Array.isArray(rawCalls) || rawCalls.length === 0) {
      return undefined;
    }
    const parsed: ToolCall[] = [];
    const isRecord = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value);
    // eslint-disable-next-line functional/no-loop-statements
    for (const entryUnknown of rawCalls) {
      if (entryUnknown === null || typeof entryUnknown !== 'object' || Array.isArray(entryUnknown)) {
        return undefined;
      }
      const entry = entryUnknown as { id?: unknown; tool?: unknown; parameters?: unknown };
      const toolRaw = entry.tool;
      if (typeof toolRaw !== 'string' || toolRaw.trim().length === 0) {
        return undefined;
      }
      const sanitizedTool = sanitizeToolName(toolRaw);
      const clampResult = clampToolName(sanitizedTool) as { name: string; truncated: boolean };
      const clampedToolName = clampResult.name;
      const idRaw = entry.id;
      const toolCallId = (() => {
        if (typeof idRaw === 'string' && idRaw.length > 0) {
          return idRaw;
        }
        if (typeof idRaw === 'number' && Number.isFinite(idRaw)) {
          return Math.trunc(idRaw).toString();
        }
        return 'call-' + (parsed.length + 1).toString();
      })();
      const parametersCandidate = entry.parameters;
      const toolParameters: Record<string, unknown> = isRecord(parametersCandidate) ? parametersCandidate : {};
      parsed.push({ id: toolCallId, name: clampedToolName, parameters: toolParameters });
    }
    return parsed.length > 0 ? parsed : undefined;
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
