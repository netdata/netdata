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
 * - FINAL_REPORT_REMINDER: Guidance for calling final_report
 * - TOOL_RESULTS: Messages returned as tool execution results
 * - XML_PROTOCOL: Errors for the XML final-report transport (via TURN-FAILED)
 */

// =============================================================================
// TURN CONTROL MESSAGES
// Injected into the conversation to control agent behavior
// =============================================================================

/**
 * Injected when the agent reaches maximum turns.
 * Forces the model to provide its final report immediately.
 * Used in: session-turn-runner.ts (pushed to conversation)
 *
 * CONDITION: isFinalTurn && forcedFinalTurnReason !== 'context'
 * Where isFinalTurn = (currentTurn >= maxTurns - 1) || forcedFinalTurn
 */
export const MAX_TURNS_FINAL_MESSAGE =
  'Maximum number of turns/steps reached. You **MUST NOW** provide your final report/answer. Do NOT attempt to call any other tool. Read carefully the final report/answer instructions and provide your final report/answer based on the information already gathered. If the information is insufficient, provide the best possible answer based on what you have and note the limitation in your final report/answer.';

/**
 * Injected when context window limit is reached.
 * Forces immediate finalization without further tool calls.
 * Used in: session-turn-runner.ts (pushed to conversation)
 *
 * CONDITION: isFinalTurn && forcedFinalTurnReason === 'context'
 * Where forcedFinalTurn is set when context budget is exhausted
 */
export const CONTEXT_FINAL_MESSAGE =
  'The conversation reached the context window limit. You **MUST NOW** provide your final report/answer. Do NOT attempt to call any other tool. Read carefully the final report/answer instructions and provide your final report/answer based on the information already gathered. If the information is insufficient, provide the best possible answer based on what you have and note the limitation in your final report/answer.';

/**
 * Nudge to use tools instead of plain text.
 * Injected on last retry attempt before advancing turns.
 * Used in: session-turn-runner.ts (pushed as user message)
 *
 * CONDITION: (attempts === maxRetries - 1) && currentTurn < (maxTurns - 1)
 * (last retry attempt within a non-final turn)
 */
export const toolReminderMessage = (excludeProgress: string): string =>
  `Reminder: do not end with plain text. Use an available tool${excludeProgress} to make progress. When ready to conclude, provide your final report/answer in the required XML wrapper.`;

/**
 * On final turn without final answer.
 * Used in: session-turn-runner.ts (pushed as system retry message)
 *
 * CONDITION: isFinalTurn && !turnResult.status.finalAnswer
 * (final turn reached, but response has no final report)
 */
export const FINAL_TURN_NOTICE =
  'This is the final turn. You **MUST NOW** provide your final report/answer and you are not allowed to call any other tool. Use the required XML wrapper and format instructions to deliver your final report/answer. If information is insufficient or missing, state the limitation explicitly in your final report.';

/**
 * Injected when task completion is signaled via task_status tool.
 * Forces immediate finalization with task completion message.
 * Used in: session-turn-runner.ts (pushed to conversation)
 *
 * CONDITION: forcedFinalTurnReason === 'task_status_completed'
 * Where forcedFinalTurn is set when model calls task_status with status: completed
 */
export const TASK_STATUS_COMPLETED_FINAL_MESSAGE = FINAL_TURN_NOTICE;

/**
 * Injected when consecutive standalone task_status calls are detected.
 * Forces immediate finalization to prevent progress-only loops.
 *
 * CONDITION: forcedFinalTurnReason === 'task_status_only'
 * Where forcedFinalTurn is set after consecutive standalone task_status calls
 */
export const TASK_STATUS_ONLY_FINAL_MESSAGE = FINAL_TURN_NOTICE;

/**
 * Injected when all retry attempts are exhausted.
 * Forces graceful finalization instead of session failure.
 * Used in: session-turn-runner.ts (pushed to conversation)
 *
 * CONDITION: forcedFinalTurnReason === 'retry_exhaustion'
 * Where forcedFinalTurn is set when all retry attempts within a turn fail
 */
export const RETRY_EXHAUSTION_FINAL_MESSAGE = FINAL_TURN_NOTICE;
  
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
  `TURN-FAILED: ${reasons.join(' | ')}`;

const JSON_PARSE_ERROR_MAX = 200;

