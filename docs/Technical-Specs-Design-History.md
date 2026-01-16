# Design History

Architectural Decision Records (ADRs) documenting key design choices.

---

## Overview

ADRs capture significant architectural decisions, their context, and rationale. They serve as historical documentation of why the system is designed the way it is.

---

## ADR-001: Sub-Agent as Tool

**Status**: Accepted
**Date**: 2025-09-04

### Context

Multi-agent behavior can be architected two ways:
1. Sub-agents as first-class tools (tool abstraction)
2. Recursive orchestrator coordinating autonomous agents

### Decision

**Sub-agent = Tool, always and only.**

The master agent is unaware of a tool's origin (MCP server, sub-agent, internal tool). The agent loop interacts with a unified tool interface.

### Rationale

- **Predictability**: Single abstraction reduces cognitive load
- **Isolation**: Tool calls are naturally bounded
- **Maintainability**: Unified registry is easier to test

### Invariants

1. **Opaque Origin**: No branching on tool origin type
2. **No Global Mutable State**: No `process.chdir`, no direct `process.env` reads
3. **Per-Session Isolation**: Tools receive only explicit inputs
4. **Uniform Budgets**: Depth caps and resource limits apply uniformly
5. **Unified Observability**: Consistent logging independent of origin
6. **Semantics over Optimization**: Pooling must not change isolation

---

## ADR-002: Session Model

**Status**: Accepted
**Date**: 2025-09-04

### Context

Need to guarantee isolation and predictability across agent runs:
- Reusable, stateful sessions (risk of shared mutation)
- Fresh session per run (stateless factory)

### Decision

**Fresh session per run.**

- Public API constructs new session per run
- Retry operations create new sessions internally
- Sessions are not reused across runs

### Rationale

- **Isolation**: No cross-run state bleed
- **Concurrency**: Parallel runs never share state
- **API clarity**: Simple create → run → result model

### Sub-Agent Semantics

- Sub-agents treated like foreign services
- No implicit retries on failure
- Context provided explicitly in request payload
- Session state represented as explicit tokens/handles

### Invariants

1. **Fresh instances per run**: No session reuse
2. **No global mutable state**: Enforced by ADR-001
3. **Structured results at boundaries**: No thrown exceptions
4. **Explicit context transfer**: No hidden state

---

## Design Principles

These principles guide ongoing development:

### 1. Tool Abstraction

All external capabilities are tools:
- MCP servers
- REST APIs
- Sub-agents
- Internal utilities

### 2. Session Isolation

Each session is independent:
- Own conversation history
- Own accounting
- Own context budget
- No shared mutable state

### 3. Fail-Fast with Boom

- Silent failures are not justified
- All error conditions must be logged
- Only user config errors stop execution
- Other errors: retry, recover, continue

### 4. Model-Facing Error Quality

Error messages to the model must be:
- Extremely detailed
- Descriptive of the exact issue
- Provide direct instructions to overcome

### 5. Thin Orchestration Loops

Keep main loops lean:
- Move complexity to specialized modules
- Separation of concerns is paramount
- Gradual improvement over time

---

## Future Considerations

Areas for potential architectural evolution:

1. **Tool/MCP Pooling**
   - Reduce cold-start latency
   - Must preserve overlays and isolation

2. **Interactive Agents**
   - Human or agent-to-agent messaging
   - Built on tool abstraction

3. **Streaming Improvements**
   - Real-time token output
   - Progressive tool results

---

## See Also

- [Architecture](Technical-Specs-Architecture) - Current architecture
- [specs/ADR-001-sub-agent-as-tool.md](specs/ADR-001-sub-agent-as-tool.md) - Full ADR
- [specs/ADR-002-session-model.md](specs/ADR-002-session-model.md) - Full ADR
- [specs/DESIGN.md](specs/DESIGN.md) - Design overview

