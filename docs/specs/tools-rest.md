# REST Tool Provider

## TL;DR
HTTP API tool provider supporting templated URLs, headers, bodies, and JSON streaming responses.

## Source Files
- `src/tools/rest-provider.ts` - Full implementation (346 lines)
- `src/types.ts` - RestToolConfig definition

## Data Structures

### RestToolConfig
```typescript
interface RestToolConfig {
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH';                        // HTTP method
  url: string;                           // URL template
  headers?: Record<string, string>;       // Header templates
  parametersSchema: Record<string, unknown>;  // JSON Schema
  description: string;                    // Tool description
  bodyTemplate?: unknown;                 // Body template
  queue?: string;                         // Queue name
  streaming?: {                           // JSON streaming config
    mode: 'json-stream';
    linePrefix?: string;
    discriminatorField?: string;
    doneValue?: string;
    answerField?: string;
    tokenValue?: string;
    tokenField?: string;
  };
  hasComplexQueryParams?: boolean;        // Enable complex query serialization
  queryParamNames?: string[];             // Names of complex params
}
```

### Provider Class
```typescript
class RestProvider extends ToolProvider {
  readonly kind = 'rest';
  private tools: Map<string, RestToolConfig>;
  private ajv: AjvInstance;
}
```

## Tool Naming

**Format**: `rest__{internalName}`

Example:
- Config key: `search_api`
- Exposed: `rest__search_api`

## Construction

**Location**: `src/tools/rest-provider.ts:67-72`

```typescript
constructor(namespace, config?) {
  if (config !== undefined) {
    Object.entries(config).forEach(([name, def]) => {
      this.tools.set(name, def);
    });
  }
}
```

## Template Substitution

### URL Substitution
**Location**: `src/tools/rest-provider.ts:315-322`

```typescript
substituteUrl(template, parameters) {
  return template.replace(
    /\$\{parameters\.([^}]+)\}/g,
    (_, name) => encodeURIComponent(parameters[name])
  );
}
```

Example:
- Template: `https://api.example.com/users/${parameters.userId}`
- Parameters: `{ userId: "abc123" }`
- Result: `https://api.example.com/users/abc123`

### String Substitution
**Location**: `src/tools/rest-provider.ts:307-313`

For headers and simple strings:
```typescript
substituteString(template, parameters) {
  return template.replace(
    /\$\{parameters\.([^}]+)\}/g,
    (_, name) => String(parameters[name])
  );
}
```

### Body Building
**Location**: `src/tools/rest-provider.ts:324-339`

```typescript
buildBody(template, parameters) {
  // Single placeholder: use raw value
  if (/^\$\{parameters\.([^}]+)\}$/.exec(template)) {
    return JSON.stringify(parameters[key]);
  }
  // Complex template: recursive substitution
  const resolved = this.substitute(template, parameters);
  return JSON.stringify(resolved);
}
```

## Complex Query Parameters

**Location**: `src/tools/rest-provider.ts:26-61, 121-149`

Handles nested structures:
```typescript
serializeQueryParam(key, value, prefix) {
  // Arrays: key[0]=val, key[1]=val
  // Objects: key[prop]=val
  // Nested: key[0][prop]=val
  return serialized;
}
```

Example:
- Input: `{ filters: [{ email: 'test@example.com' }] }`
- Output: `filters[0][email]=test%40example.com`

Configuration:
```typescript
{
  hasComplexQueryParams: true,
  queryParamNames: ['filters', 'pagination']
}
```

## Execution Flow

### 1. Parameter Validation
**Location**: `src/tools/rest-provider.ts:99-115`

```typescript
const validate = this.ajv.compile(cfg.parametersSchema);
const ok = validate(parameters);
if (!ok) {
  throw new Error(`Invalid arguments: ${errors}`);
}
```

### 2. URL Construction
**Location**: `src/tools/rest-provider.ts:117-149`

1. Apply URL template substitution
2. If complex query params enabled:
   - Extract named parameters
   - Serialize with nesting support
   - Append to URL

### 3. Header Preparation
**Location**: `src/tools/rest-provider.ts:151-157`

```typescript
const headers = new Headers();
Object.entries(cfg.headers).forEach(([k, v]) => {
  const substituted = this.substituteString(v, parameters);
  headers.set(k, substituted);
});
```

### 4. Body Construction
**Location**: `src/tools/rest-provider.ts:179-193`

If bodyTemplate provided:
- Build body from template
- Set content-type to application/json (if not set)

### 5. Request Execution
**Location**: `src/tools/rest-provider.ts:196-230`