export const formatJsonParseHint = (error?: string): string => {
  if (error === undefined || error.trim().length === 0) {
    return 'unable to parse JSON (check quotes/braces)';
  }
  const trimmed = error.replace(/\s+/g, ' ').trim();
  if (trimmed === 'empty') return 'empty JSON payload';
  if (trimmed === 'non_string') return 'non-string JSON payload';
  if (trimmed === 'parse_failed') return 'unable to parse JSON (check quotes/braces)';
  if (trimmed === 'expected_json_object') return 'expected a JSON object';
  if (trimmed === 'missing_json_payload') return 'missing JSON payload';
  return trimmed.length > JSON_PARSE_ERROR_MAX ? `${trimmed.slice(0, JSON_PARSE_ERROR_MAX - 3)}...` : trimmed;
};

export const buildInvalidJsonFailure = (base: string, error?: string): string => (
  `${base} invalid_json: ${formatJsonParseHint(error)}`
);

export const buildSchemaMismatchFailure = (errors: string, preview?: string): string => (
  `schema_mismatch: ${errors}${preview !== undefined ? ` (preview: ${preview})` : ''}`
);

export const formatSchemaMismatchSummary = (value?: string): string => {
  if (value === undefined || value.trim().length === 0) return 'unknown validation error';
  const trimmed = value.replace(/\s+/g, ' ').trim();
  const limit = 160;
  return trimmed.length > limit ? `${trimmed.slice(0, limit - 3)}...` : trimmed;
};

/**
 * When final report format doesn't match expected.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: final_report tool called && report_format !== expectedFormat
 */
export const finalReportFormatMismatch = (expected: string, received: string): string =>
  `Final report format mismatch: expected "${expected}", received "${received}". Use the required format and resend your final report/answer.`;

/**
 * When final report content is empty or missing.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: final_report tool called && (format === 'sub-agent' || format === text/markdown/etc)
 *            && (rawPayload ?? contentParam) is undefined or empty
 */
export const FINAL_REPORT_CONTENT_MISSING =
  'Final report content is missing or empty. Provide non-empty content in the required format and resend your final report/answer.';

/**
 * When JSON format expected but got non-JSON.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: final_report tool called && expectedFormat === 'json'
 *            && content_json is undefined after parsing attempts
 */
export const FINAL_REPORT_JSON_REQUIRED =
  'Final report must be a JSON object that matches the provided schema. Provide a JSON object (not a string) and retry.';

/**
 * When Slack Block Kit messages array is missing.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: final_report tool called && expectedFormat === 'slack-block-kit'
 *            && messagesArray is undefined or empty
 */
export const FINAL_REPORT_SLACK_MESSAGES_MISSING =
  'Slack Block Kit final report is missing a `messages` array. Provide valid Block Kit messages and retry.';

/**
 * When final report fails schema validation.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: final_report tool called && payload fails schema validation
 */
export const finalReportSchemaValidationFailed = (summary: string): string =>
  `Final report failed schema validation: ${summary}. Fix the payload to match the schema and resend your final report/answer.`;

/**
 * When tool message fallback fails schema validation.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: tool_message_fallback validation failed
 */
export const toolMessageFallbackValidationFailed = (summary: string): string =>
  `Fallback final report from tool output failed schema validation: ${summary}. Provide your valid final report/answer in the required format.`;

/**
 * When response has content but no valid tool calls and no final report.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: this.finalReport === undefined
 *            && !turnResult.status.finalAnswer
 *            && !sanitizedHasToolCalls (no valid tool calls after sanitization)
 *            && sanitizedHasText (response has non-empty text content)
 *
 * NOTE: This does NOT trigger for reasoning-only responses (sanitizedHasText checks
 *       assistantForAdoption.content, not reasoning blocks)
 */
export const TURN_FAILED_NO_TOOLS_NO_REPORT_CONTENT_PRESENT =
  'Text output detected without any tool calls or final report/answer. Call tool(s) or provide your final report/answer in the required wrapper exactly as instructed.';

/**
 * When response is empty (no tools, no final report).
 * Used in: session-turn-runner.ts via addTurnFailure
 */
export const TURN_FAILED_EMPTY_RESPONSE =
  'Empty response detected: no tool calls and no final report/answer were received. Call a tool or output your final report/answer in the required XML wrapper.';

/**
 * When response contains only reasoning (no visible content, no tools).
 * Used in: session-turn-runner.ts via addTurnFailure
 */
export const TURN_FAILED_REASONING_ONLY =
  'Reasoning-only output detected with no visible answer, tool calls, or final report. Call tools or provide your final report/answer in the required wrapper.';

