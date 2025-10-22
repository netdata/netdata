import type { OutputFormatId } from './formats.js';
import type { SessionNode } from './session-tree.js';
import type { PreloadedSubAgent } from './subagent-registry.js';
import type { ReasoningOutput } from 'ai';
// Core status interfaces for clear decision making
export type TurnStatus = 
  | { type: 'success'; hasToolCalls: boolean; finalAnswer: boolean }
  | { type: 'rate_limit'; retryAfterMs?: number; sources?: string[] }
  | { type: 'auth_error'; message: string }
  | { type: 'model_error'; message: string; retryable: boolean }
  | { type: 'network_error'; message: string; retryable: boolean }
  | { type: 'timeout'; message: string }
  | { type: 'invalid_response'; message: string }
  | { type: 'quota_exceeded'; message: string };

export type ReasoningLevel = 'minimal' | 'low' | 'medium' | 'high';

export type CachingMode = 'none' | 'full';

export type ProviderReasoningValue = string | number;

export type ProviderReasoningMapping =
  | ProviderReasoningValue
  | [ProviderReasoningValue, ProviderReasoningValue, ProviderReasoningValue, ProviderReasoningValue];

export interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  // Back-compat aggregate (keep for summaries); equals cacheReadInputTokens when available
  cachedTokens?: number;
  // Provider-specific cache splits when available
  cacheReadInputTokens?: number;
  cacheWriteInputTokens?: number;
  totalTokens: number;
}

export interface ProviderTurnMetadata {
  actualProvider?: string;
  actualModel?: string;
  reportedCostUsd?: number;
  upstreamCostUsd?: number;
  cacheWriteInputTokens?: number;
}

export interface TurnRetryDirective {
  action: 'retry' | 'skip-provider' | 'abort';
  backoffMs?: number;
  logMessage?: string;
  systemMessage?: string;
  sources?: string[];
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
  stopReason?: string;
  providerMetadata?: ProviderTurnMetadata;
  retry?: TurnRetryDirective;
}

export type ToolStatus =
  | { type: 'success' }
  | { type: 'mcp_server_error'; serverName: string; message: string }
  | { type: 'tool_not_found'; toolName: string; serverName: string }
  | { type: 'invalid_parameters'; toolName: string; message: string }
  | { type: 'execution_error'; toolName: string; message: string }
  | { type: 'timeout'; toolName: string; timeoutMs: number }
  | { type: 'connection_error'; serverName: string; message: string };

interface ToolMetadata {
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
  // Stable hierarchical path label tied to opTree node (e.g., 1.2 or 1.2.1.1)
  path?: string;
  direction: 'request' | 'response';    // Request or response
  type: 'llm' | 'tool';                 // Operation type (llm = model-side, tool = any tool-side)
  // Optional precise tool kind for 'tool' logs.
  toolKind?: 'mcp' | 'rest' | 'agent' | 'command';
  remoteIdentifier: string;             // 'provider:model' or 'mcp-server:tool-name'
  fatal: boolean;                       // True if this caused agent to stop
  message: string;                      // Human readable message
  // Optional emphasis hint for TTY renderers (bold in same severity color)
  bold?: boolean;
  // Optional headend identifier for multi-headend logging (e.g., "mcp:stdio")
  headendId?: string;
  // Optional tracing fields (multi-agent)
  agentId?: string;
  callPath?: string;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
  // Optional planning fields for richer live status in UIs (Slack/web)
  // Enriched on existing log events (no new event types):
  // - max_turns: declared maximum number of turns for this agent session
  // - max_subturns: number of tool calls requested by the LLM for the current turn (when known)
  'max_turns'?: number;
  'max_subturns'?: number;
}

export interface ProgressMetrics {
  durationMs?: number;
  tokensIn?: number;
  tokensOut?: number;
  tokensCacheRead?: number;
  tokensCacheWrite?: number;
  toolsRun?: number;
  costUsd?: number;
  latencyMs?: number;
  charactersIn?: number;
  charactersOut?: number;
  agentsRun?: number;
}

interface AgentProgressBase {
  type: 'agent_started' | 'agent_update' | 'agent_finished' | 'agent_failed';
  callPath: string;
  agentId: string;
  agentName?: string;
  timestamp: number;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
}

export interface AgentStartedEvent extends AgentProgressBase {
  type: 'agent_started';
  reason?: string;
}

