export interface AIAgentCallbacks {
  onLog?: (level: 'debug' | 'info' | 'warn' | 'error', message: string) => void;
  onOutput?: (text: string) => void;
  onAccounting?: (entry: AccountingEntry) => void;
}

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

export interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  cachedTokens?: number;
  totalTokens: number;
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

export interface ProviderConfig {
  apiKey?: string;
  baseUrl?: string;
  headers?: Record<string, string>;
  custom?: Record<string, unknown>;
  mergeStrategy?: "overlay" | "override" | "deep";
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
  };
}
