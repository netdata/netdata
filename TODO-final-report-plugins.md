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
| 20 | XML-NEXT rendering strategy | **NONCE-only input; renderer builds FINAL + META guidance** | Keeps XML-NEXT short while guaranteeing META guidance is always paired with FINAL guidance. |
| 21 | Execution autonomy contract | **Proceed autonomously; ask Costa only for design conflicts** | Required by Costa for this task; all other uncertainty must be resolved via code review and/or second opinions. |
| 22 | Test execution policy | **Phase1 + Phase2 during work; Phase3:tier1 only at end; never Phase3:tier2** | Explicit instruction for this task. |
| 23 | Mandatory review gates | **Run Claude + Codex + GLM-4.7 after major milestones and at the end** | Required unanimous consensus unless an agent fails twice. |
| 24 | Model-facing messaging quality bar | **All model-visible instructions/errors must be coherent from the model’s point of view** | Explicit risk called out by Costa; treat as a hard requirement. |

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
- `schema`
- Full system prompt instructions
- Snippet for XML-NEXT
- Snippet to be inserted into examples in final-report instructions

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

3e) XML-NEXT rendering strategy — DECIDED
Decision:
- Pass only the NONCE and plugin requirement snippets to the XML-NEXT renderer.
- The XML-NEXT renderer itself must render both FINAL and META guidance together.
- Rationale: avoid large per-turn payloads while keeping the FINAL+META pairing unconditional.

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
  // Full instructions injected in the system prompt
  systemPromptInstructions: string;
  // Snippet injected into XML-NEXT for per-turn prominence
  xmlNextSnippet: string;
  // Snippet injected into final-report examples section
  finalReportExampleSnippet: string;
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

- META is part of the final-report contract, not a tool.
- Do not add an `agent__meta` tool.
- Update XML slot recognition and parsing helpers only as needed to avoid unknown-tag failures and to pass META parsing to the transport layer.

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
- Inject `systemPromptInstructions` from each plugin into internal instructions.
- Inject `finalReportExampleSnippet` into the final-report examples section.

**File:** `src/llm-messages-xml-next.ts`

- Render FINAL and META requirements together inside XML-NEXT.
- Use NONCE-only rendering inputs plus plugin-provided `xmlNextSnippet` values.
- Do not depend on template loops/Handlebars; render via explicit string assembly aligned with current loader capabilities (`src/prompts/loader.ts:70`).

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
        systemPromptInstructions: `
Provide support request metadata using:
<ai-agent-NONCE-META plugin="support-metadata"> ... </ai-agent-NONCE-META>
Include fields:
- user_language: The language the user is writing in
- categories: Array of categories for this request
- search_terms: Relevant search terms
- response_accuracy: { score: "0-100%" }
- response_completeness: { score: "0-100%", limitations: [] }
- user_frustration: { score: "0-100%" }
`,
        xmlNextSnippet:
          'You MUST also emit <ai-agent-NONCE-META plugin="support-metadata">{...}</ai-agent-NONCE-META> with valid JSON.',
        finalReportExampleSnippet:
          'After the final report, include <ai-agent-NONCE-META plugin="support-metadata">{...}</ai-agent-NONCE-META>.',
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
10. **Plugin requirement fields must be non-empty** - empty schema/instruction/snippet is a configuration error
11. **Unknown META plugins are ignored (with logs)** - do not fail turns for extra META blocks
12. **Invalid META does not override existing valid META** - log and keep the last valid META
13. **Invalid META does not fail the turn if valid META exists by end-of-turn** - prefer the last valid META per plugin
14. **META validation failures are aggregated per turn** - one invalid-META failure reason summarizing all invalid plugins
15. **Cache META is revalidated** - missing/invalid META in cache causes a cache miss (with logs)
16. **Final-report META instructions are rendered per session** - mandatory rules and final-report prompts will be rendered with session nonce + plugin META guidance (no static constant)
17. **Plugin runtime initializes before cache checks** - cache acceptance is gated by META validation and required plugin schemas
18. **Shared META guidance builder** - a single helper (`buildMetaPromptGuidance`) renders reminders and META blocks consistently across prompts
19. **LLM message constants replaced with per-session renderers** - task-status and mandatory rules now accept META reminders instead of using static strings
20. **Phase2 context-guard thresholds must be recalibrated after META prompt expansion** - the threshold constants were tuned for ~1832 tokens but the current prompt projection is ~2198 tokens, causing the below/equal threshold scenarios to fail
    - Evidence: `src/tests/phase2-harness-scenarios/phase2-runner.ts:439`
    - Evidence: `src/tests/phase2-harness-scenarios/phase2-runner.ts:2119`
    - Evidence: `src/tests/phase2-harness-scenarios/phase2-runner.ts:2160`