export interface AgentUpdateEvent extends AgentProgressBase {
  type: 'agent_update';
  message: string;
}

export interface AgentFinishedEvent extends AgentProgressBase {
  type: 'agent_finished';
  metrics?: ProgressMetrics;
}

export interface AgentFailedEvent extends AgentProgressBase {
  type: 'agent_failed';
  metrics?: ProgressMetrics;
  error?: string;
}

interface ToolProgressBase {
  callPath: string;
  agentId: string;
  timestamp: number;
  tool: { name: string; provider: string };
  metrics?: ProgressMetrics;
  error?: string;
}

export interface ToolStartedEvent extends ToolProgressBase {
  type: 'tool_started';
}

export interface ToolFinishedEvent extends ToolProgressBase {
  type: 'tool_finished';
  status: 'ok' | 'failed';
}

export type ProgressEvent =
  | AgentStartedEvent
  | AgentUpdateEvent
  | AgentFinishedEvent
  | AgentFailedEvent
  | ToolStartedEvent
  | ToolFinishedEvent;

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
  reasoning?: ReasoningOutput[];
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
  toolsAllowed?: string[];
  toolsDenied?: string[];
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
export interface ProviderModelOverrides {
  temperature?: number | null;
  topP?: number | null;
  top_p?: number | null;
}

export interface ProviderModelConfig {
  overrides?: ProviderModelOverrides;
  reasoning?: ProviderReasoningMapping | null;
}

export interface ProviderConfig {
  apiKey?: string;
  baseUrl?: string;
  headers?: Record<string, string>;
  custom?: Record<string, unknown>;
  mergeStrategy?: "overlay" | "override" | "deep";
  type: 'openai' | 'anthropic' | 'google' | 'openrouter' | 'ollama' | 'test-llm';
  openaiMode?: 'responses' | 'chat';
  models?: Record<string, ProviderModelConfig>;
  toolsAllowed?: string[];
  toolsDenied?: string[];
  stringSchemaFormatsAllowed?: string[];
  stringSchemaFormatsDenied?: string[];
  reasoning?: ProviderReasoningMapping | null;
  reasoningValue?: ProviderReasoningValue | null;
  reasoningAutoStreamLevel?: ReasoningLevel;
}