/**
 * When response contains only reasoning on a final turn (no visible content, no tools).
 * Used in: session-turn-runner.ts via addTurnFailure
 */
export const TURN_FAILED_REASONING_ONLY_FINAL =
  'Reasoning-only output detected with no visible final report/answer. Relax. This is easy. You must now summarize and provide your final report/answer. Output now the XML wrapper exactly as instructed, and then summarize your work in the requested format.';

/**You MUST provide the final report/answer now using the required XML wrapper and do not call any tools.';

/**
 * When response is truncated due to stopReason=length/max_tokens.
 * Used in: session-turn-runner.ts via addTurnFailure
 */
export const turnFailedOutputTruncated = (maxOutputTokens?: number): string => {
  const tokenLimit = maxOutputTokens !== undefined ? ` (maxOutputTokens=${String(maxOutputTokens)})` : '';
  return `Output was truncated (stopReason=length${tokenLimit}). Retry with a shorter response that still includes the required tool calls or a complete final report.`;
};

/**
 * When final turn has no valid final report.
 * Used in: session-turn-runner.ts via addTurnFailure
 */
export const TURN_FAILED_FINAL_TURN_NO_REPORT =
  'Final turn ended without a valid final report. Provide the final report/answer now using the required XML wrapper and do not call any tools.';

/**
 * When tool call parameters are malformed.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: droppedInvalidToolCalls > 0
 * (one or more tool calls had malformed JSON payloads and were dropped)
 */
export const TOOL_CALL_MALFORMED =
  'Tool call payload is not valid JSON for the tool schema. Fix the arguments to match the schema and retry the tool call.';

/**
 * When an unknown tool is called.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: executionStats.unknownToolEncountered === true
 */
export const TURN_FAILED_UNKNOWN_TOOL =
  'Unknown tool name detected. Use only the tool names shown in your tools list (including the full namespace prefix) and retry. If this is the final turn, do not call tools; provide your final report/answer in the required wrapper.';

/**
 * When tool calls exist but none executed.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: had tool calls && executedTools === 0 && not unknown && not dropped-invalid
 */
export const TURN_FAILED_TOOL_CALL_NOT_EXECUTED =
  'Tool calls were not executed. This usually means the tool name is missing its namespace prefix or is not permitted on this turn. Retry with an allowed tool name exactly as listed, or provide your final report/answer if tools are not allowed.';

/**
 * When per-turn tool limit blocks execution.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: toolLimitExceeded === true
 */
export const TURN_FAILED_TOOL_LIMIT_EXCEEDED =
  'Per-turn tool limit reached, so tool calls were rejected. Reduce the number of tool calls in a single turn and retry. If you are done, provide your final report/answer in the required wrapper.';

/**
 * When only task_status was called without any productive tools.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: executedNonProgressBatchTools === 0 && executedProgressBatchTools > 0
 * (task_status was called but no other tools)
 */
export const TURN_FAILED_PROGRESS_ONLY =
  'You called agent__task_status without any other tools, so no work was performed. Call other tools to make progress, or if the task is complete, provide your final report/answer in the required XML wrapper.';

/**
 * When model calls XML wrapper tag as a tool instead of outputting it as text.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: Model emits tool_use with name matching ai-agent-{nonce}-FINAL pattern
 */
export const turnFailedXmlWrapperAsTool = (sessionNonce: string, format: string): string =>
  `You called the XML wrapper tag (ai-agent-${sessionNonce}-FINAL) as a tool, but it must be plain text in your response. Do NOT use tool-calling syntax; output the XML wrapper directly. Example: <ai-agent-${sessionNonce}-FINAL format="${format}">YOUR CONTENT HERE</ai-agent-${sessionNonce}-FINAL>`;

/**
 * Check if a tool name matches the XML final report tag pattern.
 * Pattern: ai-agent-{8-hex-chars}-FINAL
 */
export const isXmlFinalReportTagName = (name: string): boolean =>
  /^ai-agent-[a-f0-9]{8}-FINAL$/i.test(name);

// =============================================================================
// TOOL RESULTS
// Messages returned as tool execution results (seen by LLM)
// =============================================================================

/**
 * Error when the XML wrapper tag is called as a tool instead of being output as text.
 * Used in: llm-providers/base.ts injectMissingToolResults()
 */
export const XML_WRAPPER_CALLED_AS_TOOL_RESULT =
  'You called the XML wrapper tag as if it were a tool. The XML wrapper is NOT a tool — it is plain text you output directly in your response. Do NOT use tool calling for your final report/answer. Instead, write the XML tags directly in your response text, exactly as instructed.';

