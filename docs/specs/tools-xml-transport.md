# XML Tool Transport

## TL;DR
XML transport is fixed to xml-final: native tool_calls remain for regular tools, while the final report must be sent over XML. Each turn advertises XML-NEXT (nonce, final slot, optional schema; progress slot omitted). Tags are parsed by substring; valid tags map to existing tools with the same orchestrator, budgets, and accounting. CLI override for transport has been removed.

## Turn Messages
- **XML-NEXT (ephemeral)**: user notice with nonce, final-report slot, and optional schema; counts toward context like any user message. It reminds the model that non-final tools must be called via native tool_calls.
- **XML-PAST**: suppressed; native tool results stay in the normal transcript.

## Slots & Validation
- Nonce per session: `ai-agent-<8hex>` (fixed across all turns).
- Slots: dedicated `NONCE-FINAL` slot for the final report.
- Valid tag: matching nonce, declared slot, allowed tool, non-empty content. Allowed tools via XML are restricted to `agent__final_report`; non-final tools must use native tool_calls; progress uses native tool_calls.
- Status enum for final-report: `success|failure|partial` (extracted from XML tag attributes); payload format must match the agent's expected format.

## Execution Flow
1) Build XML-NEXT each turn; provider tool definitions remain visible for native tool_calls.
2) Model emits XML tag for final report; parser extracts tag into toolCalls. Native tool_calls are preserved and merged with the parsed XML final-report.
3) Orchestrator executes tools unchanged (`source: xml` in accounting); final-report uses `agent__final_report`.
4) Final-report ends session (no response message).

## Progress
- Progress follows native tool_calls; there is no XML progress slot.

## Context Guard
- XML-NEXT contributes context tokens for nonce/final slot and any final-report schema; guard still runs on messages/tool outputs.

## Error Handling
- Invalid/mismatched tags: ignored.
- Leading `<think>...</think>` blocks (including leading whitespace) are stripped before XML parsing, unclosed-final extraction, and malformed-tag checks, so XML examples inside reasoning are never treated as final reports.
- Truncated structured output (stopReason=length/max_tokens) is rejected for JSON/slack-block-kit; the failure reason includes the token limit (e.g., `token_limit: N tokens`) and no JSON repair is attempted on truncated XML wrappers.
- Payload JSON parsing/validation still done at orchestrator (same repair/validation path as native).
- Over-budget tool outputs replaced with `(tool failed: context window budget exceeded)` as usual.

## Accounting & Telemetry
- Tool accounting: `source: xml`, `command` = real tool name; final-report `command: agent__final_report_xml`.
- Metrics unchanged; toolChoice=auto recorded in provider options.

## Final Report Processing (3-Layer Architecture)
When processing XML final reports, the system uses a 3-layer architecture:
1. **Layer 1 (Transport)**: Extract wrapper fields (`status`, `report_format`) from XML tag attributes; extract raw payload from tag content via `_rawPayload` internal field.
2. **Layer 2 (Format Processing)**: Process payload based on expected format:
   - `sub-agent`: Opaque blob passthrough (no validation, no parsing)
   - `json`: Parse JSON, validate against schema if provided
   - `slack-block-kit`: Parse JSON, expect the messages array directly (`[...]`); legacy `{messages: [...]}` wrappers are tolerated but not recommended. Payload is sanitized to Slack mrkdwn and validated against the strict Block Kit schema; invalid payloads fall back to a safe single-section message.
   - Text formats (`text`, `markdown`, `tty`, `pipe`): Use raw payload directly
3. **Layer 3 (Final Report)**: Construct clean final report object with status, format, content, and optional metadata.

This separation ensures wrapper fields (transport metadata) never pollute the payload content.

## Test Requirements (planned)
- Parser chunking/nonce/slot/tool validation
- XML-NEXT rendering (nonce + final slot)
- Final-report precedence in XML
- Accounting `source: xml`
- Phase 1 harness scenarios covering: native tools + XML final-report, invalid tag/nonce ignored
- 3-layer final report processing for each format type
