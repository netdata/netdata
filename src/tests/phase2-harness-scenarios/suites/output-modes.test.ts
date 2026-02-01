/**
 * Output mode test suite.
 * Tests for chat mode stop behavior at the session turn level.
 */

import type { AIAgentResult, AIAgentSessionConfig, Configuration } from '../../../types.js';
import type { HarnessTest } from '../infrastructure/index.js';

import {
  invariant,
  runWithExecuteTurnOverride,
} from '../infrastructure/index.js';

export const OUTPUT_MODES_TESTS: HarnessTest[] = [];
const CHAT_OUTPUT = 'hello from chat mode';
const RETRY_OUTPUT = 'real output';
const FINAL_OUTPUT = 'final output';
const UNKNOWN_TOOL_FINAL_OUTPUT = 'final output after unknown tool';
const UNDEFINED_STOP_OUTPUT = 'final output after undefined stop';
const PARTIAL_OUTPUT = 'partial output';

// Test: chat mode stops on stopReason=stop with output and no tools
OUTPUT_MODES_TESTS.push({
  id: 'suite-output-mode-chat-stop-no-tools',
  description: 'Suite: Chat mode stops on stopReason=stop with output and no tools',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.outputMode = 'chat';
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 1;
    return await runWithExecuteTurnOverride(sessionConfig, () => Promise.resolve({
      status: { type: 'success', hasToolCalls: false, finalAnswer: false },
      latencyMs: 5,
      response: CHAT_OUTPUT,
      messages: [
        { role: 'assistant', content: CHAT_OUTPUT },
      ],
      tokens: { inputTokens: 4, outputTokens: 3, totalTokens: 7 },
      stopReason: 'stop',
    }));
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Chat mode should stop successfully');
    invariant(result.finalReport !== undefined, 'Chat mode should produce a final report payload');
    invariant(result.finalReport.content === CHAT_OUTPUT, 'Chat mode final report should match output');
  },
} satisfies HarnessTest);

// Test: chat mode retries when stopReason=stop but output is whitespace, then succeeds
OUTPUT_MODES_TESTS.push({
  id: 'suite-output-mode-chat-stop-empty-retry',
  description: 'Suite: Chat mode retries on stopReason=stop with empty output and then succeeds',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.outputMode = 'chat';
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 2;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          response: ' \n  ',
          messages: [
            { role: 'assistant', content: ' \n  ' },
          ],
          tokens: { inputTokens: 4, outputTokens: 1, totalTokens: 5 },
          stopReason: 'stop',
        });
      }
      return Promise.resolve({
        status: { type: 'success', hasToolCalls: false, finalAnswer: false },
        latencyMs: 5,
        response: RETRY_OUTPUT,
        messages: [
          { role: 'assistant', content: RETRY_OUTPUT },
        ],
        tokens: { inputTokens: 4, outputTokens: 2, totalTokens: 6 },
        stopReason: 'stop',
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Chat mode should succeed after retry');
    invariant(result.finalReport?.content === RETRY_OUTPUT, 'Chat mode should use the non-empty retry output');
  },
} satisfies HarnessTest);

// Test: chat mode continues when stopReason is not stop, then succeeds on stop
OUTPUT_MODES_TESTS.push({
  id: 'suite-output-mode-chat-non-stop-continues',
  description: 'Suite: Chat mode continues on non-stop stopReason and succeeds on stop',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.outputMode = 'chat';
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 2;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          response: PARTIAL_OUTPUT,
          messages: [
            { role: 'assistant', content: PARTIAL_OUTPUT },
          ],
          tokens: { inputTokens: 4, outputTokens: 2, totalTokens: 6 },
          stopReason: 'length',
        });
      }
      return Promise.resolve({
        status: { type: 'success', hasToolCalls: false, finalAnswer: false },
        latencyMs: 5,
        response: FINAL_OUTPUT,
        messages: [
          { role: 'assistant', content: FINAL_OUTPUT },
        ],
        tokens: { inputTokens: 4, outputTokens: 2, totalTokens: 6 },
        stopReason: 'stop',
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Chat mode should succeed after non-stop retry');
    invariant(result.finalReport?.content === FINAL_OUTPUT, 'Chat mode should use the stopReason=stop output');
  },
} satisfies HarnessTest);

// Test: chat mode continues when stopReason is undefined, then succeeds on stop
OUTPUT_MODES_TESTS.push({
  id: 'suite-output-mode-chat-undefined-stop-retries',
  description: 'Suite: Chat mode retries when stopReason is undefined and succeeds on stop',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.outputMode = 'chat';
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 2;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          response: PARTIAL_OUTPUT,
          messages: [
            { role: 'assistant', content: PARTIAL_OUTPUT },
          ],
          tokens: { inputTokens: 4, outputTokens: 2, totalTokens: 6 },
        });
      }
      return Promise.resolve({
        status: { type: 'success', hasToolCalls: false, finalAnswer: false },
        latencyMs: 5,
        response: UNDEFINED_STOP_OUTPUT,
        messages: [
          { role: 'assistant', content: UNDEFINED_STOP_OUTPUT },
        ],
        tokens: { inputTokens: 4, outputTokens: 2, totalTokens: 6 },
        stopReason: 'stop',
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Chat mode should succeed after undefined stopReason retry');
    invariant(result.finalReport?.content === UNDEFINED_STOP_OUTPUT, 'Chat mode should use the stopReason=stop output');
  },
} satisfies HarnessTest);

// Test: chat mode does not stop when unknown tool encountered (no tools of any kind condition fails)
OUTPUT_MODES_TESTS.push({
  id: 'suite-output-mode-chat-unknown-tool-prevents-stop',
  description: 'Suite: Chat mode retries when unknown tool is encountered despite stopReason=stop',
  execute: async (_configuration: Configuration, sessionConfig: AIAgentSessionConfig) => {
    sessionConfig.outputMode = 'chat';
    sessionConfig.maxTurns = 1;
    sessionConfig.maxRetries = 2;
    return await runWithExecuteTurnOverride(sessionConfig, ({ invocation }) => {
      if (invocation === 1) {
        return Promise.resolve({
          status: { type: 'success', hasToolCalls: false, finalAnswer: false },
          latencyMs: 5,
          response: 'attempt with unknown tool',
          messages: [
            { role: 'assistant', content: 'attempt with unknown tool' },
          ],
          tokens: { inputTokens: 4, outputTokens: 2, totalTokens: 6 },
          stopReason: 'stop',
          executionStats: { executedTools: 0, executedNonProgressBatchTools: 0, executedProgressBatchTools: 0, unknownToolEncountered: true },
        });
      }
      return Promise.resolve({
        status: { type: 'success', hasToolCalls: false, finalAnswer: false },
        latencyMs: 5,
        response: UNKNOWN_TOOL_FINAL_OUTPUT,
        messages: [
          { role: 'assistant', content: UNKNOWN_TOOL_FINAL_OUTPUT },
        ],
        tokens: { inputTokens: 4, outputTokens: 2, totalTokens: 6 },
        stopReason: 'stop',
      });
    });
  },
  expect: (result: AIAgentResult) => {
    invariant(result.success, 'Chat mode should succeed after retrying unknown tool attempt');
    invariant(result.finalReport?.content === UNKNOWN_TOOL_FINAL_OUTPUT, 'Chat mode should use the second attempt output');
  },
} satisfies HarnessTest);
