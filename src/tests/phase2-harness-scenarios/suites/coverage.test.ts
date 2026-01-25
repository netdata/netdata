/**
 * Coverage test suite.
 * Miscellaneous tests for code coverage of edge cases.
 */

import type { AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage } from '../../../types.js';
import type { HarnessTest } from '../infrastructure/index.js';

import {
  invariant,
  runWithExecuteTurnOverride,
  SECOND_TURN_FINAL_ANSWER,
} from '../infrastructure/index.js';

// Shared constants
const FINAL_REPORT_TOOL = 'agent__final_report';
const FINAL_REPORT_CALL_ID = 'final-report-call';
const FINAL_REPORT_PRESENT_MSG = 'Final report should be present';

/**
 * Coverage tests covering:
 * - Edge cases in session handling
 * - Token tracking edge cases
 * - Multi-turn edge cases
 * - Boundary conditions
 */
export const COVERAGE_TESTS: HarnessTest[] = [];

// Test: Session with minimal tokens (suite version)
COVERAGE_TESTS.push({
  id: 'suite-coverage-minimal-tokens',
  description: 'Suite: Session completes with minimal token counts',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const toolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 1,
      response: '',
      messages: [
        {
          role: 'assistant',
          content: '',
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: { report_format: 'markdown', report_content: 'Minimal.' },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: 'Minimal.',
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 1, outputTokens: 1, totalTokens: 2 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Minimal tokens should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
  },
} satisfies HarnessTest);

// Test: Second turn final answer (suite version)
COVERAGE_TESTS.push({
  id: 'suite-coverage-second-turn-final',
  description: 'Suite: Session succeeds with final answer on second turn',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 1;
    const toolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      // First turn: tool execution without final report
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: false },
          latencyMs: 5,
          response: 'Processing...',
          messages: [
            {
              role: 'assistant',
              content: 'Processing...',
              toolCalls: [{
                id: 'test-call',
                name: 'test__test',
                parameters: { text: 'work' },
              }],
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
          executionStats: { executedTools: 1, executedNonProgressBatchTools: 1, executedProgressBatchTools: 0, unknownToolEncountered: false },
        });
      }
      // Second turn: final report
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
              parameters: { report_format: 'markdown', report_content: SECOND_TURN_FINAL_ANSWER },
            }],
          } as ConversationMessage,
          {
            role: 'tool',
            toolCallId,
            content: SECOND_TURN_FINAL_ANSWER,
          } as ConversationMessage,
        ],
        tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Second turn final answer should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === SECOND_TURN_FINAL_ANSWER, 'Second turn content should match');
  },
} satisfies HarnessTest);

// Test: Large token count handling (suite version)
COVERAGE_TESTS.push({
  id: 'suite-coverage-large-tokens',
  description: 'Suite: Session handles large token counts correctly',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const toolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 100,
      response: '',
      messages: [
        {
          role: 'assistant',
          content: '',
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: { report_format: 'markdown', report_content: 'Large response.' },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: 'Large response.',
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 50000, outputTokens: 10000, totalTokens: 60000 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Large token counts should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
  },
} satisfies HarnessTest);

// Test: Zero latency response (suite version)
COVERAGE_TESTS.push({
  id: 'suite-coverage-zero-latency',
  description: 'Suite: Session handles zero latency response',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const toolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 0,
      response: '',
      messages: [
        {
          role: 'assistant',
          content: '',
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: { report_format: 'markdown', report_content: 'Instant.' },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: 'Instant.',
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 5, outputTokens: 3, totalTokens: 8 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Zero latency should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
  },
} satisfies HarnessTest);