21. **META blocks will be extracted and removed before the XML tool parser runs** - this prevents valid META tags from being treated as unknown/malformed XML tool calls, because the parser only accepts allowed slots + tools today
    - Evidence: `src/xml-tools.ts:115`
    - Evidence: `src/xml-tools.ts:116`
22. **Streaming must strip all META content and suppress FINAL streaming during META-only retries** - without this, headends will receive duplicate FINAL tokens after the final report is locked but META is still missing
    - Evidence: `src/session-turn-runner.ts:3746`
23. **TURN-FAILED notices must replace NONCE with the session nonce and include META reminders when plugins are required** - otherwise the model sees literal `NONCE` and ambiguous META guidance
    - Evidence: `src/llm-messages-turn-failed.ts:31`
    - Evidence: `src/llm-messages-turn-failed.ts:35`
24. **XML-NEXT must explicitly switch to META-only mode when the final report is locked** - once FINAL exists but META is missing, the model should be told that FINAL is already accepted and only the missing META blocks should be sent
    - Evidence: `src/final-report-manager.ts:154`
    - Evidence: `src/session-turn-runner.ts:1616`
    - Evidence: `src/session-turn-runner.ts:1669`
25. **Remaining turns will collapse to `currentTurn + 1` when FINAL exists but META is missing** - this mirrors existing final-report collapse behavior by allowing exactly one additional final turn for META correction
    - Evidence: `src/final-report-manager.ts:154`
    - Evidence: `src/session-turn-runner.ts:1669`
26. **Router handoff options will be suppressed during META-only mode** - once the final report is locked, advertising a handoff tool is misleading and can break completion semantics
    - Evidence: `src/llm-messages-xml-next.ts:170`
    - Evidence: `src/final-report-manager.ts:154`
27. **XML-NEXT warnings will switch to a META-only directive when FINAL is locked** - final-turn warnings currently demand a new final report/answer, which would conflict with the locked-final contract
    - Evidence: `src/llm-messages-xml-next.ts:31`
    - Evidence: `src/final-report-manager.ts:154`
28. **Malformed META wrappers will be treated as `invalid_response` with `final_meta_invalid` feedback** - this aligns META failures with the existing retry mechanism that is triggered by `lastErrorType = 'invalid_response'`
    - Evidence: `src/session-turn-runner.ts:1496`
    - Evidence: `src/session-turn-runner.ts:1698`
29. **If the session ends with FINAL present but required META missing, we will synthesize a failure final report and clear the locked FINAL** - max-turn synthesis currently triggers only when no final report exists, and the lock prevents replacement otherwise
    - Evidence: `src/session-turn-runner.ts:1986`
    - Evidence: `src/final-report-manager.ts:154`
    - Evidence: `src/final-report-manager.ts:173`
30. **Missing or invalid META will force `lastErrorType = 'invalid_response'` even if other tools succeeded** - turn success is explicitly gated on `lastErrorType !== 'invalid_response'`, so META enforcement must drive that flag
    - Evidence: `src/session-turn-runner.ts:1496`
    - Evidence: `src/session-turn-runner.ts:1689`
31. **META failures will be enforced only after a final report is committed/accepted (`hasReport()` is true)** - META slugs are critical and the turn-failed notice is limited to two reasons, so enforcing META before a valid final report can hide the real final-report error
    - Evidence: `src/final-report-manager.ts:102`
    - Evidence: `src/llm-messages-turn-failed.ts:34`
    - Evidence: `src/llm-messages-turn-failed.ts:110`
32. **META blocks will be collected during sanitization but validated after tool execution/final-report commit** - this ensures META enforcement runs with the most up-to-date final-report state
    - Evidence: `src/session-turn-runner.ts:747`
    - Evidence: `src/session-turn-runner.ts:1501`
