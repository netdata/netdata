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
export const toolReminderMessage = (excludeProgress: string, finalReportTool: string): string =>
  `Reminder: do not end with plain text. Use an available tool${excludeProgress} to make progress. When ready to conclude, provide your final report/answer (${finalReportTool}).`;

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
export const emptyResponseRetryNotice = (finalReportTool: string): string =>
  `System notice: No progress made in this turn: no tools called and no final report/answer provided. To progress you MUST call tools or provide a final report/answer (${finalReportTool}). Review carefully the provided instructions and tools (if any), decide your next action(s), and follow the instructions precisely to progress. If you believe you called tools or provided a final report, it did not work: ensure the tool calls and final report are correctly formatted as per the instructions. Try again NOW.`;

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
  finalReportTool: string,
  format: string,
  formatDescription: string,
  contentGuidance: string
): string =>
  `System notice: call ${finalReportTool} with report_format="${format}" (${formatDescription}), and ${contentGuidance}.`;

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

Use this tool to update the user on your overall progress and next steps.

**Rules:**
- This tool is OPTIONAL — only call it when you have something meaningful to report
- ONLY call progress_report when you are ALSO calling other tools in the same turn
- NEVER call progress_report standalone (if you have no other tools to call, skip it entirely)
- NEVER combine progress_report with your final report/answer
- NEVER call more than one progress_report per turn
- Keep messages concise (max 15-20 words), no formatting or newlines

**Good examples:**
- Found the data about X, now searching for Y and Z.
- Discovered how X works, checking if it can also do Y or Z.
- Looks like X is not available, trying Y and Z instead.

**CRITICAL:** progress_report only informs the user; it does NOT perform actions. You must call other tools to actually do work.`;

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
  mode: 'xml' | 'xml-final';
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
  const { nonce, turn, maxTurns, tools, slotTemplates, progressSlot, mode } = payload;
  const toolList = tools.filter((t) => t.name !== 'agent__final_report' && t.name !== 'agent__progress_report');
  const lines: string[] = [];
  lines.push('# System Notice');
  lines.push('');
  lines.push(`This is turn No ${String(turn)}${maxTurns !== undefined ? ` of ${String(maxTurns)}` : ''}.`);
  lines.push('');

  if (mode === 'xml') {
    lines.push('## Available Tools for this Turn');
    if (toolList.length > 0) {
      const slotIds = slotTemplates
        .filter((s) => s.tools.some((t) => toolList.map((tt) => tt.name).includes(t)))
        .map((s) => s.slotId);
      lines.push('Replace XXXX with one of the available slot numbers and include your JSON body inside the tag.');
      lines.push(`Slots available: ${slotIds.join(', ')}`);
      lines.push('');
      toolList.forEach((t) => {
        const exampleSlot = `${nonce}-XXXX`;
        lines.push(`- tool \`${t.name}\`, schema:`);
        lines.push('```');
        lines.push(`<ai-agent-${exampleSlot} tool="${t.name}">`);
        lines.push(JSON.stringify(t.schema ?? {}, null, 2));
        lines.push(`</ai-agent-${exampleSlot}>`);
        lines.push('```');
        lines.push('');
      });
    } else {
      lines.push('No tools are available for this turn.');
      lines.push('');
    }

    if (progressSlot !== undefined) {
      lines.push('## Progress Updates');
      lines.push(PROGRESS_TOOL_INSTRUCTIONS_BRIEF);
      lines.push('');
      lines.push('```');
      lines.push(`<ai-agent-${progressSlot.slotId} tool="agent__progress_report">`);
      lines.push('Found X, now searching for Y');
      lines.push(`</ai-agent-${progressSlot.slotId}>`);
      lines.push('```');
      lines.push('');
    }
  }

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
 * Tool-based final_report instructions (for native mode).
 * Used in: internal-provider.ts buildInstructions() when toolTransport === 'native'
 */
export const finalReportToolInstructions = (
  _formatId: string,
  formatFields: string
): string => `## How to Deliver Your Final Report/Answer to the User

When your task is complete you MUST call the 'agent__final_report' tool to provide your final answer to the user.
All the content of your final report MUST be delivered using this tool.

Required fields:
${formatFields}

Include optional \`metadata\` only when explicitly relevant.
**CRITICAL:** The content of your final report MUST be delivered using this tool ONLY, not as part of your regular output.`;

/**
 * XML-based final_report instructions (for xml-final mode).
 * Used in: internal-provider.ts buildInstructions() when toolTransport === 'xml-final'
 */
export const finalReportXmlInstructions = (
  formatId: string,
  formatDescription: string,
  schemaBlock: string,
  sessionNonce?: string
): string => {
  const slotId = sessionNonce !== undefined ? `${sessionNonce}-FINAL` : 'NONCE-FINAL';
  // For slack-block-kit: show array example (messages array directly, no wrapper)
  // For json: show object example
  // For text formats: show text placeholder
  const exampleContent = formatId === 'slack-block-kit'
    ? '[ { "blocks": [ ... ] } ]'
    : formatId === 'json'
      ? '{ ... your JSON here ... }'
      : '[Your final report/answer here]';
  return `

## How to Deliver Your Final Report/Answer to the User

When your task is complete you MUST provide your final report/answer using XML tags as described below.

\`\`\`
<ai-agent-${slotId} tool="agent__final_report" format="${formatId}">
${exampleContent}
</ai-agent-${slotId}>
\`\`\`

**Final Report/Answer Format**
${formatId}** — ${formatDescription}
${schemaBlock}

**Final Report/Answer Checklist**
- [ ] The opening XML tag \`<ai-agent-${slotId}\` MUST be first in your response
- [ ] Your report content/payload matches the expected format (${formatId})
- [ ] Your output MUST end with the closing XML tag \`</ai-agent-${slotId}>\`
- [ ] Your entire report is between the opening and closing XML tags, not outside them`;
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
- Always respond with valid tool calls, even for your final report.
- You must provide your final report to the user using the agent__final_report tool, with the correct format (pay attention to formatting and newlines handling).
- The CONTENT of your final report must be delivered using the agent__final_report tool ONLY.`;

/**
 * Mandatory rules for XML final report.
 * Used in: internal-provider.ts buildInstructions() when toolTransport === 'xml-final'
 */
export const MANDATORY_XML_FINAL_RULES = `### MANDATORY RULE FOR FINAL REPORT
- You run in agentic mode, interfacing with software tools with specific formatting requirements.
- Always respond with valid tool calls for regular tools.
- Your final report MUST be delivered using XML tags as described above, NOT as a tool call.
- The CONTENT of your final report must be between the XML tags ONLY.`;

/**
 * Mandatory rules for JSON newlines.
 * Used in: internal-provider.ts buildInstructions()
 */
export const MANDATORY_JSON_NEWLINES_RULES = `### MANDATORY RULE FOR NEWLINES IN JSON STRINGS
- To add newlines in JSON string fields, use the \`\\n\` escape sequence within the string value.
- When your final report includes newlines, you MUST use the \`\\n\` escape sequence within the string value for every newline you want to add, instead of raw newline characters.
- Do not include raw newline characters in JSON string values (json becomes invalid); use '\\n' instead.`;
