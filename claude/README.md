# AI Agent - Universal LLM Tool Calling Interface

A powerful TypeScript/Node.js CLI tool that provides a universal interface for interacting with multiple LLM providers with comprehensive tool calling support via the Model Context Protocol (MCP).

## Features

- **Multi-Provider Support**: OpenAI, Anthropic (Claude), Google Gemini, and Ollama
- **Tool Calling**: Full MCP (Model Context Protocol) integration with multiple transport types
- **Streaming Responses**: Real-time streaming of LLM responses
- **Configuration Management**: JSON-based configuration with environment variable support
- **Conversation Management**: Save and load conversation history
- **Accounting**: Usage tracking and logging
- **Error Handling**: Comprehensive error handling with provider fallback
- **Flexible Prompts**: Support for file input, stdin, and direct string prompts

## Installation

```bash
# Clone the repository
git clone <repository-url>
cd claude

# Install dependencies
npm install

# Build the project
npm run build

# Optionally install globally
npm install -g .
```

## Quick Start

### Basic Usage

```bash
# Simple conversation with OpenAI
ai-agent openai gpt-4o-mini "" "You are a helpful assistant." "Hello, how are you?"

# Use Ollama for local inference
ai-agent ollama llama3.2:3b "" "You are a coding assistant." "Write a Hello World in Python"

# Multiple providers with fallback
ai-agent "openai,ollama" "gpt-4o-mini,llama3.2:3b" "" "System prompt" "User message"
```

### Configuration

Create a configuration file (e.g., `config.json`):

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}",
      "baseUrl": "https://api.openai.com/v1"
    },
    "anthropic": {
      "apiKey": "${ANTHROPIC_API_KEY}"
    },
    "ollama": {
      "baseUrl": "http://localhost:11434/v1"
    }
  },
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "/path/to/filesystem-mcp-server"
    }
  },
  "defaults": {
    "llmTimeout": 30000,
    "toolTimeout": 10000,
    "temperature": 0.7
  }
}
```

## Command Line Options

### Arguments

- `providers` - Comma-separated list of LLM providers (openai, anthropic, google, ollama)
- `models` - Comma-separated list of model names  
- `mcp-tools` - Comma-separated list of MCP tools (can be empty for simple chat)
- `system-prompt` - System prompt (string, @filename, or - for stdin)
- `user-prompt` - User prompt (string, @filename, or - for stdin)

### Options

- `--config <filename>` - Configuration file path
- `--llm-timeout <ms>` - Timeout for LLM responses (default: 30000)
- `--tool-timeout <ms>` - Timeout for tool execution (default: 10000)
- `--max-parallel-tools <n>` - Max tools to accept from LLM
- `--max-concurrent-tools <n>` - Max tools to run concurrently
- `--temperature <n>` - LLM temperature 0.0-2.0 (default: 0.7)
- `--top-p <n>` - LLM top-p sampling 0.0-1.0 (default: 1.0)
- `--save <filename>` - Save conversation to JSON file
- `--load <filename>` - Load conversation from JSON file
- `--accounting <filename>` - Override accounting file from config

## Supported Providers

### OpenAI
```bash
ai-agent openai gpt-4o-mini "" "System prompt" "User message" --config config.json
```

### Anthropic (Claude)
```bash
ai-agent anthropic claude-3-5-sonnet-20241022 "" "System prompt" "User message" --config config.json
```

### Google Gemini
```bash
ai-agent google gemini-1.5-pro "" "System prompt" "User message" --config config.json
```

### Ollama (Local)
```bash
ai-agent ollama llama3.2:3b "" "System prompt" "User message" --config config.json
```

## MCP Tool Integration

The AI Agent supports the Model Context Protocol for tool calling:

### Supported Transport Types

- **stdio**: Standard input/output transport
- **sse**: Server-sent events transport  
- **websocket**: WebSocket transport

### Configuration Example

```json
{
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "node",
      "args": ["/path/to/filesystem-server.js"]
    },
    "database": {
      "type": "websocket",
      "url": "ws://localhost:8080/mcp"
    },
    "api": {
      "type": "sse",
      "url": "http://localhost:3000/mcp"
    }
  }
}
```

### Tool Execution

```bash
# Use filesystem tools
ai-agent openai gpt-4o-mini filesystem "You can read and write files." "Read the contents of README.md"

