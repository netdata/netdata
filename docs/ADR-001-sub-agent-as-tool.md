# ADR-001: Sub‑Agent as Tool (Canonical Model)

- Status: Accepted
- Date: 2025-09-04
- Owners: Core maintainers
- Related: docs/DESIGN.md, docs/MULTI-AGENT.md

## Context

We support multi‑agent behavior. There are multiple ways to architect this:
1) Sub‑agents as first‑class tools invoked by the main agent (tool abstraction).
2) A recursive orchestrator that instantiates and coordinates autonomous agents.

To minimize complexity and preserve isolation, we adopt (1): a sub‑agent is always and only a tool.

## Decision

- Sub‑agent = Tool, always and only. The master agent is unaware of a tool’s origin (MCP server, sub‑agent, internal tool, etc.).
- The agent loop interacts with a unified tool interface (name, description, input schema, execute(params) → result/error) with no branching on origin.
- This is the canonical, permanent abstraction for multi‑agent execution in this project.

## Rationale

- Predictability: A single abstraction reduces cognitive load and avoids a second orchestration plane.
- Isolation: Tool calls are naturally bounded; budgets and depth caps apply uniformly.
- Maintainability: A unified registry and execution path are easier to reason about, optimize, and test.

## Invariants (Must Hold)

1) Opaque Origin
   - The session/loop does not branch on tool origin type (MCP vs sub‑agent vs internal). Any origin metadata is for logs only.

2) No Global Mutable State
   - No `process.chdir` during execution.
   - No direct `process.env` reads during a session. Use per‑session environment overlays.

3) Per‑Session Isolation
   - Tools (including sub‑agents) receive only explicit inputs and session overlays.
   - No implicit shared state across agents/sessions.

4) Uniform Budgets and Guards
   - Depth caps, resource budgets (time/tokens/turns), and concurrency gating apply uniformly to all tool calls.

5) Unified Observability
   - Structured logs and accounting identify tool calls consistently (name, latency, sizes, trace IDs) independent of origin.

6) Semantics over Optimization
   - Pooling (e.g., pre‑initialized MCP servers or sub‑agent runners) is an internal optimization only. It must not change semantics or isolation.

## Non‑Goals

- We will not implement a recursive orchestrator that merges agent coordination into the main loop.
- We will not expose origin‑dependent behaviors to the agent loop.

## Consequences

- Simpler control flow and reduced surface for bugs.
- Clearer guardrails (budgets, depth, concurrency) enforced in one place.
- Performance work (e.g., pools) happens behind the registry and respects overlays/isolations.

## Acceptance Criteria (Engineering)

- Code does not branch on tool origin in the agent loop. Sub‑agent execution is encapsulated behind the tool registry.
- No `process.chdir` usage; all path resolution uses explicit `baseDir`.
- No `process.env` reads within a session/provider path; resolver/providers accept per‑session overlays.
- Concurrency gate and recursion depth cap apply equally to all tool calls.
- Logs/accounting are origin‑agnostic in structure; origin metadata (if present) is optional and for debugging only.

## Future Work (Compatible with this ADR)

- Tool/MCP pooling to reduce cold‑start latency (must preserve overlays and isolation).
- Interactive agents (human or agent‑to‑agent) built on the same tool abstraction (message‑passing through execute()).

## Glossary

- Tool: Executable capability with a name, description, input schema, and execute(params) → result/error.
- Sub‑Agent: A tool whose implementation internally runs another agent session, opaque to the caller.
