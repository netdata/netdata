# Multi-Agent Workflow Orchestration Design

## TL;DR
- Add 4 orchestration patterns: **Handoff**, **Router**, **Advisors**, **Team**
- Patterns are frontmatter-driven (decentralized, each agent knows only its immediate needs)
- Orchestration layer wraps existing session layer (sessions stay pure)
- `AIAgentSession.run()` becomes recursive: checks frontmatter, applies patterns, calls child sessions
- Terminal vs. non-terminal agents: only terminal agents own their final answer
- Team pattern still needs detailed design (most complex, requires coordination hooks)

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

#### 2. Router (`router: true` or `route_to` tool)
**What**: Single-turn agent that selects which agent handles the request. No tools/subagents except `route_to`.
**Real-world**: Receptionist, triage, dispatcher.
**Why**: Cost savings, intelligent routing without full orchestration overhead.
**Terminal**: Non-terminal (delegates entire request).

```yaml
# frontmatter
router: true
agents:
  - legal-department
  - technical-support
  - billing
```

**Execution flow**:
```
agent.run(input)
  → session.start(input)  // single turn, calls route_to tool
  → selected_agent = from tool call
  → selected_agent.run(input)  // recursive!
  → return selected_agent's result
```

**Tool schema**:
```typescript
{
  name: "route_to",
  description: "Select the agent to handle this request",
  parameters: {
    agent: { type: "string", enum: [...available_agents] },
    reason: { type: "string" }
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
**What**: Multiple agents collaborate with shared message history. Each maintains independent session but broadcasts to common channel.
**Real-world**: Code review panel, planning committee, stakeholder alignment.
**Why**: Extreme cost savings (impractical to transfer full discussions via tool calls), parallel deliberation.
**Terminal**: Coordinator is terminal (makes final decision).

```yaml
# frontmatter
team:
  members:
    - analyst
    - developer
    - security-reviewer
  terminationMode: consensus | maxRounds | coordinatorDecides
  maxRounds: 10
```

**Execution model**: See "Team Pattern - Detailed Design Needed" section below.

### Terminal vs. Non-Terminal Agents

**Critical concept**: Only terminal agents own their final answer.

| Pattern | Terminal? | Why |
|---------|-----------|-----|
| Standard orchestrator | Yes | Owns its final answer |
| Has `handoff` | No | Delegates final answer to next agent |
| Router | No | Delegates entire request to selected agent |
| Has `advisors` | Yes | Makes final decision (advisors only advise) |
| Team coordinator | Yes | Synthesizes team discussion into final answer |
| Team member (non-coordinator) | No | Contributes to discussion, doesn't own final answer |

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
    if (this.config.router && result.routedTo) {
      return this.runRouter(result.routedTo);
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
  router?: boolean;                    // Is this a routing agent?
  advisors?: string[];                 // Agents to consult before execution
  team?: {
    members: string[];                 // Team member agents
    terminationMode: 'consensus' | 'maxRounds' | 'coordinatorDecides';
    maxRounds?: number;
  };
}
```

