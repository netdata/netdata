# Frontmatter Reference

Complete reference for all YAML frontmatter keys in `.ai` agent files.

---

## Overview

Frontmatter is the YAML configuration block at the top of every `.ai` file. It defines:
- **Agent identity** (name, description, usage instructions)
- **Model selection** (which LLMs to use)
- **Tool access** (MCP servers, REST tools, sub-agents)
- **Behavior limits** (turns, retries, timeouts)
- **Sampling parameters** (temperature, topP, topK)
- **Input/output contracts** (schemas for structured I/O)
- **Orchestration** (advisors, router, handoff)

**Prerequisites**: [Agent Files](Agent-Development-Agent-Files) explains the overall `.ai` file structure.

---

## Quick Example

Minimal working frontmatter:

```yaml
---
description: A helpful assistant that answers questions
models:
  - openai/gpt-4o
---

You are a helpful assistant. Answer the user's question clearly and concisely.

Respond in ${FORMAT}.
```

Typical production agent:

```yaml
---
description: Research assistant with web search capabilities
usage: "Ask any research question. Returns structured findings."
toolName: research_assistant
models:
  - anthropic/claude-sonnet-4-20250514
  - openai/gpt-4o
tools:
  - brave
  - fetcher
maxTurns: 15
maxRetries: 3
temperature: 0.3
output:
  format: json
  schema:
    type: object
    properties:
      findings:
        type: array
        items:
          type: string
      conclusion:
        type: string
    required:
      - findings
      - conclusion
---
```

---

## Configuration Reference

### Identity Keys

These keys identify the agent and provide documentation.

#### description

| Property | Value |
|----------|-------|
| Type | `string` |
| Default | None |
| Required | **Yes** (for sub-agents) |

**Description**: Short description of what the agent does. Shown in tool listings when the agent is exposed as a sub-agent tool.

**What it affects**:
- Displayed to parent agents when this agent is a sub-agent tool
- Used in `--list-tools` output
- Required for sub-agents (agent loader throws an error if missing)

**Example**:
```yaml
---
description: Analyzes company financials and returns structured reports
---
```

**Notes**:
- Keep it concise (one sentence)
- Describe the agent's purpose, not its implementation

---

#### usage

| Property | Value |
|----------|-------|
| Type | `string` |
| Default | None |
| Required | No |

**Description**: Usage instructions for users or parent agents. Describes how to interact with the agent.

**What it affects**:
- Displayed in tool documentation
- Helps parent agents understand how to call this agent

**Example**:
```yaml
---
usage: "Provide a company name or domain. Returns financial analysis in JSON."
---
```

---

#### toolName

| Property | Value |
|----------|-------|
| Type | `string` |
| Default | Derived from filename |
| Required | No (but recommended for sub-agents) |

**Description**: Stable identifier when exposing the agent as a callable tool. Parent agents call this agent using `agent__<toolName>`.

**What it affects**:
- Tool name exposed to parent agents
- Must be unique among sibling sub-agents
- Cannot use reserved names (`final_report`, `task_status`, `batch`)

**Example**:
```yaml
---
toolName: company_researcher
---
```

**Notes**:
- Use lowercase with underscores
- If omitted, derived from filename (e.g., `company-researcher.ai` becomes `company_researcher`)
- Explicit `toolName` is safer for refactoring

---

### Model Configuration

#### models

| Property | Value |
|----------|-------|
| Type | `string` or `string[]` |
| Default | None |
| Required | **Yes** (at least one model required) |
| Valid values | `provider/model` pairs |

**Description**: List of provider/model pairs to use. The agent tries each in order if one fails (fallback chain).

**What it affects**:
- Which LLM(s) handle requests
- Fallback order on failures
- Cost and performance characteristics

**Example**:
```yaml
---
# Single model
models: openai/gpt-4o

# Multiple models (fallback chain)
models:
  - anthropic/claude-sonnet-4-20250514
  - openai/gpt-4o
  - openai/gpt-4o-mini
---
```

**Notes**:
- Provider names must match entries in `.ai-agent.json` `providers` section
- Format is always `provider/model` with exactly one `/`
- First model is primary; others are fallbacks
- Common providers: `openai`, `anthropic`, `google`, `openrouter`, `ollama`

---

#### temperature

| Property | Value |
|----------|-------|
| Type | `number` or `null` |
| Default | `0.0` |
| Valid values | `0.0` to `2.0`, or `null`/`none`/`off`/`unset`/`default` |

