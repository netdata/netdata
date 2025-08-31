# AI Agent - Universal LLM Tool Calling Interface

A TypeScript-based command-line tool and library for interacting with Large Language Models (LLMs) with Model Context Protocol (MCP) tool integration.

## Overview

The AI Agent provides a unified interface to interact with multiple LLM providers while seamlessly integrating with MCP (Model Context Protocol) tools. It supports streaming responses, parallel tool execution, and comprehensive accounting. Configuration lives in a JSON file; by default the local filename is `.ai-agent.json`.

## High-Level Operation

1. **Configuration**: All settings are stored in a JSON config (default `.ai-agent.json`) including provider API keys and MCP server definitions
2. **Execution**: Command-line interface accepts providers, models, MCP tools, and prompts (positionals), plus optional flags
3. **Bootstrap**: Validates config and initializes MCP servers. Initialization is non-fatal: failures are logged and the agent can still proceed to the LLM. Use `--trace-mcp` to inspect initialization details. In `--dry-run`, both MCP spawn and LLM calls are skipped.
4. **Processing**: Sends requests to LLMs with available tools (schemas). The LLM orchestrates repeated tool calls. The agent preserves assistant tool_calls and tool results in history and loops until completion (see Agentic Behavior)
5. **Output**: Streams LLM responses to stdout in real time, logs errors to stderr

## Agentic Behavior

- The agent is fully agentic: the LLM decides when to call tools, with repeated invocations across multiple turns.
- Tool calls are preserved as assistant messages with `tool_calls` (id, name, arguments). Tool results are preserved as `tool` role messages with `tool_call_id`.
- The next turn always includes all prior assistant/user/tool messages in order, giving the LLM full transparency into requests and responses.
- A maximum tool-turns cap (`defaults.maxToolTurns`) is enforced. On the final allowed turn, tools are disabled and a single user message is appended instructing the LLM to conclude using existing tool results (see Hardcoded Strings). This guarantees a final answer without an error.

## Hardcoded Strings (LLM-facing)

The application only injects the following LLM-facing strings:

1. `## TOOLS' INSTRUCTIONS`
2. `## TOOL {name} INSTRUCTIONS` (per tool instructions)
3. Final-turn message when tools are no longer permitted: "You are not allowed to run any more tools. Use the tool responses you have so far to answer my original question. If you failed to find answers for something, please state the areas you couldn't investigate"

No other hardcoded content is sent to the LLM.

## Command Line Interface

```bash
ai-agent [options] provider(s) model(s) mcp-tool(s) system-prompt user-prompt
```

### Parameters (Positionals vs Flags)

- Positionals (in order; required unless noted):
  - **provider(s)**: Comma-separated list of LLM providers (must be defined in config)
  - **model(s)**: Comma-separated list of model names (tried sequentially; see fallback)
  - **mcp-tool(s)**: Comma-separated list of MCP tools (must be defined in config)
  - **system-prompt**: System prompt string or `@filename` or `-` for stdin
  - **user-prompt**: User prompt string or `@filename` or `-` for stdin
- Flags: Any parameter starting with `--` is a non-positional option (see below).

### Example Usage

```bash
# Basic usage with OpenAI
ai-agent openai gpt-4o-mini file-operations "You are a helpful assistant" "List files in current directory"

# Multiple providers with fallback
ai-agent openai,anthropic gpt-4o,claude-3-sonnet netdata-tools @system.txt "What's the CPU usage?"

# Using stdin for prompt
echo "Analyze this log file" | ai-agent openai gpt-4o file-ops @system.txt -
```

## Configuration File (.ai-agent.json)

The configuration file must contain providers and MCP tools. Models are specified at runtime. The default local filename is `.ai-agent.json`.

### Configuration File Resolution
The configuration file is resolved in the following order:
1. `--config <filename>` command line option
2. `.ai-agent.json` in current directory  
3. `~/.ai-agent.json` in home directory
4. If none found, the program fails with error