export interface Configuration {
  providers: Record<string, ProviderConfig>;
  mcpServers: Record<string, MCPServerConfig>;
  // Optional REST tools registry (manifest-driven)
  restTools?: Record<string, RestToolConfig>;
  // Optional OpenAPI specs to auto-generate REST tools
  openapiSpecs?: Record<string, OpenAPISpecConfig>;
  accounting?: { file: string };
  // Persistence configuration
  persistence?: {
    sessionsDir?: string;
    billingFile?: string;
  };
  // Optional pricing table by provider/model. Prices are per "unit" tokens (1k or 1m).
  pricing?: Record<string, Record<string, {
    unit?: 'per_1k' | 'per_1m';
    currency?: 'USD';
    // Base token rates
    prompt?: number;        // input
    completion?: number;    // output
    // Cache-specific when available
    cacheRead?: number;     // cached input
    cacheWrite?: number;    // cache creation input
  }>>;
  defaults?: {
    llmTimeout?: number;
    toolTimeout?: number;
    temperature?: number;
    topP?: number;
    parallelToolCalls?: boolean;
    maxToolTurns?: number;
    maxToolCallsPerTurn?: number;
    stream?: boolean;
    maxRetries?: number;
    // Maximum allowed MCP tool response size in bytes. If exceeded, a tool error is injected.
    toolResponseMaxBytes?: number;
    mcpInitConcurrency?: number;
    maxConcurrentTools?: number;
    outputFormat?: 'markdown' | 'markdown+mermaid' | 'slack-block-kit' | 'tty' | 'pipe' | 'json' | 'sub-agent';
    formats?: {
      cli?: 'markdown' | 'markdown+mermaid' | 'slack-block-kit' | 'tty' | 'pipe' | 'json' | 'sub-agent';
      slack?: 'markdown' | 'markdown+mermaid' | 'slack-block-kit' | 'tty' | 'pipe' | 'json' | 'sub-agent';
      api?: 'markdown' | 'markdown+mermaid' | 'slack-block-kit' | 'tty' | 'pipe' | 'json' | 'sub-agent';
      web?: 'markdown' | 'markdown+mermaid' | 'slack-block-kit' | 'tty' | 'pipe' | 'json' | 'sub-agent';
      subAgent?: 'markdown' | 'markdown+mermaid' | 'slack-block-kit' | 'tty' | 'pipe' | 'json' | 'sub-agent';
    };
    reasoningValue?: ProviderReasoningValue | null;
  };
  // Server headend configuration (optional)
  slack?: {
    enabled?: boolean;
    mentions?: boolean; // handle app_mention in channels
    dms?: boolean;      // handle message.im in DMs
    updateIntervalMs?: number; // throttled progress update interval
    historyLimit?: number;     // messages to prefetch
    historyCharsCap?: number;  // cap for prefetched context (characters)
    botToken?: string;         // ${SLACK_BOT_TOKEN}
    appToken?: string;         // ${SLACK_APP_TOKEN} (Socket Mode)
    openerTone?: 'random' | 'cheerful' | 'formal' | 'busy';
    signingSecret?: string;    // ${SLACK_SIGNING_SECRET} for slash commands
    // Optional per-channel routing rules
    routing?: {
      default?: {
        agent: string; // path to .ai file
        engage?: ('mentions' | 'channel-posts' | 'dms')[];
        promptTemplates?: Partial<{
          mention: string;
          dm: string;
          channelPost: string; // template for channel-posts
        }>;
        contextPolicy?: Partial<{
          channelPost: 'selfOnly' | 'previousOnly' | 'selfAndPrevious';
        }>;
      };
      rules?: {
        channels: string[]; // accepts #name, C/G ids, supports wildcards (*, ?)
        agent: string; // path to .ai file
        engage?: ('mentions' | 'channel-posts' | 'dms')[];
        promptTemplates?: Partial<{
          mention: string;
          dm: string;
          channelPost: string;
        }>;
        contextPolicy?: Partial<{
          channelPost: 'selfOnly' | 'previousOnly' | 'selfAndPrevious';
        }>;
      }[];
      deny?: {
        channels: string[];
        engage?: ('mentions' | 'channel-posts' | 'dms')[];
      }[];
    };
  };
  api?: {
    enabled?: boolean;
    port?: number;             // ${PORT}
    bearerKeys?: string[];     // e.g., ["${API_BEARER_TOKEN}"] or multiple
  };
}

// REST tool configuration (minimal phase 1)
export interface RestToolConfig {
  description: string;
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH';
  url: string;
  headers?: Record<string, string>;
  // JSON Schema for parameters (Ajv-compatible)
  parametersSchema: Record<string, unknown>;
  // Templated JSON body for POST/PUT/PATCH (substitute ${parameters.*})
  bodyTemplate?: unknown;
  // Optional streaming config for SSE/JSON-lines
  streaming?: {
    mode: 'json-stream';
    linePrefix?: string; // e.g., 'data:'
    discriminatorField: string; // e.g., 'type'
    doneValue: string;         // e.g., 'done'
    answerField?: string;      // e.g., 'answer'
    tokenValue?: string;       // e.g., 'token'
    tokenField?: string;       // e.g., 'content'
    timeoutMs?: number;
    maxBytes?: number;
  };
}

// OpenAPI spec registration in config
export interface OpenAPISpecConfig {
  // Path or URL to the OpenAPI document (YAML or JSON)
  spec: string;
  // Optional override when the spec does not have servers[] or you want to change it
  baseUrl?: string;
  // Default headers applied to every tool generated (supports ${VAR} expansion)
  headers?: Record<string, string>;
  // Optional filters
  includeMethods?: ('get'|'post'|'put'|'patch'|'delete')[];
  tagFilter?: string[];
}

// Session persistence payloads
export interface SessionSnapshotPayload {
  reason?: string;
  sessionId: string;
  originId: string;
  timestamp: number;
  snapshot: { version: number; opTree: unknown };
}

export interface AccountingFlushPayload {
  sessionId: string;
  originId: string;
  timestamp: number;
  entries: AccountingEntry[];
}

// Session configuration and callbacks
export interface AIAgentCallbacks {
  onLog?: (entry: LogEntry) => void;
  onOutput?: (text: string) => void;
  onThinking?: (text: string) => void;
  onAccounting?: (entry: AccountingEntry) => void;
  onSessionSnapshot?: (payload: SessionSnapshotPayload) => void | Promise<void>;
  onAccountingFlush?: (payload: AccountingFlushPayload) => void | Promise<void>;
  onProgress?: (event: ProgressEvent) => void;
  // Live snapshot of the hierarchical operation tree (Option C)
  onOpTree?: (tree: unknown) => void;
}