**Description**: Controls response creativity/randomness. Lower values produce more focused, deterministic outputs.

**What it affects**:
- Response variability (0 = most deterministic, 2 = most creative)
- Task suitability (use low for factual tasks, higher for creative tasks)

**Example**:
```yaml
---
temperature: 0.3    # Focused, factual responses
temperature: 0.7    # Balanced
temperature: 1.2    # More creative
temperature: null   # Let provider decide (don't send parameter)
---
```

**Notes**:
- Set to `null`, `none`, `off`, `unset`, or `default` to not send the parameter (let provider use its default)
- Some models ignore temperature when using certain features

---

#### topP

| Property | Value |
|----------|-------|
| Type | `number` or `null` |
| Default | `null` (not sent) |
| Valid values | `0.0` to `1.0`, or `null`/`none`/`off`/`unset`/`default` |

**Description**: Nucleus sampling - limits token selection to the smallest set of tokens whose cumulative probability exceeds this threshold.

**What it affects**:
- Token selection diversity
- Alternative to temperature for controlling randomness

**Example**:
```yaml
---
topP: 0.9    # Consider tokens in top 90% probability mass
topP: null   # Don't send (use provider default)
---
```

**Notes**:
- Generally, use either `temperature` OR `topP`, not both
- `null` means the parameter is not sent to the provider

---

#### topK

| Property | Value |
|----------|-------|
| Type | `integer` or `null` |
| Default | `null` (not sent) |
| Valid values | `1` or greater, or `null`/`none`/`off`/`unset`/`default` |

**Description**: Limits token selection to the K most probable tokens.

**What it affects**:
- Token vocabulary per generation step
- Provider-dependent support

**Example**:
```yaml
---
topK: 40     # Consider only top 40 tokens
topK: null   # Don't send (use provider default)
---
```

**Notes**:
- Not all providers support `topK`
- `null` means the parameter is not sent

---

#### maxOutputTokens

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `4096` |
| Valid values | `1` or greater |

**Description**: Maximum number of tokens the model can generate per turn.

**What it affects**:
- Maximum response length
- Cost (longer responses use more tokens)
- Truncation behavior

**Example**:
```yaml
---
maxOutputTokens: 8192    # Allow longer responses
maxOutputTokens: 1024    # Enforce shorter responses
---
```

**Notes**:
- Actual limit depends on model capabilities
- Very long outputs may hit context window limits

---

#### repeatPenalty

| Property | Value |
|----------|-------|
| Type | `number` or `null` |
| Default | `null` (not sent) |
| Valid values | `0.0` or greater, or `null`/`none`/`off`/`unset`/`default` |

**Description**: Penalizes repetitive text. Higher values reduce repetition.

**What it affects**:
- Text repetition in outputs
- Some providers call this "frequency penalty"

**Example**:
```yaml
---
repeatPenalty: 1.0    # No penalty (default behavior)
repeatPenalty: 1.5    # Moderate penalty
repeatPenalty: null   # Don't send (use provider default)
---
```

**Notes**:
- Provider-dependent implementation
- `null` means the parameter is not sent

---

#### reasoning

| Property | Value |
|----------|-------|
| Type | `string` |
| Default | Provider default (not sent) |
| Valid values | `none`, `unset`, `default`, `inherit`, `minimal`, `low`, `medium`, `high` |

**Description**: Controls extended thinking/reasoning effort for models that support it.

**What it affects**:
- Whether the model uses extended reasoning
- Reasoning effort level and token budget
- Cost and latency (higher reasoning = more expensive/slower)

**Example**:
```yaml
---
reasoning: none      # Disable extended reasoning
reasoning: medium    # Moderate reasoning effort
reasoning: high      # Maximum reasoning effort
reasoning: default   # Inherit from parent or global config
---
```

**Notes**:
- `none` or `unset` explicitly disables reasoning
- `default` or `inherit` (or omitting the key) uses global fallback from `--default-reasoning` or `defaults.reasoning`
- Reasoning is model-dependent (e.g., Anthropic's thinking mode, OpenAI's reasoning models)

---

#### reasoningTokens

| Property | Value |
|----------|-------|
| Type | `number` or `string` |
| Default | Undefined (provider decides) |
| Valid values | Positive integer, or `0`/`disabled` |

**Description**: Token budget for extended reasoning (Anthropic thinking mode).

**What it affects**:
- Maximum tokens the model can use for internal reasoning
- `0` or `disabled` turns off reasoning cache

