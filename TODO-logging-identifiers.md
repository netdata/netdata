# TL;DR
- Tool provider identifiers renamed to `namespace`; remote identifiers now follow `protocol:namespace:tool` so console/journald can reconstruct names like `brave:brave_web_search`.
- Console/journald formatter enriches tool/LLM logs from structured metadata; we still emit the legacy message text for logfmt/json during the transition.
- Removed the automatic session-label prefix in `AIAgentSession.addLog`, eliminating duplicated `web-search:` strings in enriched sinks while preserving a fallback for empty messages.
- Hierarchical turn numbering is back (`turnPath` like `1.1-0.2`) and `agentPath`/`callPath` now describe agent chains without the legacy `child:` prefix; batch tools emit unique subturns for each queued request.

# Analysis
- Reviewed `docs/LOGS.md`, `docs/DESIGN.md`, and the current logging implementation to confirm remote identifier expectations (`protocol:namespace:tool`).
- `AIAgentSession.addLog` now records `agentPath`, updates `callPath` for tool messages, and emits a derived `turnPath` so sinks can render hierarchical identifiers without mutating the raw message text.
- Sub-agent sessions inherit both the numeric turn prefix and the colon-separated agent path; console/journald render them via `buildRichLogLine`, while logfmt/journald/OTLP receive `agent_path` and `turn_path` labels.
- Remote identifiers no longer receive the synthetic `child:` prefix—the hierarchy is expressed via `agentPath`/`callPath`—so MCP/rest tooling keeps its native identifiers.
- Batch execution assigns deterministic subturns (parent-subturn + index) and forwards that context via `parentContext`, fixing the flat `turn.0` output for inner tool calls.

# Decisions
1. Provider identifiers renamed to namespaces; remote identifiers use `protocol:namespace:tool` everywhere structured logs are created.
2. Source auto-prefix removed; enriched sinks now own the responsibility to show agent context using structured metadata.
3. Continue surfacing namespace/tool via labels so telemetry/OTLP consumers can pivot without parsing free-form text.
4. Hierarchical identifiers must be restored per Costa’s guidance: structure every log as `<severity> <turn>.<subturn> <direction> <kind> <agentPath>`, where `agentPath` is `agent1:agent2:...`. `callPath` should append the tool name (`agent1:agent2:agent3:tool`). No `child:` or `agent__` wrappers in the rendered line.
5. Batch execution must allocate unique subturns to each queued tool so they appear as `turn.subturn`, `turn.subturn+1`, etc., within the parent turn.

# Plan
1. Monitor downstream telemetry/OTLP dashboards for the new `agent_path`/`turn_path` labels and adjust visualisations if needed.
2. Audit documentation (`docs/LOGS.md`, `docs/AI-AGENT-INTERNAL-API.md`, CLI help) to ensure they explain the hierarchical turn/agent paths.
3. Add deterministic coverage that captures a nested-agent run and a batch tool invocation to prevent regressions.

# Follow-ups
1. Verify OTLP dashboards/alerts ingest `tool_namespace` without schema drift.
2. Confirm once sources emit simplified messages (e.g., `started`) that logfmt/json sinks still meet documentation expectations.
3. Consider regression coverage that exercises the rich formatter to prevent reintroducing duplicated prefixes.
4. After hierarchical numbering fix, add deterministic tests ensuring sub-agent and batch logs present unique prefixes.

# Implied Decisions / Edge Cases
- Trace log identifiers now emit `trace:${protocol}:${namespace}`; fallback remains the namespace reported by the provider when unknown.
- Tools lacking a namespace delimiter fall back to `protocol:namespace:tool`, where `namespace` is the provider namespace string.
- Structured logging still populates `provider` with `protocol:namespace` to preserve dashboards that slice by provider.

# Testing Requirements
- `npm run lint`
- `npm run build`
- `./test-hierarchical-logging.sh` (or equivalent updated script) to capture new console output for manual verification.

# Documentation Updates
- Ensure `docs/DESIGN.md`, `docs/AI-AGENT-INTERNAL-API.md`, `docs/LOGS.md`, and `src/types.ts` all reference the `protocol:namespace:tool` shape and `tool_namespace` terminology. Remaining doc updates tracked here until confirmed complete.
