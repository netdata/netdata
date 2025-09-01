import Ajv from 'ajv';

import type { AIAgentSessionConfig, AIAgentResult, ConversationMessage, LogEntry, AccountingEntry, Configuration, TurnRequest, MCPTool, LLMAccountingEntry, ToolAccountingEntry } from './types.js';

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

// Immutable session class according to DESIGN.md
export class AIAgentSession {
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
  // Finalization state captured via agent_final_report
  private finalReport?: {
    status: 'success' | 'failure' | 'partial';
    format: 'json' | 'markdown' | 'text';
    content?: string;
    content_json?: Record<string, unknown>;
    metadata?: Record<string, unknown>;
    ts: number;
  };

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
  }

  // Static factory method for creating new sessions
  static create(sessionConfig: AIAgentSessionConfig): AIAgentSession {
    // Validate configuration
    validateProviders(sessionConfig.config, sessionConfig.targets.map((t) => t.provider));
    validateMCPServers(sessionConfig.config, sessionConfig.tools);
    validatePrompts(sessionConfig.systemPrompt, sessionConfig.userPrompt);

    // Create session-owned MCP client
    const mcpClient = new MCPClientManager({ 
      trace: sessionConfig.traceMCP === true, 
      verbose: sessionConfig.verbose,
      onLog: sessionConfig.callbacks?.onLog 
    });

    // Create session-owned LLM client
    const llmClient = new LLMClient(sessionConfig.config.providers, {
      traceLLM: sessionConfig.traceLLM,
      onLog: sessionConfig.callbacks?.onLog
    });

    return new AIAgentSession(
      sessionConfig.config,
      [], // empty conversation
      [], // empty logs  
      [], // empty accounting
      false, // not successful yet
      undefined, // no error yet
      0, // start at turn 0
      mcpClient,
      llmClient,
      sessionConfig
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
      message: `${exitCode}: ${reason} (fatal=true)`
    };
    logs.push(logEntry);
    this.sessionConfig.callbacks?.onLog?.(logEntry);
  }

  // Main execution method - returns immutable result
  async run(): Promise<AIAgentResult> {
    let currentConversation = [...this.conversation];
    let currentLogs = [...this.logs];
    let currentAccounting = [...this.accounting];
    let currentTurn = this.currentTurn;

    try {
      // Initialize MCP servers (non-fatal if they fail)
      try {
        const selected = Object.fromEntries(
          this.sessionConfig.tools.map((t) => [t, this.sessionConfig.config.mcpServers[t]])
        );
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
        currentLogs.push(logEntry);
        this.sessionConfig.callbacks?.onLog?.(logEntry);
      }

      // Build enhanced system prompt with tool instructions
      const toolInstructions = this.mcpClient.getCombinedInstructions();
      const enhancedSystemPrompt = this.enhanceSystemPrompt(
        this.sessionConfig.systemPrompt, 
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
      currentConversation.push({ role: 'user', content: this.sessionConfig.userPrompt });

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
        accounting: result.accounting
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
      currentLogs.push(logEntry);
      this.sessionConfig.callbacks?.onLog?.(logEntry);
      
      // Log exit for uncaught exception
      this.logExit(
        'EXIT-UNCAUGHT-EXCEPTION',
        `Uncaught exception: ${message}`,
        currentTurn,
        currentLogs
      );

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
    const maxRetries = this.sessionConfig.maxRetries ?? 3;
    const pairs = this.sessionConfig.targets;

    // Turn loop - necessary for control flow with early termination
    // eslint-disable-next-line functional/no-loop-statements
    for (currentTurn = 0; currentTurn < maxTurns; currentTurn++) {
      this.mcpClient.setTurn(currentTurn, 0);
      this.llmClient.setTurn(currentTurn, 0);

      let lastError: string | undefined;
      let turnSuccessful = false;
      let finalTurnWarnLogged = false;

      // Retry loop over all provider/model pairs - necessary for early termination
      // eslint-disable-next-line functional/no-loop-statements
      for (let retry = 0; retry < maxRetries && !turnSuccessful; retry++) {
        // eslint-disable-next-line functional/no-loop-statements
        for (let pairIndex = 0; pairIndex < pairs.length && !turnSuccessful; pairIndex++) {
          const { provider, model } = pairs[pairIndex];
          
          try {
            // Build per-attempt conversation with optional guidance injection
            let attemptConversation = [...conversation];
            // On the last retry within this turn, nudge the model to use tools (not append_notes)
            if (retry === (maxRetries - 1) && currentTurn < (maxTurns - 1)) {
              attemptConversation.push({
                role: 'user',
                content: 'Reminder: do not end with plain text. Use an available tool (excluding `agent_append_notes`) to make progress. When ready to conclude, call the tool `agent_final_report` to provide the final answer.'
              });
            }
            // Do not inject final-turn user message here to avoid duplication.
            // Providers append a single, standardized final-turn instruction.

            const isFinalTurn = currentTurn >= maxTurns - 1;
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
                message: 'Final turn detected: restricting tools to `agent_final_report` and injecting finalization instruction.'
              };
              logs.push(warn);
              this.sessionConfig.callbacks?.onLog?.(warn);
              finalTurnWarnLogged = true;
            }

            this.llmAttempts++;
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
                  ('message' in turnResult.status ? turnResult.status.message : turnResult.status.type) : undefined
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
            logs.push(warnEntry);
            this.sessionConfig.callbacks?.onLog?.(warnEntry);
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
          // Print final output according to expected format
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
                  logs.push(warn);
                  this.sessionConfig.callbacks?.onLog?.(warn);
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
                logs.push(warn);
                this.sessionConfig.callbacks?.onLog?.(warn);
              }
            }
            const out = fr.content_json !== undefined ? JSON.stringify(fr.content_json) : (fr.content ?? '');
            if (out.length > 0) {
              this.sessionConfig.callbacks?.onOutput?.(out);
              if (!out.endsWith('\n')) this.sessionConfig.callbacks?.onOutput?.('\n');
            }
          } else {
            const out = fr.content ?? '';
            if (out.length > 0) {
              this.sessionConfig.callbacks?.onOutput?.(out);
              if (!out.endsWith('\n')) this.sessionConfig.callbacks?.onOutput?.('\n');
            }
          }

          // Log successful exit
          this.logExit(
            'EXIT-FINAL-ANSWER',
            'Final report received (agent_final_report), session complete',
            currentTurn,
            logs
          );

          // Emit FIN summary log entry
          this.emitFinalSummary(logs, accounting);

          return {
            success: true,
            conversation,
            logs,
            accounting
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
                logs.push(debugEntry);
                this.sessionConfig.callbacks?.onLog?.(debugEntry);
                
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
                logs.push(warnEntry);
                this.sessionConfig.callbacks?.onLog?.(warnEntry);
                this.llmSyntheticFailures++;
                lastError = 'invalid_response: empty_without_tools';
                // do not mark turnSuccessful; continue retry loop
                continue;
              }
              
              if (turnResult.status.finalAnswer) {
                // Treat as non-final unless a final_report was provided
                // Continue to next turn to allow the model to call agent_final_report
                turnSuccessful = true;
                break;
              } else {
                // Continue to next turn (tools already executed if hasToolCalls was true)
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
            continue;
          }
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
        
        return {
          success: false,
          error: lastError ?? 'All provider/model targets failed',
          conversation,
          logs,
          accounting
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
      accounting
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
        const key = `${e.provider}/${e.actualProvider ?? 'n/a'}:${e.model}`;
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
      logs.push(fin);
      this.sessionConfig.callbacks?.onLog?.(fin);

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
      const finMcp: LogEntry = {
        timestamp: Date.now(),
        severity: 'FIN',
        turn: this.currentTurn,
        subturn: 0,
        direction: 'response',
        type: 'mcp',
        remoteIdentifier: 'summary',
        fatal: false,
        message: `requests=${String(mcpRequests)}, failed=${String(mcpFailures)}, bytes in=${String(totalToolCharsIn)} out=${String(totalToolCharsOut)}, providers/tools: ${toolPairsStr.length > 0 ? toolPairsStr : 'none'}`,
      };
      logs.push(finMcp);
      this.sessionConfig.callbacks?.onLog?.(finMcp);
    } catch { /* swallow summary errors */ }
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
    const availableTools = [...this.mcpClient.getAllTools(), ...this.getInternalTools()];
    
    // Track if we've shown thinking for this turn
    let shownThinking = false;
    
    // Create tool executor function that delegates to MCP client with accounting
    const toolExecutor = async (toolName: string, parameters: Record<string, unknown>): Promise<string> => {
      const startTime = Date.now();
      try {
        // Intercept internal tools (agent_*)
        if (toolName === 'agent_append_notes') {
          const textParam = (parameters as { text?: unknown }).text;
          const text = typeof textParam === 'string' ? textParam : textParam === undefined ? '' : JSON.stringify(textParam);
          const tags = Array.isArray((parameters as { tags?: unknown }).tags) ? (parameters as { tags?: string[] }).tags : undefined;
          if (text.trim().length > 0) this.internalNotes.push({ text, tags, ts: Date.now() });
          const latency = Date.now() - startTime;
          const accountingEntry: AccountingEntry = {
            type: 'tool', timestamp: startTime, status: 'ok', latency,
            mcpServer: 'agent', command: toolName,
            charactersIn: JSON.stringify(parameters).length, charactersOut: 15,
          };
          accounting.push(accountingEntry);
          this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
          return JSON.stringify({ ok: true, totalNotes: this.internalNotes.length });
        }
        if (toolName === 'agent_final_report') {
          const p = parameters;
          const statusParam = p.status;
          const formatParam = p.format;
          const status = (typeof statusParam === 'string' ? statusParam : 'success') as 'success' | 'failure' | 'partial';
          const format = (typeof formatParam === 'string' ? formatParam : 'markdown') as 'json' | 'markdown' | 'text';
          const content = typeof p.content === 'string' ? p.content : undefined;
          const content_json = (p.content_json !== null && p.content_json !== undefined && typeof p.content_json === 'object') ? (p.content_json as Record<string, unknown>) : undefined;
          const metadata = (p.metadata !== null && p.metadata !== undefined && typeof p.metadata === 'object') ? (p.metadata as Record<string, unknown>) : undefined;
          this.finalReport = { status, format, content, content_json, metadata, ts: Date.now() };
          const latency = Date.now() - startTime;
          const accountingEntry: AccountingEntry = {
            type: 'tool', timestamp: startTime, status: 'ok', latency,
            mcpServer: 'agent', command: toolName,
            charactersIn: JSON.stringify(parameters).length, charactersOut: 12,
          };
          accounting.push(accountingEntry);
          this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
          return JSON.stringify({ ok: true });
        }

        const { result, serverName } = await this.mcpClient.executeToolByName(toolName, parameters);
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
        };
        accounting.push(accountingEntry);
        this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
        
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
          error: errorMsg
        };
        accounting.push(accountingEntry);
        this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
        
        // Re-throw the error - AI SDK's tool execution system will catch this and convert
        // it to a proper 'tool-error' result that gets included in the conversation history.
        // The AI SDK handles thrown errors by creating TypedToolError parts that are surfaced 
        // to the LLM as tool result messages, ensuring failed tools never cause turn failures.
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
            logs.push(thinkingHeader);
            this.sessionConfig.callbacks?.onLog?.(thinkingHeader);
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
        name: 'agent_append_notes',
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

    const exp = this.sessionConfig.expectedOutput;
    const common = {
      status: { enum: ['success', 'failure', 'partial'] },
      metadata: { type: 'object' },
    } as Record<string, unknown>;

    if (exp?.format === 'json') {
      tools.push({
        name: 'agent_final_report',
        description: 'Finalize the task with a JSON report matching the required schema.',
        inputSchema: {
          type: 'object',
          additionalProperties: false,
          required: ['status', 'format', 'content_json'],
          properties: {
            status: common.status,
            format: { const: 'json' },
            content_json: (exp.schema ?? { type: 'object' }),
            metadata: common.metadata,
          },
        },
      });
    } else if (exp?.format === 'text') {
      tools.push({
        name: 'agent_final_report',
        description: 'Finalize the task with a complete plain-text report.',
        inputSchema: {
          type: 'object',
          additionalProperties: false,
          required: ['status', 'format', 'content'],
          properties: {
            status: common.status,
            format: { const: 'text' },
            content: { type: 'string', minLength: 1 },
            metadata: common.metadata,
          },
        },
      });
    } else {
      // default to markdown when unspecified
      tools.push({
        name: 'agent_final_report',
        description: 'Finalize the task with a complete Markdown report.',
        inputSchema: {
          type: 'object',
          additionalProperties: false,
          required: ['status', 'format', 'content'],
          properties: {
            status: common.status,
            format: { const: 'markdown' },
            content: { type: 'string', minLength: 1 },
            metadata: common.metadata,
          },
        },
      });
    }
    return tools;
  }

  private buildInternalToolsInstructions(): string {
    const exp = this.sessionConfig.expectedOutput;
    const lines: string[] = [];
    const FINISH_ONLY = '- Finish ONLY by calling `agent_final_report` exactly once.';
    const ARGS = '- Arguments:';
    const STATUS_LINE = '  - `status`: one of `success`, `failure`, `partial`.';
    lines.push('- Use tool `agent_append_notes` sparingly for brief housekeeping notes; it is not graded and does not count as progress.');
    if (exp?.format === 'json') {
      lines.push(FINISH_ONLY);
      lines.push(ARGS);
      lines.push(STATUS_LINE);
      lines.push('  - `format`: "json".');
      lines.push('  - `content_json`: MUST match the required JSON Schema exactly.');
    } else if (exp?.format === 'text') {
      lines.push(FINISH_ONLY);
      lines.push(ARGS);
      lines.push(STATUS_LINE);
      lines.push('  - `format`: "text".');
      lines.push('  - `content`: complete plain text (no Markdown).');
    } else {
      lines.push(FINISH_ONLY);
      lines.push(ARGS);
      lines.push(STATUS_LINE);
      lines.push('  - `format`: "markdown".');
      lines.push('  - `content`: complete Markdown deliverable.');
    }
    lines.push('- Do NOT end with plain text. The session ends only after `agent_final_report`.');
    return lines.join('\n');
  }
}

// Export session class as main interface
export { AIAgentSession as AIAgent };
