# Reasoning Controls

This agent exposes a provider-neutral reasoning level that can be driven from
frontmatter, CLI flags, overrides, or configuration. The internal scale uses
four symbolic levels:

- `minimal`
- `low`
- `medium`
- `high`

When a reasoning level is selected, the agent looks up the appropriate payload
per provider/model before each turn. The lookup happens in the following
order:

1. Model-specific mapping in `.ai-agent.json` (`providers.<name>.models.<id>.reasoning`)
2. Provider-wide mapping in `.ai-agent.json` (`providers.<name>.reasoning`)
3. Built-in defaults for the provider (if any)

Each mapping entry can be:

- `null`: disable reasoning for that scope (models can still override)
- A single string/number: always send that value regardless of level
- An array with four strings/numbers: positional mapping for
  `[minimal, low, medium, high]`

### Example configuration

```json
{
  "providers": {
    "openai": {
      "reasoning": ["minimal", "low", "medium", "high"]
    },
    "anthropic": {
      "reasoning": [1024, 8000, 16000, 32000],
      "reasoningAutoStreamLevel": "medium",
      "models": {
        "claude-3-opus-20250514": {
          "reasoning": null
        }
      }
    },
    "google": {
      "models": {
        "gemini-2.5-pro": {
          "reasoning": [1024, 4096, 8192, 16384]
        }
      }
    }
  }
}
```

- The `openai` provider uses the provided four-level string mapping for every
  model.
- The `anthropic` provider defaults to the numeric mapping, automatically
  enabling streaming whenever `medium` or `high` is selected. The Opus model
  is explicitly opted out (`null`).
- `google` gets a model-specific numeric mapping while leaving other Gemini
  models on their defaults.

### Selecting a reasoning level

- Frontmatter: `reasoning: minimal | low | medium | high`
- CLI flags: `--reasoning minimal` (auto-completes through Commander help)
- Overrides: `--override reasoning=high`

Leaving the option unset preserves provider defaults (no reasoning payload is
sent). Setting a level that resolves to `null` disables reasoning for the turn.

### Auto-enabling Anthropic streaming

Anthropic models require streaming when large reasoning budgets are used. Set
`reasoningAutoStreamLevel` on the provider entry to state the minimum level
that should force streaming. When triggered, the agent flips the request to
streaming even if the session was configured otherwise.

### Provider defaults

If no configuration is supplied, the agent falls back to inline defaults:

- **OpenAI/OpenRouter** – effort labels are forwarded directly, relying on the
  provider to translate them.
- **Anthropic/Google** – numeric budgets are derived dynamically from the
  effective `maxOutputTokens` using the following ratios:
  - `minimal`: 1,024 tokens (or the maximum available when the cap is lower)
  - `low`: 20% of `maxOutputTokens`
  - `medium`: 50% of `maxOutputTokens`
  - `high`: 80% of `maxOutputTokens`

All computed budgets are clamped to the provider’s documented bounds (e.g.
Anthropic 1,024–128,000, Google 1,024–32,768) and never exceed the turn’s
`maxOutputTokens`.

Providers not listed above ignore the reasoning level unless a mapping is
provided in configuration.

### Anthropic caching toggle

Anthropic caching can be controlled with the parallel `caching` option:

- Frontmatter: `caching: full | none`
- CLI: `--caching full` (or `none`)
- Overrides: `--override caching=none`

`full` retains the existing ephemeral cache control on the final message,
while `none` removes cache directives entirely.