/**
 * Placeholder for tool output when context budget exceeded.
 * Returned as tool result to the LLM.
 * Used in: session-tool-executor.ts, session-turn-runner.ts
 *
 * CONDITION: Tool output would exceed context budget
 * (content.length > 0 && content !== TOOL_NO_OUTPUT checked to avoid counting)
 */
export const TOOL_NO_OUTPUT = '(tool failed: context window budget exceeded)';

const UNKNOWN_TOOL_FAILURE_PREFIX = 'Unknown tool `';

/**
 * Unknown tool name (tool call does not match any available tool).
 * Returned as tool result to the LLM.
 * Used in: session-tool-executor.ts, session-turn-runner.ts, tests
 */
export const unknownToolFailureMessage = (name: string): string =>
  `${UNKNOWN_TOOL_FAILURE_PREFIX}${name}\`: you called tool \`${name}\` but it does not match any of the tools in this session. Review carefully the tools available and copy the tool name verbatim. Tool names are made of a namespace (or tool provider) + double underscore + the tool name of this namespace/provider. When you call a tool, you must include both the namespace/provider and the tool name. You may now repeat the call to the tool, but this time you MUST supply the exact tool name as given in your list of tools.`;

export const isUnknownToolFailureMessage = (content: string): boolean =>
  content.includes(UNKNOWN_TOOL_FAILURE_PREFIX);

// =============================================================================
// XML PROTOCOL ERRORS
// Sent via recordTurnFailure callback (become part of TURN-FAILED message)
// =============================================================================

/**
 * Final report payload is not valid JSON (XML mode).
 * Used in: xml-transport.ts via recordTurnFailure
 *
 * CONDITION: XML mode && final_report tag found && JSON.parse(payload) fails
 */
export const XML_FINAL_REPORT_NOT_JSON =
  'Final report payload is not valid JSON. Provide a JSON object that matches the final-report schema and retry.';

/**
 * Tool payload is not valid JSON (XML mode).
 * Used in: xml-transport.ts via recordTurnFailure
 *
 * CONDITION: XML mode && tool tag found && JSON.parse(payload) fails
 */
export const xmlToolPayloadNotJson = (toolName: string): string =>
  `Tool \`${toolName}\` payload is not valid JSON. Provide a JSON object for the tool parameters and retry.`;

/**
 * XML tag slot mismatch.
 * Used in: xml-transport.ts via recordTurnFailure
 *
 * CONDITION: XML mode && tag slot attribute !== expected nonce/slot for this turn
 */
export const xmlSlotMismatch = (capturedSlot: string): string =>
  `Tag ignored: slot '${capturedSlot}' does not match the current nonce/slot for this turn. Use the exact slot id from XML-NEXT and retry.`;

/**
 * XML missing closing tag.
 * Used in: xml-transport.ts via recordTurnFailure
 *
 * CONDITION: XML mode && opening tag found && corresponding closing tag not found
 */
export const xmlMissingClosingTag = (capturedSlot: string): string =>
  `Malformed XML: missing closing tag for '${capturedSlot}'. Close the tag exactly as shown and retry.`;

/**
 * XML malformed - nonce/slot/tool mismatch.
 * Used in: xml-transport.ts via recordTurnFailure
 *
 * CONDITION: XML mode && tag validation fails (nonce/slot mismatch or empty content)
 */
export const xmlMalformedMismatch = (slotInfo: string): string =>
  `Malformed XML: nonce/slot/tool mismatch or empty content for '${slotInfo}'. Use the exact nonce and slot, include non-empty content, and retry.`;

/**
 * Structured output (json, slack-block-kit) truncated due to stopReason=length.
 * Model must retry with shorter output.
 * Used in: xml-transport.ts via recordTurnFailure
 *
 * CONDITION: XML mode && final report detected && stopReason=length && format is structured
 */
export const turnFailedStructuredOutputTruncated = (maxOutputTokens?: number): string => {
  const tokenLimit = maxOutputTokens !== undefined ? ` (${String(maxOutputTokens)} tokens)` : '';
  return `Your response was truncated (stopReason=length) because it exceeded the output token limit${tokenLimit}. ` +
    'Retry with a shorter output that still delivers your complete final report/answer in the required XML wrapper.';
};


