# Technical Specifications

Deep-dive technical documentation for contributors and maintainers.

---

## Overview

Technical specifications provide:
- Detailed architecture documentation
- Session lifecycle and state management
- Context window management algorithms
- Retry and recovery strategies
- Tool system internals
- User-facing contracts (SLAs)

---

## Specification Documents

### Core Architecture

| Document | Description |
|----------|-------------|
| [Architecture](Technical-Specs-Architecture) | Layered component architecture |
| [Session Lifecycle](Technical-Specs-Session-Lifecycle) | Session creation to completion |
| [Design History](Technical-Specs-Design-History) | Architectural decisions (ADRs) |

### Runtime Behavior

| Document | Description |
|----------|-------------|
| [Context Management](Technical-Specs-Context-Management) | Token budgets and context guard |
| [Retry Strategy](Technical-Specs-Retry-Strategy) | Error handling and provider cycling |
| [Tool System](Technical-Specs-Tool-System) | Tool providers and execution |

### Contracts

| Document | Description |
|----------|-------------|
| [User Contract](Technical-Specs-User-Contract) | End-user guarantees and SLAs |
| [Specifications Index](Technical-Specs-Index) | Full spec document index |

---

## Source Files

Key implementation files:

| Component | File | Description |
|-----------|------|-------------|
| Session | `src/ai-agent.ts` | Main orchestration |
| LLM Client | `src/llm-client.ts` | Provider interface |
| Tools | `src/tools/tools.ts` | Tool orchestration |
| Context | `src/ai-agent.ts` | Context guard |
| Types | `src/types.ts` | Core type definitions |

---

## Invariants

Core invariants that MUST hold:

1. **Session Immutability**: Config immutable after creation
2. **Turn Ordering**: Turn 0 = system, action turns are 1-based
3. **Provider Isolation**: Each provider handles own auth/protocol
4. **Context Guard**: Token budget checked before each LLM request
5. **Tool Budget**: Tool output size checked before commit
6. **Abort Propagation**: Cancellations propagate to all wait loops
7. **OpTree Consistency**: Every operation has begin/end lifecycle

---

## See Also

- [Agent Development](Agent-Development) - Building agents
- [Configuration](Configuration) - System configuration
- [Operations](Operations) - Debugging and monitoring

