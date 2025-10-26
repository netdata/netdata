# Phase 2 Integration Tests

## Goal
- Prove that the ai-agent framework works end-to-end against real LLM providers (single-turn and multi-turn, streaming and non-streaming) while keeping runs affordable.
- Capture the Vercel AI SDK request/response payloads per turn so future runs can replay deterministically without re-contacting live APIs.
- Maintain a zero-tolerance pass bar: every scenario must succeed; failures surface framework regressions rather than model behaviour.

## Current Status (October 26, 2025)
- `src/tests/phase2-runner.ts` drives two scenarios (`basic-llm`, `multi-turn`) for each configured model with `stream=false` and `stream=true` variants.
- `src/tests/phase2-fixtures.ts` provides `Phase2FixtureManager`, an `LLMInterceptor` that records SDK-level turn payloads under `fixtures/phase2/<provider>/<model>/<scenario>/<stream-{on|off}>/turn-XXXX.json` and replays them when present.
- Fixtures are generated automatically when missing and the run succeeds; `--refresh-llm-fixtures` forces regeneration.
- Model ordering and tiering live in `src/tests/phase2-models.ts`. Current list (cheapest → most expensive tier) matches the agreed set: vllm/default-model, ollama/gpt-oss:20b, anthropic/claude-3-haiku-20240307, openrouter/x-ai/grok-code-fast-1, openai/gpt-5-mini, google/gemini-2.5-flash, anthropic/claude-haiku-4-5, openai/gpt-5, google/gemini-2.5-pro, anthropic/claude-sonnet-4-5.
- Runner defaults to completing the full matrix even after failures; set `PHASE2_STOP_ON_FAILURE=1` for the cost-control behaviour we used during development.
- Optional tracing/logging toggles are wired (`PHASE2_TRACE_LLM`, `PHASE2_TRACE_SDK`, `PHASE2_TRACE_MCP`, `PHASE2_VERBOSE`).
- Recorded fixtures exist for `vllm/default-model` (basic stream on/off, multi stream-off so far).
- Structured logging now depends on `systemd-cat-native` when journald is available; runs without the helper automatically fall back to logfmt (no native module rebuilds required).

## Scenario Matrix (per model)
### Test 1 – Basic LLM Call
- System prompt: `Call agent__final_report with status="success", report_format="text", and content set to the required text. The content must be exactly "test".`
- User prompt: `Invoke agent__final_report(status="success", report_format="text", content="test") and produce no other output.`
- Reasoning: unset (defaults).
- Runs: stream-off, stream-on.
- Pass criteria: session status `success`, agent calls `agent__final_report`, accounting entry emitted. Note: the current tool schema forces `report_format="markdown"`; prompts still mention "text" and the model reconciles this automatically.

### Test 2 – Multi-Turn Reasoning & Sub-Agents
- System prompt: `1) Call agent__test-agent1({"prompt":"run","reason":"execute phase two","format":"sub-agent"}). 2) Wait for the response and call agent__test-agent2({"prompt":<agent1 output>,"reason":"execute phase two","format":"sub-agent"}). 3) Finish by calling agent__final_report(status="success", report_format="text", content=<agent2 output>). Do not emit plain text.`
- User prompt: `Execute agent__test-agent1({"prompt":"run","reason":"execute phase two","format":"sub-agent"}), pass its output into agent__test-agent2({"prompt":<agent1 output>,"reason":"execute phase two","format":"sub-agent"}), and conclude with agent__final_report(status="success", report_format="text", content=<agent2 output>).`
- Reasoning: `high`.
- Runs: stream-off, stream-on.
- Pass criteria: session status `success`; ≥3 turns; sub-agent tools invoked in order; `agent__final_report` called; accounting captured; for Anthropic, reasoning signatures survive across turns when emitted.

## Fixture Workflow
- Live call performed only when a matching fixture is missing (or `--refresh-llm-fixtures` is supplied). Successful runs persist each turn to disk; failures do not overwrite existing fixtures.
- During replay, requests are hashed to detect drift; mismatches trigger a clear error instructing re-recording.
- `Phase2FixtureManager.stats()` reports recorded vs replayed turn counts so the runner can surface when new data was captured.

## Remaining Work / Follow-Ups
- Record fixtures for all scenarios (stream-on multi-turn for vllm, remaining models) once stable.
- Investigate replay gaps noted during early trials (e.g., validator complaining about missing final-report tool logs) and ensure recorded fixtures satisfy the validator without manual log injection.
- Align prompts with the `agent__final_report` schema (text vs markdown) to remove unnecessary model reconciliation.
- Extend documentation (`docs/TESTING.md`, README) with Phase 2 instructions, including fixture refresh workflow and cost expectations.
- Consider lightweight unit coverage for `Phase2FixtureManager` (hash mismatches, refresh flow) to catch regressions without running costly integration tests.
