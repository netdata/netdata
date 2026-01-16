# Specifications Index

Complete index of all technical specification documents for ai-agent.

---

## Table of Contents

- [Overview](#overview) - What this index covers
- [Core Specifications](#core-specifications) - Architecture, session, context, retry
- [Provider Specifications](#provider-specifications) - LLM provider integrations
- [Model Specifications](#model-specifications) - Model catalog and capabilities
- [Tool Specifications](#tool-specifications) - Tool system and providers
- [Headend Specifications](#headend-specifications) - Deployment interfaces
- [Configuration Specifications](#configuration-specifications) - Config loading and formats
- [Operations Specifications](#operations-specifications) - Logging, telemetry, accounting
- [Internal Documentation](#internal-documentation) - Internal reference documents
- [Design Documents](#design-documents) - Architecture decisions
- [Contract Documents](#contract-documents) - User guarantees
- [Implementation Documents](#implementation-documents) - SDK and library usage
- [Testing Documents](#testing-documents) - Testing guidance
- [See Also](#see-also) - Related documentation

---

## Overview

This index provides links to all technical specification documents. Specifications are organized by subsystem and provide detailed technical information for contributors and maintainers.

**Location**: Specs are stored in `docs/specs/` directory.

**Purpose**: Specifications document:

- Exact behaviors and algorithms
- Data structures and schemas
- Invariants and constraints
- Edge cases and error handling

---

## Core Specifications

Fundamental architecture and runtime behavior.

| Document           | Location                                                   | Description                       |
| ------------------ | ---------------------------------------------------------- | --------------------------------- |
| Architecture       | [specs/architecture.md](specs/architecture.md)             | Component structure and layering  |
| Session Lifecycle  | [specs/session-lifecycle.md](specs/session-lifecycle.md)   | Session creation to completion    |
| Context Management | [specs/context-management.md](specs/context-management.md) | Token budgets and guard algorithm |
| Retry Strategy     | [specs/retry-strategy.md](specs/retry-strategy.md)         | Error classification and recovery |

**Wiki Pages**:

- [Architecture](Technical-Specs-Architecture)
- [Session Lifecycle](Technical-Specs-Session-Lifecycle)
- [Context Management](Technical-Specs-Context-Management)
- [Retry Strategy](Technical-Specs-Retry-Strategy)

---

## Provider Specifications

LLM provider integrations and protocol handling.

| Document | Location | Description |
| -------- | -------- | ----------- |

| OpenAI Provider | [specs/providers-openai.md](specs/providers-openai.md) | OpenAI API integration |
| Anthropic Provider | [specs/providers-anthropic.md](specs/providers-anthropic.md) | Anthropic API integration |
| Google Provider | [specs/providers-google.md](specs/providers-google.md) | Google AI API integration |
| OpenRouter Provider | [specs/providers-openrouter.md](specs/providers-openrouter.md) | OpenRouter API integration |
| Ollama Provider | [specs/providers-ollama.md](specs/providers-ollama.md) | Ollama local API integration |
| Test Provider | [specs/providers-test.md](specs/providers-test.md) | Mock provider for testing |

---

## Model Specifications

Model catalog and capabilities.

| Document        | Location                                             | Description            |
| --------------- | ---------------------------------------------------- | ---------------------- |
| Models Overview | [specs/models-overview.md](specs/models-overview.md) | Model catalog overview |

---

## Tool Specifications

Tool system, providers, and execution.

| Document           | Location                                                     | Description                 |
| ------------------ | ------------------------------------------------------------ | --------------------------- |
| Tools Overview     | [specs/tools-overview.md](specs/tools-overview.md)           | Tool system architecture    |
| MCP Provider       | [specs/tools-mcp.md](specs/tools-mcp.md)                     | MCP protocol implementation |
| REST Provider      | [specs/tools-rest.md](specs/tools-rest.md)                   | REST/OpenAPI tools          |
| Final Report Tool  | [specs/tools-final-report.md](specs/tools-final-report.md)   | Built-in final_report tool  |
| Task Status Tool   | [specs/tools-task-status.md](specs/tools-task-status.md)     | Built-in task_status tool   |
| Agent Tool         | [specs/tools-agent.md](specs/tools-agent.md)                 | Agent spawning tool         |
| Batch Tool         | [specs/tools-batch.md](specs/tools-batch.md)                 | Batch execution tool        |
| XML Transport Tool | [specs/tools-xml-transport.md](specs/tools-xml-transport.md) | XML-based tool transport    |

**Wiki Page**: [Tool System](Technical-Specs-Tool-System)

---

## Headend Specifications

Deployment interfaces and protocols.

| Document          | Location                                                 | Description              |
| ----------------- | -------------------------------------------------------- | ------------------------ |
| Headends Overview | [specs/headends-overview.md](specs/headends-overview.md) | Headend architecture     |
| Embed Headend     | [specs/headend-embed.md](specs/headend-embed.md)         | Library embedding        |
| Manager Headend   | [specs/headend-manager.md](specs/headend-manager.md)     | Headend manager          |
| REST Headend      | [specs/headend-rest.md](specs/headend-rest.md)           | REST API server          |
| MCP Headend       | [specs/headend-mcp.md](specs/headend-mcp.md)             | MCP server mode          |
| OpenAI Headend    | [specs/headend-openai.md](specs/headend-openai.md)       | OpenAI-compatible API    |
| Anthropic Headend | [specs/headend-anthropic.md](specs/headend-anthropic.md) | Anthropic-compatible API |
| Slack Headend     | [specs/headend-slack.md](specs/headend-slack.md)         | Slack bot integration    |

**Wiki Pages**: [Headends](Headends)

---

## Configuration Specifications

Configuration loading, formats, and schemas.

| Document              | Location                                                         | Description                         |
| --------------------- | ---------------------------------------------------------------- | ----------------------------------- |
| Configuration Loading | [specs/configuration-loading.md](specs/configuration-loading.md) | Config file resolution and layering |
| Frontmatter           | [specs/frontmatter.md](specs/frontmatter.md)                     | Agent file YAML format              |
| Pricing               | [specs/pricing.md](specs/pricing.md)                             | Cost configuration and calculation  |

**Wiki Pages**: [Configuration](Configuration)

---

## Operations Specifications

Production operations and observability.

| Document   | Location                                                   | Description                     |
| ---------- | ---------------------------------------------------------- | ------------------------------- |
| Logging    | [specs/logging-overview.md](specs/logging-overview.md)     | Log system architecture         |
| Telemetry  | [specs/telemetry-overview.md](specs/telemetry-overview.md) | Metrics and distributed tracing |
| Accounting | [specs/accounting.md](specs/accounting.md)                 | Cost tracking and aggregation   |
| Snapshots  | [specs/snapshots.md](specs/snapshots.md)                   | Session snapshot format         |
| OpTree     | [specs/optree.md](specs/optree.md)                         | Operation tree structure        |
| Call Path  | [specs/call-path.md](specs/call-path.md)                   | Call path tracking              |
| Logs       | [specs/LOGS.md](specs/LOGS.md)                             | Log format reference            |

**Wiki Pages**: [Operations](Operations)

---

## Internal Documentation

Internal reference and troubleshooting documents.

| Document     | Location                                                         | Description                  |
| ------------ | ---------------------------------------------------------------- | ---------------------------- |
| Specs Index  | [specs/index.md](specs/index.md)                                 | Complete specs index         |
| Specs README | [specs/README.md](specs/README.md)                               | Directory overview           |
| Internal API | [specs/AI-AGENT-INTERNAL-API.md](specs/AI-AGENT-INTERNAL-API.md) | Internal agent API reference |
| AGENTS.md    | [specs/AGENTS.md](specs/AGENTS.md)                               | Agent system reference       |
| CLAUDE.md    | [specs/CLAUDE.md](specs/CLAUDE.md)                               | Meta-specification format    |
| GEMINI.md    | [specs/GEMINI.md](specs/GEMINI.md)                               | Meta-specification format    |
| REASONING.md | [specs/REASONING.md](specs/REASONING.md)                         | Reasoning guidance           |
| SLACK.md     | [specs/SLACK.md](specs/SLACK.md)                                 | Slack integration notes      |
| CRUSH.md     | [specs/CRUSH.md](specs/CRUSH.md)                                 | CRUSH protocol notes         |
| WRAP.md      | [specs/WRAP.md](specs/WRAP.md)                                   | WRAP protocol notes          |

---

## Design Documents

Architecture decisions and design rationale.

| Document                   | Location                                                                 | Description                  |
| -------------------------- | ------------------------------------------------------------------------ | ---------------------------- |
| Design Overview            | [specs/DESIGN.md](specs/DESIGN.md)                                       | High-level design philosophy |
| ADR-001: Sub-Agent as Tool | [specs/ADR-001-sub-agent-as-tool.md](specs/ADR-001-sub-agent-as-tool.md) | Tool abstraction decision    |
| ADR-002: Session Model     | [specs/ADR-002-session-model.md](specs/ADR-002-session-model.md)         | Fresh session decision       |
| Multi-Agent                | [specs/MULTI-AGENT.md](specs/MULTI-AGENT.md)                             | Multi-agent design patterns  |

**Wiki Page**: [Design History](Technical-Specs-Design-History)

---

## Contract Documents

User-facing guarantees and SLAs.

| Document               | Location                                                                               | Description                           |
| ---------------------- | -------------------------------------------------------------------------------------- | ------------------------------------- |
| User Contract          | [specs/CONTRACT.md](specs/CONTRACT.md)                                                 | End-user behavioral guarantees        |
| AI Configuration Guide | [docs/skills/ai-agent-configuration.md](docs/skills/ai-agent-configuration.md)         | AI-facing configuration reference     |
| AI Snapshots Guide     | [docs/skills/ai-agent-session-snapshots.md](docs/skills/ai-agent-session-snapshots.md) | AI-facing session snapshots reference |

**Wiki Page**: [User Contract](Technical-Specs-User-Contract)

---

## Implementation Documents

SDK integration and library usage.

| Document       | Location                                           | Description                   |
| -------------- | -------------------------------------------------- | ----------------------------- |
| Implementation | [specs/IMPLEMENTATION.md](specs/IMPLEMENTATION.md) | Vercel AI SDK integration     |
| Library API    | [specs/library-api.md](specs/library-api.md)       | Embedding ai-agent in Node.js |
| Parameters     | [specs/PARAMETERS.md](specs/PARAMETERS.md)         | CLI parameter reference       |

---

## Testing Documents

Testing approaches and harness usage.

| Document      | Location                                     | Description               |
| ------------- | -------------------------------------------- | ------------------------- |
| Testing Guide | [Contributing-Testing](Contributing-Testing) | Test harness and coverage |

---

## Quick Reference

### By Use Case

| Use Case                   | Start Here                                                 |
| -------------------------- | ---------------------------------------------------------- |
| Understanding architecture | [specs/architecture.md](specs/architecture.md)             |
| Debugging session issues   | [specs/session-lifecycle.md](specs/session-lifecycle.md)   |
| Context window problems    | [specs/context-management.md](specs/context-management.md) |
| Retry/error behavior       | [specs/retry-strategy.md](specs/retry-strategy.md)         |
| Model information          | [specs/models-overview.md](specs/models-overview.md)       |

| Tool development | [specs/tools-overview.md](specs/tools-overview.md) |
| Headend development | [specs/headends-overview.md](specs/headends-overview.md) |
| Configuration questions | [specs/configuration-loading.md](specs/configuration-loading.md) |
| Contract guarantees | [specs/CONTRACT.md](specs/CONTRACT.md) |

### By File Type

| Extension        | Content                        |
| ---------------- | ------------------------------ |
| `specs/*.md`     | Technical specifications       |
| `specs/ADR-*.md` | Architectural Decision Records |
| `skills/*.md`    | AI-facing documentation        |
| `specs/*-*.md`   | Internal reference documents   |

---

## See Also

- [Technical-Specs](Technical-Specs) - Technical specifications overview
- [Contributing](Contributing) - Contribution guidelines
- [Operations](Operations) - Production operations
- [Configuration](Configuration) - Configuration reference
