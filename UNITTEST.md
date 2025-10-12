# Unittest Framework Plan

Owner: ai-agent core
Status: Design approved (implementation pending)
Scope: End-to-end, deterministic tests for multi-turn, multi-tool, multi-agent sessions using AI SDK v2

## Goals

- Validate end-to-end behavior through the real AI SDK v2 integration (tool calls, message conversion, streaming, usage), not by bypassing it.
- Exercise ToolsOrchestrator, providers (MCP/REST/Internal), sub-agent sessions, and the unified opTree (logs, request/response, accounting, reasoning, child sessions).
- Run offline with deterministic outputs, zero network, zero secrets, zero flakiness.
- Maintain strict TypeScript + ESLint rules (0 warnings, 0 errors).

## Non-Goals

- No coverage of MCP websocket/http/sse transports (stdio is sufficient to validate our business logic).
- No golden text snapshots with timestamps; assertion helpers will focus on structure, ordering, labels, and counts.

## Deliverables (Checklist)

1) AI SDK v2 unittest provider
- Provider id: `ai-sdk-unittest`
- Location: `src/llm-providers/unittest.ts`
- Behavior:
  - Consumes a test scenario plan and emits deterministic assistant outputs:
    - Turn N: tool calls (correct AI SDK content parts for tool-call events)
    - Turn M: final content (stream and non-stream variants)
    - Optional reasoning deltas to exercise THK streaming and `opTree.reasoning`
  - Reports `usage` (tokens) and deterministic latency to feed accounting

2) Fake tools per transport
- `internal-unittest` (in-memory):
  - Implementation: test-only `TestToolProvider` extending `ToolProvider`
  - Validates args via Ajv; returns deterministic JSON or text; optional delay for latency
- `rest-unittest` (REST):
  - Uses `RestProvider`; tests monkey-patch `globalThis.fetch` to return canned `Response` objects (OK/error/stream)
  - Validates URL templating, headers/args substitution, streaming JSON-lines handling, timeouts, and redaction paths
- `mcp-stdio-unittest` (MCP stdio):
  - Small Node child process that speaks MCP protocol:
    - `listTools` returns `unittest.echo` with a trivial schema
    - `callTool` responds deterministically based on args
    - Emits stderr lines for trace logging capture
- Note: We will NOT implement `mcp-ws-unittest`, `mcp-http-unittest`, or `mcp-sse-unittest` — stdio is sufficient for business logic

3) Scenario DSL + assets
- Test-only scenario schema describing per-turn behavior:
  - `providers`: [{ provider: 'ai-sdk-unittest', model: 'unit' }]
  - `plan`: array of turns (toolCalls[] or final text), optional reasoning stream, optional error nodes
  - `tools`: handler map for `internal-unittest`; fetch stubs for `rest-unittest`; MCP stdio echo tool
  - `subagents`: mapping of tool → child .ai file and a child plan (reuses `ai-sdk-unittest`)
- Assets: tiny `.ai` prompts in `src/tests/assets/` for child agents

4) Assertion helpers
- Location: `src/tests/utils/expect-op-tree.ts`
- Capabilities:
  - `expectOpTree(tree).hasTurns(n)`
  - `op(turn, idx).isKind('llm'|'tool'|'session').hasStatus('ok'|'failed')`
  - `opHasRequest(payloadLike, capped=true)`, `opHasResponse(payloadLike)`
  - `opHasLogs({ severity, direction, type, pathPresent: true })`
  - `opHasAccounting({ type: 'llm'|'tool', latencyMin, tokensLike })`
  - Child session assertions: `.hasChildSession().childHasTurns(m)`
  - Totals verification using status aggregator semantics where appropriate

5) Test harness + runner
- Directory structure:
  - `src/tests/fakes/fake-llm.ts` (LanguageModelV2 impl driving the scenario plan)
  - `src/tests/fakes/test-tool-provider.ts` (internal-unittest)
  - `src/tests/assets/*.ai` (sub-agent prompts)
  - `src/tests/utils/expect-op-tree.ts` (helpers)
  - `src/tests/run-tests.ts` (orchestrates one or more scenarios and assertions)
- NPM script: add `"test": "npm run build && node dist/tests/run-tests.js"`

6) Minimal production changes only
- Add new provider adapter file: `src/llm-providers/unittest.ts`
- Add switch case in `LLMClient.createProvider()` to support type `'ai-sdk-unittest'`
- No other production code changes needed

## Architecture & Flow