**Validation rules**:
- `router: true` incompatible with `subagents` (router only routes, no tools)
- `team` coordinator cannot have `handoff` (team owns final answer)
- Cycle detection: handoff chains, team membership (no agent can be ancestor of itself)
- Advisors cannot be terminal (they don't have `final_report`)

## Decisions Needed

### 1. Naming Confirmation
- ~~`handoff` vs `next` vs `delegate`~~ **DECIDED: `handoff`**
- ~~`router` vs `dispatch` vs `triage`~~ **DECIDED: `router`**
- ~~`consult` vs `advisors` vs `gather`~~ **DECIDED: `advisors`**
- ~~`team` vs `panel` vs `council`~~ **DECIDED: `team`**

### 2. Router Tool Semantics
- Single `route_to` tool, or multiple options?
- Can router have other tools (analysis), or pure routing only?
- How to handle routing failures (no suitable agent)?

### 3. Advisor Failure Handling
- All advisors must succeed, or partial results acceptable?
- Timeout per advisor?
- Error propagation vs. graceful degradation?

### 4. Team Pattern (Major Design Work)
See section below.

### 5. Accounting Aggregation
- How do parallel advisor costs aggregate?
- How does handoff chain appear in opTree?
- Team coordination overhead tracking?

### 6. Frontmatter Validation
- Load-time vs. runtime validation of orchestration patterns?
- Cycle detection algorithm (DFS for handoff chains)?
- Schema validation for pattern compatibility?

## Team Pattern - Detailed Design Needed

This is the most complex pattern. Key open questions:

### 1. Message Broadcasting
- **What gets broadcast?**: Only `final_report` calls? All assistant messages? Only marked messages?
- **Broadcast tool**: New `broadcast(message)` tool, or reuse `progress_report`?
- **Injection mechanism**: How to inject broadcast messages into member sessions?

**Current thinking**: Use `progress_report` for broadcasts. Add message queue that agent loop checks before each turn.

### 2. Session Coordination
- **Parallel async**: All members run simultaneously, blocked waiting for broadcasts
- **Turn-based**: Members take turns speaking (simpler, but loses parallelism)
- **Hybrid**: Parallel execution with synchronization points

### 3. Termination Condition
- **Consensus voting**: All members call `vote_conclude()` tool
- **Coordinator decides**: One member is coordinator, calls `conclude_team()`
- **Max rounds**: Forced termination after N rounds

### 4. Who Is Coordinator?
- **External orchestrator**: Code, not LLM agent (simplest)
- **Designated member**: One team member marked as coordinator
- **Emergent**: Last agent to vote becomes coordinator

**Current thinking**: Designated coordinator member. Coordinator has `final_report`, other members only have `broadcast` and `vote_conclude`.

### 5. Hook Points Needed
```typescript
interface TeamOrchestrator {
  onMemberBroadcast(memberId: string, message: string): void;
  onMemberVote(memberId: string, ready: boolean): void;
  injectMessage(memberId: string, message: string): void;
  shouldContinue(): boolean;
}
```

### 6. Blocking Model
- Members wait for new broadcasts before continuing?
- Or run continuously, checking for new messages each turn?
- Or event-driven (wake on broadcast)?

## Plan

### Phase 1: Foundation
1. Extend frontmatter parsing to accept `handoff`, `router`, `consult`, `team` fields
2. Add validation: cycle detection, pattern compatibility checks
3. Implement `isTerminal` flag computation based on frontmatter
4. Add orchestration layer wrapper in `AIAgentSession.run()`

### Phase 2: Handoff Pattern
1. Implement `runHandoff()` in AIAgentSession
2. Chain resolution at load time (validate references exist)
3. Accounting aggregation for handoff chains
4. Update logging/tracing to show handoff flow

### Phase 3: Advisors Pattern
1. Implement `runAdvisors()` with parallel execution
2. Message enrichment formatting
3. Advisor failure handling
4. Accounting aggregation for parallel advisors

### Phase 4: Router Pattern
1. Implement router agent type (single-turn, `route_to` tool only)
2. Router execution flow in `AIAgentSession.run()`
3. Routing failure handling
4. Validation: router cannot have subagents

### Phase 5: Team Pattern (Requires Separate Design Document)
1. Design message broadcasting infrastructure
2. Implement coordination hooks
3. Add termination logic
4. Test with simple 2-agent team
5. Scale to N-agent teams

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

## Next Steps

1. ~~**Costa to decide**: Confirm naming choices~~ **DONE: team, handoff, router, advisors**
2. **Costa to decide**: Router semantics (pure routing vs. with analysis tools)
3. **Costa to decide**: Advisor failure handling strategy
4. **Detailed team design**: Requires separate focused discussion on hooks, broadcasting, coordination
5. **Implementation order**: Recommend Handoff → Advisors → Router → Team (increasing complexity)
