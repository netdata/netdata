# In‑Memory Multi‑Agent Architecture

This document proposes and specifies an in‑memory, preloaded, multi‑agent system built on top of the existing AI Agent architecture (see DESIGN.md). The goal is to let one “parent” agent orchestrate other “sub‑agents” as callable tools, while preserving strict session isolation, performance, and developer ergonomics.

## Executive Summary

- Preload sub‑agents in the same process and expose each as a first‑class tool to the parent agent.
- Preserve session isolation per DESIGN.md: each agent run is a completely independent session (MCP clients, provider instances, prompts, accounting, logs). Avoid global state by using per‑session env overlays instead of mutating `process.env`.
- Use frontmatter as the single source of truth for sub‑agent input/output contracts and tool metadata. Print a copy‑pasteable frontmatter template in `--help` for discoverability.
- Provide a tiny in‑process Subagent Registry that validates/loads children at startup and executes them as tools with bounded concurrency, recursion caps, and budgets.
- Integrate with Slack and a web chat as thin I/O layers over the parent agent session, streaming child events.

## Goals & Non‑Goals

### Goals
- Minimal cognitive overhead for composing agents: each agent remains a single prompt file with frontmatter; the parent consumes child agents as tools.
- Strong isolation: no shared mutable state; per‑session clients; per‑session env overlays.
- Performance: preload children; run sub‑agents in‑process without OS process overhead.
- Transparency: `--help` prints a complete frontmatter template for any agent (copy‑pasteable), reflecting effective defaults.
- Governed execution: concurrency caps, recursion caps, and token/time budgets; clear failure semantics.

### Non‑Goals
- Replacing MCP: MCP remains the mechanism for external tools. “Agents as tools” are an internal execution path (in‑process), not exposed via MCP.
- Remote orchestration: this design focuses on a single process. Horizontal scaling and remote fan‑out are future work.

## Key Requirements (from DESIGN.md)

- Complete session autonomy: each run creates new MCP clients, provider instances, conversation, accounting, and retry state.
- No shared state, no process‑global mutable configuration used across sessions.
- Clear status interfaces and accounting.
- The core library does no I/O; the CLI or integrators wire callbacks.

## Architecture Overview

### Parent Agent (Orchestrator)
- Loads a Subagent Registry at startup (paths configured in a special config or the parent’s frontmatter).
- Exposes one AI‑SDK‑compatible tool per sub‑agent to the parent LLM (tool name, description, input schema from child frontmatter).
- On tool call, spawns a fresh in‑process child `AIAgentSession`, runs it, and returns its final output to the parent as the tool result.

### Subagent Registry
- Discovers child agent prompt paths.
- Parses child frontmatter (description, usage, input/output schemas, optional toolName, limits).
- Resolves a per‑agent env overlay from `.ai-agent.json` + `.ai-agent.env` + runtime env, **without mutating `process.env`**.
- Validates providers and MCP servers; optionally preconnect or lazy connect.
- Produces AI‑SDK tool definitions and a callable `run(childName, args)` method that launches a new `AIAgentSession` for each execution.

### Child Agent Execution (In‑Process)
- For each call:
  - Create a fresh `AIAgentSession` using the child’s prompt, expected output, providers, MCP servers, and session limits.
  - Pass a per‑session env overlay down to provider/MCP layers (no global env reads).
  - Stream tokens and tool events to the parent’s callbacks; aggregate accounting.
  - Return the final report (prefer JSON) as the tool result to the parent LLM.

## Frontmatter Contract (Composition‑Friendly)

All agents already use frontmatter; for composition we standardize the following keys. The `--help` output prints a YAML block so users can copy, paste, and edit.

Required:
- `description`: Human‑readable what/why the agent does.
- `usage`: A concise example of expected input; used in help and usage line.
- `output`: `{ format: json|markdown|text, schema?: object }` — JSON is strongly recommended for machine composition.

Recommended for sub‑agent tools:
- `toolName`: Stable identifier exposed to the parent LLM.
- `input`: `{ format: text|json, schema?: object }` — define JSON schema for structured inputs when possible.
- `limits`: `{ maxToolTurns, llmTimeout, toolTimeout, maxRetries, parallelToolCalls }` — per‑subagent call overrides.