33. **Turn-failed reasons for META slugs will be replaced with the latest reason across retries** - the missing META list can change after each attempt, and the current de-duplication logic preserves stale reasons
    - Evidence: `src/session-turn-runner.ts:2286`
    - Evidence: `src/final-report-manager.ts:146`
34. **Retry slugs will use finalization readiness (FINAL+META) instead of only FINAL presence** - `buildTurnFailureInfo(...)` currently reports `final_report_missing` whenever the final report is present but META is missing, which is now incorrect under the new contract
    - Evidence: `src/session-turn-runner.ts:2415`
    - Evidence: `src/final-report-manager.ts:158`
35. **`final_meta_missing` feedback will be emitted only after a committed final report exists** - META is part of the final-report contract, so missing-META feedback without a final report would be confusing to the model
    - Evidence: `src/final-report-manager.ts:102`
    - Evidence: `src/llm-messages-turn-failed.ts:30`
36. **META issues will block completion only when finalization is not ready** - malformed/extra META tags should be logged, but they should not prevent completion when FINAL + required META are already valid
    - Evidence: `src/xml-transport.ts:420`
    - Evidence: `src/final-report-manager.ts:158`
37. **When META enforcement fails, `lastError` will be set to the META failure summary** - `lastError` is persisted into `state.lastTurnError` and later surfaced in synthetic failure reports, so it must reflect missing/invalid META when that is the blocking condition
    - Evidence: `src/session-turn-runner.ts:1984`
    - Evidence: `src/session-turn-runner.ts:2015`
38. **`buildTurnFailureInfo(...)` will derive `final_meta_missing` from finalization readiness and `final_meta_invalid` from the current attempt’s turn-failed events** - turn-failed events are flushed at the start of each attempt, so the remaining events reflect only the latest attempt
    - Evidence: `src/session-turn-runner.ts:588`
    - Evidence: `src/session-turn-runner.ts:2415`
    - Evidence: `src/final-report-manager.ts:158`
39. **On exhaustion with FINAL present but META missing, we will finalize with a synthetic failure report even though finalization readiness is false** - normal finalization now requires `finalizationReady`, so the exhaustion path must bypass the gate to avoid returning without a report
    - Evidence: `src/session-turn-runner.ts:1702`
    - Evidence: `src/final-report-manager.ts:154`
40. **Synthetic failure reports due to missing META will set `metadata.reason = "final_meta_missing"` and include explicit META diagnostics fields** - consumers need structured fields to detect missing META and to understand which plugins were missing
    - Evidence: `src/session-turn-runner.ts:2055`
    - Evidence: `src/final-report-manager.ts:146`
41. **When the final report is locked, streaming output will be fully suppressed (not only FINAL content), and META blocks will always be stripped from streaming** - the current filter streams all content outside the FINAL wrapper, which would leak META payloads and duplicate outputs during META-only retries
    - Evidence: `src/xml-transport.ts:776`
    - Evidence: `src/final-report-manager.ts:154`
42. **When the final report is locked, we will not reset `finalReportStreamed` or the streamed-output tail between attempts** - otherwise the finalize step can re-emit the previously streamed final report after META-only retries
    - Evidence: `src/session-turn-runner.ts:3861`
    - Evidence: `src/final-report-manager.ts:154`
43. **When a synthetic failure report replaces a locked final report, the streaming dedupe state must be reset** - otherwise `finalReportStreamed === true` will suppress the synthetic failure output during finalize
    - Evidence: `src/session-turn-runner.ts:2838`
    - Evidence: `src/final-report-manager.ts:189`
44. **Streaming filters will support an explicit suppression mode and always strip META wrappers** - the filter currently has no way to block streaming during META-only retries, and it leaks META content because it streams everything outside the FINAL wrapper
    - Evidence: `src/xml-transport.ts:776`
    - Evidence: `src/session-turn-runner.ts:3746`
45. **Cache writes will be gated on finalization readiness (FINAL + required META)** - writing cache entries when META is missing would store unusable results that can never be served under the new contract
    - Evidence: `src/ai-agent.ts:1481`
    - Evidence: `src/final-report-manager.ts:158`