// =============================================================================
// TASK STATUS INSTRUCTIONS
// Single source of truth for agent__task_status tool guidance.
// Used by: internal-provider.ts (system prompt, tool schema), xml-tools.ts (XML-NEXT)
// =============================================================================

/**
 * Instructions for agent__task_status tool.
 * Used in: internal-provider.ts buildInstructions()
 */
export const TASK_STATUS_TOOL_INSTRUCTIONS = `#### agent__task_status — Task Status Feedback

**Purpose:** This tool provides live feedback to the user about your accomplishments, pendings and immediate goals related to the task assigned to you. It is purely for REPORTING. It does not perform any actions and it does not help in completing your task. Use it ONLY to communicate your progress to the user, while you work on the assigned task.

All its fields are MANDATORY:

Field \`status\`: indicates your current state in the task:
- "starting": You just started and you are planning your actions
- "in-progress": You are currently working on this task - you are not yet ready to provide your final report/answer
- "completed": You completed the task and you are now ready to provide your final report/answer

Field \`done\`: A brief summary of what you have accomplished so far:
- Describe which parts of the task you have completed - not the tools you run - the user cares about results, not methods.
- Focus on outcomes and findings relevant to the task.

Field \`pending\`: A brief summary of what is still pending or left to do to complete the task:
- Describe which parts of the task are still pending or need further work.
- Focus on outstanding items that are necessary to complete the task.

Field \`now\`: A brief description of the actions you are taking IN THIS TURN:
- Describe what tools you are calling alongside this status update
- Explain what you are trying to achieve with your current actions
- If you are providing your final report, state that here

Field  \`ready_for_final_report\`:
- Set to true when you have enough information to provide your final report/answer
- Set to false when you still need to gather more information or perform more actions

Field  \`need_to_run_more_tools\`:
- Set to true when you need to run more tools
- Set to false if you are done with tools

What to report when tools fail:
- Be honest: if you are facing difficulties or limitations, clearly state them in the \`done\` field.
- Try to work around failures: explain in the \`now\` field what alternative steps you are taking to overcome the issues.
- If you tried everything and you still cannot proceed to complete the task, provide your final report/answer, honestly stating the limitations you faced.

**WRONG (wastes a turn):**
- Calling agent__task_status with now="Searching for more data" without calling any data retrieval tool

**RIGHT:**
- Calling agent__task_status TOGETHER with other tools in the same turn

**Good Examples:**
- status: "starting", done: "Planning...", pending: "Find error logs", now: "gather system error logs for the last 15 mins", ready_for_final_report: false, need_to_run_more_tools: true, and at the same time executing the right tools for log retrieval
- status: "in-progress", done: "got error logs for the last 15 mins", pending: "Find the specific error", now: "expand search to 30 mins", ready_for_final_report: false, need_to_run_more_tools: true, and at the same time executing the right tools for expanded log retrieval
- status: "in-progress", done: "Found relevant logs", pending: "Identify root cause", now: "Examining source code", ready_for_final_report: true, need_to_run_more_tools: true, and at the same time executing the right tools for code analysis
- status: "completed", done: "Found 3 critical errors", pending: "All done", now: "Compile the final report/answer", ready_for_final_report: true, need_to_run_more_tools: false, and at the same time providing the final report/answer

**Best Practices:**
- Call agent__task_status alongside other tools or your final report/answer (calling it alone wastes turns)
- Include clear descriptions for "done", "pending" and "now", for the user to understand your progress
- Be honest about your limitations and failures, and explain how you are trying to overcome them
- Never call this tool repeatedly without calling other tools - the system will detect repeated standalone calls and enforce finalization
`;

/**
 * Brief instructions for XML-NEXT progress slot.
 * Used in: xml-tools.ts renderXmlNext()
 */
export const TASK_STATUS_TOOL_INSTRUCTIONS_BRIEF =
  'Update the user about your current task status and progress:';

/**
 * Batch-specific rules for task_status.
 * Used in: internal-provider.ts buildInstructions() batch section
 */
export const TASK_STATUS_TOOL_BATCH_RULES = `- Include at most one task_status per batch
- task_status updates the user; to perform actions, use other tools in the same batch
- task_status can be called standalone to track task progress`;

/**
 * Mandatory workflow line mentioning task_status for XML-NEXT.
 * Used in: xml-tools.ts renderXmlNext()
 */
export const TASK_STATUS_TOOL_WORKFLOW_LINE =
  'Call one or more tools to collect data or perform actions (optionally include task_status to update the user)';

