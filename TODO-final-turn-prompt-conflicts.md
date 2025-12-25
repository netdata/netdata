# TL;DR
- Only two system notice types should remain: XML-NEXT (ephemeral) and TURN-FAILED (permanent), with at most one of each per turn.
- All injected non-tool messages must be consolidated into XML-NEXT or TURN-FAILED; no other system notices should be appended to requests.
- XML-NEXT must cover every case; TURN-FAILED must cover every failure case; synthetic tool responses remain separate and must still be emitted.
- Requirement: every tool call emitted by the model must be preserved in messages and must always have a tool response; therefore no tool-related TURN-FAILED slugs are needed.

# Analysis
- Inventory of synthetic **user** messages injected into LLM requests (non-tool):
  - XML-NEXT notice (ephemeral):
    - Source: `src/xml-transport.ts` → `renderXmlNextTemplate` in `src/llm-messages.ts`
    - Role: `user`, `noticeType: 'xml-next'`
    - Persistence: **Ephemeral** (added to `attemptConversation`, not persisted)
    - Current classification: **XML-NEXT**
  - XML-PAST notice (inactive in xml-final):
    - Source: `renderXmlPastTemplate` in `src/llm-messages.ts` (wired in `xml-transport.ts`)
    - Role: `user`, `noticeType: 'xml-past'`
    - Persistence: Ephemeral, **but currently not injected** because `xml-final` sets `pastMessage` to `undefined`
    - Current classification: Would be XML-NEXT/ephemeral if enabled, but should remain **disabled** per “only XML-NEXT + TURN-FAILED”
  - Last‑retry tool reminder (ephemeral):
    - Source: `toolReminderMessage` in `src/llm-messages.ts` → injected in `src/session-turn-runner.ts`
    - Role: `user`
    - Persistence: **Ephemeral** (in `attemptConversation` only)
    - Current classification: **Not XML-NEXT** (must be folded into XML-NEXT)
  - Final‑turn instruction variants (ephemeral):
    - Sources: `MAX_TURNS_FINAL_MESSAGE`, `CONTEXT_FINAL_MESSAGE`, `TASK_STATUS_*_FINAL_MESSAGE`, `RETRY_EXHAUSTION_FINAL_MESSAGE` in `src/llm-messages.ts`
    - Injection: `src/session-turn-runner.ts` when `isFinalTurn`
    - Role: `user`
    - Persistence: **Ephemeral** (in `attemptConversation` only)
    - Current classification: **Not XML-NEXT** (must be folded into XML-NEXT)
  - Provider final‑turn “CRITICAL/status” notice (ephemeral):
    - Source: `BaseLLMProvider.buildFinalTurnMessages(...)` in `src/llm-providers/base.ts`
    - Role: `user`
    - Persistence: **Ephemeral** (request-only)
    - Current classification: **Not XML-NEXT** (must be removed or absorbed into XML-NEXT)
  - TURN‑FAILED message (permanent):
    - Source: `turnFailedPrefix(...)` in `src/llm-messages.ts` → `flushTurnFailureReasons(...)` in `src/session-turn-runner.ts`
    - Role: `user`
    - Persistence: **Permanent** (pushed to `conversation`)
    - Current classification: **TURN-FAILED**

- Draft slug map (TURN-FAILED reasons currently emitted via addTurnFailure):
  - final_report_format_mismatch → `finalReportFormatMismatch(...)` (expected vs received format)
  - final_report_content_missing → `FINAL_REPORT_CONTENT_MISSING`
  - final_report_json_required → `FINAL_REPORT_JSON_REQUIRED`
  - final_report_slack_messages_missing → `FINAL_REPORT_SLACK_MESSAGES_MISSING`
  - final_report_schema_validation_failed → `finalReportSchemaValidationFailed(...)`
  - tool_message_fallback_schema_failed → `toolMessageFallbackValidationFailed(...)`
  - content_without_tools_or_report → `TURN_FAILED_NO_TOOLS_NO_REPORT_CONTENT_PRESENT`
  - empty_response → `TURN_FAILED_EMPTY_RESPONSE`
  - reasoning_only → `TURN_FAILED_REASONING_ONLY`
  - reasoning_only_final → `TURN_FAILED_REASONING_ONLY_FINAL`
  - output_truncated → `turnFailedOutputTruncated(...)`
  - final_turn_no_report → `TURN_FAILED_FINAL_TURN_NO_REPORT`
  - tool_call_malformed → `TOOL_CALL_MALFORMED`
  - unknown_tool → `TURN_FAILED_UNKNOWN_TOOL`
  - tool_call_not_executed → `TURN_FAILED_TOOL_CALL_NOT_EXECUTED`
  - tool_limit_exceeded → `TURN_FAILED_TOOL_LIMIT_EXCEEDED`
  - xml_wrapper_called_as_tool → `turnFailedXmlWrapperAsTool(...)`
  - xml_final_report_not_json → `XML_FINAL_REPORT_NOT_JSON`
  - xml_tool_payload_not_json → `xmlToolPayloadNotJson(...)`
  - xml_slot_mismatch → `xmlSlotMismatch(...)`
  - xml_missing_closing_tag → `xmlMissingClosingTag(...)`
  - xml_malformed_mismatch → `xmlMalformedMismatch(...)`
  - xml_structured_output_truncated → `turnFailedStructuredOutputTruncated(...)`
  - progress_only_task_status → `TURN_FAILED_PROGRESS_ONLY` (defined but **unused** today)

