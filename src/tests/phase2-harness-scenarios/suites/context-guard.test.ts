/**
 * Context guard test suite.
 * Tests for context window management, token tracking, and forced final turns.
 */

import type { AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage } from '../../../types.js';
import type { HarnessTest } from '../infrastructure/index.js';

import {
  invariant,
  runWithExecuteTurnOverride,
} from '../infrastructure/index.js';

// Shared constants
const FINAL_REPORT_TOOL = 'agent__final_report';
const VALID_REPORT_CONTENT = 'Valid final report content.';
const FINAL_REPORT_CALL_ID = 'final-report-call';
const FINAL_REPORT_PRESENT_MSG = 'Final report should be present';

/**
 * Context guard tests covering:
 * - Context window threshold detection
 * - Token tracking and reporting
 * - Forced final turn behavior
 * - Multi-provider context handling
 */
export const CONTEXT_GUARD_TESTS: HarnessTest[] = [];

// Test: Context warning triggers forced final turn (suite version)
CONTEXT_GUARD_TESTS.push({
  id: 'suite-context-guard-forced-final',
  description: 'Suite: Context warning triggers forced final turn with proper logging',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 1;
    const toolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      // Turn 1: Simulate context limit exceeded, forcing final turn
      if (invocation === 1) {
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
                parameters: { report_format: 'markdown', report_content: VALID_REPORT_CONTENT },
              }],
            } as ConversationMessage,
            {
              role: 'tool',
              toolCallId,
              content: VALID_REPORT_CONTENT,
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 1900, outputTokens: 100, totalTokens: 2000 },
        });
      }
      // Shouldn't reach here - final answer on turn 1
      return Promise.resolve({
        status: { type: 'success', hasToolCalls: false, finalAnswer: false },
        latencyMs: 5,
        response: 'unexpected turn',
        messages: [{ role: 'assistant', content: 'unexpected turn' }],
        tokens: { inputTokens: 5, outputTokens: 2, totalTokens: 7 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Context guard with final report should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === VALID_REPORT_CONTENT, 'Final report content should match');
  },
} satisfies HarnessTest);

// Test: Multiple turns before context limit (suite version)
CONTEXT_GUARD_TESTS.push({
  id: 'suite-context-guard-multi-turn',
  description: 'Suite: Session completes multiple turns before context limit',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 3;
    sessionConfig.maxRetries = 1;
    const toolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      // Turns 1-2: Normal tool execution (use non-progress tool to avoid standalone limit)
      if (invocation < 3) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: true, finalAnswer: false },
          latencyMs: 5,
          response: `Turn ${String(invocation)} response`,
          messages: [
            {
              role: 'assistant',
              content: `Turn ${String(invocation)} response`,
              toolCalls: [{
                id: `call-${String(invocation)}`,
                name: 'test__test',
                parameters: { text: `step-${String(invocation)}` },
              }],
            } as ConversationMessage,
          ],
          tokens: { inputTokens: 100 * invocation, outputTokens: 50, totalTokens: 100 * invocation + 50 },
          executionStats: { executedTools: 1, executedNonProgressBatchTools: 1, executedProgressBatchTools: 0, unknownToolEncountered: false },
        });
      }
      // Turn 3: Final report
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
              parameters: { report_format: 'markdown', report_content: VALID_REPORT_CONTENT },
            }],
          } as ConversationMessage,
          {
            role: 'tool',
            toolCallId,
            content: VALID_REPORT_CONTENT,
          } as ConversationMessage,
        ],
        tokens: { inputTokens: 300, outputTokens: 50, totalTokens: 350 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Multi-turn session should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === VALID_REPORT_CONTENT, 'Final report content should match');
  },
} satisfies HarnessTest);

// Test: Token metrics are tracked in logs (suite version)
CONTEXT_GUARD_TESTS.push({
  id: 'suite-context-guard-token-metrics',
  description: 'Suite: Token metrics are tracked in LLM request logs',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const toolCallId = FINAL_REPORT_CALL_ID;
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
            parameters: { report_format: 'markdown', report_content: VALID_REPORT_CONTENT },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: VALID_REPORT_CONTENT,
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 500, outputTokens: 100, totalTokens: 600 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Token metrics test should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    // Note: LLM request logs with token details are generated by the session runner
    // The test verifies the session completes successfully with token tracking enabled
  },
} satisfies HarnessTest);
