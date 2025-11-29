/**
 * Consolidated LLM-facing messages for AI Agent.
 *
 * This file contains ONLY messages that are actually sent to the LLM as part of
 * the conversation (user messages, system notices, tool results, TURN-FAILED feedback).
 *
 * NOT included here:
 * - Internal log messages (use inline strings in source files)
 * - Synthetic report content (not LLM instructions)
 *
 * Categories:
 * - TURN_CONTROL: Messages injected to control turn flow
 * - TURN_FAILURE: Validation errors sent via TURN-FAILED feedback
 * - SYSTEM_NOTICES: Provider state notices (rate limits, auth, quotas)
 * - FINAL_REPORT_REMINDER: Guidance for calling final_report
 * - TOOL_RESULTS: Messages returned as tool execution results
 * - XML_PROTOCOL: Errors for XML tool transport mode (via TURN-FAILED)
 */

// =============================================================================
// TURN CONTROL MESSAGES
// Injected into the conversation to control agent behavior
// =============================================================================

/**
 * Injected when the agent reaches maximum turns.
 * Forces the model to provide its final report immediately.
 * Used in: session-turn-runner.ts (pushed to conversation)
 */
export const MAX_TURNS_FINAL_MESSAGE =
  'Maximum number of turns/steps reached. You MUST NOW provide your final report. Do NOT attempt to call any other tool. Read carefully the final report instructions and provide your final report/answer now.';

/**
 * Injected when context window limit is reached.
 * Forces immediate finalization without further tool calls.
 * Used in: session-turn-runner.ts (pushed to conversation)
 */
export const CONTEXT_FINAL_MESSAGE =
  'The conversation reached the context window limit. You MUST NOW provide your final report. Do NOT attempt to call any other tool. Read carefully the final report instructions and provide your final report/answer based on the information already gathered. If the information is insufficient, provide the best possible answer based on what you have and note the limitation in the final report.';

/**
 * Nudge to use tools instead of plain text.
 * Injected on last retry attempt before advancing turns.
 * Used in: session-turn-runner.ts (pushed as user message)
 */
export const toolReminderMessage = (excludeProgress: string, finalReportTool: string): string =>
  `Reminder: do not end with plain text. Use an available tool${excludeProgress} to make progress. When ready to conclude, provide your final report/answer (${finalReportTool}).`;

/**
 * On final turn without final answer.
 * Used in: session-turn-runner.ts (pushed as system retry message)
 */
export const FINAL_TURN_NOTICE =
  'System notice: this is the final turn. You MUST NOW provide your final report. Do NOT attempt to call any other tool. Read carefully the final report instructions and provide your final report/answer now.';

// =============================================================================
// TURN FAILURE MESSAGES
// Sent to LLM via TURN-FAILED: prefix when validation fails
// =============================================================================

/**
 * Prefix for turn failure feedback to LLM.
 * Reasons are joined with ' | ' separator.
 * Used in: session-turn-runner.ts (sent as user message)
 */
export const turnFailedPrefix = (reasons: string[]): string =>
  `TURN-FAILED: ${reasons.join(' | ')}.`;

/**
 * When final report status is invalid.
 * Used in: session-turn-runner.ts via addTurnFailure
 */
export const FINAL_REPORT_INVALID_STATUS =
  'Final report status invalid; expected: success|failure|partial.';

/**
 * When final report format doesn't match expected.
 * Used in: session-turn-runner.ts via addTurnFailure
 */
export const finalReportFormatMismatch = (expected: string, received: string): string =>
  `Final report format must be ${expected}. Received ${received}.`;

/**
 * When final report content is empty or missing.
 * Used in: session-turn-runner.ts via addTurnFailure
 */
export const FINAL_REPORT_CONTENT_MISSING =
  'Final report content missing; provide your final report in the requested format.';

/**
 * When JSON format expected but got non-JSON.
 * Used in: session-turn-runner.ts via addTurnFailure
 */
export const FINAL_REPORT_JSON_REQUIRED =
  'Final report must be JSON per schema; received non-JSON content.';

/**
 * When Slack Block Kit messages array is missing.
 * Used in: session-turn-runner.ts via addTurnFailure
 */
