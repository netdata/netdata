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

import { renderPromptTemplate } from './prompts/templates.js';
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
export const FINAL_REPORT_JSON_REQUIRED = renderPromptTemplate('toolResultFinalReportJsonRequired', {});

/**
 * When Slack Block Kit messages array is missing.
 * Used in: session-turn-runner.ts via addTurnFailure
 *
 * CONDITION: final_report tool called && expectedFormat === 'slack-block-kit'
 *            && messagesArray is undefined or empty
 */
export const FINAL_REPORT_SLACK_MESSAGES_MISSING = renderPromptTemplate('toolResultFinalReportSlackMissing', {});

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
export const XML_WRAPPER_CALLED_AS_TOOL_RESULT = renderPromptTemplate('toolResultXmlWrapperCalledAsTool', {});

/**
 * Placeholder for tool output when context budget exceeded.
 * Returned as tool result to the LLM.
 * Used in: session-tool-executor.ts, session-turn-runner.ts
 *
 * CONDITION: Tool output would exceed context budget
 * (content.length > 0 && content !== TOOL_NO_OUTPUT checked to avoid counting)
 */
export const TOOL_NO_OUTPUT = renderPromptTemplate('toolOutputNoOutput', {});

const UNKNOWN_TOOL_FAILURE_PREFIX = 'Unknown tool `';

/**
 * Unknown tool name (tool call does not match any available tool).
 * Returned as tool result to the LLM.
 * Used in: session-tool-executor.ts, session-turn-runner.ts, tests
 */
export const unknownToolFailureMessage = (name: string): string => (
  renderPromptTemplate('toolResultUnknownTool', { tool_name: name })
);

export const isUnknownToolFailureMessage = (content: string): boolean =>
  content.includes(UNKNOWN_TOOL_FAILURE_PREFIX);

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
  return renderPromptTemplate('xmlPast', {
    entries: past.entries.map((entry) => ({
      slot_id: entry.slotId,
      tool: entry.tool,
      status: entry.status,
      duration_text: entry.durationText ?? '',
      request: entry.request,
      response: entry.response,
    })),
  });
};
