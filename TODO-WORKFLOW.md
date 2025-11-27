# Multi-Agent Workflow Orchestration Design

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
  → handoff_agent.run(result)  // recursive!
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
      ## ORIGINAL USER REQUEST

      [original input]

      ## MESSAGE FROM AGENT `router-agent-name` WHO ROUTED THIS REQUEST TO YOU

      [optional message]
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
```markdown
## ORIGINAL USER REQUEST

[original input]

## ANALYSIS GATHERED

### From compliance-checker

[compliance-checker output]

### From risk-assessor

[risk-assessor output]

### From technical-reviewer

[technical-reviewer output]
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
1. **Handoff payload shape** – Downstream agents receive only the upstream agent’s `final_report` body (markdown/JSON). No headers, metadata, or serialized `AIAgentResult` objects are forwarded.
2. **Router guard-rails** – Routers retain `agent__final_report`. If the LLM calls it, the router is effectively terminal and no handoff occurs. Otherwise routers are expected to finish via `handoff-to`.
3. **Advisor transcript formatting** – The enriched prompt includes a markdown header per advisor (e.g., `### From risk-assessor`) followed by the advisor output raw, without transformation or summarization. Failures still get a header but the body is the synthetic failure report as-is.

## Clarification Decisions (Nov 17 2025)
- **Phase 1 scope** – Deliver only “sub-agent chat”: every existing agent can continue talking to any sub-agent it already spawned. No new orchestration patterns ship in this phase.
- **Implementation locus** – Reuse the current sub-agent plumbing (`SubAgentRegistry` + `AIAgentSession`). We freeze the live child session after its first `agent__final_report` and resume it in-place when the parent calls `agent__ask_clarification`.
- **Resource policy** – No artificial caps or idle timeouts. The only limits are the declared `maxToolCallsPerTurn` and `maxTurns`; worst-case depth is bounded by those budgets. Frozen sessions live until the parent session ends, at which point we cascade `destroy()` down the tree.
- **Economics** – Handoffs, routers, and advisors already work today; they're simply expensive because every follow-up respawns a fresh sub-agent. Clarifications reduce those costs by letting us reuse the existing child session instead of rerunning it.
- **Frontmatter toggle** – Add `chat: true|false` (default **false**) so prompts must opt into clarification-friendly behavior. Advisors/teams (later phases) will require `chat: true`.
- **Clarification recursion** – Unlimited. A child being clarified may spawn or clarify its own children; chains can be arbitrarily deep.
- **Pattern composition** – Advisors fire before the main session (pre-spawned chat-enabled helpers). Handoffs/routers run after the main session completes (delegation of the answer). These shortcuts coexist without conflict.
- **Turn accounting** – Clarifications consume turns for both parent and child. Invoking `agent__ask_clarification` uses up one of the parent’s tool turns, and the child’s response decrements its remaining `maxTurns`/`maxToolTurns`.
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