**Example**:
```yaml
---
reasoningTokens: 16000   # Allow up to 16K reasoning tokens
reasoningTokens: 0       # Disable reasoning
---
```

---

#### caching

| Property | Value |
|----------|-------|
| Type | `string` |
| Default | `full` |
| Valid values | `none`, `full` |

**Description**: Controls Anthropic prompt caching behavior.

**What it affects**:
- Whether Anthropic's prompt cache is used
- Cost optimization for repeated prompts

**Example**:
```yaml
---
caching: full    # Enable prompt caching (default)
caching: none    # Disable prompt caching
---
```

**Notes**:
- Only affects Anthropic provider
- `full` enables cache reuse for repeated prompt prefixes

---

### Tool Configuration

#### tools

| Property | Value |
|----------|-------|
| Type | `string` or `string[]` |
| Default | `[]` (no tools) |
| Valid values | MCP server names, REST tool names, `openapi:` prefixes |

**Description**: Which tools the agent can use. Lists MCP server names (from `.ai-agent.json`), REST tools, or OpenAPI providers.

**What it affects**:
- Available tools for the agent
- Which MCP servers are initialized
- Tool-related costs and latency

**Example**:
```yaml
---
# Single tool source
tools: filesystem

# Multiple tool sources
tools:
  - brave          # MCP server named "brave"
  - fetcher        # MCP server named "fetcher"
  - rest__catalog  # REST tool named "catalog"
  - openapi:github # All tools from OpenAPI spec "github"
---
```

**Notes**:
- MCP server names must match entries in `.ai-agent.json` `mcpServers` section
- Listing a server exposes ALL its tools (use `toolsAllowed`/`toolsDenied` in config to filter)
- REST tools are prefixed with `rest__`
- OpenAPI tools use `openapi:<provider-name>` to import all operations

---

#### agents

| Property | Value |
|----------|-------|
| Type | `string` or `string[]` |
| Default | `[]` (no sub-agents) |
| Valid values | Relative or absolute paths to `.ai` files |

**Description**: Sub-agent `.ai` files to load as tools. Each sub-agent becomes callable via `agent__<toolName>`.

**What it affects**:
- Available sub-agent tools
- Multi-agent composition capabilities
- Agent hierarchy and recursion

**Example**:
```yaml
---
agents:
  - ./helpers/researcher.ai
  - ./helpers/summarizer.ai
  - /absolute/path/to/analyzer.ai
---
```

**Notes**:
- Paths are relative to the parent `.ai` file's directory
- Sub-agents must have `description` in frontmatter
- Each sub-agent's `toolName` must be unique among siblings
- Recursion is detected and prevented

---

### Execution Limits

#### maxTurns

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `10` |
| Valid values | `1` or greater |

**Description**: Maximum number of LLM turns (request-response cycles) before the agent must complete.

**What it affects**:
- Prevents infinite loops in agentic behavior
- On the final turn, tools are disabled and the agent is forced to provide a final answer
- Higher values allow more complex multi-step tasks but increase cost and time

**Example**:
```yaml
---
maxTurns: 15    # Allow more tool-using turns
maxTurns: 5     # Limit to fewer turns
---
```

**Notes**:
- If your agent consistently hits maxTurns without completing, consider: (1) simplifying the task, (2) improving the prompt, or (3) increasing the limit
- The final turn forces a final answer even if more work is needed

---

#### maxToolCallsPerTurn

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `10` |
| Valid values | `1` or greater |

**Description**: Maximum number of tools the agent can call in a single turn.

**What it affects**:
- Caps parallel tool usage per turn
- Prevents excessive tool calls from a single LLM response

**Example**:
```yaml
---
maxToolCallsPerTurn: 20    # Allow more parallel tools
maxToolCallsPerTurn: 5     # Limit parallel execution
---
```

---

#### maxRetries

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `5` |
| Valid values | `0` or greater |

**Description**: How many times to retry when LLM calls fail. Goes through all fallback models before giving up.

**What it affects**:
- Resilience to transient failures
- Rate limit handling
- Fallback model utilization

**Example**:
```yaml
---
maxRetries: 3     # Standard retry count
maxRetries: 10    # More retries for critical tasks
maxRetries: 0     # No retries (fail immediately)
---
```

**Notes**:
- Retries cycle through all configured models in the fallback chain
- Rate limit errors wait before retrying

---

#### llmTimeout

| Property | Value |
|----------|-------|
| Type | `number` (ms) or `string` (duration) |
| Default | `600000` (10 minutes) |
| Valid values | Positive integer (ms) or duration string |

