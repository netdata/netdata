# Configuration Loading

## TL;DR
The CLI and headends assemble configuration by merging multiple `.ai-agent.json` layers (and matching `.ai-agent.env` files) via `discoverLayers()`/`buildUnifiedConfiguration()`, resolving placeholders per layer, and validating the merged result with schema/queue checks before constructing sessions. The older `loadConfiguration()` path (single JSON + env expansion) remains only for deterministic tests/back-compat.

## Source Files
- `src/config-resolver.ts` – Layer discovery, placeholder expansion, `buildUnifiedConfiguration`
- `src/agent-loader.ts` – Validations (`validateNoPlaceholders`, `validateQueueBindings`), persistence overrides, CLI merge logic
- `src/config.ts` – Legacy single-file loader (phase2 harness)
- `src/types.ts` – `Configuration`/provider/MCP schema definitions
- `src/subagent-registry.ts`, `src/cli.ts` – Consumers of the resolved configuration

## Layer Discovery & Priority
**Location**: `src/config-resolver.ts:44-140`

```typescript
const order: LayerOrigin[] = [
  '--config', // explicit --config path
  'cwd',      // process.cwd()
  'prompt',   // directory of the .ai file (if different from cwd)
  'binary',   // directory that contains the ai-agent executable
  'home',     // $HOME/.ai-agent/ai-agent.json
  'system',   // /etc/ai-agent/ai-agent.json
];
```

- Each origin contributes a JSON path (e.g., `/etc/ai-agent/ai-agent.json`) and an optional `.ai-agent.env` file in the same directory.
- `discoverLayers({ configPath, promptPath })` returns `ResolvedConfigLayer[]` preserving priority. Higher entries override lower ones when values collide.
- Every layer is optional: missing JSON/ENV files simply skip that origin.

## Per-Layer Placeholder Expansion & Errors
**Location**: `src/config-resolver.ts:18-215`

- **Env parsing**: `readEnvIfExists` supports `KEY=value`, `export KEY=value`, `#` comments, and quoted values. Parsed variables live only inside the originating layer.
- **Placeholder expansion**: `expandPlaceholders(raw, vars)` walks each JSON shape and replaces `${NAME}` using the layer's env map first, then `process.env`. Strings under `mcpServers.*.env`, `mcpServers.*.headers`, and REST tool payload templates are left untouched when they intentionally include `${parameters.foo}` tokens.
- **MCP_ROOT handling**: When `${MCP_ROOT}` resolves to blank, `resolveMCPServer` falls back to `process.cwd()` and emits a verbose notice so stdio servers always have a working directory (`src/config-resolver.ts:201-224`).
- **MissingVariableError**: Thrown when a placeholder cannot be resolved. It captures the scope (`provider|mcp|defaults|accounting`), identifier, layer origin, and variable name (`src/config-resolver.ts:18-41`). CLI surfaces the exact layer so operators know which `.ai-agent.env` file to fix.
- **Type inference**: Providers missing `type` inherit sensible defaults based on the provider name (e.g., `openai` → `type: 'openai'`), but a warning is logged until the JSON is updated (`src/config-resolver.ts:170-199`).

## Building the Unified Configuration
**Location**: `src/config-resolver.ts:383-452`

```typescript
const cfg = buildUnifiedConfiguration(
  { providers: needProviders, mcpServers: externalToolNames, restTools: externalToolNames },
  layers,
  { verbose }
);
```

- **On-demand resolution**: Only the provider/MCP/rest-tool names actually requested by the prompt/CLI are resolved. This prevents unrelated secrets from blocking execution.
- **Providers**: `resolveProvider` expands placeholders, applies fallback `type`, and deep-merges with `providerOverrides` when `--provider-override` is supplied (`src/agent-loader.ts:351-389`).
- **MCP servers**: `resolveMCPServer` expands placeholders, enforces `MCP_ROOT`, and leaves raw `env`/`headers` values untouched so secrets are passed to child processes without double expansion.
- **REST tools & OpenAPI specs**: `resolveRestTool`/inline OpenAPI merging keep `${parameters.foo}` verbatim by short-circuiting placeholders that start with `parameters.` (`src/config-resolver.ts:240-269`, `src/config-resolver.ts:412-423`).
- **Defaults/pricing/telemetry/accounting**: Helper resolvers (`resolveDefaults`, `resolvePricing`, `resolveTelemetry`, `resolveAccounting`) scan layers top-down and return the first entry containing the field (`src/config-resolver.ts:272-364`). Accounting files run through placeholder expansion with the same MissingVariable safeguards.
- **Queues**: `resolveQueues` normalizes every declared queue entry, enforcing positive integers and adding `default` automatically when absent (`src/config-resolver.ts:306-334`). When multiple layers define the same queue, the highest concurrency wins.
- **Return shape**: `buildUnifiedConfiguration` emits `{ providers, mcpServers, restTools, openapiSpecs, queues, defaults, accounting, pricing, telemetry, cache }`, matching `Configuration` in `src/types.ts`.

