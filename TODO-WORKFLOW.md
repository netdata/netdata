# Multi-Agent Workflow Orchestration Design

## Implementation + Verification Task (Jan 12 2026)

### TL;DR
- Implement + verify the 3 commits ahead of `origin/master` (cdd45d9, 4b158da, a8b3421).
- Focus: testing framework rework + advisors/router/handoff runtime integration + docs/test alignment.
- Run all non-commercial tests (including `nova/xxx`); report any gaps.

### Analysis
- Repo state: `master` is ahead of `origin/master` by 3 commits (cdd45d9, 4b158da, a8b3421).
- Orchestration modules exist (`src/orchestration/*`), but runtime integration is missing:
  - Tools registration only includes MCP/REST/Internal/Subagent providers; no router/advisor/handoff provider is registered. (`src/ai-agent.ts:560-771`)
  - `AIAgentSession.run()` has no pre/post orchestration flow (no advisors/handoff/router orchestration layer). (`src/ai-agent.ts:1152-1290`)
  - Frontmatter parsing accepts `routerDestinations`/`advisors`/`handoff`, but `LoadedAgent` has no orchestration fields to carry these values forward. (`src/frontmatter.ts:14-186`, `src/agent-loader.ts:49-96`)
  - Router tool schema only accepts `destination` (no optional `message` payload) and uses `router__handoff-to`. (`src/orchestration/router.ts:21-125`)
  - Orchestration types use trigger/condition objects (not the TODO-WORKFLOW frontmatter schema). (`src/orchestration/types.ts:1-58`)
- Docs do not mention `routerDestinations`/`advisors`/`handoff` frontmatter keys (README + docs search).
- Tests (from prior verification run):
  - `npm run lint` ✅
  - `npm run build` ✅
  - `npm run test:phase1` ✅ (Vitest warning: deprecated `test.poolOptions`)
  - `npm run test:phase2` ✅ (expected warning noise in harness)
  - `npm run test:phase3:tier1` ❌ (router scenarios failed: `router__handoff-to` not invoked; tool not registered)

### Current Status Review (Jan 12 2026)
- **Frontmatter supports `router`, `advisors`, `handoff`** and rejects unknown keys (`routerDestinations` will error).  
  Evidence: `src/frontmatter.ts:14-21`, `src/frontmatter.ts:109-127`, `src/frontmatter.ts:179-201`.
- **Handoff is strict single string**; arrays are rejected.  
  Evidence: `src/frontmatter.ts:196-199`.
- **Options registry includes advisors/handoff keys (no routerDestinations alias)**.  
  Evidence: `src/options-registry.ts:103-118`.
- **Loader resolves orchestration runtime/config + validates router.destinations**.  
  Evidence: `src/agent-loader.ts:579-633`, `src/agent-loader.ts:596-600`, `src/agent-loader.ts:636-639`.
- **Router tool provider is registered per session when destinations exist**.  
  Evidence: `src/ai-agent.ts:761-772`.
- **Router tool schema supports optional `message` and name `router__handoff-to`**.  
  Evidence: `src/orchestration/router.ts:11-55`.
- **Router selection is captured on tool execution**.  
  Evidence: `src/session-tool-executor.ts:402-416`.
- **Router selection short-circuits session result in TurnRunner** (returns routerSelection result).  
  Evidence: `src/session-turn-runner.ts:1786-1796`.
- **Orchestration wrapper `AIAgent.run` executes advisors pre-run and router/handoff post-run**.  
  Evidence: `src/ai-agent.ts:2332-2460`.
- **Tagging uses nonce-suffixed XML-like blocks for advisory/response/original user**.  
  Evidence: `src/orchestration/prompt-tags.ts:1-38`, `src/orchestration/handoff.ts:60-75`, `src/ai-agent.ts:2378-2421`.

### Current Gaps / Test Mismatches (Jan 12 2026)
- **Phase 3 test agents still use `routerDestinations`** (now rejected).  
  Evidence: `src/tests/phase3/test-agents/orchestration-master.ai:9-10`, `src/tests/phase3/test-agents/legal-team.ai:4-5`.
- **Phase 3 specialist-agent uses `handoff` as list** (now rejected).  
  Evidence: `src/tests/phase3/test-agents/specialist-agent.ai:4-5`.
- **Phase 3 router scenario expects router session to call sub-agent directly** (now impossible because router tool selection ends the session and routing happens in `AIAgent.run`).  
  Evidence: `src/tests/phase3-runner.ts:195-214` (requires `agent__legal-team`), `src/session-turn-runner.ts:1786-1796` (routerSelection returns early).
- **Phase 3 "advisor" scenario is still tool-driven** (calls `agent__legal-advisor`), not using `advisors:` frontmatter-based orchestration.  
  Evidence: `src/tests/phase3-runner.ts:170-184`, `src/tests/phase3/test-agents/orchestration-master.ai:14-16`.
- **Status**: Costa confirmed these Phase 3 tests are wrong and must be rewritten to match the orchestration design.

### Decisions Required (Costa)
1) **Phase 3 orchestration coverage scope (live LLM; flake-sensitive)**  
   **Evidence**: Phase 3 runs real models (`docs/TESTING.md:61-116`), and orchestration is specified in README (`README.md:17-41`).  
   - A) **Minimal core coverage**: 3 scenarios (advisors pre-run, router routing, handoff post-run).  
     - Pros: lower flake risk/cost; still proves each pattern.  
     - Cons: misses composition precedence.
   - B) **Core + precedence**: add router+parent-handoff precedence scenario.  
     - Pros: validates agreed precedence rule end-to-end.  
     - Cons: extra runtime/cost; more flake surface.
   - C) **Full composition**: add advisors+handoff and router+handoff scenarios.  
     - Pros: strongest end-to-end proof.  
     - Cons: highest flake/cost; more maintenance.
   - **Recommendation: B**.