**Description**: How long to wait for the LLM to respond before timing out.

**What it affects**:
- Maximum wait time per LLM request
- Timeout resets on each streamed chunk (inactivity timeout)

**Example**:
```yaml
---
llmTimeout: 120000    # 2 minutes in milliseconds
llmTimeout: 5m        # 5 minutes as duration string
llmTimeout: 30s       # 30 seconds
---
```

**Notes**:
- Duration strings support: `ms`, `s`, `m`, `h`, `d`
- For streaming, timeout resets on each received chunk

---

#### toolTimeout

| Property | Value |
|----------|-------|
| Type | `number` (ms) or `string` (duration) |
| Default | `300000` (5 minutes) |
| Valid values | Positive integer (ms) or duration string |

**Description**: How long to wait for each tool call to complete.

**What it affects**:
- Maximum execution time per tool
- Applies to MCP tools, REST tools, and sub-agent calls

**Example**:
```yaml
---
toolTimeout: 60000    # 1 minute in milliseconds
toolTimeout: 2m       # 2 minutes
toolTimeout: 30s      # 30 seconds
---
```

---

### Caching

#### cache

| Property | Value |
|----------|-------|
| Type | `number` (ms) or `string` |
| Default | Undefined (no caching) |
| Valid values | `off`, milliseconds, or duration string |

**Description**: Response cache TTL. Caches agent responses to avoid redundant LLM calls.

**What it affects**:
- Whether responses are cached
- How long cached responses remain valid
- Cost savings for repeated identical requests

**Example**:
```yaml
---
cache: off       # Disable caching
cache: 300000    # 5 minutes in milliseconds
cache: 1h        # 1 hour
cache: 1d        # 1 day
cache: 5m        # 5 minutes
---
```

**Notes**:
- Duration units: `ms`, `s`, `m`, `h`, `d`, `w`, `mo`, `y`
- Cache key includes: prompt content, model, parameters
- Requires cache backend configuration in `.ai-agent.json`

---

### Tool Output Handling

#### toolResponseMaxBytes

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `12288` (12 KB) |
| Valid values | `0` or greater |

**Description**: Maximum tool output size kept in conversation. Larger outputs are stored separately and replaced with a `tool_output` handle.

**What it affects**:
- How large tool outputs are handled
- Context window usage
- Whether `tool_output` tool is invoked

**Example**:
```yaml
---
toolResponseMaxBytes: 24576    # 24 KB before storing
toolResponseMaxBytes: 8192     # 8 KB (more aggressive storage)
---
```

**Notes**:
- Oversized outputs are stored in `/tmp/ai-agent-<run-hash>/`
- The model receives a handle message and can call `tool_output` to retrieve data

---

#### toolOutput

| Property | Value |
|----------|-------|
| Type | `object` |
| Default | Undefined (uses global defaults) |

**Description**: Overrides for the `tool_output` module behavior.

**Sub-keys**:

| Sub-key | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | `boolean` | `true` | Enable/disable tool output chunking |
| `maxChunks` | `number` | Varies | Maximum chunks for large outputs |
| `overlapPercent` | `number` | Varies | Overlap between chunks (%) |
| `avgLineBytesThreshold` | `number` | Varies | Threshold for line-based chunking |
| `models` | `string` or `string[]` | None | Models for extraction (optional) |

**Example**:
```yaml
---
toolOutput:
  enabled: true
  maxChunks: 5
  overlapPercent: 10
  avgLineBytesThreshold: 1000
  models: openai/gpt-4o-mini
---
```

**Notes**:
- `storeDir` is accepted but ignored (root is always `/tmp/ai-agent-<run-hash>`)

---

### Input/Output Contracts

#### input

| Property | Value |
|----------|-------|
| Type | `object` |
| Default | `{ format: 'json', schema: <default> }` |

**Description**: Input specification for sub-agent tools. Defines how parent agents should provide input.

**Sub-keys**:

| Sub-key | Type | Required | Description |
|---------|------|----------|-------------|
| `format` | `'text'` or `'json'` | No | Input format |
| `schema` | `object` | No | JSON Schema for validation |
| `schemaRef` | `string` | No | Path to external schema file |

**Example**:
```yaml
---
input:
  format: json
  schema:
    type: object
    properties:
      query:
        type: string
        description: The research query
      maxResults:
        type: number
        minimum: 1
        maximum: 100
    required:
      - query
---
```

**Notes**:
- When `format: json`, inputs are validated against the schema
- `schemaRef` points to a JSON/YAML file relative to the prompt file
- Invalid inputs return as tool errors to the parent agent

