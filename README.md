# AI Agent - Composable Autonomous AI Agents Framework

> Build powerful AI agents in minutes, not months. Write a prompt, add tools, run it. That's it.

## What is AI Agent?

AI Agent is a framework that transforms how you build and deploy AI agents. Instead of wrestling with complex orchestration code, you simply write prompts in `.ai` files, you define the provider/model and tools to use, and run them. Once an agent like that works, it can instantly become a tool for another agent, enabling infinite composition and reusability.

One simple format (`.ai` files) works as standalone agents, sub-agents, master orchestrators, API services, Slack bots, or web apps. Write once, use everywhere.

Key features at a glance:
- Multi-provider support (OpenAI, Anthropic, Google, OpenRouter, Ollama, Test LLM)
- MCP/REST/sub-agent/internal tools with concurrency queues and context guard
- Deterministic Phase 1/2 harnesses for regression coverage
- XML transport: fixed to `xml-final` (native tool calls for tools; final report must be emitted via `<ai-agent-NONCE-XXXX tool="agent__final_report">…</ai-agent-NONCE-XXXX>`). Progress remains native.
- CLI override for tool transport has been removed; xml-final is mandatory.

## Architecture: Recursive Autonomous Agents

AI Agent implements a **recursive planning agent architecture** where every agent continuously plans, executes, observes, and adapts. Unlike traditional frameworks that separate "orchestrators" from "workers," every agent in this system is both a planner and an executor.

### How It Works: Plan-Execute-Observe-Adapt Loop

Each agent operates autonomously in a continuous reasoning cycle:

1. **Reason & Plan**: The LLM analyzes the task and decides what to do next
2. **Act**: Executes tools or calls sub-agents based on its reasoning
3. **Observe**: Receives results and updates its understanding
4. **Adapt**: Refines its approach based on what it learned
5. **Repeat** until the goal is achieved

The conversation history acts as a **dynamic task ledger** - a living document showing:
- What's been tried and in what order
- What worked and what failed
- Current understanding of the problem
- Reasoning behind each decision
- Next steps being considered

### The Recursive Power: Every Sub-Agent Plans Too

Here's what makes this architecture unique: **every sub-agent is also a full autonomous planning agent**.

**Real-World Example from Production:**

The [Neda CRM system](neda/) is a production multi-agent system serving Netdata's sales and management teams. It demonstrates true recursive autonomy:

```
Neda (master orchestrator)
├─ company.ai (company research specialist)
│  ├─ Plans: "I need official info, then validate with community sources"
│  ├─ Uses: web-search.ai, web-fetch.ai
│  └─ Adapts: Stops when new searches yield no new info
│
├─ web-research.ai (investigative researcher)
│  ├─ Plans: Multi-phase research strategy with source prioritization
│  ├─ Uses: web-search.ai, web-fetch.ai, reddit.ai
│  └─ Adapts: Refines search terms based on findings, runs parallel searches
│
├─ hubspot.ai (CRM data specialist)
│  ├─ Plans: Query strategy based on available identifiers
│  ├─ Uses: hubspot MCP tools
│  └─ Adapts: Follows associations to gather complete picture
│
└─ 19 more specialized agents, each autonomous...
```

**Key Insight:** Notice `web-search.ai` is used by *both* `company.ai` and `web-research.ai`. Each parent agent:
- Doesn't know it's being called by another agent
- Makes its own decisions about when to call `web-search.ai`
- Provides its own context and requirements
- Evaluates results independently
- Determines its own success criteria

This is **emergent complexity from simple composition** - sophisticated multi-agent systems arise naturally from combining simple, reusable agents.

### Real Agent Example: Company Research

