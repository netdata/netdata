# TODO: Final Report Plugins

## TL;DR

Introduce a plugin system for extending final report requirements with metadata. Plugins:
- Are `.js` files relative to the `.ai` agent file (pre-compiled, not `.ts`)
- Use factory pattern for session isolation
- Each plugin gets its own `<ai-agent-NONCE-META plugin="name">` tag
- Receive session data on completion (fire and forget, errors logged)
- Can spawn new sessions without blocking the core

This replaces the current hardcoded `support_request_metadata_json_object` pattern in `neda/support-public.ai` with a generic, extensible mechanism.

## Analysis

### Current State

1. **Metadata is embedded in final report content** (`neda/support-public.ai:386-527`)
   - 140+ lines of schema and instructions in the prompt
   - Model outputs `<support_request_metadata_json_object>` inside the final report
   - Best effort - no enforcement, no validation, no retry

2. **Streaming problem**
   - Head-ends receive final report tokens as they stream
   - If we retry for missing metadata, the final report streams twice
   - Current workaround: repeat instructions loudly in prompt

3. **No separation of concerns**
   - Metadata schema is hardcoded in prompt text
   - No callback to process/store the metadata
   - No validation against schema

4. **Models ignore instructions** (proven by evidence)
   - Even with explicit "put metadata INSIDE the wrapper" instructions
   - MiniMax-M2.1 output metadata OUTSIDE the closing tag
   - Parser correctly ignored it (per current design)
   - We MUST handle any order/position

### Critical Constraints (from code review)

1. **Frontmatter strict validation** (`src/frontmatter.ts:111-126`)
   - `plugins:` is NOT in allowed keys - will be rejected
   - Must add to `src/options-registry.ts` allowedTopLevel set

2. **Build pipeline** (`tsconfig.json`)
   - Only `src/**/*` is compiled
   - `.ts` plugins next to `.ai` files won't work
   - Plugins MUST be `.js` (pre-compiled or hand-written)

3. **Session finalization** (`src/session-turn-runner.ts:1613, 1666`)
   - Session finalizes immediately when final report exists
   - Must gate finalization on META when plugins require it

4. **XML transport single-slot** (`src/xml-transport.ts:170, 202`)
   - Only `-FINAL` slot defined
   - Must add `-META` slot handling with `plugin` attribute
   - `src/xml-tools.ts:104` - tool inference only for `-FINAL`
   - `src/xml-transport.ts:451` - unrecognized XML tags → turn failure

5. **Streaming filter** (`src/xml-transport.ts:664`)
   - Currently streams everything outside FINAL tag
   - Must strip all META tags and content from stream

6. **Cache payload** (`src/ai-agent.ts:87, 1481, 1535`)
   - No META field in cache payload today
   - Cache hits skip all plugin behavior
   - Must add META to cache and call onComplete on cache hit with `fromCache: true`

7. **Agent hash** (`src/agent-loader.ts:753`)
   - Plugin content not included in agentHash
   - Plugin changes won't invalidate cache
   - Must include plugin file hashes

8. **ES Module caching** (Node.js behavior)
   - `import()` caches modules by URL
   - Multiple imports return same instance
   - Must use factory pattern for true session isolation

### Proposed Solution

1. **Separate XML wrapper per plugin**: `<ai-agent-NONCE-META plugin="name">`
   - Each plugin gets its own META tag
   - FINAL content streams to head-ends
   - META content is internal (not streamed)
   - Can retry for specific missing plugin META

2. **Two-phase parsing**
   - Streaming phase: Stream FINAL content, strip all META tags
   - Completion phase: Extract FINAL and all META tags from anywhere in response
   - No retries for "wrong order" - only for "missing"

3. **Factory pattern for plugins**
   - Plugin exports factory function, not object
   - Fresh instance created per session
   - True isolation guaranteed

4. **Hard META enforcement**
   - When plugins define requirements, all META tags are REQUIRED
   - Session does NOT finalize until all plugin META received and valid
   - Missing/invalid META triggers normal retry flow

### Feasibility Review (2026-01-25)

Status: Feasible, but NOT ready for implementation yet. A few design decisions and doc scope updates are still missing.

