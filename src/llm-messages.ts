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
 * When only progress_report was called without any productive tools.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: executedNonProgressBatchTools === 0 && executedProgressBatchTools > 0
 * (progress_report was called but no other tools)
 */
export const TURN_FAILED_PROGRESS_ONLY =
  'You called agent__progress_report without calling any other tools together with it. agent__progress_report does not perform any actions other than showing a message to the user. To make actual progress, you must either:\n\n1. call other tools to collect data or perform actions (following their schemas precisely), or\n2. if your task is complete and you can now conclude, provide your final report/answer (using the XML wrapper as instructed - your final report/answer is not a tool call)';

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
// PROGRESS REPORT INSTRUCTIONS
// Single source of truth for agent__progress_report tool guidance.
// Used by: internal-provider.ts (system prompt, tool schema), xml-tools.ts (XML-NEXT)
// =============================================================================

/**
 * Tool description for agent__progress_report schema.
 * Used in: internal-provider.ts listTools()
 */
export const PROGRESS_TOOL_DESCRIPTION =
  'Report current progress to user (max 15 words). Only call when also invoking other tools. Never call standalone.';

/**
 * Full instructions for agent__progress_report.
 * Used in: internal-provider.ts buildInstructions()
 */
export const PROGRESS_TOOL_INSTRUCTIONS = `#### agent__progress_report — Progress Updates for the User

When calling other tools, inject also a call to agent__progress_report, to provide information to the user about your current status and the reason you are calling the other tools.

**Good examples:**
- "Found the data about X, now searching for Y and Z": let the user know you have already acquired information about X and now you are calling more tools to search for Y and Z
- "Discovered how X works, now checking if it can also do Y or Z": informs the user that you found how X works, and you are now calling more tools to check for Y and Z
- "Looks like X is not available, trying Y and Z instead": updates the user that X is not available, and you are calling other tools to try Y and Z

**Bad examples:**
- "I am calling tool X": the user is not aware of your tools - the useful information is why you call a tool, not that you call it
- "I am now ready to provide my final report": incorrect, final report is NOT a tool call - provide your final report instead
- "I now have all the information to complete my task": incorrect, if you have completed your task, provide your final report instead of calling progress_report
- "Extracted X and Y": too vague, does not inform the user about your next steps

**Mandatory Rules about agent__progress_report:**
- Call agent__progress_report ONLY when you are ALSO calling other tools
- NEVER call agent__progress_report standalone; if you are not calling other tools, skip it entirely
- Keep messages concise (max 20 words), no formatting or newlines
- agent__progress_report only informs the user; it does NOT perform actions; you must call other tools to do actual work
`;

/**
 * Brief instructions for XML-NEXT progress slot.
 * Used in: xml-tools.ts renderXmlNext()
 */
export const PROGRESS_TOOL_INSTRUCTIONS_BRIEF =
  'Update the user about your current status (only when also calling other tools):';

/**
 * Batch-specific rules for progress_report.
 * Used in: internal-provider.ts buildInstructions() batch section
 */
export const PROGRESS_TOOL_BATCH_RULES = `- Include at most one progress_report per batch
- progress_report updates the user; to perform actions, use other tools in the same batch
- If you have no other tools to call, do NOT include progress_report`;

/**
 * Mandatory workflow line mentioning progress_report for XML-NEXT.
 * Used in: xml-tools.ts renderXmlNext()
 */
export const PROGRESS_TOOL_WORKFLOW_LINE =
  'Call one or more tools to collect data or perform actions (optionally include progress_report to update the user)';

// XML System Notices (moved from xml-tools.ts to keep all LLM-facing strings here)
export interface XmlNextTemplatePayload {
  nonce: string;
  turn: number;
  maxTurns?: number;
  tools: { name: string; schema?: Record<string, unknown> }[];
  slotTemplates: { slotId: string; tools: string[] }[];
  progressSlot?: { slotId: string };
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
  const { nonce, turn, maxTurns, attempt, maxRetries, contextPercentUsed, hasExternalTools, expectedFinalFormat } = payload;
  const finalSlotId = `${nonce}-FINAL`;
  const lines: string[] = [];

  lines.push('# System Notice');
  lines.push('');

  // Turn info with optional retry count
  const turnInfo = `This is turn ${String(turn)}${maxTurns !== undefined ? ` of ${String(maxTurns)}` : ''}`;
  const retryInfo = attempt > 1 ? ` (retry ${String(attempt)} of ${String(maxRetries)})` : '';
  lines.push(`${turnInfo}${retryInfo}.`);
  lines.push(`Your context window is ${String(contextPercentUsed)}% full.`);
  lines.push('');

  // Guidance based on tool availability
  if (hasExternalTools) {
    lines.push('You now need to decide your next move:');
    lines.push('1. Call tools to advance your task (pay attention to their formatting and schema requirements).');
    lines.push(`2. Provide your final report/answer in the expected format (${expectedFinalFormat}) using the XML wrapper (\`<ai-agent-${finalSlotId} format="${expectedFinalFormat}">\`)`);
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

You run in agentic/investigation mode with strict output formatting requirements. Depending on the user request and the task at hand, you may need to run several turns/steps, calling tools to gather information or perform actions, adapting to the data at hand, before providing your final report/answer. When tools are available and applicable, and you can utilize them to complete the task. You are expected to run an iterative process, making use of the available tools to complete the task assigned to you.

The system allows you to perform a limited number of turns to complete the task, monitors your context window size, and enforces certain limits.

Once you are ready to provide your final report/answer (or when the system will tell you to do so), you **MUST** follow these instructions carefully:
1. Your final response **MUST** be in your output (it is not a tool call).
1. Your final response **MUST** use the XML wrapper shown below, at your output
2. The opening XML tag **MUST** be the **FIRST** thing in your output
3. Do NOT output plain text, greetings, or explanations outside the XML tags
4. ALL content must be between the opening and closing XML tags

**Pre-Response Checklist:**
- [ ] Response starts with \`<ai-agent-${slotId}\` (no text before it)
- [ ] Content matches ${formatId} format requirements, including any schema if provided
- [ ] Response ends with \`</ai-agent-${slotId}>\`
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

Your final report/answer content must follow any instructions given to accurately and precisely reflect the information available. If you encountered limitations, tool failures that you couldn't overcome, or you were unable to complete certain aspects of the task, clearly state these limitations in your final report/answer.

In some cases, you may receive requests that are irrelevant to your instructions, such as greetings, casual conversation, or questions outside your domain. In such cases, be polite and helpful, and respond to the best of your knowledge, stating that the information provided is outside your scope, but always adhere to the final report/answer format and XML wrapper instructions provided above.

CRITICAL: You should deliver your final report/answer on your output with the given XML wrapper. Your final report/answer is NOT a tool call.
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

/**
 * Mandatory rules for JSON newlines.
 * Used in: internal-provider.ts buildInstructions()
 */
export const MANDATORY_JSON_NEWLINES_RULES = `### MANDATORY RULE FOR NEWLINES IN JSON STRINGS
- To add newlines in JSON string fields, use the \`\\n\` escape sequence within the string value.
- When your final report includes newlines, you MUST use the \`\\n\` escape sequence within the string value for every newline you want to add, instead of raw newline characters.
- Do not include raw newline characters in JSON string values (json becomes invalid); use '\\n' instead.`;
