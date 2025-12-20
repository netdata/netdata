# final_report Tool

## TL;DR
Mandatory tool for agent to deliver final answer. Captures format, encoding, content, optional JSON payload, and metadata. Terminates session execution. `report_content` may be `raw` or `base64`; `content_json` is schema-validated with a light repair loop for stringified nested JSON. The model must call `agent__final_report` via the XML slot in XML-NEXT (single transport path: native tools + XML final report). Provider tool definitions remain native, but the final report must be sent via the XML tag. When streaming is enabled, the session runner deduplicates output so the final report is not emitted twice (streaming + finalize).

## Source Files
- `src/tools/internal-provider.ts` - Tool definition and handler
- `src/ai-agent.ts:123` - Tool name constant
- `src/ai-agent.ts:678-688` - setFinalReport callback
- `src/ai-agent.ts:1891-1948` - Result adoption logic
- `src/ai-agent.ts:2018-2034` - Schema validation

## Tool Definition

**Name**: `agent__final_report`
**Short Name**: `final_report`

### Input Schema
**Note**: Schema is dynamic based on output format. Required fields vary.

#### Common Properties
```json
{
  "type": "object",
  "properties": {
    "report_format": {
      "type": "string",
      "const": "<format>",
      "description": "Output format (enforced by session)"
    },
    "metadata": {
      "type": "object",
      "description": "Optional metadata about the answer"
    }
  }
}
```

`format` is accepted as an alias for `report_format`, but the canonical property is always `report_format` with a `const` that matches the session’s expected output format; mismatches log a warning and are normalized before storing the final report (`src/tools/internal-provider.ts:398-410`).

#### JSON Format
```json
{
  "required": ["report_format", "content_json"],
  "properties": {
    "content_json": {
      "type": "object",
      "description": "Structured JSON output (validated against schema if provided)"
    }
  }
}
```

#### Slack Block Kit Format
```json
{
  "required": ["report_format", "messages"],
  "properties": {
    "messages": {
      "type": "array",
      "description": "Slack Block Kit message blocks"
    }
  }
}
```

#### Other Formats (markdown, tty, pipe, text, sub-agent)
```json
{
  "required": ["report_format", "report_content", "encoding"],
  "properties": {
    "encoding": {
      "type": "string",
      "enum": ["raw", "base64"],
      "description": "Use base64 when content has heavy markdown/newlines; otherwise raw"
    },
    "report_content": {
      "type": "string",
      "description": "The final answer text"
    }
  }
}
```

**Important**: `report_format` is always required and enforced via `const` keyword to match session format.

## Format Values

**Supported Formats** (`src/ai-agent.ts:47`):
- `json` - Structured JSON output (validated against schema if provided)
- `markdown` - Markdown formatted text
- `markdown+mermaid` - Markdown with Mermaid diagrams
 - `slack-block-kit` - Slack Block Kit payload (messages array)
- `tty` - Terminal-optimized text with ANSI color codes
- `pipe` - Plain text for piping
- `sub-agent` - Internal agent-to-agent exchange format (opaque blob, no validation)
- `text` - Legacy plain text

**Default**: Session's `outputFormat` setting

## Execution Flow

### 1. Tool Invocation
**Location**: InternalToolProvider.execute()

```typescript
if (name === 'agent__final_report') {
  const status = params.status;
  const format = params.report_format ?? sessionFormat;
  const content = params.report_content;
  const contentJson = params.content_json;
  const metadata = params.metadata;

  setFinalReport({ status, format, content, content_json: contentJson, metadata });
  return { ok: true, result: 'Final report captured.' };
}
```

### 2. Report Capture
**Location**: `src/ai-agent.ts:678-688`

```typescript
setFinalReport: (p) => {
  this.finalReport = {
    format: p.format,
    content: p.content,
    content_json: p.content_json,
    metadata: p.metadata,
    ts: Date.now()
  };
}
```

### 3. Session Termination
**Location**: `src/ai-agent.ts:2018-2019`

```typescript
if (this.finalReport !== undefined) {
  // Deterministic finalization
  // Break from turn loop
}
```

### 4. JSON Schema Validation
**Location**: `src/ai-agent.ts:2022-2034`

