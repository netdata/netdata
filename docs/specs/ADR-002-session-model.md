# ADR-002: Session Model (Fresh Session per Run)

- Status: Accepted
- Date: 2025-09-04
- Owners: Core maintainers
- Related: DESIGN.md, ADR-001-sub-agent-as-tool.md

## Context

We must guarantee isolation, predictability, and safe concurrency across agent runs and sub-agent calls. Two patterns exist:
- Reusable, stateful sessions (risk of accidental reuse/shared mutation)
- Fresh session per run (stateless factory creates a new stateful session instance)

## Decision

Adopt a stateless factory model that creates a fresh session for each run.
- Public API constructs a new session instance per run.
- Any “retry” operation creates a new session under the hood (or is internal-only) and does not mutate the prior instance.
- Sessions are not reused across runs.

## Rationale

- Isolation: No cross-run state bleed by construction.
- Concurrency: Parallel runs never share mutable state.
- API clarity: A simple create → run → result model.
- Alignment: Matches the design principle of session autonomy (see DESIGN.md).

## Sub‑Agent Semantics (as Remote Tools)

- Sub‑agents are treated like foreign services (see ADR‑001). A sub‑agent call is one discrete tool execution (like an HTTP request).
- No implicit retries: on failure, the caller issues a new tool call (reconnect). If context is needed, it must be provided explicitly in the request payload.
- Any sub‑agent “session” state is represented as explicit tokens/handles returned by the tool and passed back by the caller.

## Invariants (Must Hold)

1) Fresh instances per run
   - Each run uses a newly constructed session instance. Public API does not encourage reusing sessions.

2) No global mutable state
   - No process-global environment or CWD effects within a session (see ADR‑001 invariants).

3) Structured results at boundaries
   - Public APIs return structured results (success/error), not thrown exceptions for predictable failures; internal layers may throw and are mapped to structured errors at boundaries.

4) Explicit context transfer
   - Any continued interaction with a sub‑agent uses explicit context tokens; no hidden state is retained between tool calls.

## Consequences

- Safer parallelism and simpler mental model for users.
- “Retry” semantics are explicit reconnects with optional explicit context.
- Pools (e.g., MCP/sub‑agent process pools) remain optimizations and must not preserve agent-level state.

## Acceptance Criteria (Engineering)

- Factory creates fresh sessions per run; code discourages/blocks reusing a session for multiple runs.
- If a retry API exists, it returns a new session instance or is internal-only.
- No `process.chdir` or `process.env` reads in session execution paths (enforced by ADR‑001).
- Public APIs consistently return structured results with error codes; internal exceptions are mapped to a unified error type.

