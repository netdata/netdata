import crypto from 'node:crypto';
import fs from 'node:fs';
// import os from 'node:os';
import path from 'node:path';

import Ajv from 'ajv';

import type { OutputFormatId } from './formats.js';
import type { AIAgentSessionConfig, AIAgentResult, ConversationMessage, LogEntry, AccountingEntry, Configuration, TurnRequest, MCPTool, LLMAccountingEntry, ToolAccountingEntry, RestToolConfig } from './types.js';

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
import { ExecutionTree } from './execution-tree.js';
import { parseFrontmatter, parsePairs, extractBodyWithoutFrontmatter } from './frontmatter.js';
import { LLMClient } from './llm-client.js';
import { buildPromptVars, applyFormat, expandVars } from './prompt-builder.js';
import { SessionTreeBuilder } from './session-tree.js';
import { SubAgentRegistry } from './subagent-registry.js';
import { AgentProvider } from './tools/agent-provider.js';
import { InternalToolProvider } from './tools/internal-provider.js';
import { MCPProvider } from './tools/mcp-provider.js';
import { RestProvider } from './tools/rest-provider.js';
import { ToolsOrchestrator } from './tools/tools.js';
import { formatToolRequestCompact, truncateUtf8WithNotice } from './utils.js';

// Immutable session class according to DESIGN.md
export class AIAgentSession {
  // Log identifiers (avoid duplicate string literals)
  private static readonly REMOTE_CONC_SLOT = 'agent:concurrency-slot';
  private static readonly REMOTE_CONC_HINT = 'agent:concurrency-hint';
  readonly config: Configuration;
  readonly conversation: ConversationMessage[];
  readonly logs: LogEntry[];
  readonly accounting: AccountingEntry[];
  readonly success: boolean;
  readonly error?: string;
  readonly currentTurn: number;
  
  private readonly llmClient: LLMClient;
  private readonly sessionConfig: AIAgentSessionConfig;
  private readonly abortSignal?: AbortSignal;
  private canceled = false;
  private readonly stopRef?: { stopping: boolean };
  private ajv?: Ajv;
  // Internal housekeeping notes
  private internalNotes: { text: string; tags?: string[]; ts: number }[] = [];
  private childConversations: { agentId?: string; toolName: string; promptPath: string; conversation: ConversationMessage[]; trace?: { originId?: string; parentId?: string; selfId?: string; callPath?: string } }[] = [];
  private readonly subAgents?: SubAgentRegistry;
  private readonly txnId: string;
  private readonly originTxnId?: string;
  private readonly parentTxnId?: string;
  private readonly callPath?: string;
  private readonly tree: ExecutionTree;
  private readonly toolsOrchestrator?: ToolsOrchestrator;
  private readonly opTree: SessionTreeBuilder;
  // Per-turn planned subturns (tool call count) discovered when LLM yields toolCalls
  private plannedSubturns: Map<number, number> = new Map<number, number>();
  private resolvedFormat?: OutputFormatId;
  private resolvedFormatDescription?: string;
  private resolvedUserPrompt?: string;
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
    this.currentTurn = currentTurn;
    this.llmClient = llmClient;
    this.sessionConfig = sessionConfig;
    this.abortSignal = sessionConfig.abortSignal;
    this.stopRef = sessionConfig.stopRef;
    try {
    if (this.abortSignal !== undefined) {
      if (this.abortSignal.aborted) {
        this.canceled = true;
        try { this.toolsOrchestrator?.cancel(); } catch { /* ignore */ }
      } else {
        this.abortSignal.addEventListener('abort', () => { this.canceled = true; try { this.toolsOrchestrator?.cancel(); } catch { /* ignore */ } }, { once: true });
      }
    }
    } catch { /* ignore */ }
    // Initialize sub-agents registry if provided
    if (Array.isArray(sessionConfig.subAgentPaths) && sessionConfig.subAgentPaths.length > 0) {
      const reg = new SubAgentRegistry(undefined, [], { traceLLM: sessionConfig.traceLLM, traceMCP: sessionConfig.traceMCP, verbose: sessionConfig.verbose });
      reg.load(sessionConfig.subAgentPaths);
      this.subAgents = reg;
    }
    // REST tools handled by RestProvider; no local registry here
    // Tracing context
    this.txnId = sessionConfig.trace?.selfId ?? crypto.randomUUID();
    this.originTxnId = sessionConfig.trace?.originId ?? this.txnId;
    this.parentTxnId = sessionConfig.trace?.parentId;
    this.callPath = sessionConfig.trace?.callPath ?? sessionConfig.agentId;

    // Central execution tree (source of truth for logs/accounting)
    this.tree = new ExecutionTree(
      { sessionId: this.txnId, agentId: sessionConfig.agentId, callPath: this.callPath, maxTurns: sessionConfig.maxTurns },
      { onLog: sessionConfig.callbacks?.onLog, onAccounting: sessionConfig.callbacks?.onAccounting }
    );
    // Hierarchical operation tree (Option C)
    this.opTree = new SessionTreeBuilder({ traceId: this.txnId, agentId: sessionConfig.agentId, callPath: this.callPath });