export interface AIAgentSessionConfig {
  config: Configuration;
  targets: { provider: string; model: string }[];
  tools: string[];
  // Optional preloaded sub-agent definitions (fully resolved at bootstrap)
  subAgents?: PreloadedSubAgent[];
  // Agent identity (prompt path or synthetic id)
  agentId?: string;
  systemPrompt: string;
  userPrompt: string;
  // Resolved output format for this session (must be provided by caller)
  outputFormat: OutputFormatId;
  // Optional rendering target for diagnostics
  renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent';
  conversationHistory?: ConversationMessage[];
  // Expected output contract parsed from prompt frontmatter
  expectedOutput?: { format: 'json' | 'markdown' | 'text'; schema?: Record<string, unknown> };
  temperature?: number;
  topP?: number;
  maxOutputTokens?: number;
  repeatPenalty?: number;
  maxRetries?: number;
  maxTurns?: number;
  maxToolCallsPerTurn?: number;
  llmTimeout?: number;
  toolTimeout?: number;
  parallelToolCalls?: boolean;
  stream?: boolean;
  maxConcurrentTools?: number;
  callbacks?: AIAgentCallbacks;
  traceLLM?: boolean;
  traceMCP?: boolean;
  verbose?: boolean;
  reasoning?: ReasoningLevel;
  reasoningValue?: ProviderReasoningValue | null;
  caching?: CachingMode;
  // Optional pre-set session title (does not consume a tool turn)
  initialTitle?: string;
  // Enforced cap for MCP tool response size (bytes)
  toolResponseMaxBytes?: number;
  // Preferred MCP init concurrency override for this session
  mcpInitConcurrency?: number;
  // Trace context propagation
  trace?: { selfId?: string; originId?: string; parentId?: string; callPath?: string };
  // External cancellation signal to abort the session immediately
  abortSignal?: AbortSignal;
  // Graceful stop reference toggled by headend (no abort); agent should avoid starting new work
  stopRef?: { stopping: boolean };
  // Ancestors chain of sub-agent prompt paths (for recursion prevention across nested sessions)
  ancestors?: string[];
}

// Session result
export interface AIAgentResult {
  success: boolean;
  error?: string;
  conversation: ConversationMessage[];
  logs: LogEntry[];
  accounting: AccountingEntry[];
  // Optional ASCII representation of the internal execution tree (when requested by CLI)
  treeAscii?: string;
  // Optional ASCII representation of the new hierarchical operation tree (Option C)
  opTreeAscii?: string;
  // Optional hierarchical operation tree structure (Option C)
  opTree?: SessionNode;
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
    // Allow all known output formats plus legacy 'text'
    format: 'json' | 'markdown' | 'markdown+mermaid' | 'slack-block-kit' | 'tty' | 'pipe' | 'sub-agent' | 'text';
    content?: string;
    content_json?: Record<string, unknown>;
    // Optional provider-specific extras (e.g., Slack Block Kit messages)
    metadata?: Record<string, unknown>;
    ts: number;
  };
}

// Accounting system
export type AccountingEntry = LLMAccountingEntry | ToolAccountingEntry;

interface BaseAccountingEntry {
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
  stopReason?: string;
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
type ToolExecutor = (
  toolName: string,
  parameters: Record<string, unknown>,
  options?: { toolCallId?: string }
) => Promise<string>;

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
  toolChoiceRequired?: boolean;
  maxConcurrentTools?: number;
  isFinalTurn?: boolean;
  llmTimeout?: number;
  turnMetadata?: {
    attempt: number;
    maxAttempts: number;
    turn: number;
    isFinalTurn: boolean;
    reasoningLevel?: ReasoningLevel;
    reasoningValue?: ProviderReasoningValue | null;
  };
  // External cancellation signal to immediately abort LLM calls
  abortSignal?: AbortSignal;
  onChunk?: (chunk: string, type: 'content' | 'thinking') => void;
  reasoningLevel?: ReasoningLevel;
  reasoningValue?: ProviderReasoningValue | null;
  caching?: CachingMode;
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
  // Optional pre-set session title (does not consume a tool turn)
  initialTitle?: string;
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