### Environment Variable Expansion
All string values in the configuration support environment variable expansion using `${VARIABLE_NAME}` syntax:

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}",
      "baseUrl": "https://api.openai.com/v1"
    },
    "anthropic": {
      "apiKey": "${ANTHROPIC_API_KEY}",
      "baseUrl": "https://api.anthropic.com/v1"
    },
    "ollama": {
      "baseUrl": "http://localhost:11434"
    }
  },
  "mcpServers": {
    "file-operations": {
      "type": "stdio",
      "command": "/usr/local/bin/file-mcp-server",
      "args": []
    },
    "netdata-tools": {
      "type": "websocket",
      "url": "ws://localhost:8080/mcp",
      "headers": {
        "Authorization": "Bearer ${NETDATA_TOKEN}"
      }
    },
    "remote-api": {
      "type": "http",
      "url": "https://api.example.com/mcp",
      "headers": {
        "X-API-Key": "${REMOTE_API_KEY}"
      }
    }
  },
  "accounting": {
    "file": "${HOME}/ai-agent-accounting.jsonl"
  },
  "defaults": {
    "llmTimeout": 120000,
    "toolTimeout": 60000,
    "temperature": 0.7,
    "topP": 1.0,
    "parallelToolCalls": true,
    "stream": true
  }
}
```

### Command Line vs Configuration Priority
- **Flags override config**: Command line options override configuration file settings
- **Defaults in config**: All timeout, limit, and model parameters can be set in the config file under `"defaults"`
- **Positionals only via CLI**: Providers, models, MCP tools, and prompts are specified via positional CLI arguments (not in the config)

## Command Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `--llm-timeout <ms>` | Inactivity timeout per LLM call (resets on each streamed chunk) | 120000 |
| `--tool-timeout <ms>` | Timeout for tool execution | 60000 |
| `--trace-llm` | Trace LLM HTTP requests and responses (Authorization redacted) | off |
| `--trace-mcp` | Trace MCP connect, tools/prompts list, stderr, callTool | off |
| `--parallel-tool-calls` | Enable parallel tool calls (OpenAI-compatible) | true (default) |
| `--no-parallel-tool-calls` | Disable parallel tool calls | - |
| `--temperature <n>` | LLM temperature (0.0-2.0) | 0.7 |
| `--top-p <n>` | LLM top-p sampling (0.0-1.0) | 1.0 |
| `--stream` | Force streaming responses | on by default |
| `--no-stream` | Disable streaming (use non-streaming responses) | - |
| `--save <filename>` | Save conversation to JSON file | - |
| `--load <filename>` | Load conversation from JSON file | - |
| `--config <filename>` | Configuration file path | See resolution order |
| `--accounting <filename>` | Override accounting file from config | - |
| `--dry-run` | Validate inputs only; skip MCP spawn and LLM | - |

Notes:
- `--save/--load`: When loading a conversation, the system prompt is replaced by the provided system prompt (or loaded one if none provided) and the new user prompt is appended. MCP tool instructions are appended to the system prompt at runtime (once) and are not stored as separate messages.

## Main Processing Loop

0. **MCP Bootstrap**: Validate MCP servers, connect, fetch tool schemas and any instructions. Initialization errors are non‑fatal: they are logged and the agent may proceed to the LLM without those tools.
1. **Validation**: Validate configuration parameters - exit on any error
2. **LLM Request**: Send request to LLM with available tools (schemas) and conversation history
3. **Response Handling**:
   - If no tools requested: Stream output to stdout in real time and exit with code 0
   - If tools requested: Stream any assistant text to stdout, then proceed to tool execution
4. **Tool Execution**:
  - Tool selection and execution are handled by the provider/AI SDK; the application does not impose a tool count limit. Parallel tool calls can be toggled via `parallelToolCalls` for OpenAI‑compatible providers (default true).
  - Never retry a tool call
  - Wait for all tools to complete (successful or failed)
  - Record each tool result in message history in the exact order specified by the LLM
5. **Loop**: Return to step 2 with updated conversation history

## Core Requirements

### Tool Execution Rules
- **MANDATORY**: Every tool call must have a corresponding response in message history
- **Order Preservation**: Tool responses must maintain the exact order they were received from the LLM
- **Failure Handling**: Failed tools are included in history with error details; tools are never retried
- **Error Messages**: If a tool can't be executed and no response message is received, generate an explanatory error message
- **Parallel Execution**: Tools run concurrently but results are ordered correctly
- **Performance Tracking**: Include latency and request/response size accounting for each tool execution
<!-- Tool limits are now handled by the provider/SDK; no app-level limit option. -->

### Provider Fallback
- Models are tried sequentially on failure (model-first), and for each model all providers are attempted in order
- Each model/provider attempt receives the **exact same request** without modifications
- Streaming behavior: assistant tokens are streamed to stdout in real time; if a provider/model fails mid-stream, the partial assistant text is discarded from conversation history (not persisted) and the next attempt proceeds. A warning is written to stderr.
- No conversation history is lost between model attempts; MCP tool calls are never retried across attempts

### MCP Server Integration
- Validates MCP server connectivity and retrieves tool schemas before the first LLM request. Initialization errors are non‑fatal: they are logged and the agent may continue to the LLM without those tools.
- Supports local (`stdio`) and remote (`http` for streamable HTTP, `websocket`, and `sse`) MCP servers
- Full MCP protocol compliance for tool discovery and execution
- **Schemas and Instructions**: MCP tools provide schemas and optional instructions
- **System Prompt Integration**: At program start, append only tool instructions (if any) to the system prompt once, using the following headers:
  - `## TOOLS' INSTRUCTIONS`
  - `## TOOL {name} INSTRUCTIONS`
  Schemas are NOT appended to the system prompt; they are exposed to the LLM via the request's tool definitions.