- Draft slug map (XML-NEXT reasons to consolidate all ephemerals):
  - xml_next_base → Always present; includes turn/step, context % usage, and XML slot template.
  - xml_next_tools_available → `hasExternalTools === true` (shows EITHER tools or final report path).
  - xml_next_tools_unavailable → `hasExternalTools === false` (must provide final report now).
  - xml_next_retry_attempt → `attempt > 1` (shows retry X of Y).
  - xml_next_last_retry → `attempt === maxRetries` AND not final turn (currently toolReminderMessage).
  - xml_next_final_turn_max_turns → `forcedFinalTurnReason === 'max_turns'`.
  - xml_next_final_turn_context → `forcedFinalTurnReason === 'context'`.
  - xml_next_final_turn_task_status_completed → `forcedFinalTurnReason === 'task_status_completed'`.
  - xml_next_final_turn_task_status_only → `forcedFinalTurnReason === 'task_status_only'`.
  - xml_next_final_turn_retry_exhaustion → `forcedFinalTurnReason === 'retry_exhaustion'`.
  - xml_next_task_status_enabled → `taskStatusToolEnabled === true` (adds task_status guidance line).

- Tool-related TURN-FAILED slugs: current trigger behavior (not limited to “all tools failed”)
  - tool_call_malformed → fires when any malformed tool call is dropped (can co-exist with successful tools).
  - unknown_tool → fires if any unknown tool is encountered (can co-exist with successful tools).
  - tool_limit_exceeded → fires if per-turn tool limit exceeded (can co-exist with successful tools).
  - tool_call_not_executed → fires only when **no known tool was attempted** (executedTools === 0).
  - progress_only_task_status → defined but currently unused (no trigger in code).

- /tmp/session-request.json shows three consecutive user messages at the end of the prompt:
  - XML-NEXT “System Notice” (turn/step + EITHER/OR + tool guidance).
  - FINAL_TURN_NOTICE (“This is the final turn…”).
  - Provider final-turn message (“**CRITICAL**: You cannot collect more data… set `status`…”).
  Evidence: /tmp/session-request.json lines ~169–177 (search “System Notice”, “final turn”, “CRITICAL”).
- Source 1: XML-NEXT is generated by renderXmlNextTemplate and injected every turn in session-turn-runner via xmlTransport.buildMessages.
  - XML-NEXT uses `hasExternalTools` only, and does not check `isFinalTurn`, so it can still offer tool calls even in final turn.
  Evidence: src/llm-messages.ts (renderXmlNextTemplate), src/xml-transport.ts (buildMessages), src/session-turn-runner.ts (attemptConversation push).
- Source 2: session-turn-runner adds a final-turn instruction as an extra user message whenever `isFinalTurn` is true.
  Evidence: src/session-turn-runner.ts (finalInstruction branch), src/llm-messages.ts (MAX_TURNS_FINAL_MESSAGE / FINAL_TURN_NOTICE).
- Source 3: BaseLLMProvider injects a final-turn message with the “status: failure/partial/success” requirement.
  Evidence: src/llm-providers/base.ts (buildFinalTurnMessages).
- Contract mismatch: docs/CONTRACT.md states final report status is system-determined and models should not set it; the base provider message instructs the opposite.
  Evidence: docs/CONTRACT.md (“Status is not provided by the model…” in XML final-report section).
- Tool exposure mismatch: final-turn tool selection reduces tools to `agent__final_report`, and provider filtering removes that tool in XML-final mode, resulting in zero tools, yet XML-NEXT still tells the model to “call tools.”
  Evidence: src/session-turn-runner.ts (selectToolsForTurn), src/llm-providers/base.ts (filterToolsForFinalTurn), src/llm-messages.ts (XML-NEXT EITHER/OR).
