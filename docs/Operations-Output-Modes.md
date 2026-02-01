# Output Modes (Agentic vs Chat)

How ai-agent decides when a response is “final” and how output is streamed to users.

---

## Table of Contents

- [Overview](#overview) - When to use each mode
- [Agentic Mode](#agentic-mode) - Strict final-report enforcement
- [Chat Mode](#chat-mode) - Stream-first UX for chat bots
- [Analysis: Differences](#analysis-differences) - Side-by-side comparison
- [Defaults and Configuration](#defaults-and-configuration) - Headend and CLI defaults
- [See Also](#see-also) - Related documentation

---

## Overview

ai-agent supports two output modes:

- **Agentic mode**: strict final-report enforcement (robust, tool-heavy workflows)
- **Chat mode**: stream-first output for chat bots (fast UX, no XML wrapper enforcement)

Both modes apply only to the **master agent** and its **handoff sessions**. Advisors and sub-agents remain agentic.

---

## Agentic Mode

Agentic mode is the default for most headends and for CLI (unless `--chat` is set).

Key behavior:

- Requires an accepted **final_report** (XML-wrapped) before the turn can finish
- Retries if the XML wrapper or required META is missing/invalid
- Best for **tools**, **schemas**, and **structured output contracts**

See [Analysis: Differences](#analysis-differences) for a direct comparison.

---

## Chat Mode

Chat mode is built for chat bots where users expect immediate, uninterrupted streaming.

Key behavior:

- Final report content equals the **streamed output** (think/META filtered), aggregated across turns
- Treats all model output as **final-report content** (no XML wrapper enforcement)
- Stop condition: `stop=stop` **and** no tools (ok/failed/unknown) **and** non-empty output
- If `stop=stop` arrives with empty output, the turn **retries**
- Any stop reason other than `stop` means **continue**
- Required META is **still enforced**; missing META triggers a retry with META-only guidance

See [Analysis: Differences](#analysis-differences) for a direct comparison.

---

## Analysis: Differences

| Dimension | Agentic Mode | Chat Mode |
| --- | --- | --- |
| Finalization rule | Requires accepted XML final-report | `stop=stop` + no tools + non-empty output |
| Retries | Retries on missing/invalid XML/META | Retries on empty output, non-`stop` reasons, and missing META |
| Streaming UX | Can duplicate output on retries | Stream-first, avoids duplicate output |
| Tool workflows | Strong guarantees | Stops only when no tool activity |
| Plugin META | Enforced | Enforced |
| Best for | Structured/contracted outputs | Human-facing chat bots |

**Implications / risks**:

- Chat mode trades strict wrapper enforcement for UX. Use agentic mode if you need XML wrapper guarantees.
- Chat mode still enforces required META. Missing META triggers a META-only retry.
- Tools delay finalization in chat mode. Any tool activity prevents stop.

---

## Defaults and Configuration

**Headend defaults**:

- **Chat mode ON**: Embed, OpenAI-Compatible, Anthropic-Compatible
- **Chat mode OFF**: REST, MCP, Slack, Library

**CLI defaults**:

- `--chat` enables chat mode
- `--no-chat` disables chat mode (default)

---

## See Also

- [Headends](Headends)
- [CLI](CLI)
- [Technical Specs - Session Lifecycle](Technical-Specs-Session-Lifecycle)
