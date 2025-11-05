# Deterministic Sub-Agent Chaining Design

## TL;DR
- Goal remains to support deterministic sub-agent chaining; current code still leaves orchestration entirely to the parent LLM.
- `SubAgentRegistry` and frontmatter parsing lack any `next` metadata or chain runner logic (`src/subagent-registry.ts`, `src/frontmatter.ts`).
- Need design confirmations (e.g., mandatory chaining semantics, injected `reason` text, error propagation) before implementing.

## Analysis
- **Registry model**: `PreloadedSubAgent` / `ChildInfo` contain no `next` or downstream metadata, and `SubAgentRegistry` has no resolution pass (`src/subagent-registry.ts:20-125`).
- **Execution flow**: `execute()` only validates inputs, runs the child, and returns its output; there is no chained follow-up call (`src/subagent-registry.ts:126-265`).
- **Prompt metadata**: `parseFrontmatter` still rejects unknown top-level keys; `next` is neither allowed nor normalized today (`src/frontmatter.ts:16-118`).
- **Reason enforcement**: Registry continues to require a `reason` field for every invocation, meaning any chaining layer must decide what fixed reason text to supply (`src/subagent-registry.ts:60-122`).
- **Result packaging**: Child runs now preserve accounting/opTree metadata for observability; chaining logic must propagate these structures without breaking existing aggregation (`src/subagent-registry.ts:229-264`).

## Decisions Needed (awaiting Costa)
1. Confirm whether Phase 1 should still hard-chain only the first `next` entry (no branching/optional hops).
2. Approve injecting a fixed `reason` string for chained executions vs. forwarding the upstream reason.
3. Decide how to surface chain failures to the parent: immediate error vs. wrapped diagnostic payload.
4. Clarify whether chained agents should always honor the downstream `expectedOutput` format, even when the upstream agent declared a different default.

## Plan (pending decisions)
1. Extend frontmatter parsing/validation to accept `next` (string|string[]) and store it in the loaded agent metadata.
2. Enhance `SubAgentRegistry` preload to keep `rawNext`/`resolvedNext` data, perform resolution + cycle checks, and capture downstream IO schemas.
3. Add a chain runner inside `execute()` that aligns upstream output with downstream input, injects the approved reason, executes the next stage, and aggregates accounting/opTree metadata.
4. Update instrumentation (logs, accounting extras, session tree) and add deterministic harness scenarios covering successful chain, schema mismatch, and downstream failure.

## Implied Decisions
- Chaining stays opt-in via frontmatter; agents without `next` remain unchanged.
- Cycle detection must reuse existing ancestor protections to avoid recursion when agents re-import each other.
- Any automatic JSON parsing implies strict schema validation with actionable Ajv error messages for operators.

## Testing Requirements
- `npm run lint`, `npm run build`.
- Phase 1 harness cases exercising chained success, invalid payload, and downstream failure paths.
- Manual smoke via `./run.sh` with chained agents once implemented.

## Documentation Updates Required
- Update README and DESIGN once deterministic chaining lands.
- Append frontmatter template guidance for `next` (and caveats about mandatory execution) to CLI help/docs.
- Mention deterministic chaining behavior in SPECS whenever the implementation is ready.

---

## Legacy Phase 1 Draft (for reference)

### Background
- Sub-agents are currently exposed as tools via `SubAgentRegistry` and executed through `SubAgentRegistry.execute` (`src/subagent-registry.ts:240`).
- Frontmatter parsing disallows unknown keys (`src/frontmatter.ts:54-103`), so new metadata (e.g., `next`) must be registered explicitly.
- Tool execution contracts: `getTools()` adds a required `reason` field for JSON payloads and wraps text inputs into `{ input, reason }` (`src/subagent-registry.ts:118-177`).
- Session execution captures the child result, conversation, and accounting before returning to the parent LLM (`src/subagent-registry.ts:312-338`).
- There is no native facility for deterministic chaining; the LLM currently orchestrates additional sub-agent calls.