// XML System Notices (moved from xml-tools.ts to keep all LLM-facing strings here)
export interface XmlNextTemplatePayload {
  nonce: string;
  turn: number;
  maxTurns?: number;
  tools: { name: string; schema?: Record<string, unknown> }[];
  slotTemplates: { slotId: string; tools: string[] }[];
  taskStatusToolEnabled: boolean;
  expectedFinalFormat: string;
  attempt: number;
  maxRetries: number;
  contextPercentUsed: number;
  hasExternalTools: boolean;
}

export interface XmlPastTemplateEntry {
  slotId: string;
  tool: string;
  status: 'ok' | 'failed';
  durationText?: string;
  request: string;
  response: string;
}

export interface XmlPastTemplatePayload {
  entries: XmlPastTemplateEntry[];
}

export const renderXmlNextTemplate = (payload: XmlNextTemplatePayload): string => {
  const { nonce, turn, maxTurns, attempt, maxRetries, contextPercentUsed, hasExternalTools, expectedFinalFormat, taskStatusToolEnabled } = payload;
  const finalSlotId = `${nonce}-FINAL`;
  const lines: string[] = [];

  lines.push('# System Notice');
  lines.push('');

  // Turn info with optional retry count
  const turnInfo = `This is turn/step ${String(turn)}${maxTurns !== undefined ? ` of ${String(maxTurns)}` : ''}`;
  const retryInfo = attempt > 1 ? ` (retry ${String(attempt)} of ${String(maxRetries)})` : '';
  lines.push(`${turnInfo}${retryInfo}.`);
  lines.push(`Your context window is ${String(contextPercentUsed)}% full.`);
  lines.push('');

  // Guidance based on tool availability
  if (hasExternalTools) {
    lines.push('You now need to decide your next move:');
    lines.push('EITHER');
    lines.push('- Call tools to advance your task following the main prompt instructions (pay attention to tool formatting and schema requirements).');
    if (taskStatusToolEnabled) {
      lines.push('- Together with these tool calls, also call `agent__task_status` to explain what you are doing and why you are calling these tools.');
    }
    lines.push('OR');
    lines.push(`- Provide your final report/answer in the expected format (${expectedFinalFormat}) using the XML wrapper (\`<ai-agent-${finalSlotId} format="${expectedFinalFormat}">\`)`);
    if (taskStatusToolEnabled) {
      lines.push('');
      lines.push('Call `agent__task_status` to track your progress and mark tasks as complete.');
    }
  } else {
    lines.push(`You MUST now provide your final report/answer in the expected format (${expectedFinalFormat}) using the XML wrapper (\`<ai-agent-${finalSlotId} format="${expectedFinalFormat}">\`).`);
  }
  lines.push('');

  return lines.join('\n');
};

export const renderXmlPastTemplate = (past: XmlPastTemplatePayload): string => {
  const lines: string[] = [];
  lines.push('# System Notice');
  lines.push('');
  lines.push('## Previous Turn Tool Responses');
  lines.push('');
  past.entries.forEach((entry) => {
    const duration = entry.durationText !== undefined ? ` ${entry.durationText}` : '';
    lines.push(`<ai-agent-${entry.slotId} tool="${entry.tool}" status="${entry.status}"${duration}>`);
    lines.push('<request>');
    lines.push(entry.request);
    lines.push('</request>');
    lines.push('<response>');
    lines.push(entry.response);
    lines.push('</response>');
    lines.push(`</ai-agent-${entry.slotId}>`);
    lines.push('');
  });
  return lines.join('\n');
};

// =============================================================================
// FINAL REPORT INSTRUCTIONS
// Single source of truth for agent__final_report guidance.
// Used by: internal-provider.ts (system prompt)
// =============================================================================

/**
 * Final report instructions with XML wrapper.
 * Used in: internal-provider.ts buildInstructions()
 *
 * Structure optimized for first-try success:
 * 1. Critical rules first (what MUST happen)
 * 2. Pre-response checklist before example (reinforces "first in response")
 * 3. XML wrapper example
 * 4. Format-specific details last
 */