# Multiple tools
ai-agent openai gpt-4o-mini "filesystem,database" "System prompt" "List files and query user data"
```

## Advanced Features

### Conversation Management

```bash
# Save conversation
ai-agent openai gpt-4o-mini "" "System" "Hello" --save conversation.json

# Load and continue conversation  
ai-agent openai gpt-4o-mini "" "System" "Continue our chat" --load conversation.json --save conversation.json
```

### File-based Prompts

```bash
# Read system prompt from file
ai-agent openai gpt-4o-mini "" "@system-prompt.txt" "User message"

# Read user prompt from stdin
echo "What is TypeScript?" | ai-agent openai gpt-4o-mini "" "System prompt" "-"
```

### Provider Fallback

The system tries providers and models in order, falling back on failures:

```bash
# Try OpenAI first, then Ollama if it fails
ai-agent "openai,ollama" "gpt-4o-mini,llama3.2:3b" "" "System" "User message"
```

### Concurrency Control

```bash
# Limit parallel tool execution
ai-agent openai gpt-4o-mini tools "System" "User" --max-parallel-tools 3 --max-concurrent-tools 2
```

## Environment Variables

The configuration supports environment variable expansion:

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}"
    },
    "anthropic": {
      "apiKey": "${ANTHROPIC_API_KEY}"  
    }
  }
}
```

Set your environment variables:

```bash
export OPENAI_API_KEY="your-openai-key"
export ANTHROPIC_API_KEY="your-anthropic-key"
```

## Usage Tracking

The system automatically logs usage statistics:

```json
{
  "timestamp": "2025-01-15T10:30:00.000Z",
  "type": "llm",
  "provider": "openai",
  "model": "gpt-4o-mini",
  "tokens": {
    "inputTokens": 150,
    "outputTokens": 75,
    "totalTokens": 225
  },
  "latency": 1250,
  "status": "ok"
}
```

## Error Handling

The system provides comprehensive error handling:

- **Configuration Errors**: Exit code 1
- **Provider/Model Errors**: Exit code 2  
- **Tool Errors**: Exit code 3
- **Command Line Errors**: Exit code 4

## Development

### Building

```bash
npm run build
```

### Development Mode

```bash
npm run dev  # Watch mode with TypeScript compilation
```

### Testing

```bash
npm test
```

## Architecture

### Core Components

- `AIAgent` - Main orchestration class
- `MCPClientManager` - Manages MCP server connections and tool execution
- `AccountingManager` - Usage tracking and logging
- `Configuration` - Config loading and validation

### Key Features

- **Type Safety**: Full TypeScript with strict mode
- **Streaming**: Real-time response streaming  
- **Async/Await**: Modern async patterns throughout
- **Error Recovery**: Graceful degradation and fallback
- **Resource Management**: Automatic cleanup of connections

## Examples

### Basic Chat
```bash
ai-agent openai gpt-4o-mini "" "You are a helpful assistant." "Explain quantum computing"
```

### Code Generation
```bash  
ai-agent ollama qwen2.5-coder:7b "" "You are a coding assistant." "Write a REST API in Node.js"
```

### Tool-Assisted Task
```bash
ai-agent anthropic claude-3-5-sonnet-20241022 filesystem "You can read/write files." "Create a Python script to analyze CSV data"
```

### Multi-Provider Reliability
```bash
ai-agent "openai,anthropic,ollama" "gpt-4o-mini,claude-3-5-sonnet-20241022,llama3.2:3b" "" "System" "Complex reasoning task"
```

## License

MIT License - see LICENSE file for details.