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
  'System notice: this is the final turn. You **MUST NOW** provide your final report/answer. Do NOT attempt to call any other tool. Read carefully the final report/answer instructions and provide your final report/answer based on the information already gathered. If the information is insufficient, provide the best possible answer based on what you have and note the limitation in your final report/answer.';

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
  `TURN-FAILED: ${reasons.join(' | ')}.`;

/**
 * When final report format doesn't match expected.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: final_report tool called && report_format !== expectedFormat
 */
export const finalReportFormatMismatch = (expected: string, received: string): string =>
  `Final report format must be ${expected}. Received ${received}.`;

/**
 * When final report content is empty or missing.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: final_report tool called && (format === 'sub-agent' || format === text/markdown/etc)
 *            && (rawPayload ?? contentParam) is undefined or empty
 */
export const FINAL_REPORT_CONTENT_MISSING =
  'Final report content missing; provide your final report in the requested format.';

/**
 * When JSON format expected but got non-JSON.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: final_report tool called && expectedFormat === 'json'
 *            && content_json is undefined after parsing attempts
 */
export const FINAL_REPORT_JSON_REQUIRED =
  'Final report must be JSON per schema; received non-JSON content.';

/**
 * When Slack Block Kit messages array is missing.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: final_report tool called && expectedFormat === 'slack-block-kit'
 *            && messagesArray is undefined or empty
 */
export const FINAL_REPORT_SLACK_MESSAGES_MISSING =
  'Final report missing messages array; provide Slack Block Kit messages.';

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
  'No progress made in this turn: No tools called, no final report/answer provided, but unexpected content is present in your output.\n- If you believe you called tools, the system did not detect any tool calls. This usually means tool calls were not recognized at all and they leaked to your output.\n- If you believe you provided a final report/answer, it was not detected either. Ensure the final report is correctly formatted as instructed.\nRetry NOW: pay attention to the required syntax for tool calls and your final report/answer. Try again NOW';

/**
 * When tool call parameters are malformed.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: droppedInvalidToolCalls > 0
 * (one or more tool calls had malformed JSON payloads and were dropped)
 */
export const TOOL_CALL_MALFORMED =
  'Tool call payload malformed; provide JSON arguments matching the schema.';

/**
 * When only task_status was called without any productive tools.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: executedNonProgressBatchTools === 0 && executedProgressBatchTools > 0
 * (task_status was called but no other tools)
 */
export const TURN_FAILED_PROGRESS_ONLY =
  'You called agent__task_status without calling any other tools together with it. agent__task_status only reports status to the user — it does NOT perform any actions. You must either:\n\n1. call other tools to collect data or perform actions (following their schemas precisely), or\n2. if your task is complete, call agent__task_status with status="completed" and then provide your final report/answer (using the XML wrapper as instructed).\n\n**NOTE:** agent__task_status can be called standalone, but consecutive standalone calls will force you to provide your final report.';

/**
 * When model calls XML wrapper tag as a tool instead of outputting it as text.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: Model emits tool_use with name matching ai-agent-{nonce}-FINAL pattern
 */
export const turnFailedXmlWrapperAsTool = (sessionNonce: string, format: string): string =>
  `You called the XML wrapper tag (ai-agent-${sessionNonce}-FINAL) as a tool. This is incorrect — the XML wrapper is NOT a tool, it is plain text that you output directly in your response.\n\nTo provide your final report/answer:\n1. Do NOT use tool calling syntax\n2. Write the XML wrapper directly in your response text\n3. Example: <ai-agent-${sessionNonce}-FINAL format="${format}">YOUR CONTENT HERE</ai-agent-${sessionNonce}-FINAL>\n\nThe system is waiting for your final report/answer as XML text output, not as a tool call.`;

// =============================================================================
// SYSTEM NOTICES (LLM-facing only)
// =============================================================================

/**
 * After empty response - no tools called and no final report.
 * Used in: session-turn-runner.ts (pushed as system retry message)
 *
 * CONDITION: isEmptyWithoutTools && turnResult.hasReasoning !== true
 * Where isEmptyWithoutTools = !turnResult.status.finalAnswer
 *                             && !turnResult.status.hasToolCalls
 *                             && (turnResult.response === undefined || turnResult.response.trim().length === 0)
 *
 * NOTE: Reasoning-only responses skip this check (hasReasoning === true bypasses)
 */
export const EMPTY_RESPONSE_RETRY_NOTICE =
  'System notice: No progress made in this turn: no tools called and no final report/answer provided. To advance you MUST call tools or provide your final report/answer. Review carefully the provided instructions and tools (if any), decide your next action(s), and follow the instructions precisely to continue. If you believe you called tools or provided a final report, it did not work: ensure the tool calls and final report are correctly formatted as per the instructions. Try again NOW.';

// =============================================================================
// FINAL REPORT REMINDER
// Injected to guide correct final_report usage
// =============================================================================

/**
 * Builds a detailed reminder for how to call final_report correctly.
 * Used in: session-turn-runner.ts buildFinalReportReminder (system retry message)
 *
 * CONDITION: Called when final report validation fails or on final turn without final answer
 * - After FINAL_TURN_NOTICE (isFinalTurn && !turnResult.status.finalAnswer)
 * - After final report validation errors (format mismatch, content missing, etc.)
 */
