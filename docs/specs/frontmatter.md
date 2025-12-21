# Agent Frontmatter Schema

## TL;DR
YAML frontmatter parser for agent prompt files, extracting configuration options, I/O specifications, and metadata with strict validation.

## Source Files
- `src/frontmatter.ts` - Full implementation (300 lines)
- `src/options-registry.ts` - Options registry for validation
- `js-yaml` - YAML parsing library

## Frontmatter Format

```yaml
---
description: Agent description
usage: How to use the agent
toolName: custom_tool_name

# I/O specifications
input:
  format: text | json
  schema: { ... }
  schemaRef: path/to/schema.json

output:
  format: json | markdown | text
  schema: { ... }
  schemaRef: path/to/schema.json

# Runtime options
models: anthropic/claude-3-5-sonnet
tools: mcp-server-1,mcp-server-2
agents: sub-agent-1,sub-agent-2
maxTurns: 10
maxToolCallsPerTurn: 20
maxRetries: 3
llmTimeout: 60000
toolTimeout: 30000
temperature: 0.7
topP: 0.9
maxOutputTokens: 16384
repeatPenalty: 1.0
toolResponseMaxBytes: 100000
reasoning: medium
reasoningTokens: 16000
caching: full
---

System prompt content here...
```

## Parsing Function

**Location**: `src/frontmatter.ts:30-165`

```typescript
function parseFrontmatter(
  src: string,
  opts?: { baseDir?: string; strict?: boolean }
): {
  expectedOutput?: { format: 'json'|'markdown'|'text'; schema?: Record<string, unknown> };
  inputSpec?: { format: 'text'|'json'; schema?: Record<string, unknown> };
  toolName?: string;
  options?: FrontmatterOptions;
  description?: string;
  usage?: string;
} | undefined
```

## Parsing Flow

1. **Strip shebang** (if present):
   ```typescript
   if (text.startsWith('#!')) {
     const nl = text.indexOf('\n');
     text = nl >= 0 ? text.slice(nl + 1) : '';
   }
   ```

2. **Extract YAML block**:
   ```typescript
   const m = /^---\n([\s\S]*?)\n---\n/.exec(text);
   if (m === null) return undefined;
   ```

3. **Parse YAML**:
   ```typescript
   const raw = yaml.load(m[1]);
   ```

4. **Validate keys** (strict mode):
   - Check against OPTIONS_REGISTRY allowed keys
   - Reject runtime-only keys (traceLLM, verbose, accounting, save, load, stream, targets)

5. **Parse output spec**:
   ```typescript
   if (output.format === 'json') {
     schema = loadSchemaValue(output.schema, output.schemaRef, baseDir);
   }
   ```

6. **Parse input spec** (for sub-agents):
   ```typescript
   if (input.format === 'json') {
     schema = loadSchemaValue(input.schema, input.schemaRef, baseDir);
   }
   ```

7. **Extract options** from known keys

## FrontmatterOptions

**Location**: `src/frontmatter.ts:10-28`

```typescript
interface FrontmatterOptions {
  models?: string | string[];
  tools?: string | string[];
  agents?: string | string[];
  usage?: string;
  maxTurns?: number;
  maxToolCallsPerTurn?: number;
  maxRetries?: number;
  llmTimeout?: number;
  toolTimeout?: number;
  temperature?: number;
  topP?: number;
  maxOutputTokens?: number;
  repeatPenalty?: number;
  toolResponseMaxBytes?: number;
  reasoning?: ReasoningLevel | 'none';
  reasoningTokens?: number | string;
  caching?: CachingMode;
}
```

## Schema Loading

**Location**: `src/frontmatter.ts:167-190`