If format is `json` and schema provided:
```typescript
if (fr.format === 'json' && schema !== undefined && fr.content_json !== undefined) {
  const validate = this.ajv.compile(schema);
  const valid = validate(fr.content_json);
  if (!valid) {
    // Log validation errors but don't fail
  }
}
```

### 5. Output Emission (Streaming Deduplication)
When a headend supplies an `onOutput` callback, the session runner may stream model output in real time. When the final report is accepted, the runner emits the final report content via `onOutput` only if it was not already streamed, preventing duplicated answers in streaming UIs.

## Final Report Structure

**Session State** (`src/ai-agent.ts:189-197`):
```typescript
private finalReport?: {
  format: FormatType;
  content?: string;
  content_json?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  ts: number;
};
```

## Result in AIAgentResult

```typescript
interface AIAgentResult {
  // ...
  finalReport?: {
    format: string;
    content?: string;
    content_json?: Record<string, unknown>;
    metadata?: Record<string, unknown>;
    ts: number;
  };
}
```

## Adoption Strategies

### XML Transport
- Transport is fixed to XML-final: `agent__final_report` must be emitted inside `<ai-agent-NONCE-FINAL tool="agent__final_report" format="...">payload</ai-agent-NONCE-FINAL>` matching the session nonce.
- The `format` attribute is extracted from the XML tag (wrapper layer), while the tag content becomes the raw payload.
- Progress always follows native tool_calls; only the final report travels via XML.

### 3-Layer Processing Architecture
Final report processing uses a 3-layer architecture to cleanly separate transport concerns from payload content:

1. **Layer 1 (Transport/Wrapper Extraction)**:
   - Extract `report_format` from XML tag attribute or tool params
   - Extract raw payload from `_rawPayload` (XML) or content fields (native)
   - Extract optional `metadata` from params

2. **Layer 2 (Format-Specific Processing)**:
   - `sub-agent`: Opaque blob passthrough—no validation, no JSON parsing. Whatever the model returns is passed through unchanged.
   - `json`: Parse JSON from raw payload, validate structure. If parsing fails, reject and retry.
   - `slack-block-kit`: Parse JSON, expect the messages array directly (`[...]`). Legacy `{messages: [...]}` wrappers are tolerated for backwards compatibility but not instructed.
   - Text formats (`text`, `markdown`, `markdown+mermaid`, `tty`, `pipe`): Use raw payload directly as text content.

3. **Layer 3 (Final Report Construction)**:
   - Build clean final report object with: format, content, content_json (for json format), metadata
   - Wrapper fields never pollute payload content

This separation ensures user schema fields (e.g., a `status` field in their JSON schema) are never overwritten by transport-level metadata.

### From Tool Call
**Location**: `src/ai-agent.ts:1892-1926`

When LLM explicitly calls final_report:
1. Parse tool call parameters
2. Validate status enum
3. Validate format enum
4. Set finalReport
5. Mark turn as having tool calls

### From Text Content
**Location**: `src/ai-agent.ts:1885-1889`

Fallback when LLM provides text:
```typescript
if (this.finalReport === undefined && !hasToolCalls && hasText) {
  if (this.tryAdoptFinalReportFromText(assistantMessage, textContent)) {
    // Adopted from text
  }
}
```

### From Tool Message
**Location**: `src/ai-agent.ts:1927-1942`

When adoption from call fails:
```typescript
adoptFromToolMessage() {
  const toolMessage = sanitizedMessages.find(msg => msg.role === 'tool');
  this.finalReport = {
    format: sessionFormat,
    content: toolMessage.content,
    ts: Date.now()
  };
}
```

## Schema Validation and Retry Behavior

### Pre-Commit Validation
**Location**: `src/session-turn-runner.ts:1329-1358`

Before committing a final report, the runner validates the payload against the expected schema:

1. **Validation Check**: `ctx.finalReportManager.validatePayload()` validates before commit
2. **On Failure**:
   - Sets `finalReportToolFailedThisTurn = true`
   - Sets `finalReportSchemaFailed = true`
   - Sets `lastErrorType = 'invalid_response'`
   - Returns `false` to trigger retry within the same turn
3. **On Success**: Proceeds to `commitFinalReport()`

### Retry Semantics
**Location**: `src/session-turn-runner.ts:1500-1507`

