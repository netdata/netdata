# AI Agent - Composable Autonomous AI Agents Framework

> Build powerful AI agents in minutes, not months. Write a prompt, add tools, run it. That's it.

## What is AI Agent?

AI Agent is a framework that transforms how you build and deploy AI agents. Instead of wrestling with complex orchestration code, you simply write prompts in `.ai` files, you define the provider/model and tools to use, and run them. Once an agent like that works, it can instantly become a tool for another agent, enabling infinite composition and reusability.

One simple format (`.ai` files) works as standalone agents, sub-agents, master orchestrators, API services, Slack bots, or web apps. Write once, use everywhere.

## Core Features

### 🚀 **Single-Agent Capabilities**
- [x] **Multi-Model Support with Intelligent Fallback**: Configure multiple LLM models in priority order. If the primary model fails (rate limits, errors, downtime), the system automatically falls back to secondary models, ensuring your agents never stop working.
  
- [x] **Universal LLM Provider Support**: AI SDK provides all the abstraction need to transparently use any provider (OpenAI, Anthropic, Google, Ollama, OpenRouter)
- [x] **MCP (Model Context Protocol) for Tools**: Connect any MCP-compatible tool server via `stdio`, `websocket`, `sse`, `http`
- [x] **Autonomous Multi-Turn Execution**: Agents independently decide when and how to use tools across multiple turns, automatically handling tool responses and determining next steps until task completion.
- [x] **Streaming & Real-Time Output**: Stream tokens as they're generated, with real-time progress updates, tool execution visibility, and comprehensive logging.

### 🔗 **Multi-Agent Orchestration**
- [x] **Agent-as-a-Tool Architecture**: Any `.ai` agent file can be instantly used as a tool by another agent. No code changes required - just reference it in the master agent's configuration.

- [x] **Recursive Agent Composition**: Build complex systems by composing specialized agents:
  - Research agent uses web search tools
  - Analysis agent uses the research agent as a tool
  - Report writer uses the analysis agent
  - Reviewer validates the report writer's output
  
- [x] **Isolated Agent Universes**: Each agent run is completely isolated with its own:
  - MCP client connections
  - LLM provider instances
  - Conversation history
  - Token accounting
  - Environment variables
  - Resource limits

- [x] **Intelligent Load Distribution**: Built-in concurrency controls, recursion limits, and resource management ensure system stability even with complex agent hierarchies.

### 🛡️ **Production-Ready Safeguards**
- [ ] **Loop Prevention**: Configurable recursion depth limits prevent infinite agent loops
- [ ] **Token Budget Management**: Set token limits per agent, per turn, and globally
- [x] **Timeout Controls**: Granular timeouts for LLM calls, tool execution, and total runtime
- [x] **Concurrency Limits**: Control parallel tool calls and simultaneous agent executions
- [x] **Cost Tracking**: Real-time token usage and cost accounting per provider
- [x] **Retry Strategies**: Intelligent retry with exponential backoff and jitter
- [x] **Error Recovery**: Graceful degradation with detailed error reporting

### 📝 **Simple Configuration Format**
- [x] **Frontmatter-Driven**: Configure agents with simple YAML frontmatter in `.ai` files:
  ```yaml
  ---
  description: Research Assistant
  llms: openai/gpt-4o,anthropic/claude-3.5-sonnet,google/gemini-pro  # Fallback chain
  tools: web-search,calculator,file-system   # MCP tools
  limits:
    maxToolTurns: 10
    maxTokens: 50000
    timeout: 300000  # 5 minutes
  ---
  
  You are a research assistant that helps users find accurate, 
  up-to-date information...
  ```

- [x] **Input/Output Contracts**: Define structured schemas for agent inputs and outputs:
  ```yaml
  input:
    format: json
    schema:
      type: object
      required: [query, depth]
  output:
    format: json
    schema:
      type: object
      required: [results, confidence]
  ```

### 🔧 **Developer Experience**
- [x] **CI/CD Friendly**: 
  - Test each agent in isolation
  - Version control with Git (just text files)
  - Automated testing with deterministic outputs
  - Performance benchmarking built-in

- [x] **Observability**:
  - `--verbose` flag for detailed execution logs
  - `--trace-llm` to debug model interactions
  - `--trace-mcp` to debug tool calls
  - Structured logging with severity levels
  - Complete accounting exports

