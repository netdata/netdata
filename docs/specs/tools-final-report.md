# final_report Tool

## TL;DR
Mandatory tool for agent to deliver the final answer. Captures format, encoding, content, optional JSON payload, and metadata. Successful completion now requires **finalization readiness**: the final report plus any required final-report plugin META blocks. The model must call `agent__final_report` via the XML FINAL wrapper in XML-NEXT; plugin META blocks use the XML META wrapper. Streaming is deduplicated and suppressed during META-only retries.

## Source Files
- `src/tools/internal-provider.ts` - Tool definition and handler
- `src/session-turn-runner.ts` - Finalization readiness, retries, and synthetic failures
- `src/final-report-manager.ts` - Final report locking and META readiness gate
- `src/plugins/runtime.ts` - Plugin META validation, cache gating, onComplete
- `src/plugins/meta-guidance.ts` - System prompt, XML-NEXT, and example guidance
- `src/xml-transport.ts` - XML FINAL/META extraction and streaming filters
- `src/ai-agent.ts` - Cache gating and completion hooks

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

**Supported Formats** (`src/final-report-manager.ts`):
- `json` - Structured JSON output (validated against schema if provided)
- `markdown` - Markdown formatted text
- `markdown+mermaid` - Markdown with Mermaid diagrams
- `slack-block-kit` - Slack Block Kit payload (messages array)
- `tty` - Terminal-optimized text with ANSI color codes
- `pipe` - Plain text for piping
- `sub-agent` - Internal agent-to-agent exchange format (opaque blob, no validation)
- `text` - Legacy plain text

**Default**: Session's `outputFormat` setting

## Final-Report Plugins (META Contract)

- Finalization readiness requires BOTH the final report and all required plugin META blocks.
- META blocks are sent via XML wrappers: `<ai-agent-NONCE-META plugin="name">{...}</ai-agent-NONCE-META>`.
- META blocks can appear before, after, or inside the FINAL wrapper; they are extracted before XML tool parsing.
- When FINAL exists but META is missing/invalid, the final report is locked, XML-NEXT switches to META-only guidance, and streaming is suppressed to avoid duplicate final output.
- Cache hits and cache writes both require valid META blocks; cache entries missing META are rejected.
- Plugin `onComplete` runs only when finalization readiness is achieved.

**Each plugin MUST provide:**
1. `schema`
2. `systemPromptInstructions`
3. `xmlNextSnippet`
4. `finalReportExampleSnippet`

