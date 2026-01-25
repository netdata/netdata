/**
 * Reasoning test suite.
 * Tests for reasoning content handling and retry behavior with non-tool responses.
 * Note: Actual think-tag parsing tests are in the main phase2-runner.ts file.
 */

import type { AIAgentResult, AIAgentSessionConfig, Configuration, ConversationMessage } from '../../../types.js';
import type { HarnessTest } from '../infrastructure/index.js';

import {
  invariant,
  runWithExecuteTurnOverride,
  THINK_TAG_NONSTREAM_FRAGMENT,
  THINK_TAG_STREAM_FRAGMENT,
} from '../infrastructure/index.js';

// Shared constants
const FINAL_REPORT_TOOL = 'agent__final_report';
const FINAL_REPORT_CALL_ID = 'final-report-call';
const FINAL_REPORT_PRESENT_MSG = 'Final report should be present';
const REASONING_CONTENT = 'Model reasoning about the task.';
const AFTER_REASONING_MSG = 'After reasoning.';

/**
 * Reasoning tests covering:
 * - Reasoning content alongside final reports
 * - Response content with reasoning fragments
 * - Retry behavior when response has content but no tools
 */
export const REASONING_TESTS: HarnessTest[] = [];

// Test: Reasoning content with final report (suite version)
REASONING_TESTS.push({
  id: 'suite-reasoning-with-report',
  description: 'Suite: Session processes reasoning content alongside final report',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const toolCallId = FINAL_REPORT_CALL_ID;
    const reportContent = 'Report with reasoning.';
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 5,
      response: REASONING_CONTENT,
      messages: [
        {
          role: 'assistant',
          content: REASONING_CONTENT,
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: { report_format: 'markdown', report_content: reportContent },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: reportContent,
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 10, outputTokens: 8, totalTokens: 18 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Session with reasoning should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === 'Report with reasoning.', 'Report content should match');
  },
} satisfies HarnessTest);

// Test: Response content with reasoning fragments (suite version)
// Note: This tests that response content is handled correctly, not actual think-tag parsing
REASONING_TESTS.push({
  id: 'suite-reasoning-content-stream-fragment',
  description: 'Suite: Session handles response content with stream reasoning text',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const toolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 5,
      response: THINK_TAG_STREAM_FRAGMENT,
      messages: [
        {
          role: 'assistant',
          content: THINK_TAG_STREAM_FRAGMENT,
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: { report_format: 'markdown', report_content: THINK_TAG_STREAM_FRAGMENT },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: THINK_TAG_STREAM_FRAGMENT,
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 10, outputTokens: 8, totalTokens: 18 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Think tag stream should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
  },
} satisfies HarnessTest);

// Test: Response content with non-streaming reasoning fragments (suite version)
// Note: This tests that response content is handled correctly, not actual think-tag parsing
REASONING_TESTS.push({
  id: 'suite-reasoning-content-nonstream-fragment',
  description: 'Suite: Session handles response content with non-stream reasoning text',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    const toolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: true, finalAnswer: true },
      latencyMs: 5,
      response: THINK_TAG_NONSTREAM_FRAGMENT,
      messages: [
        {
          role: 'assistant',
          content: THINK_TAG_NONSTREAM_FRAGMENT,
          toolCalls: [{
            id: toolCallId,
            name: FINAL_REPORT_TOOL,
            parameters: { report_format: 'markdown', report_content: THINK_TAG_NONSTREAM_FRAGMENT },
          }],
        } as ConversationMessage,
        {
          role: 'tool',
          toolCallId,
          content: THINK_TAG_NONSTREAM_FRAGMENT,
        } as ConversationMessage,
      ],
      tokens: { inputTokens: 10, outputTokens: 8, totalTokens: 18 },
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Think tag non-stream should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
  },
} satisfies HarnessTest);

// Test: Content without tools triggers retry then succeeds (suite version)
// Note: Tests the retry path when LLM returns content but no tool calls
REASONING_TESTS.push({
  id: 'suite-reasoning-content-no-tools-retries',
  description: 'Suite: Response with content but no tools retries then succeeds',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.maxTurns = 2;
    sessionConfig.maxRetries = 2;
    const toolCallId = FINAL_REPORT_CALL_ID;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      // First: reasoning only (no tools)
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          response: REASONING_CONTENT,
          messages: [
            { role: 'assistant', content: REASONING_CONTENT } as ConversationMessage,
          ],
          tokens: { inputTokens: 10, outputTokens: 8, totalTokens: 18 },
        });
      }
      // Second: proper final report
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
              parameters: { report_format: 'markdown', report_content: AFTER_REASONING_MSG },
            }],
          } as ConversationMessage,
          {
            role: 'tool',
            toolCallId,
            content: AFTER_REASONING_MSG,
          } as ConversationMessage,
        ],
        tokens: { inputTokens: 10, outputTokens: 8, totalTokens: 18 },
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Reasoning-only retry should succeed');
    invariant(result.finalReport !== undefined, FINAL_REPORT_PRESENT_MSG);
    invariant(result.finalReport.content === AFTER_REASONING_MSG, 'Report after reasoning should match');
  },
} satisfies HarnessTest);