#### Per‑Tool Environment Scoping
For `stdio` servers, only the environment variables explicitly configured for that MCP tool are passed to the spawned process. `${VAR}` placeholders are resolved from the current process environment. Variables are not leaked across tools.

### Accounting System
- **JSONL Logging**: All accounting data logged to JSONL file specified in config or command line
- **Entry Types**: Each entry has `"type": "llm"` or `"type": "tool"` and `"status": "ok"` or `"failed"`
- **LLM Accounting**: Track tokens per provider/model (input, completion, cached tokens, etc.) with latency
- **Tool Accounting**: Track MCP server (as named in config), tool command, latency, and character counts (in/out)
- **Privacy**: Never log prompt or completion content. Tool parameters are not logged; only tool name/command and metadata are recorded
- **Callbacks vs Files**: The core library never writes files or stdout/stderr; accounting is emitted via callbacks. The CLI may write JSONL when no custom accounting callback is supplied. If a callback is provided, file writing is skipped.

### Streaming Support
- LLM responses stream to stdout in real time
- If a stream fails mid-response, partial content is discarded from conversation history and a retry may proceed with the next provider/model
- Tools do not stream (their output is for the LLM, not user)
- Vercel AI SDK handles tool call detection automatically during streaming

#### Inactivity Timeout (LLM)
- The `llmTimeout` is an inactivity timeout during streaming: it resets on each received chunk and only aborts if no data arrives for the configured duration.
- In non-streaming mode, `llmTimeout` is a fixed per-call timeout.

### Verbose Logging (--verbose)
- For each request/response, prints a single concise line to stderr with key numbers:
  - `[llm] req: {provider}, {model}, messages N, X chars`
  - `[llm] res: input A, output B, cached C tokens, tools T, latency L ms, size S chars`
  - `[mcp] initializing {server}` / `[mcp] initialized {server}, {N} tools (...), latency Z ms`
  - `[mcp] req: {id} {server}, {tool}` / `[mcp] res: {id} {server}, {tool}, latency X ms, size Y chars`
  - `[fin] finally: llm requests R (tokens: A in, B out, C cached, tool-calls T, output-size S chars, latency-sum L ms), mcp requests M (serverA X, ...)`

### Stream / No-Stream Options
- Global default via `defaults.stream` (true/false)
- Per-provider override: `providers.<name>.custom.stream`
- CLI override: `--stream` / `--no-stream`
- Streaming recommended for interactivity; non-streaming can be used for providers or queries sensitive to streaming edge cases.

