import crypto from 'node:crypto';
import fs from 'node:fs';
// import os from 'node:os';
import path from 'node:path';
import zlib from 'node:zlib';

import Ajv from 'ajv';

import type { OutputFormatId } from './formats.js';
import type { SessionNode } from './session-tree.js';
import type { AIAgentSessionConfig, AIAgentResult, ConversationMessage, LogEntry, AccountingEntry, Configuration, TurnRequest, LLMAccountingEntry, MCPTool, ToolAccountingEntry, RestToolConfig, ProgressMetrics } from './types.js';

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


// empty line between import groups enforced by linter
import { validateProviders, validateMCPServers, validatePrompts } from './config.js';
import { parseFrontmatter, parsePairs, extractBodyWithoutFrontmatter } from './frontmatter.js';
import { LLMClient } from './llm-client.js';
import { buildPromptVars, applyFormat, expandVars } from './prompt-builder.js';
import { SessionProgressReporter } from './session-progress-reporter.js';
import { SessionTreeBuilder } from './session-tree.js';
import { SubAgentRegistry } from './subagent-registry.js';
import { AgentProvider } from './tools/agent-provider.js';
import { InternalToolProvider } from './tools/internal-provider.js';
import { MCPProvider } from './tools/mcp-provider.js';
import { RestProvider } from './tools/rest-provider.js';
import { ToolsOrchestrator } from './tools/tools.js';
import { formatToolRequestCompact, sanitizeToolName, truncateUtf8WithNotice, warn } from './utils.js';

// Immutable session class according to DESIGN.md
export class AIAgentSession {
  // Log identifiers (avoid duplicate string literals)
  private static readonly REMOTE_CONC_SLOT = 'agent:concurrency-slot';
  private static readonly REMOTE_CONC_HINT = 'agent:concurrency-hint';
  private static readonly REMOTE_AGENT_TOOLS = 'agent:tools';
  private static readonly FINAL_REPORT_TOOL = 'agent__final_report';
  private static readonly FINAL_REPORT_SHORT = 'final_report';
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
  private ajv?: Ajv;
  // Internal housekeeping notes
  private childConversations: { agentId?: string; toolName: string; promptPath: string; conversation: ConversationMessage[]; trace?: { originId?: string; parentId?: string; selfId?: string; callPath?: string } }[] = [];
  private readonly subAgents?: SubAgentRegistry;
  private readonly txnId: string;
  private readonly originTxnId?: string;
  private readonly parentTxnId?: string;
  private readonly callPath?: string;
  // opTree-only tracking (canonical)
  private readonly toolsOrchestrator?: ToolsOrchestrator;
  private readonly opTree: SessionTreeBuilder;
  private readonly progressReporter: SessionProgressReporter;
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

  // Concurrency gate for tool executions
  private toolSlotsInUse = 0;
  private toolWaiters: (() => void)[] = [];

  // Counters for summary
  private llmAttempts = 0;
  private llmSyntheticFailures = 0;
  private centralSizeCapHits = 0;
  private currentLlmOpId?: string;
  private systemTurnBegan = false;