```typescript
function loadSchemaValue(v: unknown, schemaRef?: string, baseDir?: string): Record<string, unknown> | undefined {
  // Try direct object
  if (typeof v === 'object' && v !== null) return v;

  // Try parsing string as JSON or YAML
  if (typeof v === 'string') {
    try { return JSON.parse(v); } catch {}
    try { return yaml.load(v); } catch {}
  }

  // Try loading from schemaRef file
  if (typeof schemaRef === 'string') {
    const resolvedPath = path.resolve(baseDir ?? process.cwd(), schemaRef);
    const content = fs.readFileSync(resolvedPath, 'utf-8');
    if (/\.json$/i.test(resolvedPath)) return JSON.parse(content);
    if (/\.(ya?ml)$/i.test(resolvedPath)) return yaml.load(content);
    // Try JSON first, then YAML
  }

  return undefined;
}
```

## Reasoning Parsing

**Location**: `src/frontmatter.ts:128-141`

```typescript
if (raw.reasoning === null) {
  options.reasoning = 'none';
} else if (typeof raw.reasoning === 'string') {
  const normalized = raw.reasoning.trim().toLowerCase();
  if (normalized === 'none' || normalized === 'unset') {
    options.reasoning = 'none';
  } else if (normalized === 'default' || normalized === 'inherit' || normalized === '') {
    // Treat as omitted - inherits from parent
  } else if (normalized === 'minimal' || normalized === 'low' || normalized === 'medium' || normalized === 'high') {
    options.reasoning = normalized;
  } else {
    throw new Error(`Invalid reasoning level '${raw.reasoning}'`);
  }
}
```

Valid values: `none`, `unset`, `default`, `inherit`, `minimal`, `low`, `medium`, `high`, `null`

## Caching Parsing

**Location**: `src/frontmatter.ts:147-154`

```typescript
if (typeof raw.caching === 'string') {
  const normalized = raw.caching.toLowerCase();
  if (normalized === 'none' || normalized === 'full') {
    options.caching = normalized;
  } else {
    throw new Error(`Invalid caching mode '${raw.caching}'`);
  }
}

## Business Logic Coverage (Verified 2025-11-16)

- **Strict option gating**: `OPTIONS_REGISTRY` marks which CLI flags are valid in frontmatter; `parseFrontmatter` rejects runtime-only keys such as `traceLLM`, `verbose`, or `accounting` even if they exist in the YAML block (`src/frontmatter.ts:50-120`).
- **Provider/model parsing**: `parsePairs` enforces `provider/model` syntax and throws immediately on malformed tokens so invalid combinations never reach session creation (`src/frontmatter.ts:210-236`).
- **Shebang + include safety**: Shebang lines are stripped before parsing and `stripFrontmatter` only returns user content after the closing `---`, preventing YAML parsers from reading prompt bodies as configuration (`src/frontmatter.ts:182-209`).
- **Schema resolution precedence**: Inline schema objects win, then embedded JSON/YAML strings, then files referenced by `schemaRef` resolved relative to the prompt directory; AJV validation happens later in `AIAgentSession` (`src/frontmatter.ts:167-190`, `src/ai-agent.ts:2022-2076`).
- **Help template parity**: `buildFrontmatterTemplate` enumerates all allowed FM options dynamically, so `ai-agent --help` always prints an up-to-date YAML skeleton that matches parser rules (`src/frontmatter.ts:224-310`).
```

Valid values: `none`, `full`

## Strict Validation

**Location**: `src/frontmatter.ts:78-96`

```typescript
if (opts?.strict !== false) {
  // Get allowed keys from options registry
  const fmAllowed = getFrontmatterAllowedKeys();
  const allowedTopLevel = new Set([
    'description', 'usage', 'output', 'input', 'toolName',
    ...fmAllowed,
  ]);

  // Reject unknown keys
  const unknownKeys = Object.keys(raw).filter((k) => !allowedTopLevel.has(k));
  if (unknownKeys.length > 0) {
    throw new Error(`Unsupported frontmatter key(s): ${unknownKeys.join(', ')}`);
  }

  // Explicitly reject runtime-only keys
  const forbidden = ['traceLLM', 'traceMCP', 'verbose', 'accounting', 'save', 'load', 'stream', 'targets'];
  const presentForbidden = forbidden.filter((k) => k in raw);
  if (presentForbidden.length > 0) {
    throw new Error(`Invalid frontmatter key(s): ${presentForbidden.join(', ')}. Use CLI flags instead.`);
  }
}
```