export const FINAL_REPORT_SLACK_MESSAGES_MISSING =
  'Final report missing messages array; provide Slack Block Kit messages.';

/**
 * When response has content but no valid tool calls and no final report.
 * Used in: session-turn-runner.ts via addTurnFailure
 */
export const FINAL_REPORT_MISSING =
  'No progress made in this turn: no tools called and no final report/answer provided. To progress you MUST call tools or provide a final report/answer. Review carefully the provided instructions and tools (if any), decide your next action(s), and follow the instructions precisely to progress.';

/**
 * When tool call parameters are malformed.
 * Used in: session-turn-runner.ts via addTurnFailure
 */
export const TOOL_CALL_MALFORMED =
  'Tool call payload malformed; provide JSON arguments matching the schema.';

// =============================================================================
// SYSTEM NOTICES (LLM-facing only)
// =============================================================================

/**
 * After empty response - no tools called and no final report.
 * Used in: session-turn-runner.ts (pushed as system retry message)
 */
export const emptyResponseRetryNotice = (finalReportTool: string): string =>
  `System notice: No progress made in this turn: no tools called and no final report/answer provided. To progress you MUST call tools or provide a final report/answer (${finalReportTool}). Review carefully the provided instructions and tools (if any), decide your next action(s), and follow the instructions precisely to progress.`;

// =============================================================================
// FINAL REPORT REMINDER
// Injected to guide correct final_report usage
// =============================================================================

/**
 * Builds a detailed reminder for how to call final_report correctly.
 * Used in: session-turn-runner.ts buildFinalReportReminder (system retry message)
 */
export const finalReportReminder = (
  finalReportTool: string,
  format: string,
  formatDescription: string,
  contentGuidance: string
): string =>
  `System notice: call ${finalReportTool} with report_format="${format}" (${formatDescription}), set status to success, failure, or partial as appropriate, and ${contentGuidance}.`;

/**
 * Content guidance for JSON format.
 */
export const CONTENT_GUIDANCE_JSON =
  'include a `content_json` object that matches the expected schema';

/**
 * Content guidance for Slack Block Kit format.
 */
export const CONTENT_GUIDANCE_SLACK =
  'include a `messages` array populated with the final Slack Block Kit blocks';

/**
 * Content guidance for text formats.
 */
export const CONTENT_GUIDANCE_TEXT =
  'include `report_content` containing the full final answer';

// =============================================================================
// TOOL RESULTS
// Messages returned as tool execution results (seen by LLM)
// =============================================================================

/**
 * Placeholder for tool output when context budget exceeded.
 * Returned as tool result to the LLM.
 * Used in: session-tool-executor.ts, session-turn-runner.ts
 */
export const TOOL_NO_OUTPUT = '(tool failed: context window budget exceeded)';

// =============================================================================
// XML PROTOCOL ERRORS
// Sent via onTurnFailure callback (become part of TURN-FAILED message)
// =============================================================================

/**
 * Final report payload is not valid JSON (XML mode).
 * Used in: xml-transport.ts via onTurnFailure
 */
export const XML_FINAL_REPORT_NOT_JSON =
  'Final report payload is not valid JSON. Use the JSON schema from XML-NEXT.';

/**
 * Tool payload is not valid JSON (XML mode).
 * Used in: xml-transport.ts via onTurnFailure
 */
export const xmlToolPayloadNotJson = (toolName: string): string =>
  `Tool \`${toolName}\` payload is not valid JSON. Provide a JSON object.`;

/**
 * XML tag slot mismatch.
 * Used in: xml-transport.ts via onTurnFailure
 */
export const xmlSlotMismatch = (capturedSlot: string): string =>
  `Tag ignored: slot '${capturedSlot}' does not match the current nonce/slot for this turn.`;

/**
 * XML missing closing tag.
 * Used in: xml-transport.ts via onTurnFailure
 */
export const xmlMissingClosingTag = (capturedSlot: string): string =>
  `Malformed XML: missing closing tag for '${capturedSlot}'.`;

/**
 * XML malformed - nonce/slot/tool mismatch.
 * Used in: xml-transport.ts via onTurnFailure
 */
export const xmlMalformedMismatch = (slotInfo: string): string =>
  `Malformed XML: nonce/slot/tool mismatch or empty content for '${slotInfo}'.`;