- Tests construct a `Configuration` whose providers include `{ type: 'ai-sdk-unittest' }` targets and select test tools.
- `AIAgentSession` + `ToolsOrchestrator` run as in production:
  - LLM (via `ai-sdk-unittest`) emits tool calls/final content per plan
  - Tools execute via providers:
    - internal-unittest: in-memory `TestToolProvider`
    - rest-unittest: `RestProvider` with fetch stub
    - mcp-stdio-unittest: MCP stdio child
  - Sub-agent: parent calls tool `agent__child` which loads a tiny `.ai` prompt; child session is driven by the same unittest provider and attaches its opTree under the parent ‘session’ op
- Assertions verify the opTree shape, logs, path labels, request/response caps, accounting, and totals

## Scenario DSL (example)

```ts
const scenario = {
  providers: [{ provider: 'ai-sdk-unittest', model: 'unit' }],
  tools: ['internal-unittest', 'rest-unittest', 'mcp-stdio-unittest'],
  plan: [
    { turn: 1, toolCalls: [
      { name: 'internal-unittest.echo', parameters: { text: 'hello' } },
      { name: 'rest-unittest.search', parameters: { query: 'abc', page: 1 } },
    ], reasoning: ["Thinking step 1", "Thinking step 2"] },
    { turn: 2, final: 'All done.' },
  ],
  subagents: {
    'agent__child': {
      file: 'src/tests/assets/child.ai',
      plan: [
        { turn: 1, toolCalls: [ { name: 'internal-unittest.child', parameters: { n: 3 } } ] },
        { turn: 2, final: 'Child done.' },
      ]
    }
  }
};
```

## Assertions (example)

- Turn 1 contains:
  - One LLM op: request summary present, logs include `[txn:…] [path:1]`, reasoning chunks recorded
  - Two tool ops (paths `1.1`, `1.2`): request/response present, accounting with non-zero latency, tool kind captured
- Turn 2 contains:
  - One LLM op with final content; assistant text captured in `TurnNode.attributes.assistant.content`
- Sub-agent:
  - Parent op with `kind:'session'` and `childSession.turns.length === 2`
- Totals: `toolsRun === 3` (two parent tools + one child tool), tokens counts reported

## MCP stdio unittest tool

- CLI program under `src/tests/fakes/mcp-stdio-unittest.js` started by MCPProvider (stdio):
  - Speaks MCP protocol: `initialize`, `tools/list`, `tools/call`
  - `tools/list` → 1 tool `unittest.echo` with simple JSON schema `{ text: string }`
  - `tools/call` → returns `{ echoed: text, ts: <fixed or seeded time> }`
  - Emits stderr lines (e.g., `stderr: unittest ready`) to exercise MCP trace-capture

## REST unittest via fetch stub

- Tests monkey-patch `globalThis.fetch` to handle URLs under `https://unittest.local/...` and return canned responses:
  - JSON OK (200), JSON error (4xx/5xx), streaming JSON-lines
  - Support timeouts/abort for exercising timeout paths

## Sub-agent cycle prevention

- Covered by existing ancestry threading:
  - Parent passes `ancestors` → child → nested children; cycles are rejected on load
  - Tests include a cycle attempt to assert a clear error

## Running tests

- Add NPM script:
  - `"test": "npm run build && node dist/tests/run-tests.js"`
- Tests must run offline with zero network and pass ESLint/TS checks

## Risks & Mitigations

- Flakiness due to timing: Assertions avoid timestamps, focus on ordering/labels/sizes
- API drift in AI SDK: Unittest provider implementation isolated in `src/llm-providers/unittest.ts`; adjust if SDK surface changes
- MCP stdio differences: The stdio flow shares business logic with other transports, so it’s representative for provider-level validation

## Phase Plan

1) Provider adapter
- Add `src/llm-providers/unittest.ts`
- Switch case in `LLMClient` for `ai-sdk-unittest`

2) Internal + REST unittest tools
- Implement `TestToolProvider` (internal-unittest)
- Add fetch stub harness for rest-unittest

3) MCP stdio unittest tool
- Implement `src/tests/fakes/mcp-stdio-unittest.js`

4) Scenarios + assertions
- Implement one happy-path (multi-turn + sub-agent) scenario with assertions
- Add error-path scenarios (LLM timeout/error, tool timeout)

5) Runner + script
- Add `src/tests/run-tests.ts`
- Add NPM script `test`

## Acceptance Criteria

- `npm run lint` → 0 warnings/errors
- `npm run build` → success
- `npm run test` → all scenarios pass
- opTree assertions confirm:
  - LLM/tool/session ops structure, stable path labels, logs include `[txn:origin] [path:…]`
  - request/response normalization present with caps
  - accounting entries for LLM/tool operations
  - child session attachment for sub-agents

---

This document is the single source of truth for the unittest framework deliverables. Any deviations or additions should be reflected here before implementation.