  // Per-turn buffer of tool failures to synthesize messages when provider omits tool_error
  private pendingToolErrors: { id?: string; name: string; message: string; parameters?: Record<string, unknown> }[] = [];


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
    if (typeof this.sessionConfig.agentId === 'string' && this.sessionConfig.agentId.length > 0) {
      return this.sessionConfig.agentId;
    }
    return 'agent';
  }

  private getAgentIdLabel(): string {
    if (typeof this.sessionConfig.agentId === 'string' && this.sessionConfig.agentId.length > 0) {
      return this.sessionConfig.agentId;
    }
    return this.getCallPathLabel();
  }

  private getAgentDisplayName(): string {
    const label = this.getCallPathLabel();
    const parts = label.split('->').map((part) => part.trim()).filter((part) => part.length > 0);
    if (parts.length > 0) return parts[parts.length - 1];
    return label;
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
        if (o !== undefined && o.kind === 'llm' && typeof o.opId === 'string' && o.opId.length > 0) return o.opId;
      }
    } catch (e) {
      warn(`getLastLlmOpIdForTurn failed: ${e instanceof Error ? e.message : String(e)}`);
    }
    return undefined;
  }
  
  private persistSessionSnapshot(reason?: string): void {
    try {
      const sess = this.opTree.getSession();
      const originId = this.originTxnId ?? this.txnId;
      // Resolve sessions dir from config or fallback to ~/.ai-agent/sessions
      const overrideDir = this.sessionConfig.config.persistence?.sessionsDir;
      const home = process.env.HOME ?? process.env.USERPROFILE ?? '';
      const defaultBase = home ? path.join(home, '.ai-agent') : process.cwd();
      const sessDir = typeof overrideDir === 'string' && overrideDir.length > 0
        ? overrideDir
        : path.join(defaultBase, 'sessions');
      try { fs.mkdirSync(sessDir, { recursive: true }); } catch (e) { warn(`failed to create sessions dir '${sessDir}': ${e instanceof Error ? e.message : String(e)}`); }
      const json = JSON.stringify({ version: 1, reason, opTree: sess });
      const gz = zlib.gzipSync(Buffer.from(json, 'utf8'));
      const filePath = path.join(sessDir, `${originId}.json.gz`);
      const tmp = `${filePath}.tmp-${String(process.pid)}-${String(Date.now())}`;
      fs.writeFileSync(tmp, gz);
      fs.renameSync(tmp, filePath);
    } catch (e) { warn(`persistSessionSnapshot failed: ${e instanceof Error ? e.message : String(e)}`); }
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
    if (Array.isArray(sessionConfig.subAgentPaths) && sessionConfig.subAgentPaths.length > 0) {
      const reg = new SubAgentRegistry(undefined, Array.isArray(sessionConfig.ancestors) ? sessionConfig.ancestors : [], { traceLLM: sessionConfig.traceLLM, traceMCP: sessionConfig.traceMCP, verbose: sessionConfig.verbose });
      reg.load(sessionConfig.subAgentPaths);
      this.subAgents = reg;
    }
    // REST tools handled by RestProvider; no local registry here
    // Tracing context
    this.txnId = sessionConfig.trace?.selfId ?? crypto.randomUUID();
    this.originTxnId = sessionConfig.trace?.originId ?? this.txnId;
    this.parentTxnId = sessionConfig.trace?.parentId;
    this.callPath = sessionConfig.trace?.callPath ?? sessionConfig.agentId;

    // Hierarchical operation tree (Option C)
    this.opTree = new SessionTreeBuilder({ traceId: this.txnId, agentId: sessionConfig.agentId, callPath: this.callPath, sessionTitle: sessionConfig.initialTitle ?? '' });

    this.progressReporter = new SessionProgressReporter((event) => {
      try {
        this.sessionConfig.callbacks?.onProgress?.(event);
      } catch (e) {
        warn(`onProgress callback failed: ${e instanceof Error ? e.message : String(e)}`);
      }
    });
    // Begin system preflight turn (turn 0) and log init
    try {
      if (!this.systemTurnBegan) {
        this.opTree.beginTurn(0, { system: true, label: 'init' });
        this.systemTurnBegan = true;
      }
      const sysInitOp = this.opTree.beginOp(0, 'system', { label: 'init' });
      this.log({ timestamp: Date.now(), severity: 'VRB', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'agent:init', fatal: false, message: 'session initialization' }, { opId: sysInitOp });
      this.opTree.endOp(sysInitOp, 'ok');
      this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
    } catch (e) { warn(`system init logging failed: ${e instanceof Error ? e.message : String(e)}`); }

    // Tools orchestrator (MCP + REST + Internal + Subagents)
    const orch = new ToolsOrchestrator({
      toolTimeout: sessionConfig.toolTimeout,
      toolResponseMaxBytes: sessionConfig.toolResponseMaxBytes,
      maxConcurrentTools: sessionConfig.maxConcurrentTools,
      parallelToolCalls: sessionConfig.parallelToolCalls,
      traceTools: sessionConfig.traceMCP === true,
    },
    this.opTree,
    (tree: SessionNode) => { try { this.sessionConfig.callbacks?.onOpTree?.(tree); } catch (e) { warn(`onOpTree callback failed: ${e instanceof Error ? e.message : String(e)}`); } },
    (entry, opts) => { this.log(entry, opts); },
    sessionConfig.callbacks?.onAccounting,
    { agentId: sessionConfig.agentId, callPath: this.getCallPathLabel(), txnId: this.txnId },
    this.progressReporter);
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
      const enableBatch = this.sessionConfig.tools.includes('batch');
      const eo = this.sessionConfig.expectedOutput;
      const expectedJsonSchema = (eo !== undefined && eo.format === 'json') ? eo.schema : undefined;
      const internalProvider = new InternalToolProvider('agent', {
        enableBatch,
        outputFormat: this.sessionConfig.outputFormat,
        expectedOutputFormat: eo?.format === 'json' ? 'json' : undefined,
        expectedJsonSchema,
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
        setFinalReport: (p) => { this.finalReport = { status: p.status, format: p.format as 'json'|'markdown'|'markdown+mermaid'|'slack-block-kit'|'tty'|'pipe'|'sub-agent'|'text', content: p.content, content_json: p.content_json, metadata: p.metadata, ts: Date.now() }; },
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
      const execFn = async (name: string, args: Record<string, unknown>, opts?: { onChildOpTree?: (tree: SessionNode) => void; parentOpPath?: string }) => {
        const exec = await subAgents.execute(name, args, {
          config: this.sessionConfig.config,
          callbacks: this.sessionConfig.callbacks,
          targets: this.sessionConfig.targets,
          stream: this.sessionConfig.stream,
          traceLLM: this.sessionConfig.traceLLM,
          traceMCP: this.sessionConfig.traceMCP,
          verbose: this.sessionConfig.verbose,
          temperature: this.sessionConfig.temperature,
          topP: this.sessionConfig.topP,
          llmTimeout: this.sessionConfig.llmTimeout,
          toolTimeout: this.sessionConfig.toolTimeout,
          maxRetries: this.sessionConfig.maxRetries,
          maxTurns: this.sessionConfig.maxTurns,
          toolResponseMaxBytes: this.sessionConfig.toolResponseMaxBytes,
          parallelToolCalls: this.sessionConfig.parallelToolCalls,
          // propagate control signals so children can stop/abort
          abortSignal: this.abortSignal,
          stopRef: this.stopRef,
          trace: { originId: this.originTxnId, parentId: this.txnId, callPath: `${this.callPath ?? ''}->${name}` },
          onChildOpTree: opts?.onChildOpTree
        }, opts);
        // Keep child conversation list (may be reported in results for compatibility)
        this.childConversations.push({ agentId: exec.child.toolName, toolName: exec.child.toolName, promptPath: exec.child.promptPath, conversation: exec.conversation, trace: exec.trace });
        try { this.persistSessionSnapshot('subagent_finish'); } catch (e) { warn(`persist subagent snapshot failed: ${e instanceof Error ? e.message : String(e)}`); }
        return { result: exec.result, childAccounting: exec.accounting, childOpTree: exec.opTree };
      };
      // Register AgentProvider synchronously so sub-agent tools are known before first turn
      orch.register(new AgentProvider('subagent', subAgents, execFn));
    }
    // Populate mapping now (before warmup) so hasTool() sees all registered providers
    void orch.listTools();
    this.toolsOrchestrator = orch;
    // Preflight: billing ledger writability check (if configured)
    try {
      const configuredLedger = this.sessionConfig.config.persistence?.billingFile
        ?? ((): string | undefined => {
          const home = process.env.HOME ?? process.env.USERPROFILE ?? '';
          if (!home) return undefined;
          const baseDir = path.join(home, '.ai-agent');
          try { fs.mkdirSync(baseDir, { recursive: true }); } catch (e) { warn(`failed to create base dir '${baseDir}': ${e instanceof Error ? e.message : String(e)}`); }
          return path.join(baseDir, 'accounting.jsonl');
        })();
      if (typeof configuredLedger === 'string' && configuredLedger.length > 0) {
        const dir = path.dirname(configuredLedger);
        let ok = false;
        try {
          fs.mkdirSync(dir, { recursive: true });
          const tmp = path.join(dir, `.wcheck-${String(process.pid)}-${String(Date.now())}`);
          fs.writeFileSync(tmp, 'x');
          fs.unlinkSync(tmp);
          ok = true;
        } catch {
          ok = false;
        }
        if (!ok) {
          const op = this.opTree.beginOp(0, 'system', { label: 'preflight' });
          this.log({ timestamp: Date.now(), severity: 'WRN', turn: 0, subturn: 0, direction: 'response', type: 'tool', remoteIdentifier: 'agent:billing', fatal: false, message: `billing ledger not writable: ${configuredLedger}` }, { opId: op });
          this.opTree.endOp(op, 'ok');
          this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
        }
      }
    } catch (e) { warn(`preflight billing check failed: ${e instanceof Error ? e.message : String(e)}`); }


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
    const enrichedSessionConfig: AIAgentSessionConfig = {
      ...sessionConfig,
      trace: {
        selfId: sessionTxnId,
        originId: sessionConfig.trace?.originId ?? sessionTxnId,
        parentId: sessionConfig.trace?.parentId,
        callPath: sessionConfig.trace?.callPath ?? sessionConfig.agentId,
      },
    };

    // Apply sensible defaults for runtime concurrency if undefined
    if (typeof enrichedSessionConfig.maxConcurrentTools !== 'number' || !Number.isFinite(enrichedSessionConfig.maxConcurrentTools)) {
      enrichedSessionConfig.maxConcurrentTools = 3;
    }

    // External log relay placeholder; bound after session instantiation to tree
    let externalLogRelay: (entry: LogEntry) => void = (e: LogEntry) => { void e; };
    // Wrap onLog to inject trace fields and route only through the execution tree (which fans out to callbacks)
    const wrapLog = (_fn?: (entry: LogEntry) => void) => (entry: LogEntry): void => {
      const enriched: LogEntry = {
        ...entry,
        agentId: enrichedSessionConfig.agentId,
        callPath: enrichedSessionConfig.trace?.callPath ?? enrichedSessionConfig.agentId,
        txnId: enrichedSessionConfig.trace?.selfId,
        parentTxnId: enrichedSessionConfig.trace?.parentId,
        originTxnId: enrichedSessionConfig.trace?.originId,
      };
      try { externalLogRelay(enriched); } catch (e) { warn(`external log relay failed: ${e instanceof Error ? e.message : String(e)}`); }
    };

    // Create session-owned MCP client
    // Create session-owned LLM client
    const llmClient = new LLMClient(enrichedSessionConfig.config.providers, {
      traceLLM: enrichedSessionConfig.traceLLM,
      onLog: wrapLog(enrichedSessionConfig.callbacks?.onLog),
      pricing: enrichedSessionConfig.config.pricing
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
      callPath: this.callPath,
      txnId: this.txnId,
      parentTxnId: this.parentTxnId,
      originTxnId: this.originTxnId,
      ...(this.sessionConfig.maxTurns !== undefined ? { 'max_turns': this.sessionConfig.maxTurns } : {}),
      ...(planned !== undefined ? { 'max_subturns': planned } : {}),
      ...entry,
    };
    logs.push(enriched);
    // Anchor log to opTree when an opId is known, else best-effort for LLM logs
    try {
      const explicitOp = opts?.opId;
      if (typeof explicitOp === 'string' && explicitOp.length > 0) {
        this.opTree.appendLog(explicitOp, enriched);
        this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
      } else if (enriched.type === 'llm') {
        const active = this.currentLlmOpId;
        const targetOp = (typeof active === 'string' && active.length > 0)
          ? active
          : this.getLastLlmOpIdForTurn(typeof enriched.turn === 'number' ? enriched.turn : this.currentTurn);
        if (typeof targetOp === 'string' && targetOp.length > 0) {
          this.opTree.appendLog(targetOp, enriched);
          this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
        }
      }
    } catch (e) { warn(`addLog opTree anchor failed: ${e instanceof Error ? e.message : String(e)}`); }
    // Single place try/catch for external sinks
    try { this.sessionConfig.callbacks?.onLog?.(enriched); } catch (e) { warn(`onLog callback failed: ${e instanceof Error ? e.message : String(e)}`); }
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

  // Main execution method - returns immutable result
  async run(): Promise<AIAgentResult> {
    let currentConversation = [...this.conversation];
    let currentLogs = [...this.logs];
    let currentAccounting = [...this.accounting];
    let currentTurn = this.currentTurn;

    this.progressReporter.agentStarted({
      callPath: this.getCallPathLabel(),
      agentId: this.getAgentIdLabel(),
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
          maxConcurrentTools: this.sessionConfig.maxConcurrentTools,
          stream: this.sessionConfig.stream,
          parallelToolCalls: this.sessionConfig.parallelToolCalls,
          traceLLM: this.sessionConfig.traceLLM,
          traceMCP: this.sessionConfig.traceMCP,
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
      const resultShape = {
        success: result.success,
        error: result.error,
        conversation: result.conversation,
        logs: flat.logs,
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
        this.log({ timestamp: Date.now(), severity: 'VRB', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'agent:fin', fatal: false, message: 'session finalization' }, { opId: finOp });
        this.opTree.endOp(finOp, 'ok');
        // End system turn
        this.opTree.endTurn(0);
        this.opTree.endSession(result.success, result.error);
      } catch (e) { warn(`endSession failed: ${e instanceof Error ? e.message : String(e)}`); }
      this.emitAgentCompletion(result.success, result.error);
      // Phase B/C: persist final session and accounting ledger
      try {
        this.persistSessionSnapshot('final');
        const configuredLedger = this.sessionConfig.config.persistence?.billingFile
          ?? ((): string | undefined => {
            const home = process.env.HOME ?? process.env.USERPROFILE ?? '';
            if (!home) return undefined;
            const baseDir = path.join(home, '.ai-agent');
            try { fs.mkdirSync(baseDir, { recursive: true }); } catch (e) { warn(`failed to create base dir '${baseDir}': ${e instanceof Error ? e.message : String(e)}`); }
            return path.join(baseDir, 'accounting.jsonl');
          })();
        if (typeof configuredLedger === 'string' && configuredLedger.length > 0) {
          const lines = (resultShape.accounting).map((a) => JSON.stringify(a));
          if (lines.length > 0) fs.appendFileSync(configuredLedger, lines.join('\n') + '\n');
        }
      } catch (e) { warn(`final persistence failed: ${e instanceof Error ? e.message : String(e)}`); }
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
      const failShape = {
        success: false,
        error: message,
        conversation: currentConversation,
        logs: flatFail.logs,
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
      return failShape;
    } finally {
      await this.toolsOrchestrator?.cleanup();
    }
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
    
    // Track the last turn where we showed thinking header
    let lastShownThinkingHeaderTurn = -1;

    let maxTurns = this.sessionConfig.maxTurns ?? 10;
    const maxRetries = this.sessionConfig.maxRetries ?? 3; // GLOBAL attempts cap per turn
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

          try {
            // Build per-attempt conversation with optional guidance injection
            let attemptConversation = [...conversation];
            // On the last allowed attempt within this turn, nudge the model to use tools (not progress_report)
            if ((attempts === maxRetries - 1) && currentTurn < (maxTurns - 1)) {
              attemptConversation.push({
                role: 'user',
                content: `Reminder: do not end with plain text. Use an available tool (excluding \`agent__progress_report\`) to make progress. When ready to conclude, call the tool \`${AIAgentSession.FINAL_REPORT_TOOL}\` to provide the final answer.`
              });
            }
            // Do not inject final-turn user message here to avoid duplication.
            // Providers append a single, standardized final-turn instruction.

            const isFinalTurn = currentTurn === maxTurns;
            if (isFinalTurn && !finalTurnWarnLogged) {
              const warn: LogEntry = {
                timestamp: Date.now(),
                severity: 'WRN',
                turn: currentTurn,
                subturn: 0,
                direction: 'request',
                type: 'llm',
                remoteIdentifier: 'agent:final-turn',
                fatal: false,
                message: `Final turn detected: restricting tools to \`${AIAgentSession.FINAL_REPORT_TOOL}\` and injecting finalization instruction.`
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
            const turnResult = await this.executeSingleTurn(
              attemptConversation,
              provider,
              model,
              isFinalTurn, // isFinalTurn
              currentTurn,
              logs,
              accounting,
              lastShownThinkingHeaderTurn
            );
            
            // Update tracking if thinking was shown
            if (turnResult.shownThinking) {
              lastShownThinkingHeaderTurn = currentTurn;
            }

            // Emit WRN for unknown tool calls that the AI SDK could not execute (name not in ToolSet)
            try {
              // Consider only internal tool set as always known
              // Internal tools always available; include optional batch tool if enabled for this session
        const internal = new Set<string>(['agent__progress_report', AIAgentSession.FINAL_REPORT_TOOL]);
              if (this.sessionConfig.tools.includes('batch')) internal.add('agent__batch');
              const normalizeTool = (n: string) => n.replace(/^<\|[^|]+\|>/, '').trim();
              const lastAssistant = turnResult.messages.filter((m) => m.role === 'assistant');
              const assistantMsg = lastAssistant.length > 0 ? lastAssistant[lastAssistant.length - 1] : undefined;
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
              const computeCost = (): { costUsd?: number } => {
                try {
                  const pricing = (this.sessionConfig.config.pricing ?? {}) as Partial<Record<string, Partial<Record<string, { unit?: 'per_1k'|'per_1m'; currency?: 'USD'; prompt?: number; completion?: number; cacheRead?: number; cacheWrite?: number }>>>>;
                  const effProvider = provider === 'openrouter' ? (this.llmClient.getLastActualRouting().provider ?? provider) : provider;
                  const effModel = provider === 'openrouter' ? (this.llmClient.getLastActualRouting().model ?? model) : model;
                  const provTable = pricing[effProvider];
                  const modelTable = provTable !== undefined ? provTable[effModel] : undefined;
                  if (modelTable === undefined) return {};
                  const denom = (modelTable.unit === 'per_1k') ? 1000 : 1_000_000;
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
                actualProvider: provider === 'openrouter' ? this.llmClient.getLastActualRouting().provider : undefined,
                actualModel: provider === 'openrouter' ? this.llmClient.getLastActualRouting().model : undefined,
                costUsd: provider === 'openrouter' ? (this.llmClient.getLastCostInfo().costUsd ?? computed.costUsd) : computed.costUsd,
                upstreamInferenceCostUsd: provider === 'openrouter' ? this.llmClient.getLastCostInfo().upstreamInferenceCostUsd : undefined,
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
                const lastAssistant = [...turnResult.messages].filter((m) => m.role === 'assistant').pop();
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
              // Synthetic error: success with content but no tools and no final_report → retry this turn
              if (this.finalReport === undefined) {
          const lastAssistant = [...turnResult.messages].filter(m => m.role === 'assistant').pop();
          const hasTools = (lastAssistant?.toolCalls?.length ?? 0) > 0;
          const hasText = (lastAssistant?.content.trim().length ?? 0) > 0;
          if (!hasTools && hasText) {
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
                lastError = 'invalid_response: content_without_tools_or_final';
                if (cycleComplete) {
                  rateLimitedInCycle = 0;
                  maxRateLimitWaitMs = 0;
                }
                // do not append these messages; try next provider/model in same turn
                continue;
              }
            }

        // (removed misplaced orchestrator path; correct handling is below in toolExecutor)

        // Add new messages to conversation
        conversation.push(...turnResult.messages);
        // Deterministic finalization: if final_report has been set, finish now
        if (this.finalReport !== undefined) {
          const fr = this.finalReport;
          // Validate final JSON against schema if provided (no output to onOutput here)
          if (fr.format === 'json') {
            // Validate JSON against frontmatter schema if available
            const schema = this.sessionConfig.expectedOutput?.schema;
            if (schema !== undefined && fr.content_json !== undefined) {
              try {
                this.ajv = this.ajv ?? new Ajv({ allErrors: true, strict: false });
                const validate = this.ajv.compile(schema);
                const valid = validate(fr.content_json);
                if (!valid) {
                  const errs = (validate.errors ?? []).map((e) => {
                    const path = typeof e.instancePath === 'string' && e.instancePath.length > 0
                      ? e.instancePath
                      : (typeof e.schemaPath === 'string' ? e.schemaPath : '');
                    const msg = typeof e.message === 'string' ? e.message : '';
                    return `${path} ${msg}`.trim();
                  }).join('; ');
                  const warn: LogEntry = {
                    timestamp: Date.now(),
                    severity: 'WRN',
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response',
                    type: 'llm',
                    remoteIdentifier: 'agent:ajv',
                    fatal: false,
                    message: `final_report JSON does not match schema: ${errs}`
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

          // Log successful exit
        this.logExit('EXIT-FINAL-ANSWER', `Final report received (${AIAgentSession.FINAL_REPORT_TOOL}), session complete`, currentTurn);

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
              
              // Debug logging
              if (this.sessionConfig.verbose === true) {
                const hasToolResults = turnResult.messages.some((m: ConversationMessage) => m.role === 'tool');
                const hasContent = turnResult.hasContent ?? (() => {
                  const lastAssistant = [...turnResult.messages].filter(m => m.role === 'assistant').pop();
                  return (lastAssistant?.content.trim().length ?? 0) > 0;
                })();
                const hasReasoning = turnResult.hasReasoning ?? false;
                const hasToolCallsStr = String(turnResult.status.hasToolCalls);
                const hasToolResultsStr = String(hasToolResults);
                const finalAnswerStr = String(turnResult.status.finalAnswer);
                const hasReasoningStr = String(hasReasoning);
                const hasContentStr = String(hasContent);
                const responseLenStr = String(turnResult.response?.length ?? 0);
                const messagesLenStr = String(turnResult.messages.length);
                const debugEntry: LogEntry = {
                  timestamp: Date.now(),
                  severity: 'VRB',
                  turn: currentTurn,
                  subturn: 0,
                  direction: 'response',
                  type: 'llm',
                  remoteIdentifier: 'debug',
                  fatal: false,
                  message: `Turn result: hasToolCalls=${hasToolCallsStr}, hasToolResults=${hasToolResultsStr}, finalAnswer=${finalAnswerStr}, hasReasoning=${hasReasoningStr}, hasContent=${hasContentStr}, response length=${responseLenStr}, messages=${messagesLenStr}`
                };
                this.log(debugEntry);
                
                // Debug: show the actual messages content
                if (process.env.DEBUG === 'true') {
                  // eslint-disable-next-line functional/no-loop-statements
                  for (const msg of turnResult.messages) {
                    console.error(`[DEBUG] Message role=${msg.role}, content length=${String(msg.content.length)}, hasToolCalls=${String(msg.toolCalls !== undefined)}`);
                    if (msg.content.length > 0) {
                      console.error(`[DEBUG] Content: ${msg.content.substring(0, 200)}`);
                    }
                  }
                }
              }
              // Capture planned subturns for this turn (enriches subsequent logs)
              try {
                const lastAssistant = [...turnResult.messages].filter((m: ConversationMessage) => m.role === 'assistant').pop();
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
                this.sessionConfig.callbacks?.onOutput?.(turnResult.response);
                // Add newline if response doesn't end with one
                if (!turnResult.response.endsWith('\n')) {
                  this.sessionConfig.callbacks?.onOutput?.('\n');
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
                // Log warning and retry this turn on another provider/model
                const routed = this.llmClient.getLastActualRouting();
                const remoteId = (provider === 'openrouter' && routed.provider !== undefined)
                  ? `${provider}/${routed.provider}:${model}`
                  : `${provider}:${model}`;
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
                // do not mark turnSuccessful; continue retry loop
                continue;
              }
              
              if (turnResult.status.finalAnswer) {
                // Check if final_report was actually successful by checking if finalReport was set
                // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
                const finalReportSucceeded = this.finalReport !== undefined;

                if (finalReportSucceeded) {
                  // Final report succeeded, we're done
                  turnSuccessful = true;
                  break;
                }

                // Final report was called but failed
                if (!isFinalTurn) {
                  // On non-final turns, move to next turn to retry
                  turnSuccessful = true;
                  break;
                }
                // On final turn with failed final_report, continue retry loop
                lastError = 'final_report_failed';
              } else {
                // If this is the final turn and we still don't have a final answer,
                // retry within this turn across provider/model pairs up to maxRetries.
                if (isFinalTurn) {
                  const routed = this.llmClient.getLastActualRouting();
                  const remoteId = (provider === 'openrouter' && routed.provider !== undefined)
                    ? `${provider}/${routed.provider}:${model}`
                    : `${provider}:${model}`;
                  const warnEntry: LogEntry = {
                    timestamp: Date.now(),
                    severity: 'WRN',
                    turn: currentTurn,
                    subturn: 0,
                    direction: 'response',
                    type: 'llm',
                  remoteIdentifier: remoteId,
                  fatal: false,
                  message: 'Final turn did not produce final_report; retrying with next provider/model in this turn.'
                };
                this.log(warnEntry);
                // Continue attempts loop (do not mark successful)
                if (cycleComplete) {
                  rateLimitedInCycle = 0;
                  maxRateLimitWaitMs = 0;
                }
                continue;
              }
              // Non-final turns: proceed to next turn; tools already executed if present
              turnSuccessful = true;
              break;
            }
          } else {
            // Handle various error types
            lastError = 'message' in turnResult.status ? turnResult.status.message : turnResult.status.type;
            
            // Some errors should skip this provider entirely
            if (turnResult.status.type === 'auth_error' || turnResult.status.type === 'quota_exceeded') {
              break; // Skip to next provider/model pair
            }
            if (turnResult.status.type === 'rate_limit') {
              rateLimitedInCycle += 1;
              const routed = this.llmClient.getLastActualRouting();
              const remoteId = (provider === 'openrouter' && routed.provider !== undefined)
                ? `${provider}/${routed.provider}:${model}`
                : `${provider}:${model}`;
              const RATE_LIMIT_MIN_WAIT_MS = 1_000;
              const RATE_LIMIT_MAX_WAIT_MS = 60_000;
              const fallbackWait = Math.min(Math.max(attempts * 1_000, RATE_LIMIT_MIN_WAIT_MS), RATE_LIMIT_MAX_WAIT_MS);
              const providerWaitRaw = turnResult.status.retryAfterMs;
              const providerWaitMs = typeof providerWaitRaw === 'number' && Number.isFinite(providerWaitRaw)
                ? providerWaitRaw
                : undefined;
              const effectiveWaitMs = Math.min(
                Math.max(Math.round(providerWaitMs ?? fallbackWait), RATE_LIMIT_MIN_WAIT_MS),
                RATE_LIMIT_MAX_WAIT_MS
              );
              maxRateLimitWaitMs = Math.max(maxRateLimitWaitMs, effectiveWaitMs);
              const warnEntry: LogEntry = {
                timestamp: Date.now(),
                severity: 'WRN',
                turn: currentTurn,
                subturn: 0,
                direction: 'response',
                type: 'llm',
                remoteIdentifier: remoteId,
                fatal: false,
                message: `Rate limited; suggested wait ${String(effectiveWaitMs)}ms before retry.`
              };
              this.log(warnEntry);

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
            
            // Continue trying other providers
            if (cycleComplete) {
              rateLimitedInCycle = 0;
              maxRateLimitWaitMs = 0;
            }
            continue;
          }
        } catch (error) {
          lastError = error instanceof Error ? error.message : String(error);
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
            continue;
        }
      }

      if (!turnSuccessful) {
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
      this.log({ timestamp: Date.now(), severity: 'VRB', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'agent:fin', fatal: false, message: 'session finalization (uncaught)' }, { opId: finOp });
      this.opTree.endOp(finOp, 'ok');
      this.opTree.endTurn(0);
      this.opTree.endSession(false, errMsg);
    } catch (e) { warn(`endSession failed: ${e instanceof Error ? e.message : String(e)}`); }
    return { success: false, error: errMsg, conversation, logs, accounting };
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
      this.log({ timestamp: Date.now(), severity: 'VRB', turn: 0, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'agent:fin', fatal: false, message: 'session finalization' }, { opId: finOp });
      this.opTree.endOp(finOp, 'ok');
      this.opTree.endTurn(0);
      this.opTree.endSession(true);
    } catch (e) { warn(`endSession failed: ${e instanceof Error ? e.message : String(e)}`); }
    return { success: true, conversation, logs, accounting } as AIAgentResult;
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

      const msg = `requests=${String(llmRequests)} failed=${String(llmFailures)}, tokens prompt=${String(tokIn)} output=${String(tokOut)} cacheR=${String(tokCacheRead)} cacheW=${String(tokCacheWrite)} total=${String(tokTotal)}, cost total=$${totalCost.toFixed(5)} upstream=$${totalUpstreamCost.toFixed(5)}, latency sum=${String(llmLatencySum)}ms avg=${String(llmLatencyAvg)}ms, providers/models: ${pairsStr.length > 0 ? pairsStr : 'none'}`;
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

  
  private async acquireToolSlot(): Promise<number> {
    const cap = Math.max(1, this.sessionConfig.maxConcurrentTools ?? 1);
    const effectiveCap = (this.sessionConfig.parallelToolCalls === false) ? 1 : cap;
    if (this.toolSlotsInUse < effectiveCap) {
      this.toolSlotsInUse += 1;
    // No per-slot verbose log; slot is shown on MCP request lines via slot=N
      return this.toolSlotsInUse;
    }
    await new Promise<void>((resolve) => { this.toolWaiters.push(resolve); });
    this.toolSlotsInUse += 1;
    // No per-slot verbose log; slot is shown on MCP request lines via slot=N
    return this.toolSlotsInUse;
  }

  private releaseToolSlot(): void {
    this.toolSlotsInUse = Math.max(0, this.toolSlotsInUse - 1);
    const next = this.toolWaiters.shift();
    // No per-slot verbose log
    if (next !== undefined) { try { next(); } catch { } }
  }

  private async executeSingleTurn(
    conversation: ConversationMessage[],
    provider: string,
    model: string,
    isFinalTurn: boolean,
    currentTurn: number,
    logs: LogEntry[],
    accounting: AccountingEntry[],
    lastShownThinkingHeaderTurn: number
  ) {
    // no spans; opTree is canonical
    // Expose provider tools (which includes internal tools from InternalToolProvider)
    const allTools = [
      ...(this.toolsOrchestrator?.listTools() ?? [])
    ];

    const filteredSelection = this.filterToolsForProvider(allTools, provider);
    const availableTools = filteredSelection.tools;
    const allowedToolNames = filteredSelection.allowedNames;

    
    // Track if we've shown thinking for this turn
    let shownThinking = false;
    
    // Create tool executor function that delegates to MCP client with accounting
    let subturnCounter = 0;
    let incompleteFinalReportDetected = false;
    const maxToolCallsPerTurn = Math.max(1, this.sessionConfig.maxToolCallsPerTurn ?? 10);
    const toolExecutor = async (toolName: string, parameters: Record<string, unknown>): Promise<string> => {
      if (this.stopRef?.stopping === true) throw new Error('stop_requested');
      if (this.canceled) throw new Error('canceled');
      // Advance subturn for each tool call within this turn
      subturnCounter += 1;
      if (subturnCounter > maxToolCallsPerTurn) {
        const msg = `Tool calls per turn exceeded: limit=${String(maxToolCallsPerTurn)}. Switch strategy: avoid further tool calls this turn; either summarize progress or call ${AIAgentSession.FINAL_REPORT_TOOL} to conclude.`;
        const warn: LogEntry = { timestamp: Date.now(), severity: 'WRN', turn: currentTurn, subturn: subturnCounter, direction: 'response', type: 'tool', remoteIdentifier: 'agent:limits', fatal: false, message: msg };
        this.log(warn);
        throw new Error('tool_calls_per_turn_limit_exceeded');
      }
      // Orchestrator receives ctx turn/subturn per call

      // (no local preview helpers needed)

      // Normalize tool name to strip router/provider wrappers like <|constrain|>
      const effectiveToolName = sanitizeToolName(toolName);
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
          // Ensure we always return a valid string
          return managed.result || '(no output)';
        }

        // Unknown tool after all paths
        {
          const req = formatToolRequestCompact(toolName, parameters);
          const warn: LogEntry = { timestamp: Date.now(), severity: 'WRN', turn: currentTurn, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'assistant:tool', fatal: false, message: `Unknown tool requested: ${req}` };
          this.log(warn);
          throw new Error(`No server found for tool: ${effectiveToolName}`);
        }
      } catch (error) {
        const latency = Date.now() - startTime;
        const errorMsg = error instanceof Error ? error.message : String(error);

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
        this.releaseToolSlot();
        return `(tool failed: ${errorMsg})`;
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

    const request: TurnRequest = {
      messages: conversation,
      provider,
      model,
      tools: availableTools,
      toolExecutor,
      temperature: effectiveTemperature,
      topP: effectiveTopP,
      maxOutputTokens: this.sessionConfig.maxOutputTokens,
      repeatPenalty: this.sessionConfig.repeatPenalty,
      parallelToolCalls: this.sessionConfig.parallelToolCalls,
      stream: this.sessionConfig.stream,
      isFinalTurn,
      llmTimeout: this.sessionConfig.llmTimeout,
      abortSignal: this.abortSignal,
      onChunk: (chunk: string, type: 'content' | 'thinking') => {
        if (type === 'content' && this.sessionConfig.callbacks?.onOutput !== undefined) {
          this.sessionConfig.callbacks.onOutput(chunk);
        } else if (type === 'thinking') {
          // Check if we need to show the thinking header for this turn
          if (lastShownThinkingHeaderTurn !== currentTurn) {
            // Show the THK header (without the chunk content)
            const thinkingHeader: LogEntry = {
              timestamp: Date.now(),
              severity: 'THK',
              turn: currentTurn,
              subturn: 0,
              direction: 'response',
              type: 'llm',
              remoteIdentifier: 'thinking',
              fatal: false,
              message: ''  // Header only, no content
            };
            this.log(thinkingHeader);
            shownThinking = true;
            // Update tracking to prevent duplicate headers
            lastShownThinkingHeaderTurn = currentTurn;
          }
          // Always stream the thinking text (including the first chunk)
          this.sessionConfig.callbacks?.onThinking?.(chunk);
          try {
            if (typeof this.currentLlmOpId === 'string') this.opTree.appendReasoningChunk(this.currentLlmOpId, chunk);
          } catch (e) { warn(`appendReasoningChunk failed: ${e instanceof Error ? e.message : String(e)}`); }
        }
      }
    };

    try {
      const result = await this.llmClient.executeTurn(request);
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
    interface BatchCall { id: string; tool: string; args: Record<string, unknown> }
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
        const args = (isPlainObject(c.args) ? c.args : {});
        return { id, tool, args };
      });
      return { calls };
    })();

    if (bi === undefined || bi.calls.some((c) => c.id.length === 0 || c.tool.length === 0)) {
      // Warn once for invalid batch input
      try {
        const warn: LogEntry = { timestamp: Date.now(), severity: 'WRN', turn: currentTurn, subturn: 0, direction: RESP_DIR, type: LLM_TYPE, remoteIdentifier: BATCH_REMOTE_ID, fatal: false, message: 'Invalid batch input: each call requires id, tool, args' };
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
      throw new Error('invalid_batch_input: calls[] requires id, tool, args');
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
        const progress = typeof (c.args.progress) === 'string' ? c.args.progress : '';
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
        const orchestrator = (this.toolsOrchestrator as unknown) as { executeWithManagement: (t: string, a: Record<string, unknown>, ctx: { turn: number; subturn: number }, opts?: { timeoutMs?: number }) => Promise<{ result: string; latency: number }> };
        const managed = await orchestrator.executeWithManagement(c.tool, c.args, { turn: currentTurn, subturn: 0 }, { timeoutMs: this.sessionConfig.toolTimeout });
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
