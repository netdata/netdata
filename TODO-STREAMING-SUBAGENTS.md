TL;DR
- Verify sub-agent thinking/output streaming and headend filtering behavior across openai, anthropic, and CLI entrypoints.

Analysis
- Sub-agent callbacks forward thinking/output with per-child metadata (agentId=child call path, callPath, sessionId, parentId, originId) in `src/subagent-registry.ts:245-273` and `src/subagent-registry.ts:313-333`, ensuring parent callbacks receive every child stream chunk.
- Turn runner emits streaming chunks for both content and reasoning for all sessions; callback meta for the master session is set in `src/session-turn-runner.ts:2540-2550`, and child sessions keep their own meta, so headends see all agents’ events.
- Headends:
  - OpenAI compatibility filters thinking/output to the master agent by checking `meta.agentId === agent.id` before emitting SSE (`src/headends/openai-completions-headend.ts:675-714`).
  - Anthropic compatibility applies the same filter (`src/headends/anthropic-completions-headend.ts:685-716`).
  - CLI callbacks ignore meta entirely; they log/output all thinking/output chunks they receive (`src/cli.ts:2052-2072`), so CLI currently displays sub-agent streams when verbose/TTY and cannot filter to master only.
- Master final report streaming suppression for sub-agents exists (`src/session-turn-runner.ts:1968-1987`), but streaming chunks still flow for sub-agents, relying on headend filtering.

Decisions
- Costa chose: filter CLI thinking/output to master agent only (option 2).

Plan
- Present findings to Costa: confirm openai/anthropic comply; highlight CLI mismatch and propose options.
- Implement CLI meta-aware filtering and add regression coverage if time permits.

Implied decisions
- Streaming infrastructure already propagates all agent events; only presentation layer (headends) decides what to surface. No code changes planned until Costa approves.

Testing requirements
- If changes are requested: update/add Phase 1 harness coverage to assert headend filtering behavior; rerun `npm run lint` and `npm run build`.

Documentation updates required
- If behavior changes, update `README.md`/`docs/IMPLEMENTATION.md` to note CLI filtering rules.