2) **Phase 3 advisor failure testing**  
   **Evidence**: Advisor failure is specified in TODO (synthetic failure blocks) but real LLM failures are nondeterministic.  
   - A) **Skip in Phase 3** (keep in unit tests only).  
     - Pros: stable; avoids nondeterministic failures.  
     - Cons: no live-provider coverage for failure path.
   - B) **Include a forced failure scenario** (advisor instructed to error).  
     - Pros: live coverage.  
     - Cons: fragile; may pass inconsistently.
   - **Recommendation: A**.

### Decisions Made (Costa, Jan 12 2026)
1) **Phase 3 orchestration coverage scope**  
   - **Chosen: C** — full composition coverage (core + precedence + advisors+handoff + router+handoff).
2) **Phase 3 advisor failure testing**  
   - **Chosen: B** — include a forced failure scenario for advisor failure in Phase 3.
3) **Phase 3 model tiering**  
   - **Chosen**: nova provider models are Tier 1 (non-commercial), not Tier 2.

### Decisions
1) **Which orchestration schema should be the source of truth?**
   - A) **Align implementation to TODO-WORKFLOW spec** (frontmatter `handoff`, `router.destinations`, `advisors` list; router tool accepts destination + optional message; orchestration runs pre/post session).  
     - Pros: matches existing design doc; clearer UX; supports handoff/router/advisor semantics as agreed.  
     - Cons: requires wiring + docs updates + tests adjustments.
   - B) **Adopt current trigger-based types** (`HandoffConfig`/`AdvisorConfig`/`RouterConfig`) and update TODO-WORKFLOW/docs to match.  
     - Pros: less refactor in orchestration modules/tests.  
     - Cons: deviates from agreed design; frontmatter format still undefined; higher complexity.
   - C) **Pause orchestration integration** (leave modules as experimental, remove/skip router scenarios in Phase 3 for now).  
     - Pros: minimal change now.  
     - Cons: features remain incomplete; Phase 3 fails; TODO-WORKFLOW stays unfulfilled.
   - **Recommendation: A** (keep TODO-WORKFLOW as contract and wire implementation to match it).
   - **Chosen: A (Costa confirmed on Jan 12 2026)** — align implementation to TODO-WORKFLOW spec.

2) **Orchestration config + runtime decisions (Costa, Jan 12 2026)**
   - **Agent identifiers**: Frontmatter agent references include `.ai` and must reuse existing agent identification logic (no new resolution rules).
   - **Tool namespaces**: Router/handoff tool must be namespaced; **chosen namespace: `router__`**. Namespace handling must work in final turns and existing namespace checks.
   - **Missing agents**: Hard failure at load-time if any referenced agent is missing.
   - **Router terminal choice**: Routers may call `final_report` for trivial replies (e.g., “hello”); allow terminal response per TODO.
   - **Advisors**: Run with full tools enabled.
   - **Prompt encapsulation**: Use XML-like tags with a **stable prefix + nonce suffix** and no escaping.
     - Tag names: `<original_user_request__{nonce}>`, `<advisory__{nonce}>`, `<response__{nonce}>`
     - Nonce format: **12-char hex**
     - Nonce scope: **per block** (each tag gets its own nonce)
     - Advisors: `<original_user_request__{n}>...</original_user_request__{n}>` + `<advisory__{n} agent="...">...</advisory__{n}>` per advisor.
     - Router: `<original_user_request__{n}>...</original_user_request__{n}>` + optional `<advisory__{n} agent="router-agent-name">...</advisory__{n}>`.
     - Handoff: `<original_user_request__{n}>...</original_user_request__{n}>` + `<response__{n} agent="agent-name">...</response__{n}>`.

3) **Pending decisions (needs Costa before implementation continues)**
   - **Orchestration entry-point** (to keep sessions pure while covering all headends).
   - **Result merge semantics** for handoff/router (what becomes the top-level `AIAgentResult`: `success`, `error`, `finalReport`, `conversation`, `logs`, `accounting`, `opTree`).
   - **Router + handoff precedence** if both are declared in frontmatter.
   - **Handoff schema strictness**: accept only a single string or also allow a single-item list for backward tolerance.

### Decisions Made
1) **Orchestration entry-point** (Costa, Jan 12 2026)
   - **Chosen**: introduce an orchestration wrapper (e.g., `AIOrchestration.run`) that replaces all direct `AIAgentSession.run` calls.
   - Behavior: wrapper is **pass-through** when no advisors/router/handoff are configured; otherwise it performs orchestration and then calls the session internally.
2) **Result merge semantics** (Costa, Jan 12 2026)
   - **Chosen: 2A** — Parent is primary; child is attached.
   - Keep parent conversation/logs as top-level; child logs/accounting appended; child final report becomes top-level final report.
3) **Router + handoff precedence** (Costa, Jan 12 2026)
   - **Chosen: 3C** — Always apply parent handoff **after** router chain completes.
   - Flow: `parent -> (router-selected agent + its own handoffs) -> parent handoff target`.
   - **Clarification**: parent handoff is **never bypassed**. If you want to bypass handoff for a route, attach the handoff to the router destination instead.