Example:
```yaml
---
description: ICP Intelligence Researcher
usage: company name or contact email
toolName: research.icp
input:
  format: text
output:
  format: json
  schema:
    type: object
    required: [company, insights]
    properties:
      company: { type: string }
      insights: { type: array, items: { type: string } }
limits:
  maxToolTurns: 6
  llmTimeout: 120000
  toolTimeout: 60000
  maxRetries: 2
  parallelToolCalls: true
---
```

## Library Code Organization & Virtual Tools

To keep concerns separated and the library manageable, the library should be focused, embeddable, and accept "virtual tools" via a clean interface. Multi‑agent orchestration lives outside the library and prepares those virtual tools.

### Library (Core)
- `core/agent-session`: `AIAgentSession` (turn loop, retries, fallback, finalization)
- `core/types`: `TurnStatus/Result`, `AccountingEntry`, `ConversationMessage`, etc.
- `core/tooling`:
  - `ToolDefinition` interface (virtual tools):
    - `name: string`
    - `description?: string`
    - `instructions?: string` (optional system‑prompt append)
    - `inputSchema: object` (JSON Schema; AI‑SDK compatible)
    - `execute(args: Record<string, unknown>, ctx?: { signal?: AbortSignal }): Promise<string | { type: 'json'|'text', value: unknown }>`
  - `ToolSource` (optional): `list(): Promise<ToolDefinition[]>` for lazy sources
- `llm/*`: provider adapters (OpenAI, OpenRouter, Anthropic, Google, Ollama)
- `mcp/*`: adapter layer to turn MCP tools into `ToolDefinition`s
- `env/overlay`: per‑session env overlay (stop reading `process.env` in deep layers)

### Library API Changes
- `AIAgentSessionConfig`:
  - `tools: ToolDefinition[]` (generic virtual tools; MCP becomes an adapter)
  - `envOverlay?: Record<string, string>` per‑session overlay passed to providers/MCP clients
  - existing: limits, callbacks, trace/verbose flags, etc.
- System prompt builder accepts call‑supplied `instructions` aggregate from tools; library does not inject arbitrary content.

### Adapters (Outside Library)
- `frontmatter`: parse `.ai` prompts to `{ description, usage, input/output, limits, toolName }` and build `ToolDefinition`s
- `config+env`: resolve `.ai-agent.json` + `.ai-agent.env` into per‑agent env overlays (no `process.env` mutation)

### Orchestrators (Outside Library)
- `subagent-registry`: preload child prompts → validate → export `ToolDefinition`s
- `multi-agent runner`: bounded concurrency, recursion caps, budgets

### Integrations (Outside Library)
- CLI wiring (current)
- Slack bot / Web chat

## Environment & Secret Handling (Per‑Session Overlay)

Problem: Today configuration expansion and MCP header/env resolution read `process.env`. Our `.ai-agent.env` sidecar logic currently hydrates `process.env`. For strict in‑process isolation:

- Replace global env mutation with a per‑session env overlay:
  - Resolve placeholders `${VAR}` for only the providers/MCP servers needed by the session from:
    1) `.ai-agent.json` (raw parse for placeholders)
    2) `.ai-agent.env` (sidecar file; supports blank lines and `#` comments; `KEY=VALUE` or `KEY="VALUE"`)
    3) The process environment (read‑only, last resort)
  - Build an immutable map `{ VAR -> value }` per session and pass it to provider/MCP layers in place of `process.env`.
  - Do not mutate `process.env`.

Benefits:
- No cross‑talk between sessions.
- Tight scoping: each sub‑agent uses only its own env overlay.
- Easy validation and helpful errors (“Missing: OPENAI_API_KEY; define in shell or .ai-agent.env”).

## Execution Model & Scheduling

- Parent LLM sees each sub‑agent as a tool (from registry). Input schemas derive from child frontmatter (`input` block).
- On tool call, orchestrator runs the child session:
  - Fresh session state; isolated provider/MCP clients; limits from child frontmatter.
  - Returns final output to the parent as tool result.
- `AbortSignal` support: `AIAgentSession.run()` and `ToolDefinition.execute()` accept cancellation; parent can cancel children on stop.

Concurrency:
- Bounded parallel sub‑agent calls per turn (e.g., 2–4). Queue excess calls.

Recursion & Cycle Guards:
- Track ancestry (stack of tool names/paths). Prevent cycles (A→B→A) and set a max depth (e.g., 2–3).

Budgets:
- Per‑child token/time budget and a global budget per parent run.
- Timeouts and graceful fallbacks (partial results are allowed with explicit status).

## Accounting & Observability

