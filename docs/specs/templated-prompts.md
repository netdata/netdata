# Templated Prompts (LiquidJS)

## TL;DR
- All model-visible dynamic strings and schemas are rendered through Liquid templates.
- Base `.ai` system prompts are rendered once at load-time and stay static per agent.
- Per-session/per-turn ephemerals live in XML-NEXT and runtime templates.

## Goals
- Make every model-facing string editable via `.md` / `.json` Liquid templates.
- Keep outputs identical to current behavior.
- Keep base `.ai` prompts static to preserve LLM prefill caching.
- Allow load-time overrides (no behavior change).

## Non-goals
- No changes to tool or user-facing output formats.
- No changes to agent behavior beyond prompt plumbing.

## Implementation Summary (current)
- LiquidJS strict mode is enforced (unknown variables and missing includes fail).
- Includes are resolved relative to the file containing the directive.
- Dynamic include paths are rejected.
- Maximum include depth is 8; `.env` is blocked.
- Base `.ai` prompts are rendered at load-time with an empty context (no variables allowed).
- Runtime templates cover XML-NEXT, TURN-FAILED, internal tools, tool schemas, tool-output prompts, tool results, and router instructions.
- System nonce is static for the lifetime of the ai-agent process (per agent).
- Runtime system prompt is: base `.ai` + internal tools + MCP instructions (per session).
- Runtime hash warning tracks the last 5 unique hashes for internal + MCP sections.

### Code touchpoints
- Template engine: `src/prompts/template-engine.ts`
- Template registry: `src/prompts/templates.ts`
- Base `.ai` rendering: `src/agent-loader.ts` (renderPromptBodyLiquid)
- Runtime system prompt composition: `src/ai-agent.ts`, `src/agent-registry.ts`, `src/tools/internal-provider.ts`
- XML-NEXT: `src/llm-messages-xml-next.ts` + `src/prompts/xml-next.md`
- TURN-FAILED: `src/llm-messages-turn-failed.ts` + `src/prompts/turn-failed.md`
- Tool-output prompts: `src/tool-output/*` + `src/prompts/tool-output/*`
- Tool schemas: `src/tools/internal-provider.ts` + `src/prompts/tool-schemas/*.json`
- Tool results: `src/xml-tools.ts` + `src/prompts/tool-results/*`

## Template Inventory

### Core prompt templates
- `final-report.md`
- `internal-tools.md`
- `mandatory-rules.md`
- `task-status.md`
- `batch-with-progress.md`
- `batch-without-progress.md`
- `router-instructions.md`
- `xml-next.md`
- `xml-past.md`
- `turn-failed.md`
- `meta/*.md` (META blocks and snippets)

### Tool schema templates (`.json`)
- `tool-schemas/final-report.json`
- `tool-schemas/task-status.json`
- `tool-schemas/batch.json`
- `tool-schemas/router-handoff.json`
- `tool-schemas/tool-output.json`

### Tool-output templates
- `tool-output/instructions.md`
- `tool-output/handle-message.md`
- `tool-output/success-message.md`
- `tool-output/failure-message.md`
- `tool-output/no-output.md`
- `tool-output/truncated-message.md`
- `tool-output/error-invalid-params.md`
- `tool-output/error-handle-missing.md`
- `tool-output/error-read-grep-failed.md`
- `tool-output/error-read-grep-synthetic.md`
- `tool-output/warning-truncate.md`
- `tool-output/map-system.md`
- `tool-output/reduce-system.md`
- `tool-output/reduce-user.md`
- `tool-output/read-grep-system.md`
- `tool-output/read-grep-user.md`

### Tool-result templates
- `tool-results/unknown-tool.md`
- `tool-results/xml-wrapper-called-as-tool.md`
- `tool-results/final-report-json-required.md`
- `tool-results/final-report-slack-messages-missing.md`

## Variable Conventions
- Every Liquid template must start with a `{% comment %}` block listing all available variables.
- Variable names in templates are `snake_case`.
- Base `.ai` prompts are rendered with an empty context. Any variable use fails load.

### Runtime variable set (examples)
Use template header comments as the source of truth. Highlights:

- `xml-next.md`: `nonce`, `turn`, `max_turns`, `max_tools`, `attempt`, `max_retries`,
  `context_percent_used`, `expected_final_format`, `format_prompt_value`,
  `datetime`, `timestamp`, `day`, `timezone`, `has_external_tools`,
  `task_status_tool_enabled`, `forced_final_turn_reason`, `final_turn_tools`,
  `final_report_locked`, `consecutive_progress_only_turns`, `show_last_retry_reminder`,
  `allow_router_handoff`, `plugin_requirements`, `missing_meta_plugin_names`.

- `internal-tools.md`: `format_id`, `format_description`, `expected_json_schema`,
  `slack_schema`, `slack_mrkdwn_rules`, `plugin_requirements`, `nonce`,
  `max_tool_calls_per_turn`, `batch_enabled`, `progress_tool_enabled`,
  `has_external_tools`, `has_router_handoff`.

- Tool schema templates: `format_id`, `format_description`, `expected_json_schema`,
  `slack_schema`, `items_schema` (batch), and related schema inputs.

- Tool-output templates: tool name, args, response, chunk stats, and extraction hints
  (see each template header).

## Rendering Pipeline

### Load-time (per agent)
1. Read `.ai` file.
2. Resolve Liquid includes and render base prompt (empty context).
3. Store base prompt and a static nonce in the agent registry.

### Runtime (per session / per turn)
1. Render `internal-tools.md` using runtime context.
2. Append MCP instructions (runtime).
3. Concatenate: base `.ai` + internal tools + MCP instructions → total system prompt.
4. Compute runtime hash and warn only on new hashes (last 5 unique).
5. Render `xml-next.md` per turn.
6. Render `turn-failed.md` when needed.
7. Render tool schemas and tool-output prompts on demand.

## XML-NEXT and runtime markers
- Prompts should reference XML-NEXT markers (e.g., `XML-NEXT.DATETIME`, `XML-NEXT.FORMAT`) for runtime values.
- These markers are static text; values are delivered in the XML-NEXT block each turn.