46. **TURN-FAILED notices will be rendered per session with nonce replacement and optional META reminders** - static TURN-FAILED text currently includes the literal `NONCE` placeholder and cannot reflect per-session plugin requirements
    - Evidence: `src/llm-messages-turn-failed.ts:31`
    - Evidence: `src/session-turn-runner.ts:585`
47. **Partial META buffers will be dropped on stream flush** - streaming can end mid-wrapper; dropping partial META prefixes avoids leaking internal META tags to headends
    - Evidence: `src/xml-transport.ts:836`
    - Evidence: `src/xml-transport.ts:903`

## Security Considerations

1. **Arbitrary code execution**: Plugins are `.js` files loaded via dynamic import. If agent files are user-controlled, this is RCE. Mitigations:
   - Document that `.ai` files must be trusted
   - Infrastructure assumption: agent/plugin files are admin-controlled and trusted

2. **Path traversal**: Paths are required to be relative (`/abs/path.js` rejected), but `..` segments are allowed by design.
   - Implication: plugins can resolve outside the agent directory.
   - Rationale: explicitly accepted risk in admin-controlled environments (Decision #6).

3. **Fail fast**: All plugin loading errors are fatal configuration errors

## Testing Requirements

1. Plugin factory pattern works
2. Plugin loading from relative paths
3. Relative path with `..` resolves and loads when the target file exists
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

1. Before runtime implementation, identify all touch points and ensure they are fully tested. Add missing tests first.
2. During the work, run `npm run lint`, `npm run build`, `npm run test:phase1`, and `npm run test:phase2` as many times as needed.
3. Run `npm run test:phase3:tier1` only at the end of the implementation.
4. Never run `npm run test:phase3:tier2`.
5. Model-facing instructions, notices, and error messages must be coherent from the model’s point of view (no developer-only assumptions).
6. After each major milestone and at the end, consult other agents for review.
7. Final review protocol: read `~/.AGENTS.md` after implementation is complete.
8. Run Claude, Codex, and GLM-4.7 in parallel for a full review, including the TODO filename in the prompt.
9. All reviewer prompts must include: "DO NOT MAKE CHANGES, DO NOT CREATE FILES, DO NOT ASK FOR PERMISSIONS. THIS IS A READ-ONLY REQUEST. PROVIDE YOUR REVIEW."
10. Show the full prompts before execution and address findings until unanimous consensus, unless a reviewer fails twice.

## Baseline Test Status (2026-01-25)

Current baseline must be green before implementation begins.

- `npm run lint` — PASS
- `npm run build` — PASS
- `npm run test:phase1` — PASS
- `npm run test:phase2` — PASS

Previously failing expectations (resolved, kept as evidence):

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

## Touchpoints & Coverage (Pre-Implementation)

Finalization and XML transport touchpoints that will be affected by the new contract (final-report + META):

1. **Early finalization on final-report only**
   - Evidence: `src/session-turn-runner.ts:1666`
   - Risk: would finalize before required META exists
2. **Final-report overwrites are allowed**
   - Evidence: `src/session-turn-runner.ts:2511`
   - Risk: answer drift + double streaming while META is missing
3. **Final-report event emission is unconditional after report acceptance**
   - Evidence: `src/session-turn-runner.ts:2741`
   - Risk: would emit `final_report` before META requirements are satisfied
4. **Final-turn tool filtering enforces final-report-only**
   - Evidence: `src/session-turn-runner.ts:2179`
   - Risk: META requirements must be treated as part of finalization, not as extra tools
5. **XML transport defines only FINAL slot and allows only final-report tool**
   - Evidence: `src/xml-transport.ts:171`
   - Evidence: `src/xml-transport.ts:203`
   - Risk: META tags will be rejected or ignored
6. **XML tool inference treats only `-FINAL` as `agent__final_report`**
   - Evidence: `src/xml-tools.ts:104`
   - Risk: META wrappers need explicit handling
7. **Malformed or unknown XML tags currently trigger turn failure**
   - Evidence: `src/xml-transport.ts:451`
   - Risk: META tags must be recognized to avoid retries
8. **Streaming filter keeps only FINAL wrapper content**
   - Evidence: `src/tests/unit/xml-final-report-strict-filter.spec.ts:10`
   - Risk: META must never stream to headends
9. **Tool instructions are injected into the system prompt**
   - Evidence: `src/ai-agent.ts:1715`
   - Risk: contract docs must be updated to match runtime behavior
10. **Agent cache finalizes immediately with cached final-report**
    - Evidence: `src/ai-agent.ts:1481`
    - Risk: META requirements must gate cache hits
11. **Agent hash does not include plugin content**
    - Evidence: `src/agent-loader.ts:753`
    - Risk: schema changes would not invalidate cache
12. **Frontmatter strict validation and manual parsing**
    - Evidence: `src/frontmatter.ts:113`
    - Evidence: `src/frontmatter.ts:175`
    - Risk: `plugins:` will be rejected even if registry allows it
13. **XML-NEXT rendering is centralized in the turn runner**
    - Evidence: `src/session-turn-runner.ts:601`
    - Evidence: `src/xml-tools.ts:145`
    - Risk: META guidance must be injected via `buildXmlNextNotice`, not slot templates
14. **Streaming filter is applied in the turn runner and only strips FINAL**
    - Evidence: `src/session-turn-runner.ts:3739`
    - Evidence: `src/xml-transport.ts:635`
    - Risk: META tags would leak to headends unless the filter is extended
15. **Max-turn synthetic final reports finalize immediately**
    - Evidence: `src/session-turn-runner.ts:1976`
    - Evidence: `src/session-turn-runner.ts:2029`
    - Risk: META requirements need explicit handling at exhaustion points
16. **FinalReportManager commit overwrites prior reports**
    - Evidence: `src/final-report-manager.ts:127`
    - Risk: conflicts with the lock-first final-report policy while waiting for META
17. **XML wrapper-as-tool detection is FINAL-only**
    - Evidence: `src/session-turn-runner.ts:854`
    - Evidence: `src/llm-providers/base.ts:2193`
    - Evidence: `src/llm-messages.ts:77`
    - Risk: META wrappers called as tools would not receive targeted guidance
18. **Tool-output extractor only understands the FINAL wrapper**
    - Evidence: `src/tool-output/extractor.ts:222`
    - Evidence: `src/tool-output/extractor.ts:447`
    - Risk: META could be lost in tool-output map/reduce flows
19. **Deterministic nonce extraction relies on the FINAL wrapper pattern**
    - Evidence: `src/llm-providers/test-llm.ts:64`
    - Evidence: `src/tests/phase2-harness-scenarios/infrastructure/harness-helpers.ts:297`
    - Risk: XML-NEXT must continue to include the FINAL wrapper example
20. **Cache payload guard has no META awareness**
    - Evidence: `src/ai-agent.ts:94`
    - Risk: plugin requirements need explicit cache gating beyond the existing guard
21. **Headends rely on `final_report` events from finalize**
    - Evidence: `src/headends/embed-headend.ts:619`
    - Evidence: `src/session-turn-runner.ts:2741`
    - Risk: the new gating must preserve a single, authoritative final report emission
22. **Contract docs contradict runtime system-prompt behavior today**
    - Evidence: `docs/specs/CONTRACT.md:509`
    - Evidence: `src/ai-agent.ts:1715`
    - Evidence: `src/tools/internal-provider.ts:229`
    - Risk: documentation must be updated in the same commit as runtime changes
23. **Streaming currently replays FINAL content on every retry**
    - Evidence: `src/xml-transport.ts:694`
    - Evidence: `src/session-turn-runner.ts:3739`
    - Risk: once FINAL is locked but META is missing, headends will receive duplicate FINAL tokens unless we suppress FINAL streaming during META-only retries
24. **Limit exhaustion does not account for required META**
    - Evidence: `src/session-turn-runner.ts:1976`
    - Evidence: `src/session-turn-runner.ts:2768`
    - Risk: forced finalization paths can report success even when required META is missing, unless we mark the result as incomplete and log the missing META explicitly
25. **Agent loading is synchronous and widely used in synchronous call sites**
    - Evidence: `src/agent-loader.ts:967`
    - Evidence: `src/tests/phase2-harness-scenarios/phase2-runner.ts:521`
    - Risk: plugin module imports must occur during session initialization in `run()` (before the first LLM turn), while agent-loader still performs synchronous path validation and plugin hashing

Coverage status (today) — existing coverage:

1. XML-final transport and parser have unit coverage, but only for FINAL slot:
   - `src/tests/unit/xml-transport.spec.ts:30`
   - `src/tests/unit/xml-tools.spec.ts:68`
2. Final-report streaming filter has unit coverage, but only for FINAL tag:
   - `src/tests/xml-final-report-filter.spec.ts:5`
   - `src/tests/unit/xml-final-report-strict-filter.spec.ts:5`
3. Deterministic harness has extensive final-report and retry coverage, but no META contract coverage yet:
   - `src/tests/phase2-harness-scenarios/phase2-runner.ts:13045`
4. System prompt enhancement with tool instructions is already covered:
   - `src/tests/phase2-harness-scenarios/phase2-runner.ts:4821`
5. XML wrapper called as a tool is already covered:
   - `src/tests/phase2-harness-scenarios/phase2-runner.ts:14594`
6. Synthetic final-report paths are covered in the harness:
   - `src/tests/phase2-harness-scenarios/phase2-runner.ts:9013`
   - `src/tests/phase2-harness-scenarios/phase2-runner.ts:11971`
7. Tool-output extraction of FINAL wrappers has unit coverage:
   - `src/tests/unit/tool-output.spec.ts:184`

Coverage gaps to close before runtime changes:

1. FinalReportManager has no direct unit coverage:
   - Evidence: no matches for `FinalReportManager` under `src/tests/unit`
2. Prompt loader / final-report instruction rendering has no direct unit coverage:
   - Evidence: no matches for `loadFinalReportInstructions` under `src/tests`
3. Multiple final-report emissions and overwrite behavior are not explicitly covered:
   - Evidence: no overwrite/multiple-final scenarios in `src/tests/phase2-harness-scenarios/phase2-runner.ts`
4. XML mismatch failure paths are only partially covered:
   - Evidence: no direct assertions for `xml_slot_mismatch` / unknown slot leftovers in `src/tests/unit/xml-transport.spec.ts`

Pre-implementation test actions:

1. Keep Phase1 and Phase2 fully green after recalibration (DONE).
2. [DONE] Add unit tests for FinalReportManager BEFORE refactoring it.
   Evidence: `src/tests/unit/final-report-manager.spec.ts:14`
3. [DONE] Add unit tests for prompt loading / final-report instruction rendering BEFORE injection changes.
   Evidence: `src/tests/unit/prompts-loader.spec.ts:11`
4. [DONE] Add unit tests for XML mismatch failure paths (unknown slot leftovers) BEFORE transport changes.
   Evidence: `src/tests/unit/xml-transport.spec.ts:146`
5. [DONE] Add a deterministic harness scenario that covers multiple final-report emissions BEFORE we change the replacement policy.
   Evidence: `src/tests/phase2-harness-scenarios/suites/final-report.test.ts:73`
6. [DONE] Run `npm run lint`, `npm run build`, `npm run test:phase1`, and `npm run test:phase2` after the pre-implementation test additions (verified on 2026-01-25).
7. [DONE] Add unit coverage for `xml_slot_mismatch` before transport changes.
   Evidence: `src/tests/unit/xml-transport.spec.ts:183`
8. [DONE] Extend prompt loader tests for format-specific branches and schema block insertion.
   Evidence: `src/tests/unit/prompts-loader.spec.ts:37` and `src/tests/unit/prompts-loader.spec.ts:48`
9. [DONE] Extend FinalReportManager unit tests for lifecycle and empty-state accessors.
   Evidence: `src/tests/unit/final-report-manager.spec.ts:34`, `src/tests/unit/final-report-manager.spec.ts:70`, and `src/tests/unit/final-report-manager.spec.ts:115`
10. [DONE] Add a harness assertion that XML-NEXT continues to expose the FINAL wrapper pattern used for nonce extraction.
   Evidence: `src/tests/unit/harness-helpers.spec.ts:17`
11. [TODO] META gating scenarios (final-report present but required META missing) will require new Phase 2 coverage during implementation, because current semantics finalize immediately on the first valid final report.
   Evidence: early finalize occurs at `src/session-turn-runner.ts:1666` and the `final_report` event is emitted during finalize at `src/session-turn-runner.ts:2741`.

## Documentation updates required

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