**`company.ai` (68 lines total):**
```yaml
#!/usr/bin/env ai-agent
---
description: Company Researcher - rigorous, evidence‑backed prospect intelligence
models:
  - anthropic/claude-sonnet-4-5
  - openai/gpt-5.1
agents:
  - web-fetch.ai    # ← These agents are also full planners!
  - web-search.ai   # ← They reason and adapt independently
maxTurns: 20
---
You are an elite AI company researcher.

## Investigation Mode

**YOUR FOCUS:**
- Official name, domain, HQ location
- Employee count, revenue, funding
- Key executive names
- Market intel, online reviews

**DO NOT RESEARCH:**
- Specific persons (there's a contact.ai agent for that)
- Technology stack (there's a company-tech.ai agent for that)

### Search Strategy

To gather information search the MOST RECENT and authoritative sources.

**CRITICAL:**
1. Provide context to `web-search` - it needs details
2. The `web-search` agent performs multiple searches - don't repeat what it already did

### When to Stop

Stop when:
1. Company is too small to be relevant
2. Additional searches yield no new information

Provide your report in: ${FORMAT}
```

**That's it.** No orchestration code. No state management. No error handling. Just:
- What the agent should know (prompt)
- What it can do (agents/tools)
- Its limits and preferences

### The Development Paradigm Shift

**Traditional AI frameworks require you to write orchestration code:**

```python
# Traditional approach - hundreds of lines of glue code
class ResearchPipeline:
    def __init__(self):
        self.company_agent = CompanyAgent(config)
        self.search_agent = SearchAgent(config)
        self.analyzer = AnalysisAgent(config)

    async def research_company(self, company_name):
        # Manual orchestration logic
        search_results = await self.search_agent.run(company_name)
        if not search_results.success:
            # Retry logic
            search_results = await self.backup_search(company_name)

        company_data = await self.company_agent.run(search_results.data)
        if company_data.confidence < 0.8:
            # More manual coordination
            additional_data = await self.search_agent.run(
                self.build_refined_query(company_data.gaps)
            )
            company_data = await self.company_agent.run(
                self.merge(search_results, additional_data)
            )

        analysis = await self.analyzer.run(company_data)
        # ... more coordination code
        return self.format_report(analysis)
```

**With AI Agent - just prompts and composition:**

Already shown above - `company.ai` is 68 lines including comments and formatting. The complexity is handled by:
- ✅ LLM reasoning (decides when to call what)
- ✅ Framework plumbing (retries, state, routing)
- ✅ Agent autonomy (each adapts independently)

### What You Focus On vs What the Framework Handles

**You focus on:**
- ✅ What each agent should know (system prompt)
- ✅ What each agent can do (tools/agents list)
- ✅ How agents compose (simple frontmatter references)
- ✅ Resource limits and preferences (timeouts, model choices)

**Framework handles automatically:**
- ✅ Orchestration logic (LLM reasoning decides the flow)
- ✅ Retries and fallbacks (built-in with provider switching)
- ✅ State management (conversation history)
- ✅ Result routing (tools return to conversation)
- ✅ Error recovery (graceful degradation)
- ✅ Token accounting (comprehensive tracking)
- ✅ Streaming (real-time output)
- ✅ Logging and observability (structured logs, telemetry)

### Why This Architecture Matters

1. **Radical Reusability**: Write `web-search.ai` once, use it in 10 different agents
2. **Natural Composition**: Complex systems emerge from simple components
3. **Easy Maintenance**: Each agent is a single text file with a clear purpose
4. **Independent Testing**: Test each agent in isolation
5. **Runtime Flexibility**: Agents adapt their strategy based on actual results
6. **Zero Boilerplate**: No orchestration code, no state management, no error handling
7. **Rapid Development**: Build in minutes what traditional frameworks need months for

**Real Impact:**
- Neda CRM: 22 specialized agents, 2,000 lines of prompts total
- Traditional equivalent: 20,000+ lines of orchestration code
- Development time: weeks vs months
- Maintenance: text file edits vs code refactoring

This is why you can build production-ready multi-agent systems in the time it takes to write a good prompt.

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
- [x] **Token Budget Management**: Enforce per-model context limits with tokenizers and early final turns