- **Flexible Deployment**:
  - [x] CLI for development and automation
  - [x] Library for embedding in applications
  - [x] Slack bot mode
  - [ ] Web UI mode (experimental)
  - [x] REST API server

### 🏗️ **Advanced Architecture**
- [x] **Session Management**: Complete session isolation with immutable state transitions
- [x] **Provider Abstraction**: Unified interface across all LLM providers
- [x] **Tool Execution Engine**: Parallel tool execution with result aggregation
- [x] **Streaming Pipeline**: Efficient token streaming with backpressure handling
- [x] **Configuration Layers**: Hierarchical config resolution (defaults → global → local → runtime)

### 🎯 **Use Cases Enabled**
- [x] **Customer Support**: Multi-agent system with specialized agents for different domains
- [x] **Code Analysis**: Agents that read, analyze, test, and refactor code
- [x] **Research Automation**: Hierarchical research with fact-checking and synthesis
- [x] **Data Processing**: ETL pipelines with intelligent error handling
- [x] **Content Generation**: Multi-stage content creation with review cycles
- [x] **DevOps Automation**: Infrastructure management with approval workflows
- [x] **Business Intelligence**: Data analysis with natural language queries

## Quick Start

### Installation
```bash
# Clone the repository
git clone https://github.com/netdata/ai-agent.git
cd ai-agent

# Install dependencies and build
npm install
npm run build

# Or install everything to /opt/ai-agent
sudo ./build-and-install.sh
```

### Configuration System

The framework uses a **multi-level configuration system** with clear precedence rules. Settings are resolved through the following hierarchy (highest priority first):

#### Complete Precedence Order (highest → lowest)

1. **Command-line parameters**: Direct CLI options like `--temperature 0.5`, `--max-tool-turns 20`, `--llm-timeout 300000`
   - These always override all other settings
   - Perfect for testing and debugging

2. **Frontmatter in `.ai` files**: YAML configuration at the top of agent files
   ```yaml
   ---
   temperature: 0.8
   maxToolTurns: 15
   ---
   ```
   - Agent-specific settings that override config files

3. **Configuration files** (searched in order):
   
   a. **`--config` specified file**: Explicitly provided via `--config /path/to/config.json`
      - When specified, becomes the highest priority config file

   b. **Current working directory**: `.ai-agent.json` and `.ai-agent.env`
      - Where you run the command from
      - Ideal for project-specific settings

### Deterministic Testing
- Run `npm run test:phase1` to execute the deterministic harness across scripted provider + MCP scenarios.
- Detailed steps, coverage guidance, and debugging tips live in [`docs/TESTING.md`](docs/TESTING.md).

   c. **Prompt file directory**: `.ai-agent.json` and `.ai-agent.env`
      - The directory containing your prompt file (`.ai` file)
      - Perfect for prompt-specific configurations
      - Automatically discovered when running prompts from subdirectories

   d. **Binary directory**: `.ai-agent.json` and `.ai-agent.env`
      - Where the `ai-agent` executable is physically located
      - Perfect for portable installations

   e. **User home directory**: `~/.ai-agent/ai-agent.json` and `~/.ai-agent/ai-agent.env`
      - Personal configuration for the current user
      - Shared across all your projects

   f. **System directory**: `/etc/ai-agent/ai-agent.json` and `/etc/ai-agent/ai-agent.env`
      - System-wide defaults for all users
      - Typically set by administrators

4. **Internal defaults**: Built-in sensible defaults in the code
   - Temperature: 0.7, Top-P: 1.0, LLM timeout: 120s, Tool timeout: 60s, etc.
   - Always available as fallback

#### Environment Variables & API Key Security

The entire `.env` file system exists to **keep your API keys private and out of git repositories**. Here's how it works:

**Variable Resolution Order:**
1. Variables from `.ai-agent.env` files (at each configuration location)
2. System environment variables (`process.env`)
3. Empty string if not found

**How it works:**
- When the system sees `${VARIABLE_NAME}` in any `.ai-agent.json` file, it first checks the corresponding `.ai-agent.env` file
- If not found there, it falls back to `process.env` (system environment)
- This dual approach is perfect for containers and CI/CD:
  - Development: Keep keys in `.ai-agent.env` files (git-ignored)
  - Production/Containers: Pass keys via environment variables
  - Docker: Use `docker run -e OPENAI_API_KEY=... ai-agent`
  - Kubernetes: Use secrets mounted as environment variables