    // Tools orchestrator (MCP + REST + Internal + Subagents)
    const orch = new ToolsOrchestrator(this.tree, {
      toolTimeout: sessionConfig.toolTimeout,
      toolResponseMaxBytes: sessionConfig.toolResponseMaxBytes,
      maxConcurrentTools: sessionConfig.maxConcurrentTools,
      parallelToolCalls: sessionConfig.parallelToolCalls,
      traceTools: sessionConfig.traceMCP === true,
    }, this.opTree, (tree) => { try { this.sessionConfig.callbacks?.onOpTree?.(tree); } catch { /* ignore */ } });
    orch.register(new MCPProvider('mcp', sessionConfig.config.mcpServers, { trace: sessionConfig.traceMCP, verbose: sessionConfig.verbose, requestTimeoutMs: sessionConfig.toolTimeout, onLog: (e) => { try { this.addLog(this.logs, e); } catch { /* ignore */ } } }));
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
        expectedJsonSchema,
        appendNotes: (text: string, tags?: string[]) => {
          const t = text.trim();
          if (t.length > 0) {
            const ts = Date.now();
            this.internalNotes.push({ text: t, tags, ts });
            // Log note in VRB, bold, without truncation
            const noteMsg = (() => {
              const tagStr = Array.isArray(tags) && tags.length > 0 ? ` [${tags.join(', ')}]` : '';
              return `${t}${tagStr}`;
            })();
            const entry: LogEntry = {
              timestamp: ts,
              severity: 'VRB',
              turn: this.currentTurn,
              subturn: 0,
              direction: 'response',
              type: 'tool',
              remoteIdentifier: 'agent:note',
              fatal: false,
              bold: true,
              message: noteMsg,
            };
            this.addLog(this.logs, entry);
          }
        },
        setTitle: (title: string, emoji?: string) => {
          const clean = title.trim();
          if (clean.length === 0) return;
          this.sessionTitle = { title: clean, emoji, ts: Date.now() };
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
          this.addLog(this.logs, entry);
        },
        setFinalReport: (p) => { this.finalReport = { status: p.status, format: p.format as 'json'|'markdown'|'markdown+mermaid'|'slack-block-kit'|'tty'|'pipe'|'sub-agent'|'text', content: p.content, content_json: p.content_json, metadata: p.metadata, ts: Date.now() }; },
        orchestrator: orch,
        toolTimeoutMs: sessionConfig.toolTimeout
      });
      orch.register(internalProvider);
    }
    if (this.subAgents !== undefined) {
      const subAgents = this.subAgents;
      const execFn = async (name: string, args: Record<string, unknown>) => {
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
          trace: { originId: this.originTxnId, parentId: this.txnId, callPath: `${this.callPath ?? ''}->${name}` }
        });
        // Keep child conversation list for save-all
        this.childConversations.push({ agentId: exec.child.toolName, toolName: exec.child.toolName, promptPath: exec.child.promptPath, conversation: exec.conversation, trace: exec.trace });
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
        this.addLog(this.logs, entry);
      }
    } catch { /* ignore initialTitle issues */ }
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
      try { externalLogRelay(enriched); } catch { /* ignore tree relay errors */ }
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
    // Bind external log relay to session's execution tree
    externalLogRelay = (e: LogEntry) => { try { sess.recordExternalLog(e); } catch { /* noop */ } };
    return sess;
  }

  // Helper method to log exit with proper format
  private logExit(
    exitCode: ExitCode,
    reason: string,
    turn: number,
    logs: LogEntry[]
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
    this.addLog(logs, logEntry);
  }

  // Centralized helper to ensure all logs carry trace fields
  private addLog(logs: LogEntry[], entry: LogEntry): void {
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
    try { this.tree.recordLog(enriched); } catch { /* ignore tree errors */ }
    // Do not call callbacks directly here; ExecutionTree handles fan-out.
  }

  // Relay for logs originating outside AIAgentSession (LLM/MCP internals)
  recordExternalLog(entry: LogEntry): void {
    try { this.tree.recordLog(entry); } catch { /* ignore */ }
  }

  // Optional: expose a snapshot for progress UIs or web monitoring
  getExecutionSnapshot(): { logs: number; accounting: number } {
    const snap = this.tree.getSnapshot();
    return { logs: snap.counters.logs, accounting: snap.counters.accounting };
  }

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
      this.addLog(logs, warn);
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

    try {
      // Start execution session in the tree
      try { this.tree.startSession(); } catch { /* ignore */ }
      // Warmup providers (ensures MCP tools/instructions are available) and refresh mapping
      try { await this.toolsOrchestrator?.warmup(); } catch { /* ignore warmup errors */ }

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
          return { providers: prov, mcpServers: srvs, accountingFile: this.config.accounting?.file };
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
        this.addLog(currentLogs, entry);

        // Emit a concise concurrency hint for quick visibility
        const cap = Math.max(1, this.sessionConfig.maxConcurrentTools ?? 1);
        const effectiveCap = (this.sessionConfig.parallelToolCalls === false) ? 1 : cap;
        const concEntry: LogEntry = {
          timestamp: Date.now(),
          severity: 'VRB',
          turn: currentTurn,
          subturn: 0,
          direction: 'response',
          type: 'llm',
          remoteIdentifier: AIAgentSession.REMOTE_CONC_HINT,
          fatal: false,
          message: `tool concurrency: cap=${String(effectiveCap)} (parallelToolCalls=${String(this.sessionConfig.parallelToolCalls === true)}, maxConcurrentTools=${String(this.sessionConfig.maxConcurrentTools)})`,
        };
        this.addLog(currentLogs, concEntry);
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
          remoteIdentifier: 'agent:tools',
          fatal: false,
          message: `tools: mcp=${String(mcpToolNames.length)} [${mcpToolNames.join(', ')}]; rest=${String(restToolNames.length)} [${restToolNames.join(', ')}]; subagents=${String(subAgentTools.length)} [${subAgentTools.join(', ')}]`
        };
        this.addLog(currentLogs, banner);
      } catch { /* ignore banner errors */ }

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
          } catch { /* ignore per-child read/parse errors */ }
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
          this.addLog(currentLogs, warn);
        }
      } catch { /* ignore pricing check errors */ }

      // Resolve FORMAT centrally from configuration (with sane fallback)
      const { describeFormat } = await import('./formats.js');
      const resolvedFmt: OutputFormatId = this.sessionConfig.outputFormat;
      const fmtDesc = describeFormat(resolvedFmt);
      this.resolvedFormat = resolvedFmt;
      this.resolvedFormatDescription = fmtDesc;

      // Apply ${FORMAT} replacement first, then expand variables
      // Safety: strip any shebang/frontmatter from system prompt to avoid leaking YAML to the LLM
      const sysBody = extractBodyWithoutFrontmatter(this.sessionConfig.systemPrompt);
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
          this.addLog(currentLogs, entry);
        }
      } catch { /* ignore startup log errors */ }

      // Build enhanced system prompt with tool instructions
      const toolInstructions = this.toolsOrchestrator?.getMCPInstructions() ?? '';
      const enhancedSystemPrompt = this.enhanceSystemPrompt(systemExpanded, toolInstructions);

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

      const resultShape = {
        success: result.success,
        error: result.error,
        conversation: result.conversation,
        logs: result.logs,
        accounting: result.accounting,
        finalReport: result.finalReport,
        // Provide ASCII tree for downstream consumers (CLI may choose to print)
        treeAscii: (() => { try { return this.tree.renderAscii(); } catch { return undefined; } })(),
        opTreeAscii: (() => { try { return this.opTree.renderAscii(); } catch { return undefined; } })(),
        opTree: (() => { try { return this.opTree.getSession(); } catch { return undefined; } })(),
      } as AIAgentResult;

      try { this.tree.endSession(result.success, result.error); } catch { /* ignore */ }
      try { this.opTree.endSession(result.success, result.error); } catch { /* ignore */ }
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
      this.addLog(currentLogs, logEntry);
      
      // Log exit for uncaught exception
      this.logExit(
        'EXIT-UNCAUGHT-EXCEPTION',
        `Uncaught exception: ${message}`,
        currentTurn,
        currentLogs
      );

      // Emit FIN summary even on failure
      this.emitFinalSummary(currentLogs, currentAccounting);
      const failShape = {
        success: false,
        error: message,
        conversation: currentConversation,
        logs: currentLogs,
        accounting: currentAccounting
      } as AIAgentResult;
      try { this.tree.endSession(false, message); } catch { /* ignore */ }
      try { this.opTree.endSession(false, message); } catch { /* ignore */ }
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

    const maxTurns = this.sessionConfig.maxTurns ?? 10;
    const maxRetries = this.sessionConfig.maxRetries ?? 3; // GLOBAL attempts cap per turn
    const pairs = this.sessionConfig.targets;

    // Turn loop - necessary for control flow with early termination
    // Turn 0 is initialization; action turns are 1..maxTurns
    // eslint-disable-next-line functional/no-loop-statements
    for (currentTurn = 1; currentTurn <= maxTurns; currentTurn++) {
      if (this.canceled) {
        const errMsg = 'canceled';
        this.emitFinalSummary(logs, accounting);
        try { this.tree.endSession(false, errMsg); } catch { /* ignore */ }
        try { this.opTree.endSession(false, errMsg); } catch { /* ignore */ }
        return { success: false, error: errMsg, conversation, logs, accounting };
      }
      if (this.stopRef?.stopping === true) {
        // Graceful stop: do not start further turns
        this.emitFinalSummary(logs, accounting);
        try { this.tree.endSession(true); } catch { /* ignore */ }
        try { this.opTree.endSession(true); } catch { /* ignore */ }
        return { success: true, conversation, logs, accounting } as AIAgentResult;
      }
      try { this.opTree.beginTurn(currentTurn, {}); this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession()); } catch { /* ignore */ }
      // Orchestrator receives ctx turn/subturn per call; no MCP client turn management
      this.llmClient.setTurn(currentTurn, 0);

      let lastError: string | undefined;
      let turnSuccessful = false;
      let finalTurnWarnLogged = false;

      // Global attempts across all provider/model pairs for this turn
      let attempts = 0;
      let pairCursor = 0;
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
            this.addLog(logs, entry);
          } catch { /* ignore log errors */ }
          this.masterLlmStartLogged = true;
        }
        const pair = pairs[pairCursor % pairs.length];
        pairCursor += 1;
        const { provider, model } = pair;
          
          try {
            // Build per-attempt conversation with optional guidance injection
            let attemptConversation = [...conversation];
            // On the last allowed attempt within this turn, nudge the model to use tools (not append_notes)
            if ((attempts === maxRetries - 1) && currentTurn < (maxTurns - 1)) {
              attemptConversation.push({
                role: 'user',
                content: 'Reminder: do not end with plain text. Use an available tool (excluding `agent__append_notes`) to make progress. When ready to conclude, call the tool `agent__final_report` to provide the final answer.'
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
                message: 'Final turn detected: restricting tools to `agent__final_report` and injecting finalization instruction.'
              };
              this.addLog(logs, warn);
              finalTurnWarnLogged = true;
            }

            this.llmAttempts++;
            attempts += 1;
            // Begin hierarchical LLM operation (Option C)
            const llmOpId = (() => { try { return this.opTree.beginOp(currentTurn, 'llm', { provider, model, isFinalTurn }); } catch { return undefined; } })();
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
              const internal = new Set<string>(['agent__append_notes', 'agent__final_report']);
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
                    this.addLog(logs, warn);
                  }
                });
              }
            } catch { /* ignore unknown-tool warning errors */ }

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
              try { this.tree.recordAccounting(accountingEntry); } catch { /* ignore */ }
              try {
                if (llmOpId !== undefined) this.opTree.appendAccounting(llmOpId, accountingEntry);
              } catch { /* ignore */ }
            }
            // Close hierarchical LLM op
            try {
              if (llmOpId !== undefined) {
                this.opTree.endOp(llmOpId, (turnResult.status.type === 'success') ? 'ok' : 'failed', { latency: turnResult.latencyMs });
                this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession());
              }
            } catch { /* ignore */ }

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
            this.addLog(logs, warnEntry);
            lastError = 'invalid_response: content_without_tools_or_final';
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
                  this.addLog(logs, warn);
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
                this.addLog(logs, warn);
              }
            }
          } else {
            // For markdown/text, no onOutput; CLI will print via formatter
          }

          // Log successful exit
          this.logExit(
            'EXIT-FINAL-ANSWER',
            'Final report received (agent__final_report), session complete',
            currentTurn,
            logs
          );

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
                this.addLog(logs, debugEntry);
                
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
                    if (name === 'agent__append_notes' || name === 'agent__final_report' || name === 'agent__batch') return acc;
                    // Count only known tools (non-internal) present in orchestrator
                    const isKnown = this.toolsOrchestrator?.hasTool(name) ?? false;
                    return isKnown ? acc + 1 : acc;
                  }, 0);
                  this.plannedSubturns.set(currentTurn, count);
                }
              } catch { /* ignore */ }

              // Output response text if we have any (even with tool calls)
              // Only in non-streaming mode (streaming already called onOutput per chunk)
              if (this.sessionConfig.stream !== true && turnResult.response !== undefined && turnResult.response.length > 0) {
                this.sessionConfig.callbacks?.onOutput?.(turnResult.response);
                // Add newline if response doesn't end with one
                if (!turnResult.response.endsWith('\n')) {
                  this.sessionConfig.callbacks?.onOutput?.('\n');
                }
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
                this.addLog(logs, warnEntry);
                this.llmSyntheticFailures++;
                lastError = 'invalid_response: empty_without_tools';
                // do not mark turnSuccessful; continue retry loop
                continue;
              }
              
              if (turnResult.status.finalAnswer) {
                // Treat as non-final unless a final_report was provided
                // Continue to next turn to allow the model to call agent__final_report
                turnSuccessful = true;
                break;
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
                  this.addLog(logs, warnEntry);
                  // Continue attempts loop (do not mark successful)
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
              
              // Continue trying other providers
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
            try { this.tree.recordAccounting(accountingEntry); } catch { /* ignore */ }
            this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
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
        
        this.logExit(
          exitCode,
          reason,
          currentTurn,
          logs
        );
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
        try { this.opTree.endTurn(currentTurn, {}); this.sessionConfig.callbacks?.onOpTree?.(this.opTree.getSession()); } catch { /* ignore */ }
      }
    }

    // Max turns exceeded
    this.logExit(
      'EXIT-MAX-TURNS-NO-RESPONSE',
      `Max turns (${String(maxTurns)}) reached without final response`,
      currentTurn,
      logs
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
      this.addLog(logs, fin);

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
      this.addLog(logs, finMcp);
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
    const turnSpanId = this.tree.startSpan('llm_turn', 'llm', { provider, model, turn: currentTurn });
    // Expose provider tools plus session-specific internal tools (overrides schemas, same names)
    const availableTools = [
      ...(this.toolsOrchestrator?.listTools() ?? []),
      ...this.getInternalTools()
    ];

    
    // Track if we've shown thinking for this turn
    let shownThinking = false;
    
    // Create tool executor function that delegates to MCP client with accounting
    let subturnCounter = 0;
    const maxToolCallsPerTurn = Math.max(1, this.sessionConfig.maxToolCallsPerTurn ?? 10);
    const toolExecutor = async (toolName: string, parameters: Record<string, unknown>): Promise<string> => {
      if (this.stopRef?.stopping === true) throw new Error('stop_requested');
      if (this.canceled) throw new Error('canceled');
      // Advance subturn for each tool call within this turn
      subturnCounter += 1;
      if (subturnCounter > maxToolCallsPerTurn) {
        const msg = `Tool calls per turn exceeded: limit=${String(maxToolCallsPerTurn)}. Switch strategy: avoid further tool calls this turn; either summarize progress or call agent__final_report to conclude.`;
        const warn: LogEntry = { timestamp: Date.now(), severity: 'WRN', turn: currentTurn, subturn: subturnCounter, direction: 'response', type: 'tool', remoteIdentifier: 'agent:limits', fatal: false, message: msg };
        this.addLog(logs, warn);
        throw new Error('tool_calls_per_turn_limit_exceeded');
      }
      // Orchestrator receives ctx turn/subturn per call

      // (no local preview helpers needed)

      // Normalize tool name to strip router/provider wrappers like <|constrain|>
      const normalizeToolName = (n: string): string => n.replace(/<\|[^|]+\|>/g, '').trim();
      const effectiveToolName = normalizeToolName(toolName);
      const startTime = Date.now();
      try {
        // Internal tools are handled by InternalToolProvider via orchestrator

        // Sub-agent execution is handled by the orchestrator (AgentProvider)

        // Orchestrator-managed execution for MCP + REST
        if (this.toolsOrchestrator?.hasTool(effectiveToolName) === true) {
          const managed = await this.toolsOrchestrator.executeWithManagement(
            effectiveToolName,
            parameters,
            { turn: currentTurn, subturn: subturnCounter },
            { timeoutMs: this.sessionConfig.toolTimeout, bypassConcurrency: effectiveToolName === 'agent__batch' }
          );
          return managed.result;
        }

        // Unknown tool after all paths
        {
          const req = formatToolRequestCompact(toolName, parameters);
          const warn: LogEntry = { timestamp: Date.now(), severity: 'WRN', turn: currentTurn, subturn: 0, direction: 'response', type: 'llm', remoteIdentifier: 'assistant:tool', fatal: false, message: `Unknown tool requested: ${req}` };
          this.addLog(logs, warn);
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
        try { this.tree.recordAccounting(accountingEntry); } catch { /* ignore */ }
        // accounting callback is invoked via execution tree
        
        // Re-throw to let AI SDK create a tool-error part so the LLM sees structured failure
        this.releaseToolSlot();
        throw error;
      }
    };
    
    const request: TurnRequest = {
      messages: conversation,
      provider,
      model,
      tools: availableTools,
      toolExecutor,
      temperature: this.sessionConfig.temperature,
      topP: this.sessionConfig.topP,
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
            this.addLog(logs, thinkingHeader);
            shownThinking = true;
            // Update tracking to prevent duplicate headers
            lastShownThinkingHeaderTurn = currentTurn;
          }
          // Always stream the thinking text (including the first chunk)
          this.sessionConfig.callbacks?.onThinking?.(chunk);
        }
      }
    };

    try {
      const result = await this.llmClient.executeTurn(request);
      this.tree.endSpan(turnSpanId, result.status.type === 'success' ? 'ok' : 'failed', { latency: result.latencyMs, status: result.status.type });
      return { ...result, shownThinking };
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      this.tree.endSpan(turnSpanId, 'failed', { error: msg });
      throw e;
    }
  }

  private enhanceSystemPrompt(systemPrompt: string, mcpInstructions: string): string {
    const internal = this.buildInternalToolsInstructions();
    const blocks: string[] = [systemPrompt];
    if (mcpInstructions.trim().length > 0) blocks.push(`## TOOLS' INSTRUCTIONS\n\n${mcpInstructions}`);
    if (internal.trim().length > 0) blocks.push(`## INTERNAL TOOLS\n\n${internal}`);
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
        this.addLog(logs, warn);
      } catch { /* ignore logging errors */ }
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
      return JSON.stringify({ ok: false, error: { code: 'INVALID_INPUT', message: 'calls[] requires id, tool, args' } });
    }

    // Batch execution: orchestrator-only inner calls; record a span for the batch
    const batchSpan = this.tree.startSpan('agent_batch', 'tool', { count: bi.calls.length, turn: currentTurn });
    const results = await Promise.all(bi.calls.map(async (c) => {
      const t0 = Date.now();
      if (c.tool === 'agent__append_notes' || c.tool === 'agent__final_report' || c.tool === 'agent__batch') {
        return { id: c.id, tool: c.tool, ok: false, elapsedMs: 0, error: { code: 'INTERNAL_NOT_ALLOWED', message: 'Internal tools are not allowed in batch' } };
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
    this.tree.endSpan(batchSpan, 'ok', { results: results.length });
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

  // Build internal tools list dynamically based on expected output
  private getInternalTools(): MCPTool[] {
    const tools: MCPTool[] = [
      {
        name: 'agent__append_notes',
        description: 'Side notes for operators. Use sparingly to record brief interim findings, assumptions, blockers, or caveats. Notes are appended to metadata and shown separately in the UI; they are NOT graded and should NOT be duplicated in final content.',
        inputSchema: {
          type: 'object',
          additionalProperties: false,
          required: ['text'],
          properties: {
            text: { type: 'string', minLength: 1 },
            tags: { type: 'array', items: { type: 'string' } },
          },
        },
      }
    ];

    const exp = this.sessionConfig.expectedOutput;
    const common = { status: { type: 'string', enum: ['success','failure','partial'] }, metadata: { type: 'object' } } as Record<string, unknown>;
    const rf: OutputFormatId = this.sessionConfig.outputFormat;
    if ((exp?.format === 'json') && rf !== 'json') {
      const rfStr = rf as string;
      throw new Error(`Output format mismatch: expectedOutput.format=json but session outputFormat=${rfStr}`);
    }
    if ((exp?.format === 'json') || rf === 'json') {
      {
        const descStr = (typeof this.resolvedFormatDescription === 'string' && this.resolvedFormatDescription.length > 0) ? ` ${this.resolvedFormatDescription}` : '';
        tools.push({
          name: 'agent__final_report',
          description: (`Finalize the task with a JSON report.` + descStr).trim(),
          inputSchema: {
            type: 'object',
            additionalProperties: false,
            required: ['status', 'format', 'content_json'],
            properties: {
              status: common.status,
              format: { type: 'string', const: 'json', description: this.resolvedFormatDescription },
              content_json: (exp?.schema ?? { type: 'object' }),
              metadata: common.metadata,
            },
          },
        });
      }
    } else {
      // Non-JSON: expose resolved format id and description; content is string
      const id = this.sessionConfig.outputFormat as string;
      const desc = this.resolvedFormatDescription;
      {
        const suffix = (typeof desc === 'string' && desc.length > 0) ? ` ${desc}` : '';
        // Specialize Slack schema and instructions to support Block Kit messages array
        if (id === 'slack' || id === 'slack-block-kit') {
          const S_TYPE_MRKDWN = '\\"type\\": \\"mrkdwn\\"';
          const S_TYPE_SECTION = '\\"type\\": \\"section\\"';
          const slackInstructions = (
            'Use Slack\'s block kit like this:\n\n'
            + '"messages": [\n'
            + '  {\n'
            + '    "blocks": [\n'
            + '      {\n'
            + '        "type": "header",\n'
            + '        "text": {\n'
            + '          "type": "plain_text",\n'
            + '          "text": "Company Intelligence: Acme Corp"\n'
            + '        }\n'
            + '      },\n'
            + '      {\n'
            + '        "type": "context",\n'
            + '        "elements": [\n'
            + '          {\n'
            + `            ${S_TYPE_MRKDWN},\n`
            + '            "text": "Generated: 2025-01-05 | Domain: acme.com"\n'
            + '          }\n'
            + '        ]\n'
            + '      },\n'
            + '      { "type": "divider" },\n'
            + '      {\n'
            + `        ${S_TYPE_SECTION},\n`
            + '        "text": { "type": "mrkdwn", "text": "*🏢 Company Profile*" }\n'
            + '      },\n'
            + '      {\n'
            + `        ${S_TYPE_SECTION},\n`
            + '        "text": {\n'
            + `          ${S_TYPE_MRKDWN},\n`
            + '          "text": "• *Industry:* B2B SaaS\\n• *Employees:* 500-1000\\n• *Revenue:* $50M ARR\\n• *Funding:* Series C ($75M)"\n'
            + '        }\n'
            + '      },\n'
            + '      { "type": "divider" },\n'
            + '      {\n'
            + `        ${S_TYPE_SECTION},\n`
            + '        "text": { "type": "mrkdwn", "text": "*📊 Netdata Usage*" }\n'
            + '      },\n'
            + '      {\n'
            + `        ${S_TYPE_SECTION},\n`
            + '        "text": {\n'
            + `          ${S_TYPE_MRKDWN},\n`
            + '          "text": "```\\nSpace: acme-production\\nNodes: 247 connected (7d)\\nUsers: 12 active\\nLast Activity: 2 hours ago\\n```"\n'
            + '        }\n'
            + '      }\n'
            + '    ]\n'
            + '  },\n'
            + '  {\n'
            + '    "blocks": [ /* next message blocks */ ]\n'
            + '  }\n'
            + ']\n\n'
            + 'Split long output into multiple messages. You can post up to 20 messages, each message having up to 50 blocks, each block having up to 2000 characters.\n\n'
            + 'Slack mrkdwn format instructions:\n'
            + '- *bold* → bold\n'
            + '- _italic_ → italic\n'
            + '- ~strikethrough~ → strikethrough\n'
            + '- `inline code`\n'
            + '- ```code block```\n'
            + '- > quoted text (at line start)\n'
            + '- <https://example.com|Link Text> → clickable links\n'
            + '- Lists: Use •, -, or numbers (no nesting)\n\n'
            + 'GitHub markdown is NOT Supported:\n'
            + '- ❌ Tables\n'
            + '- ❌ Headers (#, ##)\n'
            + '- ❌ Horizontal rules (---)\n'
            + '- ❌ Nested lists\n'
            + '- ❌ Image embedding\n'
            + '- ❌ HTML tags'
          );
          tools.push({
            name: 'agent__final_report',
            description: (
              'Finalize the task with a Slack report. ' + suffix
              + '\nREQUIREMENT: Provide up to 20 `messages` with Block Kit + mrkdwn only. Do NOT provide plain `content`. Do NOT use GitHub markdown.\n\n'
              + slackInstructions
            ).trim(),
            inputSchema: {
              type: 'object',
              additionalProperties: false,
              required: ['status', 'format', 'messages'],
              properties: {
                status: common.status,
                format: { type: 'string', const: 'slack-block-kit', description: desc },
                // Block Kit messages for multi-message output
                messages: {
                  type: 'array',
                  minItems: 1,
                  maxItems: 20,
                  items: {
                    type: 'object',
                    additionalProperties: true,
                    required: ['blocks'],
                    properties: {
                      blocks: {
                        type: 'array',
                        minItems: 1,
                        maxItems: 50,
                        items: {
                          oneOf: [
                            {
                              type: 'object',
                              additionalProperties: true,
                              required: ['type', 'text'],
                              properties: {
                                type: { const: 'section' },
                                text: {
                                  type: 'object',
                                  additionalProperties: true,
                                  required: ['type', 'text'],
                                  properties: {
                                    type: { const: 'mrkdwn' },
                                    text: { type: 'string', minLength: 1, maxLength: 2900 }
                                  }
                                },
                                fields: {
                                  type: 'array',
                                  maxItems: 10,
                                  items: {
                                    type: 'object',
                                    additionalProperties: true,
                                    required: ['type', 'text'],
                                    properties: {
                                      type: { const: 'mrkdwn' },
                                      text: { type: 'string', minLength: 1, maxLength: 2000 }
                                    }
                                  }
                                }
                              }
                            },
                            {
                              type: 'object',
                              additionalProperties: true,
                              required: ['type', 'text'],
                              properties: {
                                type: { const: 'header' },
                                text: {
                                  type: 'object',
                                  additionalProperties: true,
                                  required: ['type', 'text'],
                                  properties: {
                                    type: { const: 'plain_text' },
                                    text: { type: 'string', minLength: 1, maxLength: 150 }
                                  }
                                }
                              }
                            },
                            {
                              type: 'object',
                              additionalProperties: true,
                              required: ['type'],
                              properties: { type: { const: 'divider' } }
                            },
                            {
                              type: 'object',
                              additionalProperties: true,
                              required: ['type', 'elements'],
                              properties: {
                                type: { const: 'context' },
                                elements: {
                                  type: 'array',
                                  minItems: 1,
                                  maxItems: 10,
                                  items: {
                                    type: 'object',
                                    additionalProperties: true,
                                    required: ['type', 'text'],
                                    properties: {
                                      type: { const: 'mrkdwn' },
                                      text: { type: 'string', minLength: 1, maxLength: 2000 }
                                    }
                                  }
                                }
                              }
                            }
                          ]
                        }
                      }
                    }
                  }
                },
                metadata: common.metadata,
              },
              // Enforce messages only
            },
          });
        } else {
          tools.push({
            name: 'agent__final_report',
            description: ('Finalize the task with a ' + id + ' report.' + suffix).trim(),
            inputSchema: {
              type: 'object',
              additionalProperties: false,
              required: ['status', 'format', 'content'],
              properties: {
                status: common.status,
                format: { type: 'string', const: id, description: desc },
                content: { type: 'string', minLength: 1, description: desc },
                metadata: common.metadata,
              },
            },
          });
        }
      }
    }
    return tools;
  }

  private buildInternalToolsInstructions(): string {
    const exp = this.sessionConfig.expectedOutput;
    const lines: string[] = [];
    const FINISH_ONLY = '- Finish ONLY by calling `agent__final_report` exactly once.';
    const ARGS = '- Arguments:';
    const STATUS_LINE = '  - `status`: one of `success`, `failure`, `partial`.';
    lines.push('- Use tool `agent__append_notes` sparingly for brief housekeeping notes; it is not graded and does not count as progress.');
    const rf2: OutputFormatId = this.sessionConfig.outputFormat;
    const rdesc = this.resolvedFormatDescription;
    if ((exp?.format === 'json') || rf2 === 'json') {
      lines.push(FINISH_ONLY);
      lines.push(ARGS);
      lines.push(STATUS_LINE);
      lines.push('  - `format`: "json".');
      lines.push('  - `content_json`: MUST match the required JSON Schema exactly.');
    } else {
      lines.push(FINISH_ONLY);
      lines.push(ARGS);
      lines.push(STATUS_LINE);
      const id2 = this.sessionConfig.outputFormat as string;
      const descLine = (typeof rdesc === 'string' && rdesc.length > 0) ? ` ${rdesc}` : '';
      lines.push('  - `format`: "' + id2 + '".' + descLine);
      if (id2 === 'slack') {
        lines.push('  - `messages`: array of Slack Block Kit messages (no plain `content`).');
        lines.push('    • Max 50 blocks per message; mrkdwn per section ≤ 2800 chars.');
      } else {
        lines.push('  - `content`: complete deliverable.');
      }
    }
    lines.push('- Do NOT end with plain text. The session ends only after `agent__final_report`.');

    // If user enabled batch tool, add concise usage guidance with a compact example
    if (this.sessionConfig.tools.includes('batch')) {
      lines.push('');
      lines.push('- Use tool `agent__batch` to call multiple tools in one go.');
      lines.push('  - `calls[]`: items with `id`, `tool` (exposed name), `args`.');
      lines.push('  - Execution is always parallel.');
      lines.push('  - Keep `args` minimal; they are validated server-side against real schemas.');
      lines.push('  - Examples (brave + jina + fetcher):');
      lines.push('    1) Sequential batch (no deps): 2x Jina searches, 2x Brave searches, 2x Fetcher fetches');
      lines.push('    {');
      lines.push('      "calls": [');
      lines.push('        { "id": 1, "tool": "jina__jina_search_web",');
      lines.push('          "args": { "query": "Netdata real-time monitoring features", "num": 10 } },');
      lines.push('');
      lines.push('        { "id": 2, "tool": "jina__jina_search_arxiv",');
      lines.push('          "args": { "query": "eBPF monitoring 2024", "num": 20 } },');
      lines.push('');
      lines.push('        { "id": 3, "tool": "brave__brave_web_search",');
      lines.push('          "args": { "query": "Netdata Cloud pricing", "count": 8, "offset": 0 } },');
      lines.push('');
      lines.push('        { "id": 4, "tool": "brave__brave_local_search",');
      lines.push('          "args": { "query": "coffee near Athens", "count": 5 } },');
      lines.push('');
      lines.push('        { "id": 5, "tool": "fetcher__fetch_url",');
      lines.push('          "args": { "url": "https://www.netdata.cloud" } },');
      lines.push('');
      lines.push('        { "id": 6, "tool": "fetcher__fetch_url",');
      lines.push('          "args": { "url": "https://learn.netdata.cloud/" } }');
      lines.push('      ]');
      lines.push('    }');
      lines.push('');
      lines.push('    2) Parallel batch (same tools)');
      lines.push('    {');
      lines.push('      "calls": [');
      lines.push('        { "id": 1, "tool": "jina__jina_search_web",');
      lines.push('          "args": { "query": "Netdata vs Prometheus 2024", "num": 10 } },');
      lines.push('');
      lines.push('        { "id": 2, "tool": "jina__jina_search_arxiv",');
      lines.push('          "args": { "query": "time-series anomaly detection streaming", "num": 15 } },');
      lines.push('');
      lines.push('        { "id": 3, "tool": "brave__brave_web_search",');
      lines.push('          "args": { "query": "Netdata agent install Ubuntu", "count": 10 } },');
      lines.push('');
      lines.push('        { "id": 4, "tool": "brave__brave_local_search",');
      lines.push('          "args": { "query": "devops meetups near Athens", "count": 5 } },');
      lines.push('');
      lines.push('        { "id": 5, "tool": "fetcher__fetch_url",');
      lines.push('          "args": { "url": "https://github.com/netdata/netdata" } },');
      lines.push('');
      lines.push('        { "id": 6, "tool": "fetcher__fetch_url",');
      lines.push('          "args": { "url": "https://community.netdata.cloud/" } }');
      lines.push('      ]');
      lines.push('    }');
    }
    return lines.join('\n');
  }
}

// Export session class as main interface
export { AIAgentSession as AIAgent };