### 📝 **Simple Configuration Format**
- [x] **Frontmatter-Driven**: Configure agents with simple YAML frontmatter in `.ai` files:
  ```yaml
  ---
  description: Research Assistant
  llms: openai/gpt-4o,anthropic/claude-3.5-sonnet,google/gemini-pro  # Fallback chain
  tools: web-search,calculator,file-system   # MCP tools
  limits:
    maxTurns: 10
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

### 📏 **Token Budget Guardrails**
- [x] **Per-Model Context Windows**: Configure `contextWindow`, optional `tokenizer`, and `contextWindowBufferTokens` per provider/model in `.ai-agent.json`; when omitted, the agent falls back to a 131072-token window so the guard is always active
- [x] **Session Override**: `--override contextWindow=<tokens>` (or `globalOverrides.contextWindow` in the library API) forces a single context window across all targets and takes precedence over model/provider settings.
- [x] **Tool Output Guard**: Tool responses are preflight token-counted; overflows are rejected with `(tool failed: context window budget exceeded)` and a forced final turn
- [x] **Context Exit Reporting**: Sessions terminated by the guard emit `EXIT-TOKEN-LIMIT`, annotate accounting entries with projected vs. allowed tokens, and export telemetry via `ai_agent_context_guard_events_total` and `ai_agent_context_guard_remaining_tokens`

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
  - Structured logging with severity levels (journald/logfmt by default, JSON optional)
  - Telemetry pipeline: OTLP metrics, optional spans, Prometheus exporter, and opt-in OTLP log export
  - Complete accounting exports

### Telemetry & Logging

- **Default sinks**: when the agent runs under systemd, logs flow to journald; otherwise they are printed in logfmt to `stderr`. You can override this with `telemetry.logging.formats` (config) or `--telemetry-log-format` (CLI) using any combination of `journald`, `logfmt`, `json`, or `none` (first valid entry wins).
- **Metrics & traces**: keep telemetry opt-in by default. Enable the exporters with `telemetry.enabled: true` (or `--telemetry-enabled`) and configure the OTLP collector endpoint via `telemetry.otlp.endpoint`. Tracing stays disabled until you also set `telemetry.traces.enabled: true` (or `--telemetry-traces-enabled`) and optionally choose a sampler (`always_on`, `always_off`, `parent`, `ratio`).
- **JSON output**: select `json` in `telemetry.logging.formats` (or pass `--telemetry-log-format json`) to stream newline-delimited JSON logs on `stderr`—handy for shipping into structured log processors when journals aren’t available.
- **OTLP log export**: keep the local sink and add OTLP shipping by including `otlp` in `telemetry.logging.extra` (or `--telemetry-log-extra otlp`). Use `telemetry.logging.otlp.endpoint`/`timeoutMs` (or `--telemetry-logging-otlp-*`) to override the collector target if it differs from the metrics endpoint.
- **Environment override**: set `AI_TELEMETRY_DISABLE=1` to skip all telemetry initialisation during local testing.

Example config snippet (`ai-agent.json`):

```jsonc
{
  "providers": {
    "openai": {
      "type": "openai",
      "models": {
        "gpt-4o": {
          "contextWindow": 128000,
          "tokenizer": "tiktoken:gpt-4o",
          "contextWindowBufferTokens": 512
        }
      }
    }
  },
  "defaults": {
    "contextWindowBufferTokens": 512
  },
  "telemetry": {
    "enabled": true,
    "otlp": { "endpoint": "grpc://localhost:4317", "timeoutMs": 2000 },
    "traces": { "enabled": true, "sampler": "ratio", "ratio": 0.25 },
    "logging": {
      "formats": ["journald", "json"],
      "extra": ["otlp"],
      "otlp": { "endpoint": "grpc://collector.internal:4317", "timeoutMs": 1500 }
    },
    "labels": { "environment": "production", "cluster": "headend-a" },
    "prometheus": { "enabled": true, "host": "127.0.0.1", "port": 9464 }
  }
}
```
Tokenizers also accept `anthropic`/`claude` and `gemini`/`google:gemini` prefixes (for example `anthropic:claude-3-5-sonnet` or `google:gemini-1.5-pro`), leveraging local libraries before falling back to the approximate heuristic.

CLI equivalents:

```bash
ai-agent run \
  --telemetry-enabled \
  --telemetry-otlp-endpoint grpc://localhost:4317 \
  --telemetry-traces-enabled \
  --telemetry-trace-sampler ratio \
  --telemetry-trace-ratio 0.25 \
  --telemetry-log-format json \
  --telemetry-log-extra otlp \
  --telemetry-logging-otlp-endpoint grpc://collector.internal:4317
