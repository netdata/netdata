# Testing Roadmap

## 2025-10-27 – Phase 2 Live Runs Only

### TL;DR
- Phase 2 fixture recording/replay has been removed; scenarios now hit real providers every time.
- Runner/LLM client/provider stack no longer accept fixture overrides, `--refresh` flags, or interceptors.
- CI and local workflows must budget for live model latency and cost; failures reflect real upstream behaviour.

### Analysis
- `src/tests/phase2-runner.ts` now always loads the master test agent with live providers (no `providerOverrides`, no `llmInterceptor`).
- `src/llm-client.ts` and `src/llm-providers/base.ts` dropped wire-recording hooks; `PHASE2_STREAM_TRACE` instrumentation was removed.
- Stored fixtures under `fixtures/phase2/**` have been deleted; docs updated to emphasise live-only operation.
- Risk area: upstream variability (reasoning enablement, tool parity, quota). Use targeted re-runs instead of relying on cached transcripts.

#### 2025-10-27 Test Status Snapshot
- Manual Tier 1 live runs for `vllm/default-model` (stream on/off, basic + multi-turn) pass end-to-end with correct sub-agent sequencing.
- Tier 2 matrices not yet re-run after cleanup; schedule a full sweep once credentials/cost approvals are confirmed.

### Decisions (Confirmed 2025-10-27)
- No fixture recording/replay will be reintroduced without explicit approval.
- `npm run test:phase2` is inherently live; failures must be triaged against real provider responses.
- Docs and onboarding now highlight credential setup and cost awareness instead of fixture refresh instructions.

## Remaining Backlog / Next Scenarios
- **Anthropic Adapter Coverage** *(pending)* — deterministic harness case for retry/error mapping without hitting the network.
- **OpenAI Adapter Coverage** *(pending)* — similar deterministic coverage for OpenAI-specific options/error normalization.
- **Google Adapter Coverage** *(pending)* — validate format filtering and retry handling.
- **REST/MCP Mixed Batch Paths** *(pending)* — deterministic coverage for mixed success/failure batches.
- **Coverage Snapshot & Gating** *(pending)* — capture baseline coverage before turning on gating rules.

## Phase 2 Coverage Objectives (Updated)
- Exercise `src/ai-agent.ts`, `src/llm-client.ts`, and `src/tools/internal-provider.ts` via deterministic harnesses where feasible.
- Stretch goal: surface provider-specific adapter branches (`openai`, `anthropic`, `google`, etc.) through scripted harness flows.

## Next Additions (Planned)
- **Agent orchestration** — deterministic checks for final-turn tool filtering, thinking stream toggles, persistence snapshots.
- **Internal provider advanced paths** — cover Slack metadata merging, REST fallback accounting, JSON schema validation failures.
- Continue running `npx c8 npm run test:phase1` after scenario updates to track coverage.

## Implementation Notes
- Scenario definitions live in `src/tests/fixtures/test-llm-scenarios.ts`; harness configuration hooks in `src/tests/phase1-harness.ts` manage overrides.
- MCP test server already exposes timeout/long-payload/failure scenarios for future cases.
- Phase 2 runs now require valid provider credentials and may incur cost; use environment gating to control frequency (e.g., Tier filters, model filters).