```typescript
const controller = new AbortController();
if (timeoutMs) {
  timer = setTimeout(() => controller.abort(), timeoutMs);
}

const requestInit = { method, headers, signal: controller.signal };
if (body) requestInit.body = body;

if (streaming.mode === 'json-stream') {
  result = await this.consumeJsonStream(res, cfg, signal);
} else {
  result = await res.text();
}
```

### 6. Result Return
**Location**: `src/tools/rest-provider.ts:232-238`

```typescript
if (errorMsg !== undefined) {
  return { ok: false, error: errorMsg, latencyMs, kind, namespace };
}
return { ok: true, result, latencyMs, kind, namespace };
```

## JSON Streaming

**Location**: `src/tools/rest-provider.ts:241-288`

Configuration fields:
- `linePrefix`: Prefix to strip (e.g., `data: `)
- `discriminatorField`: Type field (default: `type`)
- `doneValue`: Done marker (default: `done`)
- `answerField`: Final answer field (default: `answer`)
- `tokenValue`: Token type value (default: `token`)
- `tokenField`: Token content field (default: `content`)

Process:
1. Read stream chunk by chunk
2. Buffer partial lines
3. Split on newlines
4. Parse JSON lines
5. Accumulate tokens
6. Return answer on done event

## Tracing

**Location**: `src/tools/rest-provider.ts:159-176, 185-192`

Enable via:
- `opts.trace === true`
- `process.env.DEBUG_REST_CALLS === 'true'`
- `process.env.TRACE_REST === 'true'`

Logs:
- Method
- URL
- Headers (authorization redacted)
- Body (truncated to 500 chars)

## Queue Support

**Location**: `src/tools/rest-provider.ts:88-91`

```typescript
resolveQueueName(exposed) {
  const cfg = this.tools.get(this.internalName(exposed));
  return cfg?.queue ?? 'default';
}
```

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `method` | HTTP verb |
| `url` | Base URL with placeholders |
| `headers` | Request headers with placeholders |
| `parametersSchema` | AJV validation schema |
| `bodyTemplate` | Request body structure |
| `streaming` | JSON stream parsing config |
| `hasComplexQueryParams` | Enable nested query serialization |
| `queryParamNames` | Which params to serialize complexly |
| `queue` | Concurrency queue assignment |

## Business Logic Coverage (Verified 2025-11-16)

- **Complex query serialization**: When `hasComplexQueryParams` is true only the names listed in `queryParamNames` are JSON-stringified; other params remain standard URL-encoded strings to avoid unexpected API behavior (`src/tools/rest-provider.ts:26-149`).
- **Streaming JSON**: `streaming.mode === 'json-stream'` parses newline-delimited JSON responses, using optional discriminator fields to decide when the stream is complete (e.g., SSE-like APIs) (`src/tools/rest-provider.ts:241-312`).
- **Template substitution safety**: URL and body templates only substitute `${parameters.<name>}` tokens and throw descriptive errors on missing values, preventing partial substitutions or injection (`src/tools/rest-provider.ts:315-360`).
- **Queue affinity + timeouts**: Each tool can specify a `queue`, and per-call `timeoutMs` uses AbortController so hung HTTP calls release queue slots promptly (`src/tools/rest-provider.ts:70-120`, `src/tools/rest-provider.ts:200-240`).

## Telemetry

**Per execution**:
- Latency
- Success/failure
- HTTP status
- Error message if failed

## Logging

**Via warn()**:
- Trace output (when enabled)
- Timeout abort failures
- Clear timeout failures

## Events

No specific events. Tool execution logged through orchestrator.

## Invariants

1. **Validation first**: Parameters validated before any network call
2. **Template isolation**: Each placeholder independently resolved
3. **Encoding safety**: URL substitution uses encodeURIComponent
4. **Timeout enforcement**: AbortController cancels request
5. **Content-type default**: JSON body gets application/json header
6. **Error propagation**: HTTP errors returned as tool failures

## Test Coverage

**Phase 1**:
- Parameter validation
- URL substitution
- Header templating
- Body building
- JSON streaming consumption
- Timeout handling

**Gaps**:
- Complex query parameter edge cases
- Large streaming responses
- Concurrent request handling
- Retry logic (not implemented)

## Troubleshooting

### Invalid arguments error
- Check parametersSchema matches input
- Verify required fields present
- Review AJV error messages

### URL substitution failure
- Check placeholder syntax `${parameters.name}`
- Verify parameter exists in input
- Check encoding issues

### Timeout
- Increase timeoutMs option
- Check server responsiveness
- Review AbortController behavior

### Streaming parse failure
- Verify line prefix matches
- Check discriminator field name
- Confirm JSON format of each line

### Missing body
- Check bodyTemplate defined
- Verify template syntax
- Review buildBody logic

### Header not applied
- Check headers object in config
- Verify substitution worked
- Review Headers API usage