**Security Best Practices:**
- **NEVER** commit API keys to git - add `*.env` to `.gitignore`
- Use `.ai-agent.env` for local development (these files are searched alongside each `.ai-agent.json`)
- Use system environment variables in production/containers
- Different keys for different environments (dev/staging/prod)

Example `.ai-agent.json` (safe to commit):
```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}"
    },
    "anthropic": {
      "apiKey": "${ANTHROPIC_API_KEY}"
    }
  },
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "mcp-server-filesystem",
      "args": ["--read-only"]
    }
  }
}
```

Example `.ai-agent.env` (NEVER commit):
```bash
# Development API keys - DO NOT COMMIT
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
```

Example Docker deployment:
```bash
# Keys passed as environment variables, no .env files needed
docker run -e OPENAI_API_KEY=$OPENAI_API_KEY \
           -e ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY \
           -v $(pwd):/workspace \
           ai-agent ./assistant.ai "help me"
```

### Headend Server Mode

Expose one or more agents as network services by combining `--agent` registrations with the headend flags you need:

```bash
ai-agent \
  --agent agents/master.ai \
  --agent agents/sub/research.ai \
  --api 8123 \
  --mcp stdio \
  --mcp http:8124 \
  --openai-completions 8082 \
  --anthropic-completions 8083
```

Each flag is repeatable—every port or transport spins up an independent headend managed by a shared supervisor:

- `--api <port>` – REST API serving `GET /health` and `GET /v1/:agent?q=...&format=...`. The `format` query parameter defaults to `markdown`; pass `format=json` when the agent advertises a JSON schema.
- `--mcp stdio|http:PORT|sse:PORT|ws:PORT` – Model Context Protocol servers. Tool invocations must include a `format` argument, and when `format=json` the payload must also provide a `schema` object.
- `--openai-completions <port>` – OpenAI Chat Completions compatible endpoint exposing agents as models via `/v1/models` and `/v1/chat/completions` (supports SSE streaming).
- `--anthropic-completions <port>` – Anthropic Messages compatible endpoint with `/v1/models` and `/v1/messages`, including SSE streaming events.
- `--slack` – Slack Socket Mode headend that mirrors the existing Slack bot behaviour. Slash commands reuse the first registered REST headend when available; otherwise a fallback listener starts on the configured `api.port` (default `8080`).

Per-headend concurrency guards are available (e.g., `--api-concurrency 8`). Each incoming request acquires a slot before the agent session is spawned, keeping the system responsive under load.

### Your First Agent
Create `assistant.ai`:
```yaml
---
description: Helpful Assistant
llms: gpt-4o-mini
tools: filesystem
---

You are a helpful assistant. Answer the user's questions concisely and accurately.
```

Run it:
```bash
./assistant.ai "What files are in the current directory?"
```

### Multi-Agent Example
Create `researcher.ai`:
```yaml
---
description: Research Agent
llms: gpt-4o
tools: web-search
---

You are a research specialist. Find accurate, current information.
```

Create `analyst.ai`:
```yaml
---
description: Analysis Agent
llms: claude-3.5-sonnet
tools: researcher  # Uses researcher.ai as a tool!
---

You are an analyst. Use the researcher to gather information, then provide insights.
```

Run the multi-agent system:
```bash
./analyst.ai "Analyze the AI startup landscape in 2024"
```

## Documentation

- Specs (SPECS): docs/SPECS.md
- Implementation details: docs/IMPLEMENTATION.md
- Design overview: docs/DESIGN.md
- Multi-agent patterns: docs/MULTI-AGENT.md
- Internal API: docs/AI-AGENT-INTERNAL-API.md
- Slack bot notes: docs/SLACK-BOT.md

## Why AI Agent?

Traditional AI agent frameworks require hundreds of lines of code for basic functionality. AI Agent reduces this to a single prompt file that works everywhere. The same `.ai` file can be:

- A standalone CLI tool
- A sub-agent in a larger system
- A Slack bot
- A web service
- An API endpoint
- Part of your CI/CD pipeline

This isn't just another wrapper - it's a fundamental rethinking of how AI agents should be built and composed.

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Support

- GitHub Issues: https://github.com/netdata/ai-agent/issues
- Discussions: https://github.com/netdata/ai-agent/discussions
- Documentation: docs/ (see the list above)
