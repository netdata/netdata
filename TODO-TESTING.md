# Testing Roadmap

## Phase 1 — Deterministic Core Harness

**Goal**: stand up a fully deterministic end-to-end harness that exercises the agent loop (LLM turn orchestration + MCP tool execution + final report) without touching production code paths.

### Scope
- Implement a custom AI SDK provider (`test-llm`) that returns scripted responses based on the active scenario key (e.g., user prompt `run-test-1`).
  - Count turns from the incoming prompt (assistant message count + 1) and assert expected system/user content and tool availability per turn.
  - Emit tool calls, assistant text, and final `agent__final_report` messages exactly as the scenario specifies.
  - Surface deterministic token/cost usage in the returned `TurnResult` for accounting checks.
- Add provider support in the agent (`LLMClient`) for the new `test-llm` type, mirroring existing OpenAI integration but pointing to our scripted model.
- Build a toy MCP stdio server with the official MCP SDK exposing 2–3 deterministic tools.
  - Each tool response (success, error, timeout) is driven by the same scenario definition so LLM instructions can trigger specific behaviors.
  - Ensure initialization works through the normal MCP discovery path (tools/instructions) so no production code is bypassed.
- Author a baseline scenario (`run-test-1`) that covers the happy path:
  1. Turn 1: LLM requests the configured tools, issues tool calls.
  2. Agent executes real tool calls against the toy MCP server and records accounting.
  3. Turn 2: LLM produces the final report (assistant text + `agent__final_report`).
- Provide a lightweight runner that loads a dedicated `.ai-agent.json` pointing at `test-llm` + the toy MCP server, sends the scenario prompt, and asserts on the returned `AIAgentResult` (conversation, accounting, FIN logs).
- Document how to enable coverage using V8/c8 so we can measure per-file/module coverage once tests exist.

### Deliverables
- `src/llm-providers/test-llm.ts` (or equivalent) + configuration schema updates.
- Toy MCP server implementation and wiring in test harness.
- Scenario fixtures describing turns, tool expectations, outputs.
- A CLI or script that runs the scenario end-to-end and asserts the outcome.
- README snippet (or dedicated doc) explaining how to invoke the harness.
- `package.json` script (if needed) for running the deterministic test.

### Running Phase 1
- Build the project (`npm run build`).
- Execute the deterministic harness: `node dist/tests/phase1-harness.js` or `npm run test:phase1`.
- The harness executes three scripted scenarios:
  1. `run-test-1`: LLM success with MCP success.
  2. `run-test-2`: LLM success with MCP failure.
  3. `run-test-3`: Initial LLM failure followed by retry success.
- Enable coverage when needed: `NODE_V8_COVERAGE=coverage node dist/tests/phase1-harness.js` or wrap with `npx c8`.

### Acceptance Criteria
- Running the Phase 1 scenario exercises two turns (tool call turn + final report turn) and succeeds without touching production agent code.
- Deterministic assertions confirm system prompt enforcement, tool availability, tool execution order, final report emission, and accounting entries.
- Coverage tooling instructions exist (e.g., `c8 node --test` or `NODE_V8_COVERAGE=...`) so we can quantify future test reach.

### After Phase 1
Once Phase 1 is delivered, we will enumerate additional scenarios to drive coverage toward 100% across LLM error handling, tool failures, sub-agent recursion, accounting edge cases, etc.