4) **Handoff schema strictness** (Costa, Jan 12 2026)
   - **Chosen**: strict single handoff only (string). Lists are invalid for now.
   - If multiple are ever supported later, they must be a **pipeline** (sequential).
5) **Agent label for XML tags** (Costa, Jan 12 2026)
   - **Chosen**: use the session/agent ID (current `agentId` / basename) for `agent="..."` in advisory/response tags.
6) **Orchestrator naming** (Costa, Jan 12 2026)
   - **Chosen**: use `AIAgent.run` as the orchestration wrapper name (replacing AIOrchestrator).
   - Rule: only `AIAgent.run` is allowed to call `AIAgentSession.run`; all other call sites must use `AIAgent.run`.
7) **Current task scope** (Costa, Jan 12 2026)
   - **Chosen**: verify current code status for routers/advisors/handoff and update TODO with concrete evidence.
8) **Phase 3 test correction** (Costa, Jan 12 2026)
   - **Chosen**: Phase 3 tests are incorrect; rewrite them to match the agreed orchestration design (routers/advisors/handoff).

### Plan
- Read required docs (SPECS, IMPLEMENTATION, DESIGN, MULTI-AGENT, TESTING, AI-AGENT-INTERNAL-API, README, AI-AGENT-GUIDE, SESSION-SNAPSHOTS).
- Re-audit routers/advisors/handoff runtime against the design (confirm no missing behaviors).
- Redesign Phase 3 orchestration scenarios to test **frontmatter-driven** advisors/router/handoff flows (ignore current Phase 3 files).
- Update Phase 3 test agents + runner expectations accordingly (new prompts, new assertions, new agent frontmatter).
- Update docs as needed (README/TESTING/etc) to describe the new Phase 3 orchestration coverage.
- Run tests: `npm run lint`, `npm run build`, plus all non-commercial tests (confirm commands; run `nova/xxx`).
- Report issues with concrete evidence (file + line references) and confirm completeness or list gaps.

### Implied Decisions
- Proceed with implementation to complete advisors/router/handoff runtime flow per TODO-WORKFLOW.

### Testing Requirements
- `npm run lint`
- `npm run build`
- All non-commercial tests; include `nova/xxx` (confirm exact script names in `package.json`).

### Documentation Updates Required
- Update docs when behavior changes (README, SPECS/specs, DESIGN, MULTI-AGENT, AI-AGENT-GUIDE, TESTING). Keep in same commit.

## TL;DR
- Add 4 orchestration patterns: **Handoff**, **Router**, **Advisors**, **Team**
- Patterns are frontmatter-driven (decentralized, each agent knows only its immediate needs)
- Orchestration layer wraps existing session layer (sessions stay pure)
- `AIAgentSession.run()` becomes recursive: checks frontmatter, applies patterns, calls child sessions
- Terminal vs. non-terminal agents: only terminal agents own their final answer
- Team pattern baseline decided (broadcast-first, supervisor-controlled); remaining work is implementation detail for queues/hooks

## Analysis

### Current Architecture

- **Session model**: `AIAgentSession` manages single-agent conversation loop (`src/ai-agent.ts:118`)
- **Execution entry**: `AIAgentSession.run()` → executeAgentLoop → multi-turn conversation (`src/ai-agent.ts:1037`)
- **Sub-agents**: Exposed as tools via `SubAgentRegistry`, each spawns independent session (`src/subagent-registry.ts`)
- **Complete isolation**: Each session has own conversation, tools, providers, accounting
- **Accounting rollup**: Child accounting aggregates to parent via opTree
- **No orchestration layer**: Currently all coordination is tool-call based (LLM decides)

### Proposed Layered Architecture

```
AIAgentSession.run(input)
  │
  ├─ Orchestration Layer (NEW)
  │   ├─ Pre-session: Consult pattern (enrich input)
  │   ├─ Session execution (existing behavior)
  │   └─ Post-session: Handoff/Router patterns (delegate output)
  │
  └─ Session Layer (EXISTING - unchanged)
      └─ executeAgentLoop → multi-turn conversation
```

**Key principle**: Sessions remain pure single-agent loops. Orchestration is external coordination using recursive `AIAgentSession.run()` calls.

### Current Implementation Status (Nov 16 2025 review)
- `src/frontmatter.ts:14-120` only accepts the classic options registry keys, so prompts currently have no way to declare `handoff`, `router`, `advisors`, or `team` metadata. The new YAML fields will simply be rejected until we extend the parser and allowed-key registry.
- `src/agent-loader.ts:24-210` builds `LoadedAgent` objects without any orchestration fields. Even if frontmatter parsing were extended, the loader would drop those values before sessions are created, so we need explicit storage plus validation hooks.
- `src/ai-agent.ts:1037-1400` runs sessions directly via `executeSession()` with no pre- or post-processing stages; the recursion helper described earlier (`spawnChildSession`) does not exist yet. All child invocations still go through `SubAgentRegistry`, meaning orchestration has zero entry-points today.
- `src/subagent-registry.ts` remains the sole recursion surface (tool-driven). There is no accounting pipeline today for “out-of-band” session launches, so once orchestration calls are added we must plumb opTree + accounting aggregation manually instead of relying on existing tool metadata.

### Pattern Definitions

#### 1. Handoff (`handoff: agent_name`)
**What**: Agent completes its work, then delegates final response to another agent.
**Real-world**: Approval workflows, department-to-department handoff, review gates.
**Why**: Cost savings (skip return path), enforce process, hardcode workflows.
**Terminal**: Non-terminal (delegates final answer).

