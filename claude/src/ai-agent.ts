import type { 
  AIAgentSessionConfig, 
  AIAgentResult, 
  ConversationMessage, 
  LogEntry, 
  AccountingEntry,
  Configuration,
  TurnRequest
} from './types.js';

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
    validateProviders(sessionConfig.config, sessionConfig.providers);
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

    const maxTurns = this.sessionConfig.maxTurns ?? 30;
    const maxRetries = this.sessionConfig.maxRetries ?? 3;
    const pairs = this.sessionConfig.models.flatMap((model) => 
      this.sessionConfig.providers.map((provider) => ({ provider, model }))
    );

    // Turn loop - necessary for control flow with early termination
    // eslint-disable-next-line functional/no-loop-statements
    for (currentTurn = 0; currentTurn < maxTurns; currentTurn++) {
      this.mcpClient.setTurn(currentTurn, 0);
      this.llmClient.setTurn(currentTurn, 0);

      let lastError: string | undefined;
      let turnSuccessful = false;

      // Retry loop over all provider/model pairs - necessary for early termination
      // eslint-disable-next-line functional/no-loop-statements
      for (let retry = 0; retry < maxRetries && !turnSuccessful; retry++) {
        // eslint-disable-next-line functional/no-loop-statements
        for (let pairIndex = 0; pairIndex < pairs.length && !turnSuccessful; pairIndex++) {
          const { provider, model } = pairs[pairIndex];
          
          try {
            const turnResult = await this.executeSingleTurn(
              conversation,
              provider,
              model,
              currentTurn >= maxTurns - 1, // isFinalTurn
              currentTurn,
              logs,
              lastShownThinkingHeaderTurn
            );
            
            // Update tracking if thinking was shown
            if (turnResult.shownThinking) {
              lastShownThinkingHeaderTurn = currentTurn;
            }

            // Record accounting
            if (turnResult.tokens !== undefined) {
              const accountingEntry: AccountingEntry = {
                type: 'llm',
                timestamp: Date.now(),
                status: turnResult.status.type === 'success' ? 'ok' : 'failed',
                latency: turnResult.latencyMs,
                provider,
                model,
                tokens: turnResult.tokens,
                error: turnResult.status.type !== 'success' ? 
                  ('message' in turnResult.status ? turnResult.status.message : turnResult.status.type) : undefined
              };
              accounting.push(accountingEntry);
              this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
            }

            // Handle turn result based on status
            if (turnResult.status.type === 'success') {
              // Add new messages to conversation
              conversation.push(...turnResult.messages);
              
              // Debug logging
              if (this.sessionConfig.verbose === true) {
                const hasToolResults = turnResult.messages.some((m: ConversationMessage) => m.role === 'tool');
                const debugEntry: LogEntry = {
                  timestamp: Date.now(),
                  severity: 'VRB',
                  turn: currentTurn,
                  subturn: 0,
                  direction: 'response',
                  type: 'llm',
                  remoteIdentifier: 'debug',
                  fatal: false,
                  message: `Turn result: hasToolCalls=${String(turnResult.status.hasToolCalls)}, hasToolResults=${String(hasToolResults)}, finalAnswer=${String(turnResult.status.finalAnswer)}, response length=${String(turnResult.response?.length ?? 0)}, messages=${String(turnResult.messages.length)}`
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
              
              if (turnResult.status.finalAnswer) {
                // We have a final answer - successful completion
                
                // Log successful exit
                this.logExit(
                  'EXIT-FINAL-ANSWER',
                  'Final answer received, session complete',
                  currentTurn,
                  logs
                );
                
                return {
                  success: true,
                  conversation,
                  logs,
                  accounting
                };
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
        
        const reason = `No LLM response after ${String(maxRetries)} retries across ${String(pairs.length)} provider/model pairs: ${lastError ?? 'All providers and models failed'}`;
        
        this.logExit(
          exitCode,
          reason,
          currentTurn,
          logs
        );
        
        return {
          success: false,
          error: lastError ?? 'All providers and models failed',
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
    
    return {
      success: false,
      error: 'Max tool turns exceeded',
      conversation,
      logs,
      accounting
    };
  }

  private async executeSingleTurn(
    conversation: ConversationMessage[],
    provider: string,
    model: string,
    isFinalTurn: boolean,
    currentTurn: number,
    logs: LogEntry[],
    lastShownThinkingHeaderTurn: number
  ) {
    const availableTools = this.mcpClient.getAllTools();
    
    // Track if we've shown thinking for this turn
    let shownThinking = false;
    
    // Create tool executor function that delegates to MCP client with accounting
    const toolExecutor = async (toolName: string, parameters: Record<string, unknown>): Promise<string> => {
      const startTime = Date.now();
      try {
        const { result, serverName } = await this.mcpClient.executeToolByName(toolName, parameters);
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
          charactersOut: result.length,
        };
        
        this.sessionConfig.callbacks?.onAccounting?.(accountingEntry);
        
        return result;
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
    if (mcpInstructions.trim().length === 0) return systemPrompt;
    return `${systemPrompt}\n\n## TOOLS' INSTRUCTIONS\n\n${mcpInstructions}`;
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
}

// Export session class as main interface
export { AIAgentSession as AIAgent };
