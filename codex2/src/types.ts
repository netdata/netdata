export interface AIAgentCallbacks {
  onLog?: (entry: LogEntry) => void;
  onOutput?: (text: string) => void;
  onAccounting?: (entry: AccountingEntry) => void;
}

export type Severity = 'VRB' | 'WRN' | 'ERR' | 'TRC';
export type Direction = 'request' | 'response';

export interface LogEntry {
  timestamp: number;
  severity: Severity;
  turn: number;
  subturn: number;
  direction: Direction;
  type: 'llm' | 'mcp';
  remoteIdentifier: string; // provider:model or mcp-server:tool
  fatal: boolean;
  message: string;
}

export interface AIAgentCreateOptions {
  config: Configuration;
  providers: string[];
  models: string[];
  tools: string[];
  systemPrompt: string;
  userPrompt: string;
  conversationHistory?: ConversationMessage[];
  llmTimeout?: number;
  toolTimeout?: number;
  temperature?: number;
  topP?: number;
  parallelToolCalls?: boolean;
  verbose?: boolean;
  stream?: boolean;
  maxRetries?: number;
  maxToolTurns?: number;
  callbacks?: AIAgentCallbacks;
  traceLLM?: boolean;
  traceMCP?: boolean;
}

export interface AIAgentRunResult {
  success: boolean;
  error?: string;
  conversation: ConversationMessage[];
  logs: LogEntry[];
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

export interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  cachedTokens?: number;
}

export interface AccountingEntryLLM {
  type: 'llm';
  timestamp: number;
  status: 'ok' | 'failed';
  latency: number;
  provider: string;
  model: string;
  tokens: TokenUsage;
  error?: string;
}

export interface AccountingEntryTool {
  type: 'tool';
  timestamp: number;
  status: 'ok' | 'failed';
  latency: number;
  mcpServer: string;
  command: string;
  charactersIn: number;
  charactersOut: number;
  error?: string;
}

export type AccountingEntry = AccountingEntryLLM | AccountingEntryTool;

export interface ProviderConfig {
  apiKey?: string;
  baseUrl?: string;
  headers?: Record<string, string>;
  custom?: Record<string, unknown>;
  mergeStrategy?: 'overlay' | 'override' | 'deep';
  type?: 'openai' | 'anthropic' | 'google' | 'openrouter' | 'ollama';
  openaiMode?: 'responses' | 'chat';
}

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

export interface Configuration {
  providers: Record<string, ProviderConfig>;
  mcpServers: Record<string, MCPServerConfig>;
  accounting?: { file: string };
  defaults?: {
    llmTimeout?: number;
    toolTimeout?: number;
    temperature?: number;
    topP?: number;
    stream?: boolean;
    parallelToolCalls?: boolean;
    maxToolTurns?: number;
    maxRetries?: number;
  };
}

// LLM single turn
export type TurnStatus =
  | { type: 'success'; hasToolCalls: boolean; finalAnswer: boolean }
  | { type: 'rate_limit'; retryAfterMs?: number }
  | { type: 'auth_error'; message: string }
  | { type: 'model_error'; message: string; retryable: boolean }
  | { type: 'network_error'; message: string; retryable: boolean }
  | { type: 'timeout'; message: string }
  | { type: 'invalid_response'; message: string }
  | { type: 'quota_exceeded'; message: string };

export interface ToolCall {
  id: string;
  name: string;
  parameters: Record<string, unknown>;
}

export interface TurnRequest {
  provider: string;
  model: string;
  messages: ConversationMessage[];
  tools: { name: string; description?: string; inputSchema: Record<string, unknown> }[];
  isFinalTurn: boolean;
  temperature: number;
  topP: number;
  stream: boolean;
  llmTimeout: number;
  providerOptions?: Record<string, unknown>;
  onOutput?: (text: string) => void;
  onReasoning?: (text: string) => void;
  onAccounting?: (entry: AccountingEntry) => void;
  log?: (entry: LogEntry) => void;
  toolExecutor: (call: ToolCall) => Promise<{ ok: boolean; text: string; serverName?: string; error?: string; latency?: number; inSize?: number; outSize?: number }>;
  // Provider factory for this request
  getModel: (name: string) => LanguageModel;
}

export interface TurnResult {
  status: TurnStatus;
  responseMessages: ConversationMessage[];
  tokens?: TokenUsage;
  latencyMs: number;
  executedTools: { name: string; output: string }[];
}
import type { LanguageModel } from 'ai';