Evidence from the current codebase:

1. XML-NEXT ignores `slotTemplates`. Evidence: `renderXmlNext()` drops them in `src/xml-tools.ts:145`. The final wrapper is hardcoded to `NONCE-FINAL` in `src/llm-messages-xml-next.ts:219`.
2. Prompt templates do not support loops/Handlebars. Evidence: the loader uses simple `.replace(...)` calls in `src/prompts/loader.ts:70`.
3. META parsing via tool calls is not defined. Evidence: tool inference is only for `-FINAL` in `src/xml-tools.ts:104`, and non-allowed tools are dropped in `src/xml-tools.ts:114`.
4. Cache behavior is undefined when plugins are required. Evidence: cache payload has no plugin metas in `src/ai-agent.ts:88`, and agent hash has no plugin hash in `src/agent-loader.ts:753`.
5. Frontmatter parsing is manual. Evidence: options are parsed explicitly in `src/frontmatter.ts:175`, so registry updates alone will not parse `plugins`.
6. Documentation scope is larger than listed. Evidence: the contract mentions only `NONCE-FINAL` in `docs/specs/CONTRACT.md:459`, and XML transport spec also assumes final-only slots in `docs/specs/tools-xml-transport.md:10`.
7. Final-turn enforcement depends on tool filtering + XML-NEXT; system prompt instructions persist across final turns.
Evidence: system prompt is enhanced with tool instructions (`src/ai-agent.ts:1715-1717`), final-turn provider messages are not modified (`src/llm-providers/base.ts:1167-1168`), final turn filters tools to `final_report` (+ optional allowed tools) (`src/session-turn-runner.ts:2179-2187`), unknown tool attempts are tracked and cause retry slugs (`src/session-tool-executor.ts:786-792`, `src/session-turn-runner.ts:2422-2424`).

## Decisions

### Made (with rationale)

| # | Decision | Choice | Rationale |
|---|----------|--------|-----------|
| 1 | Plugin isolation mechanism | **Factory Pattern** | ES module caching prevents re-import isolation. Factory creates fresh instance per session. |
| 2 | Cache hit + plugin behavior | **Call onComplete with `fromCache: boolean`** | Plugins should run on every response served, not just generated. |
| 3 | META placement relative to FINAL | **Any order, two-phase parsing** | Models ignore instructions. Evidence: MiniMax output META outside wrapper despite explicit instruction. |
| 4 | Error handling in onComplete | **`Promise<void>`, log at WARN** | Catches sync and async errors. Fire-and-forget but observable. |
| 5 | Schema collision handling | **Separate META tags per plugin** | Each plugin has own tag `<ai-agent-NONCE-META plugin="name">`. No collision possible. |
| 6 | Symlink path traversal | **No special protection** | Invalid files rejected by import. Admin-controlled infrastructure. |
| 7 | Plugin load failure | **Fail agent startup** | Configuration error = fail fast. User can comment out plugin if needed. |
| 8 | createSession limits | **No limits** | Trust plugin author. Admin-controlled infrastructure. |
| 9 | Cache backward compatibility | **Let it crash** | onComplete errors already caught and logged. Plugin learns to handle undefined. |
| 10 | META transport path | **Out-of-band XML (1B)** | Avoids new tool surface, keeps tool filtering stable, and aligns with PR-006. |
| 11 | Schema change vs cache | **2bA: hash plugin file content** | Schema changes invalidate `agentHash`, forcing cache miss safely. |
| 12 | Cache without META | **2A: treat as cache miss** | Keeps META enforcement consistent and avoids finalization deadlocks. |
| 13 | Plugin requirements contract | **REQUIRED: schema + system prompt instructions + XML-NEXT snippet + final-report examples snippet** | META must be prominent everywhere final report is mentioned; never show final report without META guidance. |
| 14 | Model conclusion semantics | **Finalization requires BOTH final-report AND required META (0+)** | META is part of the final report contract. Final-report without META is a final-report failure and MUST retry. |
| 15 | Contract direction | **3dA: update contract to allow tool instructions in system prompt** | XML-NEXT must stay short; system prompt is the only viable place for full instructions today. |
| 16 | Instruction injection points | **3C: system prompt + XML-NEXT + final-report examples** | Omnipresence requirement: final report must never appear without META guidance. |
| 17 | Final-report replacement policy | **5A: lock first final-report while META is missing** | Prevents double streaming and answer drift during META-only retries. |
| 18 | Plugin session spawning (V1) | **4A: include `createSession` now** | Enables plugins to fan out and run follow-up sessions immediately. |
| 19 | META wrapper contract | **6A: per-plugin META wrappers** | Enables targeted retries and isolated validation per plugin. |

