# TODO - A2A Headend Research

## TL;DR
- Research the A2A (Agent2Agent) protocol, spec, and official SDKs to assess feasibility for an ai-agent A2A headend.
- Map A2A requirements to existing headend architecture (HeadendManager + per-headend adapters) and identify integration implications.

## Analysis
- Current headends are: REST (`RestHeadend`), MCP (`McpHeadend`), OpenAI Chat Completions compat, Anthropic Messages compat, and Slack (`SlackHeadend`). These are managed by `HeadendManager` with per-headend concurrency limits and shared agent registry. (docs/IMPLEMENTATION.md:207-217, docs/DESIGN.md:50, docs/SPECS.md:43-69)
- Headend mode is CLI-driven: repeated `--agent` registrations + one or more headend flags (`--api`, `--mcp`, `--openai-completions`, `--anthropic-completions`). (docs/SPECS.md:43-69, 211-215)
- Session creation can be tagged with `headendId` to correlate logs/telemetry (internal API). (docs/AI-AGENT-INTERNAL-API.md:13-41)
- Protocol surfaces expect consistent output contracts (format + schema) and enforce them at the headend boundary; this will matter for any A2A schema/format contract. (docs/SPECS.md:64-65, docs/DESIGN.md:50)
- New headends must remain thin and avoid bloating core orchestration loops; separation of concerns is a hard requirement. (docs/DESIGN.md:10-55, PR-006)
- Behavior-changing headend additions require synchronized documentation updates. (AGENTS.md project-doc rules)

## Decisions
1. A2A server will be implemented as a **headend** (peer‑facing server entry point), comparable to existing headends. (User decision)
2. A2A client will be implemented as a **tool** (new tool method, comparable to MCP), callable by agents. (User decision)
3. Both A2A server and client will use the **official A2A SDKs** to implement the protocol. (User decision)
4. A2A protocol capabilities may require enhancements to headend/tool features; proceed with full capability scope. (User decision)
5. Transport scope: **all transports the official SDK supports** (no selective subset). (User decision)
6. Spec version target: **latest stable official release**. (User decision)
7. Auth model: **public/unauthed** for now; authentication/authorization out of scope. (User decision)
8. Task persistence: **in-memory initially**, but **pluggable by design** to allow future persistence backends. (User decision)
9. A2A client tool streaming: **aggregate stream to a single tool result for now**, with async/background support deferred to the async‑tools roadmap. (User decision, confirmed 3.2)
10. Open decision: **A2A headend integration approach** — use SDK Express integration (new dependency) vs use SDK core handlers with existing Node `http` server. (Pending)
11. Open decision: **AgentCard ↔ ai-agent mapping** — one A2A headend per agent vs a single AgentCard advertising multiple skills mapped to multiple registered agents. (Pending)
12. Open decision: **Input/output modes + file/data parts** — how to represent A2A file/data parts inside ai-agent’s string‑based prompts and tool results without losing “full capability” semantics. (Pending)

## Plan
- Research A2A protocol: spec scope, transport(s), message formats, auth, versioning, and official SDKs.
- Identify whether A2A expects server-side agent or client-side peer behavior, and how that maps to ai-agent headend mode.
- Summarize integration risks (compatibility, maintenance, test coverage, docs updates).
- Propose next-step options for Costa if/when implementation is desired.

## Implied Decisions
- None yet. Will depend on which A2A role/transports and spec version are selected.

## Testing Requirements
- Research-only task: none.
- If implementing later: add/update Phase 1 harness scenarios for new headend entry path and error cases per testing policy. (docs/TESTING.md)

## Documentation Updates Required
- Research-only task: none.
- If implementing later: update docs/SPECS.md, docs/IMPLEMENTATION.md, docs/DESIGN.md, README.md, and any headend docs impacted. (AGENTS.md project-doc rules)