```yaml
# frontmatter
handoff: approval-gateway
```

**Execution flow**:
```
agent.run(input)
  → session.start(input)
  → result = session output
  → handoff_agent.run(
      <original_user_request>...</original_user_request>
      <response agent="agent-name">...</response>
    )
  → return handoff_agent's result
```

#### 2. Router (`router` block + `handoff-to` tool)
**What**: Normal multi-turn agent (full tools, sub-agents, reasoning) whose only exit path is a `handoff-to` tool instead of `final_report`. When invoked, the tool selects one destination from a fixed enum and can attach an optional message that becomes part of the downstream prompt.
**Real-world**: Receptionist, triage, dispatcher that can gather context before delegating.
**Why**: Gives routers the full power of the platform while still forcing deterministic delegation.
**Terminal**: Non-terminal (delegates entire request).

```yaml
# frontmatter
router:
  destinations:
    - legal-department
    - technical-support
    - billing
```

**Execution flow**:
```
agent.run(input)
  → session.start(input)  // normal conversation with tools/sub-agents
  → router eventually calls handoff-to(destination, message?)
  → downstreamInput =
      <original_user_request>
      [original input]
      </original_user_request>
      <advisory agent="router-agent-name">
      [optional message]
      </advisory>
  → selectedAgent.run(downstreamInput)
  → return selectedAgent's result
```

If the router omits `message`, only the original user section is forwarded.

**Tool schema**:
```typescript
{
  name: "handoff-to",
  description: "Send the user and an optional note to the selected agent",
  parameters: {
    agent: { type: "string", enum: [...destinations] },
    message: { type: "string", description: "Optional context for the next agent" }
  }
}
```

#### 3. Advisors (`advisors: [agents...]`)
**What**: Before agent starts, run multiple agents in parallel with same input, enrich original request with their outputs.
**Real-world**: Manager requiring legal + operational review before decision.
**Why**: Cost savings (vs. tool calls), time savings (parallel execution), structured analysis gathering.
**Terminal**: Terminal (this agent makes final decision, advisors only advise).

```yaml
# frontmatter
advisors:
  - compliance-checker
  - risk-assessor
  - technical-reviewer
```

**Execution flow**:
```
agent.run(input)
  → parallel: advisor.run(input) for each advisor
  → enriched_input = format_enriched_message(input, advisor_outputs)
  → session.start(enriched_input)
  → return result
```

**Failure handling**:
- Each advisor that fails produces a synthetic `final_report` describing the failure.
- The primary agent receives that failure text in place of the advisor's normal output and proceeds with whatever information is available.

**Enriched message format**:
```text
<original_user_request>
[original input]
</original_user_request>
<advisory agent="compliance-checker">
[compliance-checker output]
</advisory>
<advisory agent="risk-assessor">
[risk-assessor output]
</advisory>
<advisory agent="technical-reviewer">
[technical-reviewer output]
</advisory>
```

#### 4. Team (`team: { members: [...], ... }`)
**What**: Multiple agents collaborate asynchronously via a broadcast channel. Each member keeps an independent session and can only emit `team_broadcast` messages (no `final_report` tool for members). Members exit the team via a final broadcast (success or failure). The supervisor is the sole terminal agent.
**Real-world**: Code review panel, planning committee, stakeholder alignment.
**Why**: Extreme cost savings (impractical to transfer full discussions via tool calls), parallel deliberation.
**Terminal**: Supervisor/boss is terminal (makes final decision). Other members are non-terminal contributors.

```yaml
# frontmatter
team:
  members:
    - analyst
    - developer
    - security-reviewer
  supervisor: planning-lead
  memberMaxTurns: 6
  supervisorMaxTurns: 12
```

**Execution model**: See "Team Pattern - Broadcast Baseline" section below.

### Terminal vs. Non-Terminal Agents

**Critical concept**: Only terminal agents own their final answer.

| Pattern | Terminal? | Why |
|---------|-----------|-----|
| Standard orchestrator | Yes | Owns its final answer |
| Has `handoff` | No | Delegates final answer to next agent |
| Router | No | Delegates entire request to selected agent |
| Has `advisors` | Yes | Makes final decision (advisors only advise) |
| Team supervisor | Yes | Synthesizes team discussion into final answer |
| Team member (non-supervisor) | No | Contributes to discussion, doesn't own final answer |

**Composition rule**: When referencing an agent (for team membership, tool call, etc.), the **terminal agent** is the one whose output is returned.

**Example chain**:
```
Agent A (handoff: B)     → non-terminal, terminal agent = D
Agent B (handoff: C)     → non-terminal, terminal agent = D
Agent C (handoff: D)     → non-terminal, terminal agent = D
Agent D (no handoff)     → terminal
```

Calling A returns D's output. This is resolved at runtime as `AIAgentSession.run()` executes recursively.

**Team composition**: If team member has handoff chain, the terminal agent at the end of the chain participates in team.

### Implementation in AIAgentSession.run()

