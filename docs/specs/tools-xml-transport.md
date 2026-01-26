# XML Tool Transport

## TL;DR
XML transport is fixed to `xml-final`: native `tool_calls` remain for regular tools, while the final report and final-report plugin META blocks are sent over XML. Each turn advertises XML-NEXT (nonce, FINAL wrapper, optional schema, and META reminders). XML-PAST stays suppressed.

## Turn Messages
- XML-NEXT (ephemeral) is the only per-turn system notice. It carries the nonce, FINAL wrapper, optional schema, and META guidance.
- The system prompt may still include full tool/final-report/META instructions loaded at initialization time.
- XML-PAST is suppressed; native tool results stay in the normal transcript.

## Slots & Validation
- Nonce per session: `ai-agent-<8hex>` (fixed across all turns).
- Special slots: `NONCE-FINAL` for the final report and `NONCE-META` for per-plugin META blocks.
- FINAL wrapper: `<ai-agent-NONCE-FINAL format="<expected-format>">...</ai-agent-NONCE-FINAL>`.
- META wrapper: `<ai-agent-NONCE-META plugin="<name>">{...}</ai-agent-NONCE-META>`.
- META blocks can appear before, after, or inside FINAL content.
- META wrappers are extracted before XML tool parsing, so they never become malformed tool calls.
- FINAL tag attributes: `format` is required and must match the expected output format; `status` is optional and treated as diagnostics only (transport success is based on source).

## Execution Flow
1. Build XML-NEXT each turn; provider tool definitions remain visible for native tool calls.
2. Extract META blocks first, then parse the FINAL wrapper.
3. Execute native tool calls and the parsed final-report tool call through the standard orchestrator (`source: xml`).
4. Enforce finalization readiness.

## Finalization Readiness
- Finalization requires BOTH the final report and all required plugin META blocks.
- When FINAL exists but META is missing/invalid, the FINAL report is locked and XML-NEXT switches to META-only guidance.
- Synthetic failures may finalize with `reason: "final_meta_missing"` even when META is still missing.

## Context Guard
- XML-NEXT contributes context tokens for nonce, FINAL wrapper, optional schema, and META guidance.
- Guard behavior is unchanged; it still evaluates messages and tool outputs.

## Error Handling
- Invalid or mismatched tags are ignored.
- Malformed META wrappers are stripped. When plugins are configured, they also emit `final_meta_invalid` feedback.
- Unknown META plugin names are ignored with WRN logs.
- Leading `<think>...</think>` blocks (including leading whitespace) are stripped before XML parsing and malformed-tag checks.
- Truncated structured output (`stopReason=length|max_tokens`) is rejected for `json` and `slack-block-kit`.
- Payload JSON parsing/validation still occurs in the orchestrator (same repair/validation path as native).

## Streaming Behavior
- FINAL wrapper content is streamed with tags stripped; non-wrapper text may still stream when using `XmlFinalReportFilter`.
- META wrappers are always stripped from streaming output.
- When the final report is locked (META-only retries), streaming output is fully suppressed to avoid duplicate final output.
- Partial or unclosed META wrappers are buffered and dropped on flush to prevent wrapper leakage.

## Accounting & Telemetry
- Tool accounting uses `source: xml`, and `command` is the real tool name.
- Final report accounting uses `command: agent__final_report_xml`.
- Metrics remain unchanged.

## Final Report Processing (3-Layer Architecture)
When processing XML final reports, the system uses a 3-layer architecture:
1. Layer 1 (Transport): Extract wrapper fields (format/status) from XML tag attributes and capture raw payload from tag content.
2. Layer 2 (Format Processing): Process payload based on expected format (`sub-agent` passthrough, `json` parsing/validation, `slack-block-kit` messages validation, text formats raw).
3. Layer 3 (Final Report): Construct a clean final report object. Wrapper fields never pollute payload content.

## Test Requirements
- Parser: nonce/slot/tool validation, truncation handling, malformed META wrappers.
- XML-NEXT rendering: FINAL and META guidance must stay paired.
- Finalization readiness: FINAL-only responses retry for missing META; META-only retries do not re-stream FINAL.
- Streaming filters: META stripped, locked-final suppression, partial META buffers dropped.
- Cache: cache hits require valid META; cache entries missing META are rejected.