```

- **Flexible Deployment**:
  - [x] CLI for development and automation
  - [x] Library for embedding in applications
  - [x] **Slack bot integration** with real-time progress, interactive controls, and intelligent routing
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

1. **Command-line parameters**: Direct CLI options like `--temperature 0.5`, `--top-p 0.8`, `--max-output-tokens 8192`, `--repeat-penalty 1.2`, `--max-tool-turns 20`, `--max-tool-calls-per-turn 8`, `--llm-timeout 300000`
   - These always override all other settings
   - Perfect for testing and debugging
   - Use `--reasoning <none|minimal|low|medium|high>` when you need to stomp every agent/sub-agent, or `--default-reasoning <value>` when you only want to fill in prompts that omit a reasoning level. `--default-reasoning none` is especially useful when moving from Anthropic-style “high reasoning” defaults to smaller models—you can disable reasoning for prompts that never asked for it without rewriting their frontmatter.

2. **Frontmatter in `.ai` files**: YAML configuration at the top of agent files
   ```yaml
   ---
   temperature: 0.8
   maxTurns: 15
   ---
   ```
   - Agent-specific settings that override config files
   - `reasoning: none` (or `reasoning: unset`) now explicitly disables reasoning for that agent, while omitting the key (or writing `default`) lets the global fallback (`defaults.reasoning`/`--default-reasoning`) decide.

3. **Configuration files** (searched in order):
   
   a. **`--config` specified file**: Explicitly provided via `--config /path/to/config.json`
      - When specified, becomes the highest priority config file

   b. **Current working directory**: `.ai-agent.json` and `.ai-agent.env`
      - Where you run the command from
      - Ideal for project-specific settings

### Deterministic Testing
- Run `npm run test:phase1` (or `npm test` if you prefer Vitest) to execute the deterministic harness across scripted provider + MCP scenarios.
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

Add a `queues` block whenever you need to throttle heavy MCP servers (Playwright/fetcher, OpenAPI imports, etc.):

```json
"queues": {
  "default": { "concurrent": 32 },
  "fetcher": { "concurrent": 4 }
},
"mcpServers": {
  "fetcher": {
    "type": "stdio",
    "command": "npx",
    "args": ["-y", "fetcher-mcp"],
    "queue": "fetcher"
  }
}
```

All REST/OpenAPI tools accept the same `queue` field and fall back to `default` when omitted. The queue manager logs whenever a tool waits for a slot (`queued` log entries) and exports telemetry gauges/histograms (`ai_agent_queue_depth`, `ai_agent_queue_wait_duration_ms`).

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
- `--slack` – Slack Socket Mode headend with @mentions, DMs, channel posts, shortcuts, and slash commands. Real-time progress updates with Block Kit UI, per-channel routing, and interactive controls. See [docs/SLACK.md](docs/SLACK.md) for setup and configuration.

Per-headend concurrency guards are available (e.g., `--api-concurrency 8`). Each incoming request acquires a slot before the agent session is spawned, keeping the system responsive under load.

Graceful shutdown is built in: the supervisor listens for `SIGINT`/`SIGTERM`, flips a shared `stopRef` so active sessions finish their current turns, closes REST/MCP/Slack sockets, and finally tears down shared MCP transports. A single signal triggers the graceful path (with a watchdog to avoid hanging forever); sending a second signal forces an immediate exit. This keeps systemd/PM2 happy and prevents MCP restart loops after you've asked the process to stop.

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
- Slack integration: docs/SLACK.md

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