export const finalReportXmlInstructions = (
  formatId: string,
  formatDescription: string,
  schemaBlock: string,
  sessionNonce?: string
): string => {
  // Caller MUST provide a nonce - without it, LLM will output literal 'NONCE-FINAL'
  if (sessionNonce === undefined) {
    throw new Error('sessionNonce is required for XML final report instructions');
  }
  const slotId = `${sessionNonce}-FINAL`;
  // For slack-block-kit: show array example (messages array directly, no wrapper)
  // For json: show object example
  // For text formats: show text placeholder
  const exampleContent = formatId === 'slack-block-kit'
    ? '[ { "blocks": [ ... ] } ]'
    : formatId === 'json'
      ? '{ ... your JSON here ... }'
      : '[Your final report/answer here]';
  const slackMrkdwnGuidance = formatId === 'slack-block-kit'
    ? `\n${SLACK_BLOCK_KIT_MRKDWN_RULES}\n`
    : '';
  return `
## MANDATORY READ-FIRST: How to Provide Your Final Report/Answer

You run in agentic/investigation mode with strict output formatting requirements. Depending on the user request and the task assigned to you, you may need to run several turns/steps, calling tools to gather information or perform actions, adapting to the data discovered, before providing your final report/answer. You are expected to run an iterative process, making use of the available tools and following the instructions provided, to complete the task assigned to you.

The system allows you to perform a limited number of turns to complete the task, monitors your context window size, and enforces certain limits.

Your final report/answer may be positive (you successfully completed the task) or negative (you were unable to complete the task due to limitations, tool failures, or insufficient information). If for any reason you failed to complete the task successfully, you **MUST** clearly state it in your final report/answer. You are expected to be honest and transparent about your limitations and failures.

To provide your final report/answer (or when the system will tell you to do so), you **MUST** follow these instructions carefully:
1. Your final report/answer **MUST** be in your output (it is not a tool call).
1. Your final report/answer **MUST** use the XML wrapper shown below, at your output
2. The opening XML tag **MUST** be the **FIRST** thing in your output
3. Do NOT output plain text, greetings, or explanations outside the XML tags
4. ALL content must be between the opening and closing XML tags

**Pre-Final Report/Answer Checklist:**
- [ ] Your final report/answer starts with \`<ai-agent-${slotId}\` (no text before it)
- [ ] Content matches ${formatId} format requirements, including any schema if provided
- [ ] Your final report/answer ends with \`</ai-agent-${slotId}>\` (no text after it)
- [ ] No text outside the XML tags

**Required XML Wrapper:**
\`\`\`
<ai-agent-${slotId} format="${formatId}">
${exampleContent}
</ai-agent-${slotId}>
\`\`\`

**Output Format: ${formatId}**
${formatDescription}
${schemaBlock}
${slackMrkdwnGuidance}

Your final report/answer content must follow any instructions provided to accurately and precisely reflect the information available. If you encountered limitations, tool failures that you couldn't overcome, or you were unable to complete certain aspects of the task, clearly state these limitations in your final report/answer.

In some cases, you may receive requests that are irrelevant to your instructions, such as greetings, casual conversation, or questions outside your domain. In such cases, be polite and helpful, and respond to the best of your knowledge, stating that the information provided is outside your scope, but always adhere to the final report/answer format and XML wrapper instructions provided above.

Reminders:
1. You should deliver your final report/answer on your output with the given XML wrapper. Your final report/answer is NOT a tool call.
2. You should be transparent about your limitations and failures in your final report/answer.
3. You should provide your final report/answer in in the requested output format (${formatId}) and according to any structure/schema instructions given.
`;
};

/**
 * Format-specific required fields for tool-based instructions.
 */
export const FINAL_REPORT_FIELDS_JSON =
  '  - `report_format`: "json".\n  - `content_json`: MUST match the required JSON Schema exactly.';

export const FINAL_REPORT_FIELDS_SLACK =
  '  - `report_format`: "slack-block-kit".\n  - `messages`: array of Slack Block Kit messages (no plain `report_content`).\n    • Up to 20 messages, each with ≤50 blocks. Sections/context mrkdwn ≤2000 chars; headers plain_text ≤150.';

export const SLACK_BLOCK_KIT_MRKDWN_RULES_TITLE = 'Slack mrkdwn rules (NOT GitHub Markdown)';

