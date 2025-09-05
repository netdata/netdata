import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';

import Ajv from 'ajv';

import type { OutputFormatId } from './formats.js';
import type { AIAgentSessionConfig, AIAgentResult, ConversationMessage, LogEntry, AccountingEntry, Configuration, TurnRequest, MCPTool, LLMAccountingEntry, ToolAccountingEntry, MCPServerConfig } from './types.js';

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
import { LLMClient } from './llm-client.js';
import { MCPClientManager } from './mcp-client.js';
import { SubAgentRegistry } from './subagent-registry.js';
import { formatToolRequestCompact } from './utils.js';

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
  
  private readonly mcpClient: MCPClientManager;
  private readonly llmClient: LLMClient;
  private readonly sessionConfig: AIAgentSessionConfig;
  private ajv?: Ajv;
  // Internal housekeeping notes
  private internalNotes: { text: string; tags?: string[]; ts: number }[] = [];
  private childConversations: { agentId?: string; toolName: string; promptPath: string; conversation: ConversationMessage[]; trace?: { originId?: string; parentId?: string; selfId?: string; callPath?: string } }[] = [];
  private readonly subAgents?: SubAgentRegistry;
  private readonly txnId: string;
  private readonly originTxnId?: string;
  private readonly parentTxnId?: string;
  private readonly callPath?: string;
  private resolvedFormat?: OutputFormatId;
  private resolvedFormatDescription?: string;
  // Finalization state captured via agent__final_report
  private finalReport?: {
    status: 'success' | 'failure' | 'partial';
    // Allow all known output formats plus legacy 'text'
    format: 'json' | 'markdown' | 'markdown+mermaid' | 'slack' | 'tty' | 'pipe' | 'sub-agent' | 'text';
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

  private constructor(
    config: Configuration,
    conversation: ConversationMessage[],
    logs: LogEntry[],
    accounting: AccountingEntry[],
    success: boolean,
    error: string | undefined,
    currentTurn: number,
    mcpClient: MCPClientManager,
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
    this.mcpClient = mcpClient;
    this.llmClient = llmClient;
    this.sessionConfig = sessionConfig;
    // Initialize sub-agents registry if provided
    if (Array.isArray(sessionConfig.subAgentPaths) && sessionConfig.subAgentPaths.length > 0) {
      const reg = new SubAgentRegistry();
      reg.load(sessionConfig.subAgentPaths);
      this.subAgents = reg;
    }
    // Tracing context
    this.txnId = sessionConfig.trace?.selfId ?? crypto.randomUUID();
    this.originTxnId = sessionConfig.trace?.originId ?? this.txnId;
    this.parentTxnId = sessionConfig.trace?.parentId;
    this.callPath = sessionConfig.trace?.callPath ?? sessionConfig.agentId;
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

    // Wrap onLog to inject trace fields
    const wrapLog = (fn?: (entry: LogEntry) => void) => (entry: LogEntry): void => {
      if (fn === undefined) return;
      const enriched: LogEntry = {
        ...entry,
        agentId: enrichedSessionConfig.agentId,
        callPath: enrichedSessionConfig.trace?.callPath ?? enrichedSessionConfig.agentId,
        txnId: enrichedSessionConfig.trace?.selfId,
        parentTxnId: enrichedSessionConfig.trace?.parentId,
        originTxnId: enrichedSessionConfig.trace?.originId,
      };
      fn(enriched);
    };

    // Create session-owned MCP client
    const mcpClient = new MCPClientManager({ 
      trace: enrichedSessionConfig.traceMCP === true, 
      verbose: enrichedSessionConfig.verbose,
      onLog: wrapLog(enrichedSessionConfig.callbacks?.onLog),
      maxToolResponseBytes: enrichedSessionConfig.toolResponseMaxBytes,
      maxConcurrentInit: enrichedSessionConfig.config.defaults?.mcpInitConcurrency ?? enrichedSessionConfig.mcpInitConcurrency ?? Number.MAX_SAFE_INTEGER
    });

    // Create session-owned LLM client
    const llmClient = new LLMClient(enrichedSessionConfig.config.providers, {
      traceLLM: enrichedSessionConfig.traceLLM,
      onLog: wrapLog(enrichedSessionConfig.callbacks?.onLog)
    });

    return new AIAgentSession(
      enrichedSessionConfig.config,
      [], // empty conversation
      [], // empty logs  
      [], // empty accounting
      false, // not successful yet
      undefined, // no error yet
      0, // start at turn 0
      mcpClient,
      llmClient,
      enrichedSessionConfig
    );
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
    const enriched: LogEntry = {
      agentId: this.sessionConfig.agentId,
      callPath: this.callPath,
      txnId: this.txnId,
      parentTxnId: this.parentTxnId,
      originTxnId: this.originTxnId,
      ...entry,
    };
    logs.push(enriched);
    this.sessionConfig.callbacks?.onLog?.(enriched);
  }

  // Main execution method - returns immutable result
  async run(): Promise<AIAgentResult> {
    let currentConversation = [...this.conversation];
    let currentLogs = [...this.logs];
    let currentAccounting = [...this.accounting];
    let currentTurn = this.currentTurn;

    try {
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
      // Initialize MCP servers (non-fatal if they fail)
      try {
        const selected = Object.fromEntries(
          this.sessionConfig.tools
            .map((t) => [t, this.sessionConfig.config.mcpServers[t] as unknown])
            .filter(([, cfg]) => cfg !== undefined)
        ) as Record<string, MCPServerConfig>;
        await this.mcpClient.initializeServers(selected);
      } catch (e) {
        const message = e instanceof Error ? e.message : String(e);
        const logEntry: LogEntry = {
          timestamp: Date.now(),
          severity: 'WRN',
          turn: 0,
          subturn: 0,
          direction: 'response',
          type: 'mcp',
          remoteIdentifier: 'init',
          fatal: false,
          message: `MCP initialization failed and will be skipped: ${message}`
        };
        this.addLog(currentLogs, logEntry);
      }

      // Startup banner: list resolved MCP tools and sub-agent tools (exposed names)
      try {
        const mcpToolNames = this.mcpClient.getAllTools().map((t) => t.name);
        const subAgentTools = (this.subAgents?.getTools() ?? []).map((t) => t.name);
        const banner: LogEntry = {
          timestamp: Date.now(),
          severity: 'VRB',
          turn: 0,
          subturn: 0,
          direction: 'response',
          type: 'llm',
          remoteIdentifier: 'agent:tools',
          fatal: false,
          message: `tools: mcp=${String(mcpToolNames.length)} [${mcpToolNames.join(', ')}]; subagents=${String(subAgentTools.length)} [${subAgentTools.join(', ')}]`
        };
        this.addLog(currentLogs, banner);
      } catch { /* ignore banner errors */ }

      // Build prompt variables and expand placeholders in prompts
      const buildPromptVars = (): Record<string, string> => {
        const pad2 = (n: number): string => (n < 10 ? `0${String(n)}` : String(n));
        const formatRFC3339Local = (d: Date): string => {
          const y = d.getFullYear();
          const m = pad2(d.getMonth() + 1);
          const da = pad2(d.getDate());
          const hh = pad2(d.getHours());
          const mm = pad2(d.getMinutes());
          const ss = pad2(d.getSeconds());
          const tzMin = -d.getTimezoneOffset();
          const sign = tzMin >= 0 ? '+' : '-';
          const abs = Math.abs(tzMin);
          const tzh = pad2(Math.floor(abs / 60));
          const tzm = pad2(abs % 60);
          return `${String(y)}-${m}-${da}T${hh}:${mm}:${ss}${sign}${tzh}:${tzm}`;
        };
        const detectTimezone = (): string => { try { return Intl.DateTimeFormat().resolvedOptions().timeZone; } catch { return process.env.TZ ?? 'UTC'; } };
        const detectOS = (): string => {
          try {
            const contentOs = fs.readFileSync('/etc/os-release', 'utf-8');
            const match = /^PRETTY_NAME=\"?([^\"\n]+)\"?/m.exec(contentOs);
            if (match?.[1] !== undefined) return `${match[1]} (kernel ${os.release()})`;
          } catch { /* ignore */ }
          return `${os.type()} ${os.release()}`;
        };
        const now = new Date();
        return {
          DATETIME: formatRFC3339Local(now),
          DAY: now.toLocaleDateString(undefined, { weekday: 'long' }),
          TIMEZONE: detectTimezone(),
          MAX_TURNS: String(this.sessionConfig.maxTurns ?? 10),
          OS: detectOS(),
          ARCH: process.arch,
          KERNEL: `${os.type()} ${os.release()}`,
          CD: process.cwd(),
          HOSTNAME: os.hostname(),
          USER: (() => { try { return os.userInfo().username; } catch { return process.env.USER ?? process.env.USERNAME ?? ''; } })(),
        };
      };
      const expandPrompt = (str: string, vars: Record<string, string>): string => {
        const replace = (s: string, re: RegExp) => s.replace(re, (_m, name: string) => (name in vars ? vars[name] : _m));
        let out = str;
        out = replace(out, /\$\{([A-Z_]+)\}/g);
        out = replace(out, /\{\{([A-Z_]+)\}\}/g);
        return out;
      };

      // Resolve FORMAT centrally from configuration (with sane fallback)
      const { describeFormat } = await import('./formats.js');
      const resolvedFmt: OutputFormatId = this.sessionConfig.outputFormat;
      const fmtDesc = describeFormat(resolvedFmt);
      this.resolvedFormat = resolvedFmt;
      this.resolvedFormatDescription = fmtDesc;

      // Apply ${FORMAT} replacement first, then expand variables
      const withFormat = this.sessionConfig.systemPrompt.replace(/\$\{FORMAT\}|\{\{FORMAT\}\}/g, fmtDesc);
      const vars = buildPromptVars();
      const systemExpanded = expandPrompt(withFormat, vars);
      const userExpanded = expandPrompt(this.sessionConfig.userPrompt, vars);

      // Build enhanced system prompt with tool instructions
      const toolInstructions = this.mcpClient.getCombinedInstructions();
      const enhancedSystemPrompt = this.enhanceSystemPrompt(
        systemExpanded, 
        toolInstructions
      );

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

      return {
        success: result.success,
        error: result.error,
        conversation: result.conversation,
        logs: result.logs,
        accounting: result.accounting,
        finalReport: result.finalReport,
      };

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
      return {
        success: false,
        error: message,
        conversation: currentConversation,
        logs: currentLogs,
        accounting: currentAccounting
      };
    } finally {
      await this.mcpClient.cleanup();
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
      this.mcpClient.setTurn(currentTurn, 0);
      this.llmClient.setTurn(currentTurn, 0);

      let lastError: string | undefined;
      let turnSuccessful = false;
      let finalTurnWarnLogged = false;

      // Global attempts across all provider/model pairs for this turn
      let attempts = 0;
      let pairCursor = 0;
      // eslint-disable-next-line functional/no-loop-statements
      while (attempts < maxRetries && !turnSuccessful) {
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
              const mapping = this.mcpClient.getToolServerMapping();
              // Also consider sub-agent tools as known tools for this warning check
              const subAgentTools = (this.subAgents?.getTools() ?? []).map((t) => t.name);
              const subAgentSet = new Set(subAgentTools);
              // Internal tools always available; include optional batch tool if enabled for this session
              const internal = new Set<string>(['agent__append_notes', 'agent__final_report']);
              if (this.sessionConfig.tools.includes('batch')) internal.add('agent__batch');
              const normalizeTool = (n: string) => n.replace(/^<\|[^|]+\|>/, '').trim();
              const lastAssistant = turnResult.messages.filter((m) => m.role === 'assistant');
              const assistantMsg = lastAssistant.length > 0 ? lastAssistant[lastAssistant.length - 1] : undefined;
              if (assistantMsg?.toolCalls !== undefined && assistantMsg.toolCalls.length > 0) {
                (assistantMsg.toolCalls).forEach((tc) => {
                  const n = normalizeTool(tc.name);
                  if (!internal.has(n) && !mapping.has(n) && !subAgentSet.has(n)) {
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
              const accountingEntry: AccountingEntry = {
                type: 'llm',
                timestamp: Date.now(),
                status: turnResult.status.type === 'success' ? 'ok' : 'failed',
                latency: turnResult.latencyMs,
                provider,
                model,
                actualProvider: provider === 'openrouter' ? this.llmClient.getLastActualRouting().provider : undefined,
                actualModel: provider === 'openrouter' ? this.llmClient.getLastActualRouting().model : undefined,
                costUsd: provider === 'openrouter' ? this.llmClient.getLastCostInfo().costUsd : undefined,
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
              this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
            }

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
      const tokCached = llmEntries.reduce((s, e) => s + (e.tokens.cachedTokens ?? 0), 0);
      const tokTotal = llmEntries.reduce((s, e) => s + e.tokens.totalTokens, 0);
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

      const msg = `requests=${String(llmRequests)} failed=${String(llmFailures)}, tokens in=${String(tokIn)} out=${String(tokOut)} cached=${String(tokCached)} total=${String(tokTotal)}, cost total=$${totalCost.toFixed(5)} upstream=$${totalUpstreamCost.toFixed(5)}, latency sum=${String(llmLatencySum)}ms avg=${String(llmLatencyAvg)}ms, providers/models: ${pairsStr.length > 0 ? pairsStr : 'none'}`;
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
      const sizeCaps = typeof (this.mcpClient as unknown as { getSizeCapHits?: () => number }).getSizeCapHits === 'function'
        ? (this.mcpClient as unknown as { getSizeCapHits: () => number }).getSizeCapHits()
        : 0;
      const finMcp: LogEntry = {
        timestamp: Date.now(),
        severity: 'FIN',
        turn: this.currentTurn,
        subturn: 0,
        direction: 'response',
        type: 'mcp',
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
    const availableTools = [...this.mcpClient.getAllTools(), ...(this.subAgents?.getTools() ?? []), ...this.getInternalTools()];
    
    // Track if we've shown thinking for this turn
    let shownThinking = false;
    
    // Create tool executor function that delegates to MCP client with accounting
    let subturnCounter = 0;
    const toolExecutor = async (toolName: string, parameters: Record<string, unknown>): Promise<string> => {
      // Advance subturn for each tool call within this turn
      subturnCounter += 1;
      try { this.mcpClient.setTurn(currentTurn, subturnCounter); } catch { /* ignore */ }

      const singleLine = (s: string, max = 200): string => {
        const one = s.replace(/[\r\n]+/g, ' ').trim();
        return one.length > max ? `${one.slice(0, max - 1)}…` : one;
      };
      const previewParams = (obj: Record<string, unknown>): string => {
        try { return singleLine(JSON.stringify(obj)); } catch { return '[unserializable]'; }
      };

      // Normalize tool name to strip router/provider wrappers like <|constrain|>
      const normalizeToolName = (n: string): string => n.replace(/<\|[^|]+\|>/g, '').trim();
      const effectiveToolName = normalizeToolName(toolName);
      const startTime = Date.now();
      const acquiredSlot = await this.acquireToolSlot();
      try {
        // Intercept internal tools (agent_*)
        if (effectiveToolName === 'agent__append_notes') {
          this.addLog(logs, { timestamp: Date.now(), severity: 'VRB', turn: currentTurn, subturn: subturnCounter, direction: 'request', type: 'llm', remoteIdentifier: 'agent', fatal: false, message: `append_notes(${previewParams(parameters)}) <<< internal` });
          const textParam = (parameters as { text?: unknown }).text;
          const text = typeof textParam === 'string' ? textParam : textParam === undefined ? '' : JSON.stringify(textParam);
          const tags = Array.isArray((parameters as { tags?: unknown }).tags) ? (parameters as { tags?: string[] }).tags : undefined;
          if (text.trim().length > 0) this.internalNotes.push({ text, tags, ts: Date.now() });
          const latency = Date.now() - startTime;
          const accountingEntry: AccountingEntry = {
            type: 'tool', timestamp: startTime, status: 'ok', latency,
            mcpServer: 'agent', command: toolName,
            charactersIn: JSON.stringify(parameters).length, charactersOut: 15,
            agentId: this.sessionConfig.agentId, callPath: this.callPath, txnId: this.txnId, parentTxnId: this.parentTxnId, originTxnId: this.originTxnId
          };
          accounting.push(accountingEntry);
          this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
          this.releaseToolSlot();
          return JSON.stringify({ ok: true, totalNotes: this.internalNotes.length });
        }
        if (effectiveToolName === 'agent__final_report') {
          this.addLog(logs, { timestamp: Date.now(), severity: 'VRB', turn: currentTurn, subturn: subturnCounter, direction: 'request', type: 'llm', remoteIdentifier: 'agent', fatal: false, message: `final_report(${previewParams(parameters)}) <<< internal` });
          const p = parameters;
          const statusParam = p.status;
          const formatParam = p.format;
          const status = (typeof statusParam === 'string' ? statusParam : 'success') as 'success' | 'failure' | 'partial';
          const format = (typeof formatParam === 'string' ? formatParam : 'markdown') as 'json' | 'markdown' | 'markdown+mermaid' | 'slack' | 'tty' | 'pipe' | 'sub-agent' | 'text';
          const content = typeof p.content === 'string' ? p.content : undefined;
          const isPlainObject = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object' && !Array.isArray(v);
          const content_json = isPlainObject(p.content_json) ? p.content_json : undefined;
          // Attach Slack messages (if provided) into metadata.slack.messages for server-side rendering
          let metadata: Record<string, unknown> | undefined = isPlainObject(p.metadata) ? p.metadata : undefined;
          const maybeMessages = p.messages;
          if (Array.isArray(maybeMessages)) {
            const raw = maybeMessages;
            const msgs = raw.filter(isPlainObject);
            const getProp = (o: Record<string, unknown> | undefined, k: string): unknown => (o !== undefined && Object.prototype.hasOwnProperty.call(o, k)) ? (o[k]) : undefined;
            const slackMeta = getProp(metadata, 'slack');
            const prevSlack = isPlainObject(slackMeta) ? slackMeta : {};
            metadata = { ...(metadata ?? {}), slack: { ...prevSlack, messages: msgs } };
          }
          // Strict Slack enforcement: require messages when format is slack
          if (format === 'slack') {
            if (!Array.isArray(maybeMessages) || maybeMessages.length === 0) {
              this.releaseToolSlot();
              throw new Error('Slack final_report must provide `messages` (Block Kit). Do not use `content`.');
            }
          }
          this.finalReport = { status, format, content, content_json, metadata, ts: Date.now() };
          const latency = Date.now() - startTime;
          const accountingEntry: AccountingEntry = {
            type: 'tool', timestamp: startTime, status: 'ok', latency,
            mcpServer: 'agent', command: toolName,
            charactersIn: JSON.stringify(parameters).length, charactersOut: 12,
            agentId: this.sessionConfig.agentId, callPath: this.callPath, txnId: this.txnId, parentTxnId: this.parentTxnId, originTxnId: this.originTxnId
          };
          accounting.push(accountingEntry);
          this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
          this.releaseToolSlot();
          return JSON.stringify({ ok: true });
        }

        // Batch execution internal tool (only if enabled in session tools)
        if (effectiveToolName === 'agent__batch' && this.sessionConfig.tools.includes('batch')) {
          // Outer batch call should not hold a slot; free it and let inner calls acquire slots fairly
          this.releaseToolSlot();
          const out = await this.executeBatchCalls(parameters, currentTurn, startTime, logs, accounting, toolName);
          return out;
        }

        // Sub-agent execution
        if (this.subAgents?.hasTool(effectiveToolName) === true) {
          try {
            const subName = effectiveToolName.startsWith('agent__') ? effectiveToolName.slice('agent__'.length) : effectiveToolName;
            this.addLog(logs, { timestamp: Date.now(), severity: 'VRB', turn: currentTurn, subturn: subturnCounter, direction: 'request', type: 'llm', remoteIdentifier: 'agent', fatal: false, message: `${subName}(${previewParams(parameters)}) <<< subagent ${subName}` });
            const exec = await this.subAgents.execute(effectiveToolName, parameters, {
              config: this.sessionConfig.config,
              callbacks: this.sessionConfig.callbacks,
              targets: this.sessionConfig.targets,
              // propagate all-model overrides
              stream: this.sessionConfig.stream,
              traceLLM: this.sessionConfig.traceLLM,
              traceMCP: this.sessionConfig.traceMCP,
              verbose: this.sessionConfig.verbose,
              // provide master defaults for sub-agents (used only if child undefined)
              temperature: this.sessionConfig.temperature,
              topP: this.sessionConfig.topP,
              llmTimeout: this.sessionConfig.llmTimeout,
              toolTimeout: this.sessionConfig.toolTimeout,
              maxRetries: this.sessionConfig.maxRetries,
              maxTurns: this.sessionConfig.maxTurns,
              toolResponseMaxBytes: this.sessionConfig.toolResponseMaxBytes,
              parallelToolCalls: this.sessionConfig.parallelToolCalls,
              trace: { originId: this.originTxnId, parentId: this.txnId, callPath: `${this.callPath ?? ''}->${effectiveToolName}` }
            });
            // Collect child conversation for save-all support
            this.childConversations.push({ agentId: exec.child.toolName, toolName: exec.child.toolName, promptPath: exec.child.promptPath, conversation: exec.conversation, trace: exec.trace });
            const latency = Date.now() - startTime;
            const accountingEntry: AccountingEntry = {
              type: 'tool', timestamp: startTime, status: 'ok', latency,
              mcpServer: 'subagent', command: effectiveToolName,
              charactersIn: JSON.stringify(parameters).length, charactersOut: exec.result.length,
              agentId: this.sessionConfig.agentId, callPath: this.callPath, txnId: this.txnId, parentTxnId: this.parentTxnId, originTxnId: this.originTxnId
            };
            accounting.push(accountingEntry);
            this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
            // Merge child accounting into parent
            exec.accounting.forEach((a) => {
              accounting.push(a);
              this.sessionConfig.callbacks?.onAccounting?.(a);
            });
            this.releaseToolSlot();
            return exec.result;
          } catch (e) {
            const latency = Date.now() - startTime;
            const msg = e instanceof Error ? e.message : String(e);
            // Verbose error breadcrumb for sub-agent failures
            try {
              const errLog: LogEntry = {
                timestamp: Date.now(),
                severity: 'ERR',
                turn: currentTurn,
                subturn: subturnCounter,
                direction: 'response',
                type: 'llm',
                remoteIdentifier: 'subagent:execute',
                fatal: false,
                message: `error ${effectiveToolName}: ${msg}`,
              };
              this.addLog(logs, errLog);
            } catch { /* ignore logging errors */ }
            const accountingEntry: AccountingEntry = {
              type: 'tool', timestamp: startTime, status: 'failed', latency,
              mcpServer: 'subagent', command: effectiveToolName,
              charactersIn: JSON.stringify(parameters).length, charactersOut: 0,
              error: msg,
              agentId: this.sessionConfig.agentId, callPath: this.callPath, txnId: this.txnId, parentTxnId: this.parentTxnId, originTxnId: this.originTxnId
            };
            accounting.push(accountingEntry);
            this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
            this.releaseToolSlot();
            throw e;
          }
        }

        // Warn and fail fast if the tool is unknown (not internal and not from any MCP server)
        {
          const isInternal = (effectiveToolName === 'agent__append_notes' || effectiveToolName === 'agent__final_report');
          const mapping = this.mcpClient.getToolServerMapping();
          const isSub = this.subAgents?.hasTool(effectiveToolName) === true;
          if (!isInternal && !isSub && !mapping.has(effectiveToolName)) {
            const req = formatToolRequestCompact(toolName, parameters);
            const warn: LogEntry = {
              timestamp: Date.now(),
              severity: 'WRN',
              turn: currentTurn,
              subturn: 0,
              direction: 'response',
              type: 'llm',
              remoteIdentifier: 'assistant:tool',
              fatal: false,
              message: `Unknown tool requested: ${req}`
            };
            this.addLog(logs, warn);
            this.releaseToolSlot();
            throw new Error(`No server found for tool: ${effectiveToolName}`);
          }
        }

        const { result, serverName } = await this.mcpClient.executeToolByName(effectiveToolName, parameters, { slot: acquiredSlot });
        // Ensure non-empty tool result so providers that require one response per call don't reject
        const safeResult = (typeof result === 'string' && result.length === 0) ? ' ' : result;
        const latency = Date.now() - startTime;
        
        // Add tool accounting
        const accountingEntry: AccountingEntry = {
          type: 'tool',
          timestamp: startTime,
          status: 'ok',
          latency,
          mcpServer: serverName,
          command: toolName,
          charactersIn: JSON.stringify(parameters).length,
          charactersOut: safeResult.length,
          agentId: this.sessionConfig.agentId, callPath: this.callPath, txnId: this.txnId, parentTxnId: this.parentTxnId, originTxnId: this.originTxnId
        };
        accounting.push(accountingEntry);
        this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
        
        this.releaseToolSlot();
        return safeResult;
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
        this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
        
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

    const result = await this.llmClient.executeTurn(request);
    return { ...result, shownThinking };
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

    // Emit a concise batch start line for visibility
    try {
      const cap0 = Math.max(1, this.sessionConfig.maxConcurrentTools ?? 1);
      const eff0 = (this.sessionConfig.parallelToolCalls === false) ? 1 : cap0;
      const info: LogEntry = {
        timestamp: Date.now(),
        severity: 'VRB',
        turn: currentTurn,
        subturn: 0,
        direction: 'request',
        type: 'llm',
        remoteIdentifier: 'agent:batch',
        fatal: false,
        message: `starting batch: calls=${String(bi.calls.length)} cap=${String(eff0)}`,
      };
      this.addLog(logs, info);
    } catch { /* ignore batch log errors */ }

    // Prepare AJV for schema validation of per-call args
    const ajv = this.ajv ?? new Ajv({ allErrors: true, strict: false });
    this.ajv = ajv;

    interface ResultItem {
      id: string;
      tool: string;
      ok: boolean;
      elapsedMs: number;
      output?: unknown;
      error?: { code: string; message: string; schemaExcerpt?: Record<string, unknown> };
    }

    const byId = new Map<string, BatchCall>();
    bi.calls.forEach((c) => { byId.set(c.id, c); });

    // Execute all calls in parallel
    const results = new Map<string, ResultItem>();
    const runAll = async (): Promise<void> => {
      const ids = [...byId.keys()];
      const cap = Math.max(1, this.sessionConfig.maxConcurrentTools ?? 1);
      const effectiveCap = (this.sessionConfig.parallelToolCalls === false) ? 1 : cap;
      let idx = 0
      const workers: Promise<void>[] = [];
      let subturn = 0;
      const runOne = async (id: string): Promise<void> => {
        const BATCH_ID = BATCH_REMOTE_ID;
        const RESP = RESP_DIR;
        const LLM = LLM_TYPE;
        const c = byId.get(id);
        if (c === undefined) {
          results.set(id, { id, tool: '', ok: false, elapsedMs: 0, error: { code: 'NOT_FOUND', message: 'Call not found' } });
          try {
            const warn: LogEntry = { timestamp: Date.now(), severity: 'WRN', turn: currentTurn, subturn, direction: RESP, type: LLM, remoteIdentifier: BATCH_ID, fatal: false, message: `Call '${id}' not found` };
            this.addLog(logs, warn);
          } catch { /* ignore */ }
          return;
        }
        // Acquire a tool slot for this inner call and advance subturn
        const slot = await this.acquireToolSlot();
        // Advance subturn for each inner call
        subturn += 1;
        try { this.mcpClient.setTurn(currentTurn, subturn); } catch { /* ignore */ }
        const callStart = Date.now();
        // Validate args against real schema when available
        try {
          const resolved = this.mcpClient.resolveExposedTool(c.tool);
          const schema = resolved !== undefined ? this.mcpClient.getToolSchema(resolved.serverName, resolved.originalName) : undefined;
          if (schema !== undefined) {
            const validate = ajv.compile(schema);
            const valid = validate(c.args);
            if (!valid) {
              const errs = (validate.errors ?? []).map((e) => {
                const path = typeof e.instancePath === 'string' ? e.instancePath : '';
                const msg = typeof e.message === 'string' ? e.message : '';
                return `${path} ${msg}`.trim();
              }).join('; ');
              results.set(id, { id, tool: c.tool, ok: false, elapsedMs: Date.now() - callStart, error: { code: 'SCHEMA_VALIDATION', message: errs, schemaExcerpt: schema } });
              try {
                const warn: LogEntry = { timestamp: Date.now(), severity: 'WRN', turn: currentTurn, subturn, direction: RESP, type: LLM, remoteIdentifier: BATCH_ID, fatal: false, message: `Schema validation failed for ${c.tool} (id=${id}): ${errs}` };
                this.addLog(logs, warn);
              } catch { /* ignore */ }
              this.releaseToolSlot();
              return;
            }
          }
        } catch (e) {
          results.set(id, { id, tool: c.tool, ok: false, elapsedMs: Date.now() - callStart, error: { code: 'VALIDATION_INIT', message: e instanceof Error ? e.message : String(e) } });
          try {
            const msg = e instanceof Error ? e.message : String(e);
            const warn: LogEntry = { timestamp: Date.now(), severity: 'WRN', turn: currentTurn, subturn, direction: RESP, type: LLM, remoteIdentifier: BATCH_ID, fatal: false, message: `Validation init error for ${c.tool} (id=${id}): ${msg}` };
            this.addLog(logs, warn);
          } catch { /* ignore */ }
          this.releaseToolSlot();
          return;
        }
        // Disallow only reserved internal tools in batch (allow sub‑agents)
        if (c.tool.startsWith('agent__')) {
          const internalName = c.tool.slice('agent__'.length);
          // Block housekeeping/finalization tools from batch to prevent recursion/termination inside batch
          if (internalName === 'append_notes' || internalName === 'final_report' || internalName === 'batch') {
            results.set(id, { id, tool: c.tool, ok: false, elapsedMs: 0, error: { code: 'INTERNAL_NOT_ALLOWED', message: 'Internal tools are not allowed in batch' } });
            try {
              const warn: LogEntry = { timestamp: Date.now(), severity: 'WRN', turn: currentTurn, subturn, direction: RESP, type: LLM, remoteIdentifier: BATCH_ID, fatal: false, message: `Internal tool not allowed in batch: ${c.tool} (id=${id})` };
              this.addLog(logs, warn);
            } catch { /* ignore */ }
            this.releaseToolSlot();
            return;
          }
        }
        // Execute via sub-agent or MCP path
        try {
          if (this.subAgents?.hasTool(c.tool) === true) {
            const exec = await this.subAgents.execute(c.tool, c.args, { config: this.sessionConfig.config, callbacks: this.sessionConfig.callbacks, targets: this.sessionConfig.targets, trace: { originId: this.originTxnId, parentId: this.txnId, callPath: `${this.callPath ?? ''}->${c.tool}` } });
            results.set(id, { id, tool: c.tool, ok: true, elapsedMs: Date.now() - callStart, output: exec.result });
            exec.accounting.forEach((a) => { accounting.push(a); this.sessionConfig.callbacks?.onAccounting?.(a); });
          } else {
            const out = await this.mcpClient.executeToolByName(c.tool, c.args, { slot });
            results.set(id, { id, tool: c.tool, ok: true, elapsedMs: Date.now() - callStart, output: out.result });
          }
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err);
          results.set(id, { id, tool: c.tool, ok: false, elapsedMs: Date.now() - callStart, error: { code: 'EXECUTION_ERROR', message: msg } });
          try {
            const warn: LogEntry = { timestamp: Date.now(), severity: 'WRN', turn: currentTurn, subturn, direction: RESP, type: LLM, remoteIdentifier: BATCH_ID, fatal: false, message: `Execution error for ${c.tool} (id=${id}): ${msg}` };
            this.addLog(logs, warn);
          } catch { /* ignore */ }
        } finally {
          this.releaseToolSlot();
        }
      };
      const launch = async () => {
        // eslint-disable-next-line functional/no-loop-statements
        while (idx < ids.length) {
          const id = ids[idx++];
          await runOne(id);
        }
      };
      // eslint-disable-next-line functional/no-loop-statements
      for (let i = 0; i < effectiveCap; i++) workers.push(launch());
      await Promise.all(workers);
    };

    await runAll();
    const latency = Date.now() - startTime;
    const response = { results: bi.calls.map((c) => results.get(c.id)) };
    const accountingEntry: AccountingEntry = {
      type: 'tool', timestamp: startTime, status: 'ok', latency,
      mcpServer: 'agent', command: toolName,
      charactersIn: JSON.stringify(parameters).length, charactersOut: JSON.stringify(response).length,
      agentId: this.sessionConfig.agentId, callPath: this.callPath, txnId: this.txnId, parentTxnId: this.parentTxnId, originTxnId: this.originTxnId
    };
    accounting.push(accountingEntry);
    this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
    return JSON.stringify(response);
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
      this.mcpClient, // reuse MCP client
      this.llmClient, // reuse LLM client
      this.sessionConfig
    );
  }

  // Build internal tools list dynamically based on expected output
  private getInternalTools(): MCPTool[] {
    const tools: MCPTool[] = [
      {
        name: 'agent__append_notes',
        description: 'Housekeeping notes. Use sparingly for brief interim findings.',
        inputSchema: {
          type: 'object',
          additionalProperties: false,
          required: ['text'],
          properties: {
            text: { type: 'string', minLength: 1 },
            tags: { type: 'array', items: { type: 'string' } },
          },
        },
      },
    ];

    // Optional batch tool (exposed as agent__batch) toggled by listing 'batch' in tools
    const wantsBatch = this.sessionConfig.tools.includes('batch');
    if (wantsBatch) {
      tools.push({
        name: 'agent__batch',
        description: 'Execute multiple tools in one call (always parallel). Use exposed tool names.',
        inputSchema: {
          type: 'object',
          additionalProperties: false,
          required: ['calls'],
          properties: {
            calls: {
              type: 'array',
              items: {
                type: 'object',
                additionalProperties: true,
                required: ['id', 'tool', 'args'],
                properties: {
                  id: { oneOf: [ { type: 'string', minLength: 1 }, { type: 'number' } ] },
                  tool: { type: 'string', minLength: 1, description: 'Exposed tool name (e.g., brave__brave_web_search)' },
                  args: { type: 'object', additionalProperties: true }
                }
              }
            }
          }
        }
      });
    }

    const exp = this.sessionConfig.expectedOutput;
    const common = {
      status: { type: 'string', enum: ['success', 'failure', 'partial'] },
      metadata: { type: 'object' },
    } as Record<string, unknown>;
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
        if (id === 'slack') {
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
            + '          "text": "Claude\'s Analysis: Acme Corp"\n'
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
            + 'Maximum text length per block: 3000 bytes, maximum number of blocks per message: 50.\n\n'
            + 'mrkdwn format:\n'
            + '- *bold* → bold\n'
            + '- _italic_ → italic\n'
            + '- ~strikethrough~ → strikethrough\n'
            + '- `inline code`\n'
            + '- ```code block```\n'
            + '- > quoted text (at line start)\n'
            + '- <https://example.com|Link Text> → clickable links\n'
            + '- Lists: Use •, -, or numbers (no nesting)\n\n'
            + 'NOT Supported:\n'
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
              + '\nREQUIREMENT: Provide `messages` with Block Kit only. Do NOT provide plain `content`.\n\n'
              + slackInstructions
            ).trim(),
            inputSchema: {
              type: 'object',
              additionalProperties: false,
              required: ['status', 'format', 'messages'],
              properties: {
                status: common.status,
                format: { type: 'string', const: id, description: desc },
                // Block Kit messages for multi-message output
                messages: {
                  type: 'array',
                  minItems: 1,
                  items: {
                    type: 'object',
                    additionalProperties: true,
                    properties: {
                      blocks: {
                        type: 'array',
                        minItems: 1,
                        maxItems: 50,
                        items: { type: 'object', additionalProperties: true }
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
      lines.push('  - `content`: complete deliverable.');
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