## Goals (Phase 1)
1. Support linear chaining with a single `next` stage (B → C → …) where the downstream agent runs automatically.
2. Ensure the downstream agent receives the upstream output directly, without re-entering the parent LLM loop.
3. Align upstream output contracts with downstream input contracts to avoid schema drift.
4. Enforce cycle prevention and fail fast when configuration is invalid.

## Non-Goals (Phase 1)
- No branching or conditional routing (handled in later phases).
- No multi-stage fan-out; only the first `next` item is honored.
- No runtime toggles to bypass chaining (chained agents always chain).
- No modifications to MCP or REST tool registries beyond the chain logic.

## Terminology
- **Stage**: a sub-agent involved in a chain.
- **Root stage**: the agent invoked by the parent (B in A→B→C).
- **Downstream stage**: the immediate `next` agent (C in B→C).
- **Chain runner**: the internal helper that executes the cascade within `SubAgentRegistry`.

## Metadata Specification
- **Frontmatter key**: `next`
  - Type: string or array of strings.
  - Presence is mandatory: once declared, the agent will always invoke the downstream stage (no override hooks).
  - Each value may be
    - the child tool name (with or without `agent__` prefix), or
    - a prompt path / alias already resolvable by the agent registry.
  - Phase 1 only uses the first resolved entry.
- Parser updates
  - Extend `parseFrontmatter` to accept `next` (string | string[]).
  - Normalize into `string[]` early for future expansion.
- Storage
  - Extend `ChildInfo` to include `rawNext: string[]` and `resolvedNext?: ChainLink`.
  - `ChainLink` holds the canonical tool name, prompt path, and input schema snapshot of the downstream stage.

## Load-Time Resolution
1. During `SubAgentRegistry.load(paths)`
   - Parse `next` into `rawNext` while loading each child.
   - Skip resolution until all children are loaded (to allow forward references).
2. Add a second pass (e.g., `resolveChains()`)
   - Iterate over children with `rawNext.length > 0`.
   - Resolve reference order:
     1. Exact child tool name (with/without prefix).
     2. Canonical prompt path.
     3. Registered alias (if available; see `src/agent-registry.ts:144-187`).
   - If unresolved, throw a descriptive error.
3. Cycle detection
   - Maintain a DFS stack of tool names.
   - If any `next` resolves to an ancestor in the stack, throw an error (`recursion detected in chain B → … → B`).
   - This supplements existing ancestor checks when loading nested sessions.
4. Input/Output contract extraction
   - Fetch downstream `ChildInfo.inputFormat` and `inputSchema`.
   - Capture downstream `LoadedAgent.expectedOutput?.schema` for validation or reporting.
   - Store these in `resolvedNext`.

## Execution Flow (SubAgentRegistry.execute)
1. **Pre-run alignment**
   - Lookup `child = this.children.get(name)`.
   - If `child.resolvedNext` exists:
     - Store original `expectedOutput` metadata (for analytics, if needed).
     - Derive the downstream input format/schema from `child.resolvedNext`.
     - Build an `OutputExpectation` object describing the format the current stage must yield.
     - Communicate the expectation to the session by overriding the `outputFormat` passed into `loaded.run`:
     - JSON → force `outputFormat` to `'json'` and keep schema for validation.
       - Text → force `'pipe'` (`src/subagent-registry.ts:292-309` currently maps `'text'` to `'pipe'`).
       - Markdown → `'markdown'`.
     - Optionally attach metadata to `child` so post-run logic knows the expectation applied.
2. **Stage execution**
   - Run the stage as today (`loaded.run(...)`).
   - Capture `result`, `conversation`, `accounting`, `trace`, `opTree` (existing behavior).
3. **Post-run chain dispatch**
   - If `child.resolvedNext` is undefined, return current payload.
   - Otherwise, invoke `runChain(nextLink, resultPayload, parentSession, opts)`.