---

#### output

| Property | Value |
|----------|-------|
| Type | `object` |
| Default | Undefined (markdown) |

**Description**: Output specification. Defines the expected output format and schema.

**Sub-keys**:

| Sub-key | Type | Required | Description |
|---------|------|----------|-------------|
| `format` | `'json'`, `'markdown'`, or `'text'` | Yes | Output format |
| `schema` | `object` | No (required for `json`) | JSON Schema for validation |
| `schemaRef` | `string` | No | Path to external schema file |

**Example**:
```yaml
---
output:
  format: json
  schema:
    type: object
    properties:
      summary:
        type: string
      confidence:
        type: number
        minimum: 0
        maximum: 1
    required:
      - summary
      - confidence
---
```

```yaml
---
# External schema file
output:
  format: json
  schemaRef: ./schemas/report.json
---
```

**Notes**:
- `format: json` requires a schema (inline or via `schemaRef`)
- Schema is used to validate the final report
- Headends use the format to set `${FORMAT}` placeholder

---

### Orchestration

Orchestration keys enable multi-agent workflows. See [Multi-Agent Orchestration](Agent-Development-Multi-Agent) for details.

#### advisors

| Property | Value |
|----------|-------|
| Type | `string` or `string[]` |
| Default | `[]` (no advisors) |

**Description**: Agents to run in parallel before the main session. Outputs are injected as advisory context.

**What it affects**:
- Pre-execution consultation
- Advisory context injection
- Parallel pre-processing

**Example**:
```yaml
---
advisors:
  - ./agents/compliance.ai
  - ./agents/security.ai
---
```