- Tool call drop without tool response (violates “every tool call must have a response”):
  - Source: `sanitizeTurnMessages(...)` in `src/session-turn-runner.ts` drops malformed tool calls (non-object tool call, invalid parameters JSON, non-object parameters) before execution.
  - Dropped tool calls never execute and never receive tool responses; they are removed from sanitized messages.
- Outcome evidence: /tmp/session-response-content.txt shows the model wrapped the XML output inside a JSON object with `role` + `content`, and added “Status: partial” in markdown. This matches confusion from conflicting “set status” instruction.
  Evidence: /tmp/session-response-content.txt.
- SSE confirms the model itself emitted the JSON wrapper (not a capture artifact): the stream switches from `reasoning_content` to `content` and begins with `{ "role": "assistant", "content": "<ai-agent-7b5c4660-FINAL ...` then streams the XML body.
  Evidence: /tmp/session-response.sse (around lines ~3208+).

# Decisions
- Scope decision (user): Only two system notice types are allowed — XML-NEXT (ephemeral) and TURN-FAILED (permanent). No other system notices may be injected into requests.
- Scope decision (user): XML-NEXT must cover all cases; TURN-FAILED must cover all failure cases; at most one of each per turn.
- Scope decision (user): Synthetic tool failure responses remain and are not part of XML-NEXT or TURN-FAILED; tool calls still require tool responses.
- Scope decision (user): Keep XML-tools + XML final-report parsing and slugs unchanged for now (no refactor or removal). Revisit later.
- Scope decision (user): Split notice slug configuration into two separate files (XML-NEXT and TURN-FAILED), each with a static slug map at the top that includes configuration + messages together.
- Scope decision (user): New files must be named `src/llm-messages-xml-next.ts` and `src/llm-messages-turn-failed.ts` so they sit next to `src/llm-messages.ts`.
- Scope decision (user): Slug messages should be static, with optional dynamic context appended as ` Reason: {dynamic message}` when present.
- Scope decision (user): Proceed with implementation now (“yes, go”).
- Scope decision (user): Remove all tool-related TURN-FAILED slugs. Optional keep `tool_call_not_executed` renamed to `all_tools_failed` (user ok with removal).
- Scope decision (user): XML protocol errors (xml_* reasons) remain TURN-FAILED for now (no refactor).
- Decision (user): Preserve all tool calls; sanitize invalid tool calls into a sentinel params object and short‑circuit execution with a tool failure response (no tool-related TURN-FAILED slugs).
- Decision (user): For every invalid tool call, emit an **ERR** log with the full tool-call payload.
- Decision (user): Echo invalid tool payloads back to the model **when they fit**; otherwise return truncated payload + hash + explicit note.
- Decision (user): Remove the now-empty `appendToolExecutionFailures` helper.
- Decision (user): Centralize the 4096-byte payload truncation limit into a shared constant.
- Decision (user): Add a runtime guard around TURN-FAILED slug lookup for defense-in-depth.

# Plan
- ✅ Inventory all system-injected messages (non-tool) across codepaths.
- ✅ Classify each as XML-NEXT (ephemeral) or TURN-FAILED (permanent).
- ✅ Rewrite XML-NEXT to cover every non-failure case, including final-turn variants.
- ✅ Rewrite TURN-FAILED to cover every failure case with consistent guidance.
- ✅ Remove all other injected system notices from requests.
- ✅ Enforce at most one XML-NEXT + one TURN-FAILED per turn.
- ✅ Remove tool-related TURN-FAILED reasons and ensure tool calls always emit tool responses (including malformed tool calls).
- ✅ Sanitize invalid tool calls into sentinel params and short-circuit execution with a tool failure response.
- ✅ Add ERR logging for each invalid tool call with full payload.
- ⏳ Update Phase 1 harness expectations for XML-NEXT and tool-failure behavior.
- ⏳ Run `npm run lint`, `npm run build`, and `npm run test:phase1`.

# Implied Decisions
- None.

# Testing Requirements
- Completed: Phase 1 harness updated for XML-NEXT final-turn content and tool-failure responses (no TURN-FAILED for malformed tool payloads).
- Completed: `npm run lint` (pass), `npm run build` (pass), `npm run test:phase1` (pass; 252/252, warnings expected).

# Documentation Updates Required
- Completed: docs/CONTRACT.md, docs/AI-AGENT-GUIDE.md, docs/AI-AGENT-INTERNAL-API.md, docs/SPECS.md, docs/specs/retry-strategy.md, docs/specs/session-lifecycle.md.