### Decisions (Costa)

Open decisions: 0.

1) META transport path — DECIDED: 1B (out-of-band XML)

2) Cache behavior when plugins are required but cache has no META — DECIDED: 2A (cache miss)

2b) Cache behavior when META exists but plugin schema changes — DECIDED: 2bA

3) Where to inject META instructions — DECIDED: 3C (system prompt + XML-NEXT + final-report examples)

3b) Omnipresence requirement — DECIDED
- META requirements MUST appear everywhere final report is mentioned.
- Final report guidance must NEVER appear without META guidance.
- Each plugin MUST provide:
  1. `schema`
  2. Full system prompt instructions
  3. Snippet for XML-NEXT
  4. Snippet to be inserted into examples in final-report instructions

3c) Finalization semantics — DECIDED (Costa)
- Task/model conclusion now means:
  1. `final-report`
  2. required plugin META blocks (0+)
- BOTH are required for finalization when plugins are configured.
- Final-report without META is a final-report failure and MUST retry.
- META may arrive before or after final-report, and may be delivered in different turns.
- When final-report is already captured but META is missing, retries must request META without retransmitting the final-report (avoid double-streaming to headends).

3d) Contract interpretation / update — DECIDED: 3dA
Context:
- The contract text says system prompt should not include final-report/tool instructions in XML modes.
- Evidence: `docs/specs/CONTRACT.md:509`.
- Counter-evidence in code: tool instructions are appended to the system prompt today: `src/ai-agent.ts:1715-1717`.
- Counter-evidence in code: internal provider includes final-report instructions: `src/tools/internal-provider.ts:227-234`.
Clarification:
- META is now part of the final-report contract, not a non-final tool.
Decision:
- Update the contract to explicitly allow tool instructions in the system prompt for XML modes.
- Rationale: XML-NEXT must remain short and per-turn; system prompt is the only viable place for full instructions today.

5) Final-report replacement policy when final-report exists but META is missing — DECIDED: 5A (lock first final-report)
Context:
- Final reports can be overwritten in later turns: `src/session-turn-runner.ts:2530`, `src/final-report-manager.ts:127-136`.
- Finalize emits `final_report` even when output streaming is deduped: `src/session-turn-runner.ts:2731-2742`.
Decision:
- Lock the first valid final-report once captured; ignore later final-report replacements while waiting for META.
- Rationale: prevents double streaming and answer drift; aligns with META-only retry requirement.

4) Plugin ability to spawn sessions in V1 — DECIDED: 4A (include `createSession` now)
Decision:
- Include `createSession` in the plugin context in V1.
- Rationale: plugins need the ability to fan out and run follow-up sessions immediately.

6) META wrapper contract — DECIDED: 6A (per-plugin META wrappers)
Decision:
- Use per-plugin META wrappers: `<ai-agent-NONCE-META plugin="name">...</ai-agent-NONCE-META>`.
- Rationale: enables targeted retries and isolated validation per plugin.

## Plan

### Phase 1: Core Infrastructure

#### 1.1 Plugin Interface and Types

**File:** `src/plugins/types.ts` (new)

