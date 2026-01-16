# Advanced

Power-user features for specialized workflows, deep configuration control, and programmatic embedding.

---

## Table of Contents

- [Overview](#overview) - What this section covers
- [When to Use Advanced Features](#when-to-use-advanced-features) - Decision guide for each feature
- [Advanced Topics](#advanced-topics) - Page index with descriptions
- [Stability Notes](#stability-notes) - What to expect from these features
- [See Also](#see-also) - Related documentation

---

## Overview

This section documents advanced ai-agent capabilities that are:

- **Hidden from `--help`**: CLI options for specialized orchestration patterns
- **Undocumented defaults**: Runtime overrides for testing and debugging
- **Provider-specific**: Extended reasoning configuration
- **Programmatic**: Library API for embedding in applications

These features are fully functional but may change between versions. Use them when standard features don't meet your needs.

---

## When to Use Advanced Features

| Feature | Use When |
|---------|----------|
| [Hidden CLI Options](Advanced-Hidden-CLI) | Building multi-stage workflows with advisors or handoff chains |
| [Override Keys](Advanced-Override-Keys) | Testing/debugging with forced configuration (bypasses frontmatter) |
| [Extended Reasoning](Advanced-Extended-Reasoning) | Using thinking/reasoning models (Claude, o1, etc.) |
| [Internal API](Advanced-Internal-API) | Embedding ai-agent in Node.js applications |

---

## Advanced Topics

| Document | Type | Description |
|----------|------|-------------|
| [Hidden CLI Options](Advanced-Hidden-CLI) | Reference (E) | CLI options hidden from `--help` for advisors and handoff |
| [Override Keys](Advanced-Override-Keys) | Reference (E) | Runtime `--override` keys for testing and debugging |
| [Extended Reasoning](Advanced-Extended-Reasoning) | Configuration (C) | Thinking blocks and reasoning mode configuration |
| [Internal API](Advanced-Internal-API) | Reference (E) | Library embedding API for programmatic use |

---

## Stability Notes

These features are:

| Aspect | Status |
|--------|--------|
| **Functional** | Work as designed in current version |
| **Less Stable** | May change without deprecation warnings |
| **Less Documented** | Source code is the definitive reference |
| **Supported** | Bug reports welcome, behavior changes expected |

> **Tip:** When using advanced features, pin your ai-agent version and test upgrades before deploying.

---

## See Also

- [Configuration](Configuration) - Standard configuration options
- [Agent-Files](Agent-Files) - Agent file configuration
- [CLI](CLI) - Standard CLI reference
- [Technical-Specs](Technical-Specs) - Implementation details