When pre-commit validation fails:
- The turn is NOT marked successful
- Error state is reset at the start of each attempt (line 390-393) to prevent poisoning later successful attempts
- Retries continue until `maxRetries` is exhausted
- If all retries fail, remaining turns are collapsed to force a final turn
- A synthetic failure report is generated only when: no final report exists AND `finalReportToolFailedEver` is false (i.e., no prior final report attempt)

### Post-Commit Safety Net
**Location**: `src/session-turn-runner.ts:2262-2282` (in `finalizeWithCurrentFinalReport`)

If validation somehow fails after commit (shouldn't happen with pre-commit validation):
1. Report is cleared: `ctx.finalReportManager.clear()`
2. Replaced with synthetic failure report
3. Session marked as failed

### JSON Parsing for slack-block-kit
**Location**: `src/session-turn-runner.ts:1195`

When parsing slack-block-kit payloads wrapped in XML or markdown:
- Uses `preferArrayExtraction: true` option in `parseJsonValueDetailed()`
- This prioritizes extracting outer array `[...]` over inner objects `{...}`
- Ensures the messages array wrapper is preserved, not stripped

## Final Turn Enforcement

When `isFinalTurn === true`:
1. Tools restricted to only `agent__final_report`
2. System message injected: "You must provide your final report now..."
3. If no final_report: synthetic retry triggered

**Messages** (`src/ai-agent.ts:125-126`):
- MAX_TURNS_FINAL_MESSAGE
- CONTEXT_FINAL_MESSAGE

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `outputFormat` | Default format if not specified |
| `expectedOutput.format` | Expected format validation |
| `expectedOutput.schema` | JSON schema validation |
| `maxTurns` | When to enforce final turn |

## Telemetry

**Captured on finalization**:
- Final report status
- Format used
- Content size
- JSON validation result
- Timestamp

## Logging

**Key Events**:
- Tool invocation logged
- Schema validation errors logged
- Final report capture logged
- Session finalization logged

## Business Logic Coverage (Verified 2025-11-16)

- **Slack Block Kit repair**: The handler normalizes Markdown, clamps text lengths (sections ≤2900 chars, headers ≤150, context ≤2000), and coerces message structures before returning them to Slack clients (`src/tools/internal-provider.ts:407-627`).
- **Format mismatch handling**: If `report_format` or `format` disagrees with the session’s expected format the tool logs an error and overwrites it, preventing downstream consumers from seeing inconsistent metadata (`src/tools/internal-provider.ts:398-410`).
- **Slack content fallback**: When Slack payloads omit `messages`, the provider falls back to `report_content` to avoid empty replies in Slack headend (`src/tools/internal-provider.ts:454-456`).
- **Parameter aliases**: Both `report_format`/`format` and `report_content`/`content` are accepted, but canonicalized before storing the final report (`src/tools/internal-provider.ts:398-402`).
- **Early JSON validation**: JSON outputs are validated immediately during tool execution (before `AIAgentSession` validates again), so models receive AJV diagnostics per turn (`src/tools/internal-provider.ts:629-652`).
- **Progress pairing warning**: If the agent tries to call `task_status` and `final_report` in the same turn, the tool logs a warning because finalization should not stream additional progress updates (`src/tools/internal-provider.ts:172-197`).

## Invariants

1. **Required parameters**: Varies by format (always includes status and report_format)
2. **Terminates session**: Once captured, session ends
3. **Format validation**: Must match known format values via `const` enforcement
4. **Schema validation**: JSON validated but errors don't fail
5. **Single report**: Only one final report per session

## Test Coverage

**Phase 1**:
- Successful report capture
- Different status values
- Format variations
- JSON schema validation
- Adoption fallbacks

**Gaps**:
- Complex JSON schema scenarios
- Format conversion edge cases
- Metadata handling

## Troubleshooting

### Report not captured
- Check tool call format
- Verify required parameters present
- Check parameter types

### JSON validation fails
- Check schema definition
- Verify content_json structure
- Check AJV configuration

### Wrong format
- Check report_format parameter
- Verify sessionFormat default
- Check format enum values

### Missing content
- Check report_content parameter
- Verify string encoding
- Check truncation limits