```typescript
import type { AIAgentSessionConfig, ConversationMessage, FinalReportPayload } from '../types.js';

export interface FinalReportPluginRequirements {
  // JSON Schema for this plugin's metadata (validated independently)
  schema: Record<string, unknown>;
  // Brief instruction injected in final-report examples section
  shortInstruction: string;
  // Detailed instruction injected in system prompt
  longInstruction: string;
}

export interface FinalReportPluginContext {
  sessionId: string;
  originId: string;
  agentId: string;
  agentPath: string;
  userRequest: string;
  messages: readonly ConversationMessage[];
  finalReport: FinalReportPayload;
  pluginData: Record<string, unknown>;  // This plugin's validated META data
  fromCache: boolean;  // True if response served from cache
  // Helper to spawn sessions (fire and forget pattern)
  createSession: (config: AIAgentSessionConfig) => { run: () => Promise<unknown> };
}

// Plugin exports a factory function (not an object) for session isolation
export type FinalReportPluginFactory = () => FinalReportPlugin;

export interface FinalReportPlugin {
  name: string;

  // Called during prompt construction
  getRequirements(): FinalReportPluginRequirements;

  // Called after session completes with valid META (or on cache hit)
  // Returns Promise - errors caught and logged at WARN level
  // Plugin can spawn sessions, write to DB, etc.
  onComplete(context: FinalReportPluginContext): Promise<void>;
}
```

#### 1.2 Plugin Loader

**File:** `src/plugins/loader.ts` (new)

- Load plugins from paths relative to agent file
- **Security**: Reject paths containing `..` or absolute paths
- **Fail fast**: Missing or invalid plugins → throw Error (configuration error)
- Dynamic import `.js` files only
- **Factory pattern**: Call factory function to get plugin instance per session

```typescript
export async function loadPluginFactories(
  agentFilePath: string,
  pluginPaths: string[]
): Promise<FinalReportPluginFactory[]>;

export function createPluginInstances(
  factories: FinalReportPluginFactory[]
): FinalReportPlugin[];

// Throws on:
// - Path traversal attempt (contains '..')
// - Absolute path
// - File not found
// - Invalid export (not a function, or missing name/getRequirements/onComplete)
```

#### 1.3 Plugin Content Hashing

**File:** `src/plugins/loader.ts`

- Compute hash of plugin file content
- Used for cache invalidation when plugins change

```typescript
export function computePluginHash(pluginPaths: string[], agentDir: string): string;
```

### Phase 2: Frontmatter & Options Registry

#### 2.1 Add plugins to allowed keys

**File:** `src/options-registry.ts`

Add new option definition:
```typescript
strArrayDef({
  key: 'plugins',
  default: undefined,
  description: 'Plugin .js files for final report metadata extensions (relative to .ai file)',
  cli: { names: ['--plugins'], showInHelp: true },
  fm: { key: 'plugins', allowed: true },
  config: { path: undefined }, // Not in config files
  scope: 'agent',
  groups: [G_AGENT],
}),
```

**File:** `src/frontmatter.ts`

- Add 'plugins' to allowedTopLevel set (line ~113)
- Parse as string array

### Phase 3: XML Transport Changes

#### 3.1 Add META Slot Parsing

**File:** `src/xml-transport.ts`

Key changes:
- Add META slot template with `plugin` attribute
  ```typescript
  // For each plugin, add slot template:
  // <ai-agent-NONCE-META plugin="support-metadata">...</ai-agent-NONCE-META>
  ```
- `parseAssistantMessage()`: Parse all META slots from anywhere in response
- Extract `plugin` attribute to identify which plugin's META
- New state tracking: `{ hasFinal: boolean, pluginMetas: Map<string, unknown> }`
- New method: `getPluginMeta(pluginName: string): Record<string, unknown> | undefined`

**File:** `src/xml-tools.ts`

- Parse META slots with plugin attribute
- Infer tool as `agent__meta` for META slots

#### 3.2 Two-Phase Parsing