## Helper Functions

### stripFrontmatter
```typescript
function stripFrontmatter(src: string): string {
  return src.replace(/^---\n([\s\S]*?)\n---\n/, '');
}
```

### extractBodyWithoutFrontmatter
```typescript
function extractBodyWithoutFrontmatter(src: string): string {
  // Remove shebang
  // Strip frontmatter
  // Return body
}
```

### parseList
```typescript
function parseList(value: unknown): string[] {
  // Array: map and trim
  // String: split by comma, trim
  // Otherwise: empty array
}
```

### parsePairs
```typescript
function parsePairs(value: unknown): { provider: string; model: string }[] {
  // Parse "provider/model" pairs
  // Validate format (must have exactly one /)
  // Return array of { provider, model }
}
```

### buildFrontmatterTemplate
```typescript
function buildFrontmatterTemplate(args): Record<string, unknown> {
  // Build YAML-ready frontmatter object
  // Include all fm-allowed options from OPTIONS_REGISTRY
  // Set defaults from registry
}
```

## Allowed Top-Level Keys

### Core Keys
- `description`: Agent description text
- `usage`: Usage instructions
- `output`: Output specification
- `input`: Input specification (sub-agent tools)
- `toolName`: Custom tool name

### Option Keys (from OPTIONS_REGISTRY)
- `models`: LLM providers and models
- `tools`: Tool providers (MCP servers, REST tools)
- `agents`: Sub-agent definitions
- `maxTurns`: Max LLM turns
- `maxToolCallsPerTurn`: Max tool calls per turn
- `maxRetries`: Retry limit
- `llmTimeout`: LLM timeout in ms
- `toolTimeout`: Tool timeout in ms
- `temperature`: Sampling temperature
- `topP`: Top-p sampling
- `topK`: Top-k sampling
- `maxOutputTokens`: Max output tokens
- `repeatPenalty`: Frequency penalty
- `toolResponseMaxBytes`: Max tool response size
- `reasoning`: Reasoning level
- `reasoningTokens`: Reasoning budget
- `caching`: Cache control mode

### Forbidden Keys (runtime-only)
- `traceLLM`, `traceMCP`, `verbose`
- `accounting`, `save`, `load`
- `stream`, `targets`

## Error Handling

In strict mode (default):
- Unknown keys throw Error
- Forbidden keys throw Error
- Invalid reasoning values throw Error
- Invalid caching values throw Error
- Schema parsing failures emit `[warn] frontmatter ...` messages to stderr but execution continues (schema omitted)

In non-strict mode:
- Any parsing/validation error returns `undefined` instead of throwing
- Schema parsing failures still emit stderr warnings but parsing proceeds with `schema` omitted

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `baseDir` | Schema file resolution base |
| `strict` | Enable/disable validation (default: true) |

## Invariants

1. **YAML delimiter**: Must be `---\n...\n---\n`
2. **Shebang support**: Optional `#!` line stripped
3. **Schema resolution**: Inline > string parse > file reference
4. **Reasoning inheritance**: 'default'/'inherit' treated as omitted
5. **Key validation**: Strict mode validates all keys
6. **Type coercion**: Numbers and booleans parsed from YAML

## Test Coverage

**Phase 1**:
- Frontmatter extraction
- Schema loading (inline, string, file)
- Reasoning level parsing
- Caching mode parsing
- List parsing
- Pair parsing

**Gaps**:
- Deep schema validation
- Circular schemaRef detection
- Large frontmatter handling
- Unicode in key names
- OPTIONS_REGISTRY sync validation
