# Advanced

Advanced features and internal documentation.

---

## Overview

This section covers:
- Hidden CLI options not shown in `--help`
- Override keys for deep configuration
- Extended reasoning/thinking features
- Library API for embedding

---

## Advanced Topics

| Document | Description |
|----------|-------------|
| [Hidden CLI Options](Advanced-Hidden-CLI) | Options hidden from --help |
| [Override Keys](Advanced-Override-Keys) | Runtime configuration overrides |
| [Extended Reasoning](Advanced-Extended-Reasoning) | Thinking blocks and reasoning |
| [Internal API](Advanced-Internal-API) | Library embedding API |

---

## When to Use Advanced Features

**Hidden CLI Options**: For specialized workflows like:
- Advisors (parallel pre-run agents)
- Handoff (post-run execution chains)

**Override Keys**: For runtime testing and debugging:
- Disable batch tool execution
- Override context window settings
- Control interleaved reasoning

**Extended Reasoning**: For complex reasoning tasks:
- Chain-of-thought output
- Detailed thinking traces
- Provider-specific reasoning configuration

**Internal API**: For embedding in applications:
- No CLI dependency
- Full programmatic control
- Custom event handling

---

## Caution

These features are:
- Less documented than core features
- Subject to change between versions
- May have unexpected interactions

Use them when the documented features don't meet your needs, but expect to read source code for the latest behavior.

---

## See Also

- [Configuration](Configuration) - Standard configuration
- [Agent Development](Agent-Development) - Building agents
- [Technical-Specs](Technical-Specs) - Implementation details