export const SLACK_BLOCK_KIT_MRKDWN_RULES = `### ${SLACK_BLOCK_KIT_MRKDWN_RULES_TITLE}
- All section/context text must be Slack *mrkdwn* (not GitHub Markdown).
- Only use these block types: \`header\` (plain_text), \`section\` (mrkdwn), \`divider\`, \`context\` (mrkdwn elements). Do NOT emit other block types.
- DO NOT use Markdown headings (\`#\`, \`##\`, \`###\`) or Markdown tables (\`|---|\`). Slack will render these literally.
- Titles go in **header** blocks (plain_text). Subheadings inside sections must be bold lines (e.g., \`*Section Title*\`) followed by a newline.
- Headers are **plain_text**: do NOT use emoji shortcodes like \`:database:\` there. Use Unicode emoji only (e.g., \`🗄️\`).
- Allowed formatting: \`*bold*\`, \`_italic_\`, \`~strikethrough~\`, \`inline code\`, fenced code blocks (\`\`\`code\`\`\`), bullets (\`•\` or \`-\`).
- Links must use Slack format: \`<https://example.com|link text>\`. Do NOT use \`[text](url)\`.
- Mentions are allowed when relevant: \`<@U...>\`, \`<#C...>\`, \`<!subteam^ID>\`, \`<!here>\`, \`<!channel>\`, \`<!everyone>\`. Avoid \`@here/@channel/@everyone\` unless explicitly asked.
- Escape special characters in text: \`&\` → \`&amp;\`, \`<\` → \`&lt;\`, \`>\` → \`&gt;\`.
- Never use HTML tags, GitHub Markdown extensions, Mermaid fences, or raw JSON inside mrkdwn messages.

**Quick templates (use these patterns):**
Header block:
  \`{ "type": "header", "text": { "type": "plain_text", "text": "Title" } }\`
Section with subheading + bullets:
  \`{ "type": "section", "text": { "type": "mrkdwn", "text": "*Section Title*\\n• Item one\\n• Item two" } }\`

**Tables in slack-block-kit**:
Slack does not support tables. Never use Markdown tables (e.g., \`|---|\`), or HTML tables.

For up to 10 key-value pairs use Block Kit \`section.fields\` (do not add more than 10 fields):
- Each field MUST contain ONE key/value pair (\`*Label*\\nValue\`). Do NOT put all keys in one field and all values in another.
- Fields render in a single column grid (field 1 label above, field 1 value below,field 2 label 3rd, field 2 key 4th and so on). So, they are rendered vertically, not horizontally.
Example: simple vertical key/value layout (fields). Fields shown one below the other:
  \`{ "type": "section", "fields": [ { "type": "mrkdwn", "text": "*Monthly Revenue*\\n$2.4M" }, { "type": "mrkdwn", "text": "*Active Users*\\n45,000" }, { "type": "mrkdwn", "text": "*Support Tickets*\\n23 (resolved)" }, { "type": "mrkdwn", "text": "*System Health*\\n98.5%" } ] }\`

To show a small multi-column and multi-row table, use code blocks and emulate with spaces and ASCII art some tabular data:
- keep them simple and short
- assume a monospace font
Example:
\`\`\`
┌──────────┬─────────┬────────┬────────┐
│ Month    │ Revenue │ Users  │ Growth │
├──────────┼─────────┼────────┼────────┤
│ January  │ $2.1M   │ 42,000 │ +5.0%  │
│ February │ $2.4M   │ 45,000 │ +7.0%  │
│ March    │ $2.5M   │ 47,500 │ +5.5%  │
└──────────┴─────────┴────────┴────────┘
\`\`\`
Do not make this too wide; keep it within 50 characters width and up to 10 rows.
`;

export const finalReportFieldsText = (formatId: string): string =>
  `  - \`report_format\`: "${formatId}".\n  - \`report_content\`: the content of your final report, in the requested format.`;

/**
 * Mandatory rules for tools section.
 * Used in: internal-provider.ts buildInstructions()
 */
export const MANDATORY_TOOLS_RULES = `### MANDATORY RULE FOR TOOLS
- You run in agentic mode, interfacing with software tools with specific formatting requirements.
- Always respond with valid tool calls when invoking tools, and provide your final report/answer in the required XML final wrapper.
- The CONTENT of your final report must be delivered inside the XML final wrapper only (no plain text outside the tags).`;

/**
 * Mandatory rules for XML final report.
 * Used in: internal-provider.ts buildInstructions() (xml-final final-report slot)
 *
 * Rewritten to avoid confusion when no external tools are available.
 * Focuses on output format requirements, not tool usage.
 */
export const MANDATORY_XML_FINAL_RULES = `### RESPONSE FORMAT RULES
- You operate in agentic mode with strict output formatting requirements
- Your response MUST be wrapped in the XML tags shown above
- Never respond with plain text outside the XML wrapper
- If tools are available and you need to call them, do so; your FINAL response always uses the XML wrapper
- The XML tag MUST be the first content in your response — no greetings, no preamble`;
