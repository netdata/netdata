/**
 * Final report test suite.
 * Tests for final report validation, formats, and extraction.
 */

import type { AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage } from '../../../types.js';
import type { HarnessTest } from '../infrastructure/index.js';

import {
  invariant,
  runWithExecuteTurnOverride,
} from '../infrastructure/index.js';

/**
 * Final report tests covering:
 * - Report format validation (markdown, json, text, slack)
 * - Report extraction from tool calls
 * - Report extraction from text responses
 * - Invalid report handling and retries
 */
export const FINAL_REPORT_TESTS: HarnessTest[] = [];

// Shared constants
const FINAL_REPORT_TOOL = 'agent__final_report';
const VALID_REPORT_CONTENT = 'Valid final report content.';
const FINAL_REPORT_PRESENT_MSG = 'Final report should be present';

// Test: Valid final report via tool call (suite version)
FINAL_REPORT_TESTS.push({
  id: 'suite-final-report-valid-tool-call',
  description: 'Suite: Session succeeds with valid final report via tool call',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const finalFormat = sessionConfig.outputFormat;
    const reportContent = VALID_REPORT_CONTENT;
    const toolCallId = 'final-report-call';
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 5,
      response: '',
      messages: [
        {
          role: 'assistant',
          content: '',
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: {
              report_format: finalFormat,
              report_content: reportContent,
            },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: reportContent,
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Valid final report should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === VALID_REPORT_CONTENT, 'Final report content should match');
  },
} satisfies HarnessTest);

// Test: Missing final report format (suite version)
FINAL_REPORT_TESTS.push({
  id: 'suite-final-report-missing-format',
  description: 'Suite: Session handles missing report format gracefully',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const reportContent = 'Report without format specified.';
    const toolCallId = 'final-report-no-format';
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 5,
      response: '',
      messages: [
        {
          role: 'assistant',
          content: '',
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: {
              // Missing report_format - should default or fail gracefully
              report_content: reportContent,
            },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: reportContent,
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
    }));
  },
  expect: (result: AIAgentResult) => {
    // Missing report_format defaults to session's outputFormat - session succeeds
    invariant(result.success, 'Missing format should use default and succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
  },
} satisfies HarnessTest);

// Test: Final report with empty content still succeeds (suite version)
// Note: Empty content is accepted by the validation - the model decided to produce an empty report
FINAL_REPORT_TESTS.push({
  id: 'suite-final-report-empty-content',
  description: 'Suite: Session accepts final report with empty content',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const toolCallId = 'final-report-empty';
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 5,
      response: '',
      messages: [
        {
          role: 'assistant',
          content: '',
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: {
              report_format: 'markdown',
              report_content: '', // Empty content is allowed
            },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: '',
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
    }));
  },
  expect: (result: AIAgentResult) => {
    // Empty content is valid - the session succeeds
    invariant(result.success, 'Empty content is accepted');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    // Content might be empty or have default value - just verify it exists
  },
} satisfies HarnessTest);