## Queue Defaults & Validation
- `DEFAULT_QUEUE_CONCURRENCY = min(64, max(1, availableParallelism*2))` (`src/config.ts:9-26`) and is injected when `queues.default` is missing in merged config (`src/config-resolver.ts:334-335`).
- `validateQueueBindings(config)` in `src/agent-loader.ts:305-343` verifies every MCP server, REST tool, and OpenAPI import references an existing queue name, preventing runtime deadlocks.
- Telemetry hooks `queueManager.setListeners` record queue depth/wait metrics for observability, so accurate queue definitions are mandatory.

## Placeholder Sanitization
- After building the config, `validateNoPlaceholders(config)` rejects any remaining `${VAR}` sequences in providers, MCP servers, or `accounting.file` to ensure secrets never leak into runtime objects (`src/agent-loader.ts:250-303`).
- CLI merges all layer env files plus `process.env` into a single `mergedEnv` so downstream prompt interpolation (`${FORMAT}`, `${VAR}` in frontmatter) sees the same values (`src/agent-loader.ts:390-409`).

## Legacy `loadConfiguration()` Path
**Location**: `src/config.ts:180-364`

- Still used by the deterministic phase2 harness and standalone library imports.
- Behavior: resolve a single `.ai-agent.json` (order: `--config` → `cwd` → `$HOME/.ai-agent.json`), expand `${VAR}` via `process.env`, normalize legacy MCP server shapes (`type: 'local'|'remote'`, array commands, `environment` alias), insert `queues.default`, and Zod-validate.
- MCP `env`/`headers` entries are explicitly skipped during expansion so secrets are resolved at spawn time, matching the new resolver’s behavior.

## Configuration Effects
| Setting | Impact |
|---------|--------|
| `providers.<name>` | Auth base URLs, reasoning defaults, tokenizer/context windows per provider |
| `mcpServers.<name>` | Transport, queue binding, env/header secrets, `shared` flag, restart health probe |
| `restTools` / `openapiSpecs` | REST/OpenAPI tool schemas, queues, and per-tool auth headers |
| `defaults` | Session-wide fallback for timeouts, retries, reasoning, output format |
| `queues` | Global concurrency envelopes consumed by MCP/REST tools |
| `pricing` | Optional cost tables used when providers don’t report billing |
| `telemetry` | Enables OTLP/Prometheus exporters plus logging formats |
| `persistence.sessionsDir` / `persistence.billingFile` | Snapshot and accounting sinks wired via `mergeCallbacksWithPersistence()` |
| `cache` | Global response cache backend (SQLite/Redis) used by agent/tool cache TTLs (defaults to SQLite when a TTL is enabled) |

**Duration parsing**:
- Timeouts and interval settings accept either raw milliseconds or duration strings (`N.Nu` where `u ∈ { ms, s, m, h, d, w, mo, y }`).
- Cache TTLs accept `off` \| `<ms>` \| `<N.Nu>` with the same units.

## Business Logic Coverage (Verified 2025-11-16)
- **Layer precedence**: Verified `discoverLayers()` order matches README (explicit → cwd → prompt → binary → home → system) and that CLI passes both `configPath` and `promptPath` when loading agents/headends (`src/cli.ts:127-220`, `src/agent-loader.ts:112-188`).
- **Scoped env overlays**: Confirmed placeholder evaluation always consults the originating layer’s env map before `process.env`, preventing unrelated `.ai-agent.env` files from leaking secrets across projects (`src/config-resolver.ts:88-175`).
- **Error transparency**: `MissingVariableError` propagates the human-readable scope/origin/id so operators know which file lacks a value; CLI surfaces the exact message without wrapping (`src/cli.ts:470-580`).
- **Queue guardrails**: `validateQueueBindings` blocks startup if a referenced queue is undefined, ensuring telemetry metrics (`ai_agent_queue_depth`, `ai_agent_queue_wait_duration_ms`) stay accurate and tool throttling behaves predictably (`src/agent-loader.ts:305-343`, `src/tools/queue-manager.ts:1-120`).
- **Legacy loader parity**: Phase 2 harness uses `loadConfiguration()` directly (`src/tests/phase2-harness-scenarios/phase2-runner.ts:5980-6320`), and the helper still normalizes legacy MCP types + applies `DEFAULT_QUEUE_CONCURRENCY`, so deterministic scenarios read the same schema as production.

## Troubleshooting
| Symptom | Likely Cause | Resolution |
|---------|--------------|------------|
| `MissingVariableError: provider 'openai' at home` | `${OPENAI_API_KEY}` missing in `$HOME/.ai-agent.env` or shell env | Add the key to the correct `.ai-agent.env` layer or export it before running |
| `Unknown queue 'playwright'` on startup | Queue referenced in MCP/REST tool without definition | Add `"queues": { "playwright": { "concurrent": N } }` to any Config layer |
| MCP stdio server starts in wrong directory | `${MCP_ROOT}` blank | Set `MCP_ROOT` explicitly per layer or rely on cwd (warning emitted with verbose flag) |
| Provider missing `type` warning | Legacy config omitted the field | Add `"type": "openai"` (or another allowed value) to the provider entry |
| Legacy JSON only partially applied | Running harness or direct `loadConfiguration` without `queues` block | Include `"queues": { "default": { "concurrent": N } }` or let the loader inject the CPU-based fallback |