```typescript
class AIAgentSession {
  async run(): Promise<AIAgentResult> {
    // Pre-session orchestration: Advisors pattern
    if (this.config.advisors) {
      this.userMessage = await this.runAdvisors(this.userMessage, this.config.advisors);
    }

    // Session layer (existing behavior, unchanged)
    const result = await this.executeSession();

    // Post-session orchestration: Handoff pattern
    if (this.config.handoff) {
      return this.runHandoff(result, this.config.handoff);
    }

    // Post-session orchestration: Router pattern
    if (this.config.router && result.routerSelection) {
      return this.runRouter(result.routerSelection);
    }

    return result;
  }

  private async runAdvisors(input: string, advisors: string[]): Promise<string> {
    // Spawn all advisors in parallel
    const advisorResults = await Promise.all(
      advisors.map(agent => this.spawnChildSession(agent, input))
    );
    // Format enriched message
    return this.formatEnrichedInput(input, advisorResults);
  }

  private async runHandoff(result: AIAgentResult, nextAgent: string): Promise<AIAgentResult> {
    // Recursive call to next agent
    return this.spawnChildSession(nextAgent, result.output);
  }
}
```

### Frontmatter Schema Extensions

```typescript
interface AgentFrontmatter {
  // Existing fields...

  // NEW: Orchestration patterns
  handoff?: string;                    // Agent to hand off to after completion
  router?: {                           // Router metadata when agent finishes via handoff-to
    destinations: string[];            // Whitelist of agent ids exposed in handoff-to
  };
  advisors?: string[];                 // Agents to consult before execution
  team?: {
    members: string[];                 // Team member agents (non-terminal)
    supervisor: string;                // Terminal "boss" agent controlling lifecycle
    memberMaxTurns?: number;           // Optional per-member cap
    supervisorMaxTurns?: number;       // Optional global cap mirrored from boss settings
  };
}
```

