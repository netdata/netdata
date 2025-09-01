// Main library exports for programmatic use
export { AIAgent, AIAgentSession } from './ai-agent.js';
export { LLMClient } from './llm-client.js';
export { MCPClientManager } from './mcp-client.js';
export { loadConfiguration } from './config.js';

// Type exports
export type {
  AIAgentSessionConfig,
  AIAgentResult,
  AIAgentCallbacks,
  AIAgentOptions,
  AIAgentRunOptions,
  Configuration,
  ProviderConfig,
  MCPServerConfig,
  MCPTool,
  MCPServer,
  ConversationMessage,
  ToolCall,
  ToolResult,
  TokenUsage,
  TurnRequest,
  TurnResult,
  TurnStatus,
  ToolStatus,
  LogEntry,
  AccountingEntry,
  LLMAccountingEntry,
  ToolAccountingEntry,
  LLMProvider,
} from './types.js';