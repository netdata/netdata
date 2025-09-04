// Core status interfaces for clear decision making
export type TurnStatus = 
  | { type: 'success'; hasToolCalls: boolean; finalAnswer: boolean }
  | { type: 'rate_limit'; retryAfterMs?: number }
  | { type: 'auth_error'; message: string }
  | { type: 'model_error'; message: string; retryable: boolean }
  | { type: 'network_error'; message: string; retryable: boolean }
  | { type: 'timeout'; message: string }
  | { type: 'invalid_response'; message: string }
  | { type: 'quota_exceeded'; message: string };

export interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  cachedTokens?: number;
  totalTokens: number;
}

export interface TurnResult {
  status: TurnStatus;
  response?: string;
  toolCalls?: ToolCall[];
  tokens?: TokenUsage;
  latencyMs: number;
  messages: ConversationMessage[];
  hasReasoning?: boolean;
  hasContent?: boolean;
}

export type ToolStatus =
  | { type: 'success' }
  | { type: 'mcp_server_error'; serverName: string; message: string }
  | { type: 'tool_not_found'; toolName: string; serverName: string }
  | { type: 'invalid_parameters'; toolName: string; message: string }
  | { type: 'execution_error'; toolName: string; message: string }
  | { type: 'timeout'; toolName: string; timeoutMs: number }
  | { type: 'connection_error'; serverName: string; message: string };

export interface ToolMetadata {
  latency: number;
  charactersIn: number;
  charactersOut: number;
  mcpServer: string;
  command: string;
}

export interface ToolResult {
  toolCallId: string;
  status: ToolStatus;
  result: string;
  latencyMs: number;
  metadata: ToolMetadata;
}

// Structured logging interface
export interface LogEntry {
  timestamp: number;                    // Unix timestamp (ms)
  severity: 'VRB' | 'WRN' | 'ERR' | 'TRC' | 'THK' | 'FIN'; // FIN for end-of-run summary
  turn: number;                         // Sequential turn ID  
  subturn: number;                      // Sequential tool ID within turn
  direction: 'request' | 'response';    // Request or response
  type: 'llm' | 'mcp';                 // Operation type
  remoteIdentifier: string;             // 'provider:model' or 'mcp-server:tool-name'
  fatal: boolean;                       // True if this caused agent to stop
  message: string;                      // Human readable message
  // Optional tracing fields (multi-agent)
  agentId?: string;
  callPath?: string;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
}

// Core data structures
export interface ToolCall {
  id: string;
  name: string;
  parameters: Record<string, unknown>;
}

export interface ConversationMessage {
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
  toolCalls?: ToolCall[];
  toolCallId?: string;
  metadata?: {
    provider?: string;
    model?: string;
    tokens?: TokenUsage;
    timestamp?: number;
  };
}

// MCP configuration
export interface MCPServerConfig {
  type: 'stdio' | 'websocket' | 'http' | 'sse';
  command?: string;
  args?: string[];
  url?: string;
  headers?: Record<string, string>;
  env?: Record<string, string>;
  enabled?: boolean;
  toolSchemas?: Record<string, unknown>;
}

export interface MCPTool {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
  instructions?: string;
}

export interface MCPServer {
  name: string;
  config: MCPServerConfig;
  tools: MCPTool[];
  instructions: string;
}

// Provider configuration
export interface ProviderConfig {
  apiKey?: string;
  baseUrl?: string;
  headers?: Record<string, string>;
  custom?: Record<string, unknown>;
  mergeStrategy?: "overlay" | "override" | "deep";
  type?: 'openai' | 'anthropic' | 'google' | 'openrouter' | 'ollama';
  openaiMode?: 'responses' | 'chat';
}

export interface Configuration {
  providers: Record<string, ProviderConfig>;
  mcpServers: Record<string, MCPServerConfig>;
  accounting?: { file: string };
  defaults?: {
    llmTimeout?: number;
    toolTimeout?: number;
    temperature?: number;
    topP?: number;
    parallelToolCalls?: boolean;
    maxToolTurns?: number;
    stream?: boolean;
    maxRetries?: number;
    // Maximum allowed MCP tool response size in bytes. If exceeded, a tool error is injected.
    toolResponseMaxBytes?: number;
    mcpInitConcurrency?: number;
    maxConcurrentTools?: number;
  };
}

// Session configuration and callbacks
export interface AIAgentCallbacks {
  onLog?: (entry: LogEntry) => void;
  onOutput?: (text: string) => void;
  onThinking?: (text: string) => void;
  onAccounting?: (entry: AccountingEntry) => void;
}