- Aggregate child accounting into parent FIN summaries with clear attribution:
  - LLM requests (provider, model, cost, tokens) and latencies per child.
  - MCP tool calls per child (server/tool, status, bytes/characters in/out) and latencies.
- Logging conventions:
  - Prefix child logs with `child:<toolName>` and a run id.
  - Respect library rules: no stdout from core; callbacks handle stderr.

## Developer Ergonomics

- `--help` prints:
  - DESCRIPTION
  - Usage: `<invocation> "<usage>"`
  - Frontmatter Template: a YAML block with all supported frontmatter keys and resolved defaults (frontmatter > config.defaults > internal)
- Copy/paste flow: run `--help`, paste the frontmatter block into a new `.ai` file, edit fields, and you have a new sub‑agent ready for registration.

## Slack Bot & Web Chat

Slack Bot:
- Each Slack thread represents a parent session.
- Stream tokens and child call events as messages.
- Controls: stop, retry, show child details, download JSON outputs.
- Per-user budgets and rate-limits; secret redaction.

Web Chat:
- SSE/WebSocket streaming from parent session.
- “Timeline” UI showing child calls, streamed outputs, and FIN summaries.
- Controls: stop, retry, set budgets; view frontmatter templates.

## External Headends for Multi-Agent Systems

- The headend manager allows a single CLI invocation to expose multiple agents (parent and sub-agents) over REST, MCP, or provider-compatible endpoints. Register each agent with `--agent` so the headends share the preloaded registry.
- REST clients can call `GET /v1/:agent?q=...&format=...` for any registered agent. When requesting JSON (`format=json`), the caller must supply a `schema` to match the agent’s output contract.
- MCP clients connect via the configured transport (`stdio`, `/mcp` HTTP, `/mcp/sse`, or WebSocket). Tool arguments must contain `format`, and JSON mode must include a `schema` field.
- OpenAI-compatible and Anthropic-compatible headends map agents to models/tools. These surfaces respect the same concurrency caps and stream semantics as the in-process orchestration layer.
- Because each request spawns an isolated `AIAgentSession`, agents invoked via headends remain composable: a REST call can trigger the parent agent, which in turn can call sub-agents as internal tools.

## Validation & Startup Flow

On master startup:
1. Discover child `.ai` prompt files from configured paths.
2. For each child:
   - Parse frontmatter (description, usage, input/output, limits, toolName).
   - Resolve per‑agent env overlay; validate required keys.
   - Validate providers/MCP servers and gather tool schemas; fail fast with specific errors.
3. Register each child as a tool in the Subagent Registry.
4. Expose tools to the parent LLM.

## Failure Semantics

- Child failure → tool error result to parent LLM with:
  - Class: config/auth/timeout/network/model
  - Brief stderr/summary and suggested remediation when possible
- Parent decides whether to retry, fallback to another child, or conclude.

## Migration Plan

Phase 1 — In‑Process Registry (Preload)
- SubagentRegistry loads child prompts and frontmatter; validates configs/keys/tool schemas.
- Exposes AI‑SDK tools; each call launches a new child `AIAgentSession`.
- Keep current sidecar `.ai-agent.env` behavior as a temporary step.

Phase 2 — Per‑Session Env Overlay
- Refactor provider/MCP resolution to accept a per‑session env map instead of reading `process.env`.
- Remove any global env mutation.

Phase 3 — Governance & Aggregation
- Add recursion caps, bounded parallelism, token/time budgets, and hierarchical FIN summaries.

Phase 4 — Slack & Web Chat
- Add Slack bot + web chat, streaming child events; controls and JSON inspection.

## Open Questions

- Output standardization: Should we mandate JSON for sub‑agents? If not, provide a small wrapper to convert Markdown → JSON for downstream reasoning.
- Planner prompts: Do we rely fully on provider‑native tool planners, or add soft constraints (e.g., “spawn max 2 child agents concurrently unless justified”)?
- Shared workspace: Should we offer a shared ephemeral key‑value store for the parent to stitch child outputs, or keep everything conversation‑based?

## Summary

In‑memory, preloaded composition is the right design for performance and developer ergonomics. With per‑session env overlays (to honor DESIGN.md’s isolation), a tiny Subagent Registry, and frontmatter‑driven contracts, the system becomes a fast, safe, and transparent multi‑agent platform. From there, Slack and web chat are thin integrations that surface the same orchestration with streaming, control, and inspectability.
