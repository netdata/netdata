/**
 * Core types for the AI Agent system - Production Implementation
 * Following IMPLEMENTATION.md specification exactly
 */

export interface AIAgentCallbacks {
  /** Custom logging handler (replaces stderr output) */
  onLog?: (level: 'debug' | 'info' | 'warn' | 'error', message: string) => void;
  /** Custom output handler (replaces stdout output) */
  onOutput?: (text: string) => void;
  /** Custom accounting handler (replaces JSONL file writing) */
  onAccounting?: (entry: AccountingEntry) => void;
}

/** Configuration schema matching IMPLEMENTATION.md line 18-23 */
export interface Configuration {
  providers: Record<string, ProviderConfig>;
  mcpServers: Record<string, MCPServerConfig>;
  accounting?: {
    file: string;
  };
  defaults?: DefaultsConfig;
}

export interface ProviderConfig {
  apiKey?: string;
  baseUrl?: string;
  headers?: Record<string, string>;
  name?: string;
}

/** MCP Server configuration per IMPLEMENTATION.md line 22 */
export interface MCPServerConfig {
  type: 'stdio' | 'websocket' | 'http' | 'sse';
  command?: string;
  args?: string[];
  url?: string;
  headers?: Record<string, string>;
  env?: Record<string, string>;
  enabled?: boolean;
}

export interface DefaultsConfig {
  llmTimeout?: number;
  toolTimeout?: number;
  temperature?: number;
  topP?: number;
  parallelToolCalls?: boolean;
}

export interface AIAgentOptions {
  configPath?: string;
  llmTimeout?: number;
  toolTimeout?: number;
  maxParallelTools?: number;
  maxConcurrentTools?: number;
  parallelToolCalls?: boolean;
  temperature?: number;
  topP?: number;
  callbacks?: AIAgentCallbacks;
}

export interface AIAgentRunOptions {
  providers: string[];
  models: string[];
  tools: string[];
  systemPrompt: string;
  userPrompt: string;
  conversationHistory?: ConversationMessage[];
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

export interface ToolCall {
  id: string;
  name: string;
  parameters: Record<string, unknown>;
}

export interface ToolResult {
  toolCallId: string;
  result: string;
  success: boolean;
  error?: string;
  metadata?: {
    latency: number;
    charactersIn: number;
    charactersOut: number;
    mcpServer: string;
    command: string;
  };
}

export interface CoreTool {
  name: string;
  description: string;
  parameters: Record<string, unknown>; // JSON schema for tool parameters
}

/** Token usage matching IMPLEMENTATION.md line 94 */
export interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  cachedTokens?: number;
  totalTokens: number;
  /** Provider-specific fields */
  promptTokens?: number;  // OpenAI format
  completionTokens?: number; // OpenAI format
}

export type AccountingEntry = LLMAccountingEntry | ToolAccountingEntry;

export interface BaseAccountingEntry {
  timestamp: number;
  status: 'ok' | 'failed';
  latency: number;
}

export interface LLMAccountingEntry extends BaseAccountingEntry {
  type: 'llm';
  provider: string;
  model: string;
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