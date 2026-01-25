/**
 * Error handling test suite.
 * Tests for retry behavior, error recovery, and failure modes.
 */

import type { AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage } from '../../../types.js';
import type { HarnessTest } from '../infrastructure/index.js';

import {
  invariant,
  MAX_RETRY_SUCCESS_RESULT,
  runWithExecuteTurnOverride,
} from '../infrastructure/index.js';

// Shared constants
const FINAL_REPORT_TOOL = 'agent__final_report';
const FINAL_REPORT_CALL_ID = 'final-report-call';
const FINAL_REPORT_PRESENT_MSG = 'Final report should be present';
const ERROR_RETRY_MSG = 'Error retry message';
const CORRECTED_REPORT_MSG = 'Corrected report.';

/**
 * Error handling tests covering:
 * - Recovery after transient errors (retry success)
 * - Multiple retry attempts before success
 * - Tool correction on retry
 */
export const ERROR_HANDLING_TESTS: HarnessTest[] = [];

// Test: Success after retry recovers session (suite version)
ERROR_HANDLING_TESTS.push({
  id: 'suite-error-retry-success',
  description: 'Suite: Session recovers after retry with valid final report',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 2;
    const toolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      // First attempt: error/retry trigger
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          response: ERROR_RETRY_MSG,
          messages: [
            { role: 'assistant', content: ERROR_RETRY_MSG } as ConversationMessage,
          ],
          tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
        });
      }
      // Second attempt: success with final report
      return Promise.resolve({
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
              parameters: { report_format: 'markdown', report_content: MAX_RETRY_SUCCESS_RESULT },
            }],
          } as ConversationMessage,
          {
            role: 'tool',
            toolCallId,
            content: MAX_RETRY_SUCCESS_RESULT,
          } as ConversationMessage,
        ],
        tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Session should succeed after retry');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === MAX_RETRY_SUCCESS_RESULT, 'Final report content should match');
  },
} satisfies HarnessTest);

// Test: Multiple retries before success (suite version)
ERROR_HANDLING_TESTS.push({
  id: 'suite-error-multiple-retries',
  description: 'Suite: Session succeeds after multiple retry attempts',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 3;
    sessionConfig.maxRetries = 3;
    const toolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      // Attempts 1-2: failures triggering retries
      if (invocation < 3) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          response: `Attempt ${String(invocation)} failed`,
          messages: [
            { role: 'assistant', content: `Attempt ${String(invocation)} failed` } as ConversationMessage,
          ],
          tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
        });
      }
      // Attempt 3: success
      return Promise.resolve({
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
              parameters: { report_format: 'markdown', report_content: MAX_RETRY_SUCCESS_RESULT },
            }],
          } as ConversationMessage,
          {
            role: 'tool',
            toolCallId,
            content: MAX_RETRY_SUCCESS_RESULT,
          } as ConversationMessage,
        ],
        tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Session should succeed after multiple retries');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
  },
} satisfies HarnessTest);

// Test: Retry with tool call that needs correction (suite version)
ERROR_HANDLING_TESTS.push({
  id: 'suite-error-tool-correction',
  description: 'Suite: Session recovers when tool call is corrected on retry',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 2;
    const toolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      // First attempt: invalid tool call (non-progress tool without final report)
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: false },
          latencyMs: 5,
          response: 'testing with invalid tool',
          messages: [
            {
              role: 'assistant',
              content: 'testing with invalid tool',
              toolCalls: [{
                id: 'test-call',
                name: 'test__test',
                parameters: { text: 'test' },
              }],
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
          executionStats: { executedTools: 1, executedNonProgressBatchTools: 1, executedProgressBatchTools: 0, unknownToolEncountered: false },
        });
      }
      // Second attempt: corrected with final report
      return Promise.resolve({
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
              parameters: { report_format: 'markdown', report_content: CORRECTED_REPORT_MSG },
            }],
          } as ConversationMessage,
          {
            role: 'tool',
            toolCallId,
            content: CORRECTED_REPORT_MSG,
          } as ConversationMessage,
        ],
        tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Session should succeed after tool correction');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === CORRECTED_REPORT_MSG, 'Corrected report content should match');
  },
} satisfies HarnessTest);
