# final_report Tool

## TL;DR
Mandatory tool for agent to deliver final answer. Captures status, format, content, optional JSON payload, and metadata. Terminates session execution.

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
    "status": {
      "type": "string",
      "enum": ["success", "failure", "partial"],
      "description": "Task completion status"
    },
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
  "required": ["status", "report_format", "content_json"],
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
  "required": ["status", "report_format", "messages"],
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
  "required": ["status", "report_format", "report_content"],
  "properties": {
    "report_content": {
      "type": "string",
      "description": "The final answer text"
    }
  }
}
```

**Important**: `report_format` is always required and enforced via `const` keyword to match session format.

## Status Values

| Status | Meaning |
|--------|---------|
| `success` | Task completed successfully |
| `failure` | Task could not be completed |
| `partial` | Partial completion, some issues |

## Format Values

**Supported Formats** (`src/ai-agent.ts:47`):
- `json` - Structured JSON output
- `markdown` - Markdown formatted text
- `markdown+mermaid` - Markdown with Mermaid diagrams
- `slack-block-kit` - Slack Block Kit payload
- `tty` - Terminal-optimized text
- `pipe` - Plain text for piping
- `sub-agent` - Sub-agent exchange format
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
  const normalizedStatus = p.status;
  this.finalReport = {
    status: normalizedStatus,
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

## Final Report Structure

**Session State** (`src/ai-agent.ts:189-197`):
```typescript
private finalReport?: {
  status: 'success' | 'failure' | 'partial';
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
    status: 'success' | 'failure' | 'partial';
    format: string;
    content?: string;
    content_json?: Record<string, unknown>;
    metadata?: Record<string, unknown>;
    ts: number;
  };
}
```

## Adoption Strategies

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
    status: 'success',
    format: sessionFormat,
    content: toolMessage.content,
    ts: Date.now()
  };
}
```

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
- **Progress pairing warning**: If the agent tries to call `progress_report` and `final_report` in the same turn, the tool logs a warning because finalization should not stream additional progress updates (`src/tools/internal-provider.ts:172-197`).

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