**Streaming phase** (real-time):
- Stream FINAL content to headend
- Strip all `<ai-agent-NONCE-META ...>` tags and content (don't stream)
- Best-effort, optimized for user seeing answer fast

**Completion phase** (after response finishes):
- Parse full response buffer
- Extract FINAL from anywhere
- Extract all META tags from anywhere (any order: before, after, inside, interleaved)
- Match each META to plugin by `plugin` attribute
- Validate each against its plugin's schema

#### 3.3 Streaming Filter Updates

**File:** `src/xml-transport.ts`

- `XmlFinalReportFilter`: Strip all META tags and content
  - Detect `<ai-agent-NONCE-META` opening
  - Consume until `</ai-agent-NONCE-META>` closing
  - Handle multiple META tags
- `XmlFinalReportStrictFilter`: Same treatment

### Phase 4: Session Integration

#### 4.1 Finalization Gating

**File:** `src/session-turn-runner.ts`

Critical change at line 1613:
```typescript
// Before: const finalReportReady = this.finalReport !== undefined;
// After:
const finalReportReady = this.finalReport !== undefined &&
  this.allPluginMetasPresent();
```

- Track which plugins require META
- Gate finalization on FINAL + all plugin METAs present
- Add turn failure slugs for missing/invalid plugin META

#### 4.2 Plugin Management in Session

**File:** `src/session-turn-runner.ts`

- Create plugin instances at session start (factory pattern)
- Collect requirements from all plugins
- Inject plugin instructions into prompts
- Validate each plugin's META against its schema
- Call `plugin.onComplete()` for each plugin (fire-and-forget, log errors)

```typescript
// Fire-and-forget with error logging
for (const plugin of plugins) {
  plugin.onComplete({
    ...context,
    pluginData: pluginMetas.get(plugin.name) ?? {},
    fromCache: false,
  }).catch(err => {
    warn(`Plugin ${plugin.name} onComplete failed: ${err}`);
  });
}
```

#### 4.3 Prompt Injection

**File:** `src/tools/internal-provider.ts`

Injection point at `getInstructions()` (line 212):
- Inject `longInstruction` from each plugin into internal instructions
- Inject `shortInstruction` into final-report examples section

**File:** `src/prompts/final-report.md`

Add META wrapper instructions when plugins active:
```markdown
## Metadata Requirements

In addition to your final report, you MUST provide metadata for each plugin:

{{#each plugins}}
<ai-agent-{{{../nonce}}}-META plugin="{{{name}}}">
{ ...{{{name}}} metadata JSON... }
</ai-agent-{{{../nonce}}}-META>

{{/each}}

Each metadata block can appear before, after, or around your final report.
```

#### 4.4 Turn Failure Slugs

**File:** `src/llm-messages-turn-failed.ts`

Add new slugs:
```typescript
meta_missing_plugin: {
  message: 'Your response is missing metadata for plugin "{plugin}". Provide: <ai-agent-NONCE-META plugin="{plugin}">...</ai-agent-NONCE-META>',
  priority: 'high',
},
meta_not_json: {
  message: 'Metadata for plugin "{plugin}" is not valid JSON.',
  priority: 'high',
},
meta_schema_invalid: {
  message: 'Metadata for plugin "{plugin}" failed schema validation: {details}',
  priority: 'high',
},
meta_truncated: {
  message: 'Metadata for plugin "{plugin}" appears truncated.',
  priority: 'high',
},
```

### Phase 5: Final Report Manager Updates

#### 5.1 Meta Tracking

**File:** `src/final-report-manager.ts`

- Add `pluginMetas: Map<string, Record<string, unknown>>` field
- Add `hasPluginMeta(pluginName: string): boolean` method
- Add `getPluginMeta(pluginName: string): Record<string, unknown> | undefined` method
- Add `commitPluginMeta(pluginName: string, meta: Record<string, unknown>): void` method
- Add `validatePluginMeta(pluginName: string, schema: Record<string, unknown>): ValidationResult`
- Add `allPluginMetasPresent(requiredPlugins: string[]): boolean` method

### Phase 6: Cache Updates

#### 6.1 Cache Payload

**File:** `src/ai-agent.ts`

Update `AgentCachePayload` interface (line 87):
```typescript
interface AgentCachePayload {
  finalReport?: AIAgentResult['finalReport'];
  conversation: ConversationMessage[];
  childConversations?: AIAgentResult['childConversations'];
  pluginMetas?: Record<string, Record<string, unknown>>;  // NEW - keyed by plugin name
}
```

Update cache hit handling (line 1535):
- Extract plugin METAs from cache
- Call each `plugin.onComplete()` with `fromCache: true`
- Errors caught and logged (existing error handling)

```typescript
// Cache hit - call plugins with fromCache: true
for (const plugin of plugins) {
  const pluginData = cachedPayload.pluginMetas?.[plugin.name];
  plugin.onComplete({
    ...context,
    pluginData: pluginData ?? {},
    fromCache: true,
  }).catch(err => {
    warn(`Plugin ${plugin.name} onComplete failed (cache hit): ${err}`);
  });
}
```

#### 6.2 Agent Hash

**File:** `src/agent-loader.ts`

Update `agentHashPayload` (line 753):
```typescript
const agentHashPayload = {
  prompt: promptContent,
  config,
  targets: selectedTargets,
  tools: selectedTools,
  agents: effAgents,
  plugins: pluginHash,  // NEW - hash of plugin file contents
  // ... rest
};
```

### Phase 7: Migration

#### 7.1 Create support-metadata Plugin

**File:** `neda/support-metadata.js` (new - JavaScript, not TypeScript)

```javascript
// neda/support-metadata.js
// Factory pattern - exports function that returns plugin instance

export default function createSupportMetadataPlugin() {
  return {
    name: 'support-metadata',

    getRequirements() {
      return {
        schema: {
          type: 'object',
          properties: {
            user_language: { type: 'string' },
            categories: { type: 'array', items: { type: 'string' } },
            search_terms: { type: 'array', items: { type: 'string' } },
            response_accuracy: {
              type: 'object',
              properties: { score: { type: 'string' } },
            },
            // ... rest of schema from support-public.ai
          },
          required: ['user_language', 'categories'],
        },
        shortInstruction: 'Include <ai-agent-NONCE-META plugin="support-metadata">...</ai-agent-NONCE-META>',
        longInstruction: `
Provide support request metadata with:
- user_language: The language the user is writing in
- categories: Array of categories for this request
- search_terms: Relevant search terms
- response_accuracy: { score: "0-100%" }
- response_completeness: { score: "0-100%", limitations: [] }
- user_frustration: { score: "0-100%" }
`,
      };
    },

    async onComplete(context) {
      // Fire and forget - log to analytics, DB, etc.
      console.log(`Support metadata (fromCache=${context.fromCache}):`, context.pluginData);
      // Could spawn follow-up session:
      // const session = context.createSession({ ... });
      // await session.run();
    },
  };
}
```

#### 7.2 Update support-public.ai

**File:** `neda/support-public.ai`

- Remove 140+ lines of metadata instructions
- Add `plugins: [support-metadata.js]` to frontmatter
- Simplify prompt significantly

### Phase 8: Testing

#### 8.1 Unit Tests

- `src/tests/unit/plugin-loader.spec.ts`:
  - Plugin loading from relative paths
  - Factory pattern validation
  - Path traversal rejection
  - Absolute path rejection
  - Missing file → error
  - Invalid export (not function) → error
  - Invalid plugin (missing name/getRequirements/onComplete) → error
- `src/tests/unit/xml-transport-meta.spec.ts`:
  - META slot parsing with plugin attribute
  - Multiple META tags extraction
  - META in any position (before/after/inside FINAL)
  - META stripped from streaming

#### 8.2 Integration Tests

- Plugin factory creates fresh instance per session
- Each plugin gets its own META tag
- Each plugin's META validated against its schema
- Retry on missing plugin META (specific feedback)
- Retry on invalid plugin META (schema errors)
- onComplete called for each plugin
- onComplete errors logged but don't fail session
- Cache hit calls onComplete with `fromCache: true`
- Session spawning from plugin works
- Streaming: FINAL streams, all META stripped

#### 8.3 Phase 2 Harness Scenarios

- `src/tests/phase2-harness-scenarios/suites/plugins.test.ts`
- Scenarios:
  - Single plugin, META present and valid
  - Single plugin, META missing
  - Single plugin, META invalid schema
  - Multiple plugins, all META present
  - Multiple plugins, one META missing
  - META before FINAL
  - META after FINAL
  - META inside FINAL (interleaved)
  - Cache hit with plugin METAs
  - Cache hit with old entry (no METAs) - errors logged

## Implied Decisions

1. **Plugins use factory pattern** - `export default function()` not `export default {}`
2. **Each plugin has its own META tag** - `<ai-agent-NONCE-META plugin="name">`
3. **META can appear anywhere** - before, after, inside FINAL - all valid
4. **Plugins are JavaScript files** - `.js` only, not `.ts` (outside build scope)
5. **onComplete returns Promise<void>** - errors caught and logged at WARN
6. **fromCache boolean in context** - plugin knows if serving cached response
7. **Plugin errors don't fail session** - logged but session continues
8. **No symlink protection** - invalid files rejected by import anyway
9. **No createSession limits** - trust plugin author

## Security Considerations

1. **Arbitrary code execution**: Plugins are `.js` files loaded via dynamic import. If agent files are user-controlled, this is RCE. Mitigations:
   - Document that `.ai` files must be trusted
   - Path restrictions prevent escaping agent directory

2. **Path traversal**: Reject any path containing `..` or starting with `/`

3. **Fail fast**: All plugin loading errors are fatal configuration errors

## Testing Requirements

1. Plugin factory pattern works
2. Plugin loading from relative paths
3. Path traversal rejection (paths with `..`)
4. Absolute path rejection
5. Missing plugin file → startup error
6. Invalid plugin export (not function) → startup error
7. Invalid plugin (missing methods) → startup error
8. Each plugin gets own META tag with `plugin` attribute
9. META extraction from any position in response
10. META stripped from streaming (all plugin tags)
11. Each plugin's META validated against its schema
12. Retry for missing specific plugin META
13. Retry for invalid plugin META (schema errors)
14. onComplete called for each plugin
15. onComplete errors logged at WARN
16. fromCache flag correctly set
17. Session spawning from plugin works
18. Cache hit calls onComplete with fromCache=true
19. Old cache entries (no META) - error logged, continues
20. Plugin content change invalidates cache

## Execution Contract (Costa)

1. Before implementation, identify all touch points and ensure they are fully tested. Add missing tests first.
2. Run `npm run test:phase1` and `npm run test:phase2` as many times as needed during the work.
3. Run `npm run test:phase3:tier1` only at the end of the implementation.
4. Never run `npm run test:phase3:tier2`.
5. Model-facing instructions, notices, and error messages must be coherent from the model’s point of view (no developer-only assumptions).
6. After each major milestone and at the end, consult other agents for review.
7. Final review protocol (required):
8. Read `~/.AGENTS.md`.
9. Run Claude, Codex, and GLM-4.7 in parallel for a full review using the TODO filename in the prompt.
10. Instruction for all reviewers: "DO NOT MAKE CHANGES, DO NOT CREATE FILES, DO NOT ASK FOR PERMISSIONS. THIS IS A READ-ONLY REQUEST. PROVIDE YOUR REVIEW."
11. Address findings until unanimous consensus, unless an agent repeatedly fails after a second attempt.

## Baseline Test Status (2026-01-25)

Current baseline must be green before implementation begins.

- `npm run lint` — PASS
- `npm run build` — PASS
- `npm run test:phase1` — PASS
- `npm run test:phase2` — PASS

Concrete failing expectations (evidence):

1. `context_guard__threshold_below_limit` expects no warnings:
   - `src/tests/phase2-harness-scenarios/phase2-runner.ts:2143`
2. `context_guard__threshold_equal_limit` expects no warnings:
   - `src/tests/phase2-harness-scenarios/phase2-runner.ts:2184`
3. Additional failures reported by the runner:
   - `context_guard__tool_drop_after_success`
   - `context_guard__progress_passthrough`
   - `context_guard__init_counters_from_history`
   - `run-test-context-token-double-count`

Observed runtime evidence from dump:

- Warning logged even for the below-threshold probe:
  - Message: "Context limit exceeded during turn execution; proceeding with final turn."
  - Details: `projected_tokens=1832`, `limit_tokens=1029`

Root cause (evidence-backed):

1. The initial prompt is now larger than the context windows used in these scenarios.
   - Threshold scenarios configure very small windows:
     - `src/tests/phase2-harness-scenarios/phase2-runner.ts:440-442`
   - Other failing context-guard scenarios also use tight windows:
     - `src/tests/phase2-harness-scenarios/phase2-runner.ts:2295-2306`
     - `src/tests/phase2-harness-scenarios/phase2-runner.ts:2359-2371`
     - `src/tests/phase2-harness-scenarios/phase2-runner.ts:2692-2704`
     - `src/tests/phase2-harness-scenarios/phase2-runner.ts:2762-2774`
2. The final-report prompt is large enough to exceed those windows by itself.
   - `src/prompts/final-report.md` is 117 lines / 6487 bytes.
   - Deterministic harness logs show:
     - `expected_tokens=1832` for the threshold probe
     - `expected_tokens=2895-2896` for other context-guard scenarios
3. Because the prompt already exceeds the limit, the context guard forces a final turn preflight, so tools do not execute and tests fail for the wrong reason.

Resolution (2026-01-25):

- Threshold windows recalibrated: `src/tests/phase2-harness-scenarios/phase2-runner.ts:439`
- Context guard window recalibrated: `src/tests/phase2-harness-scenarios/phase2-runner.ts:2295`
- Context guard window recalibrated: `src/tests/phase2-harness-scenarios/phase2-runner.ts:2361`
- Context guard window recalibrated: `src/tests/phase2-harness-scenarios/phase2-runner.ts:2503`
- Context guard window recalibrated: `src/tests/phase2-harness-scenarios/phase2-runner.ts:2694`
- Context guard window recalibrated: `src/tests/phase2-harness-scenarios/phase2-runner.ts:2764`
- Metrics assertion updated to match configured window: `src/tests/phase2-harness-scenarios/phase2-runner.ts:2547`
- Budget truncation scenario recalibrated to trigger after tool execution: `src/tests/phase2-harness-scenarios/phase2-runner.ts:10689`

Gating tasks status:

- [DONE] Identify root cause of the Phase2 baseline failures.
- [DONE] Restore a fully green baseline (Phase1 + Phase2) before feature implementation.
- [DONE] Recalibrate context-window test constants to current prompt size so the baseline is meaningful again.

## Documentation Updates

1. **docs/specs/DESIGN.md**: Add plugin system section
2. **docs/specs/IMPLEMENTATION.md**: Plugin interface, factory pattern, loading, lifecycle
3. **docs/specs/tools-final-report.md**: META wrapper per plugin, two-phase parsing
4. **docs/specs/AI-AGENT-INTERNAL-API.md**: Plugin context, onComplete, fromCache
5. **docs/specs/retry-strategy.md**: META-related retry slugs (per plugin)
6. **README.md**: Plugin usage example in frontmatter
7. **docs/skills/ai-agent-guide.md**: Plugin configuration

## Files Summary

### New Files
- `src/plugins/types.ts`
- `src/plugins/loader.ts`
- `neda/support-metadata.js`
- `src/tests/unit/plugin-loader.spec.ts`
- `src/tests/unit/xml-transport-meta.spec.ts`
- `src/tests/phase2-harness-scenarios/suites/plugins.test.ts`

### Modified Files
- `src/options-registry.ts` - Add plugins option
- `src/frontmatter.ts` - Add plugins to allowedTopLevel
- `src/types.ts` - Export plugin types
- `src/xml-transport.ts` - META slot parsing (per plugin), streaming filter
- `src/xml-tools.ts` - META slot with plugin attribute
- `src/final-report-manager.ts` - Plugin meta tracking
- `src/session-turn-runner.ts` - Plugin integration, finalization gating
- `src/tools/internal-provider.ts` - Instruction injection per plugin
- `src/llm-messages-turn-failed.ts` - Per-plugin META slugs
- `src/ai-agent.ts` - Cache payload with pluginMetas, cache hit handling
- `src/agent-loader.ts` - Plugin hash in agentHash
- `neda/support-public.ai` - Use plugin, remove hardcoded metadata
- `src/prompts/final-report.md` - META wrapper instructions per plugin

## Estimated Effort

| Phase | Effort |
|-------|--------|
| Phase 1: Core Infrastructure | 1.5 days |
| Phase 2: Frontmatter & Options | 0.5 day |
| Phase 3: XML Transport (two-phase, multi-META) | 1.5 days |
| Phase 4: Session Integration | 1.5 days |
| Phase 5: Final Report Manager | 0.5 day |
| Phase 6: Cache Updates | 0.5 day |
| Phase 7: Migration | 0.5 day |
| Phase 8: Testing | 2 days |

**Total: ~8.5 days**