**Notes**:
- Advisors run in parallel before the main agent
- Advisor outputs are injected into the user prompt
- Failures become synthetic advisory blocks (don't stop execution)

---

#### router.destinations

| Property | Value |
|----------|-------|
| Type | `object` with `destinations: string[]` |
| Default | Undefined (no routing) |

**Description**: Enables router mode. Registers `router__handoff-to` tool that can delegate to destination agents.

**What it affects**:
- Enables dynamic routing to specialized agents
- The main agent becomes a router/dispatcher

**Example**:
```yaml
---
router:
  destinations:
    - ./agents/legal.ai
    - ./agents/support.ai
    - ./agents/billing.ai
---
```

**Notes**:
- Creates `router__handoff-to` tool with `agent` parameter (enum of destinations)
- The router agent decides which destination to hand off to
- Must have at least one destination

---

#### handoff

| Property | Value |
|----------|-------|
| Type | `string` |
| Default | Undefined (no handoff) |

**Description**: Agent to receive output after completion. Runs after the main session finishes.

**What it affects**:
- Post-processing pipeline
- Output transformation
- Final formatting

**Example**:
```yaml
---
handoff: ./agents/formatter.ai
---
```

**Notes**:
- Runs after the main session (and after any router chain)
- Receives original user request + upstream response
- Arrays are not supported (single handoff only)

---

## Common Patterns

### Simple Question-Answering Agent

```yaml
---
description: General-purpose assistant
models:
  - openai/gpt-4o
temperature: 0.3
maxTurns: 5
---

You are a helpful assistant. Answer questions clearly and concisely.

Respond in ${FORMAT}.
```

### Tool-Using Research Agent

```yaml
---
description: Research agent with web search and analysis
usage: "Ask any research question"
models:
  - anthropic/claude-sonnet-4-20250514
tools:
  - brave
  - fetcher
maxTurns: 15
maxToolCallsPerTurn: 10
temperature: 0.2
cache: 1h
---

You are a research assistant with access to web search tools.

1. Search for relevant information
2. Verify from multiple sources
3. Synthesize findings

Respond in ${FORMAT}.
```

### Structured Output Agent

```yaml
---
description: Company analyzer returning structured data
toolName: company_analyzer
models:
  - openai/gpt-4o
tools:
  - company-data
temperature: 0
output:
  format: json
  schema:
    type: object
    properties:
      name:
        type: string
      industry:
        type: string
      summary:
        type: string
      metrics:
        type: object
        properties:
          employees:
            type: number
          revenue:
            type: string
    required:
      - name
      - industry
      - summary
input:
  format: json
  schema:
    type: object
    properties:
      company:
        type: string
    required:
      - company
---

Analyze the provided company and return structured data.
```

### Multi-Agent Hierarchy

Parent agent:
```yaml
---
description: Research coordinator
models:
  - anthropic/claude-sonnet-4-20250514
agents:
  - ./specialists/web-researcher.ai
  - ./specialists/data-analyst.ai
  - ./specialists/report-writer.ai
maxTurns: 20
---

You coordinate research tasks.

Available specialists:
- `agent__web_researcher`: Web search and data gathering
- `agent__data_analyst`: Data analysis and statistics
- `agent__report_writer`: Final report formatting

Delegate appropriately and synthesize results.
```

### Router Pattern

```yaml
---
description: Customer service router
models:
  - openai/gpt-4o-mini
router:
  destinations:
    - ./handlers/billing.ai
    - ./handlers/technical.ai
    - ./handlers/general.ai
maxTurns: 3
temperature: 0
---

You are a customer service router. Analyze the user's request and route to the appropriate specialist.

Use `router__handoff-to` with:
- `billing` for payment, invoices, subscription issues
- `technical` for product bugs, API issues, integration help
- `general` for everything else
```

### Extended Thinking Agent

```yaml
---
description: Complex problem solver with extended reasoning
models:
  - anthropic/claude-sonnet-4-20250514
reasoning: high
reasoningTokens: 32000
maxTurns: 10
temperature: 0.1
---

You solve complex problems using careful reasoning.

Take your time to think through the problem step by step.
```

---

## Troubleshooting

### "Unsupported frontmatter key(s)"

**Problem**: Error message like `Unsupported frontmatter key(s): myCustomKey`

**Cause**: Using a key that isn't allowed in frontmatter.

**Solution**:
- Check spelling (keys are case-sensitive)
- Some keys are CLI-only (see Forbidden Keys below)
- Review this reference for valid keys

### "Invalid frontmatter key(s): stream, verbose"

**Problem**: Error about runtime-only keys in frontmatter.

**Cause**: These keys are CLI flags, not frontmatter options.

**Solution**: Remove these keys from frontmatter. Use CLI flags instead:
```bash
ai-agent --stream --verbose --agent my-agent.ai
```

**Forbidden keys** (use CLI instead):
- `traceLLM`, `traceMCP`, `traceSdk`, `verbose`
- `accounting`, `save`, `load`
- `stream`, `targets`

### "Invalid provider/model pair"

**Problem**: Error like `Invalid provider/model pair 'gpt-4o'`

**Cause**: Models must include provider prefix.

**Solution**: Use `provider/model` format:
```yaml
# Wrong
models: gpt-4o

# Correct
models: openai/gpt-4o
```

### "Sub-agent missing 'description'"

**Problem**: Error when loading a sub-agent.

**Cause**: Sub-agents must have a `description` for the tool listing.

**Solution**: Add `description` to the sub-agent's frontmatter:
```yaml
---
description: Handles data processing tasks
---
```

### "Invalid reasoning level"

**Problem**: Error about invalid reasoning value.

**Cause**: Using an unsupported reasoning level.

**Solution**: Use one of: `none`, `unset`, `default`, `inherit`, `minimal`, `low`, `medium`, `high`

### "Invalid caching mode"

**Problem**: Error about invalid caching value.

**Cause**: Using unsupported caching mode.

**Solution**: Use `none` or `full`:
```yaml
---
caching: full   # Enable Anthropic prompt caching
caching: none   # Disable caching
---
```

### Agent hits maxTurns without completing

**Problem**: Agent produces synthetic failure, reaches turn limit.

**Cause**: Task requires more turns than allowed.

**Solutions**:
1. Increase `maxTurns`
2. Simplify the task in the prompt
3. Break into multiple specialized sub-agents
4. Use `agent__task_status` for progress tracking

### Tool outputs being truncated

**Problem**: Large tool outputs are replaced with handles.

**Cause**: Output exceeds `toolResponseMaxBytes`.

**Solutions**:
1. Increase `toolResponseMaxBytes` if context allows
2. The model can use `tool_output` to retrieve stored data
3. Configure `toolOutput.maxChunks` for better chunking

---

## See Also

- [Agent Files](Agent-Development-Agent-Files) - `.ai` file structure overview
- [Multi-Agent Orchestration](Agent-Development-Multi-Agent) - Advisor, router, and handoff patterns
- [Configuration Reference](Configuration) - `.ai-agent.json` configuration
- [Environment Variables](Getting-Started-Environment-Variables) - Environment configuration
- [CLI Reference](Getting-Started-CLI-Reference) - Command-line options
- [specs/frontmatter.md](specs/frontmatter.md) - Technical specification