**Validation rules**:
- Router frontmatter must list at least one destination and routers must not expose `final_report` (only `handoff-to`). Otherwise routers behave like normal agents and may still declare tools/sub-agents.
- Team supervisor (the boss) owns the terminal `final_report`; team members must not declare their own `handoff` targets.
- Cycle detection: handoff chains, router destinations, team membership (no agent can be ancestor of itself).
- Advisors cannot be terminal (they don't produce user-facing answers).
- All validations continue to run during agent loading, so misconfigurations are caught before runtime.

## Decisions

### 1. Naming Confirmation
- ~~`handoff` vs `next` vs `delegate`~~ **DECIDED: `handoff`**
- ~~`router` vs `dispatch` vs `triage`~~ **DECIDED: `router`**
- ~~`consult` vs `advisors` vs `gather`~~ **DECIDED: `advisors`**
- ~~`team` vs `panel` vs `council`~~ **DECIDED: `team`**

### 2. Router Semantics
- Routers are full-fledged agents (all normal tools, sub-agents, multi-turn reasoning) but never call `final_report`.
- They must end by invoking `handoff-to(destination, message?)`, where `destination` is one of the configured enum entries and `message` becomes an optional markdown section for the downstream agent.
- Downstream prompt format is fixed: include original user request plus the optional router message. If the router omits `message`, the supplementary section is excluded.

### 3. Advisor Failure Handling
- When an advisor fails or times out, the orchestrator emits a synthetic `final_report` describing the failure.
- The primary agent receives this synthetic report in place of the advisor's analysis and may decide how to proceed; advisors never retry automatically.

### 4. Team Pattern Direction
- Team orchestration adopts the broadcast-first model: members run asynchronously, use a dedicated broadcast tool, and their final reports both exit the member and broadcast an "unreachable" notice.
- The supervising agent owns the terminal `final_report`. When it finishes (or exhausts turns), all remaining members are cancelled.
- Remaining work is implementation detail (queue fairness, injection formatting), not a design decision.

### 5. Accounting Aggregation
- All orchestration participants—handoff partners, routers, advisors, team members—roll up costs to the terminal agent. They are treated as sub-agents even when not spawned via explicit tool calls.

### 6. Frontmatter Validation
- Validation remains a load-time concern. The loader checks router destination lists, cycle detection, advisor/team compatibility, etc., before any session starts.

## Team Pattern - Broadcast Baseline

Team orchestration adopts the broadcast-first model Costa specified. Members operate independently, exchange findings via a dedicated broadcast tool, and the supervising "boss" agent owns the final answer and global lifecycle.

### 1. Message Broadcasting
- Each member gets a `team_broadcast(message)` (name TBD) tool. Calls append to a shared transcript visible to every other member before their next turn.
- Members do **not** expose `final_report`; their only exit path is a terminal broadcast (success or failure) which marks them exited and tells peers they are no longer reachable.
- Broadcasts always include sender identity and timestamp/order. We still need to design the injection format (probably a prefixed markdown section per member) but the presence of the special tool is decided.

### 2. Session Coordination
- Members run asynchronously with their own turn limits. Each has its own session loop and only pauses to ingest new broadcasts injected between turns.
- The team orchestrator must maintain per-member queues and a global log, feeding new broadcasts into waiting members before they continue.
- Remaining design work: define fairness (e.g., round-robin vs. opportunistic) and ensure starvation does not occur, but parallel async execution is now fixed.

### 3. Termination + Bounds
- Every member enforces its own `maxTurns`, but the entire team is also bounded by the supervising agent's `maxTurns`. If the supervisor (`team boss`) delivers its `final_report`, the orchestrator cancels/aborts all remaining members immediately.
- Members that exhaust their turns or send a terminal broadcast are marked exited and removed from scheduling.
- Supervisor remains the sole terminal agent; all costs from members roll into the supervisor's accounting lineage.

### 4. Hook Points Needed
```typescript
interface TeamOrchestrator {
  onMemberBroadcast(memberId: string, message: string): void; // update global log + fan-out
  onMemberExit(memberId: string): void;                       // mark as unreachable
  injectMessages(memberId: string, messages: string[]): void; // feed queued broadcasts before next turn
  cancelRemaining(reason: string): void;                      // invoked when boss finalizes
}
```

Open work here: define the concrete queue implementation + exact markdown injected each turn, but the broadcast + supervisor-cancellation semantics are locked.

## Team Communication Implementation Options (analysis Nov 16 2025)

### Option A – Dedicated `TeamOrchestrator` + Broadcast Tool Provider (recommended)
- Introduce `src/orchestration/team-orchestrator.ts` that owns the team lifecycle and an in-memory `TeamConversationBroker`. The broker stores an append-only transcript plus per-member cursors so messages are only injected once per turn. The orchestrator spins up each member via the existing `AIAgentSession` API, passing a session-scoped `ToolProvider` that exposes a single internal tool `team__broadcast(message: string)`.
- Implementation hooks: register the provider in `ToolsOrchestrator` before `AIAgentSession.run()` (see `src/tools/tools.ts:65-210`). When the member LLM calls the tool, the provider writes to the broker and returns a short acknowledgement. Before each member turn, the orchestrator injects any unread broadcasts by prepending a markdown block to the user prompt (similar to how context guard messages are injected in `src/ai-agent.ts:118-380`).
- Benefits: keeps `AIAgentSession` untouched (it only sees another internal tool); isolation is preserved because every member still builds its own session state. Extensible: new communication primitives become more tool endpoints managed by the broker (brainstorm: `team__dm`, `team__request_help`). Maintainability: orchestration code lives in one module with a strict interface (constructor inputs = member agent IDs + config, outputs = supervisor result plus transcript snapshot).
- Considerations: orchestrator must multiplex multiple active sessions, so we need a lightweight scheduler (round-robin, fairness hooks) plus cancellation wiring (use `AbortController`s propagated to each session). Accounting/opTree rollup should treat broadcasts as zero-cost internal tools but still log events for traceability.

### Option B – Supervisor-Mediated Relay
- The supervisor remains the only agent with the broadcast tool. Other members surface updates by sending `team_update` payloads through a new orchestration callback rather than calling tools. The orchestrator materializes each update inside the supervisor’s conversation (`## Update from member`) and, when the supervisor issues `team_broadcast`, the orchestrator pushes that note into other members’ prompts.
- Benefits: simpler concurrency story (only supervisor drives prompts). Works well if Costa later requests hierarchical chains (supervisor delegating sequential turns). Also minimizes new tool plumbing.
- Drawbacks: members cannot react to each other asynchronously; every exchange funnels through the supervisor, which contradicts the “broadcast-first” requirement and increases latency/cost. Harder to extend to future strategies like “swarm” or ad-hoc committees because there is no reusable broker.

### Option C – Shared Transcript Snapshot Injection
- Maintain a shared markdown transcript under `TeamState`. After each member turn we serialize the entire transcript and inject it wholesale into every member’s next user message (similar to the advisors enrichment format defined earlier).No new tool is required; each agent simply reads/writes from the orchestrator via direct API calls.
- Benefits: implementation speed (just maintain a string buffer and splice it in). Testing is easier because the orchestrator reduces to “gather outputs, rebuild markdown, rerun sessions.”
- Drawbacks: violates session isolation ideals once transcripts grow large (every member prompt balloons each turn, increasing context churn). Hard to add selective delivery or sub-channels later. Merging in/out logic would leak orchestration concerns into `AIAgentSession`, hurting maintainability.

### Recommended Path
- Adopt Option A as the baseline, because it isolates concerns (orchestrator handles communication, sessions handle reasoning) and naturally extends to future patterns (e.g., replace the broker with a priority queue, or add new `team__` tools). It also maps directly onto the already-decided broadcast semantics and keeps the rest of the system in “sessions are pure” compliance.
- When implementing, codify a narrow interface:
  - `TeamOrchestrator.start(): Promise<TeamResult>` – spins members + supervisor.
  - `TeamConversationBroker.publish(memberId, payload)` / `consume(memberId)`.
  - Tool provider factory `createTeamToolProvider(memberId, broker)` returning a `ToolProvider` registered via `ToolsOrchestrator.register()` for that session only.
- Provide clear extension seams: e.g., broker can expose middleware hooks so other strategies (swarm/voting) can subscribe without modifying the orchestrator core.

## Team Synchronization & Failure Handling (Agreed Nov 27 2025)

- **Cycle ordering (source of truth = master session)**: User prompt → all active members run in parallel (each may take multiple internal turns, broadcast-only) → master consumes all broadcasts in one turn. If the master broadcasts (not final), that broadcast triggers the next member cycle. If the master issues `final_report`, orchestrator cancels any remaining members and returns the master result.
- **Member exits and failures**: Members have no `final_report`. Their only exit path is a terminal `team_broadcast` (success or failure). On model/tool failure or turn exhaustion, orchestrator emits a failure broadcast: `member X failed and is no longer in the team. Reason: ...`, marks the member exited, and fans this to master + remaining members.
- **Busy member delaying master is acceptable**: There is no intra-cycle cap (`maxTurnToBroadcast` deferred). If a member hogs turns, fix its prompt/tools; the orchestrator does not time-slice further.
- **Broadcast schema (required)**: Payload must carry `type` (update|failure), `memberId`, `message`, `turnCount`, `timestamp` to let the master distinguish failures from normal updates.
- **Context/window guards**: Reuse existing tool-size acceptance logic. If injecting queued broadcasts would overflow the context, drop the offending payload, replace it with a diagnostic broadcast explaining the overflow, and force the master to take its final turn so the run terminates cleanly per CONTRACT.
- **Injection fan-out**: Master broadcasts are fanned out to all non-exited members before the next cycle. Member broadcasts are buffered and injected into the next master turn; they are also fanned to peers before those peers resume.

## Plan

> Current priority: deliver **Phase 1 (Sub-Agent Clarifications)** so existing agents can chat with previously spawned children. Later phases will be revisited once Phase 1 is complete and the design has proven itself.

### Phase 1: Sub-Agent Clarifications

#### Step 1: Frontmatter & Loader
**Files**: `src/frontmatter.ts`, `src/agent-loader.ts`, `src/options-registry.ts`
- Add `chat: boolean` (default `false`) to `FrontmatterOptions`
- Parse and store on `LoadedAgent` and `PreloadedSubAgent.chatEnabled`

#### Step 2: AIAgentSession.clarify() Method
**File**: `src/ai-agent.ts`
- Add `public async clarify(question: string, parentCallPath: string): Promise<AIAgentResult>`
- Format: `## CLARIFICATION REQUEST FROM {parentCallPath}\n\n{question}`
- Append as user message, reset `this.finalReport`, call `executeAgentLoop()`
- Add `getTurnsRemaining(): number` (returns `maxTurns - currentTurn`)

#### Step 3: LiveSubAgentRegistry
**File**: `src/subagent-registry.ts`
- Add `liveChildren: Map<string, { session, chatEnabled }>`
- Modify `execute()`: if `chatEnabled`, keep session, generate ID, append footer
- Add `clarifyChild()`, `destroyChild()`, `destroyAllChildren()`, `listLiveChildren()`

#### Step 4: agent__ask_clarification Tool
**Files**: `src/tools/internal-provider.ts`, `src/internal-tools.ts`
- Register tool with dynamic enum of live child IDs
- Execute: call `registry.clarifyChild()`, return structured result
- Handle errors: `chat_disabled`, `clarification_no_turns`

#### Step 5: Cleanup
**File**: `src/ai-agent.ts`
- In `run()` finally block: call `this.subAgents?.destroyAllChildren()`
- On abort: propagate to children, destroy all

#### Step 6: Testing & Documentation
- Test cases: success, chat disabled, no turns, nested, cleanup cascade, accounting
- Update docs: `chat` frontmatter, tool usage, error conditions

---

### Phase 2: Orchestration Foundation
1. Extend frontmatter parsing to accept `handoff`, `router`, `advisors`, `team` fields
2. Add validation: cycle detection, pattern compatibility checks
3. Implement `isTerminal` flag computation based on frontmatter
4. Add orchestration layer wrapper in `AIAgentSession.run()`

### Phase 3: Handoff Pattern
1. Implement `runHandoff()` in AIAgentSession
2. Chain resolution at load time (validate references exist)
3. Accounting aggregation for handoff chains
4. Update logging/tracing to show handoff flow

### Phase 3: Advisors Pattern
1. Implement `runAdvisors()` with parallel execution
2. Message enrichment formatting
3. Emit synthetic `final_report` payloads when advisors fail/time out
4. Accounting aggregation for parallel advisors

### Phase 4: Router Pattern
1. Implement router agent type (normal loop but finalizes via `handoff-to` tool)
2. Build downstream prompt compositor (original request + optional router message)
3. Routing failure handling (e.g., no destinations, tool misuse)
4. Validation: routers require destinations and must not expose `final_report`

### Phase 5: Team Pattern (Requires Separate Design Document)
1. Implement broadcast tool + shared log injection between member sessions
2. Build async scheduler that enforces per-member and supervisor turn limits
3. Add supervisor-driven cancellation + auto-broadcast on member exit
4. Test with simple 2-agent team plus supervisor to validate broadcast + cancellation
5. Scale to N-agent teams and ensure starvation/fairness policies hold

### Phase 6: Integration Testing
1. Test composition: handoff + advisors, router + handoff, team with handoff members
2. Cycle detection edge cases
3. Error propagation across orchestration layers
4. Accounting accuracy verification

## Implied Decisions

- Orchestration patterns are intrinsic to agent (frontmatter-defined), not external configuration
- Patterns compose naturally via recursive `AIAgentSession.run()` calls
- Sessions remain pure (no knowledge of orchestration patterns)
- Terminal resolution happens at runtime as execution unfolds
- Each agent knows only its immediate orchestration needs (decentralized)
- Full workflow emerges at runtime, not predetermined
- Accounting for every orchestration hop rolls up to the terminal agent (treat all upstream participants as sub-agents regardless of invocation path)

## Pros and Cons

### Pros
- **Clean separation**: Orchestration layer vs. session layer
- **Composable**: Arbitrary nesting (team member with handoff to another team)
- **Decentralized**: No central orchestrator config to maintain
- **Evolvable**: Change one agent's patterns without touching others
- **Cost efficient**: Avoid token overhead of tool-call-based coordination
- **Parallel execution**: Consult and team patterns enable parallelism
- **Backward compatible**: Agents without patterns work as before

### Cons
- **Complexity**: 4 new patterns with different semantics
- **Debugging**: Harder to trace emergent workflows
- **Team pattern**: Requires significant infrastructure (hooks, message queues, coordination)
- **Terminal resolution**: Runtime discovery can be surprising
- **Error propagation**: Failures in deep chains harder to diagnose
- **Testing**: Many composition scenarios to validate

## Testing Requirements

1. `npm run lint`, `npm run build` pass with zero warnings/errors
2. Phase 1 harness scenarios for each pattern:
   - Handoff: single hop, multi-hop chain, cycle detection
   - Router: successful routing, routing failure, invalid selection
   - Advisors: all succeed, partial failure, timeout
   - Team: 2-agent team, N-agent team, consensus termination, max rounds
3. Composition tests: handoff + advisors, router + handoff, team with handoff members
4. Accounting aggregation tests for all patterns
5. Manual smoke tests via `./run.sh` with real agents

## Documentation Updates Required

1. **README.md**: End-user documentation for orchestration patterns
2. **docs/SPECS.md** (or docs/specs/*.md): Pattern specifications
3. **docs/DESIGN.md**: Architecture diagrams showing orchestration layer
4. **docs/MULTI-AGENT.md**: Updated multi-agent coordination patterns
5. **docs/AI-AGENT-GUIDE.md**: Frontmatter schema for new fields
6. **Frontmatter help/templates**: Guidance for `handoff`, `router`, `consult`, `team`

## Legacy: Ported from TODO-CHAINS.md

### Handoff (formerly "Deterministic Sub-Agent Chaining")

The handoff pattern replaces the previous "next" chaining design. Key elements ported:

- **Cycle detection**: DFS stack during load, fail fast on recursion
- **Input/output alignment**: Upstream output format must match downstream input expectations
- **Reason field handling**: Inject fixed reason for chained executions
- **Error propagation**: Fail fast with contextual info (which stage failed)
- **Accounting aggregation**: Chain stages contribute to opTree, tagged with `chainTrigger: 'handoff'`
- **Abort propagation**: Parent's abortSignal/stopRef cancels entire chain

**Differences from original design**:
- Was `next` field, now `handoff` (clearer naming)
- Was post-execution in `SubAgentRegistry.execute()`, now in `AIAgentSession.run()` orchestration layer
- Applies to all agents (not just sub-agents), enabling top-level handoff chains

---

## Decisions from Nov 16 2025 Sync with Costa
1. **Handoff payload shape** – Downstream agents receive:
   - `<original_user_request>...</original_user_request>` containing the original user text, and
   - `<response agent="agent-name">...</response>` containing the upstream `final_report` body.
   No serialized `AIAgentResult` objects are forwarded.
2. **Router guard-rails** – Routers retain `agent__final_report`. If the LLM calls it, the router is effectively terminal and no handoff occurs. Otherwise routers are expected to finish via `handoff-to`.
3. **Advisor transcript formatting** – The enriched prompt uses XML-like tags:
   - `<original_user_request>...</original_user_request>`
   - `<advisory agent="advisor-name">...</advisory>` per advisor (raw output, no transformation).
   Failures still produce an `<advisory>` block with the synthetic failure report.

## Clarification Decisions (Nov 17 2025)
- **Phase 1 scope** – Deliver only “sub-agent chat”: every existing agent can continue talking to any sub-agent it already spawned. No new orchestration patterns ship in this phase.
- **Implementation locus** – Reuse the current sub-agent plumbing (`SubAgentRegistry` + `AIAgentSession`). We freeze the live child session after its first `agent__final_report` and resume it in-place when the parent calls `agent__ask_clarification`.
- **Resource policy** – No artificial caps or idle timeouts. The only limits are the declared `maxToolCallsPerTurn` and `maxTurns`; worst-case depth is bounded by those budgets. Frozen sessions live until the parent session ends, at which point we cascade `destroy()` down the tree.
- **Economics** – Handoffs, routers, and advisors already work today; they're simply expensive because every follow-up respawns a fresh sub-agent. Clarifications reduce those costs by letting us reuse the existing child session instead of rerunning it.
- **Frontmatter toggle** – Add `chat: true|false` (default **false**) so prompts must opt into clarification-friendly behavior. Advisors/teams (later phases) will require `chat: true`.
- **Clarification recursion** – Unlimited. A child being clarified may spawn or clarify its own children; chains can be arbitrarily deep.
- **Pattern composition** – Advisors fire before the main session (pre-spawned chat-enabled helpers). Handoffs/routers run after the main session completes (delegation of the answer). These shortcuts coexist without conflict.
- **Turn accounting** – Clarifications consume turns for both parent and child. Invoking `agent__ask_clarification` uses up one of the parent's tool turns, and the child's response decrements its remaining `maxTurns`.
- **Clarification UX** – Inject clarification prompts as user messages prefixed with:
  ```markdown
  ## CLARIFICATION REQUEST FROM {parentCallPath}

  {question}
  ```
  Tool responses return structured payloads including `status`, `content`, `clarificationId`, and `turnsRemaining` (plus a markdown footer so the LLM can read it without JSON parsing).
- **Cleanup semantics** – Child sessions persist only for the lifetime of the parent `AIAgentSession`. They are destroyed when explicitly requested, when the parent finishes its run, or when the parent aborts; the destroy operation cascades down to all descendants.

## Next Steps

1. ~~**Costa to decide**: Confirm naming choices~~ **DONE: team, handoff, router, advisors**
2. ~~**Costa to decide**: Router semantics (pure routing vs. with analysis tools)~~ **DONE: routers are full agents that finalize via `handoff-to` with optional message**
3. ~~**Costa to decide**: Advisor failure handling strategy~~ **DONE: emit synthetic `final_report` on failure/timeouts**
4. **Detailed team design**: Remaining work is implementation detail (queue format, fairness), but broadcast + supervisor control are locked per decision above
5. **Implementation order**: Recommend Handoff → Advisors → Router → Team (increasing complexity)