**Configuration:** Use agent frontmatter `plugins:` with relative `.js` module paths. Modules must default-export a plugin factory and are loaded during initialization.

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
  return { ok: true, result: JSON.stringify({ ok: true }) };
}
```

### 2. Report Capture
**Location**: `src/ai-agent.ts`

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

### 3. Finalization Gate (Finalization Readiness)
**Location**: `src/session-turn-runner.ts`, `src/final-report-manager.ts`

```typescript
const finalizationReady = this.ctx.finalReportManager.isFinalizationReady();
if (finalizationReady) {
  return this.finalizeWithCurrentFinalReport(conversation, logs, accounting, currentTurn);
}
```

- Finalization readiness means the final report exists AND required plugin META blocks are valid.
- A final report without required META locks the report and triggers META-only retries.

### 4. JSON Schema Validation
**Location**: `src/final-report-manager.ts`, `src/session-turn-runner.ts`

```typescript
const validationResult = this.ctx.finalReportManager.validateSchema(schema);
if (!validationResult.valid) {
  // Log ERR details and replace with a synthetic failure report
}
```

- Pre-commit validation failures trigger retries.
- Post-commit validation failures are treated as transport failures and replaced with synthetic reports.

### 5. Output Emission (Streaming Deduplication)
- When a headend supplies `onEvent`, the session runner streams only FINAL wrapper content (`event.type='output'`, `meta.source='stream'`).
- META wrappers are stripped from streaming output.
- When finalization readiness is achieved, additional output derived from the final report is emitted with `meta.source='finalize'`, allowing headends to suppress duplicates.
- During META-only retries after a locked final report, streaming output is fully suppressed to avoid duplicate final output.

**Handoff note**: When a handoff is configured or a router handoff is selected, the final report payload is emitted as `event.type='handoff'` (not `final_report`). This payload is the input to the next agent and should not be treated as the user-visible final answer.

## Final Report Structure

**Session State** (`src/ai-agent.ts`):
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

**Error messaging (existing failures only):**
- JSON parse failures surface `invalid_json: <hint>` in logs and model-facing retry notices.
- Schema validation failures surface `schema_mismatch: <ajv errors>` in logs and model-facing retry notices.

3. **Layer 3 (Final Report Construction)**:
   - Build clean final report object with: format, content, content_json (for json format), metadata
   - Wrapper fields never pollute payload content

This separation ensures user schema fields (e.g., a `status` field in their JSON schema) are never overwritten by transport-level metadata.

### From Tool Call
**Location**: `src/ai-agent.ts`

When LLM explicitly calls final_report:
1. Parse tool call parameters
2. Validate status enum
3. Validate format enum
4. Set finalReport
5. Mark turn as having tool calls

### From Text Content
**Location**: `src/ai-agent.ts`

Fallback when LLM provides text:
```typescript
if (this.finalReport === undefined && !hasToolCalls && hasText) {
  if (this.tryAdoptFinalReportFromText(assistantMessage, textContent)) {
    // Adopted from text
  }
}
```

### From Tool Message
**Location**: `src/ai-agent.ts`

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
**Location**: `src/session-turn-runner.ts`

When pre-commit validation fails:
- The turn is NOT marked successful
- Error state is reset at the start of each attempt to prevent poisoning later successful attempts
- Retries continue until `maxRetries` is exhausted
- If all retries fail, remaining turns are collapsed to force a final turn
- A synthetic failure report is generated when finalization readiness cannot be achieved (no final report, final report tool failures, or required META missing on exhaustion)

### Post-Commit Safety Net
**Location**: `src/session-turn-runner.ts` (in `finalizeWithCurrentFinalReport`)

If validation somehow fails after commit (shouldn't happen with pre-commit validation):
1. Report is cleared: `ctx.finalReportManager.clear()` (plugin META records are preserved)
2. Replaced with synthetic failure report
3. Session marked as failed

### JSON Parsing for slack-block-kit
**Location**: `src/slack-block-kit.ts` (called from `src/session-turn-runner.ts`)

When parsing slack-block-kit payloads wrapped in XML or markdown:
- Uses `preferArrayExtraction: true` in `parseJsonValueDetailed()` before schema validation.
- Prioritizes extracting the outer array `[...]` over inner objects `{...}`.
- Accepts legacy `{ "messages": [...] }` and normalizes to the array.

## Final Turn Enforcement

When `isFinalTurn === true`:
1. Tools are filtered to `agent__final_report` (plus `router__handoff-to` when router handoff is allowed and the final report is not locked).
2. XML-NEXT carries the final-turn instruction; tool filtering is authoritative even if the system prompt mentions other tools.
3. Finalization requires BOTH the final report and any required plugin META blocks.
4. If the final report exists but META is missing/invalid, XML-NEXT switches to META-only mode and streaming output is suppressed.

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `outputFormat` | Default format if not specified |
| `expectedOutput.format` | Expected format validation |
| `expectedOutput.schema` | JSON schema validation |
| `finalReportPluginDescriptors` | Requires valid plugin META blocks for finalization readiness and cache writes |
| `maxTurns` | When to enforce final turn |

## Telemetry

**Captured on finalization**:
- Final report status
- Finalization readiness (final report + required META)
- Format used
- Content size
- JSON validation result
- Synthetic reason (e.g., `final_meta_missing`, `max_turns_exhausted`)
- Timestamp

## Logging

**Key Events**:
- Tool invocation logged
- Schema validation errors logged
- Final report capture logged
- META validation failures logged (`final_meta_missing`, `final_meta_invalid`)
- Session finalization logged

## Business Logic Coverage (Verified 2025-11-16)

- **Slack Block Kit repair**: Markdown is sanitized to Slack mrkdwn (headings → bold, `[text](url)` → `<url|text>`, `**`/`__` → `*`, `~~` → `~`, code-fence language stripped, tables → code blocks, `& < >` escaped, `\\n`/`\\t` escape sequences normalized) and text lengths are clamped (sections ≤2900 chars, headers ≤150, context/fields ≤2000). Normalization is shared across tool-call and XML final report paths (`src/slack-block-kit.ts`, `src/tools/internal-provider.ts`, `src/session-turn-runner.ts`).
- **Slack Block Kit schema**: Section blocks allow `text`, `fields`, or **both**; `text` is optional when fields are present (matches Slack).
- **Format mismatch handling**: If `report_format` or `format` disagrees with the session’s expected format the tool logs an error and overwrites it, preventing downstream consumers from seeing inconsistent metadata (`src/tools/internal-provider.ts:398-410`).
- **Slack content fallback**: If slack-block-kit payloads are invalid after repair, the system emits a safe single-section fallback message instead of sending invalid blocks to Slack (`src/tools/internal-provider.ts`, `src/session-turn-runner.ts`).
- **Parameter aliases**: Both `report_format`/`format` and `report_content`/`content` are accepted, but canonicalized before storing the final report (`src/tools/internal-provider.ts:398-402`).
- **Early JSON validation**: JSON outputs are validated immediately during tool execution (before `AIAgentSession` validates again), so models receive AJV diagnostics per turn (`src/tools/internal-provider.ts:629-652`).
- **Progress pairing warning**: If the agent tries to call `task_status` and `final_report` in the same turn, the tool logs a warning because finalization should not stream additional progress updates (`src/tools/internal-provider.ts:172-197`).

## Invariants

1. **Required parameters**: Varies by format; `report_format` is enforced, while `status` is optional diagnostics.
2. **Finalization readiness**: Successful termination requires BOTH the final report and all required plugin META blocks.
3. **Format validation**: `report_format` is normalized to the session’s expected format.
4. **Schema validation**: Invalid payloads trigger retries and can end in synthetic failure on exhaustion.
5. **Single final report outcome**: The session returns a single final report, but locked finals may be replaced by a synthetic failure report when META requirements cannot be satisfied.

## Test Coverage

**Phase 2**:
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