export const finalReportReminder = (
  format: string,
  formatDescription: string,
  contentGuidance: string
): string =>
  `System notice: provide your final report/answer in the required XML wrapper with format="${format}" (${formatDescription}), and ${contentGuidance}.`;

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
  `${UNKNOWN_TOOL_FAILURE_PREFIX}${name}\`: you called tool \`${name}\` but it does not match any of the tools in this session. Review carefully the tools available and copy the tool name verbatim. Tool names are usually composed of a namespace (or tool provider) + double underscore + the tool name of this namespace/provider. You may now repeat the call to the tool, but this time you MUST supply the exact tool name as given in your list of tools.`;

export const isUnknownToolFailureMessage = (content: string): boolean =>
  content.includes(UNKNOWN_TOOL_FAILURE_PREFIX);

// =============================================================================
// XML PROTOCOL ERRORS
// Sent via onTurnFailure callback (become part of TURN-FAILED message)
// =============================================================================

/**
 * Final report payload is not valid JSON (XML mode).
 * Used in: xml-transport.ts via onTurnFailure
 *
 * CONDITION: XML mode && final_report tag found && JSON.parse(payload) fails
 */
export const XML_FINAL_REPORT_NOT_JSON =
  'final-report payload is not valid JSON. Provide the correct JSON object according to the final-report/answer instructions and schema.';

/**
 * Tool payload is not valid JSON (XML mode).
 * Used in: xml-transport.ts via onTurnFailure
 *
 * CONDITION: XML mode && tool tag found && JSON.parse(payload) fails
 */
export const xmlToolPayloadNotJson = (toolName: string): string =>
  `Tool \`${toolName}\` payload is not valid JSON. Provide a JSON object.`;

/**
 * XML tag slot mismatch.
 * Used in: xml-transport.ts via onTurnFailure
 *
 * CONDITION: XML mode && tag slot attribute !== expected nonce/slot for this turn
 */
export const xmlSlotMismatch = (capturedSlot: string): string =>
  `Tag ignored: slot '${capturedSlot}' does not match the current nonce/slot for this turn.`;

/**
 * XML missing closing tag.
 * Used in: xml-transport.ts via onTurnFailure
 *
 * CONDITION: XML mode && opening tag found && corresponding closing tag not found
 */
export const xmlMissingClosingTag = (capturedSlot: string): string =>
  `Malformed XML: missing closing tag for '${capturedSlot}'.`;

/**
 * XML malformed - nonce/slot/tool mismatch.
 * Used in: xml-transport.ts via onTurnFailure
 *
 * CONDITION: XML mode && tag validation fails (nonce/slot mismatch or empty content)
 */
export const xmlMalformedMismatch = (slotInfo: string): string =>
  `Malformed XML: nonce/slot/tool mismatch or empty content for '${slotInfo}'.`;

/**
 * Structured output (json, slack-block-kit) truncated due to stopReason=length.
 * Model must retry with shorter output.
 * Used in: xml-transport.ts via onTurnFailure
 *
 * CONDITION: XML mode && final report detected && stopReason=length && format is structured
 */
export const turnFailedStructuredOutputTruncated = (maxOutputTokens?: number): string => {
  const tokenLimit = maxOutputTokens !== undefined ? ` (${String(maxOutputTokens)} tokens)` : '';
  return `Your response was truncated (stopReason=length) because it exceeded the output token limit${tokenLimit}. ` +
    `Repeat the same final-report, but this time keep your output within the required token count limit.`;
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

Provides live feedback to the user about your accomplishments, pending items and goals. Use this tool as frequently as necessary to let the user know what you are doing, while you work on the task assigned to you. This tool is only updating the user. It does not perform any other actions.

**Status Values:**
- "starting": You just started and you are planning your actions
- "in-progress": You are currently working on this task - you are not yet ready to provide your final report/answer
- "completed": You completed the task and you are now ready to provide your final report/answer

**Boolean Flags:**
- "ready_for_final_report": Set to true when you have enough information to provide your final report/answer, false otherwise
- "need_to_run_more_tools": Set to true when you need to run more tools, false if you are done with tools

**Good Examples:**
- status: "starting", done: "Planning...", pending: "Find error logs", now: "gather system error logs for the last 15 mins", ready_for_final_report: false, need_to_run_more_tools: true
- status: "in-progress", done: "got error logs for the last 15 mins", pending: "Find the specific error", now: "expand search to 30 mins", ready_for_final_report: false, need_to_run_more_tools: true
- status: "in-progress", done: "Found relevant logs", pending: "Could verify more sources", now: "Deciding next step", ready_for_final_report: true, need_to_run_more_tools: true
- status: "completed", done: "Found 3 critical errors", pending: "All done", now: "Compile the final report/answer", ready_for_final_report: true, need_to_run_more_tools: false

**Mandatory Rules:**
- Call agent__task_status alongside other tools when possible (calling it alone wastes turns)
- Include clear descriptions for "done", "pending" and "now", for the user to understand your progress
- Set status to "completed" with ready_for_final_report: true and need_to_run_more_tools: false ONLY when you are truly done - the system will force you to provide your final report/answer once all three align
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