export interface AIAgentSessionConfig {
  config: Configuration;
  targets: { provider: string; model: string }[];
  tools: string[];
  // Optional sub-agent prompt file paths (relative or absolute)
  subAgentPaths?: string[];
  // Agent identity (prompt path or synthetic id)
  agentId?: string;
  systemPrompt: string;
  userPrompt: string;
  conversationHistory?: ConversationMessage[];
  // Expected output contract parsed from prompt frontmatter
  expectedOutput?: { format: 'json' | 'markdown' | 'text'; schema?: Record<string, unknown> };
  temperature?: number;
  topP?: number;
  maxOutputTokens?: number;
  repeatPenalty?: number;
  maxRetries?: number;
  maxTurns?: number;
  llmTimeout?: number;
  toolTimeout?: number;
  parallelToolCalls?: boolean;
  stream?: boolean;
  maxConcurrentTools?: number;
  callbacks?: AIAgentCallbacks;
  traceLLM?: boolean;
  traceMCP?: boolean;
  verbose?: boolean;
  // Enforced cap for MCP tool response size (bytes)
  toolResponseMaxBytes?: number;
  // Preferred MCP init concurrency override for this session
  mcpInitConcurrency?: number;
  // Trace context propagation
  trace?: { selfId?: string; originId?: string; parentId?: string; callPath?: string };
}

// Session result
export interface AIAgentResult {
  success: boolean;
  error?: string;
  conversation: ConversationMessage[];
  logs: LogEntry[];
  accounting: AccountingEntry[];
  // Conversations of executed sub-agents (when available)
  childConversations?: {
    agentId?: string;
    toolName: string;
    promptPath: string;
    trace?: { originId?: string; parentId?: string; selfId?: string; callPath?: string };
    conversation: ConversationMessage[];
  }[];
  // Isolated final report returned by the model via agent_final_report, when available
  finalReport?: {
    status: 'success' | 'failure' | 'partial';
    format: 'json' | 'markdown' | 'text';
    content?: string;
    content_json?: Record<string, unknown>;
    metadata?: Record<string, unknown>;
    ts: number;
  };
}

// Accounting system
export type AccountingEntry = LLMAccountingEntry | ToolAccountingEntry;

export interface BaseAccountingEntry {
  timestamp: number;
  status: 'ok' | 'failed';
  latency: number;
  // Optional tracing fields (multi-agent)
  agentId?: string;
  callPath?: string;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
}

export interface LLMAccountingEntry extends BaseAccountingEntry {
  type: 'llm';
  provider: string;
  model: string;
  // Optional: actual provider used by a router (e.g., OpenRouter -> Fireworks/Cerebras/etc.)
  actualProvider?: string;
  actualModel?: string;
  // Optional: cost as reported by provider (e.g., OpenRouter)
  costUsd?: number;
  upstreamInferenceCostUsd?: number;
  tokens: TokenUsage;
  error?: string;
}

export interface ToolAccountingEntry extends BaseAccountingEntry {
  type: 'tool';
  mcpServer: string;
  command: string;
  charactersIn: number;
  charactersOut: number;
  error?: string;
}

// Tool executor function type
export type ToolExecutor = (toolName: string, parameters: Record<string, unknown>) => Promise<string>;

// LLM interfaces
export interface TurnRequest {
  messages: ConversationMessage[];
  provider: string;
  model: string;
  tools: MCPTool[];
  toolExecutor: ToolExecutor;
  temperature?: number;
  topP?: number;
  maxOutputTokens?: number;
  repeatPenalty?: number;
  parallelToolCalls?: boolean;
  stream?: boolean;
  maxConcurrentTools?: number;
  isFinalTurn?: boolean;
  llmTimeout?: number;
  onChunk?: (chunk: string, type: 'content' | 'thinking') => void;
}

export interface LLMProvider {
  name: string;
  executeTurn: (request: TurnRequest) => Promise<TurnResult>;
}

// CLI types for backward compatibility
export interface AIAgentOptions {
  configPath?: string;
  llmTimeout?: number;
  toolTimeout?: number;
  temperature?: number;
  topP?: number;
  callbacks?: AIAgentCallbacks;
  traceLLM?: boolean;
  traceMCP?: boolean;
  parallelToolCalls?: boolean;
  maxToolTurns?: number;
  verbose?: boolean;
  stream?: boolean;
  maxRetries?: number;
}

export interface AIAgentRunOptions {
  providers: string[];
  models: string[];
  tools: string[];
  systemPrompt: string;
  userPrompt: string;
  conversationHistory?: ConversationMessage[];
  saveConversation?: string;
  loadConversation?: string;
  dryRun?: boolean;
}