4. **runChain helper**
   - Accepts `nextLink`, the upstream `result` string, and the parent session context.
   - Build downstream arguments based on `nextLink.inputFormat`:
     - JSON: attempt `JSON.parse(result)`; on failure, throw `invalid_chain_payload` with contextual info.
       - Inject a fixed `reason` field (e.g., `"Chained execution from agent__b"`) when the downstream schema requires it. Chained stages never need additional rationale text.
     - Text: supply `{ input: result, reason: "Chained execution from agent__b" }`.
   - Call `this.execute(nextLink.toolName, downstreamArgs, parentSession, updatedOpts)` recursively (or via an iterative loop to avoid deep recursion).
     - Pass `parentOpPath` augmented with the current child (so logs remain hierarchical).
   - Return the downstream result to the caller.
5. **Result propagation**
   - The top-level `execute` call returns the final downstream payload (C’s output) to the parent orchestrator.
   - Record chain metadata (e.g., `extras.chainedStages = ['agent__b', 'agent__c']`) so accounting reflects the cascade. Accounting events must continue to roll up into the opTree exactly as they do today.

## Error Handling
- Missing downstream agent: throw during load; session fails fast.
- Payload mismatch:
  - JSON parse failure → propagate as `invalid_chain_payload` (fatal for the chain).
  - Schema validation errors → use Ajv with downstream schema (when present) and report detailed errors.
- Downstream execution failure: propagate error to caller; optionally attach `chainStage` context so the parent LLM/user sees where it failed.
- Abort/stop propagation: reuse existing `abortSignal`/`stopRef` so cancelling the parent session stops the entire chain.

## Logging & Trace
- Prefix logs with chain context (`child:<toolName>`) as today (`src/subagent-registry.ts:267-305`).
- Extend accounting extras with `chainParent`, `chainStageIndex` for analytics.
- Ensure `SessionTree` captures the additional call path (`parentOpPath` extended on each stage).
- Tag logs/accounting entries originating from deterministic chaining (e.g., `chainTrigger: 'next'`) so downstream consumers can distinguish mandatory chaining from LLM-selected calls.

## Compatibility Considerations
- Agents without `next` behave exactly as before.
- Chained agents cannot currently be used standalone without triggering the chain; confirm this is acceptable.
- Frontmatter templates printed via `--help` must include guidance about `next` (with warnings about required downstream configuration).
- Documentation needs updating (SPECS/DESIGN) once implemented.

## Implementation Checklist
1. **Frontmatter**
   - Allow `next` in parser + template generator.
2. **Registry Data Model**
   - Update `ChildInfo` structure; add resolution pass.
3. **Validation**
   - Implement chain resolution, schema extraction, cycle detection.
4. **Execution**
   - Add pre-run output alignment and post-run chain dispatch in `SubAgentRegistry.execute`.
   - Create `ChainRunner` helper for readability and testing.
5. **Instrumentation**
   - Update logging/accounting extras to record chained stages.
6. **Docs**
   - Update README / DESIGN after code lands.

## Future Work Hooks
- Multiple downstream entries with deterministic sequence.
- Conditional routing stage that outputs the next agent id (user’s router use case).
- Automatic fallback (e.g., try C else D on failure).
- CLI flag to disable chaining for debugging/test runs.

## Additional Risks / Validation Tasks
- **Reason field constraints**: audit existing downstream schemas to ensure a fixed injected `reason` satisfies any enum/length restrictions.
- **Output format analytics**: verify dashboards or log processors that rely on a child’s declared `expectedOutput` still behave correctly when chaining overrides runtime format expectations.
- **Alias resolution**: determine whether to reuse `AgentRegistry` alias mappings or introduce a dedicated resolver so `next` references accept the same identifiers users already rely on.

## Clarified Decisions
1. Chaining is mandatory once `next` is present; there is no bypass mode.
2. Downstream stages do not require free-form reasoning. Injecting a fixed `reason` string is acceptable whenever schemas insist on that field.
3. Accounting/trace aggregation must remain fully intact: every chained execution contributes to the opTree and accounting streams the same way as standard tool calls.
4. Schema alignment is strict. If the downstream schema cannot be satisfied by the upstream output, the chain fails fast; no additional mapping layer is required for Phase 1.
5. Observability keeps the existing format (agent invoking child) but adds a `chainTrigger: 'next'` tag so we can distinguish deterministic chains from LLM-induced tool calls.