#### Tracing
- `--trace-llm`: Logs full request headers/body (Authorization redacted) and pretty JSON responses. For SSE responses, the raw SSE is logged after the stream completes.
- `--trace-mcp`: Logs MCP connect/start, tools/list and prompts/list requests and responses, server stderr lines, callTool requests, and results using a single `[mcp]` sink.

#### OpenRouter Notes
- Uses OpenAI‑compatible Chat Completions endpoints for best tool‑calling support.
- Adds attribution headers: `HTTP-Referer` and `X-OpenRouter-Title` (configurable via env `OPENROUTER_REFERER`, `OPENROUTER_TITLE`), and `User-Agent`.

## Library Architecture

The system is designed as a library that can be embedded in larger applications:

```typescript
import { AIAgent, AIAgentCallbacks } from './ai-agent';

// Optional callbacks for complete control over I/O
const callbacks: AIAgentCallbacks = {
  onLog: (level, message) => console.error(`[${level}] ${message}`),
  onOutput: (text) => process.stdout.write(text),
  onAccounting: (entry) => { /* custom accounting logic */ }
};

const agent = new AIAgent({
  configPath: '.ai-agent.json',
  llmTimeout: 30000,
  toolTimeout: 10000,
  callbacks, // Optional: if not provided, uses default I/O
  // ... other options
});

const result = await agent.run({
  providers: ['openai'],
  models: ['gpt-4o'],
  tools: ['file-operations'],
  systemPrompt: 'You are a helpful assistant',
  userPrompt: 'List current directory files',
  conversationHistory: [] // Optional: continue existing conversation
});
```

### Library Callbacks
The library accepts optional callbacks for complete control and performs no I/O itself:
- **onLog**: Custom logging handler (debug, info, warn, error)
- **onOutput**: Custom output handler (replaces stdout output)
- **onAccounting**: Custom accounting handler (replaces JSONL file writing)
- **Silent Core**: The core library writes nothing (no files, no stdout/stderr). The CLI wires callbacks to provide default behavior.

## Input/Output Specifications

### Standard Streams
- **stdout**: LLM responses only - nothing else ever goes to stdout
- **stderr**: All errors, warnings, and debug information
- **stdin**: Supported via `-` parameter for prompts
- **Validation**: Cannot use stdin (`-`) for both system and user prompts simultaneously

### Exit Codes
- **0**: Successful completion
- **1**: Configuration error
- **2**: LLM communication error
- **3**: Tool execution error
- **4**: Invalid command line arguments

### File Formats
- **Configuration**: JSON with full schema validation and environment variable expansion
- **Conversation Save/Load**: JSON format preserving full message history with metadata
- **Prompt Files**: Plain text files (UTF-8)
- **Accounting**: JSONL format with structured entries for LLM and tool usage

## Technology & Packaging

- **Runtime**: Node.js 20+
- **Package Manager**: npm
- **LLM Communication**: Vercel AI SDK 5 (TypeScript support, streaming, unified providers)
- **MCP Integration**: Official `@modelcontextprotocol/sdk` (1.17.4)
- **CLI Framework**: Commander.js
- **Validation**: Zod for type-safe configuration
- **Language**: TypeScript with full type safety throughout
- **Binary Name**: The CLI is exposed as `ai-agent` via `package.json#bin`, so local installs and `npx ai-agent` work as expected

## Development Principles

1. **Library First**: Core functionality as embeddable library, CLI as thin wrapper
2. **Type Safety**: Full TypeScript coverage with runtime validation
3. **Streaming Native**: Built-in support for streaming responses
4. **Error Resilience**: Graceful handling of provider and tool failures
5. **Protocol Compliance**: Full MCP specification support
6. **Token Transparency**: Detailed token accounting and reporting

## Future Extensibility

The architecture supports:
- Additional LLM providers through Vercel AI SDK
- New MCP transport mechanisms
- Custom tool execution strategies
- Enhanced conversation management
- Integration with larger AI systems
