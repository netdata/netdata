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
 * - TURN_FAILURE: Validation errors sent via TURN-FAILED feedback
 * - TOOL_RESULTS: Messages returned as tool execution results
 * - TASK_STATUS: Instructions for the agent__task_status tool
 * - XML_TEMPLATES: Templates for XML-mode tool/report rendering
 */

import { loadMandatoryRules, loadTaskStatusInstructions } from './prompts/loader.js';
// Re-export Slack rules from slack-block-kit.ts for backward compatibility
export { SLACK_BLOCK_KIT_MRKDWN_RULES, SLACK_BLOCK_KIT_MRKDWN_RULES_TITLE } from './slack-block-kit.js';

// =============================================================================
// TURN FAILURE MESSAGES
// Sent to LLM via TURN-FAILED: prefix when validation fails
// =============================================================================

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
// =============================================================================
// TASK STATUS INSTRUCTIONS
// Single source of truth for agent__task_status tool guidance.
// Used by: internal-provider.ts (system prompt, tool schema), xml-tools.ts (XML-NEXT)
// =============================================================================

export const TASK_STATUS_TOOL_INSTRUCTIONS = loadTaskStatusInstructions();

export const TASK_STATUS_TOOL_BATCH_RULES = `- Include at most one task_status per batch
- task_status updates the user; to perform actions, use other tools in the same batch
- task_status can be called standalone to track task progress`;

// XML System Notices (moved from xml-tools.ts to keep all LLM-facing strings here)
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
 * Format-specific required fields for tool-based instructions.
 */
export const FINAL_REPORT_FIELDS_JSON =
  '  - `report_format`: "json".\n  - `content_json`: MUST match the required JSON Schema exactly.';

export const FINAL_REPORT_FIELDS_SLACK =
  '  - `report_format`: "slack-block-kit".\n  - `messages`: array of Slack Block Kit messages (no plain `report_content`).\n    • Up to 20 messages, each with ≤50 blocks. Sections/context mrkdwn ≤2000 chars; headers plain_text ≤150.';

export const finalReportFieldsText = (formatId: string): string =>
  `  - \`report_format\`: "${formatId}".\n  - \`report_content\`: the content of your final report, in the requested format.`;

export const MANDATORY_XML_FINAL_RULES = loadMandatoryRules();
