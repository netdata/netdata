# Override Keys

Runtime configuration overrides via `--override`.

---

## Overview

The `--override` flag allows runtime configuration changes that bypass normal config loading.

**Syntax**:
```bash
ai-agent --agent test.ai --override key=value "query"
```

---

## Available Override Keys

### no-batch

**Purpose**: Disable `agent__batch` parallel tool execution.

**Usage**:
```bash
ai-agent --agent test.ai --override no-batch=true "query"
```

**Effect**: Tools are executed sequentially instead of in parallel batches.

**Use Case**: Debugging tool execution order or race conditions.

---

### no-progress

**Purpose**: Disable progress-only `task_status` tool behavior.

**Usage**:
```bash
ai-agent --agent test.ai --override no-progress=true "query"
```

**Effect**: `task_status` tool may trigger final turn enforcement even in progress mode.

**Use Case**: Testing final turn behavior.

---

### contextWindow

**Purpose**: Override context window size for all providers.

**Usage**:
```bash
ai-agent --agent test.ai --override contextWindow=65536 "query"
```

**Effect**: Sets a custom context window regardless of provider/model defaults.

**Use Case**: Testing context guard behavior or working with custom models.

---

### interleaved

**Purpose**: Control interleaved reasoning injection.

**Usage**:
```bash
# Disable interleaved reasoning
ai-agent --agent test.ai --override interleaved=false "query"

# Enable with default field name
ai-agent --agent test.ai --override interleaved=true "query"

# Enable with custom field name
ai-agent --agent test.ai --override interleaved=reasoning_content "query"
```

**Effect**: Controls whether reasoning content is injected into assistant messages for OpenAI-compatible providers.

**Use Case**: Testing reasoning behavior or compatibility with specific providers.

---

### reasoning

**Purpose**: Override reasoning level for all agents.

**Usage**:
```bash
ai-agent --agent test.ai --override reasoning=high "query"
```

**Values**:
- `none` / `unset` - Disable reasoning
- `default` - Use configured defaults
- `minimal` - Minimal effort (~1024 tokens)
- `low` - 30% of max output tokens
- `medium` - 60% of max output tokens
- `high` - 80% to max output tokens

**Effect**: Stomps reasoning on every agent/sub-agent (ignores frontmatter).

**Note**: Use `--default-reasoning` instead if you only want to set fallback values.

---

## Multiple Overrides

Pass multiple overrides:
```bash
ai-agent --agent test.ai \
  --override no-batch=true \
  --override contextWindow=32768 \
  "query"
```

---

## Priority

Override keys have highest priority:
1. `--override` (highest)
2. CLI flags
3. Frontmatter
4. Config file
5. System defaults (lowest)

---

## Comparison: --override vs --default-*

| Mechanism | Behavior |
|-----------|----------|
| `--override key=value` | Forces value, ignores all other config |
| `--default-reasoning X` | Sets fallback when frontmatter omits the key |
| `defaults.X` in config | Sets fallback from config file |

Use `--override` for testing/debugging.
Use `--default-*` for operational defaults.

---

## Discovery

Override keys are processed in:
- `src/cli.ts` - CLI parsing
- `src/ai-agent.ts` - Session configuration

Check these files for the current list of supported keys.

---

## Stability

Override keys are:
- **Internal**: Primarily for development/testing
- **Undocumented**: Not part of stable API
- **May Change**: Without deprecation warnings

---

## See Also

- [Configuration](Configuration) - Standard configuration
- [Configuration-Parameters](Configuration-Parameters) - CLI parameters

